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
| 5 | NoSQL DB (DynamoDB) | ⬜ not started |
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

### Trying player-service (with real Postgres)

player-service requires a real Postgres database — `DATABASE_URL` is a
required env var, not defaulted, so a missing connection string fails
loudly on startup instead of silently running in memory. Easiest way to
run the whole stack (Postgres + player-service) together:

```
cd deploy
docker compose up --build -d
docker compose logs player-service   # confirms migrations ran
```

Then, from any terminal:

```
curl -s http://localhost:8080/healthz
curl -s -X POST http://localhost:8080/players -d '{"username":"alice"}'
curl -s -X POST http://localhost:8080/games -d '{"name":"dice-run","pointsPerSession":15}'
curl -s -X POST http://localhost:8080/players/player_1/sessions -d '{"gameId":"game_1"}'
curl -s http://localhost:8080/players/player_1/balance
```

The actual point of this phase — prove it's really persisted, not just
in memory:

```
docker compose restart player-service
curl -s http://localhost:8080/players   # player_1 is still there
```

Tear down when done:

```
docker compose down          # add -v too to also delete the Postgres volume
```

### Integration tests (real Postgres via testcontainers)

Excluded from the default `go test ./...` run (needs Docker, takes real
seconds) — run explicitly:

```
go test -tags=integration ./services/player-service/internal/postgres/... -v
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
