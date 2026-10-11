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
| 2 | Concurrency (event processor) | ⬜ not started |
| 3 | First HTTP service (`player-service`) | ⬜ not started |
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
go test ./... -cover
go build -o bin/rewardctl.exe ./cmd/rewardctl
./bin/rewardctl.exe demo
```

PowerShell 5.1 doesn't support `&&`, so run the two lines separately (as above).

(`make build|test|vet|run` work the same if you have GNU Make installed.)

### Two things about this machine's Go setup

- **`go test` needs `GOTMPDIR` set outside `%LOCALAPPDATA%\Temp`.** A local
  Application Control policy blocks executables run from the default temp
  directory, which is where `go test` builds and runs its test binary. Fix:
  `export GOTMPDIR="$(pwd)/.gotmp"` before `go test` (the Makefile's `test`
  target already does this).
- **The installed Go toolchain is `windows/386` (32-bit)**, so `-race` (the
  data race detector) isn't available — it requires a 64-bit toolchain.
  This is fine through phase 1, but phase 2 (concurrency) leans on `-race`
  to catch real bugs, so switching to 64-bit Go before then is recommended.
