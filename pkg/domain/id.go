package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// IDGenerator hands out globally unique, readable IDs like
// "player_4f9c2a1e8b3d6f70", scoped per prefix for readability in logs
// and URLs.
//
// This used to be a per-process sequential counter (prefix_1, prefix_2,
// ...), which is a correctness bug, not just a style choice, the moment
// more than one replica of a service exists — which horizontal scaling
// (a k8s HPA, ECS desired_count > 1) makes the normal case, not an edge
// case. Two pods each start their own counter at 1, so two different
// players registered on two different pods can both be assigned
// "player_1". Worse than a cosmetic collision: player_repository.Put
// does `ON CONFLICT (id) DO UPDATE SET username = ...`, so the second
// pod's write silently overwrites the first player's username under
// the same ID — two real, distinct players, one surviving record. The
// same collision on a ledger entry's ID is worse still: Append/
// AppendWithOutbox are `ON CONFLICT (id) DO NOTHING`, specifically so a
// *retried* request (the same logical event) is a safe no-op — but a
// collision between two *different* credits/debits from two different
// pods makes the second one silently vanish, as if it had already
// happened, when it hadn't. That's a real points/money bug, not a
// theoretical one.
//
// crypto/rand sidesteps all of that: no shared state between
// processes, so no coordination between replicas is needed at all. 16
// random bytes (128 bits) makes an accidental collision astronomically
// unlikely — the same reasoning any UUIDv4 implementation relies on,
// just without adding a dependency for it.
type IDGenerator struct{}

func NewIDGenerator() *IDGenerator {
	return &IDGenerator{}
}

func (g *IDGenerator) Next(prefix string) string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read on a small buffer essentially never fails
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b[:]))
}
