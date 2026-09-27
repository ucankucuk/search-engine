package provider

import (
	"context"
	"fmt"
	"time"

	"search-engine/internal/domain"
)

// Adapter combines a RawFetcher with a Normalizer to turn them into a
// complete Provider. This is the actual piece that makes adding new
// providers cheap: most new providers only need a new RawFetcher and
// reuse an existing Normalizer (JSON or XML).
type Adapter struct {
	name       string
	fetcher    RawFetcher
	normalizer Normalizer
}

func NewAdapter(name string, fetcher RawFetcher, normalizer Normalizer) *Adapter {
	return &Adapter{name: name, fetcher: fetcher, normalizer: normalizer}
}

func (a *Adapter) Name() string { return a.name }

func (a *Adapter) Fetch(ctx context.Context) ([]domain.Content, error) {
	raw, err := a.fetcher.FetchRaw(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to fetch raw data: %w", a.name, err)
	}
	items, err := a.normalizer.Normalize(a.name, raw)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to normalize: %w", a.name, err)
	}
	now := time.Now()
	for i := range items {
		items[i].Provider = a.name
		items[i].FetchedAt = now
	}
	return items, nil
}
