package api

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// perIPLimiter rate-limits the redemption endpoint per client IP — the
// one write path in this service that spends a scarce resource (a
// player's points, a reward's stock) and is worth protecting from a
// client retry-looping or a script hammering it, independent of
// whatever rate limiting the ALB/API Gateway in front of this in a
// real deployment would also apply.
//
// Two bugs fixed here that the first version of this had: (1) keying
// on r.RemoteAddr directly includes the ephemeral source port, which
// is different on almost every request — behind a real load balancer
// every request's RemoteAddr is the LB's own internal address anyway,
// so this used to create a fresh, never-reused limiter per request:
// the rate limit did nothing, and the map grew forever. clientIP below
// fixes the key. (2) even with a correct key, a limiter is never
// removed once created, so distinct IPs over the service's lifetime
// still leak memory without bound — sweep() below evicts limiters idle
// past a TTL.
type perIPLimiter struct {
	mu       sync.Mutex
	limiters map[string]*limiterEntry
	r        rate.Limit
	burst    int
}

type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func newPerIPLimiter(r rate.Limit, burst int) *perIPLimiter {
	l := &perIPLimiter{limiters: make(map[string]*limiterEntry), r: r, burst: burst}
	return l
}

// sweep runs for the life of the process (call it in its own goroutine,
// as server.go does), evicting any limiter that hasn't been touched in
// idleTTL — otherwise a service that's been up for weeks accumulates
// one entry per distinct client IP it's ever seen, forever.
func (l *perIPLimiter) sweep(ctx context.Context, interval, idleTTL time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			l.mu.Lock()
			for key, entry := range l.limiters {
				if now.Sub(entry.lastSeen) > idleTTL {
					delete(l.limiters, key)
				}
			}
			l.mu.Unlock()
		}
	}
}

func (l *perIPLimiter) allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	entry, ok := l.limiters[key]
	if !ok {
		entry = &limiterEntry{limiter: rate.NewLimiter(l.r, l.burst)}
		l.limiters[key] = entry
	}
	entry.lastSeen = now
	l.mu.Unlock()
	return entry.limiter.Allow()
}

// clientIP extracts the real client address to rate-limit on: the
// first hop in X-Forwarded-For when present (what an ALB/ingress sets
// — without this, every request behind one shares the load balancer's
// own address instead of the actual client's), falling back to
// RemoteAddr with its port stripped (SplitHostPort) so one client
// reusing many ephemeral source ports still hits a single bucket.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if first := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0]); first != "" {
			return first
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// rateLimitMiddleware wraps a single handler (not the whole mux — see
// server.go, applied only to the redemption endpoint) with a per-IP
// token bucket.
func rateLimitMiddleware(limiter *perIPLimiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !limiter.allow(clientIP(r)) {
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next(w, r)
	}
}
