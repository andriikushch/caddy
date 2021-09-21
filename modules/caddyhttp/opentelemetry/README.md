# OpenTelemetry module

This module provides integration with OpenTelemetry tracing facilities. It is implemented
as `caddyhttp.MiddlewareHandler` and can be chained into a list of other handlers.

When enabled, it propagates an existing tracing context or will init a new one otherwise.

It is based on `https://github.com/open-telemetry/opentelemetry-go`.

## Configuration

It can be configured using environment variables specified
by https://github.com/open-telemetry/opentelemetry-specification.

For a particular handler, a configuration can be specified or overwritten by values from a configuration file. Here is
an example **Caddyfile**:

```
handle /myHanlder {
    opentelemetry {
            tracer_name my-tracer
            span_name my-span
            exporter_traces_protocol grpc
            service_name my-service
            propagators tracecontext
            exporter_certificate my-files
            exporter_insecure false
            exporter_traces_endpoint localhost
    }       
    reverse_proxy 127.0.0.1:8081
}
```

- **tracer_name** - tracer
  name [guideline](https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/trace/api.md#get-a-tracer)
  .
- **span_name** - span
  naming [guideline](https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/trace/api.md#span)
  .
- **service_name** - overwrites values specified by environment variables *OTEL_SERVICE_NAME*,
  *OTEL_RESOURCE_ATTRIBUTES*. See
  specification [here](https://github.com/open-telemetry/opentelemetry-specification/tree/main/specification/resource).
- **propagators** - this module supports only W3C TraceContext(*tracecontext*) and W3C Baggage(*baggage*). It overwrites
  environment variable *OTEL_PROPAGATORS*.

For the exporter configuration meaning please
see [spec](https://github.com/open-telemetry/opentelemetry-specification/blob/a4440931b522c7351b0485ff4899f786b4ff4459/specification/protocol/exporter.md)
.

- **exporter_traces_protocol** - this module supports only *grpc*. This value overwrites environment variables
  *OTEL_EXPORTER_OTLP_PROTOCOL* and *OTEL_EXPORTER_OTLP_TRACES_PROTOCOL*.
- **exporter_certificate** - overwrites *OTEL_EXPORTER_OTLP_CERTIFICATE*, *OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE*.
- **exporter_insecure** - overwrites *OTEL_EXPORTER_OTLP_INSECURE*, *OTEL_EXPORTER_OTLP_SPAN_INSECURE*.
- **exporter_traces_endpoint** - it overwrites *OTEL_EXPORTER_OTLP_ENDPOINT*, *OTEL_EXPORTER_OTLP_TRACES_ENDPOINT*.