package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"reward-platform/pkg/domain"
)

// PlayerRepository is the Postgres-backed domain.PlayerRepository.
// Structurally identical in purpose to the in-memory one
// (memory_player_repo.go in pkg/domain) — Engine can't tell which one
// it's talking to.
type PlayerRepository struct {
	db *sql.DB
}

func NewPlayerRepository(db *sql.DB) *PlayerRepository {
	return &PlayerRepository{db: db}
}

func (r *PlayerRepository) Put(ctx context.Context, p domain.Player) error {
	const query = `
		INSERT INTO players (id, username, created_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (id) DO UPDATE SET username = EXCLUDED.username`

	if _, err := r.db.ExecContext(ctx, query, p.ID, p.Username, p.CreatedAt); err != nil {
		return fmt.Errorf("put player: %w", err)
	}
	return nil
}

func (r *PlayerRepository) Get(ctx context.Context, id string) (domain.Player, error) {
	const query = `SELECT id, username, created_at FROM players WHERE id = $1`

	var p domain.Player
	err := r.db.QueryRowContext(ctx, query, id).Scan(&p.ID, &p.Username, &p.CreatedAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// sql.ErrNoRows is database/sql's own not-found signal; domain
		// code doesn't know about database/sql, so it gets translated to
		// the domain's own ErrNotFound here — the same sentinel the
		// in-memory Store uses, so Engine's error handling doesn't care
		// which repository produced it.
		return domain.Player{}, fmt.Errorf("id %q: %w", id, domain.ErrNotFound)
	case err != nil:
		return domain.Player{}, fmt.Errorf("get player: %w", err)
	}
	return p, nil
}

func (r *PlayerRepository) List(ctx context.Context) ([]domain.Player, error) {
	const query = `SELECT id, username, created_at FROM players ORDER BY created_at`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list players: %w", err)
	}
	defer rows.Close()

	var players []domain.Player
	for rows.Next() {
		var p domain.Player
		if err := rows.Scan(&p.ID, &p.Username, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("list players: scan: %w", err)
		}
		players = append(players, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list players: %w", err)
	}
	return players, nil
}
