// Command api is the composition root: it wires every adapter (DynamoDB,
// the in-memory event catalog, JWT/bcrypt) into the application use cases,
// builds the single *gin.Engine (design.md's Local vs Lambda section), and
// serves it either as a local HTTP server or as a Lambda Function URL
// handler, selected by the presence of AWS_LAMBDA_FUNCTION_NAME (D12).
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	ginadapter "github.com/awslabs/aws-lambda-go-api-proxy/gin"

	httpadapter "github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/handler"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/dynamo"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/memory"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/security"
	appauth "github.com/JulioCMax/apuesta-total-code-challenge/internal/application/auth"
	appbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/application/betslip"
	appevent "github.com/JulioCMax/apuesta-total-code-challenge/internal/application/event"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/platform/config"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/platform/id"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/platform/logging"
)

// version is the value GET /health echoes; a build-time ldflags override is
// the natural next step, not needed for this challenge's scope.
const version = "1.0.0"

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("boot: invalid configuration", "error", err)
		os.Exit(1)
	}

	logger := logging.New(cfg.LogLevel, os.Stdout)
	slog.SetDefault(logger)
	logger.Info("boot", "config", cfg)

	ctx := context.Background()

	dynamoClient, err := dynamo.NewClient(ctx, cfg)
	if err != nil {
		logger.Error("boot: dynamo client", "error", err)
		os.Exit(1)
	}

	eventCatalog, err := memory.NewEventRepository()
	if err != nil {
		logger.Error("boot: event repository", "error", err)
		os.Exit(1)
	}

	userRepo := dynamo.NewUserRepository(dynamoClient, cfg.DynamoTable)
	betRepo := dynamo.NewBetRepository(dynamoClient, cfg.DynamoTable, cfg.IdempotencyTTL)
	jwt := security.NewJWT(cfg.JWTSecret, cfg.JWTTTL)
	passwords := security.NewBcrypt()
	ids := id.NewULIDGenerator()

	bounds := appbetslip.StakeBounds{
		MinStake:      cfg.BetslipMinStake,
		MaxStake:      cfg.BetslipMaxStake,
		Currency:      cfg.BetslipCurrency,
		MaxSelections: cfg.BetslipMaxSelections,
	}

	engine, err := httpadapter.NewRouter(httpadapter.Dependencies{
		Events: handler.NewEvents(appevent.NewList(eventCatalog), appevent.NewDetail(eventCatalog)),
		BetSlip: handler.NewBetSlip(
			appbetslip.NewCalculate(eventCatalog, bounds),
			appbetslip.NewPlace(eventCatalog, betRepo, ids, bounds),
			appauth.NewBalance(userRepo),
		),
		Auth:      handler.NewAuth(appauth.NewLogin(userRepo, passwords, jwt), appauth.NewBalance(userRepo), cfg.BetslipCurrency),
		Bets:      handler.NewBets(appauth.NewHistory(betRepo)),
		Verifier:  jwt,
		Logger:    logger,
		RateLimit: cfg.RateLimit,
		Version:   version,
	})
	if err != nil {
		logger.Error("boot: router", "error", err)
		os.Exit(1)
	}

	// The entire environment branch (D12): AWS_LAMBDA_FUNCTION_NAME is a
	// reserved variable the Lambda runtime always sets and a local process
	// never has, so this cannot be misconfigured. ginadapter.NewV2 parses
	// the API Gateway v2 (Function URL) payload format; a v1-only adapter
	// would silently mis-parse the request path.
	if os.Getenv("AWS_LAMBDA_FUNCTION_NAME") != "" {
		lambda.Start(ginadapter.NewV2(engine).ProxyWithContext)
		return
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           engine,
		ReadHeaderTimeout: 5 * time.Second,
		// ReadTimeout/WriteTimeout bound the entire request/response cycle,
		// not just the headers: without them a slow-body attacker (or a
		// client that never finishes sending) can hold a connection open
		// indefinitely even though middleware.BodyLimit already caps how
		// many bytes it may eventually send (finding W-body).
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		logger.Error("server error", "error", err)
		os.Exit(1)
	case <-quit:
		logger.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", "error", err)
	}
}
