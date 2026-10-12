# Multi-stage build: the "build" stage has the full Go toolchain and
# compiles the binary; only the compiled binary crosses into the final
# stage, so none of the toolchain, source code, or build cache ends up in
# the image that actually gets deployed.
FROM golang:1.27 AS build
WORKDIR /src

# See game-service/Dockerfile's comment for why go.work isn't copied
# here: its "use" list requires every workspace module to be present,
# which a trimmed per-service build context deliberately doesn't have.
# player-service/go.mod carries its own "replace" directives instead,
# resolved against exactly these three siblings.
COPY pkg/domain ./pkg/domain
COPY mq ./mq
COPY proto ./proto
COPY services/player-service ./services/player-service

# CGO_ENABLED=0 produces a fully static binary with no dynamic libc
# dependency — required for the distroless "static" base image below,
# which has no dynamic linker to satisfy a non-static binary's imports.
# -C changes directory before resolving the module: the only go.mod in
# this build context is at services/player-service/go.mod (no go.mod at
# /src itself, deliberately — see the comment above), so Go has to be
# told where its module root actually is.
RUN CGO_ENABLED=0 GOOS=linux go build -C services/player-service -o /out/player-service .

# distroless/static has no shell, no package manager, no libc — just
# enough to run a single static binary. Smaller image, and there's
# nothing in it an attacker could get a shell into even if they found a
# way in.
FROM gcr.io/distroless/static-debian12
COPY --from=build /out/player-service /player-service
COPY services/player-service/migrations /migrations

ENV MIGRATIONS_DIR=/migrations
EXPOSE 8080
EXPOSE 9090
ENTRYPOINT ["/player-service"]
