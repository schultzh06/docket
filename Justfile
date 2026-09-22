set dotenv-load
set shell := ["bash", "-euo", "pipefail", "-c"]

version := `git describe --always --dirty`

default:
    @just --list

# regenerate code from protos
gen:
    buf lint
    buf generate

# run the daemon in the foreground
server:
    cd server && go run -ldflags "-X main.version={{version}}" ./cmd/docket

# run the TUI against DOCKET_URL
tui:
    cd apps/tui && pnpm dev

# build + start daemon, run TUI, tear down
dev:
    #!/usr/bin/env bash
    set -euo pipefail
    cd server && go build -ldflags "-X main.version={{version}}" -o ../bin/docket ./cmd/docket && cd ..
    ./bin/docket & pid=$!
    trap 'kill $pid 2>/dev/null; wait $pid 2>/dev/null' EXIT
    for _ in $(seq 50); do curl -s -o /dev/null "$DOCKET_URL" && break; sleep 0.1; done
    cd apps/tui && pnpm dev

# static linux binary for the LXC
build:
    cd server && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
      -ldflags "-s -w -X main.version={{version}}" -o ../bin/docket ./cmd/docket

# everything CI runs
check:
    buf lint
    buf generate && test -z "$(git status --porcelain -- server/gen apps/tui/src/gen)"
    cd server && go vet ./... && go test ./...
    cd server && golangci-lint run
    cd apps/tui && pnpm typecheck