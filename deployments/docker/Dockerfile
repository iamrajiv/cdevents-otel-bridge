# Multi-stage image for the bridge. The builder compiles a static binary
# with the version baked in through the VERSION build argument; the runtime
# stage is a minimal Alpine image running as an unprivileged user with the
# default configuration copied in and a health check on the API.
FROM golang:1.25-alpine AS builder
ARG VERSION=dev
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.Version=${VERSION}" -o /bridge ./cmd/bridge

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata && addgroup -S bridge && adduser -S -G bridge bridge
WORKDIR /app
COPY --from=builder /bridge /app/bridge
COPY configs/config.yaml /app/configs/config.yaml
USER bridge
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s CMD wget -qO- http://127.0.0.1:8080/api/v1/health || exit 1
ENTRYPOINT ["/app/bridge"]
