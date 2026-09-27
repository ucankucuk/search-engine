// Package scoring implements the content scoring formula. This package
// knows neither HTTP nor Elasticsearch — it is a pure set of functions,
// which makes it easy to unit test, and means that ONLY this file changes
// when the formula changes.
//
// Formula (matching the given spec one-to-one):
//
//	Final Score = (Base Score * Content Type Coefficient) + Freshness Score + Engagement Score
//
//	Base Score:
//	  Video: views/1000 + likes/100
//	  Text:  reading_time + reactions/50
//
//	Content Type Coefficient:
//	  Video: 1.5
//	  Text:  1.0
//
//	Freshness Score:
//	  within 1 week:   +5
//	  within 1 month:  +3
//	  within 3 months: +1
//	  older:           +0
//
//	Engagement Score:
//	  Video: (likes/views) * 10
//	  Text:  (reactions/reading_time) * 5
//
// NOTE: There is NO "relevance" component in this formula — the BM25
// score Elasticsearch computes for the search query is not used here. The
// score is calculated purely from the content's own metrics, independent
// of which keyword it was searched with. MATCHING against the search
// keyword is a separate step (the Elasticsearch query); this package only
// determines the ORDERING among the matched candidates.
package scoring

import (
	"time"

	"search-engine/internal/domain"
)

const (
	videoTypeCoefficient = 1.5
	textTypeCoefficient  = 1.0
)

// Score calculates the final score for a single piece of content
// according to the formula.
func Score(content domain.Content) domain.ScoredContent {
	base := baseScore(content)
	coefficient := typeCoefficient(content.Type)
	freshness := freshnessScore(content.PublishedAt)
	engagement := engagementScore(content)

	final := base*coefficient + freshness + engagement

	return domain.ScoredContent{
		Content:         content,
		BaseScore:       base,
		FreshnessScore:  freshness,
		EngagementScore: engagement,
		FinalScore:      final,
	}
}

func typeCoefficient(t domain.ContentType) float64 {
	if t == domain.ContentTypeVideo {
		return videoTypeCoefficient
	}
	return textTypeCoefficient
}

func baseScore(c domain.Content) float64 {
	if c.Type == domain.ContentTypeVideo {
		return float64(c.Views)/1000 + float64(c.Likes)/100
	}
	return float64(c.ReadingTimeMinutes) + float64(c.Reactions)/50
}

// engagementScore can have a zero denominator since it is ratio-based (a
// video with no views at all, or a text with no reading_time info) — in
// that case we safely return 0 instead of 0/0.
func engagementScore(c domain.Content) float64 {
	if c.Type == domain.ContentTypeVideo {
		if c.Views == 0 {
			return 0
		}
		return (float64(c.Likes) / float64(c.Views)) * 10
	}
	if c.ReadingTimeMinutes == 0 {
		return 0
	}
	return (float64(c.Reactions) / float64(c.ReadingTimeMinutes)) * 5
}

func freshnessScore(publishedAt time.Time) float64 {
	if publishedAt.IsZero() {
		return 0
	}
	daysSince := time.Since(publishedAt).Hours() / 24
	switch {
	case daysSince <= 7:
		return 5
	case daysSince <= 30:
		return 3
	case daysSince <= 90:
		return 1
	default:
		return 0
	}
}
