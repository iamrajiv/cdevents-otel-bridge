# CI/CD integration

The bridge learns about deployments only from the events you send it. This
guide shows what to send and where the ready-made pipeline snippets live.

## The event to send after every deployment

```json
{
  "context": {
    "version": "0.4.1",
    "id": "evt-<unique>",
    "source": "https://ci.example.com/job/my-app/123",
    "type": "dev.cdevents.service.deployed.0.1.1",
    "timestamp": "2026-01-15T10:30:00Z",
    "chainId": "<optional: same id for every event of one delivery>",
    "links": [{"linkType": "triggeredBy", "linkId": "<pipeline run event id>"}]
  },
  "subject": {
    "id": "my-app",
    "type": "service",
    "content": {
      "environment": {"id": "production"},
      "artifactId": "my-app:v1.2.3"
    }
  },
  "customData": {
    "commitSha": "abc123def456",
    "branch": "main",
    "author": "developer@example.com",
    "pipelineId": "123",
    "pipelineUrl": "https://ci.example.com/job/my-app/123",
    "repository": "github.com/org/my-app",
    "deployer": "jenkins"
  }
}
```

Rules of thumb:

- `subject.id` is the service name your application reports as
  `service.name` in OpenTelemetry. The two must match exactly. If your subject
  id is something else, add `subject.content.service.name`.
- `context.id` must be unique per event; a UUID or `<tool>-<run number>` works.
- `context.timestamp` is RFC 3339 in UTC.
- Put the commit in `customData.commitSha`. The version comes from
  `subject.content.service.version` or is parsed from `artifactId`
  (`name:tag`, `pkg:docker/name@tag`).
- Add `links` to the pipeline run or change event ids you emitted earlier and
  the chain API will connect them. If you do not track event ids, set the same
  `chainId` on every event of a delivery instead.

Send it with `curl`:

```bash
curl -fsS -X POST "$BRIDGE_URL/api/v1/events" \
  -H 'Content-Type: application/json' \
  -d @event.json
```

A `201` response means the event was stored. A `400` names the missing field.

## Examples in this repository

| Tool | File | What it does |
|------|------|--------------|
| Jenkins | [examples/ci-integrations/jenkins/Jenkinsfile](../examples/ci-integrations/jenkins/Jenkinsfile) | Builds, tests, pushes, deploys with `kubectl`, then posts `service.deployed` with the Jenkins build number, commit and URL |
| GitHub Actions | [examples/ci-integrations/github-actions/deploy.yml](../examples/ci-integrations/github-actions/deploy.yml) | Same flow using `github.sha`, `github.run_number` and the workflow URL; the bridge URL comes from a secret |
| Tekton | [examples/ci-integrations/tekton/pipeline.yaml](../examples/ci-integrations/tekton/pipeline.yaml) | Pipeline with a final `send-cdevent` task using `curlimages/curl` |

Inside a cluster the bridge is reachable at
`http://cdevents-otel-bridge.cdevents-otel-bridge.svc.cluster.local:8080`
when deployed with the manifests in `deployments/kubernetes`.

## Other event types worth sending

| Event | When | Why |
|-------|------|-----|
| `dev.cdevents.change.merged.*` | A pull request merges | Provides the commit, author and message the chain API reports as `rootCause` |
| `dev.cdevents.pipelinerun.finished.*` | A pipeline completes | Connects the change to the deployment; `subject.content.outcome` is shown in summaries |
| `dev.cdevents.service.rolledback.*` | A rollback happens | Treated like a deployment: the rolled-back version becomes the current one |
| `dev.cdevents.incident.detected.*` | Alerting opens an incident | Link it with `causedBy` to the deployment (or share its `chainId`) to make `GET /api/v1/chain/{incident}` walk back to the commit |

The mock deployer (`examples/mock-deployer/events.json`) is a complete,
linked example of all four.

## Argo CD and other GitOps tools

Use a post-sync hook or a notification template that posts the event above.
`subject.id` should be the application's service name, `customData.commitSha`
the synced revision, and `customData.deployer` `argocd`.

## Troubleshooting

- **`400 validation_error`**: the message says which of `context.id`,
  `context.type`, `context.source`, `context.timestamp`, `subject.id` or
  `subject.type` is missing.
- **Traces are not enriched although the event was accepted**: compare
  `GET /api/v1/deployments/<service.name>` with the `service.name` resource
  attribute in your application; they must be identical.
- **Chain is missing events**: links only connect to event ids the bridge
  received. Send the upstream events too, or set a shared `chainId`.
- **Network**: from a pipeline agent, `curl -fsS $BRIDGE_URL/api/v1/health`
  should return `{"status":"healthy", ...}`.
