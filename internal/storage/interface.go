/*
Package storage provides the abstraction for persisting CDEvents and
deployment records.

The Storage interface is implemented by an in-memory backend (development,
tests, single instance) and a Redis backend (production). Both keep two kinds
of records:

  - Deployments, keyed by service and environment. Saving a deployment for
    the same service/environment replaces the previous one, so a lookup always
    returns what is currently running there.
  - Events, keyed by CDEvent id, with a secondary index by chainId so the
    correlator can find every event that belongs to the same delivery chain.

Implementations must return ErrNotFound (possibly wrapped) when a record does
not exist. Listing operations return deployments newest first so API output
is stable regardless of backend.
*/
package storage

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/iamrajiv/cdevents-otel-bridge/internal/cdevents"
	"github.com/iamrajiv/cdevents-otel-bridge/pkg/models"
)

var ErrNotFound = errors.New("not found")

type ListFilter struct {
	Environment string
	Since       time.Time
	Limit       int
}

type Storage interface {
	SaveDeployment(ctx context.Context, d *models.Deployment) error
	GetDeployment(ctx context.Context, service, environment string) (*models.Deployment, error)
	GetLatestDeployment(ctx context.Context, service string) (*models.Deployment, error)
	ListDeployments(ctx context.Context, filter ListFilter) ([]*models.Deployment, error)
	SaveEvent(ctx context.Context, event *cdevents.StoredEvent) error
	GetEvent(ctx context.Context, eventID string) (*cdevents.StoredEvent, error)
	GetEventsByChain(ctx context.Context, chainID string) ([]*cdevents.StoredEvent, error)
	Ping(ctx context.Context) error
	Close() error
}

func (f ListFilter) matches(d *models.Deployment) bool {
	if f.Environment != "" && d.Environment != f.Environment {
		return false
	}
	if !f.Since.IsZero() && d.DeployedAt.Before(f.Since) {
		return false
	}
	return true
}

func applyFilter(deployments []*models.Deployment, filter ListFilter) []*models.Deployment {
	result := make([]*models.Deployment, 0, len(deployments))
	for _, d := range deployments {
		if filter.matches(d) {
			result = append(result, d)
		}
	}

	sortDeploymentsNewestFirst(result)

	if filter.Limit > 0 && len(result) > filter.Limit {
		result = result[:filter.Limit]
	}
	return result
}

func sortDeploymentsNewestFirst(deployments []*models.Deployment) {
	sort.SliceStable(deployments, func(i, j int) bool {
		if !deployments[i].DeployedAt.Equal(deployments[j].DeployedAt) {
			return deployments[i].DeployedAt.After(deployments[j].DeployedAt)
		}
		if deployments[i].Service != deployments[j].Service {
			return deployments[i].Service < deployments[j].Service
		}
		return deployments[i].Environment < deployments[j].Environment
	})
}

func latestDeployment(deployments []*models.Deployment) (*models.Deployment, error) {
	if len(deployments) == 0 {
		return nil, ErrNotFound
	}
	sortDeploymentsNewestFirst(deployments)
	return deployments[0], nil
}

func sortEventsNewestFirst(events []*cdevents.StoredEvent) {
	sort.SliceStable(events, func(i, j int) bool {
		if !events[i].Timestamp.Equal(events[j].Timestamp) {
			return events[i].Timestamp.After(events[j].Timestamp)
		}
		return events[i].ID < events[j].ID
	})
}
