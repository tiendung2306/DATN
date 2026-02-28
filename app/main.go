package main

import (
	"log/slog"
	"os"
)

func main() {
	cfg := parseCLI()
	setupLogging(cfg.Headless)

	// One-shot command flags or explicit --headless → CLI mode (existing behavior).
	// No flags → GUI mode via Wails.
	if cfg.Headless || cfg.IsCommand() {
		if err := run(cfg); err != nil {
			slog.Error("Fatal error", "error", err)
			os.Exit(1)
		}
		return
	}

	runWailsApp(cfg)
}
