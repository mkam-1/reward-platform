package domain

import (
	"context"
	"fmt"
	"sync"
)

// memoryLedgerRepository is the in-memory LedgerRepository: a mutex-
// guarded map keyed by entry ID, exactly what used to live directly on
// Engine before the ledger needed to be swappable for a Postgres-backed
// implementation too. Keying by ID (rather than a plain slice) is what
// makes Append's idempotency check an O(1) lookup.
type memoryLedgerRepository struct {
	mu      sync.RWMutex
	entries map[string]LedgerEntry
}

func NewMemoryLedgerRepository() LedgerRepository {
	return &memoryLedgerRepository{entries: make(map[string]LedgerEntry)}
}

func (r *memoryLedgerRepository) Append(_ context.Context, entry LedgerEntry) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.entries[entry.ID]; exists {
		return false, nil
	}
	r.entries[entry.ID] = entry
	return true, nil
}

func (r *memoryLedgerRepository) Balance(_ context.Context, playerID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.balanceLocked(playerID), nil
}

func (r *memoryLedgerRepository) balanceLocked(playerID string) int {
	balance := 0
	for _, entry := range r.entries {
		if entry.PlayerID == playerID {
			balance += entry.Delta
		}
	}
	return balance
}

func (r *memoryLedgerRepository) DebitIfSufficient(_ context.Context, entry LedgerEntry, minBalance int) (int, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.entries[entry.ID]; exists {
		return r.balanceLocked(entry.PlayerID), false, nil
	}

	balance := r.balanceLocked(entry.PlayerID)
	if balance < minBalance {
		return balance, false, fmt.Errorf("have %d, need %d: %w", balance, minBalance, ErrInsufficientBalance)
	}

	r.entries[entry.ID] = entry
	return balance + entry.Delta, true, nil
}
