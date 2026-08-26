package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/apperror"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/middleware"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/security"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/account"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newEngine(mw ...gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	r.Use(mw...)
	return r
}

// TestRequestID_EchoesHeaderAndMatchesContextValue proves the middleware
// generates one request ID, echoes it in the X-Request-Id response header,
// and stashes the exact same value where apperror.RequestID reads it from
// (design.md's Observability section: "echoes it in the X-Request-Id
// response header and in every error envelope").
func TestRequestID_EchoesHeaderAndMatchesContextValue(t *testing.T) {
	var seenInContext string
	r := newEngine(middleware.RequestID())
	r.GET("/", func(c *gin.Context) {
		seenInContext = apperror.RequestID(c)
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	headerID := w.Header().Get(middleware.RequestIDHeader)
	require.NotEmpty(t, headerID)
	require.Equal(t, headerID, seenInContext)
}

// TestRequestID_GeneratesDistinctIDsPerRequest is the triangulation case:
// two separate requests must never collide (crypto/rand backed).
func TestRequestID_GeneratesDistinctIDsPerRequest(t *testing.T) {
	r := newEngine(middleware.RequestID())
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, httptest.NewRequest(http.MethodGet, "/", nil))
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/", nil))

	require.NotEqual(t, w1.Header().Get(middleware.RequestIDHeader), w2.Header().Get(middleware.RequestIDHeader))
}

// TestRecovery_ConvertsPanicToInternalErrorEnvelope proves a panicking
// handler never crashes the process and instead produces the standard
// error envelope at 500 (spec: api-platform/Consistent Error Envelope,
// "Unexpected server error uses the standard envelope").
func TestRecovery_ConvertsPanicToInternalErrorEnvelope(t *testing.T) {
	r := newEngine(middleware.Recovery(), middleware.RequestID())
	r.GET("/boom", func(c *gin.Context) {
		panic("something exploded")
	})

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	w := httptest.NewRecorder()

	require.NotPanics(t, func() {
		r.ServeHTTP(w, req)
	})

	require.Equal(t, http.StatusInternalServerError, w.Code)
	var env apperror.Envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.Equal(t, "INTERNAL_ERROR", env.Error.Code)
	require.NotEmpty(t, env.RequestID)
}

// TestJWTAuth_RejectsMissingOrInvalidToken proves the guard rejects a
// request with no valid Authorization: Bearer token BEFORE the wrapped
// handler runs (spec: auth-and-balance/Auth Guard Middleware, bet-slip-
// placement/JWT-Guarded Placement).
func TestJWTAuth_RejectsMissingOrInvalidToken(t *testing.T) {
	verifier := security.NewJWT("test-secret", time.Hour)
	handlerCalled := false

	tests := []struct {
		name   string
		header string
	}{
		{"no header at all", ""},
		{"missing Bearer prefix", "not-a-bearer-token"},
		{"malformed token", "Bearer garbage.not.a.jwt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handlerCalled = false
			r := newEngine(middleware.Recovery(), middleware.RequestID(), middleware.JWTAuth(verifier))
			r.GET("/protected", func(c *gin.Context) {
				handlerCalled = true
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, http.StatusUnauthorized, w.Code)
			require.False(t, handlerCalled, "handler must never run without a valid token")

			var env apperror.Envelope
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
			require.Equal(t, "UNAUTHORIZED", env.Error.Code)
		})
	}
}

// TestJWTAuth_AcceptsValidTokenAndExposesUserID is the triangulation case: a
// genuinely valid token reaches the handler and middleware.UserID returns
// exactly the subject the token carries.
func TestJWTAuth_AcceptsValidTokenAndExposesUserID(t *testing.T) {
	verifier := security.NewJWT("test-secret", time.Hour)
	token, _, err := verifier.Issue(account.User{ID: "user-42", Email: "demo@apuestatotal.com"})
	require.NoError(t, err)

	var seenUserID string
	r := newEngine(middleware.Recovery(), middleware.RequestID(), middleware.JWTAuth(verifier))
	r.GET("/protected", func(c *gin.Context) {
		seenUserID = middleware.UserID(c)
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "user-42", seenUserID)
}
