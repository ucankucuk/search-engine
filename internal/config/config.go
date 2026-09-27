// Package config reads application settings from environment variables and
// converts them into a type-safe struct. No other package calls os.Getenv
// directly — this both improves testability and collects the answer to
// "which env variables exist" into a single file.
package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPPort       string
	MongoURI       string
	MongoDatabase  string
	ElasticURL     string
	ProviderAURL   string
	ProviderBURL   string
	ProviderXMLURL string
	FetchTimeout   time.Duration
	IngestInterval time.Duration
	// FrontendOrigin is the single origin the CORS middleware returns as
	// Access-Control-Allow-Origin — the address the dashboard (frontend)
	// runs at in the browser. Same rationale as the
	// http.cors.allow-origin setting in Elasticsearch: a real origin
	// instead of a wildcard.
	FrontendOrigin string
	// RateLimitRPS/RateLimitBurst: per-IP token-bucket rate limit settings
	// (see middleware.RateLimit). RPS is "how many requests per second can
	// be sustained" (long-term average), while Burst is "how many
	// requests in a row, exceeding that average all at once, are accepted
	// immediately" — this lets a normal user fire off a few consecutive
	// requests while switching pages/searching quickly, but keeps a
	// script from firing hundreds of requests per second.
	RateLimitRPS   float64
	RateLimitBurst int
}

func Load() Config {
	return Config{
		HTTPPort:       getEnv("HTTP_PORT", "8080"),
		MongoURI:       getEnv("MONGO_URI", "mongodb://localhost:27017"),
		MongoDatabase:  getEnv("MONGO_DATABASE", "search_engine"),
		ElasticURL:     getEnv("ELASTIC_URL", "http://localhost:9200"),
		ProviderAURL:   getEnv("PROVIDER_A_URL", ""),
		ProviderBURL:   getEnv("PROVIDER_B_URL", ""),
		ProviderXMLURL: getEnv("PROVIDER_XML_URL", ""),
		FetchTimeout:   5 * time.Second,
		// IngestInterval: 10 minutes is a reasonable default in
		// production (providers don't change often). But for circuit
		// breaker/resilience demos we need to be able to bring this down
		// to the order of seconds via env — otherwise you'd have to wait
		// 10 minutes to see the breaker open and close, or manually spam
		// /refresh.
		IngestInterval: getEnvDuration("INGEST_INTERVAL_SECONDS", 10*time.Minute),
		FrontendOrigin: getEnv("FRONTEND_ORIGIN", "http://localhost:5173"),
		RateLimitRPS:   getEnvFloat("RATE_LIMIT_RPS", 5),
		RateLimitBurst: getEnvInt("RATE_LIMIT_BURST", 10),
	}
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func getEnvFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f <= 0 {
		return fallback
	}
	return f
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// getEnvDuration reads the env variable as an integer number of SECONDS
// and converts it to a time.Duration. Seconds were chosen because, in the
// demo scenario, values written by hand (5, 10, 30) are more natural than
// minutes — "INGEST_INTERVAL_SECONDS=5" is easier to read than "10m0s".
func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	seconds, err := strconv.Atoi(v)
	if err != nil || seconds <= 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}
