// Package http is the composition point for the HTTP adapter: it builds
// the *gin.Engine, wires the route table (design.md's HTTP Layer section),
// and applies the middleware chain in the documented order. cmd/api
// (Phase 13) is the only production caller of NewRouter; it uses the same
// *gin.Engine both for the local http.Server and for the Lambda Function
// URL adapter (D12).
package http

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/apperror"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/handler"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/middleware"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/web"
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

	// HandleMethodNotAllowed is false by default in gin, which means an
	// existing path called with an unsupported method falls straight
	// through to NoRoute (404) instead of NoMethod (405). Both NoRoute and
	// NoMethod are registered below with the same standard error envelope
	// (finding W-router): without them gin serves its own plain-text
	// "404 page not found" body, with no requestId and no JSON shape, which
	// also silently defeats logging.Path (finding W-logging).
	r.HandleMethodNotAllowed = true
	r.NoRoute(func(c *gin.Context) {
		apperror.WriteStatus(c, http.StatusNotFound, "NOT_FOUND", "El recurso solicitado no existe.")
	})
	r.NoMethod(func(c *gin.Context) {
		apperror.WriteStatus(c, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "El método HTTP no está permitido para este recurso.")
	})

	r.Use(
		middleware.Recovery(),
		middleware.RequestID(),
		middleware.Logging(deps.Logger),
		middleware.BodyLimit(middleware.DefaultMaxRequestBodyBytes),
	)

	r.GET("/health", handler.Health(deps.Version))

	// GET /docs and GET /openapi.yaml sit at the same top level as /health
	// (design.md's HTTP Layer route table: Public section lists all three
	// together), outside /api/v1 — never rate-limited, never JWT-guarded
	// (Phase 14).
	r.GET("/openapi.yaml", handler.OpenAPISpec())
	r.GET("/docs", handler.Docs())

	// GET /app serves the embedded web client (internal/adapters/web). It
	// belongs beside /docs for the same reasons: both are demonstration
	// surfaces rather than business endpoints, so neither is rate limited
	// nor JWT-guarded. The client is only a browser — every API call it
	// makes goes through /api/v1 and is limited and guarded exactly like
	// any other caller's.
	//
	// Only the catch-all is registered. A bare GET /app carries no
	// filepath parameter and so cannot match it; gin's RedirectTrailingSlash
	// (left at its default) answers that request with a redirect to
	// /app/, which does match. Registering /app explicitly as well would
	// collide with this catch-all in gin's route tree.
	r.GET("/app/*filepath", handler.App(web.Assets))

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
