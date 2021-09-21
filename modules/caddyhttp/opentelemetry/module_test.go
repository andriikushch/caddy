package opentelemetry

import (
	"context"
	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/zap"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestOpenTelemetry_UnmarshalCaddyfile(t *testing.T) {
	tests := []struct {
		name                   string
		tracerName             string
		spanName               string
		tracesProtocol         string
		propagators            string
		serviceName            string
		exporterCertificate    string
		exporterInsecure       string
		exporterTracesEndpoint string
		d                      *caddyfile.Dispenser
		wantErr                bool
	}{
		{
			name:                   "Full config",
			tracerName:             "my-tracer",
			spanName:               "my-span",
			tracesProtocol:         "grpc",
			propagators:            "tracecontext",
			serviceName:            "my-service",
			exporterCertificate:    "my-cert",
			exporterInsecure:       "true",
			exporterTracesEndpoint: "localhost:8080",
			d: caddyfile.NewTestDispenser(`
opentelemetry {
	tracer_name my-tracer
	span_name my-span
	exporter_traces_protocol grpc
	service_name my-service
	propagators tracecontext
	exporter_certificate my-cert
	exporter_insecure true
	exporter_traces_endpoint localhost:8080
}`),
			wantErr: false,
		},
		{
			name:       "Only tracer name in the config",
			tracerName: "my-tracer",
			d: caddyfile.NewTestDispenser(`
opentelemetry {
	tracer_name my-tracer
}`),
			wantErr: false,
		},
		{
			name:     "Only span name in the config",
			spanName: "my-span",
			d: caddyfile.NewTestDispenser(`
opentelemetry {
	span_name my-span
}`),
			wantErr: false,
		},
		{
			name: "Empty config",
			d: caddyfile.NewTestDispenser(`
opentelemetry {
}`),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ot := &OpenTelemetry{}
			if err := ot.UnmarshalCaddyfile(tt.d); (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalCaddyfile() error = %v, wantErrType %v", err, tt.wantErr)
			}

			if ot.SpanName != tt.spanName {
				t.Errorf("UnmarshalCaddyfile() SpanName = %v, want SpanName %v", ot.SpanName, tt.spanName)
			}

			if ot.TracerName != tt.tracerName {
				t.Errorf("UnmarshalCaddyfile() TracerName = %v, want TracerName %v", ot.TracerName, tt.tracerName)
			}

			if ot.ServiceName != tt.serviceName {
				t.Errorf("UnmarshalCaddyfile() ServiceName = %v, want ServiceName %v", ot.ServiceName, tt.serviceName)
			}

			if ot.ExporterTracesProtocol != tt.tracesProtocol {
				t.Errorf("UnmarshalCaddyfile() ExporterTracesProtocol = %v, want ExporterTracesProtocol %v", ot.ExporterTracesProtocol, tt.tracesProtocol)
			}

			if ot.ExporterCertificate != tt.exporterCertificate {
				t.Errorf("UnmarshalCaddyfile() ExporterCertificate = %v, want ExporterCertificate %v", ot.ExporterCertificate, tt.exporterCertificate)
			}

			if ot.ExporterInsecure != tt.exporterInsecure {
				t.Errorf("UnmarshalCaddyfile() ExporterInsecure = %v, want ExporterInsecure %v", ot.ExporterInsecure, tt.exporterInsecure)
			}

			if ot.ExporterTracesEndpoint != tt.exporterTracesEndpoint {
				t.Errorf("UnmarshalCaddyfile() ExporterTracesEndpoint = %v, want ExporterTracesEndpoint %v", ot.ExporterTracesEndpoint, tt.exporterTracesEndpoint)
			}

			if ot.Propagators != tt.propagators {
				t.Errorf("UnmarshalCaddyfile() Propagators = %v, want Propagators %v", ot.Propagators, tt.propagators)
			}
		})
	}
}

func TestOpenTelemetry_UnmarshalCaddyfile_Error(t *testing.T) {
	tests := []struct {
		name    string
		d       *caddyfile.Dispenser
		wantErr bool
	}{
		{
			name: "Unknown parameter",
			d: caddyfile.NewTestDispenser(`
		opentelemetry {
			foo bar
		}`),
			wantErr: true,
		},
		{
			name: "Missed argument",
			d: caddyfile.NewTestDispenser(`
opentelemetry {
	span_name
}`),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ot := &OpenTelemetry{}
			if err := ot.UnmarshalCaddyfile(tt.d); (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalCaddyfile() error = %v, wantErrType %v", err, tt.wantErr)
			}
		})
	}
}

func TestOpenTelemetry_Provision(t *testing.T) {
	type fields struct {
		setEnv         func() error
		unsetEnv       func() error
		logger         *zap.Logger
		tracerProvider *sdktrace.TracerProvider
		propagator     propagation.TextMapPropagator

		TracerName     string
		SpanName       string
		TracesProtocol string
		Propagators    string
	}

	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "Provision with environment variables",
			fields: fields{
				setEnv: func() error {
					if err := os.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "grpc"); err != nil {
						return err
					}

					if err := os.Setenv("OTEL_PROPAGATORS", "tracecontext,baggage"); err != nil {
						return err
					}

					return nil
				},
				unsetEnv: func() error {
					if err := os.Unsetenv("OTEL_EXPORTER_OTLP_PROTOCOL"); err != nil {
						return err
					}

					if err := os.Unsetenv("OTEL_PROPAGATORS"); err != nil {
						return err
					}

					return nil
				},
			},

			wantErr: false,
		},
		{
			name: "Provision without environment variables",
			fields: fields{
				setEnv:   func() error { return nil },
				unsetEnv: func() error { return nil },

				TracerName:     "MyTracerName",
				SpanName:       "MySpanName",
				TracesProtocol: "grpc",
				Propagators:    "tracecontext,baggage",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
			defer cancel()

			if err := tt.fields.setEnv(); err != nil {
				t.Errorf("Environment variable set error: %v", err)
			}

			defer func() {
				if err := tt.fields.unsetEnv(); err != nil {
					t.Errorf("Environment variable set error: %v", err)
				}
			}()

			ot := &OpenTelemetry{
				logger:                 tt.fields.logger,
				TracerName:             tt.fields.TracerName,
				SpanName:               tt.fields.SpanName,
				ExporterTracesProtocol: tt.fields.TracesProtocol,
				Propagators:            tt.fields.Propagators,
			}
			if err := ot.Provision(ctx); (err != nil) != tt.wantErr {
				t.Errorf("Provision() error = %v, wantErrType %v", err, tt.wantErr)
			}

			if ot.logger == nil {
				t.Error("Logger should not be emtpy")
			}

			if ot.otel.tracer == nil {
				t.Error("Tracer should not be nil")
			}
		})
	}
}

func TestOpenTelemetry_ServeHTTP_Propagation_Without_Initial_Headers(t *testing.T) {
	ot := &OpenTelemetry{
		ExporterTracesProtocol: "grpc",
		TracerName:             "myTracer",
		SpanName:               "mySpan",
		Propagators:            "tracecontext,baggage",
	}

	req := httptest.NewRequest("GET", "https://example.com/foo", nil)
	w := httptest.NewRecorder()

	var handler caddyhttp.HandlerFunc = func(writer http.ResponseWriter, request *http.Request) error {
		traceparent := request.Header.Get("Traceparent")
		if traceparent == "" || strings.HasPrefix(traceparent, "00-00000000000000000000000000000000-0000000000000000") {
			t.Errorf("Invalid traceparent: %v", traceparent)
		}

		return nil
	}

	ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
	defer cancel()

	if err := ot.Provision(ctx); err != nil {
		t.Errorf("Provision error: %v", err)
	}

	if err := ot.ServeHTTP(w, req, handler); err != nil {
		t.Errorf("ServeHTTP error: %v", err)
	}
}

func TestOpenTelemetry_ServeHTTP_Propagation_With_Initial_Headers(t *testing.T) {
	ot := &OpenTelemetry{
		ExporterTracesProtocol: "grpc",
		TracerName:             "myTracer",
		SpanName:               "mySpan",
		Propagators:            "tracecontext,baggage",
	}

	req := httptest.NewRequest("GET", "https://example.com/foo", nil)
	req.Header.Set("traceparent", "00-11111111111111111111111111111111-1111111111111111-01")
	w := httptest.NewRecorder()

	var handler caddyhttp.HandlerFunc = func(writer http.ResponseWriter, request *http.Request) error {
		traceparent := request.Header.Get("Traceparent")
		if !strings.HasPrefix(traceparent, "00-11111111111111111111111111111111") {
			t.Errorf("Invalid traceparent: %v", traceparent)
		}

		return nil
	}

	ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
	defer cancel()

	if err := ot.Provision(ctx); err != nil {
		t.Errorf("Provision error: %v", err)
	}

	if err := ot.ServeHTTP(w, req, handler); err != nil {
		t.Errorf("ServeHTTP error: %v", err)
	}
}
