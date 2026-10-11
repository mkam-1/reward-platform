package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"
)

type contextKey int

const requestIDKey contextKey = iota

// newRequestID returns a short random hex string. Good enough to
// correlate the lines one request produces in the logs — not meant to be
// globally unique the way a UUID is.
func newRequestID() string {
	var b [8]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read on a small buffer essentially never fails
	return hex.EncodeToString(b[:])
}

// requestIDMiddleware stamps every request with an ID, before anything
// else runs, so every later middleware and handler can read it back out
// of the request's context.
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := newRequestID()
		w.Header().Set("X-Request-Id", id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// statusRecorder wraps a ResponseWriter purely to remember which status
// code the handler sent — http.ResponseWriter has no Status() getter, so
// logging middleware that runs *after* the handler has no other way to
// find out what it did.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// loggingMiddleware logs one structured line per request: method, path,
// resulting status, how long it took, and the request ID that ties it to
// every other log line from the same request.
func loggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(rec, r)

			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", requestIDFromContext(r.Context()),
			)
		})
	}
}

// recoveryMiddleware turns a panic anywhere in a handler into a 500
// response and a logged error, instead of crashing the whole process —
// one misbehaving request should never take down every other in-flight
// request alongside it.
func recoveryMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic recovered",
						"error", rec,
						"path", r.URL.Path,
						"request_id", requestIDFromContext(r.Context()),
					)
					writeError(w, http.StatusInternalServerError, "internal server error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// withMiddleware composes the chain around the router. Order matters:
// requestIDMiddleware runs first so the ID exists for everything after
// it; loggingMiddleware wraps recoveryMiddleware so it measures and logs
// the *final* outcome, including requests that panicked and got turned
// into a 500 by the layer underneath it.
func withMiddleware(h http.Handler, logger *slog.Logger) http.Handler {
	h = recoveryMiddleware(logger)(h)
	h = loggingMiddleware(logger)(h)
	h = requestIDMiddleware(h)
	return h
}
