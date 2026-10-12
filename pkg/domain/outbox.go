package domain

// OutboxEvent is a message queued for reliable publishing alongside a
// local write, in the same transaction — the outbox pattern. A plain
// "write to the database, then separately call Publish" has a gap: the
// process can crash (or just lose the network) between those two steps,
// silently dropping an event a database commit already promised
// happened. Writing the event into the same transaction as the
// business write removes that gap — either both happen or neither does
// — at the cost of needing a separate relay process to actually read
// this table and publish to the real queue afterward.
//
// Not part of LedgerRepository itself: only an implementation with a
// real transactional datastore to anchor the pattern to can support it
// (see services/player-service/internal/postgres), so it's an optional
// capability a repository can additionally implement, not a universal
// requirement every LedgerRepository must satisfy.
type OutboxEvent struct {
	Type    string
	Payload string
}
