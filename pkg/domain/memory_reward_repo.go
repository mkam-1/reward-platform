package domain

import (
	"context"
	"fmt"
	"sync"
)

// memoryRewardRepository adapts Store[Reward] to RewardRepository. Its
// own mutex (separate from Store's internal one) is what makes
// DecrementStock atomic: Get-check-Put has to happen as one uninterrupted
// sequence, which Store's per-call locking alone can't guarantee.
type memoryRewardRepository struct {
	mu    sync.Mutex
	store *Store[Reward]
}

func NewMemoryRewardRepository() RewardRepository {
	return &memoryRewardRepository{store: NewStore[Reward]()}
}

func (r *memoryRewardRepository) Put(ctx context.Context, reward Reward) error {
	r.store.Put(ctx, reward.ID, reward)
	return nil
}

func (r *memoryRewardRepository) Get(ctx context.Context, id string) (Reward, error) {
	return r.store.Get(ctx, id)
}

func (r *memoryRewardRepository) List(ctx context.Context) ([]Reward, error) {
	return r.store.List(ctx), nil
}

func (r *memoryRewardRepository) DecrementStock(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	reward, err := r.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if reward.Stock <= 0 {
		return fmt.Errorf("reward %q: %w", reward.Name, ErrOutOfStock)
	}
	reward.Stock--
	r.store.Put(ctx, id, reward)
	return nil
}
