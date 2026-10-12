package outbox_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"reward-platform/services/player-service/internal/outbox"
	"reward-platform/services/player-service/internal/postgres"
)

var ctx = context.Background()

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

type fakeStore struct {
	mu        sync.Mutex
	pending   []postgres.OutboxRow
	published []string
	listErr   error
}

func (f *fakeStore) PendingOutboxEvents(_ context.Context, limit int) ([]postgres.OutboxRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	if limit < len(f.pending) {
		return f.pending[:limit], nil
	}
	return f.pending, nil
}

func (f *fakeStore) MarkOutboxPublished(_ context.Context, id string, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.published = append(f.published, id)
	// Simulate the row leaving the "pending" set once published, same as
	// the real SQL UPDATE ... WHERE published_at IS NULL would.
	for i, row := range f.pending {
		if row.ID == id {
			f.pending = append(f.pending[:i], f.pending[i+1:]...)
			break
		}
	}
	return nil
}

type fakePublisher struct {
	mu        sync.Mutex
	published []string
	failFor   map[string]bool
}

func (f *fakePublisher) Publish(_ context.Context, _ string, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failFor[body] {
		return errors.New("simulated publish failure")
	}
	f.published = append(f.published, body)
	return nil
}

func TestRelayPublishesPendingRows(t *testing.T) {
	store := &fakeStore{pending: []postgres.OutboxRow{
		{ID: "entry_1", Payload: `{"playerId":"player_1","points":15}`},
		{ID: "entry_2", Payload: `{"playerId":"player_2","points":20}`},
	}}
	pub := &fakePublisher{}
	relay := outbox.NewRelay(store, pub, "queue-url", testLogger())

	// tick is unexported, so drive it the same way Run does: directly
	// call Run with a context cancelled after one interval, rather than
	// reaching into the type.
	runCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	relay.Run(runCtx, 10*time.Millisecond)

	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.published) != 2 {
		t.Fatalf("published %d messages, want 2 (got %v)", len(pub.published), pub.published)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.published) != 2 {
		t.Fatalf("marked %d rows published, want 2", len(store.published))
	}
	if len(store.pending) != 0 {
		t.Fatalf("pending rows remaining = %d, want 0", len(store.pending))
	}
}

func TestRelayLeavesRowPendingOnPublishFailure(t *testing.T) {
	store := &fakeStore{pending: []postgres.OutboxRow{
		{ID: "entry_1", Payload: "will-fail"},
	}}
	pub := &fakePublisher{failFor: map[string]bool{"will-fail": true}}
	relay := outbox.NewRelay(store, pub, "queue-url", testLogger())

	runCtx, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()
	relay.Run(runCtx, 10*time.Millisecond)

	store.mu.Lock()
	defer store.mu.Unlock()
	// A failed publish must not be marked published — the whole point of
	// the outbox pattern is that it's retried on a later tick instead of
	// being silently dropped.
	if len(store.published) != 0 {
		t.Fatalf("marked %d rows published despite publish failing, want 0", len(store.published))
	}
	if len(store.pending) != 1 {
		t.Fatalf("pending rows = %d, want 1 (still there for the next tick)", len(store.pending))
	}
}
