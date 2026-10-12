# ADR 0004: Outbox pattern for publishing PointsAwarded

## Status
Accepted (phase 6)

## Context
After player-service credits points from a `SessionCompleted` event, it
needs to publish `PointsAwarded` to SQS. The naive approach — commit the
ledger credit, then call `Publish` — has a gap: the process can crash
(or the network can fail) in between those two steps. The database
commit already happened; the event is now silently lost, with nothing
in the system recording that a publish was ever owed.

## Decision
`PlayerLedger.CreditPoints` writes the ledger entry *and* an outbox row
in one Postgres transaction (`LedgerRepository.AppendWithOutbox`) — they
either both commit or neither does. A separate background loop
(`services/player-service/internal/outbox.Relay`) polls for unpublished
outbox rows every 2 seconds, publishes each to SQS, and marks it
published only after a successful send.

## Consequences
- A transient SQS outage can't lose a `PointsAwarded` event — the
  outbox row sits there until the relay's next tick succeeds, as many
  ticks as it takes.
- The HTTP/SQS-consumer path that triggers `CreditPoints` never blocks
  on SQS being reachable — publishing is decoupled from crediting,
  which is why the relay is a separate loop rather than inline.
- At-least-once delivery, not exactly-once: if the relay publishes
  successfully but crashes before marking the row published, the same
  event gets republished on restart. This is why `PointsAwarded`'s
  consumer (and every consumer in this system) has to tolerate
  redelivery — see ADR 0005.
- Extra moving part: a dedicated table (`outbox_events`) and a
  dedicated polling loop, for every service that needs this guarantee.
  Only player-service has it; game-service's `SessionCompleted` publish
  is a plain "write, then publish" with the same gap this ADR closes for
  the other leg — see the honesty note in `catalog.GameCatalog`'s
  godoc. Game-service doesn't have a transactional datastore to anchor
  an outbox table to (Game is still in-memory), so the pattern wasn't
  retrofitted there; a real fix would mean giving Game a real datastore
  first.

## Alternatives considered
- **Dual write, no outbox** (what game-service still does): simplest,
  but has the exact crash-window gap described above. Accepted as a
  known, explicitly-documented simplification for game-service
  specifically, not as the general policy.
- **Distributed transaction (2PC) across Postgres and SQS**: SQS doesn't
  support participating in an external transaction coordinator; not
  available as an option at all.
