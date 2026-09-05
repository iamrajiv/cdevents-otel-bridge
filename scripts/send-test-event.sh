#!/usr/bin/env bash
# Sends a single service.deployed CDEvent to the bridge and reads it back.
# Usage: BRIDGE_URL=http://localhost:8080 SERVICE=my-app ./scripts/send-test-event.sh
set -euo pipefail

BRIDGE_URL="${BRIDGE_URL:-http://localhost:8080}"
SERVICE="${SERVICE:-my-app}"
ENVIRONMENT="${ENVIRONMENT:-production}"
VERSION="${VERSION:-v1.0.0}"
COMMIT="${COMMIT:-abc123def456}"
EVENT_ID="evt-$(date +%s)"
TIMESTAMP="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

pretty() {
  if command -v jq >/dev/null 2>&1; then jq .; else python3 -m json.tool 2>/dev/null || cat; fi
}

echo "==> POST ${BRIDGE_URL}/api/v1/events (${EVENT_ID})"
curl -s -X POST "${BRIDGE_URL}/api/v1/events" \
  -H "Content-Type: application/json" \
  -d @- <<EOF | pretty
{
  "context": {
    "version": "0.4.1",
    "id": "${EVENT_ID}",
    "source": "https://ci.example.com/${SERVICE}/runs/123",
    "type": "dev.cdevents.service.deployed.0.1.1",
    "timestamp": "${TIMESTAMP}"
  },
  "subject": {
    "id": "${SERVICE}",
    "type": "service",
    "content": {
      "environment": {"id": "${ENVIRONMENT}"},
      "artifactId": "${SERVICE}:${VERSION}"
    }
  },
  "customData": {
    "commitSha": "${COMMIT}",
    "pipelineId": "build-123",
    "repository": "github.com/example/${SERVICE}",
    "deployer": "send-test-event.sh"
  }
}
EOF

echo
echo "==> GET ${BRIDGE_URL}/api/v1/deployments/${SERVICE}?environment=${ENVIRONMENT}"
curl -s "${BRIDGE_URL}/api/v1/deployments/${SERVICE}?environment=${ENVIRONMENT}" | pretty
