package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/dto"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/handler"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/middleware"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/security"
	appauth "github.com/JulioCMax/apuesta-total-code-challenge/internal/application/auth"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/account"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

// fakeUserRepo is a test double for application/auth.UserRepository.
type fakeUserRepo struct {
	byEmail map[string]account.User
	balance money.Money
}

func (f *fakeUserRepo) FindByEmail(_ context.Context, email string) (account.User, error) {
	u, ok := f.byEmail[email]
	if !ok {
		return account.User{}, account.ErrInvalidCredentials
	}
	return u, nil
}

func (f *fakeUserRepo) Balance(_ context.Context, _ string) (money.Money, error) {
	return f.balance, nil
}

func newAuthRouter(t *testing.T, users *fakeUserRepo, verifier *security.JWT) *gin.Engine {
	t.Helper()
	h := handler.NewAuth(appauth.NewLogin(users, security.NewBcrypt(), verifier), appauth.NewBalance(users), "PEN")
	r := gin.New()
	r.POST("/auth/login", h.Login)

	protected := r.Group("/")
	protected.Use(middleware.JWTAuth(verifier))
	protected.GET("/balance", h.Balance)

	return r
}

// TestLogin_ValidCredentialsReturnsToken proves a seeded demo user logging
// in with correct credentials receives an HS256 JWT (spec: auth-and-
// balance/Demo User Login Issuing JWT).
func TestLogin_ValidCredentialsReturnsToken(t *testing.T) {
	hash, err := security.NewBcrypt().Hash("correct-password")
	require.NoError(t, err)
	users := &fakeUserRepo{byEmail: map[string]account.User{
		"demo@apuestatotal.com": {ID: "user-1", Email: "demo@apuestatotal.com", PasswordHash: hash},
	}}
	verifier := security.NewJWT("test-secret", time.Hour)
	r := newAuthRouter(t, users, verifier)

	w := doJSON(t, r, http.MethodPost, "/auth/login", `{"email":"demo@apuestatotal.com","password":"correct-password"}`)

	require.Equal(t, http.StatusOK, w.Code)
	var resp dto.LoginResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Token)

	userID, _, err := verifier.Verify(resp.Token)
	require.NoError(t, err)
	require.Equal(t, "user-1", userID)
}

// TestLogin_InvalidCredentialsReturns401WithoutLeakingEmailExistence
// proves both an unknown email and a wrong password return the exact same
// 401 envelope (spec: auth-and-balance/Demo User Login Issuing JWT,
// "Invalid credentials").
func TestLogin_InvalidCredentialsReturns401WithoutLeakingEmailExistence(t *testing.T) {
	hash, err := security.NewBcrypt().Hash("correct-password")
	require.NoError(t, err)
	users := &fakeUserRepo{byEmail: map[string]account.User{
		"demo@apuestatotal.com": {ID: "user-1", Email: "demo@apuestatotal.com", PasswordHash: hash},
	}}
	verifier := security.NewJWT("test-secret", time.Hour)
	r := newAuthRouter(t, users, verifier)

	tests := []struct {
		name string
		body string
	}{
		{"unknown email", `{"email":"unknown@apuestatotal.com","password":"whatever"}`},
		{"wrong password", `{"email":"demo@apuestatotal.com","password":"wrong-password"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := doJSON(t, r, http.MethodPost, "/auth/login", tt.body)

			require.Equal(t, http.StatusUnauthorized, w.Code)
			var body map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.Equal(t, "INVALID_CREDENTIALS", body["error"].(map[string]any)["code"])
		})
	}
}

// TestBalance_RequiresAuthentication proves GET /balance rejects an
// unauthenticated caller before touching account state (spec: auth-and-
// balance/Auth Guard Middleware).
func TestBalance_RequiresAuthentication(t *testing.T) {
	users := &fakeUserRepo{byEmail: map[string]account.User{}, balance: money.Money{}}
	r := newAuthRouter(t, users, security.NewJWT("test-secret", time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/balance", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestBalance_AuthenticatedCallerReceivesCurrentBalance is the
// triangulation case: a valid token reaches the handler and returns the
// caller's balance (spec: auth-and-balance/Balance Query).
func TestBalance_AuthenticatedCallerReceivesCurrentBalance(t *testing.T) {
	bal, err := money.NewMoneyFromFloat(750)
	require.NoError(t, err)
	users := &fakeUserRepo{byEmail: map[string]account.User{}, balance: bal}
	verifier := security.NewJWT("test-secret", time.Hour)
	r := newAuthRouter(t, users, verifier)

	token, _, err := verifier.Issue(account.User{ID: "user-1", Email: "demo@apuestatotal.com"})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/balance", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.InDelta(t, 750.00, body["balance"], 0.001)
	require.Equal(t, "PEN", body["currency"])
}
