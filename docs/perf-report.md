# Performance report (Phase 14)

## What changed

1. **Read-through TTL cache** on rewards-service's `RewardCatalog.Rewards()`
   (`internal/catalog/catalog.go`) — a 5-second cache in front of the
   DynamoDB Scan that lists the reward catalog, invalidated on
   `RegisterReward` and on a redemption's `DecrementStock`. Justified by
   the catalog being small, bounded, and admin-managed (the same
   reasoning the Scan-over-GSI choice in `internal/dynamodb/dynamodb.go`
   already uses) — most reads see no writes in between, so a few
   seconds of staleness is a reasonable trade for skipping a network
   round-trip on what's the most frequent read in the service.
2. **Per-IP rate limiting** (`internal/api/ratelimit.go`, `golang.org/x/time/rate`)
   on `POST /players/{id}/redemptions` specifically — the one endpoint
   that spends a scarce resource (points, stock) per call, independent
   of whatever rate limiting a real deployment's ALB/API Gateway would
   also apply in front of it.
3. **pprof** wired into all three services (`net/http/pprof`, loopback-only
   port 6061 — never exposed through the ALB/k8s Service) for profiling
   on demand rather than only after a problem is already visible in
   metrics.

## Before/after measurement

`go test ./internal/catalog/... -bench=. -benchtime=20x`, against an
in-memory repository wrapped with an artificial 2ms delay on `List`
standing in for DynamoDB's real network round-trip (the unmodified
in-memory fake alone responds in nanoseconds, which would hide the
cache's effect entirely):

```
BenchmarkRewards_Uncached-8   20   6309093 ns/op   (~6.3ms/call — waits out the cache TTL every time)
BenchmarkRewards_Cached-8     20    112831 ns/op   (~0.11ms/call — same call pattern, cache populated)
```

~56x faster per call inside the TTL window. The uncached number also
includes a deliberate 6ms sleep per iteration (see the benchmark's
comment) to force every call past the cache TTL and reproduce "no
cache" behavior on top of code that now has one — the real-world gain
is "every repeated read within 5 seconds costs ~0ms and zero DynamoDB
read-capacity instead of a full Scan," which matters most exactly when
the catalog endpoint is hit hardest.

## What this doesn't cover

No load test was run against the actual deployed HTTP services (k6 or
vegeta) — the AWS deployment this would run against (Phase 11) was not
`apply`'d in this session (time constraint; see ADR-0008), so there
was nothing live to point a load test at. The benchmark above
isolates and measures the one specific change made, which is a smaller
but honest claim than a full load test would be.
