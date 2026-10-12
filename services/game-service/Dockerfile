FROM golang:1.27 AS build
WORKDIR /src

# Deliberately NOT copying go.work: its "use" list references every
# module in the workspace (including ones this image has no reason to
# build, like rewards-service), and go.work requires all of them to
# exist wherever it's found. Instead, game-service/go.mod carries its
# own "replace" directives pointing at these same relative paths — the
# same trick cmd/rewardctl's go.mod uses — so it builds standalone as
# long as these specific siblings are present, nothing more.
COPY pkg/domain ./pkg/domain
COPY mq ./mq
COPY services/game-service ./services/game-service

# -C changes directory before resolving the module: the only go.mod in
# this build context is at services/game-service/go.mod, not at /src.
RUN CGO_ENABLED=0 GOOS=linux go build -C services/game-service -o /out/game-service .

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/game-service /game-service

EXPOSE 8080
ENTRYPOINT ["/game-service"]
