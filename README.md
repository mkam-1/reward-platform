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
| 4 | Relational DB (Postgres) | ⬜ not started |
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

## Running things

```
go build ./...
go vet ./...
go test ./... -race -cover
go build -o bin/rewardctl.exe ./cmd/rewardctl
./bin/rewardctl.exe demo
./bin/rewardctl.exe simulate -players 15 -games 3 -events 300 -workers 8

go build -o bin/player-service.exe ./services/player-service
$env:PORT = "8080"
./bin/player-service.exe
```

PowerShell 5.1 doesn't support `&&`, so run the two lines separately (as above).

### Trying player-service

With the service running (see above), in another terminal:

```
curl -s http://localhost:8080/healthz
curl -s -X POST http://localhost:8080/players -d '{"username":"alice"}'
curl -s -X POST http://localhost:8080/games -d '{"name":"dice-run","pointsPerSession":15}'
curl -s -X POST http://localhost:8080/players/player_1/sessions -d '{"gameId":"game_1"}'
curl -s http://localhost:8080/players/player_1/balance
```

Or build and run it in Docker:

```
docker build -f services/player-service/Dockerfile -t reward-platform/player-service:dev .
docker run --rm -p 8080:8080 reward-platform/player-service:dev
```

(`make build|test|vet|run` work the same if you have GNU Make installed.)

### One thing about this machine's Go setup

- **`go test` needs `GOTMPDIR` set outside `%LOCALAPPDATA%\Temp`.** A local
  Application Control policy blocks executables run from the default temp
  directory, which is where `go test` builds and runs its test binary. Fix:
  `export GOTMPDIR="$(pwd)/.gotmp"` before `go test` (the Makefile's `test`
  target already does this).

(Go is now `windows/amd64` and a MinGW-w64 `gcc` is installed, so `-race`
works — run it from a fresh shell so it picks up the PATH change from the
`gcc` install.)
