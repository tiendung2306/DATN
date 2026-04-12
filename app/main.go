package main

import (
	"embed"
	"io/fs"
	"log/slog"
	"os"

	"app/adapter/wailsui"
	"app/cli"
	"app/config"
	applog "app/pkg/log"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	cfg := config.Parse()
	applog.Setup(cfg.Headless)

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
