// Package jsonprovider contains the concrete implementation of the
// providers that speak JSON (JSON Provider A and JSON Provider B). This
// package is never imported from outside (from the ingestion job) — it is
// only wired up via a Register() call inside cmd/api/main.go.
package jsonprovider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"search-engine/internal/provider"
)

// httpFetcher is a RawFetcher for any JSON provider reachable via a plain
// HTTP GET. The same struct is used for both "JSON provider A" and "JSON
// provider B" — only the config (URL) differs.
type httpFetcher struct {
	baseURL string
	client  *http.Client
}

func newHTTPFetcher(baseURL string, timeout time.Duration) *httpFetcher {
	return &httpFetcher{baseURL: baseURL, client: &http.Client{Timeout: timeout}}
}

func (f *httpFetcher) FetchRaw(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.baseURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

type Config struct {
	ProviderAURL string
	ProviderBURL string
	Timeout      time.Duration
}

// Register registers both JSON providers based on the config:
//   - ProviderAURL: the real, externally provided JSON provider
//   - ProviderBURL: the JSON provider we designed/run ourselves
//     (mocks/json-provider-a) — since it speaks the same envelope
//     (contents/metrics/tags), it can be parsed with the SAME Normalizer.
//
// This is the ONLY place that needs to change when a third JSON provider
// is added.
func Register(cfg Config) {
	provider.Register(provider.NewAdapter(
		"json-provider-real",
		newHTTPFetcher(cfg.ProviderAURL, cfg.Timeout),
		Normalizer{},
	))
	provider.Register(provider.NewAdapter(
		"json-provider-own",
		newHTTPFetcher(cfg.ProviderBURL, cfg.Timeout),
		Normalizer{},
	))
}
