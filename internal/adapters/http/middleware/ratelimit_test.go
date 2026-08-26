package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/apperror"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/middleware"
)

// TestRateLimit_Returns429AfterBudget proves a client IP exceeding the
// configured request threshold within the configured window is rejected
// with the standard error envelope at 429 (spec: api-platform/Per-IP Rate
// Limiting on Public Endpoints, "Exceeding the limit").
func TestRateLimit_Returns429AfterBudget(t *testing.T) {
	mw, err := middleware.RateLimit("2-M") // 2 requests/minute
	require.NoError(t, err)

	r := gin.New()
	r.SetTrustedProxies(nil)
	r.Use(mw)
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	get := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "203.0.113.7:54321"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	require.Equal(t, http.StatusOK, get().Code)
	require.Equal(t, http.StatusOK, get().Code)

	third := get()
	require.Equal(t, http.StatusTooManyRequests, third.Code)

	var env apperror.Envelope
	require.NoError(t, json.Unmarshal(third.Body.Bytes(), &env))
	require.Equal(t, "RATE_LIMITED", env.Error.Code)
}

// TestRateLimit_KeyIsClientIPNeverXForwardedFor proves the limiter never
// trusts a spoofable X-Forwarded-For header as the rate-limit key (D11): a
// budget-exhausting run of requests that all carry the SAME
// X-Forwarded-For but come from DIFFERENT RemoteAddr values must each get
// their own independent budget, because with SetTrustedProxies(nil) gin's
// ClientIP() falls back to RemoteAddr and ignores XFF entirely.
func TestRateLimit_KeyIsClientIPNeverXForwardedFor(t *testing.T) {
	mw, err := middleware.RateLimit("1-M") // 1 request/minute — tight budget
	require.NoError(t, err)

	r := gin.New()
	r.SetTrustedProxies(nil)
	r.Use(mw)
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	requestFrom := func(remoteAddr string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = remoteAddr
		req.Header.Set("X-Forwarded-For", "9.9.9.9") // same spoofed value on every call
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	require.Equal(t, http.StatusOK, requestFrom("198.51.100.1:1111").Code)
	// A different RemoteAddr with the SAME spoofed XFF must still get its
	// own fresh budget — proving XFF was never consulted as the key.
	require.Equal(t, http.StatusOK, requestFrom("198.51.100.2:2222").Code)
	// But the FIRST RemoteAddr, called again, is now over budget.
	require.Equal(t, http.StatusTooManyRequests, requestFrom("198.51.100.1:1111").Code)
}
