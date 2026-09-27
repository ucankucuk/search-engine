package search

import (
	"context"
	"errors"
	"testing"
	"time"

	"search-engine/internal/domain"
)

// fakeSearcher is a test double for the Searcher interface — it lets us
// test Service.Search's own logic (validation, scoring, sorting,
// pagination) in isolation without ever touching a real Elasticsearch.
type fakeSearcher struct {
	items []domain.Content
	err   error
}

func (f *fakeSearcher) Search(ctx context.Context, query domain.SearchQuery) ([]domain.Content, error) {
	return f.items, f.err
}

func TestNormalizePaging(t *testing.T) {
	cases := []struct {
		name             string
		page, pageSize   int
		wantPage, wantPS int
	}{
		{"valid values unchanged", 2, 10, 2, 10},
		{"page < 1 -> coerced to 1", 0, 10, 1, 10},
		{"negative page -> coerced to 1", -5, 10, 1, 10},
		{"pageSize < 1 -> default 20", 1, 0, 1, 20},
		{"pageSize exceeding the upper bound is coerced to maxPageSize", 1, 999, 1, maxPageSize},
		{"pageSize exactly maxPageSize -> unchanged", 1, maxPageSize, 1, maxPageSize},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotPage, gotPS := normalizePaging(tc.page, tc.pageSize)
			if gotPage != tc.wantPage || gotPS != tc.wantPS {
				t.Errorf("normalizePaging(%d, %d) = (%d, %d), want (%d, %d)",
					tc.page, tc.pageSize, gotPage, gotPS, tc.wantPage, tc.wantPS)
			}
		})
	}
}

func TestValidateQuery(t *testing.T) {
	videoType := domain.ContentTypeVideo
	invalidType := domain.ContentType("podcast")

	cases := []struct {
		name    string
		query   domain.SearchQuery
		wantErr bool
	}{
		{"valid query", domain.SearchQuery{Keyword: "go"}, false},
		{"empty keyword is rejected", domain.SearchQuery{Keyword: ""}, true},
		{"too-long keyword is rejected", domain.SearchQuery{Keyword: repeatChar("a", maxKeywordLength+1)}, true},
		{"keyword at exactly the limit is accepted", domain.SearchQuery{Keyword: repeatChar("a", maxKeywordLength)}, false},
		{"valid type is accepted", domain.SearchQuery{Keyword: "go", Type: &videoType}, false},
		{"invalid type is rejected", domain.SearchQuery{Keyword: "go", Type: &invalidType}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateQuery(tc.query)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateQuery(%+v) error = %v, wantErr %v", tc.query, err, tc.wantErr)
			}
			if err != nil && !errors.Is(err, domain.ErrInvalidQuery) {
				t.Errorf("expected error domain.ErrInvalidQuery, got: %v", err)
			}
		})
	}
}

func repeatChar(s string, n int) string {
	out := make([]byte, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, s[0])
	}
	return string(out)
}

func TestService_Search_EmptyKeyword(t *testing.T) {
	svc := NewService(&fakeSearcher{})
	_, err := svc.Search(context.Background(), domain.SearchQuery{Keyword: ""})
	if !errors.Is(err, domain.ErrInvalidQuery) {
		t.Fatalf("expected ErrInvalidQuery, got: %v", err)
	}
}

func TestService_Search_SearcherError(t *testing.T) {
	boom := errors.New("es crashed")
	svc := NewService(&fakeSearcher{err: boom})
	_, err := svc.Search(context.Background(), domain.SearchQuery{Keyword: "go"})
	if !errors.Is(err, boom) {
		t.Fatalf("expected %v, got: %v", boom, err)
	}
}

// TestService_Search_SortsAndPaginates verifies the most critical
// behavior of Service.Search: candidates should be sorted in DESCENDING
// order by final score, and page/page-size should be applied consistently
// — see the domain.SearchResult note that "Total is not ES's raw match
// count".
func TestService_Search_SortsAndPaginates(t *testing.T) {
	now := time.Now()
	// We deliberately give the "low-scoring" content FIRST and the
	// "high-scoring" one SECOND — to prove that sorting is actually
	// happening (that it doesn't rely on input order).
	items := []domain.Content{
		{ExternalID: "low", Type: domain.ContentTypeText, ReadingTimeMinutes: 1, PublishedAt: now},
		{ExternalID: "high", Type: domain.ContentTypeVideo, Views: 1_000_000, Likes: 100_000, PublishedAt: now},
		{ExternalID: "mid", Type: domain.ContentTypeText, ReadingTimeMinutes: 10, Reactions: 500, PublishedAt: now},
	}
	svc := NewService(&fakeSearcher{items: items})

	result, err := svc.Search(context.Background(), domain.SearchQuery{Keyword: "go", Page: 1, PageSize: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 3 {
		t.Errorf("Total = %d, want 3", result.Total)
	}
	if result.TotalPages != 2 {
		t.Errorf("TotalPages = %d, want 2 (3 results, page_size=2 -> rounded up)", result.TotalPages)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 results per page, got %d", len(result.Items))
	}
	if result.Items[0].ExternalID != "high" {
		t.Errorf("expected the highest-scoring result ('high') first, got '%s'", result.Items[0].ExternalID)
	}
	// The second page should contain the single remaining result
	// (whichever of "low" or "mid" is lower).
	result2, err := svc.Search(context.Background(), domain.SearchQuery{Keyword: "go", Page: 2, PageSize: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result2.Items) != 1 {
		t.Fatalf("expected 1 result on the second page, got %d", len(result2.Items))
	}
}
