// Package xmlprovider contains the concrete implementation of the
// provider that speaks XML.
package xmlprovider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"search-engine/internal/provider"
)

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
	URL     string
	Timeout time.Duration
}

func Register(cfg Config) {
	provider.Register(provider.NewAdapter(
		"xml-provider-real",
		newHTTPFetcher(cfg.URL, cfg.Timeout),
		Normalizer{},
	))
}
