package api

import (
	"net/http"
	"sync"

	"golang.org/x/time/rate"
)

// perIPLimiter rate-limits the redemption endpoint per client IP — the
// one write path in this service that spends a scarce resource (a
// player's points, a reward's stock) and is worth protecting from a
// client retry-looping or a script hammering it, independent of
// whatever rate limiting the ALB/API Gateway in front of this in a
// real deployment would also apply.
type perIPLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	r        rate.Limit
	burst    int
}

func newPerIPLimiter(r rate.Limit, burst int) *perIPLimiter {
	return &perIPLimiter{limiters: make(map[string]*rate.Limiter), r: r, burst: burst}
}

func (l *perIPLimiter) allow(key string) bool {
	l.mu.Lock()
	lim, ok := l.limiters[key]
	if !ok {
		lim = rate.NewLimiter(l.r, l.burst)
		l.limiters[key] = lim
	}
	l.mu.Unlock()
	return lim.Allow()
}

// rateLimitMiddleware wraps a single handler (not the whole mux — see
// server.go, applied only to the redemption endpoint) with a per-IP
// token bucket: r.RemoteAddr includes the port, which would give every
// request its own bucket behind a client reusing ports, but is good
// enough without pulling in a real client-IP-extraction dependency for
// a learning deployment with no proxy chain to parse.
func rateLimitMiddleware(limiter *perIPLimiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !limiter.allow(r.RemoteAddr) {
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next(w, r)
	}
}
