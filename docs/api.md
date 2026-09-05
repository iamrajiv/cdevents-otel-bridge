# API reference

All endpoints return JSON. There is no authentication; put the bridge behind
your ingress or service mesh if it is reachable from untrusted networks.

Base URL in the examples: `http://localhost:8080`.

## Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/api/v1/events` | Ingest a CDEvent |
| `GET` | `/api/v1/events/{eventId}` | Fetch a stored event |
| `GET` | `/api/v1/deployments` | List deployments |
| `GET` | `/api/v1/deployments/{service}` | Current deployment of a service |
| `GET` | `/api/v1/chain/{eventId}` | Event chain and root cause |
| `GET` | `/api/v1/health` | Health check |
| `GET` | `/metrics` | Prometheus metrics |

`OPTIONS` requests receive a permissive CORS preflight response so browser
dashboards can call the read endpoints.

## POST /api/v1/events

Accepts one CDEvent per request. The body is limited to 1 MiB.

### What the bridge reads

Required (validated): `context.id`, `context.type`, `context.source`,
`context.timestamp` (RFC 3339), `subject.id`, `subject.type`.

Optional: `context.version`, `context.chainId`, `context.links`,
`subject.content`, `customData`.

For **deployment events** (`service.deployed`, `service.upgraded`,
`service.rolledback`) a deployment record is stored, replacing the previous
record for the same service and environment. Fields are resolved from several
locations because CI tools disagree about where they belong:

| Deployment field | Looked up in (first non-empty wins) |
|------------------|-------------------------------------|
| `service` | `subject.content.service.name`, `customData.service`, `subject.id` |
| `version` | `subject.content.service.version`, `customData.version`, parsed from `subject.content.artifactId` (`name:tag`, `pkg:type/name@version`, `name-v1.2.3`) |
| `environment` | `subject.content.environment.id`, `subject.content.environment`, `customData.environment` |
| `commitSha` | `commitSha`, `commit`, `sha`, `gitCommit` in `customData` then `subject.content` |
| `repository` | `repository`, `repo`, `repositoryUrl` (string, or object with `id`/`url`/`name`) |
| `branch`, `author`, `pipelineId`, `pipelineUrl`, `deployedBy` | `customData` then `subject.content` (`deployer` or `deployedBy`) |

Every event, of any type, is also stored for chain reconstruction. Links are
accepted from `context.links` or a top-level `links` array in any of these
shapes:

```json
{"linkType": "triggeredBy", "linkId": "evt-1"}
{"type": "TRIGGERED_BY", "target": "evt-1"}
{"linkType": "RELATION", "linkKind": "TRIGGER", "from": {"contextId": "evt-1"}}
```

### Request

```bash
curl -X POST http://localhost:8080/api/v1/events \
  -H 'Content-Type: application/json' \
  -d '{
    "context": {
      "version": "0.4.1",
      "id": "evt-deployed-001",
      "source": "https://github.com/demo/sample-app/actions",
      "type": "dev.cdevents.service.deployed.0.1.1",
      "timestamp": "2026-01-15T10:30:00Z",
      "chainId": "chain-demo-001",
      "links": [{"linkType": "triggeredBy", "linkId": "evt-pipeline-run-001"}]
    },
    "subject": {
      "id": "sample-app",
      "type": "service",
      "content": {
        "environment": {"id": "production"},
        "artifactId": "sample-app:v1.0.0"
      }
    },
    "customData": {
      "commitSha": "abc123def456",
      "branch": "main",
      "pipelineId": "build-789",
      "repository": "github.com/demo/sample-app",
      "author": "developer@example.com",
      "deployer": "github-actions"
    }
  }'
```

### Responses

`201 Created`

```json
{
  "status": "accepted",
  "eventId": "evt-deployed-001",
  "eventType": "dev.cdevents.service.deployed.0.1.1",
  "timestamp": "2026-01-15T10:30:00Z"
}
```

| Status | `error` | When |
|--------|---------|------|
| `400` | `invalid_event` | Body is not valid JSON (or a timestamp is malformed) |
| `400` | `validation_error` | A required field is missing; `message` names it |
| `400` | `invalid_request` | Body could not be read |
| `413` | `payload_too_large` | Body exceeds 1 MiB |
| `500` | `storage_error` | The storage backend failed |

## GET /api/v1/deployments/{service}

Returns the deployment currently recorded for a service.

| Query parameter | Description |
|-----------------|-------------|
| `environment` | Optional. When omitted, the most recent deployment across all environments is returned. |

```bash
curl http://localhost:8080/api/v1/deployments/sample-app?environment=production
```

`200 OK`

```json
{
  "service": "sample-app",
  "deployment": {
    "id": "sample-app",
    "service": "sample-app",
    "version": "v1.0.0",
    "environment": "production",
    "commitSha": "abc123def456",
    "repository": "github.com/demo/sample-app",
    "branch": "main",
    "author": "developer@example.com",
    "pipelineId": "build-789",
    "pipelineUrl": "https://github.com/demo/sample-app/actions/runs/789",
    "deployedAt": "2026-01-15T10:30:00Z",
    "deployedBy": "github-actions",
    "eventId": "evt-deployed-001",
    "chainId": "chain-demo-001",
    "links": [{"linkType": "triggeredBy", "linkId": "evt-pipeline-run-001"}]
  },
  "links": {
    "self": "/api/v1/deployments/sample-app?environment=production",
    "event": "/api/v1/events/evt-deployed-001",
    "chain": "/api/v1/chain/evt-deployed-001"
  }
}
```

`404 Not Found` with `error: not_found` when nothing is recorded for the
service (or for that environment).

## GET /api/v1/deployments

Lists deployment records, newest first.

| Query parameter | Default | Description |
|-----------------|---------|-------------|
| `environment` | | Only deployments in this environment |
| `since` | | Only deployments at or after this RFC 3339 timestamp |
| `limit` | `50` | Maximum results, capped at `100` |

`200 OK`

```json
{
  "deployments": [ { "...": "same shape as above" } ],
  "total": 1,
  "limit": 50
}
```

`400 invalid_request` for a non-positive `limit` or an unparsable `since`.

## GET /api/v1/events/{eventId}

Returns the stored form of any ingested event.

```json
{
  "event": {
    "id": "evt-deployed-001",
    "type": "dev.cdevents.service.deployed.0.1.1",
    "source": "https://github.com/demo/sample-app/actions",
    "timestamp": "2026-01-15T10:30:00Z",
    "subjectId": "sample-app",
    "subjectType": "service",
    "chainId": "chain-demo-001",
    "links": [{"linkType": "triggeredBy", "linkId": "evt-pipeline-run-001"}],
    "summary": "Deployed sample-app v1.0.0 to production",
    "rawData": {
      "content": {"environment": {"id": "production"}, "artifactId": "sample-app:v1.0.0"},
      "customData": {"commitSha": "abc123def456", "pipelineId": "build-789"}
    }
  }
}
```

`404 not_found` when the id is unknown.

## GET /api/v1/chain/{eventId}

Reconstructs the chain of events behind `eventId`:

1. every event reachable by following links (`triggeredBy`, `causedBy`, any type), up to 32 hops and 256 events;
2. every other event sharing the start event's `chainId`.

Events are returned newest first. `rootCause` is taken from the earliest
`change.merged` event that carries a commit, or, failing that, from the commit
recorded on the deployment event.

```bash
curl http://localhost:8080/api/v1/chain/evt-incident-001
```

`200 OK`

```json
{
  "chainId": "chain-demo-001",
  "startEvent": "evt-incident-001",
  "events": [
    {
      "id": "evt-incident-001",
      "type": "dev.cdevents.incident.detected.0.2.0",
      "timestamp": "2026-01-15T10:45:00Z",
      "summary": "Incident detected: High latency on /api/data",
      "links": [{"linkType": "causedBy", "linkId": "evt-deployed-001"}]
    },
    {
      "id": "evt-deployed-001",
      "type": "dev.cdevents.service.deployed.0.1.1",
      "timestamp": "2026-01-15T10:30:00Z",
      "summary": "Deployed sample-app v1.0.0 to production",
      "commit": "abc123def456",
      "links": [{"linkType": "triggeredBy", "linkId": "evt-pipeline-run-001"}]
    },
    {
      "id": "evt-pipeline-run-001",
      "type": "dev.cdevents.pipelinerun.finished.0.2.0",
      "timestamp": "2026-01-15T10:25:00Z",
      "summary": "Pipeline build-and-deploy finished (success)",
      "links": [{"linkType": "triggeredBy", "linkId": "evt-change-merged-001"}]
    },
    {
      "id": "evt-change-merged-001",
      "type": "dev.cdevents.change.merged.0.4.1",
      "timestamp": "2026-01-15T10:00:00Z",
      "summary": "Merged change pr-42 by developer@example.com",
      "commit": "abc123def456"
    }
  ],
  "rootCause": {
    "commit": "abc123def456",
    "author": "developer@example.com",
    "message": "Add new checkout feature",
    "repository": "github.com/demo/sample-app"
  }
}
```

`404 not_found` when the start event is unknown. Links that point at events the
bridge never received are skipped silently.

## GET /api/v1/health

```json
{
  "status": "healthy",
  "version": "v0.1.0",
  "uptime": "2h30m15s",
  "checks": {"storage": "ok"}
}
```

Returns `503` with `status: unhealthy` and the error text in `checks.storage`
when the storage backend does not answer within two seconds. Suitable for both
liveness and readiness probes.

## GET /metrics

Prometheus text exposition. Bridge-specific series:

```
cdevents_received_total{type="service.deployed"}        events accepted and stored, by short type
cdevents_rejected_total{reason="validation"}            rejected events (parse, validation, payload_too_large, unreadable_body, storage)
cdevents_processing_duration_seconds_bucket{le="0.01"}  histogram of parse+validate+store time
```

Standard Go runtime and process collectors are included.

## Error envelope

```json
{
  "error": "validation_error",
  "message": "CDEvent validation failed: invalid event: context.id is required"
}
```

Every request is logged with a request id, the matched route pattern, status
and duration.
