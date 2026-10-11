package domain

import (
	"fmt"
	"sync"
)

// Store is a minimal thread-safe in-memory repository keyed by string ID.
// It's generic over the entity type so Engine can reuse one implementation
// for Player, Game and Reward instead of writing three near-identical maps.
//
// This is deliberately the throwaway implementation: real persistence
// (Postgres, DynamoDB) arrives in later phases behind a repository
// interface shaped like this one.
type Store[T any] struct {
	mu   sync.RWMutex
	data map[string]T
}

func NewStore[T any]() *Store[T] {
	return &Store[T]{data: make(map[string]T)}
}

func (s *Store[T]) Put(id string, v T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[id] = v
}

func (s *Store[T]) Get(id string) (T, error) {
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
func (s *Store[T]) List() []T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]T, 0, len(s.data))
	for _, v := range s.data {
		out = append(out, v)
	}
	return out
}
