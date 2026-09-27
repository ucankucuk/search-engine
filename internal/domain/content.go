// Package domain contains the application's core data models and errors,
// which have no dependency on any outer layer (HTTP, Mongo, Elasticsearch,
// provider formats). Clean architecture's "dependencies flow inward" rule
// takes concrete form here: the provider, storage, search, and transport
// packages can ALL import this package, but this package imports none of
// them.
package domain

import "time"

// ContentType expresses the standardized type of the content.
type ContentType string

const (
	ContentTypeVideo ContentType = "video"
	ContentTypeText  ContentType = "text"
)

// Content is the common shape that data from every provider is converted
// into. The storage and scoring layers know ONLY this type — which
// provider it came from, whether it was JSON or XML, is irrelevant to
// them. Provider-specific fields that don't fit the common schema are
// preserved without loss inside RawFields.
// Content.Views/Likes/ReadingTimeMinutes/Reactions: the scoring formula
// uses different raw metrics depending on the content type (video:
// views+likes, text: reading_time+reactions) — we reduce the inconsistent
// schemas across providers (e.g. "metrics.views" in JSON, "<stats><views>"
// in XML) down to THESE fields at the normalizer boundary. A field that is
// irrelevant to a given content type simply stays 0 (e.g.
// ReadingTimeMinutes is always 0 for a video).
type Content struct {
	ExternalID         string         `json:"external_id" bson:"external_id"`
	Provider           string         `json:"provider" bson:"provider"`
	Title              string         `json:"title" bson:"title"`
	Type               ContentType    `json:"type" bson:"type"`
	Views              int            `json:"views" bson:"views"`
	Likes              int            `json:"likes" bson:"likes"`
	ReadingTimeMinutes int            `json:"reading_time_minutes" bson:"reading_time_minutes"`
	Reactions          int            `json:"reactions" bson:"reactions"`
	PublishedAt        time.Time      `json:"published_at" bson:"published_at"`
	FetchedAt          time.Time      `json:"fetched_at" bson:"fetched_at"`
	RawFields          map[string]any `json:"raw_fields,omitempty" bson:"raw_fields,omitempty"`

	// Indexed/IndexedAt: a lightweight state flag kept to solve the "dual
	// write" consistency problem between Mongo and Elasticsearch (see
	// ingestion.Job.runOne). In this system Mongo is the "source of
	// truth", while ES is a DERIVED, rebuildable search view of it — but
	// if the write to Mongo succeeds while indexing into ES fails, a
	// window of inconsistency opens between these two writes (since they
	// are not in the same transaction): the data exists in Mongo but
	// cannot be searched.
	//
	// The "correct" solution for this is an outbox pattern (writing an
	// "outbox" event in the same transaction as the content write, with a
	// separate worker consuming it and writing to ES — at large scale
	// this is set up by having Debezium listen to Mongo's oplog and
	// publish it as a CDC event to Kafka, with ES also being a consumer
	// of that topic). At this project's scale (a single consumer,
	// a single event type, a single-node Mongo — transactions require a
	// replica set), setting up a full outbox+Kafka would be
	// over-engineering; instead we apply a simplified version of the SAME
	// IDEA: the content document itself takes on the role of both the
	// data and the "pending event". Indexed=false means "this document is
	// waiting to be reflected in ES"; at the end of every ingestion
	// round, Reconcile() (which runs at the end of RunAll) finds all
	// indexed=false records and tries to re-index them. With json:"-" it
	// never leaks into the API response at all — this is a purely
	// internal bookkeeping field, not part of the search result schema.
	Indexed   bool       `json:"-" bson:"indexed"`
	IndexedAt *time.Time `json:"-" bson:"indexed_at,omitempty"`
}

// ScoredContent is the form Content takes when returned as a search
// result, carrying the fields our own scoring formula adds on top. The
// field names deliberately match the formula's terms one-to-one:
//
//	FinalScore = BaseScore*TypeCoefficient + FreshnessScore + EngagementScore
type ScoredContent struct {
	Content
	BaseScore       float64 `json:"base_score"`
	FreshnessScore  float64 `json:"freshness_score"`
	EngagementScore float64 `json:"engagement_score"`
	FinalScore      float64 `json:"final_score"`
}

// SearchQuery is the standardized parameter set for a search request. The
// HTTP handler converts an incoming request into this type, so the search
// package has no knowledge of HTTP's existence.
type SearchQuery struct {
	Keyword  string
	Type     *ContentType
	Page     int
	PageSize int
}

// SearchResult carries the paginated result slice together with the total
// match count and page info. Without these, the frontend couldn't answer
// "how many results are there in total" or "how many pages are there" —
// it would have to guess "am I on the last page" just by looking at the
// size of the returned page (this was exactly the old behavior).
//
// NOTE: Total is NOT Elasticsearch's raw match count, it is the number of
// candidates search.Service actually fetched and scored (see
// elastic.candidateWindowSize — the candidate pool is capped at 500).
// This ensures the "total results" and "number of navigable pages" are
// always consistent; the user is never directed to a page that doesn't
// exist.
type SearchResult struct {
	Items      []ScoredContent `json:"results"`
	Total      int             `json:"total"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
	TotalPages int             `json:"total_pages"`
}
