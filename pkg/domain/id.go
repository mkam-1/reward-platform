package domain

import (
	"fmt"
	"sync"
)

// idGenerator hands out simple, readable, monotonically increasing IDs
// like "player_1", "game_2", scoped per prefix. Real IDs (UUIDs, ULIDs)
// arrive when a real datastore does; this exists so tests get predictable,
// dependency-free IDs in the meantime.
type idGenerator struct {
	mu     sync.Mutex
	counts map[string]int
}

func newIDGenerator() *idGenerator {
	return &idGenerator{counts: make(map[string]int)}
}

func (g *idGenerator) next(prefix string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.counts[prefix]++
	return fmt.Sprintf("%s_%d", prefix, g.counts[prefix])
}
