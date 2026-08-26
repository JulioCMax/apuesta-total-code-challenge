package config_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/platform/config"
)

// TestLoad_JWTSecretIsRequiredWithNoDefault proves boot fails when
// JWT_SECRET is unset, unlike every other variable in the configuration
// table (design.md: "no built-in default; boot fails if empty").
func TestLoad_JWTSecretIsRequiredWithNoDefault(t *testing.T) {
	t.Setenv("JWT_SECRET", "")

	_, err := config.Load()

	require.Error(t, err)
	require.Contains(t, err.Error(), "JWT_SECRET")
}

// TestLoad_AggregatesEveryMissingOrInvalidVariable proves a single Load
// call reports every problem at once (fail-fast, single aggregated error),
// not just the first one encountered.
func TestLoad_AggregatesEveryMissingOrInvalidVariable(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	t.Setenv("JWT_TTL", "not-a-duration")
	t.Setenv("BETSLIP_MIN_STAKE_AMOUNT", "not-a-number")
	t.Setenv("BETSLIP_MAX_SELECTIONS", "not-an-int")

	_, err := config.Load()

	require.Error(t, err)
	require.Contains(t, err.Error(), "JWT_SECRET")
	require.Contains(t, err.Error(), "JWT_TTL")
	require.Contains(t, err.Error(), "BETSLIP_MIN_STAKE_AMOUNT")
	require.Contains(t, err.Error(), "BETSLIP_MAX_SELECTIONS")
}

// TestLoad_SucceedsWithDefaultsWhenOnlyJWTSecretIsSet proves every other
// variable falls back to the exact default published in design.md's
// Configuration table when unset.
func TestLoad_SucceedsWithDefaultsWhenOnlyJWTSecretIsSet(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("PORT", "")
	t.Setenv("APP_ENV", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("AWS_REGION", "")
	t.Setenv("DYNAMO_TABLE", "")
	t.Setenv("DYNAMO_ENDPOINT", "")
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("JWT_TTL", "")
	t.Setenv("BETSLIP_MIN_STAKE_AMOUNT", "")
	t.Setenv("BETSLIP_MAX_STAKE_AMOUNT", "")
	t.Setenv("BETSLIP_CURRENCY_CODE", "")
	t.Setenv("BETSLIP_MAX_SELECTIONS", "")
	t.Setenv("RATE_LIMIT", "")
	t.Setenv("IDEMPOTENCY_TTL", "")
	t.Setenv("DEMO_ACCOUNT_INITIAL_BALANCE", "")
	t.Setenv("SEED_RESET", "")

	cfg, err := config.Load()

	require.NoError(t, err)
	require.Equal(t, "8080", cfg.Port)
	require.Equal(t, "local", cfg.AppEnv)
	require.Equal(t, "info", cfg.LogLevel)
	require.Equal(t, "us-east-1", cfg.AWSRegion)
	require.Equal(t, "apuesta-total", cfg.DynamoTable)
	// DYNAMO_ENDPOINT is the one variable with no default: empty means
	// real AWS (see TestLoad_DynamoEndpointDefaultsToEmptyForRealAWS).
	require.Empty(t, cfg.DynamoEndpoint)
	require.Equal(t, "local", cfg.AWSAccessKeyID)
	require.Equal(t, "local", cfg.AWSSecretAccessKey)
	require.Equal(t, time.Hour, cfg.JWTTTL)
	require.Equal(t, "1.00", cfg.BetslipMinStake.String())
	require.Equal(t, "10000.00", cfg.BetslipMaxStake.String())
	require.Equal(t, "PEN", cfg.BetslipCurrency)
	require.Equal(t, 20, cfg.BetslipMaxSelections)
	require.Equal(t, "60-M", cfg.RateLimit)
	require.Equal(t, 24*time.Hour, cfg.IdempotencyTTL)
	require.Equal(t, "1000.00", cfg.DemoAccountInitialBalance.String())
	require.Equal(t, false, cfg.SeedReset)
}

// TestLoad_OverridesDefaultsFromEnvironment proves an explicitly set
// variable wins over its default, using values distinct from every default
// above so a copy-paste default could never make this pass by accident.
func TestLoad_OverridesDefaultsFromEnvironment(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("PORT", "9090")
	t.Setenv("BETSLIP_CURRENCY_CODE", "USD")
	t.Setenv("BETSLIP_MAX_SELECTIONS", "7")
	t.Setenv("SEED_RESET", "true")

	cfg, err := config.Load()

	require.NoError(t, err)
	require.Equal(t, "9090", cfg.Port)
	require.Equal(t, "USD", cfg.BetslipCurrency)
	require.Equal(t, 7, cfg.BetslipMaxSelections)
	require.Equal(t, true, cfg.SeedReset)
}

// TestConfig_LogValueRedactsJWTSecret proves a naive
// slog.Info("boot", "config", cfg) call can never leak JWT_SECRET into the
// structured log output, while still emitting other boot-summary fields
// (design.md's Observability section: "config summary with JWT_SECRET
// redacted").
func TestConfig_LogValueRedactsJWTSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "super-secret-value-do-not-leak")
	cfg, err := config.Load()
	require.NoError(t, err)

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	logger.Info("boot", "config", cfg)

	out := buf.String()
	require.NotContains(t, out, "super-secret-value-do-not-leak")

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	configField, ok := decoded["config"].(map[string]any)
	require.True(t, ok, "config field must be a nested object, not a raw string")
	require.Equal(t, "REDACTED", configField["jwt_secret"])
	require.Equal(t, "8080", configField["port"])
}

// TestLoad_DynamoEndpointDefaultsToEmptyForRealAWS proves the DynamoDB
// endpoint has NO built-in default. A non-empty default is unreachable
// through configuration, because getEnv falls back to it whenever the
// variable is unset OR empty: there would be no way at all to say "talk to
// real AWS", and every DynamoDB call on Lambda would target the
// docker-compose hostname. The local stack sets DYNAMO_ENDPOINT
// explicitly instead (docker-compose.yml).
func TestLoad_DynamoEndpointDefaultsToEmptyForRealAWS(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("DYNAMO_ENDPOINT", "")

	cfg, err := config.Load()

	require.NoError(t, err)
	require.Empty(t, cfg.DynamoEndpoint, "an empty DYNAMO_ENDPOINT must mean real AWS, never a hardcoded local hostname")
}

// TestLoad_DynamoEndpointHonoursAnExplicitLocalValue is the triangulation
// case: docker-compose's explicit endpoint must still reach the config.
func TestLoad_DynamoEndpointHonoursAnExplicitLocalValue(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("DYNAMO_ENDPOINT", "http://dynamodb:8000")

	cfg, err := config.Load()

	require.NoError(t, err)
	require.Equal(t, "http://dynamodb:8000", cfg.DynamoEndpoint)
}
