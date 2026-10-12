# ADR 0003: gRPC for redemption's balance check, SQS for session credit

## Status
Accepted (phase 6)

## Context
Phase 6 split one process into three services, which turns what used to
be in-process function calls into real network calls. Two call sites
needed to cross a service boundary:

1. game-service finishes a session and needs player-service to credit
   points.
2. rewards-service is handling a redemption and needs to know — right
   now — whether the player can afford it, then debit if so.

These have different correctness requirements. (1) can tolerate a few
seconds of delay before the points show up; nothing downstream is
blocked waiting for it. (2) cannot: the HTTP caller is waiting for an
answer, and the answer depends on a balance that has to be current at
the moment of the check, not current "soon."

## Decision
(1) is asynchronous: game-service publishes `SessionCompleted` to SQS;
player-service consumes it whenever it gets to it. (2) is synchronous:
rewards-service calls player-service's gRPC `PlayerLedger` service
(`GetBalance`, `DebitPoints`) and waits for the real answer before
deciding whether the redemption succeeds.

## Consequences
- The two mechanisms needed genuinely different reliability treatment:
  SQS already retries delivery at the infrastructure level (ADR 0004
  builds on that with the outbox pattern); the gRPC call needed
  *application-level* retry and circuit-breaking added explicitly (ADR
  0007), because a single unary call has no built-in retry of its own.
- game-service's `RecordSession` HTTP endpoint returns `202 Accepted`
  with no result body — an honest, visible consequence of the call being
  fire-and-forget. A caller checking the response can't see the points
  land; they have to check balance separately, moments later.
- rewards-service's redemption endpoint still returns a normal
  `200`/`4xx` with the real outcome, because the call it depends on is
  synchronous.

## Alternatives considered
- **Everything over SQS** (fully async): would have made redemption's
  balance check either stale (read a cached/eventually-consistent
  balance) or required a second round trip waiting for an async reply,
  both worse than a direct synchronous call for a check that needs to be
  current *now*.
- **Everything over gRPC** (fully sync): would have made game-service's
  session recording block on player-service's availability and latency,
  for no benefit — nothing about recording a session needs an immediate
  answer.
