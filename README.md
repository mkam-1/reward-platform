# reward-platform

A mobile-gaming rewards/loyalty backend, built module by module to learn Go
from the fundamentals up to the level expected of a 2-3 year Go backend
engineer (microservices, AWS, Kubernetes, Terraform, SQL + NoSQL, testing,
CI/CD). The full curriculum lives in [docs/curriculum.md](docs/curriculum.md).

Domain: players earn points by playing games, and redeem points for rewards.
Real architectural decisions along the way are recorded as ADRs in
[docs/adr/](docs/adr/).

## Phase tracker

| # | Phase | Status |
|---|-------|--------|
| 0 | Setup | ✅ done |
| 1 | Domain modeling (`pkg/domain`, `cmd/rewardctl`) | ✅ done |
| 2 | Concurrency (event processor) | ✅ done |
| 3 | First HTTP service (`player-service`) | ✅ done |
| 4 | Relational DB (Postgres) | ✅ done |
| 5 | NoSQL DB (DynamoDB) | ✅ done |
| 6 | Split into microservices | ✅ done |
| 7 | Architecture & patterns | ✅ done |
| 8 | Testing depth | ✅ done |
| 9 | Observability | ✅ done |
| 10 | Containers & Kubernetes | ✅ done |
| 11 | AWS deployment (ECS/DynamoDB/CloudWatch) | ✅ code done, not `apply`'d |
| 12 | IaC (Terraform/Terragrunt) | ✅ done |
| 13 | CI/CD | ✅ done |
| 14 | Performance & scale (capstone) | ✅ done |

## Phase 6 architecture: three services, two ways of talking to each other

- **game-service** owns `Game`. Recording a session publishes a
  `SessionCompleted` event to SQS instead of crediting points directly —
  it has no access to the ledger at all anymore.
- **player-service** owns `Player` and the ledger. It consumes
  `SessionCompleted` (crediting points, idempotently) and exposes a small
  gRPC service (`GetBalance`, `DebitPoints`) for synchronous calls from
  rewards-service. After crediting, it writes a `PointsAwarded` event via
  the **outbox pattern** — the credit and the "publish this" record
  commit in one Postgres transaction, so a crash between committing and
  actually publishing can't silently lose the event. A separate relay
  loop (`internal/outbox`) polls that table and does the actual
  publishing.
- **rewards-service** owns `Reward` and `Redemption` (DynamoDB). Redeeming
  calls player-service's gRPC service synchronously — it needs a real
  answer (can this player afford it) before deciding whether the
  redemption succeeds, which is exactly the case gRPC fits and
  fire-and-forget SQS doesn't.

Async where eventual consistency is fine (crediting points from a
session), synchronous where it isn't (checking you can afford a
redemption *right now*) — not "gRPC vs SQS" as a style choice, but each
picked for what the specific interaction actually needs.

## Dev environment: WSL2 Ubuntu

Go development happens inside **WSL2 Ubuntu**, not native Windows. Reason:
this machine has Windows **Smart App Control** enabled, which blocks
unsigned binaries — including Go's own linker and everything `go build`
produces. That's unfixable short of disabling Smart App Control (a
one-way switch — re-enabling it later requires a full Windows reinstall),
so instead Go/gcc/Docker CLI are installed inside a WSL2 Ubuntu distro,
where Smart App Control doesn't apply at all (it only governs native
Windows binaries, not Linux ELF binaries). Docker Desktop already runs on
WSL2 as its own backend, so this adds one more distro alongside it, not a
new system-wide feature.

The repo lives at `/mnt/d/reward-platform` inside WSL — the same files as
`D:\reward-platform` on the Windows side, just accessed through WSL's
mount of the D: drive.

### Go workspace (`go.work`), since phase 6

The single root `go.mod` is gone — `pkg/domain`, `cmd/rewardctl`, `proto`
(the gRPC contract), `mq` (shared SQS client + event types), and each of
the three services are now **independent Go modules**, tied together
locally by `go.work`. Each service's own `go.mod` additionally carries
`replace` directives pointing at its sibling modules by relative path, so
it also builds standalone (no `go.work` needed) — which is exactly what
each service's Dockerfile relies on, copying in only the specific
siblings it actually needs rather than the whole repo.

One practical consequence: `go build ./...` from the repo root no longer
works (`go.work` requires module-path patterns, not relative ones). Use
the actual module path instead:

```
cd /mnt/d/reward-platform
go build reward-platform/pkg/domain/...
go vet reward-platform/pkg/domain/...
go test reward-platform/pkg/domain/... -race -cover
```

Repeat per module (`reward-platform/services/player-service/...`, `.../game-service/...`, `.../rewards-service/...`, `reward-platform/mq/...`, `reward-platform/cmd/rewardctl/...`) — or `cd` into that module's own
directory first and use plain `./...`, which works there since you're
inside a single module at that point.

(From a Windows terminal instead, prefix with `wsl.exe -d Ubuntu --`, e.g.
`wsl.exe -d Ubuntu -- go build -C /mnt/d/reward-platform reward-platform/pkg/domain/...`.)

## Quality tooling (phase 8)

```
# Lint every module (config: .golangci.yml at repo root)
bash scripts/lint-all.sh    # or: cd <module> && golangci-lint run ./...

# Coverage gate — fails if any module's real (unit-testable) coverage
# drops below the threshold; see the script's own comments for exactly
# which packages are deliberately excluded and why
bash scripts/check-coverage.sh 60

# Go's built-in fuzzer against domain validation (RegisterPlayer/Game/Reward)
cd pkg/domain
go test -run=^$ -fuzz=FuzzRegisterPlayer -fuzztime=30s
```

Contract tests for the SQS event schemas (`mq/contract_test.go`, golden
files in `mq/testdata/`) and a `gomock`-generated mock for
`rewards-service`'s `PlayerBalance` interface
(`services/rewards-service/internal/catalog/mocks/`, demonstrating
call-order verification `gomock.InOrder` gives that a hand-written fake
wouldn't without extra bookkeeping) run as part of the normal
`go test ./...` for their modules — nothing special needed.

## Running things

```
cd cmd/rewardctl
go build -o ../../bin/rewardctl .
cd ../..
./bin/rewardctl demo
./bin/rewardctl simulate -players 15 -games 3 -events 300 -workers 8
```

### Trying the full stack (player-service + game-service + rewards-service)

Seven containers now: Postgres (players/ledger), DynamoDB Local
(rewards/redemptions), ElasticMQ (SQS — `session-completed` and
`points-awarded` queues), Elasticsearch (log search), and the three Go
services. First time only:

```
cd deploy
cp .env.example .env
docker compose up --build -d
docker compose logs -f player-service   # watch migrations + queue setup
```

Player/game/reward registration and the balance check are still plain
HTTP, same as before:

```
curl -s http://localhost:8080/healthz          # player-service
curl -s http://localhost:8081/healthz          # game-service
curl -s http://localhost:8082/healthz          # rewards-service

curl -s -X POST http://localhost:8080/players -d '{"username":"alice"}'
curl -s -X POST http://localhost:8081/games -d '{"name":"dice-run","pointsPerSession":15}'
curl -s -X POST http://localhost:8082/rewards -d '{"name":"sticker-pack","costPoints":10,"stock":5}'
```

**Recording a session is now asynchronous** — it hits game-service, not
player-service, and returns 202 immediately (nothing in the response
body to show for it; the actual credit happens moments later once
player-service consumes the event):

```
curl -s -i -X POST http://localhost:8081/games/game_1/sessions -d '{"playerId":"player_1"}'
# ... give it a second for the SQS round trip ...
curl -s http://localhost:8080/players/player_1/balance
```

**Redeeming a reward now hits rewards-service**, which calls
player-service over gRPC behind the scenes to check/debit the balance:

```
curl -s -X POST http://localhost:8082/players/player_1/redemptions \
  -H "Idempotency-Key: demo-key-1" -d '{"rewardId":"reward_1"}'
curl -s http://localhost:8080/players/player_1/balance
curl -s http://localhost:8082/players/player_1/redemptions
```

Retry that exact `redemptions` POST with the same `Idempotency-Key` and
confirm the balance doesn't drop a second time.

Persistence still survives a restart of any one service — Postgres and
DynamoDB are unaffected by a service container restarting:

```
docker compose restart player-service
curl -s http://localhost:8080/players
```

Tear down when done:

```
docker compose down          # add -v too to also delete the Postgres volume
```

### Secrets

`deploy/.env` (gitignored) holds local config docker-compose loads
automatically — `deploy/.env.example` is the committed template. The
values in it are throwaway/dummy (a local-only Postgres password, and
AWS credentials DynamoDB Local doesn't even check but the SDK insists be
present). This is **not** where a real secret would go in production:
real AWS API access uses an IAM role attached to the ECS task (Phase 11)
with no stored credential at all, and an actual secret (a third-party
API key, say) would go in AWS Secrets Manager/Parameter Store, injected
into the task at runtime — never a file, never the image, never git.

### Integration tests (real Postgres / DynamoDB Local via testcontainers)

Excluded from the default `go test ./...` run (needs Docker, takes real
seconds) — run explicitly:

```
cd services/player-service
go test -tags=integration ./internal/postgres/... -v
cd ../rewards-service
go test -tags=integration ./internal/dynamodb/... -v
```

Inside WSL, testcontainers needs `DOCKER_HOST` pointed at the socket
Docker Desktop's WSL integration exposes (already set permanently in
`/etc/profile.d/go.sh` in this environment):

```
export DOCKER_HOST=unix:///mnt/wsl/docker-desktop-bind-mounts/Ubuntu/docker.sock
```

## Observability (phase 9)

Each service now exposes, alongside `/healthz` (liveness: is the process
up): `/readyz` (readiness: can it reach its real dependency — Postgres
for player-service, DynamoDB for rewards-service, nothing for
game-service which has none) and `/metrics` (Prometheus text format:
`http_requests_total` and `http_request_duration_seconds`, both labeled
by method/path/status).

```
curl -s http://localhost:8080/readyz
curl -s http://localhost:8080/metrics
```

### Log search (Elasticsearch)

Every service ships its structured logs to Elasticsearch (OpenSearch on
real AWS — see ADR-0009) in addition to stdout, via the shared
`logsink/` module. `ELASTICSEARCH_ENDPOINT` is optional — unset, a
service just logs to stdout exactly as before this existed.

```
curl -s "http://localhost:9200/logs-rewards-service-*/_search?q=level:ERROR&pretty"
```

## Kubernetes (phase 10)

`infra/k8s/` has plain manifests (no Helm) for the full six-container
stack — Postgres, DynamoDB Local, ElasticMQ, and the three services,
each with liveness/readiness probes wired to `/healthz`/`/readyz` and an
HPA scaling on CPU. Intended for a local cluster (kind/minikube), not a
real one — `imagePullPolicy: IfNotPresent` and `image:
reward-platform/<service>:latest` assume images already sit in the
cluster's own image store.

```
# Build the three service images, then load them into the cluster
# (kind) or point your shell at the cluster's own Docker daemon first
# (minikube docker-env) and build there instead.
docker build -t reward-platform/player-service:latest -f services/player-service/Dockerfile .
docker build -t reward-platform/game-service:latest -f services/game-service/Dockerfile .
docker build -t reward-platform/rewards-service:latest -f services/rewards-service/Dockerfile .
kind load docker-image reward-platform/player-service:latest reward-platform/game-service:latest reward-platform/rewards-service:latest

kubectl apply -k infra/k8s
kubectl -n reward-platform get pods -w
```

(Manifests are validated with `python3 -c "import yaml; ..."` in this
environment — no local kind/minikube cluster was stood up to `apply`
against, given the time constraint this session is under.)

## AWS deployment (phases 11-12)

`infra/terraform/` provisions the real-AWS equivalent of the Kubernetes
stack above: ECS Fargate (not EKS — see ADR-0008), the account's default
VPC (no NAT gateway, cost guardrail), DynamoDB, RDS Postgres, SQS, one
shared ALB with path-based routing, least-privilege IAM per service,
and a CloudWatch budget alarm. `infra/terragrunt/` wraps it to generate
per-environment remote state instead of hand-copying a backend block.

```
# One-time: creates the S3 bucket + DynamoDB lock table the main
# config's remote state needs to exist in the first place.
cd infra/terraform/bootstrap
terraform init && terraform apply

# Direct terraform (single "dev" state):
cd infra/terraform
terraform init -backend-config=backend.hcl
TF_VAR_db_password=... TF_VAR_alarm_email=... terraform plan

# Equivalently, via Terragrunt:
cd infra/terragrunt/dev
TF_VAR_db_password=... TF_VAR_alarm_email=... terragrunt plan
```

**Not run against real AWS this session** — validated with `terraform
validate` (both the bootstrap and main configs) and `terraform fmt`,
reviewed resource-by-resource, but never `apply`'d; see ADR-0008's
Consequences section for why and what's still open before it would be.

## CI/CD (phase 13)

`.github/workflows/ci-cd.yml`: lint → test → coverage gate → (build +
push images to ECR / `terraform plan`) → `terraform apply` → force a
new ECS deployment. The apply and deploy jobs sit behind a GitHub
Environment (`production`) with required reviewers configured in repo
settings — nothing reaches real infrastructure without a manual
approval click, even though the workflow itself triggers automatically
on every push to `main`. [docs/rollback.md](docs/rollback.md) covers
rolling back a bad deploy or a bad `apply`.

## Performance (phase 14)

pprof is wired into all three services on a loopback-only port (6061),
never exposed through the ALB or a k8s Service. rewards-service adds a
5-second read-through cache on the reward catalog listing and a per-IP
rate limiter on the redemption endpoint. [docs/perf-report.md](docs/perf-report.md)
has the before/after benchmark for the cache.

### If you're on native Windows instead

`make build|test|vet|run` work the same if you have GNU Make installed.
Two machine-specific things applied before the WSL2 move, kept here in
case native Windows Go ever comes back into play:

- **`go test` needs `GOTMPDIR` set outside `%LOCALAPPDATA%\Temp`** — Smart
  App Control blocks executables run from the default temp directory.
  Fix: `export GOTMPDIR="$(pwd)/.gotmp"` before `go test`.
- **`-race` needs cgo**, which needs a C compiler — a MinGW-w64 `gcc` is
  installed, and Go's `CC` env config points straight at it
  (`go env CC`), bypassing any PATH issues.
