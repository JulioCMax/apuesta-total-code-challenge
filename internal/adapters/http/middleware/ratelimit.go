package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	limiter "github.com/ulule/limiter"
	ginlimiter "github.com/ulule/limiter/drivers/middleware/gin"
	memorystore "github.com/ulule/limiter/drivers/store/memory"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/apperror"
)

// RateLimit builds a per-client-IP rate limiter from a ulule/limiter
// formatted rate string (e.g. "60-M" = 60 requests/minute), backed by an
// in-memory store (D10 — the only candidate with per-key expiry and a
// built-in Gin middleware; a hand-rolled map[string]*rate.Limiter would be
// an unbounded-growth footgun needing its own tests for zero product
// value). The limiter key is always gin's c.ClientIP() — the library's own
// default — which resolves to c.Request.RemoteAddr, never
// X-Forwarded-For, because router.NewRouter calls
// (*gin.Engine).SetTrustedProxies(nil) (D11): a spoofable header must never
// be trusted as the rate-limit key.
func RateLimit(rateSpec string) (gin.HandlerFunc, error) {
	rate, err := limiter.NewRateFromFormatted(rateSpec)
	if err != nil {
		return nil, err
	}

	instance := limiter.New(memorystore.NewStore(), rate)

	return ginlimiter.NewMiddleware(instance,
		ginlimiter.WithKeyGetter(func(c *gin.Context) string { return c.ClientIP() }),
		ginlimiter.WithLimitReachedHandler(func(c *gin.Context) {
			apperror.WriteStatus(c, http.StatusTooManyRequests, "RATE_LIMITED",
				"Se excedió el límite de solicitudes permitidas. Intente nuevamente más tarde.")
		}),
		ginlimiter.WithErrorHandler(func(c *gin.Context, _ error) {
			apperror.WriteStatus(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Ocurrió un error interno inesperado.")
		}),
	), nil
}
