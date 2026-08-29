package app

import (
	"log/slog"
	"os"
)

// NewLogger returns a configured slog.Logger based on configuration.
func NewLogger(cfg *Config) *slog.Logger {
	output := loggerOutput()
	if cfg != nil && cfg.LogFormat == "json" {
		return slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{AddSource: true}))
	}
	return slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{AddSource: true}))
}

func loggerOutput() *os.File {
	if os.Getenv("ODYSSEY_DUMP_ROUTES") != "" {
		// Route-dump mode writes a machine-readable JSON manifest to stdout.
		// Keep startup diagnostics off that stream so callers can pipe stdout
		// directly to jq without fabricating or post-processing the manifest.
		return os.Stderr
	}
	return os.Stdout
}
