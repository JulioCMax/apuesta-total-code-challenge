// Command seed is the single binary that prepares the DynamoDB table for
// both docker-compose (as a one-shot service) and scripts/deploy-aws.sh
// against the real table (design.md's Seeding section): wait for the
// endpoint with backoff, create the table if absent (and enable TTL), then
// seed the demo users idempotently at DEMO_ACCOUNT_INITIAL_BALANCE.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/shopspring/decimal"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/dynamo"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/security"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/account"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/platform/config"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/platform/id"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/platform/logging"
)

// demoUser is one seeded demo credential. Passwords are plaintext only
// here, in source, purely for the evaluator's convenience logging in
// against a throwaway local/demo table — never stored anywhere but as a
// bcrypt hash (design.md's HTTP Layer: "bcrypt cost 10").
type demoUser struct {
	email    string
	password string
}

// demoUsers is the fixed seed list for this challenge's demo environment.
var demoUsers = []demoUser{
	{email: "demo1@apuestatotal.com", password: "Demo1234!"},
	{email: "demo2@apuestatotal.com", password: "Demo1234!"},
}

// waitForEndpointBackoff is the maximum time cmd/seed waits for the
// DynamoDB endpoint (dynamodb-local has no shell/healthcheck tooling in its
// image, so this is the only readiness signal docker-compose can rely on).
const waitForEndpointBackoff = 60 * time.Second

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("seed: invalid configuration", "error", err)
		os.Exit(1)
	}

	logger := logging.New(cfg.LogLevel, os.Stdout)
	slog.SetDefault(logger)

	ctx := context.Background()

	client, err := dynamo.NewClient(ctx, cfg)
	if err != nil {
		logger.Error("seed: dynamo client", "error", err)
		os.Exit(1)
	}

	if err := waitForEndpoint(ctx, client, waitForEndpointBackoff); err != nil {
		logger.Error("seed: dynamodb endpoint never became reachable", "error", err)
		os.Exit(1)
	}

	if err := dynamo.EnsureTable(ctx, client, cfg.DynamoTable); err != nil {
		logger.Error("seed: ensure table", "error", err)
		os.Exit(1)
	}
	logger.Info("seed: table ready", "table", cfg.DynamoTable)

	users := dynamo.NewUserRepository(client, cfg.DynamoTable)
	passwords := security.NewBcrypt()
	ids := id.NewULIDGenerator()

	for _, du := range demoUsers {
		if err := seedOne(ctx, users, passwords, ids, du, cfg); err != nil {
			logger.Error("seed: user", "email", du.email, "error", err)
			os.Exit(1)
		}
	}

	logger.Info("seed: done", "users", len(demoUsers), "reset", cfg.SeedReset)
}

// seedOne hashes du's password and writes the profile item. When
// cfg.SeedReset is false (the default), an already-seeded profile is left
// untouched (PutUserIfAbsent) so a re-run never clobbers a played-with
// balance; SEED_RESET=true forces an overwrite via PutUser.
func seedOne(ctx context.Context, users *dynamo.UserRepository, passwords *security.Bcrypt, ids *id.ULIDGenerator, du demoUser, cfg config.Config) error {
	hash, err := passwords.Hash(du.password)
	if err != nil {
		return err
	}

	// money.Money deliberately exposes no Decimal() accessor (D6's minimal
	// API): String() -> decimal.NewFromString is the same round trip the
	// config package's own parseMoney already relies on.
	balance, err := decimal.NewFromString(cfg.DemoAccountInitialBalance.String())
	if err != nil {
		return err
	}
	user, err := account.NewUser(ids.NewID(), du.email, hash, balance, cfg.BetslipCurrency, time.Now().UTC())
	if err != nil {
		return err
	}

	if cfg.SeedReset {
		return users.PutUser(ctx, user)
	}

	if err := users.PutUserIfAbsent(ctx, user); err != nil {
		if err == dynamo.ErrUserAlreadyExists {
			slog.Info("seed: user already exists, left untouched", "email", du.email)
			return nil
		}
		return err
	}
	return nil
}

// waitForEndpoint polls DynamoDB (ListTables — the cheapest read-only
// call available on a fresh, empty table) with linear backoff until it
// responds or maxWait elapses. dynamodb-local's image ships no shell
// tooling for a docker-compose healthcheck, so this is the seeder's own
// readiness gate (design.md's docker-compose.yml section).
func waitForEndpoint(ctx context.Context, client *dynamodb.Client, maxWait time.Duration) error {
	deadline := time.Now().Add(maxWait)
	backoff := 500 * time.Millisecond
	var lastErr error

	for time.Now().Before(deadline) {
		_, err := client.ListTables(ctx, &dynamodb.ListTablesInput{})
		if err == nil {
			return nil
		}
		lastErr = err
		time.Sleep(backoff)
		if backoff < 5*time.Second {
			backoff *= 2
		}
	}
	return lastErr
}
