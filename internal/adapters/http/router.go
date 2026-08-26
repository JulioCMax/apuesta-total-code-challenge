// Package http is the composition point for the HTTP adapter: it builds
// the *gin.Engine, wires the route table (design.md's HTTP Layer section),
// and applies the middleware chain in the documented order. cmd/api
// (Phase 13) is the only production caller of NewRouter; it uses the same
// *gin.Engine both for the local http.Server and for the Lambda Function
// URL adapter (D12).
package http

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/handler"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/middleware"
)

// Dependencies carries every handler and cross-cutting collaborator
// NewRouter needs. The composition root (cmd/api) builds these from
// platform/config and the concrete adapters (dynamo, memory, security);
// this package never imports any of those directly, keeping the router
// free to be tested against fakes (router_test.go).
type Dependencies struct {
	Events    *handler.Events
	BetSlip   *handler.BetSlip
	Auth      *handler.Auth
	Bets      *handler.Bets
	Verifier  middleware.TokenVerifier
	Logger    *slog.Logger
	RateLimit string // ulule/limiter format, e.g. "60-M" (RATE_LIMIT)
	Version   string // echoed by GET /health
}

// NewRouter builds the full route table with the documented middleware
// order (design.md's HTTP Layer: recovery -> requestID -> slog logging ->
// rateLimit -> jwtAuth).
//
// SetTrustedProxies(nil) is mandatory (D11): with no trusted proxies, gin's
// ClientIP() — the rate limiter's key — resolves to the real RemoteAddr and
// never trusts a spoofable X-Forwarded-For header. Behind the Lambda
// Function URL, the aws-lambda-go-api-proxy adapter (Phase 13) populates
// RemoteAddr with the real source IP before this router ever sees the
// request, so this holds in both environments.
func NewRouter(deps Dependencies) (*gin.Engine, error) {
	r := gin.New()
	r.SetTrustedProxies(nil)

	r.Use(middleware.Recovery(), middleware.RequestID(), middleware.Logging(deps.Logger))

	r.GET("/health", handler.Health(deps.Version))

	rateLimit, err := middleware.RateLimit(deps.RateLimit)
	if err != nil {
		return nil, err
	}

	v1 := r.Group("/api/v1")
	v1.Use(rateLimit)

	v1.POST("/auth/login", deps.Auth.Login)
	v1.GET("/events", deps.Events.List)
	v1.GET("/events/:id", deps.Events.Detail)
	v1.POST("/betslip/calculate", deps.BetSlip.Calculate)

	protected := v1.Group("/")
	protected.Use(middleware.JWTAuth(deps.Verifier))
	protected.POST("/betslip/place", deps.BetSlip.Place)
	protected.GET("/balance", deps.Auth.Balance)
	protected.GET("/bets", deps.Bets.List)

	return r, nil
}
