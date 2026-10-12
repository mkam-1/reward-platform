package logsink

import (
	"log/slog"
	"os"
)

// NewLogger builds the standard logger every service's main.go starts
// with: JSON to stdout always (CloudWatch/kubectl logs picks this up
// unchanged either way), additionally shipped to Elasticsearch when
// endpoint is non-empty. Centralized here instead of duplicated three
// times across player-service/game-service/rewards-service's main.go —
// the same reasoning as mq and this package itself both existing as
// their own shared modules.
//
// endpoint being empty (ELASTICSEARCH_ENDPOINT unset) is a supported,
// normal configuration, not a degraded one — local development without
// docker-compose's elasticsearch container, or any environment that
// hasn't opted in yet, still gets ordinary stdout logging exactly as
// before this package existed.
func NewLogger(service, endpoint string) *slog.Logger {
	stdout := slog.NewJSONHandler(os.Stdout, nil)
	if endpoint == "" {
		return slog.New(stdout)
	}

	client, err := NewClient(endpoint)
	if err != nil {
		// Can't reach Elasticsearch at startup — log that fact to
		// stdout (which definitely works) and carry on without it,
		// the same "degrade, don't crash" stance the rest of this
		// project takes toward observability (metrics, pprof): losing
		// log search is never worth refusing to serve real traffic.
		logger := slog.New(stdout)
		logger.Error("failed to build elasticsearch client, continuing with stdout logging only", "error", err)
		return logger
	}
	return slog.New(NewHandler(stdout, client, service))
}
