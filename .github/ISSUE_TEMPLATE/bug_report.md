---
name: Bug Report
about: Report a bug to help us improve
title: '[BUG] '
labels: ['bug', 'needs-triage']
assignees: ''
---

## Bug Description

A clear and concise description of what the bug is.

## Steps to Reproduce

Steps to reproduce the behavior:

1. Go to '...'
2. Click on '...'
3. Send request '...'
4. See error

## Expected Behavior

A clear and concise description of what you expected to happen.

## Actual Behavior

What actually happened.

## Environment

**Bridge Version:**
- Version: [e.g., 0.1.0]
- Commit: [e.g., abc123]

**Deployment:**
- Platform: [e.g., Docker Compose, Kubernetes, Binary]
- OS: [e.g., Ubuntu 22.04, macOS 14]
- Architecture: [e.g., amd64, arm64]

**Configuration:**
```yaml
# Paste relevant configuration (redact sensitive info)
```

**Dependencies:**
- Redis version: [e.g., 7.0.0]
- OpenTelemetry Collector version: [e.g., 0.91.0]
- Go version (if building from source): [e.g., 1.22.0]

## Logs

<details>
<summary>Bridge Logs</summary>

```
Paste bridge logs here (redact sensitive information)
```

</details>

<details>
<summary>Related Service Logs (Redis, OTel Collector, etc.)</summary>

```
Paste related logs here
```

</details>

## Event Payload (if applicable)

<details>
<summary>CDEvent that triggered the issue</summary>

```json
{
  "context": {
    ...
  },
  "subject": {
    ...
  }
}
```

</details>

## API Response (if applicable)

```json
{
  "error": "...",
  "details": "..."
}
```

## Screenshots

If applicable, add screenshots to help explain your problem.

## Additional Context

Add any other context about the problem here.

## Possible Solution

If you have ideas on how to fix this, please describe them here.

## Checklist

- [ ] I have searched existing issues to ensure this is not a duplicate
- [ ] I have included all relevant configuration (with secrets redacted)
- [ ] I have included logs and error messages
- [ ] I have included steps to reproduce
- [ ] I have tested with the latest version
