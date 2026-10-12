# ADR 0005: Idempotency keys + atomic conditional writes for safe retries

## Status
Accepted (phases 5-6)

## Context
Once communication crosses a network boundary (SQS, gRPC), every
message can arrive more than once: SQS is at-least-once delivery
(confirmed by the outbox pattern's own retry-until-success logic — ADR
0004), and a gRPC caller that times out waiting for a response has no
way to know whether the call actually succeeded server-side, so a safe
caller has to be able to retry. "Redeem a reward" and "credit points"
are both money-like operations — applying one twice is a real bug
(double-spend, double-credit), not a cosmetic one.

## Decision
Every write that can be retried is made idempotent, keyed by an ID the
*caller* controls and reuses on retry:
- `RedemptionRepository.Create` / `LedgerRepository.Append` /
  `LedgerRepository.DebitIfSufficient` all report `created=false`
  instead of erroring when that ID already exists — a retry's intended
  effect ("this redemption/credit/debit happened") is already true, so
  nothing further happens.
- `Engine.RedeemReward` and `catalog.RewardCatalog.RedeemReward` check
  idempotency *first*, before any validation — a replay must return the
  original result even if the player's balance has since changed
  because of the very operation being replayed (see the ordering bug
  this caught, fixed in phase 5, documented in `RedeemReward`'s own
  comment).
- Where two concurrent *new* (not retried) requests could both pass a
  check before either commits — the classic TOCTOU race —
  `RewardRepository.DecrementStock` and `LedgerRepository.
  DebitIfSufficient` make the check-and-write atomic at the storage
  layer (a conditional `UpdateItem` for DynamoDB, a row-locked
  transaction for Postgres), not something the caller assembles from
  separate `Get`+`Put` calls.

## Consequences
- Idempotency and atomicity are two different problems solved two
  different ways: idempotency keys handle "the same request arrived
  twice"; atomic conditional writes handle "two different requests
  raced." Both are necessary; neither substitutes for the other.
- A caller that doesn't supply an idempotency key (the CLI, load
  testing) gets a server-generated one and loses retry-safety — an
  explicit opt-in, not a silent gap.
- Across *different* datastores (the DynamoDB redemption record vs the
  Postgres ledger debit it depends on), there's still no atomicity —
  see ADR 0002's and `RedeemReward`'s notes on the remaining
  distributed-transaction gap. Idempotency keys make retries safe; they
  don't make a partially-applied operation (crashed mid-way) consistent.

## Alternatives considered
- **Server-generated IDs only**: simpler, but makes retries
  fundamentally unsafe — the server has no way to recognize "this is the
  same logical request" versus "this is a new one," since every attempt
  gets a different ID.
- **Distributed lock (e.g. a Redis mutex) instead of atomic conditional
  writes**: adds an extra infrastructure dependency and a new failure
  mode (what happens if the lock service itself is down) for something
  the existing datastores can already guarantee natively.
