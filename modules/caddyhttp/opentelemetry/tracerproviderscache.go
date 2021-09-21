package opentelemetry

import (
	"context"
	"fmt"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/zap"
	"sync"
)

// defaultTracerProviderCache is a global cache for a tracer providers
var defaultTracerProviderCache = &tracerProviderCache{
	tracerProviders:        make(map[string]*sdktrace.TracerProvider),
	tracerProvidersCounter: make(map[string]int),
}

type tracerProviderCache struct {
	mu                     sync.Mutex
	tracerProviders        map[string]*sdktrace.TracerProvider
	tracerProvidersCounter map[string]int
}

// getTracerProvider create or return existing TracerProvider in/from the cache
func (t *tracerProviderCache) getTracerProvider(key string, opts ...sdktrace.TracerProviderOption) *sdktrace.TracerProvider {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.tracerProvidersCounter[key]++

	if val, ok := t.tracerProviders[key]; ok {
		return val
	}

	t.tracerProviders[key] = sdktrace.NewTracerProvider(
		opts...,
	)

	return t.tracerProviders[key]
}

// cleanupTracerProvider gracefully shutdown a TracerProvider
func (t *tracerProviderCache) cleanupTracerProvider(key string, logger *zap.Logger) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.tracerProvidersCounter[key] > 0 {
		t.tracerProvidersCounter[key]--
	}

	if t.tracerProvidersCounter[key] == 0 {
		if tracerProvider, ok := t.tracerProviders[key]; ok {
			// tracerProviderCache.ForceFlush SHOULD be invoked according to https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/trace/sdk.md#forceflush
			if err := tracerProvider.ForceFlush(context.Background()); err != nil {
				logger.Error("forceFlush error: " + err.Error())
			}

			// tracerProviderCache.Shutdown MUST be invoked according to https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/trace/sdk.md#shutdown
			if err := tracerProvider.Shutdown(context.Background()); err != nil {
				return fmt.Errorf("tracerProviderCache shutdown error: %w", err)
			}
		}

		delete(t.tracerProviders, key)
		delete(t.tracerProvidersCounter, key)
	}

	return nil
}
