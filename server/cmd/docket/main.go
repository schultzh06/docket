package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
	_ "time/tzdata" // embed the timezone database; minimal LXCs may lack /usr/share/zoneinfo

	"github.com/schultzh06/docket/server/internal/canvas"
	"github.com/schultzh06/docket/server/internal/ics"
	"github.com/schultzh06/docket/server/internal/store"
)

// Overridden at build time via -ldflags "-X main.version=..."
var version = "dev"

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err := run(os.Args[1:]); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	switch cmd {
	case "serve":
		return serve(ctx, cfg)
	case "sync":
		return syncOnce(ctx, cfg)
	default:
		return fmt.Errorf("unknown command %q (want: serve, sync)", cmd)
	}
}

type config struct {
	DataDir   string
	CanvasURL string
	Location  *time.Location
	Addr      string
	Token     string
}

func loadConfig() (config, error) {
	cfg := config{
		DataDir:   envOr("STATE_DIRECTORY", "./data"),
		CanvasURL: os.Getenv("DOCKET_CANVAS_ICS_URL"),
		Addr:      envOr("DOCKET_ADDR", "127.0.0.1:8080"),
		Token:     os.Getenv("DOCKET_TOKEN"),
	}
	loc, err := time.LoadLocation(envOr("DOCKET_TZ", "America/New_York"))
	if err != nil {
		return config{}, fmt.Errorf("DOCKET_TZ: %w", err)
	}
	cfg.Location = loc
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// openStore: returns the pool plus a cleanup func for the caller to defer.
func openStore(ctx context.Context, dataDir string) (*sql.DB, func(), error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, nil, fmt.Errorf("create data dir: %w", err)
	}
	conn, err := store.Open(ctx, filepath.Join(dataDir, "docket.db"))
	if err != nil {
		return nil, nil, fmt.Errorf("open store: %w", err)
	}
	closeFn := func() {
		if err := conn.Close(); err != nil {
			slog.Error("close store", "err", err)
		}
	}
	return conn, closeFn, nil
}

func newCanvasSyncer(cfg config, conn *sql.DB) *canvas.Syncer {
	return &canvas.Syncer{
		DB:      conn,
		Fetcher: ics.NewFetcher(),
		FeedURL: cfg.CanvasURL,
		Loc:     cfg.Location,
		Now:     time.Now,
	}
}
