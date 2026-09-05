# Contributing

Thanks for considering a contribution. This document explains how to set up
the project, what the checks expect and how changes get merged.

## Prerequisites

- Go 1.25 or newer
- Docker with Compose v2 (for the demo and image builds)
- `make`
- [golangci-lint](https://golangci-lint.run) v2, built with a Go version at
  least as new as your toolchain (a linter built with an older Go cannot
  type-check a newer standard library)
- `kubectl` (only for rendering the Kubernetes manifests)

## Setup

```bash
git clone https://github.com/iamrajiv/cdevents-otel-bridge.git
cd cdevents-otel-bridge
make deps
make check
```

`make check` runs everything CI runs: gofmt, `go vet`, golangci-lint, unit
tests with the race detector and the integration tests.

## Everyday commands

| Command | Purpose |
|---------|---------|
| `make build` | Build `bin/bridge` with the version baked in |
| `make run` | Run the bridge with `configs/config.dev.yaml` |
| `make test` / `make test-race` | Unit tests |
| `make test-integration` | HTTP-level tests in `test/integration` (build tag `integration`) |
| `make test-coverage` | Coverage report in `coverage.html` |
| `make fmt` / `make fmt-check` | Format / verify formatting |
| `make lint` | golangci-lint |
| `make demo` / `make demo-events` / `make demo-clean` | Docker Compose demo stack |
| `make docker-build` | Build the bridge image |
| `make kustomize` | Render the Kubernetes manifests |

## Project layout

```
cmd/bridge/           entry point and wiring
internal/api/         HTTP server, handlers, middleware
internal/cdevents/    parsing, validation, link normalization, model conversion
internal/storage/     Storage interface, memory and Redis backends
internal/correlator/  chain reconstruction
internal/metrics/     Prometheus metrics
internal/otel/        the bridge's own tracing
internal/config/      configuration loading
pkg/models/           shared data models (public)
pkg/otelbridge/       span processor and client for applications (public)
examples/             sample app, mock deployer, CI snippets
deployments/          Docker, Compose, Kubernetes
docs/                 documentation
test/                 integration tests and JSON fixtures
```

Anything under `pkg/` is imported by other projects; treat its API as public
and avoid breaking changes.

## Code conventions

- Standard Go style, `gofmt -s`, imports grouped as stdlib, third-party,
  then `github.com/iamrajiv/cdevents-otel-bridge/...`.
- Every file starts with one multi-line comment block describing what the
  file is for and the decisions behind it: `/* ... */` in Go and Jenkinsfiles,
  a block of `#` lines in YAML, Dockerfiles, shell scripts, the Makefile and
  ignore files. That block is the only comment in the file. No inline or
  trailing comments anywhere in code or configuration; if something needs
  explaining, restructure it or say it in the header. Build directives such
  as `//go:build` and shebang lines are not comments and stay where the
  toolchain requires them.
- Wrap errors with context (`fmt.Errorf("...: %w", err)`) and use sentinel
  errors (`storage.ErrNotFound`, `otelbridge.ErrNotFound`) for conditions
  callers branch on.
- Every function that does I/O takes a `context.Context` first.
- Structured logging with `log/slog`; handlers log with the event id and type
  attached.
- US spelling in code and comments (enforced by `misspell`).

## Tests

- Unit tests live next to the code and use the standard library only.
- Storage backends share one conformance suite (`internal/storage/storage_test.go`);
  Redis runs against miniredis, so no external service is needed.
- API tests drive the real router through `httptest`.
- `test/fixtures/*.json` are parsed by the tests; keep them valid.
- Integration tests need `-tags=integration`.

Add or update tests with every change; CI runs them with `-race`.

## Commits and pull requests

Use [Conventional Commits](https://www.conventionalcommits.org):
`feat(api): add since filter to deployment list`, `fix(correlator): skip
dangling links`, `docs: ...`, `test: ...`, `chore: ...`.

Before opening a pull request:

1. `make check` passes locally.
2. Documentation under `docs/` and the README reflect any user-visible change.
3. `CHANGELOG.md` has an entry under *Unreleased*.
4. The PR description explains the motivation and how you tested it.

Releases are tagged `vX.Y.Z`; the release workflow builds and pushes
`ghcr.io/iamrajiv/cdevents-otel-bridge` with that tag.

## Reporting issues

Use the issue templates. For bugs, include the bridge version
(`bridge -version` or `/api/v1/health`), the storage backend and, if
possible, the event that triggered the problem with secrets removed.

## License

By contributing you agree that your contributions are licensed under the
Apache License 2.0.
