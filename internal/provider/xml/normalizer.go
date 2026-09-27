package xmlprovider

import (
	"encoding/xml"
	"time"

	"search-engine/internal/domain"
)

// feed is the response shape returned by the real XML provider:
//
//	<feed>
//	  <items>
//	    <item>
//	      <id>v1</id>
//	      <headline>Introduction to Docker</headline>
//	      <type>video</type>
//	      <stats>
//	        <views>22000</views>
//	        <likes>1800</likes>
//	        <duration>25:15</duration>
//	      </stats>
//	      <publication_date>2024-03-15</publication_date>
//	      <categories>
//	        <category>devops</category>
//	        <category>containers</category>
//	      </categories>
//	    </item>
//	    <item>
//	      <id>a1</id>
//	      <headline>Clean Architecture in Go</headline>
//	      <type>article</type>
//	      <stats>
//	        <reading_time>8</reading_time>
//	        <reactions>450</reactions>
//	        <comments>25</comments>
//	      </stats>
//	      <publication_date>2024-03-14</publication_date>
//	      <categories><category>programming</category></categories>
//	    </item>
//	  </items>
//	  <meta>...</meta>
//	</feed>
//
// The "stats" content varies by type (video: views/likes/duration,
// article: reading_time/reactions/comments) — Go's encoding/xml silently
// leaves non-matching fields empty, so it's enough and safe to define all
// of them in a single flexible struct.
type feed struct {
	XMLName xml.Name `xml:"feed"`
	Items   items    `xml:"items"`
}

type items struct {
	Item []item `xml:"item"`
}

type item struct {
	ID              string     `xml:"id"`
	Headline        string     `xml:"headline"`
	Type            string     `xml:"type"` // "video" | "article"
	Stats           stats      `xml:"stats"`
	PublicationDate string     `xml:"publication_date"`
	Categories      categories `xml:"categories"`
}

type stats struct {
	Views       *int    `xml:"views"`
	Likes       *int    `xml:"likes"`
	Duration    *string `xml:"duration"`
	ReadingTime *int    `xml:"reading_time"`
	Reactions   *int    `xml:"reactions"`
	Comments    *int    `xml:"comments"`
}

type categories struct {
	Category []string `xml:"category"`
}

type Normalizer struct{}

func (Normalizer) Normalize(providerName string, raw []byte) ([]domain.Content, error) {
	var f feed
	if err := xml.Unmarshal(raw, &f); err != nil {
		return nil, err
	}

	out := make([]domain.Content, 0, len(f.Items.Item))
	for _, it := range f.Items.Item {
		// publication_date is date-only (YYYY-MM-DD) — unlike the full
		// RFC3339 timestamp on the JSON side; we absorb this
		// cross-provider format inconsistency right here, at the
		// normalizer boundary.
		publishedAt, _ := time.Parse("2006-01-02", it.PublicationDate)

		// duration/comments aren't used in the scoring formula, but we
		// keep them in RawFields so we don't lose information the
		// provider sent.
		rawFields := map[string]any{}
		if len(it.Categories.Category) > 0 {
			rawFields["categories"] = it.Categories.Category
		}
		if it.Stats.Duration != nil {
			rawFields["duration"] = *it.Stats.Duration
		}
		if it.Stats.Comments != nil {
			rawFields["comments"] = *it.Stats.Comments
		}

		out = append(out, domain.Content{
			ExternalID:         it.ID,
			Title:              it.Headline,
			Type:               mapContentType(it.Type),
			Views:              intOrZero(it.Stats.Views),
			Likes:              intOrZero(it.Stats.Likes),
			ReadingTimeMinutes: intOrZero(it.Stats.ReadingTime),
			Reactions:          intOrZero(it.Stats.Reactions),
			PublishedAt:        publishedAt,
			RawFields:          rawFields,
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
