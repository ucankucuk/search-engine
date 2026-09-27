// Package middleware — this file contains a simple IP-based rate
// limiter. DELIBERATE CHOICE: instead of a ready-made library like
// golang.org/x/time/rate, a hand-written, dependency-free token bucket
// was written — both to avoid needing an extra module at this case
// study's scale, and to be able to answer the interview question "how
// does rate limiting work" through the code itself, without hiding behind
// a library. In production, x/time/rate (better tested, edge cases
// already thought through) would be preferred.
package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// bucket holds the token-bucket state for a SINGLE IP.
//   - tokens: the number of "request credits" currently available (can be
//     fractional — for a continuous, non-discrete refill rate).
//   - lastRefill: when tokens was last updated — on the next request, NEW
//     tokens are computed based on the elapsed time (there is NO separate
//     background "refill" goroutine — it is entirely request-triggered,
//     consuming zero CPU while idle).
type bucket struct {
	tokens     float64
	lastRefill time.Time
}

// IPRateLimiter is a mutex-protected map that tracks each IP with its own
// bucket. rps (the sustainable requests-per-second rate) and burst (the
// number of requests that can be accepted immediately all at once,
// exceeding that average) are shared across all IPs.
type IPRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rps     float64
	burst   float64

	// lastSweep/sweepEvery: to keep the map from growing unbounded, IPs
	// that haven't made a request in a while are removed from the map at
	// regular intervals (5 minutes by default). INSTEAD OF a separate
	// goroutine/ticker, this cleanup "piggybacks" on normal Allow()
	// calls — no need to manage (and, on graceful shutdown, also stop) an
	// extra background loop.
	lastSweep  time.Time
	sweepEvery time.Duration
	staleAfter time.Duration
}

// NewIPRateLimiter sets up a new limiter with the given rps and burst
// values.
func NewIPRateLimiter(rps float64, burst int) *IPRateLimiter {
	return &IPRateLimiter{
		buckets:    make(map[string]*bucket),
		rps:        rps,
		burst:      float64(burst),
		lastSweep:  time.Now(),
		sweepEvery: 5 * time.Minute,
		staleAfter: 10 * time.Minute,
	}
}

// Allow tries to spend one request credit for the given IP; returns true
// and deducts a token if enough tokens are available, otherwise returns
// false (the request should be rejected).
func (l *IPRateLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	l.sweepLocked(now)

	b, ok := l.buckets[ip]
	if !ok {
		// New IP: starts with a "welcome" allotment equal to burst — so
		// its first request isn't rejected immediately (the bucket is
		// assumed to already be full).
		b = &bucket{tokens: l.burst, lastRefill: now}
		l.buckets[ip] = b
	} else {
		elapsed := now.Sub(b.lastRefill).Seconds()
		b.tokens += elapsed * l.rps
		if b.tokens > l.burst {
			b.tokens = l.burst
		}
		b.lastRefill = now
	}

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// sweepLocked must be called while l.mu is ALREADY locked. It deletes the
// buckets of IPs that haven't made a request for staleAfter.
func (l *IPRateLimiter) sweepLocked(now time.Time) {
	if now.Sub(l.lastSweep) < l.sweepEvery {
		return
	}
	for ip, b := range l.buckets {
		if now.Sub(b.lastRefill) > l.staleAfter {
			delete(l.buckets, ip)
		}
	}
	l.lastSweep = now
}

// clientIP extracts the IP the request came from. Since we don't run
// BEHIND a reverse proxy here, we DELIBERATELY don't read
// headers like X-Forwarded-For — without a proxy, this header can be
// freely spoofed by the client, which would make the rate limit
// meaningless. In production, if there's a reverse proxy/load balancer,
// a trusted header ADDED (with the previous one stripped) by that proxy
// would be read instead.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// RateLimit produces a middleware that filters requests according to the
// given limiter. It returns 429 Too Many Requests when the limit is
// exceeded — not 500/503, because there's no error on the server side,
// the client is just sending requests too fast.
func RateLimit(limiter *IPRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow(clientIP(r)) {
				w.Header().Set("Retry-After", "1")
				http.Error(w, "too many requests, please slow down", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
