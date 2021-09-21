package opentelemetry

import (
	"errors"
	"fmt"
	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"go.uber.org/zap"
	"net/http"
	"strings"
)

func init() {
	caddy.RegisterModule(OpenTelemetry{})
	httpcaddyfile.RegisterHandlerDirective("opentelemetry", parseCaddyfile)
}

// OpenTelemetry implements an HTTP handler that adds support for the opentelemetry tracing.
// It is responsible for the injection and propagation of the tracing contexts.
// OpenTelemetry module can be configured via environment variables https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/sdk-environment-variables.md. Some values can be overwritten with values from the configuration file.
type OpenTelemetry struct {
	// TracerName is a tracer instance name. It will be used instead of the default.
	TracerName string `json:"tracer_name"`
	// SpanName is a span name. It SHOULD follow the naming guideline https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/trace/api.md#span
	SpanName string `json:"span_name"`
	// ServiceName will overwrite service name from environment variables OTEL_SERVICE_NAME, OTEL_RESOURCE_ATTRIBUTES.
	ServiceName string `json:"service_name"`
	// Propagators support W3C TraceContext(tracecontext) and W3C Baggage(baggage). Environment variable OTEL_PROPAGATORS (please check status here https://github.com/open-telemetry/opentelemetry-go/issues/1698).
	Propagators string `json:"propagators"`

	// See details for the exporter configuration variables here: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md.

	// ExporterTracesEndpoint can overwrite values defined by environment variables: OTEL_EXPORTER_OTLP_ENDPOINT, OTEL_EXPORTER_OTLP_TRACES_ENDPOINT.
	ExporterTracesEndpoint string `json:"exporter_traces_endpoint"`
	// ExporterTracesProtocol is an exporter protocol. Currently, only "grpc" is supported. Corresponded environment variables are OTEL_EXPORTER_OTLP_PROTOCOL and OTEL_EXPORTER_OTLP_TRACES_PROTOCOL.
	ExporterTracesProtocol string `json:"exporter_traces_protocol"`
	// ExporterInsecure can overwrite values defined by environment variables: OTEL_EXPORTER_OTLP_INSECURE OTEL_EXPORTER_OTLP_SPAN_INSECURE.
	ExporterInsecure string `json:"exporter_insecure"`
	// ExporterCertificate can overwrite values defined by environment variables: OTEL_EXPORTER_OTLP_CERTIFICATE,OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE.
	ExporterCertificate string `json:"exporter_certificate"`

	// otel implements opentelemetry related logic.
	otel openTelemetryWrapper

	logger *zap.Logger
}

// CaddyModule returns the Caddy module information.
func (OpenTelemetry) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.opentelemetry",
		New: func() caddy.Module { return new(OpenTelemetry) },
	}
}

// Provision implements caddy.Provisioner.
func (ot *OpenTelemetry) Provision(ctx caddy.Context) error {
	ot.logger = ctx.Logger(ot)

	var err error

	insecure := false
	if ot.ExporterInsecure == strings.ToLower("true") {
		insecure = true
	}

	ot.otel, err = newOpenTelemetryWrapper(ctx,
		ot.ServiceName,
		ot.Propagators,
		ot.TracerName,
		ot.SpanName,
		tracerExporterConfig{
			exporterTracesProtocol: ot.ExporterTracesProtocol,
			exporterCertificate:    ot.ExporterCertificate,
			exporterTracesEndpoint: ot.ExporterTracesEndpoint,
			insecure:               insecure,
		},
	)

	if err != nil {
		ot.logger.Error("OpenTelemetry Provision error", zap.Error(err))
	}
	return err
}

// Cleanup implements caddy.CleanerUpper and closes any idle connections. It calls Shutdown method for a trace provider https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/trace/sdk.md#shutdown.
func (ot *OpenTelemetry) Cleanup() error {
	if err := ot.otel.cleanup(ot.logger); err != nil {
		return fmt.Errorf("tracerProvider shutdown: %w", err)
	}
	return nil
}

// Validate implements caddy.Validator.
func (ot *OpenTelemetry) Validate() error {
	if ot.otel.tracer == nil {
		return errors.New("openTelemetry tracer is nil")
	}

	return nil
}

// ServeHTTP implements caddyhttp.MiddlewareHandler.
func (ot *OpenTelemetry) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	return ot.otel.ServeHTTP(w, r, next)
}

// UnmarshalCaddyfile sets up the module from Caddyfile tokens.
func (ot *OpenTelemetry) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	setParameter := func(d *caddyfile.Dispenser, val *string) error {
		if d.NextArg() {
			*val = d.Val()
		} else {
			return d.ArgErr()
		}
		if d.NextArg() {
			return d.ArgErr()
		}
		return nil
	}

	paramsMap := map[string]*string{
		"tracer_name":              &ot.TracerName,
		"span_name":                &ot.SpanName,
		"propagators":              &ot.Propagators,
		"service_name":             &ot.ServiceName,
		"exporter_traces_protocol": &ot.ExporterTracesProtocol,
		"exporter_traces_endpoint": &ot.ExporterTracesEndpoint,
		"exporter_insecure":        &ot.ExporterInsecure,
		"exporter_certificate":     &ot.ExporterCertificate,
	}

	for d.Next() {
		args := d.RemainingArgs()
		if len(args) > 0 {
			return d.ArgErr()
		}

		for d.NextBlock(0) {

			if dst, ok := paramsMap[d.Val()]; ok {
				if err := setParameter(d, dst); err != nil {
					return err
				}
			} else {
				return d.ArgErr()
			}
		}
	}
	return nil
}

func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var m OpenTelemetry
	err := m.UnmarshalCaddyfile(h.Dispenser)
	return &m, err
}

// Interface guards
var (
	_ caddy.Provisioner           = (*OpenTelemetry)(nil)
	_ caddy.Validator             = (*OpenTelemetry)(nil)
	_ caddyhttp.MiddlewareHandler = (*OpenTelemetry)(nil)
	_ caddyfile.Unmarshaler       = (*OpenTelemetry)(nil)
)
