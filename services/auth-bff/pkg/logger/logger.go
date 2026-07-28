// Package logger provides a configured slog.Logger.
package logger

import (
	"log/slog"
	"os"
)

// New returns a JSON slog.Logger writing to stdout (text in dev).
func New(env string) *slog.Logger {
	if env == "dev" {
		return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
			// See the JSON handler below.
			AddSource: true,
		}))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		// Emits source.{file,line,function} on every record, which is what
		// lets an error in the observability UI be traced to the exact line
		// that produced it. The Dockerfile already builds with -trimpath, so
		// the file is module-relative and maps onto a path in this repo at the
		// commit the running image was built from. Without it the chain can
		// only ever name the deployed build, not the line.
		AddSource: true,
	}))
}
