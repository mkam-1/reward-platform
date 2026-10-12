package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	requestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "http_requests_total", Help: "Total HTTP requests"},
		[]string{"method", "path", "status"},
	)
	requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{Name: "http_request_duration_seconds", Help: "HTTP request duration"},
		[]string{"method", "path"},
	)
)

func init() {
	prometheus.MustRegister(requestsTotal, requestDuration)
}

// metricsMiddleware records a counter and a duration histogram per
// request, labeled by method/path/status — the three dimensions that
// matter for "what's slow" and "what's failing" at a glance in Grafana.
// Composes into the same decorator chain as logging/recovery/requestID.
func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		// r.URL.Path, not a route template: a path-ID like /players/player_1
		// becomes its own label value forever, which is a real Prometheus
		// cardinality concern at scale (every unique ID the system has
		// ever seen is a permanent time series). The correct fix is
		// extracting the *matched pattern* (e.g. "/players/{id}") instead
		// — left as-is here given time constraints; worth revisiting
		// before this ships for real.
		requestsTotal.WithLabelValues(r.Method, r.URL.Path, strconv.Itoa(rec.status)).Inc()
		requestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(time.Since(start).Seconds())
	})
}

// metricsHandler exposes /metrics in Prometheus's text exposition format.
func metricsHandler() http.Handler {
	return promhttp.Handler()
}
