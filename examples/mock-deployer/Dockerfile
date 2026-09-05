# Image for the mock deployer. Build from the repository root; events.json
# is copied next to the binary and replayed at startup:
#   docker build -f examples/mock-deployer/Dockerfile .
FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /mock-deployer ./examples/mock-deployer

FROM alpine:3.21
RUN apk add --no-cache ca-certificates && addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=builder /mock-deployer /app/mock-deployer
COPY examples/mock-deployer/events.json /app/events.json
USER app
ENTRYPOINT ["/app/mock-deployer"]
