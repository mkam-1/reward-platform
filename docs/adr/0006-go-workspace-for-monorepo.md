# ADR 0006: `go.work` multi-module monorepo instead of one module

## Status
Accepted (phase 6)

## Context
Through phase 5, the whole project was one Go module. Splitting into
three independently deployable services (phase 6) raised a real
question: should they stay in one module (simpler tooling, but nothing
stops player-service's code from accidentally importing rewards-service
internals) or become genuinely separate modules (enforces the service
boundary at compile time, but more moving parts)?

## Decision
Each service, plus the pieces they share (`pkg/domain`, `proto`, `mq`),
is its own Go module with its own `go.mod`/`go.sum`. A root `go.work`
file ties them together for local development (`go build`/`go test`
resolve across all of them without needing to `go get` published
versions of each other). Each service's own `go.mod` additionally
carries `replace` directives pointing at its sibling modules by relative
path, so it *also* builds standalone without `go.work` present — which
is what each service's trimmed-context Dockerfile relies on.

## Consequences
- A service's `go.mod` import list is an honest, enforced declaration of
  what it actually depends on — rewards-service's `go.mod` has no way to
  accidentally end up depending on player-service's internals, because
  there's no module relationship between them at all (they don't import
  each other — only `pkg/domain`, `proto`, and `mq`, which are the
  genuinely shared pieces).
- `go build ./...` from the repo root no longer works — `go.work`
  requires module-path patterns (`go build reward-platform/pkg/domain/...`)
  or running the command from inside a specific module's own directory.
  Documented in the README as the one real ergonomic cost of this
  choice.
- Each Dockerfile copies in only the specific sibling modules a service
  needs (e.g. game-service's needs `pkg/domain` and `mq`, not `proto` —
  it has no gRPC) rather than the whole repo, keeping build contexts and
  images smaller — a direct benefit of the module boundary being real
  rather than just a folder convention.
- `go mod tidy` has to run per-module, and dependency versions (e.g.
  `google.golang.org/grpc`) can drift between modules that both use it,
  since there's no single shared `go.sum` forcing them to match. Worth
  watching if that ever causes a real incompatibility — hasn't yet.

## Alternatives considered
- **One module, `internal/` packages per service**: simpler tooling
  (one `go.mod`, plain `./...` works), but `internal/` visibility rules
  wouldn't actually stop one service's code from importing another's —
  they'd just be discouraged by convention, not enforced. Rejected,
  since enforcing the boundary was the whole point of splitting at all.
- **Fully separate repositories per service**: strongest possible
  boundary, but loses the ability to see and move across the whole
  system's code in one place, and adds real coordination overhead
  (versioning `pkg/domain` as a published dependency, cutting releases)
  that isn't justified at this project's scale.
