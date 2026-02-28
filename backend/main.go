package main

import (
	"log/slog"
	"os"
)

func main() {
	cfg := parseCLI()
	setupLogging(cfg.Headless)

	if err := run(cfg); err != nil {
		slog.Error("Fatal error", "error", err)
		os.Exit(1)
	}
}
