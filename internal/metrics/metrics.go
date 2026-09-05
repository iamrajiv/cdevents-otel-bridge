/*
Package metrics exposes Prometheus metrics for the bridge.

Metrics live in a private registry (with the standard Go and process
collectors) rather than the global default one so several bridge instances
can be created in one process, which the tests rely on. The exported series
are:

  - cdevents_received_total{type}          events accepted and stored
  - cdevents_rejected_total{reason}        events refused (parse, validation,
    payload_too_large, storage)
  - cdevents_processing_duration_seconds   time from request receipt to
    storage completion for accepted events
*/
package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	registry *prometheus.Registry
	received *prometheus.CounterVec
	rejected *prometheus.CounterVec
	duration prometheus.Histogram
}

func New() *Metrics {
	registry := prometheus.NewRegistry()

	m := &Metrics{
		registry: registry,
		received: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "cdevents_received_total",
			Help: "Total number of CDEvents accepted and stored, by short event type.",
		}, []string{"type"}),
		rejected: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "cdevents_rejected_total",
			Help: "Total number of CDEvents rejected, by reason.",
		}, []string{"reason"}),
		duration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "cdevents_processing_duration_seconds",
			Help:    "Time taken to parse, validate and store an accepted CDEvent.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
		}),
	}

	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.received,
		m.rejected,
		m.duration,
	)

	return m
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *Metrics) EventReceived(eventType string) {
	m.received.WithLabelValues(eventType).Inc()
}

func (m *Metrics) EventRejected(reason string) {
	m.rejected.WithLabelValues(reason).Inc()
}

func (m *Metrics) ObserveProcessing(d time.Duration) {
	m.duration.Observe(d.Seconds())
}
