# cdevents-otel-bridge

> Connect CDEvents from your CI/CD pipelines to OpenTelemetry traces, so every trace knows which deployment, commit and pipeline produced it.

[![CI](https://github.com/iamrajiv/cdevents-otel-bridge/actions/workflows/ci.yml/badge.svg)](https://github.com/iamrajiv/cdevents-otel-bridge/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/iamrajiv/cdevents-otel-bridge)](https://goreportcard.com/report/github.com/iamrajiv/cdevents-otel-bridge)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)](go.mod)

## The problem

It is 3 AM, latency is up, and the trace in Jaeger tells you *what* is slow but
not *which deployment* changed it. Deployment facts live in Jenkins, GitHub
Actions, Tekton or Argo CD; runtime facts live in your observability backend.
Connecting the two is manual work, done under pressure, every time.

## What the bridge does

1. **Receives CDEvents** over HTTP from any CI/CD tool that speaks the
   [CDEvents](https://cdevents.dev) standard (`POST /api/v1/events`).
2. **Remembers the current deployment** of every service and environment
   (in memory for development, Redis for production).
3. **Enriches traces.** A small OpenTelemetry span processor (`pkg/otelbridge`)
   asks the bridge for a service's deployment and stamps
   `deployment.version`, `deployment.commit`, `deployment.environment`,
   `deployment.pipeline_id` and friends onto every span that service emits.
4. **Reconstructs event chains** from CDEvents links and chain ids, so an
   incident can be walked back to the pipeline run and the merged commit
   (`GET /api/v1/chain/{eventId}`).

```
 CI/CD tools                          Bridge                              Your services
 ─────────────                        ──────                              ─────────────
 Jenkins / GitHub Actions /  POST     ┌─────────────────────────┐  GET     ┌──────────────────────┐
 Tekton / Argo CD / ...     ───────►  │ /api/v1/events          │ ◄─────── │ app + OTel SDK       │
   service.deployed                   │   parse, validate, store│          │ + otelbridge         │
   pipelinerun.finished               │                         │ ───────► │   span processor     │
   change.merged                      │ /api/v1/deployments/{s} │  latest  └──────────┬───────────┘
   incident.detected                  │ /api/v1/chain/{eventId} │  deploy             │ spans with
                                      │ /metrics                │                     │ deployment.* attrs
                                      └────────────┬────────────┘                     ▼
                                                   │ memory | Redis            Jaeger / Tempo / any OTLP
```

## Quick start

Prerequisites: Docker with Compose v2, `make`, and (optionally) `jq`.

```bash
make demo          # builds and starts redis, jaeger, the bridge and a sample app
make demo-events   # replays change.merged → pipelinerun.finished → service.deployed → incident.detected
```

Then look at the result:

```bash
# what is running as sample-app right now
curl -s localhost:8080/api/v1/deployments/sample-app | jq

# walk the incident back to the commit
curl -s localhost:8080/api/v1/chain/evt-incident-001 | jq

# make some traffic, then open Jaeger and pick service "sample-app"
curl -s localhost:8081/hello
open http://localhost:16686
```

Every `sample-app` span in Jaeger carries `deployment.version=v1.0.0`,
`deployment.commit=abc123def456`, `deployment.environment=production` and the
pipeline and repository that produced it. `make demo-clean` stops everything.

`./scripts/demo.sh` does all of the above in one go, including a few sample
requests, and prints the chain at the end.

If a port is already taken on your machine (usually by a container from
another project; `docker ps` shows which), override it, for example
`SAMPLE_APP_HOST_PORT=18081 JAEGER_UI_HOST_PORT=26686 make demo`. See
[deployments/docker-compose/.env.example](deployments/docker-compose/.env.example)
for the full list.

## Add trace enrichment to your own service

```go
import (
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/iamrajiv/cdevents-otel-bridge/pkg/otelbridge"
)

client := otelbridge.NewClient("http://cdevents-otel-bridge:8080",
	otelbridge.WithEnvironment("production"))
resolver := otelbridge.NewCachedResolver(client)

tp := sdktrace.NewTracerProvider(
	sdktrace.WithBatcher(exporter),
	sdktrace.WithResource(res),                                  // must carry service.name
	sdktrace.WithSpanProcessor(otelbridge.NewSpanProcessor(resolver)),
)
```

The processor reads `service.name` from the tracer provider's resource, asks
the bridge for that service's current deployment and adds these attributes:

| Attribute | Source |
|-----------|--------|
| `deployment.id` | `subject.id` of the deployment event |
| `deployment.version` | `subject.content.service.version` or parsed from `artifactId` |
| `deployment.commit` | `commitSha` |
| `deployment.environment` | `subject.content.environment.id` |
| `deployment.pipeline_id` | `pipelineId` |
| `deployment.repository` | `repository` |
| `deployment.branch` | `branch` |
| `deployment.deployed_at` | event timestamp (RFC 3339) |
| `deployment.deployed_by` | `deployer` |
| `deployment.event_id` | `context.id` of the deployment event |

Lookups are cached (30 s, 10 s for "not found") and bounded by a 500 ms
timeout, so an unreachable bridge never fails or noticeably slows a request.
See [examples/sample-app](examples/sample-app) for a complete service.

## API

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/events` | Ingest a CDEvent (`201 Created`) |
| `GET` | `/api/v1/events/{eventId}` | Fetch a stored event |
| `GET` | `/api/v1/deployments` | List deployments (`environment`, `since`, `limit`) |
| `GET` | `/api/v1/deployments/{service}` | Current deployment of a service (`environment` optional; latest across environments when omitted) |
| `GET` | `/api/v1/chain/{eventId}` | Event chain and root cause for an event |
| `GET` | `/api/v1/health` | Health with storage check (`503` when storage is down) |
| `GET` | `/metrics` | Prometheus metrics |

Full request and response shapes are in [docs/api.md](docs/api.md).

## Configuration

Settings come from built-in defaults, an optional YAML file (`-config`), and
`BRIDGE_*` environment variables, in that order of precedence.

| Variable | Default | Purpose |
|----------|---------|---------|
| `BRIDGE_PORT` | `8080` | Listen port |
| `BRIDGE_STORAGE_TYPE` | `memory` | `memory` or `redis` |
| `BRIDGE_REDIS_URL` | | `redis://[:password@]host:port[/db]` |
| `BRIDGE_REDIS_TTL` | `86400` | Seconds to keep records in Redis (`0` = forever) |
| `BRIDGE_OTEL_ENABLED` | `false` | Export the bridge's own request traces |
| `BRIDGE_OTEL_ENDPOINT` | `localhost:4317` | OTLP gRPC endpoint |
| `BRIDGE_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `BRIDGE_LOG_FORMAT` | `json` | `json` or `text` |

Everything else is in [docs/configuration.md](docs/configuration.md).

## Deploying

- **Binary**: `make build` produces `bin/bridge`; run it with `-config configs/config.yaml`.
- **Docker**: `make docker-build`, or `docker build -f deployments/docker/Dockerfile .`
  Images are published to `ghcr.io/iamrajiv/cdevents-otel-bridge` on tagged releases.
- **Docker Compose**: `deployments/docker-compose/docker-compose.yml` runs the full demo stack.
- **Kubernetes**: `kubectl apply -k deployments/kubernetes` (namespace, ConfigMap,
  Deployment with probes and a read-only root filesystem, Service).

## Sending events from CI/CD

Ready-made examples for Jenkins, GitHub Actions and Tekton live in
[examples/ci-integrations](examples/ci-integrations), and
[docs/ci-integration.md](docs/ci-integration.md) explains the event shape the
bridge expects. The smallest useful event is:

```bash
curl -X POST http://localhost:8080/api/v1/events -H 'Content-Type: application/json' -d '{
  "context": {"version": "0.4.1", "id": "evt-1", "source": "https://ci.example.com/run/1",
              "type": "dev.cdevents.service.deployed.0.1.1", "timestamp": "2026-01-15T10:30:00Z"},
  "subject": {"id": "my-app", "type": "service",
              "content": {"environment": {"id": "production"}, "artifactId": "my-app:v1.2.3"}},
  "customData": {"commitSha": "abc123", "repository": "github.com/org/my-app", "pipelineId": "build-42"}
}'
```

## Development

```bash
make check             # gofmt, go vet, golangci-lint, unit tests (-race), integration tests
make test              # unit tests only
make test-integration  # HTTP-level tests in test/integration
make run               # run the bridge with configs/config.dev.yaml
```

Go 1.25 or newer is required. `make lint` needs
[golangci-lint](https://golangci-lint.run) v2 built with a Go version at least
as new as your toolchain.

Every source and configuration file starts with a single multi-line comment
block that explains what the file is for; there are no other comments in the
code. [CONTRIBUTING.md](CONTRIBUTING.md) has the full list of conventions.

## Project layout

```
cmd/bridge/           entry point, wiring, graceful shutdown
internal/api/         chi router, handlers, middleware
internal/cdevents/    CDEvent parsing, validation, link normalization, model conversion
internal/storage/     Storage interface, memory and Redis backends
internal/correlator/  chain reconstruction over stored events and links
internal/metrics/     Prometheus metrics
internal/otel/        the bridge's own tracer provider and storage-backed resolver
internal/config/      YAML + environment configuration
pkg/models/           Deployment, Link, EventChain, Incident, PipelineRun
pkg/otelbridge/       public span processor, HTTP client and cache for applications
examples/             sample app, mock deployer, CI pipeline snippets
deployments/          Dockerfile, Docker Compose stack, Kubernetes manifests
docs/                 architecture, API, configuration, getting started, CI integration
```

## Documentation

- [Getting started](docs/getting-started.md)
- [Architecture](docs/architecture.md)
- [API reference](docs/api.md)
- [Configuration](docs/configuration.md)
- [CI/CD integration](docs/ci-integration.md)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). This project follows the
[Contributor Covenant](CODE_OF_CONDUCT.md).

## License

Apache License 2.0. See [LICENSE](LICENSE).

## Acknowledgments

- [CDEvents](https://cdevents.dev/), a CD Foundation project
- [OpenTelemetry](https://opentelemetry.io/)
- [chi](https://github.com/go-chi/chi) and [go-redis](https://github.com/redis/go-redis)
