package opentelemetry

import (
	"context"
	"errors"
	"github.com/caddyserver/caddy/v2"
	"os"
	"testing"
)

func TestOpenTelemetry_newOpenTelemetryWrapper(t *testing.T) {
	type fields struct {
		tracesProtocol string
		propagators    string
	}

	tests := []struct {
		name     string
		setEnv   func() error
		unsetEnv func() error
		fields   fields
		wantErr  bool
	}{
		{
			name: "With environment variables",
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
			fields:  fields{},
			wantErr: false,
		},
		{
			name:     "Without environment variables",
			setEnv:   func() error { return nil },
			unsetEnv: func() error { return nil },
			fields: fields{
				tracesProtocol: "grpc",
				propagators:    "tracecontext,baggage",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
			defer cancel()

			if err := tt.setEnv(); err != nil {
				t.Errorf("Environment variable set error: %v", err)
			}
			defer func() {
				if err := tt.unsetEnv(); err != nil {
					t.Errorf("Environment variable set error: %v", err)
				}
			}()

			var otw openTelemetryWrapper
			var err error

			if otw, err = newOpenTelemetryWrapper(ctx,
				"",
				tt.fields.propagators,
				"my-tracer",
				"my-span",
				tracerExporterConfig{
					exporterTracesProtocol: tt.fields.tracesProtocol,
					insecure:               true,
				},
			); (err != nil) != tt.wantErr {
				t.Errorf("newOpenTelemetryWrapper() error = %v, wantErrType %v", err, tt.wantErr)
			}

			if otw.tracer == nil {
				t.Errorf("Tracer should not be empty")
			}

			if otw.propagators == nil {
				t.Errorf("Propagators should not be empty")
			}
		})
	}
}

func TestOpenTelemetry_newOpenTelemetryWrapper_Error(t *testing.T) {
	type fields struct {
		tracesProtocol string
		propagators    string
	}

	tests := []struct {
		name        string
		fields      fields
		wantErrType error
		setEnv      func() error
		unsetEnv    func() error
	}{
		{
			name: "With OTEL_EXPORTER_OTLP_PROTOCOL environment variables only",
			setEnv: func() error {
				return os.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "grpc")
			},
			unsetEnv: func() error {
				return os.Unsetenv("OTEL_EXPORTER_OTLP_PROTOCOL")
			},
			fields:      fields{},
			wantErrType: ErrUnspecifiedPropagators,
		},
		{
			name: "With OTEL_PROPAGATORS environment variables only",
			setEnv: func() error {
				return os.Setenv("OTEL_PROPAGATORS", "tracecontext,baggage")
			},
			unsetEnv: func() error {
				return os.Unsetenv("OTEL_PROPAGATORS")
			},
			fields:      fields{},
			wantErrType: ErrUnspecifiedTracesProtocol,
		},
		{
			name:     "With tracesProtocol only",
			setEnv:   func() error { return nil },
			unsetEnv: func() error { return nil },
			fields: fields{
				tracesProtocol: "grpc",
			},
			wantErrType: ErrUnspecifiedPropagators,
		},
		{
			name:     "With propagators only",
			setEnv:   func() error { return nil },
			unsetEnv: func() error { return nil },
			fields: fields{

				propagators: "tracecontext,baggage",
			},
			wantErrType: ErrUnspecifiedTracesProtocol,
		},
		{
			name:     "Not supported protocol",
			setEnv:   func() error { return nil },
			unsetEnv: func() error { return nil },
			fields: fields{
				propagators:    "tracecontext,baggage",
				tracesProtocol: "non supported",
			},
			wantErrType: ErrNonSupportedTracesProtocol,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
			defer cancel()

			if err := tt.setEnv(); err != nil {
				t.Errorf("Environment variable set error: %v", err)
			}
			defer func() {
				if err := tt.unsetEnv(); err != nil {
					t.Errorf("Environment variable set error: %v", err)
				}
			}()

			_, err := newOpenTelemetryWrapper(ctx,
				"",
				tt.fields.propagators,
				"my-tracer",
				"my-span",
				tracerExporterConfig{
					exporterTracesProtocol: tt.fields.tracesProtocol,
					insecure:               true,
				},
			)

			if !errors.Is(err, tt.wantErrType) {
				t.Errorf("newOpenTelemetryWrapper() error is %v, expected %v", err, tt.wantErrType)
			}
		})
	}
}

func Test_openTelemetryWrapper_newResource_WithServiceName(t *testing.T) {
	res, err := (&openTelemetryWrapper{}).newResource(context.Background(), "MyService", "TestEngine", "Version 1")

	if err != nil {
		t.Errorf("can not create resource: %v", err)
	}

	const expectedAttributesNumber = 6
	if len(res.Attributes()) != expectedAttributesNumber {
		t.Errorf("resource should have %d attributes, has : %v", expectedAttributesNumber, len(res.Attributes()))
	}

	attributesMap := make(map[string]string)
	for i := 0; i < expectedAttributesNumber; i++ {
		attributesMap[string(res.Attributes()[i].Key)] = res.Attributes()[i].Value.AsString()
	}

	for k, v := range map[string]string{
		"telemetry.sdk.language": "go",
		"telemetry.sdk.name":     "opentelemetry",
		"webengine.description":  "Version 1",
		"webengine.name":         "TestEngine",
		"service.name":           "MyService",
	} {
		if attributesMap[k] != v {
			t.Errorf("attribute %v is %v, expeted %v", k, attributesMap[k], v)
		}
	}
}
