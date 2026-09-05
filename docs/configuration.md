# Configuration

Settings are resolved in three layers. Each layer overrides the one before it.

1. Built-in defaults.
2. A YAML file. Keys that are absent keep their default.
3. `BRIDGE_*` environment variables.

The result is validated on startup; the bridge refuses to start with an
invalid configuration and prints the reason.

## Command-line flags

| Flag | Description |
|------|-------------|
| `-config PATH` | Configuration file. When omitted, `BRIDGE_CONFIG` is used if set, then `configs/config.yaml` if it exists, otherwise built-in defaults. An explicitly given file that does not exist is an error. |
| `-version` | Print the build version and exit. |

## Configuration file

```yaml
server:
  host: "0.0.0.0"
  port: 8080

storage:
  type: "memory"            # memory | redis
  redisURL: ""              # redis://[:password@]host:port[/db]
  redisTTL: 86400           # seconds; 0 keeps records forever

otel:
  enabled: false
  endpoint: "localhost:4317"
  insecure: true
  serviceName: "cdevents-otel-bridge"

log:
  level: "info"             # debug | info | warn | error
  format: "json"            # json | text
```

## Reference

| YAML key | Environment variable | Default | Notes |
|----------|----------------------|---------|-------|
| `server.host` | `BRIDGE_HOST` | `0.0.0.0` | Bind address |
| `server.port` | `BRIDGE_PORT` | `8080` | 1–65535 |
| `storage.type` | `BRIDGE_STORAGE_TYPE` | `memory` | `memory` keeps everything in process and loses it on restart; `redis` persists and can be shared by several replicas |
| `storage.redisURL` | `BRIDGE_REDIS_URL` | | Required when `type` is `redis`. Supports `redis://`, `rediss://` (TLS), passwords and database numbers |
| `storage.redisTTL` | `BRIDGE_REDIS_TTL` | `86400` | Seconds every deployment, event and chain key lives in Redis. `0` disables expiry |
| `otel.enabled` | `BRIDGE_OTEL_ENABLED` | `false` | Export the bridge's own HTTP request spans over OTLP/gRPC and enrich them with the bridge's own deployment record |
| `otel.endpoint` | `BRIDGE_OTEL_ENDPOINT` | `localhost:4317` | `host:port`, or a full URL such as `https://collector:4317`. A URL decides TLS by its scheme |
| `otel.insecure` | `BRIDGE_OTEL_INSECURE` | `true` | For `host:port` endpoints: connect without TLS |
| `otel.serviceName` | `BRIDGE_OTEL_SERVICE_NAME` | `cdevents-otel-bridge` | `service.name` on the bridge's spans |
| `log.level` | `BRIDGE_LOG_LEVEL` | `info` | |
| `log.format` | `BRIDGE_LOG_FORMAT` | `json` | `text` is easier to read locally |

Non-numeric `BRIDGE_PORT` or `BRIDGE_REDIS_TTL`, and non-boolean
`BRIDGE_OTEL_ENABLED` or `BRIDGE_OTEL_INSECURE`, are startup errors.

## Fixed behavior

These are not configurable:

- HTTP server timeouts: 10 s read header, 30 s read, 30 s write, 120 s idle.
- Request body limit: 1 MiB.
- Graceful shutdown timeout on SIGINT/SIGTERM: 15 s.
- Health check storage timeout: 2 s.
- Chain traversal limits: 32 link hops, 256 events.

## Example files

| File | Purpose |
|------|---------|
| `configs/config.yaml` | Defaults; copied into the Docker image |
| `configs/config.dev.yaml` | Local development (`make run`): text logs at debug level |
| `configs/config.prod.yaml` | Template for production: Redis with a 7 day TTL and tracing to an OTel Collector |

## Examples

Development, no file:

```bash
BRIDGE_LOG_FORMAT=text BRIDGE_LOG_LEVEL=debug ./bin/bridge
```

Production with Redis and tracing, secrets from the environment:

```bash
BRIDGE_STORAGE_TYPE=redis \
BRIDGE_REDIS_URL=rediss://:s3cret@redis.internal:6380/0 \
BRIDGE_REDIS_TTL=604800 \
BRIDGE_OTEL_ENABLED=true \
BRIDGE_OTEL_ENDPOINT=https://otel-collector.observability:4317 \
./bin/bridge -config /etc/cdevents-otel-bridge/config.yaml
```

## Configuring the span processor (client side)

The `pkg/otelbridge` processor that runs inside your services is configured in
code, not through the bridge:

| Option | Default | Purpose |
|--------|---------|---------|
| `otelbridge.WithEnvironment(env)` | none (latest across environments) | Environment to resolve deployments for |
| `otelbridge.WithRequestTimeout(d)` | `2s` | HTTP client timeout for bridge calls |
| `otelbridge.WithCacheTTL(d)` | `30s` | How long a resolved deployment is reused |
| `otelbridge.WithNegativeCacheTTL(d)` | `10s` | How long "no deployment" is remembered |
| `otelbridge.WithLookupTimeout(d)` | `500ms` | Upper bound on how long span creation may wait for a lookup |
| `otelbridge.WithServiceName(name)` | `service.name` resource attribute | Override the service looked up |
| `otelbridge.WithLogger(l)` | discard | `slog.Logger` for lookup failures (debug level) |
