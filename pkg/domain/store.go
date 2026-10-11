package domain

import (
	"context"
	"fmt"
	"sync"
)

// Store is a minimal thread-safe in-memory repository keyed by string ID.
// It's generic over the entity type so Engine can reuse one implementation
// for Game and Reward instead of writing two near-identical maps. (Player
// and the ledger used to work the same way — see memory_player_repo.go
// and memory_ledger_repo.go for why they're now their own types instead:
// they needed a real PlayerRepository/LedgerRepository shape once a
// Postgres-backed alternative existed.)
//
// Every method takes a context.Context it never actually uses — a plain
// map has nothing to cancel or time out. It's there so call sites in
// Engine look identical regardless of which kind of store they're
// talking to, and so promoting Game or Reward to a real database later
// (phase 5 does exactly this for Reward) wouldn't change Engine's code,
// only which concrete type gets wired in.
type Store[T any] struct {
	mu   sync.RWMutex
	data map[string]T
}

func NewStore[T any]() *Store[T] {
	return &Store[T]{data: make(map[string]T)}
}

func (s *Store[T]) Put(_ context.Context, id string, v T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[id] = v
}

func (s *Store[T]) Get(_ context.Context, id string) (T, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[id]
	if !ok {
		return v, fmt.Errorf("id %q: %w", id, ErrNotFound)
	}
	return v, nil
}

// List returns a snapshot of every stored value, in no particular order
// (Go map iteration order is intentionally randomized).
func (s *Store[T]) List(_ context.Context) []T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]T, 0, len(s.data))
	for _, v := range s.data {
		out = append(out, v)
	}
	return out
}
