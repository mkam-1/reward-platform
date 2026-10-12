package logsink

import (
	"testing"
)

func TestNewLoggerWithoutEndpoint(t *testing.T) {
	logger := NewLogger("test-service", "")
	if logger == nil {
		t.Fatal("expected a non-nil logger")
	}
	// Doesn't panic, logs fine with no Elasticsearch involved at all.
	logger.Info("hello")
}

func TestNewLoggerWithEndpoint(t *testing.T) {
	// elasticsearch.NewClient only validates the address shape, never
	// actually connects — so this exercises NewLogger's "endpoint given"
	// path (wrapping stdout in a Handler) without needing a real
	// Elasticsearch server reachable in the test environment.
	logger := NewLogger("test-service", "http://127.0.0.1:9")
	if logger == nil {
		t.Fatal("expected a non-nil logger")
	}
	logger.Info("hello") // shipped to the (unreachable) client asynchronously; must not block or panic
}
