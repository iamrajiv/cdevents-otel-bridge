/*
Package otelbridge enriches OpenTelemetry spans with deployment context.

This file defines DeploymentResolver, the single abstraction the span
processor depends on, and CachedResolver, which wraps any resolver with a
time-bounded cache. Span creation is on the hot path of every request, so
the processor must never make a network call per span: the cache keeps the
last answer per service for CacheTTL and remembers "not found" for
NegativeTTL so unknown services do not trigger repeated lookups either.
*/
package otelbridge

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/iamrajiv/cdevents-otel-bridge/pkg/models"
)

const (
	DefaultCacheTTL    = 30 * time.Second
	DefaultNegativeTTL = 10 * time.Second
)

var ErrNotFound = errors.New("otelbridge: deployment not found")

type DeploymentResolver interface {
	ResolveDeployment(ctx context.Context, service string) (*models.Deployment, error)
}

type ResolverFunc func(ctx context.Context, service string) (*models.Deployment, error)

func (f ResolverFunc) ResolveDeployment(ctx context.Context, service string) (*models.Deployment, error) {
	return f(ctx, service)
}

type CachedResolver struct {
	inner       DeploymentResolver
	cacheTTL    time.Duration
	negativeTTL time.Duration
	now         func() time.Time

	mu      sync.Mutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	deployment *models.Deployment
	err        error
	expiresAt  time.Time
}

type CacheOption func(*CachedResolver)

func WithCacheTTL(ttl time.Duration) CacheOption {
	return func(r *CachedResolver) {
		if ttl > 0 {
			r.cacheTTL = ttl
		}
	}
}

func WithNegativeCacheTTL(ttl time.Duration) CacheOption {
	return func(r *CachedResolver) {
		if ttl > 0 {
			r.negativeTTL = ttl
		}
	}
}

func NewCachedResolver(inner DeploymentResolver, opts ...CacheOption) *CachedResolver {
	r := &CachedResolver{
		inner:       inner,
		cacheTTL:    DefaultCacheTTL,
		negativeTTL: DefaultNegativeTTL,
		now:         time.Now,
		entries:     make(map[string]cacheEntry),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (r *CachedResolver) ResolveDeployment(ctx context.Context, service string) (*models.Deployment, error) {
	now := r.now()

	r.mu.Lock()
	entry, ok := r.entries[service]
	r.mu.Unlock()

	if ok && now.Before(entry.expiresAt) {
		return entry.deployment, entry.err
	}

	deployment, err := r.inner.ResolveDeployment(ctx, service)

	switch {
	case err == nil:
		r.store(service, cacheEntry{deployment: deployment, expiresAt: now.Add(r.cacheTTL)})
	case errors.Is(err, ErrNotFound):
		r.store(service, cacheEntry{err: ErrNotFound, expiresAt: now.Add(r.negativeTTL)})
	default:
		if ok && entry.err == nil {
			return entry.deployment, nil
		}
	}

	return deployment, err
}

func (r *CachedResolver) store(service string, entry cacheEntry) {
	r.mu.Lock()
	r.entries[service] = entry
	r.mu.Unlock()
}

func (r *CachedResolver) Invalidate(service string) {
	r.mu.Lock()
	delete(r.entries, service)
	r.mu.Unlock()
}

func (r *CachedResolver) Reset() {
	r.mu.Lock()
	r.entries = make(map[string]cacheEntry)
	r.mu.Unlock()
}
