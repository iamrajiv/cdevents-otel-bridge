/*
Unit tests for the bridge HTTP client and the caching resolver.
*/
package otelbridge

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iamrajiv/cdevents-otel-bridge/pkg/models"
)

func newBridgeStub(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	return server, &calls
}

func TestClientGetDeployment(t *testing.T) {
	server, _ := newBridgeStub(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/deployments/my%20app" && r.URL.Path != "/api/v1/deployments/my app" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.URL.Query().Get("environment") != "production" {
			t.Errorf("expected environment query, got %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"service":"my app","deployment":{"id":"dep-1","service":"my app","version":"v1.2.3","commitSha":"abc","environment":"production"}}`))
	})

	client := NewClient(server.URL + "/")
	d, err := client.GetDeployment(context.Background(), "my app", "production")
	if err != nil {
		t.Fatalf("GetDeployment failed: %v", err)
	}
	if d.Version != "v1.2.3" || d.CommitSha != "abc" || d.ID != "dep-1" {
		t.Errorf("unexpected deployment: %+v", d)
	}
}

func TestClientResolveUsesConfiguredEnvironment(t *testing.T) {
	server, _ := newBridgeStub(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("environment") != "staging" {
			t.Errorf("expected staging, got %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"deployment":{"version":"v9"}}`))
	})

	client := NewClient(server.URL, WithEnvironment("staging"))
	d, err := client.ResolveDeployment(context.Background(), "svc")
	if err != nil || d.Version != "v9" {
		t.Errorf("ResolveDeployment = %+v, %v", d, err)
	}
}

func TestClientNoEnvironmentOmitsQuery(t *testing.T) {
	server, _ := newBridgeStub(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("expected no query, got %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"deployment":{"version":"v1"}}`))
	})

	if _, err := NewClient(server.URL).ResolveDeployment(context.Background(), "svc"); err != nil {
		t.Fatal(err)
	}
}

func TestClientErrors(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		wantNotFd bool
	}{
		{name: "not found", status: http.StatusNotFound, body: `{"error":"not_found"}`, wantNotFd: true},
		{name: "server error", status: http.StatusInternalServerError, body: `boom`},
		{name: "invalid json", status: http.StatusOK, body: `{not json`},
		{name: "missing deployment", status: http.StatusOK, body: `{"service":"x"}`, wantNotFd: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, _ := newBridgeStub(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})

			_, err := NewClient(server.URL).ResolveDeployment(context.Background(), "svc")
			if err == nil {
				t.Fatal("expected error")
			}
			if errors.Is(err, ErrNotFound) != tt.wantNotFd {
				t.Errorf("ErrNotFound = %v, want %v (err: %v)", errors.Is(err, ErrNotFound), tt.wantNotFd, err)
			}
		})
	}

	if _, err := NewClient("http://localhost:1").ResolveDeployment(context.Background(), ""); err == nil {
		t.Error("expected error for empty service")
	}
	if _, err := NewClient("http://127.0.0.1:1").ResolveDeployment(context.Background(), "svc"); err == nil {
		t.Error("expected connection error")
	}
}

func TestClientTimeout(t *testing.T) {
	server, _ := newBridgeStub(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	})

	client := NewClient(server.URL, WithRequestTimeout(50*time.Millisecond))
	start := time.Now()
	_, err := client.ResolveDeployment(context.Background(), "svc")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("request took %v, timeout not applied", elapsed)
	}
}

func TestClientHonorsContext(t *testing.T) {
	server, _ := newBridgeStub(t, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if _, err := NewClient(server.URL, WithHTTPClient(&http.Client{})).ResolveDeployment(ctx, "svc"); err == nil {
		t.Error("expected context deadline error")
	}
}

type countingResolver struct {
	calls      atomic.Int32
	deployment *models.Deployment
	err        error
}

func (c *countingResolver) ResolveDeployment(context.Context, string) (*models.Deployment, error) {
	c.calls.Add(1)
	return c.deployment, c.err
}

func TestCachedResolverCachesPositiveResults(t *testing.T) {
	inner := &countingResolver{deployment: &models.Deployment{Version: "v1"}}
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	cached := NewCachedResolver(inner, WithCacheTTL(time.Minute))
	cached.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		d, err := cached.ResolveDeployment(context.Background(), "svc")
		if err != nil || d.Version != "v1" {
			t.Fatalf("ResolveDeployment = %+v, %v", d, err)
		}
	}
	if inner.calls.Load() != 1 {
		t.Errorf("inner called %d times, want 1", inner.calls.Load())
	}

	now = now.Add(2 * time.Minute)
	if _, err := cached.ResolveDeployment(context.Background(), "svc"); err != nil {
		t.Fatal(err)
	}
	if inner.calls.Load() != 2 {
		t.Errorf("inner called %d times after expiry, want 2", inner.calls.Load())
	}

	if _, err := cached.ResolveDeployment(context.Background(), "other"); err != nil {
		t.Fatal(err)
	}
	if inner.calls.Load() != 3 {
		t.Errorf("different service should miss the cache, calls=%d", inner.calls.Load())
	}
}

func TestCachedResolverCachesNotFound(t *testing.T) {
	inner := &countingResolver{err: ErrNotFound}
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	cached := NewCachedResolver(inner, WithNegativeCacheTTL(10*time.Second))
	cached.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if _, err := cached.ResolveDeployment(context.Background(), "svc"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	}
	if inner.calls.Load() != 1 {
		t.Errorf("inner called %d times, want 1", inner.calls.Load())
	}

	now = now.Add(11 * time.Second)
	_, _ = cached.ResolveDeployment(context.Background(), "svc")
	if inner.calls.Load() != 2 {
		t.Errorf("negative entry should expire, calls=%d", inner.calls.Load())
	}
}

func TestCachedResolverServesStaleOnTransientError(t *testing.T) {
	inner := &countingResolver{deployment: &models.Deployment{Version: "v1"}}
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	cached := NewCachedResolver(inner, WithCacheTTL(time.Second))
	cached.now = func() time.Time { return now }

	if _, err := cached.ResolveDeployment(context.Background(), "svc"); err != nil {
		t.Fatal(err)
	}

	now = now.Add(5 * time.Second)
	inner.err = errors.New("bridge down")
	inner.deployment = nil

	d, err := cached.ResolveDeployment(context.Background(), "svc")
	if err != nil || d == nil || d.Version != "v1" {
		t.Errorf("expected stale value on transient error, got %+v, %v", d, err)
	}

	if _, err := cached.ResolveDeployment(context.Background(), "never-seen"); err == nil {
		t.Error("transient error without cached value should propagate")
	}
}

func TestCachedResolverInvalidateAndReset(t *testing.T) {
	inner := &countingResolver{deployment: &models.Deployment{Version: "v1"}}
	cached := NewCachedResolver(inner, WithCacheTTL(0), WithNegativeCacheTTL(0))

	_, _ = cached.ResolveDeployment(context.Background(), "a")
	_, _ = cached.ResolveDeployment(context.Background(), "b")
	cached.Invalidate("a")
	_, _ = cached.ResolveDeployment(context.Background(), "a")
	_, _ = cached.ResolveDeployment(context.Background(), "b")
	if inner.calls.Load() != 3 {
		t.Errorf("calls = %d, want 3", inner.calls.Load())
	}

	cached.Reset()
	_, _ = cached.ResolveDeployment(context.Background(), "b")
	if inner.calls.Load() != 4 {
		t.Errorf("calls after reset = %d, want 4", inner.calls.Load())
	}
}

func TestResolverFunc(t *testing.T) {
	f := ResolverFunc(func(context.Context, string) (*models.Deployment, error) {
		return &models.Deployment{Version: "fn"}, nil
	})
	d, err := f.ResolveDeployment(context.Background(), "x")
	if err != nil || d.Version != "fn" {
		t.Errorf("ResolverFunc = %+v, %v", d, err)
	}
}
