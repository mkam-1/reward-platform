# ADR 0007: Retry + circuit breaker around the gRPC balance/debit calls

## Status
Accepted (phase 7)

## Context
ADR 0003 made rewards-service's balance check and debit a synchronous
gRPC call to player-service, deliberately — the caller needs a real,
current answer. But a single unary gRPC call has no retry of its own: a
momentary network blip or a player-service pod mid-restart fails the
whole redemption immediately, even though the dependency might be fine
one second later. And if player-service is down hard (not a blip, an
actual outage), every redemption attempt still pays the full cost of
dialing, waiting, and failing, pointlessly, for as long as the outage
lasts.

## Decision
Wrap the raw gRPC client in two composable decorators, in this specific
order: `CircuitBreaker(Retrying(raw))`.

- **Retrying** retries a transient failure with exponential backoff, up
  to a bounded number of attempts — but only for errors that aren't a
  recognized business-logic answer. Retrying `ErrInsufficientBalance`
  would just get the same correct "no" again; only unrecognized
  (infrastructure-shaped) errors are retried.
- **CircuitBreaker** tracks consecutive call failures (each call to
  Retrying counts as one, regardless of how many attempts it took
  internally) and, past a threshold, fails fast with `ErrCircuitOpen`
  for a cooldown period instead of attempting the call at all — the
  mechanism that stops redemption requests from queuing up behind a
  dependency that's already shown it's down.

Both are implemented as decorators, not built into `grpcclient` itself —
the same shape as the HTTP middleware chain from phase 3 — so each is
independently testable with a fake inner `caller`, and the plain
(undecorated) client remains available for anything that doesn't want
this behavior.

## Consequences
- A transient failure (one dropped packet, one momentarily slow
  response) is now invisible to the end user — it gets retried and
  succeeds, instead of surfacing as a failed redemption.
- A real player-service outage is now visibly different from a timeout:
  redemption requests fail fast with `ErrCircuitOpen` instead of each
  one hanging for the full retry budget, and player-service stops
  receiving load it can't handle anyway while it's down.
- One new failure mode to be aware of: `ErrCircuitOpen` itself. The HTTP
  layer maps it through `statusFor`'s default case today (500) — a
  dedicated 503 mapping would be a reasonable follow-up, not done yet.
- Configuration (attempt count, backoff, failure threshold, cooldown) is
  exposed via functional options (`WithMaxAttempts`, `WithCooldown`,
  etc.) rather than a constructor with a growing positional parameter
  list — the same reasoning as `domain.Repositories` bundling phase 4/5's
  growing constructor, applied here via options instead of a struct
  since these are mostly independent, individually-defaulted knobs.

## Alternatives considered
- **A third-party circuit breaker library** (e.g. `sony/gobreaker`):
  battle-tested, less code to maintain, and what a real production
  system would likely reach for once past a certain scale. Implemented
  from scratch here instead, deliberately — the point of this phase is
  understanding the mechanism, not just using it; swapping in a library
  later wouldn't change anything about `catalog.PlayerBalance`'s
  contract, since the decorator itself is an implementation detail
  behind an interface already.
- **Retry without a circuit breaker**: solves transient blips, does
  nothing for a sustained outage — retries would just keep hammering a
  dependency that's genuinely down, for every request, for as long as
  the outage lasts.
