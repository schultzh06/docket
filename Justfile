set dotenv-load
set shell := ["bash", "-euo", "pipefail", "-c"]

gen_dirs := "server/gen server/internal/store/db apps/tui/src/gen"

version := `git describe --always --dirty`

default:
    @just --list

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

# refuse to proceed with uncommitted changes
clean-tree:
    @git diff --quiet HEAD || (echo "uncommitted changes; commit first" && exit 1)

# build and ship to the LXC
deploy: clean-tree build
    rsync bin/docket root@docket:/usr/local/bin/docket.new
    ssh root@docket 'chmod 755 /usr/local/bin/docket.new && mv /usr/local/bin/docket.new /usr/local/bin/docket && systemctl restart docket'

# regenerate all code from protos + SQL
gen:
    buf lint
    buf generate
    cd server && sqlc generate

# fail if regenerating would change any generated file
gen-check:
    #!/usr/bin/env bash
    set -euo pipefail
    snap() { find {{gen_dirs}} -type f -print0 | sort -z | xargs -0 sha256sum; }
    before=$(snap)
    just gen
    after=$(snap)
    if [[ "$before" != "$after" ]]; then
        diff <(echo "$before") <(echo "$after") || true
        echo "generated code was stale (now regenerated) — review and commit"
        exit 1
    fi

# everything CI runs
check: gen-check
    cd server && go vet ./... && go test ./...
    cd server && golangci-lint run
    cd apps/tui && pnpm typecheck