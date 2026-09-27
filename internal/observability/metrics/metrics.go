// Package metrics defines, in a single place, all the custom metrics to
// be exposed to Prometheus. This collects the answer to "which metrics
// exist" into a single file in the codebase — the provider package or the
// search package doesn't define its own metrics scattered around, it
// imports them from here.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// CircuitState holds the circuit breaker state for each provider
	// (0=closed, 1=half-open, 2=open). It is the primary data source for
	// the "Provider Health" panel in Grafana.
	CircuitState = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "provider_circuit_state",
		Help: "Provider circuit breaker state: 0=closed, 1=half-open, 2=open",
	}, []string{"provider"})

	// RequestsTotal counts the result of every call made to a provider.
	RequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "provider_requests_total",
		Help: "Total number of requests made to the provider",
	}, []string{"provider", "status"})

	// RequestDuration holds the duration distribution of provider calls —
	// used to prove that the timeout setting was chosen correctly.
	RequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "provider_request_duration_seconds",
		Help:    "Provider call duration (seconds)",
		Buckets: prometheus.DefBuckets,
	}, []string{"provider"})

	// SearchLatency holds the end-to-end latency of the /search endpoint.
	SearchLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "search_request_duration_seconds",
		Help:    "End-to-end search request duration (seconds)",
		Buckets: prometheus.DefBuckets,
	})

	// PendingUnindexed holds the number of records that have been written
	// to Mongo but not yet successfully indexed into Elasticsearch
	// (indexed=false) — the backlog size observed by
	// ingestion.Job.Reconcile on every round (see the package comment on
	// domain.Content.Indexed). This makes it externally visible whether
	// the Mongo↔ES consistency mechanism is working SILENTLY on its own
	// — the ContentIndexingBacklog alert in
	// deployments/prometheus/rules.yml is based on this metric.
	PendingUnindexed = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "content_pending_indexing",
		Help: "Number of records written in Mongo but not yet indexed into Elasticsearch",
	})
)
