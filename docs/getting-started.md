# Getting started

This guide takes you from nothing to a trace in Jaeger that carries the
deployment, commit and pipeline that produced it.

## 1. Run the demo stack

Prerequisites: Docker with Compose v2, `make`. `jq` is handy but optional.

```bash
git clone https://github.com/iamrajiv/cdevents-otel-bridge.git
cd cdevents-otel-bridge
make demo
```

`make demo` builds and starts four containers and waits until they are healthy:

| Service | Host port | Role |
|---------|-----------|------|
| `bridge` | 8080 | The bridge, using Redis and exporting its own traces to Jaeger |
| `redis` | 6379 | Storage |
| `jaeger` | 16686 (UI), 4317/4318 (OTLP) | Trace backend |
| `sample-app` | 8081 | A Go service instrumented with OpenTelemetry and the `otelbridge` span processor |

If a port is already in use, override it:

```bash
SAMPLE_APP_HOST_PORT=18081 JAEGER_UI_HOST_PORT=26686 OTLP_GRPC_HOST_PORT=24317 make demo
```

All overridable ports are listed in `deployments/docker-compose/.env.example`;
copy it to `.env` in the same directory to make the change permanent.

## 2. Replay a delivery chain

```bash
make demo-events
```

This runs the mock deployer, which sends four linked CDEvents to the bridge:

1. `change.merged`: pull request `pr-42`, commit `abc123def456`
2. `pipelinerun.finished`: `build-789`, triggered by the change
3. `service.deployed`: `sample-app` v1.0.0 to `production`, triggered by the pipeline run
4. `incident.detected`: `incident-101`, caused by the deployment

Expected output ends with:

```
all 4 events accepted
current deployment:  curl -s http://bridge:8080/api/v1/deployments/sample-app | jq
incident to commit:  curl -s http://bridge:8080/api/v1/chain/evt-incident-001 | jq
```

## 3. Query the bridge

```bash
curl -s localhost:8080/api/v1/deployments/sample-app | jq .deployment
```

```json
{
  "id": "sample-app",
  "service": "sample-app",
  "version": "v1.0.0",
  "environment": "production",
  "commitSha": "abc123def456",
  "repository": "github.com/demo/sample-app",
  "branch": "main",
  "pipelineId": "build-789",
  "deployedBy": "github-actions",
  "eventId": "evt-deployed-001",
  "chainId": "chain-demo-001"
}
```

```bash
curl -s localhost:8080/api/v1/chain/evt-incident-001 | jq '{events: [.events[].summary], rootCause}'
```

```json
{
  "events": [
    "Incident detected: High latency on /api/data",
    "Deployed sample-app v1.0.0 to production",
    "Pipeline build-and-deploy finished (success)",
    "Merged change pr-42 by developer@example.com"
  ],
  "rootCause": {
    "commit": "abc123def456",
    "author": "developer@example.com",
    "message": "Add new checkout feature",
    "repository": "github.com/demo/sample-app"
  }
}
```

## 4. See enriched traces

Generate a few requests against the sample app, then open Jaeger:

```bash
curl -s localhost:8081/hello | jq
curl -s localhost:8081/api/data > /dev/null
open http://localhost:16686
```

The `/hello` response includes a `jaegerUI` link straight to its trace. In
Jaeger, pick service `sample-app` and open any trace: each span has
`deployment.version=v1.0.0`, `deployment.commit=abc123def456`,
`deployment.environment=production`, `deployment.pipeline_id=build-789` and
`deployment.repository=github.com/demo/sample-app`. The bridge itself appears
as service `cdevents-otel-bridge` with one span per API request.

Stop the stack with `make demo-clean`.

## 5. Run the bridge on its own

```bash
make build
./bin/bridge -config configs/config.dev.yaml
```

Or without a file, using environment variables:

```bash
BRIDGE_LOG_FORMAT=text BRIDGE_PORT=9090 ./bin/bridge
```

Send an event and read it back with the helper script:

```bash
BRIDGE_URL=http://localhost:9090 SERVICE=my-app ./scripts/send-test-event.sh
```

With `BRIDGE_STORAGE_TYPE=redis` and `BRIDGE_REDIS_URL=redis://localhost:6379`
data survives restarts; see [configuration.md](configuration.md).

## 6. Add enrichment to your own service

Add the module and register the span processor when you build your tracer
provider:

```go
import "github.com/iamrajiv/cdevents-otel-bridge/pkg/otelbridge"

client := otelbridge.NewClient("http://cdevents-otel-bridge:8080",
	otelbridge.WithEnvironment("production"))
resolver := otelbridge.NewCachedResolver(client)

tp := sdktrace.NewTracerProvider(
	sdktrace.WithBatcher(exporter),
	sdktrace.WithResource(res),
	sdktrace.WithSpanProcessor(otelbridge.NewSpanProcessor(resolver)),
)
```

The resource must carry `service.name`; that is the service the processor
asks the bridge about. [examples/sample-app/main.go](../examples/sample-app/main.go)
is a complete, runnable reference.

## 7. Send events from your pipelines

Every deployment step should post a `service.deployed` event with the service
name, environment, version and commit. See
[ci-integration.md](ci-integration.md) and the Jenkins, GitHub Actions and
Tekton examples under `examples/ci-integrations`.

## 8. Deploy to Kubernetes

```bash
kubectl apply -k deployments/kubernetes
kubectl -n cdevents-otel-bridge port-forward svc/cdevents-otel-bridge 8080:80
curl localhost:8080/api/v1/health
```

The manifests create a namespace, a ConfigMap with the bridge configuration
(in-memory storage by default), a Deployment with liveness and readiness
probes and a read-only root filesystem, and a ClusterIP Service on port 80.

To use Redis, set `storage.type: redis` in the ConfigMap and create the
optional secret the Deployment already references:

```bash
kubectl -n cdevents-otel-bridge create secret generic cdevents-otel-bridge-secrets \
  --from-literal=redis-url=redis://redis.data.svc.cluster.local:6379
```

## Troubleshooting

**`make demo` fails with "port is already allocated"**: another process owns
one of the host ports, most often a container from another project.
`docker ps --format '{{.Names}}\t{{.Ports}}'` shows which one; stop it, or
override the `*_HOST_PORT` variables as shown above. On macOS with Colima or
Docker Desktop the listener shows up in `lsof` as the VM's port forwarder,
not as the container.

**No `deployment.*` tags on spans**: check that the bridge has a deployment for
the exact `service.name` your app uses (`GET /api/v1/deployments/{service}`),
that the app can reach `BRIDGE_URL`, and that the environment passed to
`WithEnvironment` matches the event (or omit it to take the latest). Enable the
processor's logger with `otelbridge.WithLogger` to see lookup failures.

**Events rejected with `validation_error`**: the message names the missing
field. `context.timestamp` must be RFC 3339.

**Health returns 503**: the storage check failed; the response's
`checks.storage` contains the Redis error.

**No traces from the bridge itself**: set `BRIDGE_OTEL_ENABLED=true` and point
`BRIDGE_OTEL_ENDPOINT` at an OTLP/gRPC receiver. Health and metrics requests
are intentionally not traced.
