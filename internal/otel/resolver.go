/*
Package otel wires OpenTelemetry into the bridge itself.

StorageResolver adapts the bridge's storage backend to the
otelbridge.DeploymentResolver interface. It lets the bridge use the same
span processor external applications use, but backed by a direct storage
lookup instead of an HTTP round trip to itself.
*/
package otel

import (
	"context"
	"errors"

	"github.com/iamrajiv/cdevents-otel-bridge/internal/storage"
	"github.com/iamrajiv/cdevents-otel-bridge/pkg/models"
	"github.com/iamrajiv/cdevents-otel-bridge/pkg/otelbridge"
)

type StorageResolver struct {
	store storage.Storage
}

func NewStorageResolver(store storage.Storage) *StorageResolver {
	return &StorageResolver{store: store}
}

func (r *StorageResolver) ResolveDeployment(ctx context.Context, service string) (*models.Deployment, error) {
	deployment, err := r.store.GetLatestDeployment(ctx, service)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, otelbridge.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return deployment, nil
}
