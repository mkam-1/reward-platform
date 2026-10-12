# ADR 0001: Repository interfaces instead of direct storage access

## Status
Accepted (phase 4, extended in phase 5)

## Context
`Engine` originally (phase 1) owned its storage directly — concrete
`Store[T]` fields, in-memory only. Phase 4 needed Postgres persistence
for Players and the ledger; phase 5 needed DynamoDB for Rewards and
Redemptions. Each of those is a different underlying technology with a
different programming model (SQL transactions vs conditional
`UpdateItem`), but the business logic calling into them (`RegisterPlayer`,
`RedeemReward`, etc.) shouldn't need to know or care which one it's
talking to.

## Decision
Define narrow interfaces per entity (`PlayerRepository`,
`LedgerRepository`, `RewardRepository`, `RedemptionRepository`) in
`pkg/domain`, each describing only the operations the business logic
actually needs — not a generic CRUD interface, but operations shaped
around real use, e.g. `DecrementStock` and `DebitIfSufficient` as single
atomic operations rather than exposing `Get`+`Put` for the caller to
compose (and potentially race). Business logic depends only on these
interfaces; `main.go` in each service decides which concrete
implementation to inject (in-memory for tests, Postgres/DynamoDB for
production).

## Consequences
- Business logic (`Engine`, later `ledger.PlayerLedger` and
  `catalog.RewardCatalog`) is testable without a real database — see
  `domain.NewMemory*Repository()`, used throughout the test suite.
- Swapping or adding a persistence backend never requires changing
  business logic, only writing a new implementation and changing what
  gets injected in `main.go`. Reward/Redemption's move from "doesn't
  exist" (phase 4) to DynamoDB (phase 5) touched zero business-logic
  files.
- Interface methods had to be designed around actual atomicity needs
  (`DecrementStock`, `DebitIfSufficient`) rather than generic
  `Get`/`Set` — more upfront design thought, but it's what makes the
  concurrency-safety guarantees in ADR 0005 possible at all.

## Alternatives considered
- **Generic `Repository[T]` with `Get`/`Put`/`Delete`**: simpler to
  write, but pushes atomicity concerns (stock decrement, balance check)
  onto the caller, which can't actually implement them atomically from
  outside the storage layer. Rejected.
- **No interface, concrete `*postgres.X` / `*dynamodb.X` types directly
  in business logic**: fastest to write, but makes business logic
  untestable without a real database and couples it to a specific
  storage technology. Rejected — exactly the coupling this ADR exists
  to avoid.
