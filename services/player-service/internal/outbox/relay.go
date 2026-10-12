// Package outbox is the relay half of the outbox pattern:
// internal/ledger's CreditPoints writes rows here (via
// internal/postgres's AppendWithOutbox), and Relay is what actually gets
// them to SQS afterward.
package outbox

import (
	"context"
	"log/slog"
	"time"

	"reward-platform/services/player-service/internal/postgres"
)

// Store is the slice of *postgres.LedgerRepository Relay actually needs
// — narrow enough that a test can fake it with no real Postgres
// involved at all, the same reasoning as every other repository
// interface in this project (see ADR 0001). Still named in terms of
// postgres.OutboxRow rather than a from-scratch local type: the outbox
// table is inherently a Postgres-specific concern (see domain.
// OutboxEvent's doc comment on why only a real transactional datastore
// can back this pattern), so depending on that one neutral data-carrier
// type isn't the same coupling as depending on the concrete
// *postgres.LedgerRepository and all its other methods would be.
type Store interface {
	PendingOutboxEvents(ctx context.Context, limit int) ([]postgres.OutboxRow, error)
	MarkOutboxPublished(ctx context.Context, id string, publishedAt time.Time) error
}

// Publisher is the one mq.Client capability Relay needs.
type Publisher interface {
	Publish(ctx context.Context, queueURL, body string) error
}

// Relay polls for unpublished outbox rows and publishes them to SQS.
// Running this as a separate loop, not inline with CreditPoints itself,
// is what lets a transient SQS outage fail to publish without failing
// the player-facing request that triggered it — CreditPoints already
// committed; the event just waits in the table until this loop's next
// tick succeeds.
//
// Known gap at scale: main.go starts one of these per player-service
// *process*, and horizontal scaling means several processes running
// at once, each independently polling the same outbox_events table.
// Two replicas' ticks landing in the same window can both select and
// publish the same still-unpublished row — safe (PointsAwarded
// consumers are idempotent) but wasteful, multiplying SQS publish
// calls roughly by replica count. The real fix is `SELECT ... FOR
// UPDATE SKIP LOCKED` so concurrent relays claim disjoint rows, paired
// with a separate `claimed_at` column (distinct from `published_at`)
// so a replica that claims a row and then crashes before publishing
// doesn't strand it forever — that's a schema migration, not a
// same-session fix, so it's called out here rather than attempted
// half-done.
type Relay struct {
	store     Store
	publisher Publisher
	queueURL  string
	logger    *slog.Logger
}

func NewRelay(store Store, publisher Publisher, queueURL string, logger *slog.Logger) *Relay {
	return &Relay{store: store, publisher: publisher, queueURL: queueURL, logger: logger}
}

// Run polls every interval until ctx is cancelled.
func (r *Relay) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.tick(ctx)
		}
	}
}

func (r *Relay) tick(ctx context.Context) {
	rows, err := r.store.PendingOutboxEvents(ctx, 50)
	if err != nil {
		r.logger.Error("outbox: list pending failed", "error", err)
		return
	}

	for _, row := range rows {
		if err := r.publisher.Publish(ctx, r.queueURL, row.Payload); err != nil {
			r.logger.Error("outbox: publish failed, will retry next tick", "id", row.ID, "error", err)
			continue
		}
		if err := r.store.MarkOutboxPublished(ctx, row.ID, time.Now()); err != nil {
			r.logger.Error("outbox: mark published failed", "id", row.ID, "error", err)
		}
	}
}
