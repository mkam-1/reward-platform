package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"reward-platform/pkg/domain"
)

// LedgerRepository is the Postgres-backed domain.LedgerRepository. The
// in-memory version (memory_ledger_repo.go) computes Balance by scanning
// every entry in a Go loop; this version instead pushes that work into
// the database with SUM() — the same answer, computed where the data
// already lives, using the idx_ledger_entries_player_id index from the
// migration instead of shipping every row over the wire first.
type LedgerRepository struct {
	db *sql.DB
}

func NewLedgerRepository(db *sql.DB) *LedgerRepository {
	return &LedgerRepository{db: db}
}

// Append is ON CONFLICT (id) DO NOTHING: the id column is the table's
// primary key, so a second Append with the same entry ID is a no-op
// instead of a duplicate row or an error — that's the idempotent-write
// contract LedgerRepository promises.
func (r *LedgerRepository) Append(ctx context.Context, entry domain.LedgerEntry) (bool, error) {
	const query = `
		INSERT INTO ledger_entries (id, player_id, delta, reason, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO NOTHING`

	result, err := r.db.ExecContext(ctx, query, entry.ID, entry.PlayerID, entry.Delta, entry.Reason, entry.CreatedAt)
	if err != nil {
		return false, fmt.Errorf("append ledger entry: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("append ledger entry: rows affected: %w", err)
	}
	return rows == 1, nil
}

func (r *LedgerRepository) Balance(ctx context.Context, playerID string) (int, error) {
	var balance int
	if err := r.db.QueryRowContext(ctx, balanceQuery, playerID).Scan(&balance); err != nil {
		return 0, fmt.Errorf("ledger balance: %w", err)
	}
	return balance, nil
}

const balanceQuery = `SELECT COALESCE(SUM(delta), 0) FROM ledger_entries WHERE player_id = $1`

// DebitIfSufficient is the one operation in this package that needs a
// real transaction, not a single statement. The `SELECT ... FOR UPDATE`
// against the player's own row is what makes it atomic: it takes a
// row-level lock for the duration of the transaction, so a second,
// concurrent DebitIfSufficient call for the *same* player blocks until
// the first one commits or rolls back — serializing exactly the calls
// that need to be serialized (same player), while debits for different
// players still run fully in parallel, each locking a different row.
func (r *LedgerRepository) DebitIfSufficient(ctx context.Context, entry domain.LedgerEntry, minBalance int) (int, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, fmt.Errorf("debit: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed; here purely to cover early-return paths

	var lockedID string
	err = tx.QueryRowContext(ctx, `SELECT id FROM players WHERE id = $1 FOR UPDATE`, entry.PlayerID).Scan(&lockedID)
	if err != nil {
		return 0, false, fmt.Errorf("debit: lock player: %w", err)
	}

	var alreadyExists bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM ledger_entries WHERE id = $1)`, entry.ID).Scan(&alreadyExists)
	if err != nil {
		return 0, false, fmt.Errorf("debit: idempotency check: %w", err)
	}

	var balance int
	if err := tx.QueryRowContext(ctx, balanceQuery, entry.PlayerID).Scan(&balance); err != nil {
		return 0, false, fmt.Errorf("debit: balance: %w", err)
	}

	if alreadyExists {
		// Idempotent replay: this exact debit already happened under
		// this same transaction's lock at some point in the past, so
		// the balance computed above already reflects it. Nothing left
		// to do — commit (releases the lock) and return that balance.
		if err := tx.Commit(); err != nil {
			return 0, false, fmt.Errorf("debit: commit: %w", err)
		}
		return balance, false, nil
	}

	if balance < minBalance {
		// Rolling back here releases the row lock; the deferred
		// Rollback() above is a no-op once this explicit one has run.
		if err := tx.Rollback(); err != nil {
			return 0, false, fmt.Errorf("debit: rollback: %w", err)
		}
		return balance, false, fmt.Errorf("have %d, need %d: %w", balance, minBalance, domain.ErrInsufficientBalance)
	}

	const insert = `
		INSERT INTO ledger_entries (id, player_id, delta, reason, created_at)
		VALUES ($1, $2, $3, $4, $5)`
	if _, err := tx.ExecContext(ctx, insert, entry.ID, entry.PlayerID, entry.Delta, entry.Reason, entry.CreatedAt); err != nil {
		return 0, false, fmt.Errorf("debit: insert: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, false, fmt.Errorf("debit: commit: %w", err)
	}
	return balance + entry.Delta, true, nil
}

// AppendWithOutbox is Append plus writing an outbox row, both in one
// transaction — see domain.OutboxEvent's doc comment for why that
// matters. Reusing entry.ID as the outbox row's own id keeps a clean
// 1:1 mapping and gets the outbox insert the same ON CONFLICT DO
// NOTHING idempotency as the ledger insert, for free.
//
// This is how PlayerLedger.CreditPoints detects that *LedgerRepository
// supports the outbox pattern (see ledger.outboxAppender) — satisfying
// this method signature is the entire contract, no explicit interface
// implementation needed on either side.
func (r *LedgerRepository) AppendWithOutbox(ctx context.Context, entry domain.LedgerEntry, event domain.OutboxEvent) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("append with outbox: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	result, err := tx.ExecContext(ctx, `
		INSERT INTO ledger_entries (id, player_id, delta, reason, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO NOTHING`,
		entry.ID, entry.PlayerID, entry.Delta, entry.Reason, entry.CreatedAt)
	if err != nil {
		return false, fmt.Errorf("append with outbox: insert ledger entry: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("append with outbox: rows affected: %w", err)
	}
	created := rows == 1

	if created {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO outbox_events (id, event_type, payload, created_at)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (id) DO NOTHING`,
			entry.ID, event.Type, event.Payload, entry.CreatedAt)
		if err != nil {
			return false, fmt.Errorf("append with outbox: insert outbox event: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("append with outbox: commit: %w", err)
	}
	return created, nil
}

// OutboxRow is one unpublished outbox entry, as the relay sees it.
type OutboxRow struct {
	ID      string
	Payload string
}

// PendingOutboxEvents returns up to limit not-yet-published rows, oldest
// first — what the relay (internal/outbox) polls on a timer.
func (r *LedgerRepository) PendingOutboxEvents(ctx context.Context, limit int) ([]OutboxRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, payload FROM outbox_events
		WHERE published_at IS NULL
		ORDER BY created_at
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("pending outbox events: %w", err)
	}
	defer func() { _ = rows.Close() }() // an error closing an already-fully-read cursor isn't actionable

	var out []OutboxRow
	for rows.Next() {
		var row OutboxRow
		if err := rows.Scan(&row.ID, &row.Payload); err != nil {
			return nil, fmt.Errorf("pending outbox events: scan: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pending outbox events: %w", err)
	}
	return out, nil
}

// MarkOutboxPublished records that a row's event was handed off to SQS
// successfully, so the relay's next poll skips it.
func (r *LedgerRepository) MarkOutboxPublished(ctx context.Context, id string, publishedAt time.Time) error {
	if _, err := r.db.ExecContext(ctx, `UPDATE outbox_events SET published_at = $2 WHERE id = $1`, id, publishedAt); err != nil {
		return fmt.Errorf("mark outbox published: %w", err)
	}
	return nil
}
