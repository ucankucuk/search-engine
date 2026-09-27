package domain

import "errors"

// Error types that are meaningful across the whole application and are
// carried between layers. The HTTP handler catches these errors and maps
// them to the correct status code (e.g. ErrProviderUnavailable -> 503),
// while the storage/search layers remain unaware that HTTP even exists.
var (
	ErrContentNotFound     = errors.New("content not found")
	ErrProviderUnavailable = errors.New("provider is currently unavailable")
	ErrInvalidQuery        = errors.New("invalid search query")
)
