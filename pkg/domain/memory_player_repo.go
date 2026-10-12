package domain

import (
	"context"
	"sort"
)

// memoryPlayerRepository adapts the generic Store[Player] to the
// PlayerRepository interface's shape: Put takes the whole Player, not a
// separate id argument, because that's how a real INSERT works too (the
// id is a field on the row, not a parameter alongside it).
type memoryPlayerRepository struct {
	store *Store[Player]
}

// NewMemoryPlayerRepository is the default PlayerRepository: in-memory,
// used by tests and cmd/rewardctl. player-service wires in a Postgres-
// backed one instead (see internal/postgres).
func NewMemoryPlayerRepository() PlayerRepository {
	return &memoryPlayerRepository{store: NewStore[Player]()}
}

func (r *memoryPlayerRepository) Put(ctx context.Context, p Player) error {
	r.store.Put(ctx, p.ID, p)
	return nil
}

func (r *memoryPlayerRepository) Get(ctx context.Context, id string) (Player, error) {
	return r.store.Get(ctx, id)
}

func (r *memoryPlayerRepository) List(ctx context.Context) ([]Player, error) {
	return r.store.List(ctx), nil
}

func (r *memoryPlayerRepository) ListPage(ctx context.Context, limit int, after *PlayerPageCursor) ([]Player, error) {
	all, _ := r.List(ctx)
	sort.Slice(all, func(i, j int) bool {
		if all[i].CreatedAt.Equal(all[j].CreatedAt) {
			return all[i].ID < all[j].ID
		}
		return all[i].CreatedAt.Before(all[j].CreatedAt)
	})

	start := 0
	if after != nil {
		for i, p := range all {
			if p.CreatedAt.After(after.CreatedAt) || (p.CreatedAt.Equal(after.CreatedAt) && p.ID > after.ID) {
				start = i
				break
			}
			start = i + 1
		}
	}

	end := start + limit
	if end > len(all) {
		end = len(all)
	}
	if start >= len(all) {
		return []Player{}, nil
	}
	return all[start:end], nil
}
