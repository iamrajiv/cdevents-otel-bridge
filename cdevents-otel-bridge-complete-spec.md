# CDEvents-OTel-Bridge: Complete Project Specification

> **Purpose**: This document contains everything needed to build the cdevents-otel-bridge project. Use it as the reference while building.

---

## Table of Contents

1. [Project Overview](#1-project-overview)
2. [Problem Statement](#2-problem-statement)
3. [Solution Architecture](#3-solution-architecture)
4. [CDEvents Background](#4-cdevents-background)
5. [Technology Stack](#5-technology-stack)
6. [Project Structure](#6-project-structure)
7. [API Specification](#7-api-specification)
8. [Data Models](#8-data-models)
9. [Implementation Phases](#9-implementation-phases)
10. [Coding Standards](#10-coding-standards)
11. [Testing Strategy](#11-testing-strategy)
12. [Deployment](#12-deployment)
13. [Documentation Requirements](#13-documentation-requirements)
14. [References](#14-references)

---

## 1. Project Overview

### What We're Building

**cdevents-otel-bridge** - An open-source Go bridge that connects CDEvents (CI/CD events standard from CD Foundation) with OpenTelemetry (observability standard). 

When a deployment happens, CI/CD tools emit CDEvents. This bridge:
1. Captures those events via webhook
2. Stores deployment metadata
3. Enriches OpenTelemetry traces with deployment context
4. Enables engineers to trace any incident back to its originating commit

### Key Value Proposition

| Feature | Proprietary (Datadog) | Our Bridge |
|---------|----------------------|------------|
| Cost | $$$$ | Free (Apache 2.0) |
| Vendor Lock-in | Yes | No |
| Works with any CI/CD | Limited | Yes (CDEvents standard) |
| Works with any observability tool | No | Yes (OTel standard) |
| Open Source | No | Yes |

### Target Users

- DevOps engineers wanting deployment traceability
- SREs investigating incidents
- Platform teams building internal developer platforms
- Organizations adopting CDEvents standard

---

## 2. Problem Statement

### The 3 AM Incident Scenario

```
🚨 PagerDuty Alert: "Service latency increased 500%"

Engineer's questions:
1. What deployment caused this?
2. Which commit introduced the problem?
3. Who made the change?
4. What pipeline ran?

Current reality:
- Check Slack for deployment announcements
- Search Jenkins for recent builds
- Dig through Git history
- Cross-reference timestamps manually
- Takes 30-60 minutes to find root cause
```

### Why Current Solutions Fall Short

| Solution | Problem |
|----------|---------|
| Datadog CI Visibility | Expensive, vendor lock-in |
| Honeycomb Markers | Manual, proprietary format |
| Keptn | Heavy platform, requires full adoption |
| Manual OTel attributes | Per-app changes, no standard |
| Custom scripts | Maintenance burden, not reusable |

### Our Solution

```
With cdevents-otel-bridge:

🚨 Alert fires
    ↓
Open Jaeger trace
    ↓
See: deployment.id = "deploy-789"
     deployment.commit = "abc123"
     deployment.version = "v1.2.3"
     deployment.pipeline = "build-456"
    ↓
Click through to exact commit
    ↓
Root cause found in 2 minutes!
```

---

## 3. Solution Architecture

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                            CI/CD TOOLS                                   │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐       │
│  │ Jenkins │  │ Tekton  │  │Spinnaker│  │ GitHub  │  │ Argo CD │       │
│  │         │  │         │  │         │  │ Actions │  │         │       │
│  └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘       │
│       │            │            │            │            │             │
│       └────────────┴────────────┴────────────┴────────────┘             │
│                                 │                                        │
│                          CDEvents (HTTP POST)                            │
│                                 │                                        │
└─────────────────────────────────┼────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                      CDEVENTS-OTEL-BRIDGE                                │
│                                                                          │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────────────────┐  │
│  │              │    │              │    │                          │  │
│  │  HTTP API    │───►│   Storage    │◄───│   OTel Span Processor    │  │
│  │              │    │              │    │                          │  │
│  │ POST /events │    │ service →    │    │ Enriches spans with:     │  │
│  │ GET /deploy  │    │ deployment   │    │ - deployment.id          │  │
│  │ GET /chain   │    │ metadata     │    │ - deployment.commit      │  │
│  │              │    │              │    │ - deployment.version     │  │
│  └──────────────┘    └──────────────┘    └──────────────────────────┘  │
│                                                     │                   │
└─────────────────────────────────────────────────────┼───────────────────┘
                                                      │
                                                      │ Enriched Traces
                                                      ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                        OBSERVABILITY BACKENDS                            │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐                    │
│  │ Jaeger  │  │ Grafana │  │ Zipkin  │  │  Any    │                    │
│  │         │  │ Tempo   │  │         │  │  OTel   │                    │
│  └─────────┘  └─────────┘  └─────────┘  └─────────┘                    │
└─────────────────────────────────────────────────────────────────────────┘
```

### Data Flow

```
Step 1: Deployment Happens
────────────────────────────────────────────────────────────
Jenkins Pipeline completes
    │
    ├── Sends CDEvent to bridge
    │   POST /events
    │   {
    │     "type": "dev.cdevents.service.deployed",
    │     "subject": { "id": "my-app", ... },
    │     "customData": { "commitSha": "abc123", ... }
    │   }
    │
    └── Bridge stores: my-app → {commit: abc123, version: v1.2.3, ...}


Step 2: User Makes Request
────────────────────────────────────────────────────────────
User request → my-app (instrumented with OTel)
    │
    ├── App creates span
    │
    ├── OTel processor queries bridge: GET /deployments/my-app
    │
    ├── Bridge returns: {commit: abc123, version: v1.2.3, ...}
    │
    └── Processor adds attributes to span:
        - deployment.id = "deploy-789"
        - deployment.commit = "abc123"
        - deployment.version = "v1.2.3"


Step 3: View in Jaeger
────────────────────────────────────────────────────────────
Engineer opens Jaeger
    │
    ├── Sees trace with deployment context
    │
    └── Clicks through: trace → deployment → pipeline → commit
```

### Event Correlation with Links

```
CDEvents v0.4 "links" feature creates event chains:

┌──────────────┐     links.causedBy     ┌──────────────┐
│   incident   │◄───────────────────────│  deployment  │
│   detected   │                        │   deployed   │
│  (id: i-101) │                        │  (id: d-789) │
└──────────────┘                        └──────┬───────┘
                                               │
                                        links.triggeredBy
                                               │
                                               ▼
                                        ┌──────────────┐
                                        │  pipelineRun │
                                        │   finished   │
                                        │  (id: p-456) │
                                        └──────┬───────┘
                                               │
                                        links.triggeredBy
                                               │
                                               ▼
                                        ┌──────────────┐
                                        │    change    │
                                        │    merged    │
                                        │  (id: c-123) │
                                        └──────────────┘

API: GET /chain/i-101

Response:
{
  "chain": [
    {"type": "incident.detected", "id": "i-101"},
    {"type": "service.deployed", "id": "d-789"},
    {"type": "pipelinerun.finished", "id": "p-456"},
    {"type": "change.merged", "id": "c-123", "commit": "abc123"}
  ]
}
```

---

## 4. CDEvents Background

### What is CDEvents?

CDEvents is a common specification for Continuous Delivery events, enabling interoperability across CI/CD tools. It's an incubated project at the CD Foundation (Linux Foundation).

### CDEvent Structure

```json
{
  "context": {
    "version": "0.4.1",
    "id": "unique-event-id",
    "source": "https://jenkins.example.com/job/my-app",
    "type": "dev.cdevents.service.deployed.0.4.1",
    "timestamp": "2024-01-15T10:30:00Z",
    "schemaUri": "https://cdevents.dev/0.4.1/schema/service-deployed-event",
    "chainId": "chain-abc-123",
    "links": [
      {
        "linkType": "triggeredBy",
        "linkId": "pipelinerun-456"
      }
    ]
  },
  "subject": {
    "id": "deployment-789",
    "source": "https://k8s.example.com/namespaces/prod/deployments/my-app",
    "type": "service",
    "content": {
      "environment": {
        "id": "production",
        "source": "https://spinnaker.example.com/applications/my-app"
      },
      "artifactId": "my-app:v1.2.3"
    }
  },
  "customData": {
    "commitSha": "abc123def456",
    "pipelineId": "build-456",
    "deployer": "jenkins",
    "repository": "github.com/company/my-app"
  },
  "customDataContentType": "application/json"
}
```

### CDEvent Types We Handle

| Event Type | When Emitted | What We Extract |
|------------|--------------|-----------------|
| `service.deployed` | Deployment completes | Service, version, commit, environment |
| `pipelinerun.queued` | Pipeline starts | Pipeline ID, trigger info |
| `pipelinerun.finished` | Pipeline completes | Status, duration, artifacts |
| `change.merged` | PR/MR merged | Commit SHA, author, branch |
| `incident.detected` | Incident created | Incident ID, severity, affected service |
| `incident.resolved` | Incident closed | Resolution time, root cause |

### Links Feature (v0.4)

The `links` field enables event correlation:

```json
{
  "context": {
    "links": [
      {
        "linkType": "triggeredBy",
        "linkId": "previous-event-id"
      },
      {
        "linkType": "causedBy", 
        "linkId": "related-event-id"
      },
      {
        "linkType": "relatedTo",
        "linkId": "another-event-id"
      }
    ]
  }
}
```

**Link Types:**
- `triggeredBy` - This event was triggered by another event
- `causedBy` - This event was caused by another event
- `relatedTo` - General relation to another event

---

## 5. Technology Stack

### Core Technologies

| Component | Technology | Version | Purpose |
|-----------|------------|---------|---------|
| Language | Go | 1.21+ | Main application |
| CDEvents SDK | github.com/cdevents/sdk-go | v0.4.x | Parse/create CDEvents |
| OpenTelemetry | go.opentelemetry.io/otel | v1.24+ | Trace enrichment |
| HTTP Router | chi | v5.0+ | REST API |
| Config | yaml + env | - | Configuration |
| Logging | slog | stdlib | Structured logging |
| Storage (dev) | sync.Map | stdlib | In-memory storage |
| Storage (prod) | Redis | v7+ | Persistent storage |
| Container | Docker | 24+ | Containerization |
| Orchestration | Docker Compose / K8s | - | Deployment |

### Go Dependencies

```go
// go.mod
module github.com/YOUR_USERNAME/cdevents-otel-bridge

go 1.21

require (
    // CDEvents
    github.com/cdevents/sdk-go v0.4.1
    
    // OpenTelemetry
    go.opentelemetry.io/otel v1.24.0
    go.opentelemetry.io/otel/sdk v1.24.0
    go.opentelemetry.io/otel/trace v1.24.0
    go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.24.0
    go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.24.0
    
    // HTTP
    github.com/go-chi/chi/v5 v5.0.12
    
    // Configuration
    gopkg.in/yaml.v3 v3.0.1
    
    // Storage (optional - for production)
    github.com/redis/go-redis/v9 v9.5.1
    
    // Testing
    github.com/stretchr/testify v1.9.0
)
```

---

## 6. Project Structure

```
cdevents-otel-bridge/
│
├── cmd/
│   └── bridge/
│       └── main.go                     # Application entry point
│
├── internal/
│   │
│   ├── config/
│   │   ├── config.go                   # Configuration struct and loading
│   │   └── config_test.go
│   │
│   ├── api/
│   │   ├── server.go                   # HTTP server setup
│   │   ├── handlers.go                 # Request handlers
│   │   ├── handlers_test.go
│   │   ├── middleware.go               # Logging, recovery, CORS
│   │   └── responses.go                # Standard response helpers
│   │
│   ├── cdevents/
│   │   ├── parser.go                   # Parse incoming CDEvents
│   │   ├── parser_test.go
│   │   ├── validator.go                # Validate event structure
│   │   ├── types.go                    # Internal event types
│   │   └── links.go                    # Handle links extraction
│   │
│   ├── storage/
│   │   ├── interface.go                # Storage interface definition
│   │   ├── memory.go                   # In-memory implementation
│   │   ├── memory_test.go
│   │   ├── redis.go                    # Redis implementation
│   │   └── redis_test.go
│   │
│   ├── otel/
│   │   ├── provider.go                 # OTel provider setup
│   │   ├── processor.go                # Custom span processor
│   │   ├── enricher.go                 # Add deployment attributes
│   │   └── enricher_test.go
│   │
│   └── correlator/
│       ├── correlator.go               # Event correlation logic
│       ├── correlator_test.go
│       ├── graph.go                    # Event graph building
│       └── graph_test.go
│
├── pkg/
│   └── models/
│       ├── deployment.go               # Deployment data model
│       ├── incident.go                 # Incident data model
│       ├── pipeline.go                 # Pipeline data model
│       ├── event_chain.go              # Linked event chain
│       └── models_test.go
│
├── examples/
│   │
│   ├── sample-app/
│   │   ├── main.go                     # Demo app with OTel
│   │   ├── Dockerfile
│   │   └── README.md
│   │
│   ├── mock-deployer/
│   │   ├── main.go                     # Simulates CI/CD sending events
│   │   ├── events.json                 # Sample events
│   │   └── README.md
│   │
│   └── ci-integrations/
│       ├── jenkins/
│       │   └── Jenkinsfile             # Jenkins pipeline example
│       ├── github-actions/
│       │   └── deploy.yml              # GitHub Actions example
│       └── tekton/
│           └── pipeline.yaml           # Tekton pipeline example
│
├── deployments/
│   │
│   ├── docker/
│   │   ├── Dockerfile                  # Main app Dockerfile
│   │   └── Dockerfile.dev              # Development Dockerfile
│   │
│   ├── docker-compose/
│   │   ├── docker-compose.yml          # Full demo stack
│   │   ├── docker-compose.dev.yml      # Development stack
│   │   └── .env.example                # Environment variables
│   │
│   └── kubernetes/
│       ├── namespace.yaml
│       ├── configmap.yaml
│       ├── deployment.yaml
│       ├── service.yaml
│       └── kustomization.yaml
│
├── configs/
│   ├── config.yaml                     # Default configuration
│   ├── config.dev.yaml                 # Development configuration
│   └── config.prod.yaml                # Production configuration
│
├── scripts/
│   ├── demo.sh                         # Run full demo
│   ├── send-test-event.sh              # Send test CDEvent
│   ├── setup-jaeger.sh                 # Setup Jaeger locally
│   └── generate-mocks.sh               # Generate test mocks
│
├── docs/
│   ├── architecture.md                 # Architecture deep-dive
│   ├── getting-started.md              # Quick start guide
│   ├── api.md                          # API documentation
│   ├── configuration.md                # Configuration reference
│   ├── ci-integration.md               # CI/CD integration guide
│   └── images/
│       ├── architecture.png
│       └── demo-screenshot.png
│
├── test/
│   ├── integration/
│   │   ├── api_test.go                 # API integration tests
│   │   └── storage_test.go             # Storage integration tests
│   │
│   ├── e2e/
│   │   ├── e2e_test.go                 # End-to-end tests
│   │   └── docker-compose.test.yml     # Test environment
│   │
│   └── fixtures/
│       ├── valid_service_deployed.json
│       ├── valid_pipelinerun_finished.json
│       └── invalid_event.json
│
├── .github/
│   ├── workflows/
│   │   ├── ci.yml                      # CI pipeline
│   │   ├── release.yml                 # Release pipeline
│   │   └── codeql.yml                  # Security scanning
│   │
│   ├── ISSUE_TEMPLATE/
│   │   ├── bug_report.md
│   │   └── feature_request.md
│   │
│   └── PULL_REQUEST_TEMPLATE.md
│
├── .gitignore
├── .golangci.yml                       # Linter configuration
├── Makefile                            # Build commands
├── go.mod
├── go.sum
├── README.md
├── LICENSE                             # Apache 2.0
├── CONTRIBUTING.md
├── CHANGELOG.md
└── CODE_OF_CONDUCT.md
```

---

## 7. API Specification

### Base URL

```
http://localhost:8080/api/v1
```

### Endpoints

#### POST /api/v1/events

Receive CDEvents from CI/CD tools.

**Request Headers:**
```
Content-Type: application/json
Ce-Type: dev.cdevents.service.deployed.0.4.1  (optional, CloudEvents header)
Ce-Source: https://jenkins.example.com         (optional, CloudEvents header)
```

**Request Body:**
```json
{
  "context": {
    "version": "0.4.1",
    "id": "evt-12345",
    "source": "https://jenkins.example.com/job/my-app/123",
    "type": "dev.cdevents.service.deployed.0.4.1",
    "timestamp": "2024-01-15T10:30:00Z",
    "chainId": "chain-abc-123",
    "links": [
      {
        "linkType": "triggeredBy",
        "linkId": "pipelinerun-456"
      }
    ]
  },
  "subject": {
    "id": "deploy-789",
    "source": "https://k8s.example.com/namespaces/prod/deployments/my-app",
    "type": "service",
    "content": {
      "environment": {
        "id": "production"
      },
      "artifactId": "my-app:v1.2.3"
    }
  },
  "customData": {
    "commitSha": "abc123def456",
    "pipelineId": "build-456",
    "deployer": "jenkins",
    "repository": "github.com/company/my-app",
    "author": "developer@example.com"
  }
}
```

**Response (201 Created):**
```json
{
  "status": "accepted",
  "eventId": "evt-12345",
  "eventType": "dev.cdevents.service.deployed.0.4.1",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

**Response (400 Bad Request):**
```json
{
  "error": "validation_failed",
  "message": "Invalid CDEvent: missing required field 'context.type'",
  "details": {
    "field": "context.type",
    "reason": "required"
  }
}
```

---

#### GET /api/v1/deployments/{service}

Get latest deployment info for a service.

**Path Parameters:**
- `service` (required): Service name

**Query Parameters:**
- `environment` (optional): Filter by environment (e.g., "production")

**Response (200 OK):**
```json
{
  "service": "my-app",
  "deployment": {
    "id": "deploy-789",
    "version": "v1.2.3",
    "commitSha": "abc123def456",
    "environment": "production",
    "deployedAt": "2024-01-15T10:30:00Z",
    "deployedBy": "jenkins",
    "pipelineId": "build-456",
    "repository": "github.com/company/my-app",
    "author": "developer@example.com"
  },
  "links": {
    "pipeline": "/api/v1/events/pipelinerun-456",
    "commit": "https://github.com/company/my-app/commit/abc123def456"
  }
}
```

**Response (404 Not Found):**
```json
{
  "error": "not_found",
  "message": "No deployment found for service 'my-app'"
}
```

---

#### GET /api/v1/deployments

List all tracked deployments.

**Query Parameters:**
- `environment` (optional): Filter by environment
- `since` (optional): Filter deployments after this timestamp (ISO 8601)
- `limit` (optional): Max results (default: 50, max: 100)

**Response (200 OK):**
```json
{
  "deployments": [
    {
      "service": "my-app",
      "version": "v1.2.3",
      "environment": "production",
      "deployedAt": "2024-01-15T10:30:00Z"
    },
    {
      "service": "auth-service",
      "version": "v2.0.1",
      "environment": "production",
      "deployedAt": "2024-01-15T09:15:00Z"
    }
  ],
  "total": 2,
  "limit": 50
}
```

---

#### GET /api/v1/events/{eventId}

Get a specific event by ID.

**Response (200 OK):**
```json
{
  "event": {
    "id": "evt-12345",
    "type": "dev.cdevents.service.deployed.0.4.1",
    "source": "https://jenkins.example.com/job/my-app/123",
    "timestamp": "2024-01-15T10:30:00Z",
    "subject": {
      "id": "deploy-789",
      "type": "service"
    },
    "links": [
      {
        "linkType": "triggeredBy",
        "linkId": "pipelinerun-456"
      }
    ]
  }
}
```

---

#### GET /api/v1/chain/{eventId}

Get full event chain using links (trace from incident to commit).

**Response (200 OK):**
```json
{
  "chainId": "chain-abc-123",
  "startEvent": "incident-101",
  "events": [
    {
      "id": "incident-101",
      "type": "dev.cdevents.incident.detected",
      "timestamp": "2024-01-15T11:00:00Z",
      "summary": "High latency detected"
    },
    {
      "id": "deploy-789",
      "type": "dev.cdevents.service.deployed",
      "timestamp": "2024-01-15T10:30:00Z",
      "summary": "Deployed my-app v1.2.3"
    },
    {
      "id": "pipelinerun-456",
      "type": "dev.cdevents.pipelinerun.finished",
      "timestamp": "2024-01-15T10:25:00Z",
      "summary": "Pipeline build-456 completed"
    },
    {
      "id": "change-123",
      "type": "dev.cdevents.change.merged",
      "timestamp": "2024-01-15T10:00:00Z",
      "summary": "Merged PR #42 by developer@example.com",
      "commit": "abc123def456"
    }
  ],
  "rootCause": {
    "commit": "abc123def456",
    "author": "developer@example.com",
    "message": "Add new feature X",
    "repository": "github.com/company/my-app"
  }
}
```

---

#### GET /api/v1/health

Health check endpoint.

**Response (200 OK):**
```json
{
  "status": "healthy",
  "version": "1.0.0",
  "uptime": "2h30m15s",
  "checks": {
    "storage": "ok",
    "otel": "ok"
  }
}
```

---

#### GET /api/v1/metrics

Prometheus metrics endpoint.

**Response (200 OK):**
```
# HELP cdevents_received_total Total number of CDEvents received
# TYPE cdevents_received_total counter
cdevents_received_total{type="service.deployed"} 150
cdevents_received_total{type="pipelinerun.finished"} 300

# HELP cdevents_processing_duration_seconds CDEvent processing duration
# TYPE cdevents_processing_duration_seconds histogram
cdevents_processing_duration_seconds_bucket{le="0.01"} 100
cdevents_processing_duration_seconds_bucket{le="0.05"} 145
cdevents_processing_duration_seconds_bucket{le="0.1"} 150

# HELP deployments_tracked_total Total deployments currently tracked
# TYPE deployments_tracked_total gauge
deployments_tracked_total 25
```

---

## 8. Data Models

### Deployment Model

```go
// pkg/models/deployment.go

package models

import "time"

// Deployment represents a service deployment
type Deployment struct {
    // Core identification
    ID          string    `json:"id"`
    Service     string    `json:"service"`
    Version     string    `json:"version"`
    Environment string    `json:"environment"`
    
    // Git information
    CommitSha   string    `json:"commitSha"`
    Repository  string    `json:"repository"`
    Branch      string    `json:"branch,omitempty"`
    Author      string    `json:"author,omitempty"`
    
    // Pipeline information
    PipelineID  string    `json:"pipelineId,omitempty"`
    PipelineURL string    `json:"pipelineUrl,omitempty"`
    
    // Deployment metadata
    DeployedAt  time.Time `json:"deployedAt"`
    DeployedBy  string    `json:"deployedBy"` // jenkins, spinnaker, argocd, etc.
    
    // CDEvent metadata
    EventID     string    `json:"eventId"`
    ChainID     string    `json:"chainId,omitempty"`
    
    // Links to related events
    Links       []Link    `json:"links,omitempty"`
}

// Link represents a CDEvent link
type Link struct {
    LinkType string `json:"linkType"` // triggeredBy, causedBy, relatedTo
    LinkID   string `json:"linkId"`
}
```

### Incident Model

```go
// pkg/models/incident.go

package models

import "time"

// Incident represents a detected incident
type Incident struct {
    ID           string     `json:"id"`
    Service      string     `json:"service"`
    Environment  string     `json:"environment"`
    Severity     string     `json:"severity"` // critical, high, medium, low
    Summary      string     `json:"summary"`
    
    // Timing
    DetectedAt   time.Time  `json:"detectedAt"`
    ResolvedAt   *time.Time `json:"resolvedAt,omitempty"`
    
    // Resolution
    Status       string     `json:"status"` // open, acknowledged, resolved
    RootCause    string     `json:"rootCause,omitempty"`
    
    // Correlation
    EventID      string     `json:"eventId"`
    ChainID      string     `json:"chainId,omitempty"`
    Links        []Link     `json:"links,omitempty"`
    
    // Linked deployment (if correlated)
    LinkedDeployment *Deployment `json:"linkedDeployment,omitempty"`
}

// Duration returns incident duration (detection to resolution)
func (i *Incident) Duration() *time.Duration {
    if i.ResolvedAt == nil {
        return nil
    }
    d := i.ResolvedAt.Sub(i.DetectedAt)
    return &d
}
```

### Event Chain Model

```go
// pkg/models/event_chain.go

package models

import "time"

// EventChain represents a linked chain of events
type EventChain struct {
    ChainID    string       `json:"chainId"`
    StartEvent string       `json:"startEvent"`
    Events     []ChainEvent `json:"events"`
    RootCause  *RootCause   `json:"rootCause,omitempty"`
}

// ChainEvent represents an event in the chain
type ChainEvent struct {
    ID        string    `json:"id"`
    Type      string    `json:"type"`
    Timestamp time.Time `json:"timestamp"`
    Summary   string    `json:"summary"`
    Links     []Link    `json:"links,omitempty"`
}

// RootCause represents the identified root cause
type RootCause struct {
    Commit     string `json:"commit"`
    Author     string `json:"author"`
    Message    string `json:"message"`
    Repository string `json:"repository"`
}
```

### Pipeline Model

```go
// pkg/models/pipeline.go

package models

import "time"

// PipelineRun represents a CI/CD pipeline execution
type PipelineRun struct {
    ID          string    `json:"id"`
    Name        string    `json:"name"`
    Status      string    `json:"status"` // queued, running, finished, failed
    Outcome     string    `json:"outcome,omitempty"` // success, failure, error
    
    // Timing
    QueuedAt    time.Time  `json:"queuedAt"`
    StartedAt   *time.Time `json:"startedAt,omitempty"`
    FinishedAt  *time.Time `json:"finishedAt,omitempty"`
    
    // Source
    Source      string    `json:"source"` // jenkins, tekton, github-actions
    URL         string    `json:"url"`
    
    // Trigger info
    TriggerType string    `json:"triggerType"` // commit, pr, schedule, manual
    CommitSha   string    `json:"commitSha,omitempty"`
    Branch      string    `json:"branch,omitempty"`
    
    // CDEvent metadata
    EventID     string    `json:"eventId"`
    ChainID     string    `json:"chainId,omitempty"`
    Links       []Link    `json:"links,omitempty"`
}

// Duration returns pipeline execution duration
func (p *PipelineRun) Duration() *time.Duration {
    if p.StartedAt == nil || p.FinishedAt == nil {
        return nil
    }
    d := p.FinishedAt.Sub(*p.StartedAt)
    return &d
}
```

---

## 9. Implementation Phases

### Phase 1: Core Bridge (MVP) - Week 1

**Goal:** Receive CDEvents via HTTP, store deployment metadata, query via API.

#### Tasks

- [ ] **1.1 Project Setup**
  - Initialize Go module
  - Create project structure
  - Add dependencies to go.mod
  - Create Makefile with build commands

- [ ] **1.2 Configuration**
  - Define Config struct
  - Load from YAML file
  - Override with environment variables
  - Validate configuration

- [ ] **1.3 Data Models**
  - Create Deployment model
  - Create Link model
  - Add JSON tags and validation

- [ ] **1.4 Storage Layer**
  - Define Storage interface
  - Implement in-memory storage (sync.Map)
  - Add thread-safe operations
  - Write unit tests

- [ ] **1.5 CDEvents Parser**
  - Integrate cdevents/sdk-go
  - Parse incoming events
  - Extract deployment info
  - Handle different event types
  - Write unit tests

- [ ] **1.6 HTTP API**
  - Setup Chi router
  - Implement POST /events
  - Implement GET /deployments/{service}
  - Implement GET /health
  - Add request logging middleware
  - Write handler tests

- [ ] **1.7 Main Entry Point**
  - Wire up all components
  - Graceful shutdown handling
  - Structured logging with slog

- [ ] **1.8 Docker Setup**
  - Create Dockerfile
  - Create docker-compose.yml (bridge only)
  - Test container build and run

- [ ] **1.9 Documentation**
  - Write basic README
  - Document API endpoints
  - Add quick start instructions

#### Deliverables
- Working HTTP server that receives CDEvents
- In-memory storage of deployments
- Query endpoint for deployment info
- Docker container
- Basic README

---

### Phase 2: OpenTelemetry Integration - Week 2

**Goal:** Enrich OpenTelemetry traces with deployment context.

#### Tasks

- [ ] **2.1 OTel Provider Setup**
  - Initialize OTel SDK
  - Configure OTLP exporter
  - Setup trace provider

- [ ] **2.2 Span Processor**
  - Create custom span processor
  - Query deployment info for service
  - Add deployment attributes to spans

- [ ] **2.3 Enricher Logic**
  - Extract service name from span
  - Match to stored deployments
  - Add attributes:
    - `deployment.id`
    - `deployment.version`
    - `deployment.commit`
    - `deployment.environment`
    - `deployment.pipeline_id`

- [ ] **2.4 Sample Application**
  - Create simple Go HTTP service
  - Instrument with OTel
  - Use bridge's span processor
  - Dockerfile for sample app

- [ ] **2.5 Demo Stack**
  - Update docker-compose.yml
  - Add Jaeger service
  - Add sample app service
  - Network configuration

- [ ] **2.6 Demo Script**
  - Script to send test CDEvent
  - Script to make requests to sample app
  - Instructions to view in Jaeger

- [ ] **2.7 Testing**
  - Integration tests for OTel
  - E2E test with Jaeger

#### Deliverables
- OTel span processor enriching traces
- Sample app demonstrating feature
- Docker Compose with Jaeger
- Demo script showing end-to-end flow

---

### Phase 3: Event Correlation (Links) - Week 3

**Goal:** Build event chains using CDEvents links, enable incident-to-commit tracing.

#### Tasks

- [ ] **3.1 Links Parser**
  - Extract links from CDEvents
  - Store link relationships
  - Handle different link types

- [ ] **3.2 Event Storage**
  - Store all event types (not just deployments)
  - Index by event ID
  - Index by chain ID

- [ ] **3.3 Graph Builder**
  - Build in-memory event graph
  - Traverse links to build chains
  - Handle circular references

- [ ] **3.4 Correlator Logic**
  - Given incident, find causing deployment
  - Given deployment, find triggering pipeline
  - Given pipeline, find source commit

- [ ] **3.5 Chain API**
  - Implement GET /chain/{eventId}
  - Return full event chain
  - Include root cause analysis

- [ ] **3.6 Incident Model**
  - Add Incident model
  - Handle incident.detected events
  - Handle incident.resolved events

- [ ] **3.7 DORA Metrics Helper**
  - Calculate Change Failure Rate
  - Calculate MTTR (Mean Time to Recovery)
  - Expose via API or metrics

- [ ] **3.8 Mock Events Generator**
  - Generate realistic event chains
  - Simulate deployment → incident flow
  - Use for demos and testing

#### Deliverables
- Full event chain traversal
- GET /chain API endpoint
- Incident correlation working
- DORA metrics calculation
- Mock event generator

---

### Phase 4: Production Readiness - Week 4

**Goal:** Make the bridge production-ready with persistent storage, monitoring, and CI/CD.

#### Tasks

- [ ] **4.1 Redis Storage**
  - Implement Redis storage backend
  - Handle connection pooling
  - Add TTL for old events
  - Write integration tests

- [ ] **4.2 Prometheus Metrics**
  - Add metrics endpoint
  - Track events received (by type)
  - Track processing duration
  - Track storage size
  - Track errors

- [ ] **4.3 Configuration Enhancements**
  - Production config file
  - Secrets handling
  - Feature flags

- [ ] **4.4 Kubernetes Deployment**
  - Create Deployment manifest
  - Create Service manifest
  - Create ConfigMap
  - Create Kustomization
  - Health/readiness probes

- [ ] **4.5 CI/CD Pipeline**
  - GitHub Actions workflow for CI
  - Run tests on PR
  - Build Docker image
  - Push to registry on release

- [ ] **4.6 Security Scanning**
  - Add CodeQL analysis
  - Add dependency scanning
  - Add container scanning

- [ ] **4.7 Comprehensive Testing**
  - Increase test coverage to 70%+
  - Load testing
  - Chaos testing (optional)

- [ ] **4.8 Documentation**
  - Architecture deep-dive
  - Configuration reference
  - CI/CD integration guides
  - Troubleshooting guide
  - API documentation (OpenAPI)

- [ ] **4.9 Community Files**
  - CONTRIBUTING.md
  - CODE_OF_CONDUCT.md
  - Issue templates
  - PR template
  - CHANGELOG.md

#### Deliverables
- Redis storage backend
- Prometheus metrics
- Kubernetes manifests
- CI/CD pipeline
- Complete documentation
- Community-ready repository

---

## 10. Coding Standards

### Go Best Practices

#### File Organization
```go
// Package comment
package mypackage

// Imports (grouped: stdlib, external, internal)
import (
    "context"
    "fmt"
    
    "github.com/go-chi/chi/v5"
    
    "github.com/user/project/internal/config"
)

// Constants
const (
    DefaultTimeout = 30 * time.Second
)

// Package-level variables (minimize these)
var (
    ErrNotFound = errors.New("not found")
)

// Types (interfaces first, then structs)
type Storage interface {
    Save(ctx context.Context, d *Deployment) error
    Get(ctx context.Context, service string) (*Deployment, error)
}

type MemoryStorage struct {
    data sync.Map
}

// Constructor functions
func NewMemoryStorage() *MemoryStorage {
    return &MemoryStorage{}
}

// Methods
func (s *MemoryStorage) Save(ctx context.Context, d *Deployment) error {
    // implementation
}
```

#### Naming Conventions
```go
// Interfaces: verb + "er" suffix
type Reader interface { ... }
type Storage interface { ... }  // or Storer

// Structs: noun
type Deployment struct { ... }
type EventParser struct { ... }

// Functions: verb + noun
func ParseEvent(data []byte) (*Event, error)
func SaveDeployment(d *Deployment) error

// Variables: camelCase, descriptive
var eventCount int
var lastDeployment *Deployment

// Constants: CamelCase or ALL_CAPS for special values
const MaxRetries = 3
const DefaultPort = 8080
```

#### Error Handling
```go
// Always wrap errors with context
if err != nil {
    return fmt.Errorf("failed to parse cdevent: %w", err)
}

// Define package errors
var (
    ErrNotFound      = errors.New("not found")
    ErrInvalidEvent  = errors.New("invalid event")
    ErrStorageFailed = errors.New("storage operation failed")
)

// Check for specific errors
if errors.Is(err, ErrNotFound) {
    // handle not found
}
```

#### Context Usage
```go
// Always accept context as first parameter
func (s *Storage) Get(ctx context.Context, id string) (*Deployment, error) {
    // Check for cancellation
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }
    
    // Use context with external calls
    resp, err := http.NewRequestWithContext(ctx, "GET", url, nil)
}
```

#### Logging
```go
// Use structured logging (slog)
import "log/slog"

// Create logger with context
logger := slog.With(
    "component", "api",
    "request_id", requestID,
)

// Log with appropriate levels
logger.Debug("processing request", "path", r.URL.Path)
logger.Info("deployment stored", "service", d.Service, "version", d.Version)
logger.Warn("deprecated endpoint called", "endpoint", path)
logger.Error("failed to store deployment", "error", err, "service", d.Service)
```

### Testing Standards

#### Table-Driven Tests
```go
func TestParseEvent(t *testing.T) {
    tests := []struct {
        name    string
        input   []byte
        want    *Event
        wantErr bool
    }{
        {
            name:  "valid service deployed event",
            input: []byte(`{"context":{"type":"dev.cdevents.service.deployed"}}`),
            want:  &Event{Type: "dev.cdevents.service.deployed"},
        },
        {
            name:    "invalid json",
            input:   []byte(`{invalid`),
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := ParseEvent(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("ParseEvent() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if !reflect.DeepEqual(got, tt.want) {
                t.Errorf("ParseEvent() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

#### Test Helpers
```go
// testdata for fixtures
// test/fixtures/valid_event.json

// Helper functions
func newTestServer(t *testing.T) *httptest.Server {
    t.Helper()
    // setup
    return server
}

func loadFixture(t *testing.T, name string) []byte {
    t.Helper()
    data, err := os.ReadFile(filepath.Join("testdata", name))
    if err != nil {
        t.Fatal(err)
    }
    return data
}
```

### Documentation Standards

#### Package Documentation
```go
// Package storage provides interfaces and implementations for
// storing and retrieving deployment information.
//
// The primary interface is Storage, which can be implemented
// by different backends (memory, Redis, etc.).
//
// Example usage:
//
//     store := storage.NewMemoryStorage()
//     err := store.Save(ctx, deployment)
//
package storage
```

#### Function Documentation
```go
// Save stores a deployment in the storage backend.
// It overwrites any existing deployment for the same service/environment combination.
//
// The context can be used to cancel long-running operations.
// Returns ErrStorageFailed if the storage operation fails.
func (s *MemoryStorage) Save(ctx context.Context, d *Deployment) error {
    // implementation
}
```

---

## 11. Testing Strategy

### Test Pyramid

```
         ┌─────────┐
         │   E2E   │  (Few, slow, high confidence)
         │  Tests  │
        ─┴─────────┴─
       ┌─────────────┐
       │ Integration │  (Some, medium speed)
       │    Tests    │
      ─┴─────────────┴─
     ┌─────────────────┐
     │   Unit Tests    │  (Many, fast, isolated)
     │                 │
    ─┴─────────────────┴─
```

### Unit Tests

**Location:** `*_test.go` files alongside source code

**Scope:**
- Individual functions
- Single struct methods
- No external dependencies (mocked)

**Example:**
```go
// internal/cdevents/parser_test.go
func TestParser_Parse(t *testing.T) {
    // Test parsing logic in isolation
}
```

### Integration Tests

**Location:** `test/integration/`

**Scope:**
- Multiple components working together
- Real storage (in-memory or test containers)
- HTTP handlers with real router

**Example:**
```go
// test/integration/api_test.go
func TestAPI_PostEvents(t *testing.T) {
    // Setup real storage
    store := storage.NewMemoryStorage()
    
    // Setup real router with handlers
    router := api.NewRouter(store)
    
    // Make real HTTP requests
    req := httptest.NewRequest("POST", "/api/v1/events", body)
    // Assert responses
}
```

### End-to-End Tests

**Location:** `test/e2e/`

**Scope:**
- Full system with all services
- Docker Compose environment
- Real Jaeger, real sample app

**Example:**
```go
// test/e2e/e2e_test.go
func TestE2E_DeploymentToTrace(t *testing.T) {
    // 1. Send CDEvent to bridge
    // 2. Make request to sample app
    // 3. Query Jaeger for trace
    // 4. Assert deployment attributes present
}
```

### Test Coverage Goals

| Package | Target Coverage |
|---------|-----------------|
| internal/api | 80% |
| internal/cdevents | 90% |
| internal/storage | 85% |
| internal/otel | 75% |
| internal/correlator | 85% |
| pkg/models | 70% |
| **Overall** | **70%+** |

### Running Tests

```bash
# Run all unit tests
make test

# Run with coverage
make test-coverage

# Run integration tests
make test-integration

# Run e2e tests (requires Docker)
make test-e2e

# Run all tests
make test-all
```

---

## 12. Deployment

### Docker

#### Dockerfile
```dockerfile
# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -o /bridge ./cmd/bridge

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /bridge .
COPY configs/config.yaml ./configs/

EXPOSE 8080

ENTRYPOINT ["./bridge"]
```

#### Docker Compose (Demo)
```yaml
version: '3.8'

services:
  bridge:
    build: .
    ports:
      - "8080:8080"
    environment:
      - BRIDGE_LOG_LEVEL=debug
      - BRIDGE_OTEL_ENDPOINT=jaeger:4317
    depends_on:
      - jaeger
    networks:
      - demo

  sample-app:
    build: ./examples/sample-app
    ports:
      - "8081:8081"
    environment:
      - OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger:4317
      - BRIDGE_URL=http://bridge:8080
    depends_on:
      - bridge
      - jaeger
    networks:
      - demo

  jaeger:
    image: jaegertracing/all-in-one:1.54
    ports:
      - "16686:16686"  # UI
      - "4317:4317"    # OTLP gRPC
      - "4318:4318"    # OTLP HTTP
    environment:
      - COLLECTOR_OTLP_ENABLED=true
    networks:
      - demo

  mock-deployer:
    build: ./examples/mock-deployer
    environment:
      - BRIDGE_URL=http://bridge:8080
    depends_on:
      - bridge
    networks:
      - demo
    profiles:
      - demo  # Only start with --profile demo

networks:
  demo:
    driver: bridge
```

### Kubernetes

#### Deployment
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cdevents-otel-bridge
  labels:
    app: cdevents-otel-bridge
spec:
  replicas: 2
  selector:
    matchLabels:
      app: cdevents-otel-bridge
  template:
    metadata:
      labels:
        app: cdevents-otel-bridge
    spec:
      containers:
        - name: bridge
          image: ghcr.io/YOUR_USERNAME/cdevents-otel-bridge:latest
          ports:
            - containerPort: 8080
          env:
            - name: BRIDGE_STORAGE_TYPE
              value: "redis"
            - name: BRIDGE_REDIS_URL
              valueFrom:
                secretKeyRef:
                  name: bridge-secrets
                  key: redis-url
          livenessProbe:
            httpGet:
              path: /api/v1/health
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 10
          readinessProbe:
            httpGet:
              path: /api/v1/health
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 5
          resources:
            requests:
              memory: "64Mi"
              cpu: "100m"
            limits:
              memory: "256Mi"
              cpu: "500m"
```

#### Service
```yaml
apiVersion: v1
kind: Service
metadata:
  name: cdevents-otel-bridge
spec:
  selector:
    app: cdevents-otel-bridge
  ports:
    - port: 80
      targetPort: 8080
  type: ClusterIP
```

---

## 13. Documentation Requirements

### README.md Structure

```markdown
# cdevents-otel-bridge

> Bridge CDEvents to OpenTelemetry for full deployment traceability

[![CI](https://github.com/USER/cdevents-otel-bridge/actions/workflows/ci.yml/badge.svg)](...)
[![Go Report Card](https://goreportcard.com/badge/github.com/USER/cdevents-otel-bridge)](...)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

## The Problem

[Brief description of the 3 AM incident problem]

## The Solution

[How this bridge solves it]

## Quick Start

[5-minute getting started with Docker Compose]

## Features

- ✅ Receive CDEvents from any CI/CD tool
- ✅ Enrich OpenTelemetry traces with deployment context
- ✅ Trace incidents back to commits using CDEvents links
- ✅ Calculate DORA metrics (Change Failure Rate, MTTR)

## Architecture

[Architecture diagram]

## Documentation

- [Getting Started](docs/getting-started.md)
- [Configuration](docs/configuration.md)
- [API Reference](docs/api.md)
- [CI/CD Integration](docs/ci-integration.md)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md)

## License

Apache 2.0
```

### Required Documentation Files

| File | Purpose |
|------|---------|
| README.md | Overview and quick start |
| docs/getting-started.md | Detailed setup guide |
| docs/architecture.md | System design explanation |
| docs/api.md | API reference |
| docs/configuration.md | All config options |
| docs/ci-integration.md | Jenkins, GitHub Actions, etc. |
| CONTRIBUTING.md | How to contribute |
| CODE_OF_CONDUCT.md | Community standards |
| CHANGELOG.md | Version history |

---

## 14. References

### CDEvents

- **Specification**: https://cdevents.dev/docs/
- **Primer**: https://cdevents.dev/docs/primer/
- **Go SDK**: https://github.com/cdevents/sdk-go
- **Examples**: https://github.com/cdevents/sdk-go/tree/main/examples

### OpenTelemetry

- **Go Documentation**: https://opentelemetry.io/docs/languages/go/
- **Getting Started**: https://opentelemetry.io/docs/languages/go/getting-started/
- **SDK Reference**: https://pkg.go.dev/go.opentelemetry.io/otel

### Related Projects

- **Jaeger**: https://www.jaegertracing.io/
- **Grafana Tempo**: https://grafana.com/oss/tempo/
- **Chi Router**: https://github.com/go-chi/chi

### CD Foundation

- **Website**: https://cd.foundation/
- **CDEvents Announcement**: https://cd.foundation/blog/2023/05/08/cdevents-real-world-adoption/
- **DORA Blog**: https://cd.foundation/blog/2025/03/31/getting-started-cdevents-part1/

---

## Appendix A: Sample CDEvents

### service.deployed
```json
{
  "context": {
    "version": "0.4.1",
    "id": "evt-deploy-001",
    "source": "https://jenkins.example.com/job/my-app/123",
    "type": "dev.cdevents.service.deployed.0.4.1",
    "timestamp": "2024-01-15T10:30:00Z",
    "chainId": "chain-001",
    "links": [
      {"linkType": "triggeredBy", "linkId": "evt-pipeline-001"}
    ]
  },
  "subject": {
    "id": "my-app-prod-deploy",
    "source": "https://k8s.example.com/namespaces/prod/deployments/my-app",
    "type": "service",
    "content": {
      "environment": {"id": "production"},
      "artifactId": "my-app:v1.2.3"
    }
  },
  "customData": {
    "commitSha": "abc123def456",
    "pipelineId": "build-456",
    "repository": "github.com/company/my-app"
  }
}
```

### pipelineRun.finished
```json
{
  "context": {
    "version": "0.4.1",
    "id": "evt-pipeline-001",
    "source": "https://jenkins.example.com/job/my-app/123",
    "type": "dev.cdevents.pipelinerun.finished.0.4.1",
    "timestamp": "2024-01-15T10:25:00Z",
    "chainId": "chain-001",
    "links": [
      {"linkType": "triggeredBy", "linkId": "evt-change-001"}
    ]
  },
  "subject": {
    "id": "build-456",
    "source": "https://jenkins.example.com/job/my-app",
    "type": "pipelineRun",
    "content": {
      "pipelineName": "my-app-build",
      "outcome": "success"
    }
  },
  "customData": {
    "duration": "5m30s",
    "commitSha": "abc123def456"
  }
}
```

### incident.detected
```json
{
  "context": {
    "version": "0.4.1",
    "id": "evt-incident-001",
    "source": "https://pagerduty.example.com/incidents/12345",
    "type": "dev.cdevents.incident.detected.0.4.1",
    "timestamp": "2024-01-15T11:00:00Z",
    "chainId": "chain-001",
    "links": [
      {"linkType": "causedBy", "linkId": "evt-deploy-001"}
    ]
  },
  "subject": {
    "id": "incident-12345",
    "source": "https://pagerduty.example.com",
    "type": "incident",
    "content": {
      "service": "my-app",
      "environment": "production",
      "severity": "high",
      "description": "High latency detected"
    }
  }
}
```

---

## Appendix B: Makefile

```makefile
.PHONY: build test run clean docker

# Variables
BINARY_NAME=bridge
DOCKER_IMAGE=cdevents-otel-bridge
VERSION=$(shell git describe --tags --always --dirty)

# Build
build:
	go build -ldflags "-X main.Version=$(VERSION)" -o bin/$(BINARY_NAME) ./cmd/bridge

# Run
run:
	go run ./cmd/bridge

# Test
test:
	go test -v ./...

test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

test-integration:
	go test -v -tags=integration ./test/integration/...

test-e2e:
	docker-compose -f test/e2e/docker-compose.test.yml up --build --abort-on-container-exit

# Lint
lint:
	golangci-lint run

# Docker
docker-build:
	docker build -t $(DOCKER_IMAGE):$(VERSION) .
	docker tag $(DOCKER_IMAGE):$(VERSION) $(DOCKER_IMAGE):latest

docker-run:
	docker run -p 8080:8080 $(DOCKER_IMAGE):latest

# Demo
demo:
	docker-compose up --build

demo-clean:
	docker-compose down -v

# Clean
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html

# Generate
generate:
	go generate ./...

# Dependencies
deps:
	go mod download
	go mod tidy

# Help
help:
	@echo "Available targets:"
	@echo "  build          - Build the binary"
	@echo "  run            - Run locally"
	@echo "  test           - Run unit tests"
	@echo "  test-coverage  - Run tests with coverage"
	@echo "  test-e2e       - Run end-to-end tests"
	@echo "  lint           - Run linter"
	@echo "  docker-build   - Build Docker image"
	@echo "  demo           - Run full demo with Docker Compose"
	@echo "  clean          - Clean build artifacts"
```

---

**End of Specification**

*Last updated: January 2025*
*Version: 1.0.0*
