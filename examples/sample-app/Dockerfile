# Image for the sample application. Build from the repository root so the
# root go.mod and pkg/otelbridge are available:
#   docker build -f examples/sample-app/Dockerfile .
FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /sample-app ./examples/sample-app

FROM alpine:3.21
RUN apk add --no-cache ca-certificates && addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=builder /sample-app /app/sample-app
USER app
EXPOSE 8081
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s CMD wget -qO- http://127.0.0.1:8081/healthz || exit 1
ENTRYPOINT ["/app/sample-app"]
