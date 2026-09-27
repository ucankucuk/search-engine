// Package handler is a thin layer that converts HTTP requests into domain
// types and hands them off to the relevant business logic package
// (search, ingestion). Handlers contain NO business rules — decisions
// like "which field is required", "how is it sorted" live in the
// search/scoring packages. Thanks to this separation, the business logic
// can be run (in unit tests) without HTTP at all.
package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"search-engine/internal/domain"
	"search-engine/internal/observability/metrics"
	"search-engine/internal/search"
)

type SearchHandler struct {
	service *search.Service
}

func NewSearchHandler(service *search.Service) *SearchHandler {
	return &SearchHandler{service: service}
}

// ServeHTTP handles a GET /search?q=...&type=video&page=1&page_size=20
// request.
func (h *SearchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	query := domain.SearchQuery{
		Keyword:  q.Get("q"),
		Page:     atoiOrDefault(q.Get("page"), 1),
		PageSize: atoiOrDefault(q.Get("page_size"), 20),
	}
	if t := q.Get("type"); t != "" {
		ct := domain.ContentType(t)
		query.Type = &ct
	}

	// result is a domain.SearchResult: {results, total, page, page_size,
	// total_pages} — the frontend's pagination UI (total results/page
	// count, jumping to page numbers) needs these fields.
	//
	// SearchLatency DELIBERATELY includes failed requests too (e.g. an
	// empty "q") — the question is "how long does the total lifecycle of
	// a request to this endpoint take", not just the successful ones.
	start := time.Now()
	result, err := h.service.Search(r.Context(), query)
	metrics.SearchLatency.Observe(time.Since(start).Seconds())
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func atoiOrDefault(s string, fallback int) int {
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return fallback
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch err {
	case domain.ErrInvalidQuery:
		status = http.StatusBadRequest
	case domain.ErrContentNotFound:
		status = http.StatusNotFound
	case domain.ErrProviderUnavailable:
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
