package logsink

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type fakeIndexer struct {
	mu   sync.Mutex
	docs []map[string]any
}

func (f *fakeIndexer) Index(_ context.Context, _ string, doc any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.docs = append(f.docs, doc.(map[string]any))
	return nil
}

func (f *fakeIndexer) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.docs)
}

func newTestHandler(indexer indexer) (*Handler, *bytes.Buffer) {
	var buf bytes.Buffer
	h := &Handler{next: slog.NewJSONHandler(&buf, nil), service: "test-service", docs: make(chan map[string]any, 10)}
	go h.run(indexer)
	return h, &buf
}

func TestHandlerForwardsToNextHandler(t *testing.T) {
	h, buf := newTestHandler(&fakeIndexer{})
	logger := slog.New(h)

	logger.Info("hello", "key", "value")

	if buf.Len() == 0 {
		t.Fatal("expected the wrapped handler to still receive the record")
	}
}

func TestHandlerShipsToIndexer(t *testing.T) {
	fi := &fakeIndexer{}
	h, _ := newTestHandler(fi)
	logger := slog.New(h)

	logger.Info("hello", "key", "value")

	deadline := time.Now().Add(time.Second)
	for fi.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if fi.count() != 1 {
		t.Fatalf("count = %d, want 1", fi.count())
	}
}

func TestHandlerDropsWhenBufferFull(t *testing.T) {
	// A buffer of 0 means every send hits the "buffer full" default
	// case immediately — proving Handle never blocks waiting for the
	// indexer, which is the entire point of the select/default in
	// Handle.
	h := &Handler{next: slog.NewJSONHandler(&bytes.Buffer{}, nil), service: "test-service", docs: make(chan map[string]any)}
	logger := slog.New(h)

	done := make(chan struct{})
	go func() {
		logger.Info("should not block")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Handle blocked instead of dropping the log document")
	}
}
