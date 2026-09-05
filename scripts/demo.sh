#!/usr/bin/env bash
# Starts the full demo stack (Redis, Jaeger, bridge, sample app), replays the
# demo event chain and prints the queries that show the result.
#
# Host ports can be changed with the *_HOST_PORT variables documented in
# deployments/docker-compose/.env.example, for example:
#   SAMPLE_APP_HOST_PORT=18081 JAEGER_UI_HOST_PORT=26686 ./scripts/demo.sh
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE=(docker compose -f "${ROOT_DIR}/deployments/docker-compose/docker-compose.yml")
BRIDGE_URL="${BRIDGE_URL:-http://localhost:${BRIDGE_HOST_PORT:-8080}}"
SAMPLE_URL="${SAMPLE_URL:-http://localhost:${SAMPLE_APP_HOST_PORT:-8081}}"
JAEGER_URL="${JAEGER_URL:-http://localhost:${JAEGER_UI_HOST_PORT:-16686}}"

pretty() {
  if command -v jq >/dev/null 2>&1; then jq .; else python3 -m json.tool 2>/dev/null || cat; fi
}

echo "==> Starting demo stack"
"${COMPOSE[@]}" up -d --build --wait

echo
echo "==> Replaying demo events"
"${COMPOSE[@]}" --profile demo run --rm mock-deployer

echo
echo "==> Generating a few traces from the sample app"
for _ in 1 2 3; do
  curl -s "${SAMPLE_URL}/hello" | pretty
  curl -s "${SAMPLE_URL}/api/data" >/dev/null
done

echo
echo "==> Current deployment of sample-app"
curl -s "${BRIDGE_URL}/api/v1/deployments/sample-app" | pretty

echo
echo "==> Chain from the incident back to the commit"
curl -s "${BRIDGE_URL}/api/v1/chain/evt-incident-001" | pretty

cat <<EOF

Demo is running.
  Bridge API : ${BRIDGE_URL}/api/v1/health
  Metrics    : ${BRIDGE_URL}/metrics
  Sample app : ${SAMPLE_URL}/hello
  Jaeger UI  : ${JAEGER_URL}  (service "sample-app", look for deployment.* tags)

Stop with: make demo-clean
EOF
