// Package logging provides the process-wide structured logger: a
// log/slog JSON handler installed once at boot, plus a request-scoped
// logger carried through context.Context so use cases can log through
// FromContext(ctx) without importing anything from adapters/http
// (design.md's Observability section).
package logging

import (
	"context"
	"io"
	"log/slog"
	"strings"
)

// ctxKey is an unexported type so no other package can collide with this
// context key.
type ctxKey struct{}

// New builds a *slog.Logger writing newline-delimited JSON to out, at the
// level named by levelName ("debug"|"info"|"warn"|"error", case
// insensitive; an unrecognized or empty name defaults to "info", matching
// LOG_LEVEL's default in design.md's Configuration table).
func New(levelName string, out io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{Level: parseLevel(levelName)}))
}

func parseLevel(levelName string) slog.Level {
	switch strings.ToLower(levelName) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// IntoContext returns a copy of ctx carrying logger, retrievable later via
// FromContext.
func IntoContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, logger)
}

// FromContext returns the logger stashed by IntoContext, or slog.Default()
// when ctx carries none, so a caller never needs to nil-check the result.
func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok && logger != nil {
		return logger
	}
	return slog.Default()
}
