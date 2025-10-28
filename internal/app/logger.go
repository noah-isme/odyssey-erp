package app

import (
	"log/slog"
	"os"
)

// NewLogger returns a configured slog.Logger based on configuration.
func NewLogger(cfg *Config) *slog.Logger {
	if cfg != nil && cfg.LogFormat == "json" {
		return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{AddSource: true}))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: true}))
}
