package jsonprovider

import (
	"encoding/json"
	"time"

	"search-engine/internal/domain"
)

// payload is the response shape returned by the real JSON provider (and
// by our own mock provider that uses the same envelope):
//
//	{
//	  "contents": [
//	    {
//	      "id": "v1",
//	      "title": "Go Programming Tutorial",
//	      "type": "video",
//	      "metrics": { "views": 15000, "likes": 1200, "duration": "15:30" },
//	      "published_at": "2024-03-15T10:00:00Z",
//	      "tags": ["programming", "tutorial"]
//	    }
//	  ],
//	  "pagination": { "total": 150, "page": 1, "per_page": 10 }
//	}
//
// The "metrics" fields can vary depending on type (e.g. views/likes/duration
// are expected for a video, reading_time/reactions/comments for an
// article) — that's why they are all kept optional (pointer); whichever
// ones arrive, the code doesn't break, and the ones that don't arrive are
// silently ignored.
type payload struct {
	Contents []content `json:"contents"`
}

type content struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Type        string   `json:"type"` // "video" | "article"
	Metrics     metrics  `json:"metrics"`
	PublishedAt string   `json:"published_at"`
	Tags        []string `json:"tags"`
	// Categories: our own mock provider blends the two schemas together
	// by also carrying the "categories" concept from the XML side. The
	// real provider doesn't send this; when it isn't sent it stays nil
	// and that's fine.
	Categories []string `json:"categories,omitempty"`
}

type metrics struct {
	Views       *int    `json:"views,omitempty"`
	Likes       *int    `json:"likes,omitempty"`
	Duration    *string `json:"duration,omitempty"`
	ReadingTime *int    `json:"reading_time,omitempty"`
	Reactions   *int    `json:"reactions,omitempty"`
	Comments    *int    `json:"comments,omitempty"`
}

type Normalizer struct{}

func (Normalizer) Normalize(providerName string, raw []byte) ([]domain.Content, error) {
	var p payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}

	out := make([]domain.Content, 0, len(p.Contents))
	for _, c := range p.Contents {
		publishedAt, _ := time.Parse(time.RFC3339, c.PublishedAt)

		// tags/categories/duration/comments aren't used in the scoring
		// formula, but we keep them in RawFields so we don't lose
		// information the provider sent — it may be useful in the future
		// (e.g. a detail page, filtering).
		raw := map[string]any{"tags": c.Tags}
		if len(c.Categories) > 0 {
			raw["categories"] = c.Categories
		}
		if c.Metrics.Duration != nil {
			raw["duration"] = *c.Metrics.Duration
		}
		if c.Metrics.Comments != nil {
			raw["comments"] = *c.Metrics.Comments
		}

		out = append(out, domain.Content{
			ExternalID:         c.ID,
			Title:              c.Title,
			Type:               mapContentType(c.Type),
			Views:              intOrZero(c.Metrics.Views),
			Likes:              intOrZero(c.Metrics.Likes),
			ReadingTimeMinutes: intOrZero(c.Metrics.ReadingTime),
			Reactions:          intOrZero(c.Metrics.Reactions),
			PublishedAt:        publishedAt,
			RawFields:          raw,
		})
	}
	return out, nil
}

func intOrZero(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func mapContentType(t string) domain.ContentType {
	if t == "video" {
		return domain.ContentTypeVideo
	}
	return domain.ContentTypeText
}
