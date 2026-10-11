.PHONY: build test vet fmt run tidy

# A local Application Control policy on this machine blocks executables
# run from %LOCALAPPDATA%\Temp, which is where `go test`/`go run` build to
# by default. Redirecting GOTMPDIR into the repo works around it.
export GOTMPDIR := $(CURDIR)/.gotmp

build:
	go build ./...

test:
	@mkdir -p $(GOTMPDIR)
	go test ./... -cover

vet:
	go vet ./...

fmt:
	go fmt ./...

tidy:
	go mod tidy

run:
	go build -o bin/rewardctl.exe ./cmd/rewardctl
	./bin/rewardctl.exe demo
