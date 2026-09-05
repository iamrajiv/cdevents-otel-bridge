# Sample application

A minimal Go HTTP service instrumented with OpenTelemetry that uses the
`pkg/otelbridge` span processor. Every span it exports carries
`deployment.version`, `deployment.commit`, `deployment.environment` and the
other `deployment.*` attributes resolved from the bridge, which is how a real
service links its traces back to the deployment that produced them.

## Endpoints

| Path | Description |
|------|-------------|
| `GET /hello` | Greeting plus the trace id and a Jaeger link for the request |
| `GET /api/data` | Simulated work with child spans |
| `GET /deployment` | What the bridge currently knows about this service |
| `GET /healthz` | Liveness probe (not traced) |

## Configuration

| Variable | Default | Purpose |
|----------|---------|---------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `localhost:4317` | OTLP gRPC endpoint. `host:port` is dialed without TLS; use a full `https://` URL for TLS |
| `BRIDGE_URL` | `http://localhost:8080` | Bridge base URL |
| `SERVICE_NAME` | `sample-app` | `service.name` resource attribute and the service resolved in the bridge |
| `ENVIRONMENT` | `production` | Environment to resolve deployments for |
| `PORT` | `8081` | Listen port |

## Running locally

```bash
go run ./examples/sample-app
curl -s localhost:8081/hello
```

The Docker Compose demo in `deployments/docker-compose` runs this app next
to the bridge, Redis and Jaeger; see the repository README.

## Using the processor in your own service

```go
client := otelbridge.NewClient(bridgeURL, otelbridge.WithEnvironment("production"))
resolver := otelbridge.NewCachedResolver(client)

tp := sdktrace.NewTracerProvider(
	sdktrace.WithBatcher(exporter),
	sdktrace.WithResource(res),
	sdktrace.WithSpanProcessor(otelbridge.NewSpanProcessor(resolver)),
)
```

The processor reads `service.name` from the tracer provider's resource and
asks the bridge for that service's current deployment. Results are cached
(30 s by default, 10 s for "not found") and each lookup is bounded by a
500 ms timeout, so a slow or unreachable bridge never fails a request.
