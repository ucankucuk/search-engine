package provider

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/sony/gobreaker"

	"search-engine/internal/domain"
)

// countingProvider is a test double that counts the number of calls and
// returns success/failure results in the given order.
type countingProvider struct {
	name  string
	mu    sync.Mutex
	calls int
	// if alwaysErr != nil, EVERY call fails with this error.
	alwaysErr error
}

func (c *countingProvider) Name() string { return c.name }

func (c *countingProvider) Fetch(ctx context.Context) ([]domain.Content, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	if c.alwaysErr != nil {
		return nil, c.alwaysErr
	}
	return []domain.Content{{ExternalID: "ok"}}, nil
}

func (c *countingProvider) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func TestResilient_Fetch_Success(t *testing.T) {
	inner := &countingProvider{name: "test-resilient-success"}
	r := NewResilient(inner, WithTimeout(time.Second))

	items, err := r.Fetch(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 record, got %d", len(items))
	}
}

// TestResilient_Fetch_AllRetriesFail verifies that when the provider keeps
// returning errors, Fetch eventually returns an error. A DELIBERATELY very
// short timeout (WithTimeout) is given: instead of retry/backoff waiting
// for all 3 attempts (200ms+400ms+800ms exponential backoff), the context
// deadline expires immediately and cuts the retry loop short early. This
// keeps the test from taking seconds; what's actually being verified is
// not the full duration of the backoff, but the "persistent failure ->
// Fetch returns an error" behavior.
func TestResilient_Fetch_AllRetriesFail(t *testing.T) {
	inner := &countingProvider{name: "test-resilient-allfail", alwaysErr: errors.New("upstream 503")}
	r := NewResilient(inner, WithTimeout(20*time.Millisecond))

	_, err := r.Fetch(context.Background())
	if err == nil {
		t.Fatal("Fetch should not appear successful when the provider keeps returning errors")
	}
	if inner.callCount() == 0 {
		t.Error("the inner provider appears to have never been called")
	}
}

// TestResilient_CircuitBreaker_OpensAfterConsecutiveFailures verifies
// end-to-end the ReadyToTrip rule in resilient.go (ConsecutiveFailures >
// 5): after 6 consecutive failed Fetches the breaker should open, and the
// next call should be rejected WITHOUT EVER REACHING the provider (with
// gobreaker.ErrOpenState) — this is exactly the guarantee that "a single
// bad provider doesn't stop the system".
func TestResilient_CircuitBreaker_OpensAfterConsecutiveFailures(t *testing.T) {
	inner := &countingProvider{name: "test-resilient-breaker", alwaysErr: errors.New("upstream 503")}

	var mu sync.Mutex
	var transitions []gobreaker.State
	r := NewResilient(inner,
		WithTimeout(20*time.Millisecond), // short timeout: let every Fetch fail quickly
		WithStateChange(func(name string, from, to gobreaker.State) {
			mu.Lock()
			transitions = append(transitions, to)
			mu.Unlock()
		}),
	)

	// ReadyToTrip: ConsecutiveFailures > 5 -> the breaker should open
	// AFTER the 6th failed call. We call it 6 times.
	for i := 0; i < 6; i++ {
		_, _ = r.Fetch(context.Background())
	}

	mu.Lock()
	gotOpen := len(transitions) > 0 && transitions[len(transitions)-1] == gobreaker.StateOpen
	mu.Unlock()
	if !gotOpen {
		t.Fatalf("the breaker should be OPEN after 6 consecutive failures, transitions: %v", transitions)
	}

	callsBefore := inner.callCount()
	_, err := r.Fetch(context.Background())
	if err == nil {
		t.Fatal("Fetch should return an error while the breaker is open")
	}
	if inner.callCount() != callsBefore {
		t.Error("the inner provider should NOT have been reached at all while the breaker is open (gobreaker should reject the request itself)")
	}
}
