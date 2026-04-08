// Package logger provides a configured slog.Logger.
package logger

import (
	"log/slog"
	"os"
)

// New returns a JSON slog.Logger writing to stdout.
// In dev, set ENV=dev for text output instead.
func New(env string) *slog.Logger {
	if env == "dev" {
		return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}
