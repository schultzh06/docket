# docket

A self-hosted, single-user academic dashboard. docket pulls from Canvas, email, and (later) calendars, stores everything in SQLite, will summarize and triage with a local LLM, and presents a merged agenda and email digest in a terminal UI.

It runs on a Proxmox homelab, is reachable only over Tailscale, and keeps all data and inference local. It is also a deliberate exercise in backend and security design: every boundary is enforced in code, every decision below has a reason, and the known gaps are written down.

**Non-goals:** multi-user support, public hosting, open-source distribution, a mobile app.

## Status

| Phase | Scope | State |
|---|---|---|
| 0 | Integration spike: prove every data path | Done |
| 1 | Skeleton: proto contract, codegen, daemon, TUI, CI, LXC deploy | Done |
| 2 | Canvas ICS ingestion into SQLite, incremental sync, poller | Done |
| 3 | Email ingestion (Fastmail JMAP), deterministic, no LLM | **Next** |
| 4 | Agenda and messages API, TUI panels, mark-done, backups | Planned |
| 5 | LLM layer and eval set | Planned |
| 6 | Email digest | Planned |
| 7+ | Fastmail calendar, proposed items, approved writes, agent mode | Planned |

See [Roadmap](#roadmap) for detail.

## Architecture

```
                       ┌──────────── homelab (Proxmox) ─────────────┐
 Canvas ICS feed ──┐   │  ┌─ docket LXC ─────────────┐  ┌─ Ollama LXC ─┐
 Fastmail JMAP ────┼──────▶│ docket daemon (Go)       │  │ qwen2.5 7B   │
 (WPI + Gmail      │   │  │  pollers ─▶ SQLite (WAL) │─▶│ RTX 2060 6GB │
  redirected in)   │   │  │  Connect-RPC API         │  └──────────────┘
                   │   │  └───────────▲──────────────┘                  │
                   │   └──────────────│─────────────────────────────────┘
                   │                  │ Tailscale + bearer token
                   │           ┌──────┴───────┐
                   │           │ Ink TUI (TS) │  laptop
                   │           └──────────────┘
```

The daemon is one static Go binary. Pollers fetch each source on an interval and write to SQLite. The TUI (and later a web UI) talks to the daemon over Connect-RPC. The LLM runs in a separate container and will be called only by the daemon.

## Design principles

1. **Deterministic by default, LLM where judgment is needed.** Fetching, scheduling, storage, and change detection are plain code. The model will only summarize, categorize, and extract deadlines from unstructured text.
2. **The reader has no tools.** The model that reads raw email bodies outputs schema-validated JSON and can call nothing.
3. **Code enforces every boundary.** Model output is validated before any action. All writes require approval in the TUI and are recorded in an audit log.
4. **Start read-only.** Write capability arrives only with the approval flow.
5. **Everything is measurable.** A hand-labeled eval set scores the model, so prompt and model changes are judged on numbers.

## Security

### Threat model

The assets are API credentials (Fastmail token, Canvas feed URL, the docket API token), email contents, and Canvas data. **Every email body and every Canvas announcement is untrusted input.** The primary risk is prompt injection once an LLM reads that input; the secondary risk is credential theft from the daemon host.

### Mitigations in place

The API listens only on the Tailscale interface and is not reachable from the LAN. Every request passes a bearer-token check that uses `crypto/subtle.ConstantTimeCompare`, and the middleware wraps the whole router, so no RPC can be added without auth. Dev and prod use separate tokens.

Secrets live in `/etc/docket/docket.env`, owned by root with mode 0600. systemd reads it as root before dropping privileges, so the `docket` service user receives the values but cannot read the file from disk. The service runs as an unprivileged system user in an unprivileged LXC, separate from the Ollama container, with systemd sandboxing: `ProtectSystem=strict`, `NoNewPrivileges`, an empty capability bounding set, restricted address families, `MemoryDenyWriteExecute`, and a private `/tmp`. The only writable path is its own state directory (mode 0700).

Credentials are scoped as narrowly as the providers allow: the Fastmail token is read-only and Email-only; the Outlook calendar is published at titles-and-locations only. The Canvas and calendar ICS URLs are bearer credentials in their own right, so the fetcher redacts URLs from `*url.Error` values before anything is logged. Otherwise the first network blip would write the secret into journald.

HTTP clients always set timeouts and read response bodies through `io.LimitReader`, so a hostile or broken server cannot hang a goroutine or exhaust memory. SSH to the LXC is key-only, and `deploy` refuses to ship from a dirty git tree.

### Planned mitigations

The email-reading model gets no tools and a grammar-constrained JSON schema. Agent mode (Phase 9) gets a small allowlist of read-only tools plus `propose_item`, a step budget, full traces, and adversarial injection emails in the eval set as regression tests. Every write goes through human approval and the audit log. When calendar write access arrives, the credential will likely be read-write at the provider level; the daemon will contain no write code until the approval flow exists.

## Data sources

| Source | Access | State |
|---|---|---|
| Canvas | Calendar ICS feed | Ingesting (Phase 2) |
| Email (Fastmail, plus WPI Outlook and Gmail redirected to per-source aliases) | JMAP, read-only Email token | Verified; ingestion is Phase 3 |
| Fastmail calendar | CalDAV or JMAP (to be confirmed) | Planned; also the Phase 7 write target |
| WPI Outlook calendar | Published ICS | Verified; deferred |

WPI requires admin approval for third-party Microsoft Graph apps, so Graph access is blocked. Mail reaches docket through server-side redirects into Fastmail instead. Redirect (not forward) rules keep the original `From` header, and a separate alias per upstream source makes the message's origin a property of its delivery address rather than a guess from headers.

The Canvas ICS feed lacks submission status, announcements, and undated assignments. docket compensates with a manual `done` status; announcements will arrive through Canvas email notifications routed to Fastmail. If the API token is approved, an API-backed source replaces the ICS one behind the same interface.

## Stack

| Layer | Choice |
|---|---|
| Daemon | Go, stdlib `net/http`, `log/slog` |
| API | Connect-RPC (`connect-go`, `connect-es`), protobuf managed by `buf` |
| Storage | SQLite via `modernc.org/sqlite` (pure Go), `goose` migrations, `sqlc` queries |
| TUI | Ink (React for terminals), TypeScript |
| LLM | Ollama, `qwen2.5:7b-instruct-q4_K_M` |
| Transport | Tailscale |
| Build and CI | `just`, GitHub Actions, `golangci-lint` |

## Repository layout

```
proto/docket/v1/         API contract (the source of truth for server and client)
server/
  cmd/docket/            main, config, subcommands (serve, sync)
  internal/
    ics/                 generic ICS fetch (conditional GET) and parse
    canvas/              Canvas-specific mapping and the sync pipeline
    store/               SQLite open, pragmas, embedded migrations
      migrations/        goose SQL migrations
      queries/           sqlc query definitions
      db/                sqlc output (generated)
  gen/                   protobuf and Connect output (generated)
apps/tui/                Ink TUI; src/gen is generated
Justfile                 every build, check, and deploy command
```

Generated code is committed, so the Go and TypeScript builds need no generators installed. CI verifies that regenerating produces no changes.

## Key decisions

**Go for the daemon.** It gives breadth beyond TypeScript and has a strong stdlib for HTTP and crypto. Goroutines also fit the workload well: pollers, a GPU worker, and a server run concurrently in one process. It builds to a single static binary, so deployment is copying one file.

**Connect-RPC over REST.** The API is written once in protobuf and generated into a Go handler interface and a typed TS client, so the compiler catches contract drift on both sides. Connect runs over plain HTTP (unary calls are `POST` with JSON or protobuf), which keeps curl debugging and a future browser client simple. `buf breaking` checks wire compatibility on pull requests. Proto field numbers are never reused or renumbered.

**SQLite with deliberate settings.** One process owns the database, so a server database adds nothing but operational cost. Every connection applies `journal_mode=WAL` (readers never block the writer), `foreign_keys=ON` (off by default in SQLite), `busy_timeout=5000`, and `synchronous=NORMAL`, via the DSN so every pooled connection gets them. Transactions use `_txlock=immediate` to avoid the read-then-upgrade deadlock. Tables are `STRICT`. Timestamps are `INTEGER` Unix seconds in UTC, because ISO-8601 text only sorts correctly when every value has an identical format. `CHECK` constraints stand in for enums.

**Migrations embedded in the binary.** goose migrations are compiled in with `go:embed` and applied at startup, so the daemon migrates its own database and deploy stays a single-file copy. Applied migrations are never edited; changes are new migrations.

**sqlc instead of an ORM.** Queries are real SQL, and sqlc generates typed Go functions from the schema and queries, so a reference to a missing column fails at build time. The generated code accepts either the pool or a transaction.

**Sync is three layers.** Fetch (network: timeouts, conditional GET, status codes), parse (a pure function tested against fixture files), and persist (change detection inside one transaction). The generic ICS package is shared; source-specific interpretation, such as Canvas course tags in titles, lives in a thin mapping layer.

**Two layers of change detection.** First, a conditional GET or an identical body hash skips the sync entirely. Many calendar servers stamp every response with a fresh `DTSTAMP`, which defeats the body hash, so each item also gets a hash of only its meaningful fields, and unchanged items are not rewritten. The item hash covers source values (such as the course code), never internal database IDs.

**Decide in Go, not in one clever upsert.** Each event is looked up by `(source, source_id)` and inserted, updated, or skipped by explicit Go logic. That costs an extra query per event, which is negligible because SQLite is in-process. The rules stay readable and testable.

**Manual status survives syncs.** Updates rewrite source fields but never `status`, except to revive an item marked `removed` that reappears. A `done` mark survives a deadline change.

**Removal is windowed, and anomalous input cannot mass-delete.** An item missing from a feed is marked `removed` (never hard-deleted) only if it falls on or after the feed's earliest item, since older items may simply have aged out of the feed. An empty feed skips the removal pass entirely. *Known gap:* if the single earliest item is deleted upstream, the window shrinks past it and the deletion goes undetected.

**Idempotent by design.** Sync state is saved after the data transaction commits. If that save fails, the next run re-applies the feed and every item comes back unchanged, so no two-phase protocol is needed.

**Timezones are explicit.** All-day dates are interpreted in the configured zone (`DOCKET_TZ`), because `2026-10-03` means that day where the user lives, not midnight UTC. The IANA timezone database is embedded (`time/tzdata`) so minimal containers need no zoneinfo files. Everything is stored in UTC and converted only at display time.

**Structured process lifecycle.** `main` only calls `run`, which returns errors instead of exiting, so deferred cleanup (closing SQLite, checkpointing the WAL) always runs. The HTTP server and pollers run in an `errgroup`: a fatal error in one cancels the others and exits for systemd to restart, while a failed sync is logged and retried on the next tick. Shutdown on SIGTERM is graceful.

## Development

### Prerequisites

Go 1.24+, Node with pnpm, `buf`, `sqlc`, `just`, and `golangci-lint`.

### Configuration

The daemon is configured entirely through environment variables, read in one place (`loadConfig`). In development, `just` loads them from a gitignored `.env`; see `.env.example`.

| Variable | Used by | Purpose |
|---|---|---|
| `DOCKET_TOKEN` | daemon, TUI | API bearer token (`openssl rand -hex 32`); separate for dev and prod |
| `DOCKET_ADDR` | daemon | Listen address; defaults to `127.0.0.1:8080`, set to the Tailscale IP in prod |
| `DOCKET_URL` | TUI | Daemon URL |
| `DOCKET_CANVAS_ICS_URL` | daemon | Canvas calendar feed; a secret |
| `DOCKET_TZ` | daemon | IANA zone for all-day dates; defaults to `America/New_York` |
| `STATE_DIRECTORY` | daemon | Data directory; set by systemd, defaults to `./data` in dev |

### Commands

```bash
just gen        # lint protos, regenerate protobuf/Connect and sqlc code
just server     # run the daemon
just sync       # one-shot Canvas sync against the dev database
just tui        # run the TUI against DOCKET_URL
just dev        # build and start the daemon, run the TUI, tear down
just check      # everything CI runs
```

`just check` includes `gen-check`, which fingerprints the generated directories, regenerates, and fails if anything changed. That makes it correct whether or not the working tree is committed.

### Continuous integration

GitHub Actions runs three jobs on GitHub-hosted runners with a read-only `GITHUB_TOKEN`. The codegen job lints protos, runs `buf breaking` on pull requests, and runs `just gen-check` with sqlc pinned to the local version. The Go job runs `go vet`, `go test`, and `golangci-lint`. The TUI job runs `tsc --noEmit` against a frozen lockfile. Proto changes go through pull requests so breaking-change detection applies.

## Deployment

The daemon runs in its own unprivileged Debian LXC as a systemd service, with the TUN device passed through for Tailscale. `just deploy` refuses a dirty tree, builds a static stripped binary with the git version embedded, uploads it beside the live one, atomically renames it into place, and restarts the service. The unit uses `StateDirectory=docket` for `/var/lib/docket` and restarts on failure, which also covers the Tailscale IP not yet being assigned at boot.

## Roadmap

**Phase 3: Email ingestion.** JMAP `Email/query` and `Email/changes` with the state string stored for incremental sync; the source tagged from the delivery alias; HTML-to-text conversion, quote and signature stripping, and per-message token caps. Deterministic only. This is where untrusted email content first enters the database.

**Phase 4: API and TUI.** `GetAgenda` (time range, course filter, pagination), `ListCourses`, a messages list, and `GetStatus` extended with per-source sync health. Ink panels for today, this week, recent mail, and status. `SetItemStatus` for mark-done: the first local write. Because manual state cannot be rebuilt from sources, nightly `VACUUM INTO` backups to B2 ship in the same phase. TUI config moves to `~/.config/docket/` with a 0600 token file.

**Phase 5: LLM layer and eval set.** An Ollama wrapper with one function per task (`Summarize`, `Categorize`, `ExtractDeadline`), each with its own prompt and JSON schema. Every request sets `num_ctx` explicitly (target 8192), passes a schema in `format`, sends an explicit system message, and uses map-reduce (one email per call) through a single serialized GPU worker. Results store `model` and `prompt_version` so outputs can be recomputed when either changes. The eval set is 20 to 30 hand-labeled messages drawn from real ingested mail; `docket eval` prints accuracy.

**Phase 6: Email digest.** Summarize and course-tag new mail on a schedule and surface the digest in the TUI.

**Phase 7: Fastmail calendar.** Read access first (CalDAV or JMAP, to be confirmed), including recurrence expansion into instances with stable per-instance IDs. The shared sync pipeline gets extracted from `canvas.Syncer` once there are two concrete sources. Do not subscribe the Fastmail calendar to the Canvas feed, or docket will read assignments twice.

**Phase 8: Proposed items and approved writes.** `ExtractDeadline` writes candidates to `pending_actions`, deduplicated against Canvas; the TUI shows each beside its source email for approve, edit, or reject. Approved items become Fastmail calendar events, and every write is audited.

**Phase 9: Agent mode.** A router first (the model picks one intent from a fixed enum; code executes it), then a bounded tool loop with a read-only allowlist plus `propose_item`, a step budget, full traces, and injection regression tests.

**Later.** `WatchUpdates` server streaming with in-process pub/sub, a web UI as a second Connect client, the Outlook calendar (which will need Windows-to-IANA timezone mapping), GitHub notifications, Wazuh alerts, and benchmarking newer models against the eval set.

## Open questions

Does Fastmail expose calendars over JMAP to API tokens, or is CalDAV the only route? Does a CalDAV app password come in a read-only form? Will WPI approve the Canvas API token? Should deduplication (once there are overlapping sources) be done at read time or through a `duplicate_of` link? Rows from different sources are never merged, since that destroys the provenance re-syncing depends on.