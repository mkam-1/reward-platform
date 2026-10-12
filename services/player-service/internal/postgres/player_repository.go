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
	defer func() { _ = rows.Close() }() // an error closing an already-fully-read cursor isn't actionable

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

// ListPage is keyset pagination: `WHERE (created_at, id) > (cursor)` is
// a single indexed range scan returning exactly `limit` rows, unlike
// List's unbounded `SELECT *` or OFFSET-based paging (which still has
// to walk and discard every row before the offset — just as expensive
// as no pagination at all once that offset is a few hundred thousand
// rows in). idx_players_created_at_id (see migrations) is what makes
// this a cheap index scan rather than a sort over the whole table.
func (r *PlayerRepository) ListPage(ctx context.Context, limit int, after *domain.PlayerPageCursor) ([]domain.Player, error) {
	var rows *sql.Rows
	var err error
	if after == nil {
		rows, err = r.db.QueryContext(ctx, `
			SELECT id, username, created_at FROM players
			ORDER BY created_at, id
			LIMIT $1`, limit)
	} else {
		rows, err = r.db.QueryContext(ctx, `
			SELECT id, username, created_at FROM players
			WHERE (created_at, id) > ($1, $2)
			ORDER BY created_at, id
			LIMIT $3`, after.CreatedAt, after.ID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("list players page: %w", err)
	}
	defer func() { _ = rows.Close() }()

	players := make([]domain.Player, 0, limit)
	for rows.Next() {
		var p domain.Player
		if err := rows.Scan(&p.ID, &p.Username, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("list players page: scan: %w", err)
		}
		players = append(players, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list players page: %w", err)
	}
	return players, nil
}
