package main

import (
	"context"
	"log/slog"
	"os"
	"strings"

	ipfslog "github.com/ipfs/go-log/v2"
)

// LogFilterHandler wraps slog.Handler to suppress known noisy messages
// that are harmless on Windows with virtual network adapters.
type LogFilterHandler struct{ slog.Handler }

func (h *LogFilterHandler) Handle(ctx context.Context, r slog.Record) error {
	if strings.Contains(r.Message, "mdns: Failed to set multicast interface") {
		return nil
	}
	return h.Handler.Handle(ctx, r)
}

func setupLogging(headless bool) {
	opts := &slog.HandlerOptions{Level: slog.LevelDebug}
	base := slog.NewTextHandler(os.Stdout, opts)
	slog.SetDefault(slog.New(&LogFilterHandler{base}))
	ipfslog.SetLogLevel("mdns", "error")
	slog.Info("Starting Secure P2P System Backend", "headless", headless)
}
