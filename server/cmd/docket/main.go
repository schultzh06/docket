package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	docketv1 "github.com/schultzh06/docket/server/gen/docket/v1"
	"github.com/schultzh06/docket/server/gen/docket/v1/docketv1connect"
)

// Overridden at build time: go build -ldflags "-X main.version=$(git describe --always --dirty)"
var version = "dev"

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

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	token := os.Getenv("DOCKET_TOKEN")
	if len(token) < 32 {
		log.Error("DOCKET_TOKEN missing or too short")
		os.Exit(1)
	}
	addr := os.Getenv("DOCKET_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}

	mux := http.NewServeMux()
	path, handler := docketv1connect.NewDocketServiceHandler(&server{})
	mux.Handle(path, handler)

	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true) // h2c; Tailscale provides the encryption

	srv := &http.Server{
		Addr:              addr,
		Handler:           requireBearer([]byte(token), mux),
		Protocols:         protocols,
		ReadHeaderTimeout: 5 * time.Second,
		// No WriteTimeout: it would kill WatchUpdates streams later.
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("listening", "addr", addr, "version", version)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	log.Info("stopped")
}
