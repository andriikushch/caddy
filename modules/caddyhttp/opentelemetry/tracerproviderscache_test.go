package opentelemetry

import (
	"github.com/caddyserver/caddy/v2"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"testing"
)

func Test_tracersProviderCache_getTracerProvider(t *testing.T) {
	tpc := tracerProviderCache{
		tracerProviders:        make(map[string]*sdktrace.TracerProvider),
		tracerProvidersCounter: make(map[string]int),
	}

	tpc.getTracerProvider("myKey1")
	tpc.getTracerProvider("myKey1")

	tpc.getTracerProvider("myKey2")

	if len(tpc.tracerProviders) != 2 {
		t.Errorf("There should be 2 tracer providers in the cache")
	}

	if tpc.tracerProvidersCounter["myKey1"] != 2 {
		t.Errorf("Tracer providers 'myKey1' should be registered twice")
	}

	if tpc.tracerProvidersCounter["myKey2"] != 1 {
		t.Errorf("Tracer providers 'myKey2' should be registered once")
	}
}

func Test_tracersProviderCache_cleanupTracerProvider(t *testing.T) {
	tpc := tracerProviderCache{
		tracerProviders:        make(map[string]*sdktrace.TracerProvider),
		tracerProvidersCounter: make(map[string]int),
	}

	tpc.getTracerProvider("myKey1", sdktrace.WithBatcher(&tracetest.NoopExporter{}))
	tpc.getTracerProvider("myKey1", sdktrace.WithBatcher(&tracetest.NoopExporter{}))
	tpc.getTracerProvider("myKey2")

	// clean up "myKey", which is registered twice
	err := tpc.cleanupTracerProvider("myKey1", nil)
	if err != nil {
		t.Errorf("There should be no error, err: %v", err)
	}

	if tpc.tracerProvidersCounter["myKey1"] != 1 {
		t.Errorf("Tracer providers 'myKey1' should be registered once now")
	}

	if _, ok := tpc.tracerProviders["myKey1"]; !ok {
		t.Errorf("Tracer providers 'myKey1' should be present")
	}

	if tpc.tracerProvidersCounter["myKey2"] != 1 {
		t.Errorf("Tracer providers 'myKey2' should be registered only once")
	}

	if _, ok := tpc.tracerProviders["myKey2"]; !ok {
		t.Errorf("Tracer providers 'myKey2' should be present")
	}

	// clean up "myKey" completely
	err = tpc.cleanupTracerProvider("myKey1", caddy.Log())
	if err != nil {
		t.Errorf("There should be no error, err: %v", err)
	}

	if tpc.tracerProvidersCounter["myKey1"] != 0 {
		t.Errorf("Tracer providers 'myKey1' should be registered once now")
	}

	if _, ok := tpc.tracerProviders["myKey1"]; ok {
		t.Errorf("Tracer providers 'myKey1' should be present")
	}
}
