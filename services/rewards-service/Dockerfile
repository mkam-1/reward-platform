FROM golang:1.27 AS build
WORKDIR /src

# See game-service/Dockerfile's comment: go.work is deliberately not
# copied here either, for the same reason. rewards-service additionally
# needs proto (the gRPC contract) alongside pkg/domain and mq.
COPY pkg/domain ./pkg/domain
COPY mq ./mq
COPY proto ./proto
COPY services/rewards-service ./services/rewards-service

# -C changes directory before resolving the module: the only go.mod in
# this build context is at services/rewards-service/go.mod, not at /src.
RUN CGO_ENABLED=0 GOOS=linux go build -C services/rewards-service -o /out/rewards-service .

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/rewards-service /rewards-service

EXPOSE 8080
ENTRYPOINT ["/rewards-service"]
