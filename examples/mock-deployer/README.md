# Mock deployer

Plays the role of a CI/CD system by replaying a chain of CDEvents into the
bridge. The bundled `events.json` contains:

1. `change.merged` for pull request `pr-42` (commit `abc123def456`)
2. `pipelinerun.finished` for `build-789`, linked to the change
3. `service.deployed` for `sample-app` v1.0.0 to `production`, linked to the pipeline run
4. `incident.detected` for `incident-101`, linked to the deployment

Timestamps are shifted so the sequence ends at the current time while keeping
the original spacing. The tool waits for the bridge to report healthy before
sending anything.

## Running

```bash
# against a bridge on localhost:8080
go run ./examples/mock-deployer

# inside the Docker Compose demo
make demo-events
```

Afterwards:

```bash
curl -s localhost:8080/api/v1/deployments/sample-app | jq
curl -s localhost:8080/api/v1/chain/evt-incident-001 | jq
```

## Configuration

| Variable | Default | Purpose |
|----------|---------|---------|
| `BRIDGE_URL` | `http://localhost:8080` | Bridge base URL |
| `EVENTS_FILE` | `events.json` | Events to replay (JSON array of CDEvents) |
| `EVENT_DELAY` | `1s` | Pause between events |
| `WAIT_TIMEOUT` | `60s` | How long to wait for the bridge to become healthy |

Point `EVENTS_FILE` at your own file to replay a different scenario.
