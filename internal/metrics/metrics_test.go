/*
Unit tests for the metrics registry and exposition handler.
*/
package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetricsExposition(t *testing.T) {
	m := New()
	m.EventReceived("service.deployed")
	m.EventReceived("service.deployed")
	m.EventRejected("validation")
	m.ObserveProcessing(5 * time.Millisecond)

	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	body := rec.Body.String()
	for _, want := range []string{
		`cdevents_received_total{type="service.deployed"} 2`,
		`cdevents_rejected_total{reason="validation"} 1`,
		`cdevents_processing_duration_seconds_count 1`,
		`go_goroutines`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics output missing %q", want)
		}
	}
}

func TestIndependentRegistries(t *testing.T) {
	a := New()
	b := New()
	a.EventReceived("x")

	rec := httptest.NewRecorder()
	b.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if strings.Contains(rec.Body.String(), `cdevents_received_total{type="x"}`) {
		t.Error("registries should be independent")
	}
}
