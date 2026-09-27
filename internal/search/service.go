// Package search contains the business logic for the "what happens when a
// user sends a search query" flow: it fetches candidate results from
// Elasticsearch, re-scores them with the scoring package, and paginates
// them. The HTTP handler calls this package, but this package knows
// nothing about HTTP (request/response, status codes) — so the same
// logic could also be called from a CLI or a gRPC service in the future.
package search

import (
	"context"
	"sort"

	"search-engine/internal/domain"
	"search-engine/internal/scoring"
)

// Searcher is the minimum contract that the subsystem serving the search
// query (Elasticsearch) must provide.
type Searcher interface {
	Search(ctx context.Context, query domain.SearchQuery) ([]domain.Content, error)
}

type Service struct {
	searcher Searcher
}

func NewService(searcher Searcher) *Service {
	return &Service{searcher: searcher}
}

// Search fetches from Elasticsearch the candidates that MATCH the
// keyword, scores each one with scoring.Score (this score is independent
// of the query — see the formula note in the scoring package), sorts by
// final score, paginates, and returns the pagination metadata (total
// results/page count) together with the results — see the package
// comment on domain.SearchResult.
func (s *Service) Search(ctx context.Context, query domain.SearchQuery) (domain.SearchResult, error) {
	if err := validateQuery(query); err != nil {
		return domain.SearchResult{}, err
	}

	candidates, err := s.searcher.Search(ctx, query)
	if err != nil {
		return domain.SearchResult{}, err
	}

	scored := make([]domain.ScoredContent, 0, len(candidates))
	for _, c := range candidates {
		scored = append(scored, scoring.Score(c))
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].FinalScore > scored[j].FinalScore
	})

	page, pageSize := normalizePaging(query.Page, query.PageSize)
	total := len(scored)
	totalPages := 0
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize // round up
	}

	return domain.SearchResult{
		Items:      paginate(scored, page, pageSize),
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// maxKeywordLength/maxPageSize: input validation limits.
//   - maxKeywordLength: there is no inherent reasonable upper bound on the
//     "match" query that will go to Elasticsearch — if someone (by
//     accident or on purpose) sends a "q" that's thousands of characters
//     long, this produces both an unnecessarily expensive ES query and is
//     meaningless (no title is that long). 200 is well above the length
//     of the longest realistic search (a title that's a sentence or two).
//   - maxPageSize: if there's no upper bound on page_size (e.g.
//     "page_size=999999"), normalizePaging would accept it AS-IS and
//     paginate would return the entire candidateWindowSize (500) result
//     set on a SINGLE page — even without malicious intent, an
//     accidentally sent large value would produce an unnecessarily large
//     JSON response. 100 was deliberately kept a bit wider than the
//     largest option offered by the UI (50) — so the API isn't TIGHTLY
//     coupled to the UI's choices.
const (
	maxKeywordLength = 200
	maxPageSize      = 100
)

// validateQuery filters out clearly invalid input that can be rejected
// BEFORE going to any subsystem (Elasticsearch). Unlike normalizePaging
// (which SILENTLY coerces invalid values to reasonable defaults), the
// rule here is: "a meaningless/clearly wrong request is not silently
// fixed, it returns 400 via ErrInvalidQuery" — we want the user to know
// what went wrong.
func validateQuery(query domain.SearchQuery) error {
	if query.Keyword == "" {
		return domain.ErrInvalidQuery
	}
	if len(query.Keyword) > maxKeywordLength {
		return domain.ErrInvalidQuery
	}
	if query.Type != nil {
		switch *query.Type {
		case domain.ContentTypeVideo, domain.ContentTypeText:
			// valid
		default:
			return domain.ErrInvalidQuery
		}
	}
	return nil
}

// normalizePaging coerces invalid/missing page-page_size values into safe
// defaults — both paginate and the Page/PageSize fields on the returned
// SearchResult must use the SAME normalized values, otherwise the
// frontend would think "the page_size I asked for wasn't applied".
//
// WARNING: unlike validateQuery, for page/page_size SILENT correction was
// deliberately chosen here (instead of erroring out) — because values
// like "page=-5" or "page_size=0" usually don't come from a client bug,
// but from harmless defaults the UI sends on its first render, before a
// page number has been decided yet; giving "the most reasonable
// equivalent" instead of greeting the user with an error page is a better
// experience.
func normalizePaging(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return page, pageSize
}

func paginate(items []domain.ScoredContent, page, pageSize int) []domain.ScoredContent {
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []domain.ScoredContent{}
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}
