// Package config reads and validates every environment variable the
// service needs at boot (design.md's Configuration table), aggregating
// every missing or invalid variable into a single fail-fast error so a
// misconfigured deployment fails once with a complete diagnosis instead of
// one variable at a time.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

// Config holds every validated environment variable used by the
// composition root (cmd/api, cmd/seed) and the adapters they wire.
type Config struct {
	Port               string
	AppEnv             string
	LogLevel           string
	AWSRegion          string
	DynamoTable        string
	DynamoEndpoint     string
	AWSAccessKeyID     string
	AWSSecretAccessKey string

	// JWTSecret has no built-in default: an empty value fails boot rather
	// than silently signing tokens with a known/empty key.
	JWTSecret string
	JWTTTL    time.Duration

	BetslipMinStake      money.Money
	BetslipMaxStake      money.Money
	BetslipCurrency      string
	BetslipMaxSelections int

	RateLimit                 string
	IdempotencyTTL            time.Duration
	DemoAccountInitialBalance money.Money
	SeedReset                 bool
}

// Load reads every environment variable in one pass, applying the default
// from design.md's Configuration table when a variable is unset, and
// aggregates every missing/invalid variable into a single error.
func Load() (Config, error) {
	var errs []string
	cfg := Config{}

	cfg.Port = getEnv("PORT", "8080")
	cfg.AppEnv = getEnv("APP_ENV", "local")
	cfg.LogLevel = getEnv("LOG_LEVEL", "info")
	cfg.AWSRegion = getEnv("AWS_REGION", "us-east-1")
	cfg.DynamoTable = getEnv("DYNAMO_TABLE", "apuesta-total")
	// DYNAMO_ENDPOINT deliberately has NO default. getEnv falls back to a
	// default whenever the variable is unset OR empty, so a non-empty
	// default would be unreachable through configuration: there would be
	// no way at all to express "talk to real AWS", and every DynamoDB call
	// on Lambda would target the docker-compose hostname. Empty means real
	// AWS (SDK endpoint discovery); the local stack sets it explicitly in
	// docker-compose.yml.
	cfg.DynamoEndpoint = os.Getenv("DYNAMO_ENDPOINT")
	cfg.AWSAccessKeyID = getEnv("AWS_ACCESS_KEY_ID", "local")
	cfg.AWSSecretAccessKey = getEnv("AWS_SECRET_ACCESS_KEY", "local")

	cfg.JWTSecret = os.Getenv("JWT_SECRET")
	if cfg.JWTSecret == "" {
		errs = append(errs, "JWT_SECRET is required and has no default")
	}

	cfg.JWTTTL = parseDuration("JWT_TTL", "1h", &errs)
	cfg.BetslipMinStake = parseMoney("BETSLIP_MIN_STAKE_AMOUNT", "1", &errs)
	cfg.BetslipMaxStake = parseMoney("BETSLIP_MAX_STAKE_AMOUNT", "10000", &errs)
	cfg.BetslipCurrency = getEnv("BETSLIP_CURRENCY_CODE", "PEN")
	cfg.BetslipMaxSelections = parseInt("BETSLIP_MAX_SELECTIONS", 20, &errs)
	cfg.RateLimit = getEnv("RATE_LIMIT", "60-M")
	cfg.IdempotencyTTL = parseDuration("IDEMPOTENCY_TTL", "24h", &errs)
	cfg.DemoAccountInitialBalance = parseMoney("DEMO_ACCOUNT_INITIAL_BALANCE", "1000", &errs)
	cfg.SeedReset = parseBool("SEED_RESET", false, &errs)

	if len(errs) > 0 {
		return Config{}, fmt.Errorf("config: %s", strings.Join(errs, "; "))
	}
	return cfg, nil
}

// LogValue implements log/slog.LogValuer so any boot-summary log call
// (slog.Info("boot", "config", cfg)) renders every field except JWTSecret,
// which is always replaced by the literal "REDACTED" (design.md's
// Observability section).
func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("port", c.Port),
		slog.String("app_env", c.AppEnv),
		slog.String("log_level", c.LogLevel),
		slog.String("aws_region", c.AWSRegion),
		slog.String("dynamo_table", c.DynamoTable),
		slog.String("dynamo_endpoint", c.DynamoEndpoint),
		slog.String("jwt_secret", "REDACTED"),
		slog.String("jwt_ttl", c.JWTTTL.String()),
		slog.String("betslip_min_stake", c.BetslipMinStake.String()),
		slog.String("betslip_max_stake", c.BetslipMaxStake.String()),
		slog.String("betslip_currency", c.BetslipCurrency),
		slog.Int("betslip_max_selections", c.BetslipMaxSelections),
		slog.String("rate_limit", c.RateLimit),
		slog.String("idempotency_ttl", c.IdempotencyTTL.String()),
		slog.String("demo_account_initial_balance", c.DemoAccountInitialBalance.String()),
		slog.Bool("seed_reset", c.SeedReset),
	)
}

// getEnv returns the environment variable named key, or def when unset or
// empty.
func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseDuration(key, def string, errs *[]string) time.Duration {
	raw := getEnv(key, def)
	d, err := time.ParseDuration(raw)
	if err != nil {
		*errs = append(*errs, fmt.Sprintf("%s: invalid duration %q", key, raw))
		return 0
	}
	return d
}

func parseInt(key string, def int, errs *[]string) int {
	raw := os.Getenv(key)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		*errs = append(*errs, fmt.Sprintf("%s: invalid integer %q", key, raw))
		return def
	}
	return n
}

func parseBool(key string, def bool, errs *[]string) bool {
	raw := os.Getenv(key)
	if raw == "" {
		return def
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		*errs = append(*errs, fmt.Sprintf("%s: invalid boolean %q", key, raw))
		return def
	}
	return b
}

func parseMoney(key, def string, errs *[]string) money.Money {
	raw := getEnv(key, def)
	d, err := decimal.NewFromString(raw)
	if err != nil {
		*errs = append(*errs, fmt.Sprintf("%s: invalid amount %q", key, raw))
		return money.Money{}
	}
	m, err := money.NewMoney(d)
	if err != nil {
		*errs = append(*errs, fmt.Sprintf("%s: %s", key, err))
		return money.Money{}
	}
	return m
}
