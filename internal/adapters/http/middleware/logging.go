package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/apperror"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/platform/logging"
)

// Logging derives a request-scoped *slog.Logger (field request_id from
// RequestID's stashed context value, read via apperror.RequestID so both
// packages agree on where it lives) and stores it in the request's context
// so use cases can log through logging.FromContext(ctx) without importing
// anything HTTP-related (design.md's Observability section). On
// completion it emits one structured JSON log entry with method, path,
// status, duration_ms, bytes and client_ip (spec: api-platform/Structured
// JSON Logging).
//
// path is c.Request.URL.Path — the literal path the caller requested — not
// gin's c.FullPath(), which is "" for any request that never matched a
// route (finding W-logging: every unrouted 404 previously logged an empty
// path, making it impossible to tell which URL was actually probed). The
// registered route template is still recorded separately as route, which
// stays low-cardinality ("/events/:id" instead of "/events/42") for
// grouping/metrics.
func Logging(base *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		reqLogger := base.With("request_id", apperror.RequestID(c))
		c.Request = c.Request.WithContext(logging.IntoContext(c.Request.Context(), reqLogger))

		c.Next()

		reqLogger.Info("request completed",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"route", c.FullPath(),
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"bytes", c.Writer.Size(),
			"client_ip", c.ClientIP(),
		)
	}
}
