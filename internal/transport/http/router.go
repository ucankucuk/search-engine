// Package http sets up the HTTP router: it defines which path reaches
// which handler, through which middleware chain. This package is the
// "wiring" layer — it contains no business logic, it only connects
// things.
package http

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"search-engine/internal/transport/http/handler"
	"search-engine/internal/transport/http/middleware"
)

type Handlers struct {
	Search  *handler.SearchHandler
	Refresh *handler.RefreshHandler
}

// NewRouter sets up the router. allowedOrigin is the origin the
// dashboard (frontend) runs on (e.g. http://localhost:5173) — the CORS
// middleware only allows this origin. rateLimitRPS/rateLimitBurst are the
// parameters for middleware.RateLimit.
func NewRouter(h Handlers, allowedOrigin string, rateLimitRPS float64, rateLimitBurst int) http.Handler {
	mux := http.NewServeMux()

	// Rate limiting is applied ONLY to user/client-triggered endpoints
	// (/search, /refresh) — it is DELIBERATELY not applied to /metrics:
	// we shouldn't cause Prometheus's regular scrape requests to get
	// caught by the rate limit and see "no data". The two endpoints
	// SHARE the same limiter (the same IP-based bucket pool) — /refresh
	// is also actually an expensive operation (it triggers all
	// providers), it needs the same protection.
	limiter := middleware.NewIPRateLimiter(rateLimitRPS, rateLimitBurst)
	rateLimited := middleware.RateLimit(limiter)

	mux.Handle("/search", rateLimited(h.Search))
	mux.Handle("/refresh", rateLimited(h.Refresh))
	mux.Handle("/metrics", promhttp.Handler())

	var handler http.Handler = mux
	handler = middleware.Logging(handler)
	handler = middleware.RequestID(handler)
	handler = middleware.CORS(allowedOrigin)(handler)
	return handler
}
