package provider

import (
	"context"
	"time"

	"github.com/sethvargo/go-retry"
	"github.com/sony/gobreaker"

	"search-engine/internal/domain"
	"search-engine/internal/observability/metrics"
)

// Resilient wraps any Provider with timeout + retry/backoff + circuit
// breaker (Decorator pattern). The inner provider (json/xml adapter)
// knows nothing about resilience; this layer is applied from the outside
// (inside cmd/api/main.go).
type Resilient struct {
	inner   Provider
	timeout time.Duration
	breaker *gobreaker.CircuitBreaker
	onState func(name string, from, to gobreaker.State)
}

type ResilientOption func(*Resilient)

func WithTimeout(d time.Duration) ResilientOption {
	return func(r *Resilient) { r.timeout = d }
}

// WithStateChange fires every time the circuit breaker changes state
// (closed/open/half-open) — the Prometheus gauge is updated from here
// (see internal/observability/metrics).
func WithStateChange(fn func(name string, from, to gobreaker.State)) ResilientOption {
	return func(r *Resilient) { r.onState = fn }
}

func NewResilient(inner Provider, opts ...ResilientOption) *Resilient {
	r := &Resilient{inner: inner, timeout: 5 * time.Second}
	for _, opt := range opts {
		opt(r)
	}
	r.breaker = gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name: inner.Name(),
		// MaxRequests: the number of CONSECUTIVE successes required to go
		// back from Half-Open to Closed (healthy). DELIBERATELY chosen as
		// 3, not 1: if it closes instantly on a single successful
		// attempt (1), the Half-Open state lasts too briefly to be
		// visible (only as long as a single Fetch call) — we need the
		// breaker to STAY in Half-Open for a few ingestion rounds so the
		// "first it tries, then it closes" step can be SEEN SEPARATELY
		// in Grafana. This value was tuned together with the demo env
		// variables in docker-compose.yml (TOGGLE_INTERVAL_SECONDS,
		// INGEST_INTERVAL_SECONDS) — see the comments there.
		MaxRequests: 3,
		Timeout:     10 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures > 5
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			if r.onState != nil {
				r.onState(name, from, to)
			}
		},
	})
	Register(r)
	return r
}

func (r *Resilient) Name() string { return r.inner.Name() }

// Fetch calls the provider with timeout+retry+circuit breaker and records
// both the RESULT (RequestsTotal: success/failure) and the DURATION
// (RequestDuration) to Prometheus. The duration covers ALL of the
// retries (the full attempt+backoff time, if any) — it answers "how long
// did a Fetch call to this provider take in total", not the duration of a
// single HTTP attempt.
func (r *Resilient) Fetch(ctx context.Context) ([]domain.Content, error) {
	start := time.Now()

	result, err := r.breaker.Execute(func() (any, error) {
		fetchCtx, cancel := context.WithTimeout(ctx, r.timeout)
		defer cancel()

		backoff := retry.WithMaxRetries(3, retry.NewExponential(200*time.Millisecond))
		var items []domain.Content
		err := retry.Do(fetchCtx, backoff, func(ctx context.Context) error {
			fetched, err := r.inner.Fetch(ctx)
			if err != nil {
				return retry.RetryableError(err)
			}
			items = fetched
			return nil
		})
		return items, err
	})

	metrics.RequestDuration.WithLabelValues(r.Name()).Observe(time.Since(start).Seconds())
	status := "success"
	if err != nil {
		status = "failure"
	}
	metrics.RequestsTotal.WithLabelValues(r.Name(), status).Inc()

	if err != nil {
		return nil, err
	}
	return result.([]domain.Content), nil
}
