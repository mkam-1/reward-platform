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
| 6 | Split into microservices | ⬜ not started |
| 7 | Architecture & patterns | ⬜ not started |
| 8 | Testing depth | ⬜ not started |
| 9 | Observability | ⬜ not started |
| 10 | Containers & Kubernetes | ⬜ not started |
| 11 | AWS deployment (ECS/DynamoDB/CloudWatch) | ⬜ not started |
| 12 | IaC (Terraform/Terragrunt) | ⬜ not started |
| 13 | CI/CD | ⬜ not started |
| 14 | Performance & scale (capstone) | ⬜ not started |

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

From a WSL Ubuntu terminal:

```
cd /mnt/d/reward-platform
go build ./...
go vet ./...
go test ./... -race -cover
```

(From a Windows terminal instead, prefix with `wsl.exe -d Ubuntu --`, e.g.
`wsl.exe -d Ubuntu -- go build -C /mnt/d/reward-platform ./...`.)

## Running things

```
go build -o bin/rewardctl ./cmd/rewardctl
./bin/rewardctl demo
./bin/rewardctl simulate -players 15 -games 3 -events 300 -workers 8
```

### Trying player-service (with real Postgres + DynamoDB Local)

player-service requires a real Postgres database (players, the ledger)
and a real DynamoDB table (rewards, redemptions) — `DATABASE_URL` and
`DYNAMODB_TABLE` are both required env vars, not defaulted, so missing
config fails loudly on startup instead of silently running in memory.

First time only: copy the env template (real values aren't needed here —
see "Secrets" below for why):

```
cd deploy
cp .env.example .env
```

Then bring up the whole stack (Postgres + DynamoDB Local + player-service):

```
docker compose up --build -d
docker compose logs player-service   # confirms migrations ran + table created
```

From any terminal:

```
curl -s http://localhost:8080/healthz
curl -s -X POST http://localhost:8080/players -d '{"username":"alice"}'
curl -s -X POST http://localhost:8080/games -d '{"name":"dice-run","pointsPerSession":15}'
curl -s -X POST http://localhost:8080/players/player_1/sessions -d '{"gameId":"game_1"}'
curl -s -X POST http://localhost:8080/rewards -d '{"name":"sticker-pack","costPoints":10,"stock":5}'
curl -s -X POST http://localhost:8080/players/player_1/redemptions \
  -H "Idempotency-Key: demo-key-1" -d '{"rewardId":"reward_1"}'
curl -s http://localhost:8080/players/player_1/balance
curl -s http://localhost:8080/players/player_1/redemptions
```

Retry that exact `redemptions` POST with the same `Idempotency-Key` and
confirm the balance doesn't drop a second time — that's the point of
phase 5's idempotency key, not just its DynamoDB persistence.

Prove persistence survives a restart (now covering rewards/redemptions
too, not just players/ledger):

```
docker compose restart player-service
curl -s http://localhost:8080/players   # player_1 is still there
curl -s http://localhost:8080/rewards   # sticker-pack is still there
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
go test -tags=integration ./services/player-service/internal/postgres/... -v
go test -tags=integration ./services/player-service/internal/dynamodb/... -v
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
