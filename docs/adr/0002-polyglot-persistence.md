# ADR 0002: Postgres for players/ledger, DynamoDB for rewards/redemptions

## Status
Accepted (phases 4-5)

## Context
The JD this project is built against explicitly asks for proficiency in
both relational and NoSQL databases. Beyond the learning goal, the two
entity groups have genuinely different access patterns: the points
ledger needs `SUM(delta) WHERE player_id = ?` — a natural fit for SQL
aggregation — and later (phase 6) needed row-level locking
(`SELECT ... FOR UPDATE`) for safe concurrent debits, which requires a
real relational transaction. Rewards/Redemptions need "list this
player's redemption history" and "atomically decrement stock" — both
servable from DynamoDB's conditional writes and a GSI, without needing
cross-row transactional semantics.

## Decision
Players and the ledger live in Postgres. Rewards and Redemptions live in
DynamoDB, single-table design (`PK`/`SK` plus one GSI keyed by player for
redemption history — see the comment on
`services/rewards-service/internal/dynamodb/dynamodb.go`).

## Consequences
- Each service's `internal/postgres` or `internal/dynamodb` package is
  the only place that datastore's query patterns live — nothing in
  `pkg/domain` or the `catalog`/`ledger` business-logic packages knows
  which database is behind the `Repository` interfaces (ADR 0001).
- Two different operational models to run locally and eventually deploy
  (phase 11): a Postgres instance plus migrations, and a DynamoDB table
  plus its GSI. More moving parts than a single datastore would have
  been.
- There is **no cross-database transaction** between a Reward's stock
  decrement (DynamoDB) and a Redemption's existence (DynamoDB, same
  table — fine) versus the ledger debit (Postgres, separate database).
  `Engine.RedeemReward`'s doc comment and ADR 0004 are explicit about
  this gap: it's a real distributed-transaction problem, not something
  this design pretends to solve.

## Alternatives considered
- **Everything in Postgres**: simplest operationally, satisfies none of
  the NoSQL learning goal, and loses the chance to practice DynamoDB's
  genuinely different data-modeling approach (single-table design, GSIs,
  conditional writes) — the JD asks for this explicitly.
- **Everything in DynamoDB**: would have made the ledger's `SUM()`
  balance query and the row-locked `DebitIfSufficient` (ADR 0005) either
  impossible or far more awkward to implement correctly.
