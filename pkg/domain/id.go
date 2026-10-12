package domain

import (
	"fmt"
	"sync"
)

// IDGenerator hands out simple, readable, monotonically increasing IDs
// like "player_1", "game_2", scoped per prefix. Real IDs (UUIDs, ULIDs)
// arrive when a real datastore does; this exists so tests get predictable,
// dependency-free IDs in the meantime.
//
// Exported (phase 6) so each split-out service can mint its own IDs the
// same way Engine always has, without duplicating this type.
type IDGenerator struct {
	mu     sync.Mutex
	counts map[string]int
}

func NewIDGenerator() *IDGenerator {
	return &IDGenerator{counts: make(map[string]int)}
}

func (g *IDGenerator) Next(prefix string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.counts[prefix]++
	return fmt.Sprintf("%s_%d", prefix, g.counts[prefix])
}
