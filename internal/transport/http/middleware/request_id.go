// Package middleware contains the cross-cutting behaviors applied to all
// HTTP endpoints: assigning a request ID, structured logging, panic
// recovery. Handlers remain unaware of these details.
package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type contextKey string

const requestIDKey contextKey = "request_id"

// RequestID assigns a UUID to every request and carries it through the
// context. This is the foundation of the "follow an entire log chain with
// a single request_id" capability (searching via Loki) mentioned in the
// architecture doc.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := uuid.NewString()
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// FromContext extracts the request ID from a context — used by handlers
// and lower layers to add it to log lines.
func FromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// Logging writes every request's method, path, duration, and request ID
// as a structured log line.
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", FromContext(r.Context()),
		)
	})
}
