# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- HTTP ingestion of CDEvents (`POST /api/v1/events`) with validation of the
  required context and subject fields and a 1 MiB body limit.
- Deployment records per service and environment, derived from
  `service.deployed`, `service.upgraded` and `service.rolledback` events with
  lenient field lookup across `subject.content` and `customData`.
- Query API: current deployment (`GET /api/v1/deployments/{service}`, latest
  across environments when `environment` is omitted), deployment list with
  `environment`, `since` and `limit` filters, and stored events by id.
- Chain reconstruction (`GET /api/v1/chain/{eventId}`) over CDEvents links and
  `chainId`, with cycle and size limits and a root cause taken from
  `change.merged` or the deployment's commit.
- Acceptance of three link shapes: `{linkType, linkId}`, `{type, target}` and
  CDEvents-native `{linkType, linkKind, from.contextId}`, from `context.links`
  or a top-level `links` array.
- Storage backends: in-memory and Redis (JSON values, per-key TTL, chain
  index), sharing one conformance test suite.
- `pkg/otelbridge`: a public OpenTelemetry span processor, HTTP client and
  caching resolver that add `deployment.*` attributes to application spans.
- Optional tracing of the bridge's own requests over OTLP/gRPC, enriched by
  the same processor through a storage-backed resolver.
- Prometheus metrics at `/metrics`: events received by type, rejected by
  reason, and processing duration.
- Health endpoint with a storage check that returns 503 on failure and reports
  the build version.
- Graceful shutdown on SIGINT/SIGTERM; HTTP server timeouts.
- Configuration from defaults, YAML and `BRIDGE_*` environment variables, with
  validation and a `-version` flag.
- Docker image (non-root, health check), Docker Compose demo stack with
  Redis, Jaeger, a sample app and a mock deployer, and Kubernetes manifests
  rendered with kustomize.
- Examples: an instrumented sample application, a mock deployer that replays
  a change → pipeline → deployment → incident chain, and Jenkins, GitHub
  Actions and Tekton pipeline snippets.
- CI workflow (format, vet, golangci-lint v2, unit and integration tests with
  the race detector, image builds, manifest rendering), release workflow
  publishing to GHCR, and CodeQL analysis.

[Unreleased]: https://github.com/iamrajiv/cdevents-otel-bridge/commits/main
