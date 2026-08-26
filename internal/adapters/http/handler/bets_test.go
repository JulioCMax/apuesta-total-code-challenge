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
	domainbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/betslip"
)

// fakeBetHistory is a test double for application/auth.BetHistory. It
// enforces per-user scoping exactly like the real DynamoDB adapter's Query
// against a single user's partition (spec: bet-history/List Caller's Own
// Bets).
type fakeBetHistory struct {
	byUser map[string][]domainbetslip.Bet
}

func (f *fakeBetHistory) ListByUser(_ context.Context, userID string, _ int, _ string) ([]domainbetslip.Bet, string, error) {
	return f.byUser[userID], "", nil
}

func newBetsRouter(t *testing.T, history *fakeBetHistory, verifier *security.JWT) *gin.Engine {
	t.Helper()
	h := handler.NewBets(appauth.NewHistory(history))
	r := gin.New()
	protected := r.Group("/")
	protected.Use(middleware.JWTAuth(verifier))
	protected.GET("/bets", h.List)
	return r
}

// TestBets_RequiresAuthentication proves GET /bets rejects an
// unauthenticated caller and returns no bet data (spec: bet-history/List
// Caller's Own Bets, "Unauthenticated request").
func TestBets_RequiresAuthentication(t *testing.T) {
	history := &fakeBetHistory{byUser: map[string][]domainbetslip.Bet{
		"user-a": {{ID: "bet-a1"}},
	}}
	r := newBetsRouter(t, history, security.NewJWT("test-secret", time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/bets", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "UNAUTHORIZED", body["error"].(map[string]any)["code"])
	require.NotContains(t, w.Body.String(), "bet-a1", "an unauthenticated request must never see any bet data")
}

// TestBets_CallerSeesOnlyOwnBets proves caller A's request never surfaces
// caller B's bets (spec: bet-history/List Caller's Own Bets, "Caller sees
// only their own bets").
func TestBets_CallerSeesOnlyOwnBets(t *testing.T) {
	history := &fakeBetHistory{byUser: map[string][]domainbetslip.Bet{
		"user-a": {{ID: "bet-a1"}, {ID: "bet-a2"}},
		"user-b": {{ID: "bet-b1"}, {ID: "bet-b2"}, {ID: "bet-b3"}},
	}}
	verifier := security.NewJWT("test-secret", time.Hour)
	r := newBetsRouter(t, history, verifier)

	token, _, err := verifier.Issue(account.User{ID: "user-a", Email: "a@apuestatotal.com"})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/bets", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	// money.Money has no UnmarshalJSON (D13's minimal API), so this
	// assertion reads the raw JSON structure instead of re-hydrating
	// dto.BetsResponse.
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	items := body["items"].([]any)
	require.Len(t, items, 2)
	for _, raw := range items {
		item := raw.(map[string]any)
		require.NotEqual(t, "bet-b1", item["betId"])
		require.NotEqual(t, "bet-b2", item["betId"])
		require.NotEqual(t, "bet-b3", item["betId"])
	}
}

// TestBets_CallerWithNoBetsReceivesEmptyList proves a caller with no bets
// gets 200 with an empty list, never an error (spec: bet-history/List
// Caller's Own Bets, "Caller with no bets").
func TestBets_CallerWithNoBetsReceivesEmptyList(t *testing.T) {
	history := &fakeBetHistory{byUser: map[string][]domainbetslip.Bet{}}
	verifier := security.NewJWT("test-secret", time.Hour)
	r := newBetsRouter(t, history, verifier)

	token, _, err := verifier.Issue(account.User{ID: "user-a", Email: "a@apuestatotal.com"})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/bets", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp dto.BetsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Empty(t, resp.Items)
}

// TestBets_InvalidLimitReturns400ValidationError proves a `limit` query
// param that is non-numeric, negative, or out of DynamoDB's int32 Limit
// range is rejected with the standard VALIDATION_ERROR envelope instead of
// reaching the repository, where it previously either caused a DynamoDB
// ValidationException (500) or silently wrapped to an unintended value
// (finding W-limit).
func TestBets_InvalidLimitReturns400ValidationError(t *testing.T) {
	tests := []struct {
		name  string
		limit string
	}{
		{"non-numeric", "abc"},
		{"negative after int32 overflow", "2147483648"},
		{"wraps to a small positive after uint32 overflow", "4294967297"},
		{"negative", "-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			history := &fakeBetHistory{byUser: map[string][]domainbetslip.Bet{
				"user-a": {{ID: "bet-a1"}},
			}}
			verifier := security.NewJWT("test-secret", time.Hour)
			r := newBetsRouter(t, history, verifier)

			token, _, err := verifier.Issue(account.User{ID: "user-a", Email: "a@apuestatotal.com"})
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodGet, "/bets?limit="+tt.limit, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, http.StatusBadRequest, w.Code)
			var body map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.Equal(t, "VALIDATION_ERROR", body["error"].(map[string]any)["code"])
		})
	}
}
