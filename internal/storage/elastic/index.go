// Package elastic encapsulates access to Elasticsearch. This package is
// the "derived, rebuildable view for search" layer — none of the data
// here is authoritative, all of it must be reproducible from Mongo (see
// the architecture doc, the CQRS-like separation). This is why RawFields
// (provider-specific, non-standardized fields) is DELIBERATELY not
// indexed here — it lives only in Mongo. ES only holds the fields needed
// for search and sorting.
package elastic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v8"

	"search-engine/internal/domain"
)

const (
	indexName = "contents"

	// candidateWindowSize: the final ordering is done NOT with
	// Elasticsearch's BM25 score, but with our own scoring formula (the
	// scoring package). That is, we take the "first N matches" from ES
	// and RE-SORT them on the application side (the classic "retrieve
	// then re-rank" pattern). candidateWindowSize is the upper bound on
	// that N — fetching the entire index (hundreds of thousands of
	// documents) and sorting it in memory doesn't scale, so we cap it at
	// the top 500 most relevant by BM25. In real production this would
	// be solved either by making ES's own ranking do more of the work
	// (function_score/script_score) or by paginating with search_after.
	candidateWindowSize = 500
)

// Client is a thin wrapper around the official Elasticsearch Go client
// (elastic/go-elasticsearch).
type Client struct {
	es *elasticsearch.Client
}

// NewClient sets up a client connected to the given address(es) and makes
// sure the "contents" index exists with the correct mapping (creating it
// if it doesn't).
func NewClient(addresses ...string) (*Client, error) {
	esClient, err := elasticsearch.NewClient(elasticsearch.Config{Addresses: addresses})
	if err != nil {
		return nil, fmt.Errorf("failed to create elasticsearch client: %w", err)
	}
	c := &Client{es: esClient}
	if err := c.ensureIndex(context.Background()); err != nil {
		return nil, err
	}
	return c, nil
}

// ensureIndex creates the "contents" index with an explicit mapping if it
// doesn't exist. We don't leave the mapping to ES's dynamic field
// guessing — in particular, we DELIBERATELY chose the "turkish" analyzer
// for the "title" field: the large majority of our dataset (our own mock
// provider) produces Turkish titles, and the Turkish analyzer performs
// stemming accordingly (a search for "mikroservisler" also finds titles
// containing "mikroservis").
func (c *Client) ensureIndex(ctx context.Context) error {
	existsRes, err := c.es.Indices.Exists([]string{indexName}, c.es.Indices.Exists.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("index existence check failed: %w", err)
	}
	defer existsRes.Body.Close()
	if existsRes.StatusCode == 200 {
		return nil // already exists
	}

	mapping := `{
		"mappings": {
			"properties": {
				"external_id":           {"type": "keyword"},
				"provider":              {"type": "keyword"},
				"title":                 {"type": "text", "analyzer": "turkish"},
				"type":                  {"type": "keyword"},
				"views":                 {"type": "integer"},
				"likes":                 {"type": "integer"},
				"reading_time_minutes":  {"type": "integer"},
				"reactions":             {"type": "integer"},
				"published_at":          {"type": "date"}
			}
		}
	}`
	createRes, err := c.es.Indices.Create(
		indexName,
		c.es.Indices.Create.WithContext(ctx),
		c.es.Indices.Create.WithBody(strings.NewReader(mapping)),
	)
	if err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}
	defer createRes.Body.Close()
	if createRes.IsError() {
		return fmt.Errorf("failed to create index: %s", createRes.String())
	}
	return nil
}

// esDoc is the shape of the document written to/read from Elasticsearch —
// the subset of domain.Content needed for search (see the package
// comment: RawFields excluded).
type esDoc struct {
	ExternalID         string `json:"external_id"`
	Provider           string `json:"provider"`
	Title              string `json:"title"`
	Type               string `json:"type"`
	Views              int    `json:"views"`
	Likes              int    `json:"likes"`
	ReadingTimeMinutes int    `json:"reading_time_minutes"`
	Reactions          int    `json:"reactions"`
	PublishedAt        string `json:"published_at"`
}

// docID is the combination of provider+external_id. external_id ALONE is
// NOT enough — different providers can use the same ID (e.g. "v1" in the
// real JSON provider and "v1" in the real XML provider are not the same
// content). This is symmetric with the upsert filter in Mongo
// (external_id+provider).
func docID(provider, externalID string) string {
	return provider + "::" + externalID
}

// bulkBatchSize is the maximum number of documents sent in a single
// _bulk request — same rationale as batchSize in the Mongo repository:
// reasonable chunks instead of giant requests.
const bulkBatchSize = 1000

// IndexMany indexes the given content items in bulk (the _bulk API).
// Satisfies the ingestion.Indexer interface. When the same document (same
// provider+external_id) is indexed again, ES overwrites it by ID —
// idempotent, the ES counterpart of the upsert logic in Mongo.
func (c *Client) IndexMany(ctx context.Context, items []domain.Content) error {
	for start := 0; start < len(items); start += bulkBatchSize {
		end := start + bulkBatchSize
		if end > len(items) {
			end = len(items)
		}
		if err := c.bulkIndex(ctx, items[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) bulkIndex(ctx context.Context, items []domain.Content) error {
	var buf bytes.Buffer
	for _, item := range items {
		meta := map[string]any{
			"index": map[string]any{
				"_index": indexName,
				"_id":    docID(item.Provider, item.ExternalID),
			},
		}
		metaLine, err := json.Marshal(meta)
		if err != nil {
			return fmt.Errorf("failed to build bulk meta line: %w", err)
		}
		doc := esDoc{
			ExternalID:         item.ExternalID,
			Provider:           item.Provider,
			Title:              item.Title,
			Type:               string(item.Type),
			Views:              item.Views,
			Likes:              item.Likes,
			ReadingTimeMinutes: item.ReadingTimeMinutes,
			Reactions:          item.Reactions,
			PublishedAt:        item.PublishedAt.Format(time.RFC3339),
		}
		docLine, err := json.Marshal(doc)
		if err != nil {
			return fmt.Errorf("failed to build bulk document line: %w", err)
		}
		buf.Write(metaLine)
		buf.WriteByte('\n')
		buf.Write(docLine)
		buf.WriteByte('\n')
	}

	res, err := c.es.Bulk(bytes.NewReader(buf.Bytes()),
		c.es.Bulk.WithContext(ctx),
		c.es.Bulk.WithIndex(indexName),
	)
	if err != nil {
		return fmt.Errorf("bulk request failed: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return fmt.Errorf("bulk request returned an error: %s", res.String())
	}

	// _bulk can return HTTP 200 even with individual document errors — if
	// the "errors" field is true, at least one document failed. Same
	// logic as with Ordered:false: the other documents in the batch were
	// still written, we just surface the error for warning purposes.
	var parsed struct {
		Errors bool `json:"errors"`
		Items  []struct {
			Index struct {
				Status int    `json:"status"`
				Error  any    `json:"error,omitempty"`
				ID     string `json:"_id"`
			} `json:"index"`
		} `json:"items"`
	}
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return fmt.Errorf("failed to decode bulk response: %w", err)
	}
	if parsed.Errors {
		failed := 0
		for _, it := range parsed.Items {
			if it.Index.Status >= 300 {
				failed++
			}
		}
		return fmt.Errorf("bulk indexing: %d/%d documents failed", failed, len(parsed.Items))
	}
	return nil
}

// Search runs the full-text search query and returns the candidate
// results. We do NOT return the relevance score (BM25) here because the
// formula in the scoring package has no relevance component (see the
// package comment in scoring.go) — here ES is purely a FILTER/RECALL
// layer answering "which documents match", the final ordering is done
// entirely in search.Service with scoring.Score().
func (c *Client) Search(ctx context.Context, query domain.SearchQuery) ([]domain.Content, error) {
	must := []map[string]any{
		{"match": map[string]any{"title": query.Keyword}},
	}
	var filter []map[string]any
	if query.Type != nil {
		filter = append(filter, map[string]any{"term": map[string]any{"type": string(*query.Type)}})
	}

	esQuery := map[string]any{
		"size": candidateWindowSize,
		"query": map[string]any{
			"bool": map[string]any{
				"must":   must,
				"filter": filter,
			},
		},
	}
	body, err := json.Marshal(esQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	res, err := c.es.Search(
		c.es.Search.WithContext(ctx),
		c.es.Search.WithIndex(indexName),
		c.es.Search.WithBody(bytes.NewReader(body)),
	)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("search request returned an error: %s", res.String())
	}

	var parsed struct {
		Hits struct {
			Hits []struct {
				Source esDoc `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}

	out := make([]domain.Content, 0, len(parsed.Hits.Hits))
	for _, hit := range parsed.Hits.Hits {
		publishedAt, _ := time.Parse(time.RFC3339, hit.Source.PublishedAt)
		out = append(out, domain.Content{
			ExternalID:         hit.Source.ExternalID,
			Provider:           hit.Source.Provider,
			Title:              hit.Source.Title,
			Type:               domain.ContentType(hit.Source.Type),
			Views:              hit.Source.Views,
			Likes:              hit.Source.Likes,
			ReadingTimeMinutes: hit.Source.ReadingTimeMinutes,
			Reactions:          hit.Source.Reactions,
			PublishedAt:        publishedAt,
		})
	}
	return out, nil
}
