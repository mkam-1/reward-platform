# reward-platform

A mobile-gaming rewards/loyalty backend, built module by module to learn Go
from the fundamentals up to the level expected of a 2-3 year Go backend
engineer (microservices, AWS, Kubernetes, Terraform, SQL + NoSQL, testing,
CI/CD). The full curriculum lives in [docs/curriculum.md](docs/curriculum.md).

Domain: players earn points by playing games, and redeem points for rewards.

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
| 7 | Architecture & patterns | ⬜ not started |
| 8 | Testing depth | ⬜ not started |
| 9 | Observability | ⬜ not started |
| 10 | Containers & Kubernetes | ⬜ not started |
| 11 | AWS deployment (ECS/DynamoDB/CloudWatch) | ⬜ not started |
| 12 | IaC (Terraform/Terragrunt) | ⬜ not started |
| 13 | CI/CD | ⬜ not started |
| 14 | Performance & scale (capstone) | ⬜ not started |

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

## Running things

```
cd cmd/rewardctl
go build -o ../../bin/rewardctl .
cd ../..
./bin/rewardctl demo
./bin/rewardctl simulate -players 15 -games 3 -events 300 -workers 8
```

### Trying the full stack (player-service + game-service + rewards-service)

Six containers now: Postgres (players/ledger), DynamoDB Local
(rewards/redemptions), ElasticMQ (SQS — `session-completed` and
`points-awarded` queues), and the three Go services. First time only:

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
