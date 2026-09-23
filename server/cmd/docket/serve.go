package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	docketv1 "github.com/schultzh06/docket/server/gen/docket/v1"
	"github.com/schultzh06/docket/server/gen/docket/v1/docketv1connect"
	"github.com/schultzh06/docket/server/internal/canvas"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type server struct{}

func (s *server) GetStatus(
	ctx context.Context,
	req *connect.Request[docketv1.GetStatusRequest],
) (*connect.Response[docketv1.GetStatusResponse], error) {
	return connect.NewResponse(&docketv1.GetStatusResponse{
		Version:    version,
		ServerTime: timestamppb.Now(),
	}), nil
}

func requireBearer(token []byte, next http.Handler) http.Handler {
	errw := connect.NewErrorWriter()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || subtle.ConstantTimeCompare([]byte(got), token) != 1 {
			_ = errw.Write(w, r, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid or missing token")))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func serve(ctx context.Context, cfg config) error {
	if len(cfg.Token) < 32 {
		return errors.New("DOCKET_TOKEN missing or too short")
	}

	conn, closeStore, err := openStore(ctx, cfg.DataDir)
	if err != nil {
		return err
	}
	defer closeStore()

	mux := http.NewServeMux()
	path, handler := docketv1connect.NewDocketServiceHandler(&server{})
	mux.Handle(path, handler)

	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           requireBearer([]byte(cfg.Token), mux),
		Protocols:         protocols,
		ReadHeaderTimeout: 5 * time.Second,
	}

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		slog.Info("listening", "addr", cfg.Addr, "version", version)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		<-gctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	})

	if cfg.CanvasURL != "" {
		syncer := newCanvasSyncer(cfg, conn)
		g.Go(func() error {
			poll(gctx, "canvas", 20*time.Minute, syncer.Sync)
			return nil
		})
	} else {
		slog.Warn("DOCKET_CANVAS_ICS_URL not set; canvas sync disabled")
	}

	return g.Wait()
}

// poll runs fn immediately, then every interval, until ctx is cancelled.
// Failures are logged, not returned: one bad sync must not kill the daemon.
func poll(ctx context.Context, name string, every time.Duration, fn func(context.Context) (canvas.Stats, error)) {
	runOnce := func() {
		start := time.Now()
		st, err := fn(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return // shutting down; the error is just cancellation
			}
			slog.Error("sync failed", "source", name, "err", err)
			return
		}
		slog.Info("sync",
			"source", name,
			"not_modified", st.NotModified,
			"inserted", st.Inserted,
			"updated", st.Updated,
			"unchanged", st.Unchanged,
			"removed", st.Removed,
			"skipped", st.Skipped,
			"took", time.Since(start).Round(time.Millisecond),
		)
	}

	runOnce()
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runOnce()
		}
	}
}
