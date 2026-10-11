# Multi-stage build: the "build" stage has the full Go toolchain and
# compiles the binary; only the compiled binary crosses into the final
# stage, so none of the toolchain, source code, or build cache ends up in
# the image that actually gets deployed.
FROM golang:1.27 AS build
WORKDIR /src

# Copying go.mod/go.sum before the rest of the source means `go mod
# download` only reruns when dependencies actually change, not on every
# code edit — Docker caches each layer and reuses it unchanged as long as
# these two files are the same.
COPY go.mod go.sum ./
RUN go mod download

COPY pkg ./pkg
COPY services/player-service ./services/player-service

# CGO_ENABLED=0 produces a fully static binary with no dynamic libc
# dependency — required for the distroless "static" base image below,
# which has no dynamic linker to satisfy a non-static binary's imports.
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/player-service ./services/player-service

# distroless/static has no shell, no package manager, no libc — just
# enough to run a single static binary. Smaller image, and there's
# nothing in it an attacker could get a shell into even if they found a
# way in.
FROM gcr.io/distroless/static-debian12
COPY --from=build /out/player-service /player-service
COPY services/player-service/migrations /migrations

ENV MIGRATIONS_DIR=/migrations
EXPOSE 8080
ENTRYPOINT ["/player-service"]
