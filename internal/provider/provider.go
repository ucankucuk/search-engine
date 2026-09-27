// Package provider abstracts access to external content providers behind
// a single contract (the Provider interface). What this package
// deliberately does NOT know: which provider speaks JSON and which speaks
// XML (see the json/ and xml/ subpackages), how the data will be stored
// (see internal/storage), and which HTTP endpoint will trigger it (see
// internal/transport).
//
// Extensibility is embodied right here: adding a new provider requires no
// change to this file at all — it's enough to add a new subpackage plus a
// Register() call to the registry (see the README's "Adding a new
// provider" section).
package provider

import (
	"context"
	"fmt"
	"sync"

	"search-engine/internal/domain"
)

// Provider is the single contract every content source must provide.
type Provider interface {
	Name() string
	Fetch(ctx context.Context) ([]domain.Content, error)
}

var (
	mu       sync.RWMutex
	registry = map[string]Provider{}
)

// Register adds a provider to the global registry (or replaces an
// existing one — this is why the Resilient wrapper uses it, see
// resilient.go).
func Register(p Provider) {
	mu.Lock()
	defer mu.Unlock()
	registry[p.Name()] = p
}

// All returns all registered providers. The ingestion job iterates over
// this slice and never imports any concrete type (json/xml adapter) at
// all.
func All() []Provider {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Provider, 0, len(registry))
	for _, p := range registry {
		out = append(out, p)
	}
	return out
}

// Get returns a single provider by name — used by the "refresh only this
// provider" refresh endpoint.
func Get(name string) (Provider, error) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("provider %q is not registered", name)
	}
	return p, nil
}
