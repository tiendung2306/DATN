package main

import (
	"context"
	"embed"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"app/adapter/wailsui"
	"app/cli"
	"app/config"
	applog "app/pkg/log"
	"app/service"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	cfg := config.Parse()
	applog.Setup(cfg.Headless)

	if cfg.Headless && cfg.ControlPort > 0 && !cfg.IsCommand() {
		runManagedHeadless(cfg)
		return
	}

	if cfg.Headless || cfg.IsCommand() {
		if err := cli.Run(cfg); err != nil {
			slog.Error("Fatal error", "error", err)
			os.Exit(1)
		}
		return
	}

	dist, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		slog.Error("frontend assets", "error", err)
		os.Exit(1)
	}

	if err := wailsui.Run(cfg, dist); err != nil {
		slog.Error("Wails error", "error", err)
		os.Exit(1)
	}
}

func runManagedHeadless(cfg *config.Config) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rt := service.NewRuntime(cfg)
	rt.Startup(ctx)
	<-ctx.Done()
	rt.Shutdown(context.Background())
}
