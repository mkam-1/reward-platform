package domain

import (
	"context"
	"fmt"
	"sync"
)

// memoryRedemptionRepository is the in-memory RedemptionRepository.
// ListByPlayer is a linear scan here — fine for an in-memory map, and
// exactly the operation the DynamoDB-backed implementation's GSI exists
// to make efficient instead.
type memoryRedemptionRepository struct {
	mu   sync.RWMutex
	data map[string]Redemption
}

func NewMemoryRedemptionRepository() RedemptionRepository {
	return &memoryRedemptionRepository{data: make(map[string]Redemption)}
}

func (r *memoryRedemptionRepository) Create(_ context.Context, redemption Redemption) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.data[redemption.ID]; exists {
		return false, nil
	}
	r.data[redemption.ID] = redemption
	return true, nil
}

func (r *memoryRedemptionRepository) Get(_ context.Context, id string) (Redemption, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	redemption, ok := r.data[id]
	if !ok {
		return Redemption{}, fmt.Errorf("id %q: %w", id, ErrNotFound)
	}
	return redemption, nil
}

func (r *memoryRedemptionRepository) ListByPlayer(_ context.Context, playerID string) ([]Redemption, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []Redemption
	for _, redemption := range r.data {
		if redemption.PlayerID == playerID {
			out = append(out, redemption)
		}
	}
	return out, nil
}
