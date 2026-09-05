# Architecture

The bridge is a small Go service with one job: keep the current deployment of
every service, and hand that knowledge to whoever asks, including the
OpenTelemetry SDK inside your applications.

```
┌────────────────────────────────────────────────────────────────────────────────┐
│ CI/CD tools (Jenkins, GitHub Actions, Tekton, Argo CD, ...)                    │
│   POST /api/v1/events  {service.deployed | pipelinerun.finished | change.merged │
│                          | incident.detected | ...}                            │
└───────────────────────────────────────┬────────────────────────────────────────┘
                                        ▼
┌────────────────────────────────────────────────────────────────────────────────┐
│ cdevents-otel-bridge                                                           │
│                                                                                │
│  internal/api          chi router, middleware, handlers, otelhttp (optional)   │
│       │                                                                        │
│       ▼                                                                        │
│  internal/cdevents     Parse → NormalizeLinks → Validate → ToDeployment /      │
│                        ToStoredEvent / ToPipelineRun / ToIncident              │
│       │                                                                        │
│       ▼                                                                        │
│  internal/storage      Storage interface ── MemoryStorage | RedisStorage       │
│       ▲                    deployments (service, environment) and events (id,  │
│       │                    chainId)                                            │
│  internal/correlator   BFS over links + chainId union → EventChain + RootCause │
│  internal/metrics      Prometheus registry                                     │
│  internal/otel         bridge's own tracer provider + storage-backed resolver  │
└──────────┬────────────────────────────────────────────────┬────────────────────┘
           │ GET /api/v1/deployments/{service}               │ OTLP (bridge spans)
           ▼                                                 ▼
┌──────────────────────────────────────────┐      ┌─────────────────────────────┐
│ Your service                             │ OTLP │ Jaeger / Tempo / Collector  │
│   OpenTelemetry SDK                      │ ───► │   spans carry               │
│   + pkg/otelbridge SpanProcessor         │      │   deployment.* attributes   │
│     (Client → CachedResolver → OnStart)  │      └─────────────────────────────┘
└──────────────────────────────────────────┘
```

## Components

### `internal/api`

A [chi](https://github.com/go-chi/chi) router with request-id, real-IP,
recovery, logging and CORS middleware. Handlers are thin: they parse input,
call storage or the correlator, and write JSON envelopes. When tracing is
enabled the whole router is wrapped with `otelhttp`, excluding the health and
metrics paths. The server owns an `http.Server` with timeouts and supports
graceful shutdown.

### `internal/cdevents`

Everything that understands the CDEvents wire format.

- `Parse` decodes JSON and normalizes links (merging top-level and
  `context.links`, accepting three link shapes, dropping duplicates).
- `Validate` enforces the required fields separately, so malformed JSON and
  missing data get different error codes.
- `ToDeployment`, `ToPipelineRun`, `ToIncident` build domain models with
  ordered fallbacks across `subject.content` and `customData`, because
  producers differ in where they put commit, repository or environment.
- `ToStoredEvent` produces the backend-agnostic record kept for every event,
  including a human summary used by the chain API.
- `ShortType` reduces `dev.cdevents.service.deployed.0.1.1` to
  `service.deployed` so nothing else in the code compares versioned type
  strings.

### `internal/storage`

```go
type Storage interface {
	SaveDeployment(ctx, *models.Deployment) error
	GetDeployment(ctx, service, environment string) (*models.Deployment, error)
	GetLatestDeployment(ctx, service string) (*models.Deployment, error)
	ListDeployments(ctx, ListFilter) ([]*models.Deployment, error)
	SaveEvent(ctx, *cdevents.StoredEvent) error
	GetEvent(ctx, eventID string) (*cdevents.StoredEvent, error)
	GetEventsByChain(ctx, chainID string) ([]*cdevents.StoredEvent, error)
	Ping(ctx) error
	Close() error
}
```

Deployments are keyed by service and environment; saving replaces. Both
backends return lists newest first so API output does not depend on the
backend.

| Backend | Keys | Notes |
|---------|------|-------|
| Memory | Go maps behind a `sync.RWMutex` | Development, tests, single instance; nothing is evicted |
| Redis | `deployment:{service}:{environment}`, `event:{id}`, `chain:{chainId}` (set of ids) | JSON values, TTL on every key, listing by `SCAN`; shared by all replicas |

Redis storage is tested against [miniredis](https://github.com/alicebob/miniredis),
so the suite needs no external services.

### `internal/correlator`

`BuildChain(eventID)`:

1. Load the start event.
2. Breadth-first traversal over `links[].linkId` through storage, with a
   visited set (cycles), a depth limit (32) and a size limit (256). Links to
   events the bridge never saw are skipped.
3. Union with all events that share the start event's `chainId`, which
   rescues chains from producers that set `chainId` but no links.
4. Sort newest first and pick the root cause: the earliest `change.merged`
   event with a commit, else the deployment's own commit.

The traversal reads storage on every call rather than keeping an in-memory
graph, so it works after restarts and across replicas with Redis.

### `internal/metrics`

A private Prometheus registry (so tests can create several servers) exposing
`cdevents_received_total{type}`, `cdevents_rejected_total{reason}` and
`cdevents_processing_duration_seconds`, plus the Go and process collectors.

### `internal/otel`

The bridge's own tracing: an OTLP/gRPC exporter, a resource with the service
name and build version, W3C propagators, and `StorageResolver`, which adapts
`storage.Storage` to the public `otelbridge.DeploymentResolver` interface so
the bridge's spans are enriched with the bridge's own deployment record by
the same processor applications use.

### `pkg/otelbridge` (public)

The part that lives inside your applications.

| Type | Role |
|------|------|
| `Client` | HTTP client for `GET /api/v1/deployments/{service}`; implements `DeploymentResolver` |
| `CachedResolver` | Wraps any resolver with a TTL cache, negative caching and stale-on-error |
| `SpanProcessor` | `sdktrace.SpanProcessor`; on `OnStart` resolves the deployment for the span's `service.name` and sets `deployment.*` attributes |
| `Attributes` | The `Deployment` → attribute mapping, shared by the bridge and applications |

Span creation is on every request's hot path, so the processor never blocks
for longer than its lookup timeout (500 ms by default), detaches from the
span's context, and relies on the cache so a lookup happens at most once per
service per cache period.

## Data flows

**Ingest**: request → 1 MiB limit → `Parse` → `Validate` → (deployment event?
`SaveDeployment`) → `SaveEvent` → metrics → `201`.

**Enrich**: span starts in your app → processor reads `service.name` →
`CachedResolver` → (`Client` → `GET /api/v1/deployments/{service}` when the
cache is cold) → `span.SetAttributes(deployment.*)`.

**Chain**: `GET /api/v1/chain/{id}` → traversal + chainId union → newest-first
list with summaries and commits → root cause.

## Design decisions

- **The processor is client-side.** Traces are produced by the application,
  not by the bridge, so the enrichment must run where spans are created. That
  is why `otelbridge` is a public package with a cached HTTP client instead of
  code inside the bridge.
- **Lenient parsing, strict validation.** Only the six fields the bridge truly
  depends on are mandatory. Everything else is looked up in several places
  because real CI tools are inconsistent, and a bridge that rejects
  well-meaning events is useless.
- **Latest deployment replaces, events accumulate.** A service/environment
  pair has exactly one current deployment; history is preserved as events and
  reachable through chains.
- **Storage is the only state.** No in-process indexes or graphs; every
  replica answers the same questions from the same Redis.
- **Small surface.** No auth, rate limiting or multi-tenancy; those belong to
  the ingress in front of the bridge.

## Limits and non-goals

- One bridge per environment or per organization; there is no tenant concept.
- Memory storage is unbounded; use Redis with a TTL for long-running instances.
- Chains stop at 32 hops or 256 events.
- The bridge does not create spans for CDEvents themselves; it enriches the
  spans your applications already emit and traces its own HTTP requests.
