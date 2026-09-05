# Build, test, lint and demo entry points for cdevents-otel-bridge.
# Run `make help` for a description of every target. Targets that touch
# Docker use the Compose file under deployments/docker-compose.
.PHONY: help build run fmt fmt-check vet lint test test-race test-coverage test-integration check \
        docker-build docker-run demo demo-events demo-logs demo-clean kustomize clean deps

BINARY_NAME    := bridge
DOCKER_IMAGE   := cdevents-otel-bridge
VERSION        := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS        := -X main.Version=$(VERSION)
COMPOSE_FILE   := deployments/docker-compose/docker-compose.yml
COMPOSE        := docker compose -f $(COMPOSE_FILE)
GOFILES        := $(shell find . -name '*.go' -not -path './vendor/*')

help:
	@echo "Available targets:"
	@echo "  build            Build the bridge binary into bin/"
	@echo "  run              Run the bridge locally with configs/config.dev.yaml"
	@echo "  fmt              Format all Go files with gofmt"
	@echo "  fmt-check        Fail if any Go file is not gofmt-formatted"
	@echo "  vet              Run go vet"
	@echo "  lint             Run golangci-lint"
	@echo "  test             Run unit tests"
	@echo "  test-race        Run unit tests with the race detector"
	@echo "  test-coverage    Run unit tests and write coverage.html"
	@echo "  test-integration Run integration tests (build tag: integration)"
	@echo "  check            fmt-check + vet + lint + test-race + test-integration"
	@echo "  docker-build     Build the bridge Docker image"
	@echo "  docker-run       Run the bridge Docker image on port 8080"
	@echo "  demo             Start the full demo stack (bridge, redis, jaeger, sample-app)"
	@echo "  demo-events      Send the demo event chain via the mock deployer"
	@echo "  demo-logs        Tail demo stack logs"
	@echo "  demo-clean       Stop the demo stack and remove volumes"
	@echo "  kustomize        Render the Kubernetes manifests"
	@echo "  clean            Remove build and coverage artifacts"
	@echo "  deps             Download and tidy Go module dependencies"

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) ./cmd/bridge

run:
	go run -ldflags "$(LDFLAGS)" ./cmd/bridge -config configs/config.dev.yaml

fmt:
	gofmt -s -w $(GOFILES)

fmt-check:
	@unformatted="$$(gofmt -s -l $(GOFILES))"; \
	if [ -n "$$unformatted" ]; then \
		echo "Files not gofmt-formatted:"; echo "$$unformatted"; exit 1; \
	fi

vet:
	go vet -tags=integration ./...

lint:
	golangci-lint run ./...

test:
	go test ./...

test-race:
	go test -race ./...

test-coverage:
	go test -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html
	go tool cover -func=coverage.out | tail -1

test-integration:
	go test -race -tags=integration ./test/integration/...

check: fmt-check vet lint test-race test-integration

docker-build:
	docker build -f deployments/docker/Dockerfile --build-arg VERSION=$(VERSION) \
		-t $(DOCKER_IMAGE):$(VERSION) -t $(DOCKER_IMAGE):latest .

docker-run:
	docker run --rm -p 8080:8080 $(DOCKER_IMAGE):latest

demo:
	$(COMPOSE) up -d --build --wait
	@echo ""
	@echo "Demo stack is up:"
	@echo "  Bridge API : http://localhost:8080/api/v1/health"
	@echo "  Sample app : http://localhost:8081/hello"
	@echo "  Jaeger UI  : http://localhost:16686"
	@echo ""
	@echo "Run 'make demo-events' to send the demo event chain."

demo-events:
	$(COMPOSE) --profile demo run --rm mock-deployer

demo-logs:
	$(COMPOSE) logs -f

demo-clean:
	$(COMPOSE) --profile demo down -v --remove-orphans

kustomize:
	kubectl kustomize deployments/kubernetes

clean:
	rm -rf bin/
	rm -f coverage.out coverage.html

deps:
	go mod download
	go mod tidy
