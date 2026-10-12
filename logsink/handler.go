package logsink

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// indexer is the one Client capability Handler needs — narrow enough
// that a test can fake it with no real Elasticsearch involved, the
// same reasoning as every repository interface elsewhere in this
// project (see ADR-0001).
type indexer interface {
	Index(ctx context.Context, index string, doc any) error
}

// Handler wraps another slog.Handler (normally slog.NewJSONHandler on
// stdout, which CloudWatch already picks up) and additionally ships
// every record to Elasticsearch for search — "and" the existing
// logging, not "instead of" it. Shipping happens on a bounded
// background channel: a slow or unreachable Elasticsearch cluster
// must never add latency to the request that triggered the log line,
// and a full buffer drops the newest log documents rather than
// blocking the caller — logging is observability, not a guarantee, so
// losing some log lines under sustained backpressure is an acceptable
// trade against the alternative (logging itself becoming the
// bottleneck).
type Handler struct {
	next    slog.Handler
	service string
	docs    chan map[string]any
}

// NewHandler returns a Handler that ships to indexFunc's target index
// pattern "logs-<service>-YYYY.MM.DD" (one index per service per day —
// the standard ELK rollover convention, keeping any one index bounded
// and old ones trivially deletable by age). bufferSize bounds how many
// log records can be queued for shipping before new ones start being
// dropped; 1000 is generous for a single instance's log volume without
// risking unbounded memory growth if Elasticsearch is down for a while.
func NewHandler(next slog.Handler, client *Client, service string) *Handler {
	h := &Handler{next: next, service: service, docs: make(chan map[string]any, 1000)}
	go h.run(client)
	return h
}

func (h *Handler) run(client indexer) {
	for doc := range h.docs {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		index := fmt.Sprintf("logs-%s-%s", h.service, time.Now().UTC().Format("2006.01.02"))
		_ = client.Index(ctx, index, doc) // best-effort: a dropped log document is never worth crashing or retrying for
		cancel()
	}
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *Handler) Handle(ctx context.Context, record slog.Record) error {
	doc := map[string]any{
		"@timestamp": record.Time.UTC().Format(time.RFC3339Nano),
		"level":      record.Level.String(),
		"message":    record.Message,
		"service":    h.service,
	}
	// Only this call's own attrs, not ones attached earlier via
	// logger.With(...)/WithAttrs — those live on the wrapped h.next
	// handler for stdout's sake and aren't visible here without
	// reimplementing slog's own attr-tracking. Acceptable: every
	// logger.With(...) call site in this project also passes its
	// identifying fields (request_id, etc.) as ordinary Handle-time
	// attrs via logger.Info("msg", "key", val, ...), which this does
	// capture — see middleware.go's loggingMiddleware for the pattern.
	record.Attrs(func(a slog.Attr) bool {
		doc[a.Key] = a.Value.Any()
		return true
	})

	select {
	case h.docs <- doc:
	default:
		// Buffer full — drop rather than block. See Handler's doc
		// comment: this request's own latency matters more than this
		// one log line reaching Elasticsearch.
	}

	return h.next.Handle(ctx, record)
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{next: h.next.WithAttrs(attrs), service: h.service, docs: h.docs}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{next: h.next.WithGroup(name), service: h.service, docs: h.docs}
}
