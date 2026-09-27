// mockjsonprovider is the JSON provider "we designed/run ourselves" (our
// own third provider, alongside the two real ones). Its schema deliberately
// speaks the exact same ENVELOPE as the real JSON provider
// (contents/metrics/tags) — so the single Normalizer in
// internal/provider/json can parse both (the real one + our own mock) —
// but in terms of content it blends in the real XML provider's concepts
// too (categories, reading_time/reactions/comments for the article type),
// producing a dataset that's a "mix" of the two real providers.
// Additionally, unlike the real providers' sample files, which contain a
// meager set of 4 records, we generate a MUCH larger dataset by default
// in order to test the end-to-end pipeline (ingestion → mongo → search →
// ranking) at a realistic scale.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"
)

// content is a single record written out. The field names deliberately
// match the real JSON provider's (id/title/type/metrics/published_at/tags)
// exactly; "categories" is an extra field borrowed from the XML provider —
// the real provider doesn't send it but ours does, and the normalizer
// already supports it as optional.
type content struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Type        string   `json:"type"` // "video" | "article"
	Metrics     metrics  `json:"metrics"`
	PublishedAt string   `json:"published_at"`
	Tags        []string `json:"tags"`
	Categories  []string `json:"categories,omitempty"`
}

// metrics: a different subset is populated for video vs. article — just
// like <stats> in the real XML provider. With omitempty, whichever one
// isn't used doesn't appear in the payload at all, mimicking the real
// provider's behavior.
type metrics struct {
	Views    int    `json:"views,omitempty"`
	Likes    int    `json:"likes,omitempty"`
	Duration string `json:"duration,omitempty"`

	ReadingTime int `json:"reading_time,omitempty"`
	Reactions   int `json:"reactions,omitempty"`
	Comments    int `json:"comments,omitempty"`
}

type pagination struct {
	Total   int `json:"total"`
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
}

type payload struct {
	Contents   []content  `json:"contents"`
	Pagination pagination `json:"pagination"`
}

var (
	topics = []string{
		"Go", "Kubernetes", "Docker", "Mikroservisler", "Elasticsearch", "MongoDB",
		"Prometheus", "Grafana", "Clean Architecture", "Circuit Breaker", "gRPC",
		"REST API Tasarımı", "Event Driven Mimari", "CQRS", "Kafka", "Redis",
		"CI/CD Pipeline", "Terraform", "AWS Lambda", "PostgreSQL", "GraphQL",
		"Observability", "Service Mesh", "Load Balancing", "Rate Limiting",
		"Distributed Tracing", "Chaos Engineering", "Domain Driven Design",
		"Test Otomasyonu", "Sistem Tasarımı", "Ölçeklenebilirlik",
	}
	videoFormats = []string{
		"%s Temelleri: Başlangıç Rehberi",
		"%s ile Production'a Hazırlık",
		"%s Üzerine Derinlemesine İnceleme",
		"Sıfırdan %s Öğreniyoruz",
		"%s Mülakat Soruları ve Cevapları",
		"Gerçek Dünyada %s Kullanımı",
		"İleri Seviye %s Teknikleri",
		"%s ile Ölçeklenebilir Sistemler",
	}
	articleFormats = []string{
		"%s Neden Önemli? 2025 Perspektifi",
		"%s Best Practice'leri",
		"%s: Yaygın Hatalar ve Çözümleri",
		"%s Performans Optimizasyonu Üzerine Notlar",
		"%s Konusunda Bilmeniz Gereken Her Şey",
		"%s: Bir Vaka Analizi",
	}
	categoryPool = []string{
		"programming", "devops", "architecture", "cloud", "databases",
		"observability", "testing", "backend", "distributed-systems",
	}
	tagPool = []string{
		"tutorial", "advanced", "best-practices", "case-study", "guide",
		"performance", "production", "beginner", "deep-dive",
	}
)

func randomSubset(r *rand.Rand, pool []string, n int) []string {
	if n > len(pool) {
		n = len(pool)
	}
	idx := r.Perm(len(pool))[:n]
	out := make([]string, 0, n)
	for _, i := range idx {
		out = append(out, pool[i])
	}
	return out
}

func generateContents(seed int64, count int) []content {
	r := rand.New(rand.NewSource(seed))
	items := make([]content, 0, count)
	now := time.Now()

	for i := 0; i < count; i++ {
		topic := topics[r.Intn(len(topics))]

		isVideo := r.Float64() < 0.45
		daysAgo := r.Intn(900)
		publishedAt := now.AddDate(0, 0, -daysAgo).Format(time.RFC3339)
		categories := randomSubset(r, categoryPool, 1+r.Intn(2))
		tags := randomSubset(r, tagPool, 1+r.Intn(3))

		c := content{
			ID:          fmt.Sprintf("own-%06d", i+1),
			PublishedAt: publishedAt,
			Categories:  categories,
			Tags:        tags,
		}

		if isVideo {
			c.Type = "video"
			c.Title = fmt.Sprintf(videoFormats[r.Intn(len(videoFormats))], topic)
			// Exponential/log-skewed distribution to make the popularity
			// distribution realistic: most videos get few views, a few
			// go viral.
			views := int(r.ExpFloat64() * 4000)
			if views > 2_000_000 {
				views = 2_000_000
			}
			c.Metrics = metrics{
				Views:    views,
				Likes:    views / (5 + r.Intn(15)), // likes, roughly 5-20% of views
				Duration: fmt.Sprintf("%02d:%02d", r.Intn(60), r.Intn(60)),
			}
		} else {
			c.Type = "article"
			c.Title = fmt.Sprintf(articleFormats[r.Intn(len(articleFormats))], topic)
			reactions := int(r.ExpFloat64() * 300)
			if reactions > 50_000 {
				reactions = 50_000
			}
			c.Metrics = metrics{
				ReadingTime: 2 + r.Intn(20),
				Reactions:   reactions,
				Comments:    reactions / (3 + r.Intn(10)),
			}
		}

		items = append(items, c)
	}
	return items
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "4001"
	}
	// The real providers' sample files contained only a handful of
	// records (for demo purposes) — we keep the default much larger so
	// we can test the end-to-end pipeline at a realistic volume.
	datasetSize := envInt("DATASET_SIZE", 20000)
	seed := int64(envInt("SEED", 42))

	// FAILURE_RATE and LATENCY_MS_MAX were added deliberately: if in the
	// future you want to give a live demo of the retry/circuit
	// breaker/fallback mechanisms in the backend, you can turn this
	// service into a "misbehaving" provider via env variables — no code
	// change needed.
	failureRate := envFloat("FAILURE_RATE", 0)
	latencyMsMax := envInt("LATENCY_MS_MAX", 0)

	contents := generateContents(seed, datasetSize)
	body, err := json.Marshal(payload{
		Contents: contents,
		Pagination: pagination{
			Total:   len(contents),
			Page:    1,
			PerPage: len(contents),
		},
	})
	if err != nil {
		log.Fatalf("failed to serialize dataset: %v", err)
	}
	log.Printf("mock json-provider-own ready: %d records, %d bytes", len(contents), len(body))

	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("/json-a", func(w http.ResponseWriter, r *http.Request) {
		if latencyMsMax > 0 {
			time.Sleep(time.Duration(rand.Intn(latencyMsMax)) * time.Millisecond)
		}
		if failureRate > 0 && rand.Float64() < failureRate {
			http.Error(w, "simulated upstream error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})

	log.Printf("mock json-provider-own listening on :%s (dataset=%d, failure_rate=%.2f, latency_max=%dms)",
		port, datasetSize, failureRate, latencyMsMax)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
