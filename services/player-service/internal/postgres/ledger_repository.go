package postgres

import (
	"context"
	"database/sql"
	"fmt"

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

func (r *LedgerRepository) Append(ctx context.Context, entry domain.LedgerEntry) error {
	const query = `
		INSERT INTO ledger_entries (id, player_id, delta, reason, created_at)
		VALUES ($1, $2, $3, $4, $5)`

	_, err := r.db.ExecContext(ctx, query, entry.ID, entry.PlayerID, entry.Delta, entry.Reason, entry.CreatedAt)
	if err != nil {
		return fmt.Errorf("append ledger entry: %w", err)
	}
	return nil
}

func (r *LedgerRepository) Balance(ctx context.Context, playerID string) (int, error) {
	const query = `SELECT COALESCE(SUM(delta), 0) FROM ledger_entries WHERE player_id = $1`

	var balance int
	if err := r.db.QueryRowContext(ctx, query, playerID).Scan(&balance); err != nil {
		return 0, fmt.Errorf("ledger balance: %w", err)
	}
	return balance, nil
}
