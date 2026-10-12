// Package outbox is the relay half of the outbox pattern:
// internal/ledger's CreditPoints writes rows here (via
// internal/postgres's AppendWithOutbox), and Relay is what actually gets
// them to SQS afterward.
package outbox

import (
	"context"
	"log/slog"
	"time"

	"reward-platform/mq"
	"reward-platform/services/player-service/internal/postgres"
)

// Relay polls for unpublished outbox rows and publishes them to SQS.
// Running this as a separate loop, not inline with CreditPoints itself,
// is what lets a transient SQS outage fail to publish without failing
// the player-facing request that triggered it — CreditPoints already
// committed; the event just waits in the table until this loop's next
// tick succeeds.
type Relay struct {
	store    *postgres.LedgerRepository
	mq       *mq.Client
	queueURL string
	logger   *slog.Logger
}

func NewRelay(store *postgres.LedgerRepository, client *mq.Client, queueURL string, logger *slog.Logger) *Relay {
	return &Relay{store: store, mq: client, queueURL: queueURL, logger: logger}
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
		if err := r.mq.Publish(ctx, r.queueURL, row.Payload); err != nil {
			r.logger.Error("outbox: publish failed, will retry next tick", "id", row.ID, "error", err)
			continue
		}
		if err := r.store.MarkOutboxPublished(ctx, row.ID, time.Now()); err != nil {
			r.logger.Error("outbox: mark published failed", "id", row.ID, "error", err)
		}
	}
}
