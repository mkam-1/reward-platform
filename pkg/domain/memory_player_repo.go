package domain

import "context"

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
