package main

import (
	"context"
	"errors"
	"fmt"
)

func syncOnce(ctx context.Context, cfg config) error {
	if cfg.CanvasURL == "" {
		return errors.New("DOCKET_CANVAS_ICS_URL not set")
	}
	conn, closeStore, err := openStore(ctx, cfg.DataDir)
	if err != nil {
		return err
	}
	defer closeStore()

	st, err := newCanvasSyncer(cfg, conn).Sync(ctx)
	if err != nil {
		return fmt.Errorf("canvas sync: %w", err)
	}
	fmt.Printf("%+v\n", st)
	return nil
}
