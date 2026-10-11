# Curriculum

This project is a learning path disguised as a real product: a mobile-gaming
rewards/loyalty backend (players, games, a points ledger, a rewards catalog,
redemptions), built phase by phase to go from Go fundamentals to the level
expected of a 2-3 year Go backend engineer — mapped directly to a real Golang
backend developer JD (microservices in Go, AWS ECS/DynamoDB/CloudWatch,
Kubernetes, Terraform/Terragrunt, relational + NoSQL databases, unit/
integration testing, CI/CD & DevOps, software architecture & design
patterns).

Infra choices: real AWS free tier via Terraform (not LocalStack) with cost
guardrails (destroy scripts, free-tier-eligible sizing, a budget alarm);
Docker Desktop for local Postgres/DynamoDB-local/compose and later
Kubernetes (kind/minikube).

The system starts as a single-module Go program (domain logic + CLI), grows
into an HTTP service, then is deliberately split into independently
deployable microservices (introducing `go.work`) once there's a concrete
reason to — mirroring how real systems evolve.

## Repo layout (grows over phases)

```
reward-platform/
  README.md                 curriculum tracker + how to run things
  docs/
    curriculum.md            this file
    adr/                     architecture decision records (from phase 7)
  pkg/                       shared library code (domain, events, logging, config)
  services/
    player-service/          owns players + points ledger
    rewards-service/          owns reward catalog + redemptions
    game-service/              emits play-session events
  cmd/
    rewardctl/                 CLI used in early phases to exercise domain code
  deploy/
    docker-compose.yml
  infra/
    terraform/
    terragrunt/
    k8s/
  .github/workflows/
  go.work                    introduced in phase 6 when services split into modules
```

## Phases

| # | Phase | Concepts | Deliverable | JD tie-in |
|---|-------|----------|--------------|-----------|
| 0 | Setup | modules, project layout, `go fmt/vet`, Makefile, git | Runnable repo skeleton | baseline tooling |
| 1 | Domain modeling | structs/methods, interfaces, embedding, generics, error wrapping (`errors.Is/As`, `%w`), custom errors, table-driven tests | `pkg/domain`: Player, Game, PointsLedger, Reward, Redemption (in-memory) + `cmd/rewardctl` CLI, full unit tests | Go fundamentals |
| 2 | Concurrency | goroutines, channels, `select`, `sync.Mutex/WaitGroup`, `sync/atomic`, worker pools, `context` cancellation, `-race` | Concurrent in-memory event processor safely updating the ledger | "robust, efficient" Go |
| 3 | First HTTP service | `net/http` routing, middleware, `log/slog`, JSON, env config, graceful shutdown, handler→service→repo layering | `player-service`: REST CRUD + balance endpoint, Dockerfile | microservices in Go |
| 4 | Relational DB | database/sql + pgx, migrations, repository pattern, transactions, testcontainers-go | player-service persists to Postgres via docker-compose | relational databases |
| 5 | NoSQL DB | AWS SDK v2, DynamoDB single-table design, GSIs, idempotent writes, DynamoDB Local | Rewards data layer on DynamoDB | NoSQL databases |
| 6 | Split into microservices | `go.work`, module boundaries, gRPC + protobuf, AWS SQS, outbox pattern, idempotency keys | `rewards-service` + `game-service` become real services with an event flow between them | Microservices architecture |
| 7 | Architecture & patterns | hexagonal/clean architecture, dependency inversion, strategy/decorator/functional-options, retry/circuit-breaker, ADRs | Refactor pass + ADRs | software architecture & design patterns |
| 8 | Testing depth | mocking, contract tests, testcontainers, coverage gates, fuzzing, golangci-lint | CI-ready test suite across services | unit and integration testing |
| 9 | Observability | Prometheus, OpenTelemetry, health/readiness probes, CloudWatch, Elasticsearch | `/healthz`/`/readyz`, metrics, ES log index, CloudWatch alarms | CloudWatch, ElasticSearch |
| 10 | Containers & Kubernetes | multi-stage Dockerfiles, docker-compose, k8s Deployments/Services/ConfigMaps/Secrets/Ingress/HPA/probes | Full platform on local kind/minikube | Kubernetes |
| 11 | AWS deployment | ECR, ECS Fargate, real DynamoDB, CloudWatch, IAM, VPC, ALB, Secrets Manager | Services on ECS Fargate (AWS free tier) + teardown script | ECS, DynamoDB, CloudWatch |
| 12 | IaC | Terraform modules, Terragrunt, remote state (S3 + DynamoDB lock) | `infra/` reproduces phase 11 infra via plan/apply/destroy | Terraform, Terragrunt |
| 13 | CI/CD | GitHub Actions: lint→test→build→push→terraform→deploy | `.github/workflows/` pipeline + rollback doc | CI/CD and DevOps practices |
| 14 | Performance & scale (capstone) | pprof, load testing (k6/vegeta), Redis cache, rate limiting | Before/after perf report, cached leaderboard endpoint | performance and scalability |

## Verification per phase

- `go build ./...`, `go vet ./...`, `go test ./... -race -cover`
- Phase 4/5+: integration tests against dockerized Postgres / DynamoDB Local
- Phase 10+: manual smoke test against deployed endpoints (local k8s, then AWS)
- Phase 12+: `terraform plan` reviewed before every `apply`; `destroy` verified to fully tear down
