#!/bin/bash
# Runs golangci-lint for every module (see .golangci.yml at the repo
# root for the shared config each one discovers automatically).
# Usage: scripts/lint-all.sh — run from anywhere, inside WSL.
export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin"
cd "$(dirname "$0")/.."

MODULES="pkg/domain cmd/rewardctl mq logsink proto services/player-service services/game-service services/rewards-service"

fail=0
for mod in $MODULES; do
  echo "=== linting $mod ==="
  if ! (cd "$mod" && golangci-lint run ./...); then
    fail=1
  fi
done
echo "=== done ==="
exit $fail
