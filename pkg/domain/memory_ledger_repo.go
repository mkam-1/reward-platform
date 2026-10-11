package domain

import (
	"context"
	"sync"
)

// memoryLedgerRepository is the in-memory LedgerRepository: a mutex-
// guarded slice, exactly what used to live directly on Engine before the
// ledger needed to be swappable for a Postgres-backed implementation too.
type memoryLedgerRepository struct {
	mu      sync.RWMutex
	entries []LedgerEntry
}

func NewMemoryLedgerRepository() LedgerRepository {
	return &memoryLedgerRepository{}
}

func (r *memoryLedgerRepository) Append(_ context.Context, entry LedgerEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, entry)
	return nil
}

func (r *memoryLedgerRepository) Balance(_ context.Context, playerID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	balance := 0
	for _, entry := range r.entries {
		if entry.PlayerID == playerID {
			balance += entry.Delta
		}
	}
	return balance, nil
}
