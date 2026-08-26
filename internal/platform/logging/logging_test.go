package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/platform/logging"
)

// TestNew_EmitsJSONAtTheRequestedLevel proves New wires a JSON handler and
// respects the configured level: a Debug call at level "debug" must appear
// in the output with the given fields.
func TestNew_EmitsJSONAtTheRequestedLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.New("debug", &buf)

	logger.Debug("boot", "component", "test")

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	require.Equal(t, "boot", decoded["msg"])
	require.Equal(t, "test", decoded["component"])
}

// TestNew_DefaultLevelSuppressesDebugButShowsInfo proves the "info" level
// (the design's default LOG_LEVEL) filters out Debug records while still
// emitting Info ones — a real behavioral distinction, not a smoke test.
func TestNew_DefaultLevelSuppressesDebugButShowsInfo(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.New("info", &buf)

	logger.Debug("hidden", "x", 1)
	require.Empty(t, buf.String(), "a debug record must be suppressed at info level")

	logger.Info("shown")
	require.Contains(t, buf.String(), "shown")
}

// TestIntoContextThenFromContext_RoundTripsTheSameLogger proves a logger
// stashed via IntoContext is exactly what FromContext returns, so a use
// case can log through logging.FromContext(ctx) without importing the HTTP
// middleware that populated it (design.md's Observability section).
func TestIntoContextThenFromContext_RoundTripsTheSameLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.New("info", &buf)
	ctx := logging.IntoContext(context.Background(), logger)

	got := logging.FromContext(ctx)
	got.Info("via-context")

	require.Contains(t, buf.String(), "via-context")
}

// TestFromContext_ReturnsAUsableLoggerWhenNoneWasStored proves a context
// with no stashed logger never panics and still returns something callable
// — a use case must never nil-check the return value of FromContext.
func TestFromContext_ReturnsAUsableLoggerWhenNoneWasStored(t *testing.T) {
	logger := logging.FromContext(context.Background())

	require.NotPanics(t, func() { logger.Info("no logger was stored, must not panic") })
}
