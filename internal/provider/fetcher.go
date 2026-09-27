package provider

import (
	"context"

	"search-engine/internal/domain"
)

// RawFetcher knows how to reach a single external endpoint and returns the
// response untouched (as raw bytes). If two providers speak the same
// format but use a different URL/auth, ONLY this layer changes.
type RawFetcher interface {
	FetchRaw(ctx context.Context) ([]byte, error)
}

// Normalizer knows how to convert a provider's raw data (JSON, XML,
// whatever) into the common domain.Content shape.
type Normalizer interface {
	Normalize(providerName string, raw []byte) ([]domain.Content, error)
}
