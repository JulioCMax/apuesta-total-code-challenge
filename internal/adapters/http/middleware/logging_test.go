package middleware_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/middleware"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/platform/logging"
)

// TestLoggingMiddleware_EmitsStructuredFieldsOnCompletion proves every
// request produces one JSON log entry on completion carrying at minimum
// method, path, status and duration (spec: api-platform/Structured JSON
// Logging).
func TestLoggingMiddleware_EmitsStructuredFieldsOnCompletion(t *testing.T) {
	var buf bytes.Buffer
	base := logging.New("info", &buf)

	r := gin.New()
	r.Use(middleware.RequestID(), middleware.Logging(base))
	r.GET("/events/:id", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/events/42", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var entry map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry))
	require.Equal(t, "GET", entry["method"])
	require.Equal(t, "/events/:id", entry["path"])
	require.InDelta(t, float64(http.StatusOK), entry["status"], 0.001)
	require.Contains(t, entry, "duration_ms")
	require.Contains(t, entry, "request_id")
	require.NotEmpty(t, entry["request_id"])
}

// TestLoggingMiddleware_RecordsDifferentStatusForServerError is the
// triangulation case: a different response status must be reflected
// exactly, proving the logged "status" field reads the real response, not
// a hardcoded 200.
func TestLoggingMiddleware_RecordsDifferentStatusForServerError(t *testing.T) {
	var buf bytes.Buffer
	base := logging.New("info", &buf)

	r := gin.New()
	r.Use(middleware.RequestID(), middleware.Logging(base))
	r.GET("/boom", func(c *gin.Context) { c.Status(http.StatusInternalServerError) })

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var entry map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry))
	require.InDelta(t, float64(http.StatusInternalServerError), entry["status"], 0.001)
}
