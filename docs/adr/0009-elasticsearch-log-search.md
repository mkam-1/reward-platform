# 0009: Structured logs also indexed into Elasticsearch/OpenSearch

## Context

Every service already writes structured JSON logs to stdout (`log/slog`),
which CloudWatch Logs (real AWS) and `kubectl logs`/`docker compose logs`
(local) pick up unchanged. That's sufficient for "what did this one
request do" when you already know which service and roughly when to
look. It's not sufficient for "find every failed redemption for player
X across all three services in the last hour" — CloudWatch Logs Insights
can query within one log group reasonably well, but cross-service,
free-text, ad-hoc search across arbitrary fields is exactly what
Elasticsearch/OpenSearch is built for, and the JD for this role names it
specifically as one of the backend architecture technologies in scope
("...AWS Cloud Technologies, Kubernetes, Go Microservices and
ElasticSearch").

## Decision

- A new shared module, `logsink/`, wraps the official Elasticsearch Go
  client and exposes a `slog.Handler` (`logsink.Handler`) that **wraps**
  each service's existing stdout JSON handler rather than replacing it —
  every log line still goes to stdout (and from there, CloudWatch) as
  before; indexing into Elasticsearch is additive.
- Shipping is asynchronous and best-effort: `Handle` pushes the log
  document onto a bounded channel and returns immediately; a background
  goroutine does the actual indexing. A full buffer drops the newest
  document rather than blocking the request that triggered the log line
  — the same reasoning the outbox pattern (ADR-0004) applies to a
  different problem: observability must never become a tax on the
  request path it's trying to observe.
- `ELASTICSEARCH_ENDPOINT` is optional, not required — unset, a service
  behaves exactly as it did before this ADR (stdout-only). This matters
  for `go test` (no test wires a real Elasticsearch endpoint) and for
  any environment that hasn't opted in yet.
- **Local dev runs plain Elasticsearch** (docker-compose, Kubernetes) —
  single node, security disabled, the same "minimal local emulator"
  treatment DynamoDB Local and ElasticMQ already get.
- **Real AWS runs Amazon OpenSearch Service**, not self-managed
  Elasticsearch on EC2 — OpenSearch is AWS's managed fork, API-compatible
  with the Elasticsearch client this project already uses, so no code
  difference exists between the two beyond which URL `ELASTICSEARCH_ENDPOINT`
  points at (the same "endpoint swap, no code change" shape as every
  other local-emulator-vs-real-AWS pair in this project — DynamoDB
  Local/DynamoDB, ElasticMQ/SQS).
- Network access to the OpenSearch domain is controlled by a security
  group scoped to the ECS tasks' own SG, not IAM request signing — the
  plain Elasticsearch client this project uses doesn't speak AWS SigV4,
  and introducing that would mean either a different client library or
  hand-rolling signing, neither of which earns its complexity here.

## Consequences

- One more moving part (and, on real AWS, one more billed resource —
  OpenSearch isn't covered by the same 12-months-free tier RDS/DynamoDB
  get past a brand-new account) for a capability (log search) that's
  genuinely "nice to have, not load-bearing": nothing in the request
  path depends on Elasticsearch being up. That asymmetry is intentional
  — see the async/best-effort shipping decision above.
- Log documents currently only capture each `Handle` call's own attrs,
  not ones attached earlier via `logger.With(...)` — see logsink/handler.go's
  doc comment. Every call site in this project already passes its
  identifying fields (request_id, player_id, etc.) as direct `Info`/`Error`
  arguments rather than via `.With(...)`, so this doesn't lose anything
  today, but it's a real gap if that convention changes later.
- No index lifecycle management (ILM) policy is configured — `logs-<service>-YYYY.MM.DD`
  indices accumulate forever on a real deployment. A production rollout
  would add an ILM policy (e.g., delete after 30 days) before this ships
  for real; not implemented here as it's an operational follow-up, not
  something that changes the application code's shape.

## Alternatives considered

- **CloudWatch Logs Insights only, no Elasticsearch at all**: simplest,
  and arguably sufficient for this project's actual traffic — rejected
  specifically because the JD names Elasticsearch as an explicit skill
  area, not because CloudWatch Insights is technically inadequate for a
  system this size.
- **Ship via a log-forwarding sidecar (Filebeat/Fluent Bit) instead of
  an in-process handler**: the more common production pattern (no
  application code needs to know about Elasticsearch at all), and
  genuinely better separation of concerns. Rejected for this project
  specifically because the point is demonstrating hands-on Go/Elasticsearch
  client code, not just operational log plumbing — a sidecar would
  satisfy "logs end up in Elasticsearch" without demonstrating anything
  about working with the client itself.
