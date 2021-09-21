package opentelemetry

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	caddycmd "github.com/caddyserver/caddy/v2/cmd"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc/credentials"
	"net/http"
	"os"
	"strings"
)

const (
	envOtelPropagators = "OTEL_PROPAGATORS"

	envOtelExporterOtlpProtocol       = "OTEL_EXPORTER_OTLP_PROTOCOL"
	envOtelExporterOtlpTracesProtocol = "OTEL_EXPORTER_OTLP_TRACES_PROTOCOL"

	envOtelExporterOtlpCertificate       = "OTEL_EXPORTER_OTLP_CERTIFICATE"
	envOtelExporterOtlpTracesCertificate = "OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE"

	webEngineName      = "Caddy"
	defaultServiceName = "caddyService"
	defaultSpanName    = "handler"
)

var (
	ErrUnspecifiedTracesProtocol  = errors.New("unspecified opentelemetry traces protocol")
	ErrNonSupportedTracesProtocol = errors.New("non supported opentelemetry traces protocol")
	ErrUnspecifiedPropagators     = errors.New("unspecified opentelemtry propagators")
)

type tracerExporterConfig struct {
	exporterTracesProtocol string
	exporterCertificate    string
	exporterTracesEndpoint string
	insecure               bool
}

// openTelemetryWrapper is responsible for the tracing injection, extraction and propagation.
type openTelemetryWrapper struct {
	tracer      trace.Tracer
	propagators propagation.TextMapPropagator

	// tracerProviderKey identifies tracerProvider instance in the cache, it will allow to reuse it in the multiple handlers.
	tracerProviderKey string
	spanName          string
}

// newOpenTelemetryWrapper is responsible for the openTelemetryWrapper initialization using provided configuration.
func newOpenTelemetryWrapper(
	ctx context.Context,
	serviceName string,
	propagators string,
	tracerName string,
	spanName string,
	cfg tracerExporterConfig,
) (openTelemetryWrapper, error) {
	if serviceName == "" {
		serviceName = defaultServiceName
	}

	if spanName == "" {
		spanName = defaultSpanName
	}

	ot := openTelemetryWrapper{
		spanName: spanName,
	}

	res, err := ot.newResource(ctx, serviceName, webEngineName, caddycmd.CaddyVersion())
	if err != nil {
		return ot, fmt.Errorf("creating resource error: %w", err)
	}

	// handle exporter related configuration
	if cfg.exporterTracesProtocol == "" {
		cfg.exporterTracesProtocol = ot.getTracesProtocolFromEnv()
	}

	if cfg.exporterTracesProtocol == "" {
		return ot, ErrUnspecifiedTracesProtocol
	}

	traceExporter, err := ot.getTracerExporter(ctx, cfg)
	if err != nil {
		return ot, fmt.Errorf("creating trace exporter error: %w", err)
	}

	// handle propagators related configuration
	if propagators == "" {
		propagators = os.Getenv(envOtelPropagators)
	}

	if propagators == "" {
		return ot, ErrUnspecifiedPropagators
	}

	ot.propagators = ot.getPropagators(propagators)

	// handle tracer provider registry
	ot.tracerProviderKey = fmt.Sprintf("%s-%s-%s-%v-%s-%s",
		serviceName,
		tracerName,
		cfg.exporterTracesProtocol,
		cfg.insecure,
		cfg.exporterCertificate,
		cfg.exporterTracesEndpoint,
	)

	ot.tracer = defaultTracerProviderCache.getTracerProvider(
		ot.tracerProviderKey,
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	).Tracer(tracerName)

	return ot, nil
}

// ServeHTTP extract current tracing context or create a new one. And propagate it to the wrapped next handler.
func (ot *openTelemetryWrapper) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	// It will be default span kind as for now. Proper span kind (Span.Kind.LOAD_BALANCER (PROXY/SIDECAR)) is being discussed here https://github.com/open-telemetry/opentelemetry-specification/issues/51.
	ctx, span := ot.tracer.Start(
		ot.propagators.Extract(r.Context(), propagation.HeaderCarrier(r.Header)),
		ot.spanName,
	)
	defer span.End()

	ot.propagators.Inject(ctx, propagation.HeaderCarrier(r.Header))

	return next.ServeHTTP(w, r)
}

// cleanup flush all remaining data and shutdown a tracerProvider
func (ot *openTelemetryWrapper) cleanup(logger *zap.Logger) error {
	return defaultTracerProviderCache.cleanupTracerProvider(ot.tracerProviderKey, logger)
}

// newResource creates a resource that describe current handler instance and merge it with a default attributes value.
func (ot *openTelemetryWrapper) newResource(
	ctx context.Context,
	serviceName,
	webEngineName,
	webEngineDescription string,
) (*resource.Resource, error) {
	option := resource.WithAttributes(
		semconv.ServiceNameKey.String(serviceName),
		semconv.WebEngineNameKey.String(webEngineName),
		semconv.WebEngineDescriptionKey.String(webEngineDescription),
	)

	caddyResource, err := resource.New(ctx,
		option,
	)

	if err != nil {
		return nil, err
	}

	return resource.Merge(resource.Default(), caddyResource)
}

// getTracerExporter returns protocol specific exporter. Error if protocol is not supported by current module implementation.
func (ot *openTelemetryWrapper) getTracerExporter(ctx context.Context, cfg tracerExporterConfig) (*otlptrace.Exporter, error) {

	switch cfg.exporterTracesProtocol {
	case "grpc":
		var opts []otlptracegrpc.Option

		if cfg.insecure {
			opts = append(opts, otlptracegrpc.WithInsecure())
		} else {
			if cfg.exporterCertificate != "" {
				transportCredentials, err := credentials.NewClientTLSFromFile(cfg.exporterCertificate, "")
				if err != nil {
					return nil, fmt.Errorf("credentials creation error: %w", err)
				}
				opts = append(opts, otlptracegrpc.WithTLSCredentials(transportCredentials))
			} else if !ot.isCertificateHeaderSet() {
				var tlsConf tls.Config
				transportCredentials := credentials.NewTLS(&tlsConf)
				opts = append(opts, otlptracegrpc.WithTLSCredentials(transportCredentials))
			}
		}

		if cfg.exporterTracesEndpoint != "" {
			opts = append(opts, otlptracegrpc.WithEndpoint(cfg.exporterTracesEndpoint))
		}

		return otlptracegrpc.New(ctx, opts...)
	default:
		return nil, fmt.Errorf("%w: tracesProtocol %s", ErrNonSupportedTracesProtocol, cfg.exporterTracesProtocol)
	}
}

// getTracesProtocolFromEnv returns opentelemetry exporter otlp protocol, if it is specified via environment variable, empty otherwise.
func (ot *openTelemetryWrapper) getTracesProtocolFromEnv() string {
	protocol := os.Getenv(envOtelExporterOtlpTracesProtocol)
	if protocol == "" {
		protocol = os.Getenv(envOtelExporterOtlpProtocol)
	}

	return protocol
}

func (ot *openTelemetryWrapper) isCertificateHeaderSet() bool {
	return os.Getenv(envOtelExporterOtlpCertificate) != "" || os.Getenv(envOtelExporterOtlpTracesCertificate) != ""
}

// getPropagators deduplicate propagators, according to the specification https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/sdk-environment-variables.md#general-sdk-configuration.
// propagators is a "," separated string ex: "baggage,tracecontext".
// This method supports only "baggage" and "tracecontext" values.
func (ot *openTelemetryWrapper) getPropagators(propagators string) propagation.TextMapPropagator {
	// deduplicationMap filters duplicated propagator
	deduplicationMap := make(map[string]struct{})

	// store unique values
	var propagatorsList []propagation.TextMapPropagator

	for _, v := range strings.Split(propagators, ",") {
		propagatorName := strings.TrimSpace(v)
		if _, ok := deduplicationMap[propagatorName]; !ok {
			deduplicationMap[propagatorName] = struct{}{}
			switch propagatorName {
			case "baggage":
				propagatorsList = append(propagatorsList, propagation.Baggage{})
			case "tracecontext":
				propagatorsList = append(propagatorsList, propagation.TraceContext{})
			}
		}
	}

	return propagation.NewCompositeTextMapPropagator(propagatorsList...)
}
