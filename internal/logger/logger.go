// Package logger configures the global slog logger for renv/kctx.
// All packages in this module can call slog.Debug/Info/Warn/Error directly
// after logger.Init has been called once from main.
package logger

import (
	"log/slog"
	"os"
	"strings"
)

// Init configures the global slog logger.
// level accepts: debug, info, warn, error (default: warn).
// format accepts: text, json (default: text).
// Output always goes to stderr so it never pollutes eval'd stdout.
func Init(level, format string) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelWarn
	}

	opts := &slog.HandlerOptions{Level: lvl}
	var handler slog.Handler
	if strings.ToLower(format) == "json" {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}
	slog.SetDefault(slog.New(handler))
}
