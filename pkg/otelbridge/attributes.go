/*
Package otelbridge enriches OpenTelemetry spans with deployment context
obtained from a CDEvents-OTel Bridge.

Add it to any Go service that already uses the OpenTelemetry SDK:

	client := otelbridge.NewClient("http://bridge:8080",
		otelbridge.WithEnvironment("production"))
	resolver := otelbridge.NewCachedResolver(client)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSpanProcessor(otelbridge.NewSpanProcessor(resolver)),
	)

Every span the provider creates then carries the deployment.* attributes
listed in this file, so a trace in Jaeger or Tempo shows which version,
commit and pipeline produced it.

This file defines the attribute keys and the mapping from a Deployment to
span attributes. Empty values are omitted so spans stay compact.
*/
package otelbridge

import (
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/iamrajiv/cdevents-otel-bridge/pkg/models"
)

const (
	DeploymentIDKey          = attribute.Key("deployment.id")
	DeploymentVersionKey     = attribute.Key("deployment.version")
	DeploymentCommitKey      = attribute.Key("deployment.commit")
	DeploymentEnvironmentKey = attribute.Key("deployment.environment")
	DeploymentPipelineIDKey  = attribute.Key("deployment.pipeline_id")
	DeploymentRepositoryKey  = attribute.Key("deployment.repository")
	DeploymentBranchKey      = attribute.Key("deployment.branch")
	DeploymentDeployedAtKey  = attribute.Key("deployment.deployed_at")
	DeploymentDeployedByKey  = attribute.Key("deployment.deployed_by")
	DeploymentEventIDKey     = attribute.Key("deployment.event_id")
)

func Attributes(d *models.Deployment) []attribute.KeyValue {
	if d == nil {
		return nil
	}

	attrs := make([]attribute.KeyValue, 0, 10)
	add := func(key attribute.Key, value string) {
		if value != "" {
			attrs = append(attrs, key.String(value))
		}
	}

	id := d.ID
	if id == "" {
		id = d.EventID
	}

	add(DeploymentIDKey, id)
	add(DeploymentVersionKey, d.Version)
	add(DeploymentCommitKey, d.CommitSha)
	add(DeploymentEnvironmentKey, d.Environment)
	add(DeploymentPipelineIDKey, d.PipelineID)
	add(DeploymentRepositoryKey, d.Repository)
	add(DeploymentBranchKey, d.Branch)
	if !d.DeployedAt.IsZero() {
		add(DeploymentDeployedAtKey, d.DeployedAt.UTC().Format(time.RFC3339))
	}
	add(DeploymentDeployedByKey, d.DeployedBy)
	add(DeploymentEventIDKey, d.EventID)

	return attrs
}
