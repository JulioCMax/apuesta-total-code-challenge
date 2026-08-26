package handler_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/handler"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/middleware"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/security"
	appauth "github.com/JulioCMax/apuesta-total-code-challenge/internal/application/auth"
	appbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/application/betslip"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/account"
	domainbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/betslip"
	domainevent "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/event"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

func jsonBody(s string) io.Reader {
	return strings.NewReader(s)
}

// fakeIDGenerator is a deterministic test double for
// application/betslip.IDGenerator.
type fakeIDGenerator struct {
	mu  sync.Mutex
	n   int
	pfx string
}

func (f *fakeIDGenerator) NewID() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	return f.pfx + "-" + time.Now().UTC().Format("150405") + "-" + itoa(f.n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// fakeBetRepo is a test double for application/betslip.BetRepository,
// mirroring D8/D15/D16's contract: an insufficient balance persists (not
// just returns) a rejected bet, and a repeated Idempotency-Key returns the
// recorded outcome without re-evaluating anything.
type fakeBetRepo struct {
	mu          sync.Mutex
	balance     money.Money
	byIdemKey   map[string]domainbetslip.Bet
	placedCount int
}

func newFakeBetRepo(balance money.Money) *fakeBetRepo {
	return &fakeBetRepo{balance: balance, byIdemKey: map[string]domainbetslip.Bet{}}
}

func (f *fakeBetRepo) Place(_ context.Context, b domainbetslip.Bet, idempotencyKey string) (domainbetslip.Bet, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if idempotencyKey != "" {
		if recorded, ok := f.byIdemKey[idempotencyKey]; ok {
			return recorded, true, nil
		}
	}

	f.placedCount++
	if b.Stake.GreaterThan(f.balance) {
		rejected := b
		rejected.Status = domainbetslip.BetStatusRejected
		rejected.RejectionReason = domainbetslip.RejectionReasonInsufficientFunds
		if idempotencyKey != "" {
			f.byIdemKey[idempotencyKey] = rejected
		}
		return domainbetslip.Bet{}, false, domainbetslip.ErrInsufficientFunds{
			BetID: rejected.ID, Balance: f.balance, Required: b.Stake,
		}
	}

	f.balance = f.balance.Sub(b.Stake)
	accepted := b
	accepted.Status = domainbetslip.BetStatusAccepted
	if idempotencyKey != "" {
		f.byIdemKey[idempotencyKey] = accepted
	}
	return accepted, false, nil
}

func (f *fakeBetRepo) ListByUser(_ context.Context, _ string, _ int, _ string) ([]domainbetslip.Bet, string, error) {
	return nil, "", nil
}

// fakeBalanceUsers adapts fakeBetRepo's tracked balance to
// application/auth.UserRepository so handler.NewAuth's Balance use case can
// read the same post-placement balance the place handler needs.
type fakeBalanceUsers struct {
	repo *fakeBetRepo
}

func (f *fakeBalanceUsers) FindByEmail(context.Context, string) (account.User, error) {
	return account.User{}, account.ErrInvalidCredentials
}

func (f *fakeBalanceUsers) Balance(context.Context, string) (money.Money, error) {
	f.repo.mu.Lock()
	defer f.repo.mu.Unlock()
	return f.repo.balance, nil
}

func newPlaceRouter(t *testing.T, catalog *fakeCatalog, repo *fakeBetRepo, verifier *security.JWT, bounds appbetslip.StakeBounds) *gin.Engine {
	t.Helper()
	balanceUseCase := appauth.NewBalance(&fakeBalanceUsers{repo: repo})
	place := appbetslip.NewPlace(catalog, repo, &fakeIDGenerator{pfx: "bet"}, bounds)
	h := handler.NewBetSlip(nil, place, balanceUseCase)

	r := gin.New()
	protected := r.Group("/")
	protected.Use(middleware.JWTAuth(verifier))
	protected.POST("/betslip/place", h.Place)
	return r
}

func tokenFor(t *testing.T, verifier *security.JWT, userID string) string {
	t.Helper()
	token, _, err := verifier.Issue(account.User{ID: userID, Email: userID + "@apuestatotal.com"})
	require.NoError(t, err)
	return token
}

// TestPlace_RequiresJWT proves an unauthenticated place request is rejected
// with 401 and never touches balance or bet state (spec: bet-slip-
// placement/JWT-Guarded Placement).
func TestPlace_RequiresJWT(t *testing.T) {
	catalog := newFakeCatalog(domainevent.SelectionRef{})
	repo := newFakeBetRepo(mustMoney(t, 1000))
	verifier := security.NewJWT("test-secret", time.Hour)
	r := newPlaceRouter(t, catalog, repo, verifier, testBounds(t))

	req := httptest.NewRequest(http.MethodPost, "/betslip/place", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Equal(t, 0, repo.placedCount, "an unauthenticated request must never touch bet/balance state")
}

// TestPlace_AcceptedReturns201WithBalanceAfter proves a sufficient-balance
// placement is accepted, debits the balance, and returns 201 with the
// stored bet plus the caller's fresh balance (spec: bet-slip-placement/
// Atomic Conditional Debit and Bet Persistence, "Sufficient balance").
func TestPlace_AcceptedReturns201WithBalanceAfter(t *testing.T) {
	catalog := newFakeCatalog(domainevent.SelectionRef{ID: "s1", EventID: "e1", Odds: mustOdds(t, 1.85)})
	repo := newFakeBetRepo(mustMoney(t, 1000))
	verifier := security.NewJWT("test-secret", time.Hour)
	r := newPlaceRouter(t, catalog, repo, verifier, testBounds(t))

	req := httptest.NewRequest(http.MethodPost, "/betslip/place", jsonBody(`{"selectionIds":["s1"],"stake":100}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenFor(t, verifier, "user-1"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	// money.Money has no UnmarshalJSON (D13's minimal API), so this
	// assertion reads the raw JSON structure instead of re-hydrating
	// dto.PlaceResponse.
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "accepted", body["status"])
	require.InDelta(t, 900.00, body["balanceAfter"], 0.001)
}

// TestPlace_InsufficientFundsReturns409WithRejectedStatusInDetails proves a
// balance-insufficient placement surfaces as 409 with details.status ==
// "rejected" (spec: bet-slip-placement/Bet Status Assignment, /Atomic
// Conditional Debit and Bet Persistence "Insufficient balance").
func TestPlace_InsufficientFundsReturns409WithRejectedStatusInDetails(t *testing.T) {
	catalog := newFakeCatalog(domainevent.SelectionRef{ID: "s1", EventID: "e1", Odds: mustOdds(t, 1.85)})
	repo := newFakeBetRepo(mustMoney(t, 50))
	verifier := security.NewJWT("test-secret", time.Hour)
	r := newPlaceRouter(t, catalog, repo, verifier, testBounds(t))

	req := httptest.NewRequest(http.MethodPost, "/betslip/place", jsonBody(`{"selectionIds":["s1"],"stake":100}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tokenFor(t, verifier, "user-1"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusConflict, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	errObj := body["error"].(map[string]any)
	require.Equal(t, "INSUFFICIENT_FUNDS", errObj["code"])
	details := errObj["details"].(map[string]any)
	require.Equal(t, "rejected", details["status"])
	require.Equal(t, "insufficient_funds", details["rejectionReason"])
	require.InDelta(t, 50.00, details["balance"], 0.001)
}

// TestPlace_ReplayReturns200WithHeader proves retrying the same
// Idempotency-Key returns the original result with the Idempotent-Replay
// header, no second bet, and no second debit (spec: bet-slip-placement/
// Idempotent Placement via Idempotency-Key).
func TestPlace_ReplayReturns200WithHeader(t *testing.T) {
	catalog := newFakeCatalog(domainevent.SelectionRef{ID: "s1", EventID: "e1", Odds: mustOdds(t, 1.85)})
	repo := newFakeBetRepo(mustMoney(t, 1000))
	verifier := security.NewJWT("test-secret", time.Hour)
	r := newPlaceRouter(t, catalog, repo, verifier, testBounds(t))
	token := tokenFor(t, verifier, "user-1")

	first := httptest.NewRequest(http.MethodPost, "/betslip/place", jsonBody(`{"selectionIds":["s1"],"stake":100}`))
	first.Header.Set("Content-Type", "application/json")
	first.Header.Set("Authorization", "Bearer "+token)
	first.Header.Set("Idempotency-Key", "key-1")
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, first)
	require.Equal(t, http.StatusCreated, w1.Code)

	second := httptest.NewRequest(http.MethodPost, "/betslip/place", jsonBody(`{"selectionIds":["s1"],"stake":100}`))
	second.Header.Set("Content-Type", "application/json")
	second.Header.Set("Authorization", "Bearer "+token)
	second.Header.Set("Idempotency-Key", "key-1")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, second)

	require.Equal(t, http.StatusOK, w2.Code)
	require.Equal(t, "true", w2.Header().Get("Idempotent-Replay"))
	require.Equal(t, 1, repo.placedCount, "a replay must never create a second bet")

	var body map[string]any
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &body))
	require.InDelta(t, 900.00, body["balanceAfter"], 0.001, "a replay must never apply a second debit")
}

// TestPlace_AbsurdStakeMagnitudeReturns400WithoutPlacing proves the
// placement endpoint applies the exact same request-boundary stake
// magnitude guard as calculate, and never reaches the repository.
func TestPlace_AbsurdStakeMagnitudeReturns400WithoutPlacing(t *testing.T) {
	catalog := newFakeCatalog(domainevent.SelectionRef{ID: "s1", EventID: "e1", Odds: mustOdds(t, 1.85)})
	repo := newFakeBetRepo(mustMoney(t, 1000))
	verifier := security.NewJWT("test-secret", time.Hour)
	r := newPlaceRouter(t, catalog, repo, verifier, testBounds(t))

	type result struct {
		code int
		body []byte
	}
	done := make(chan result, 1)
	go func() {
		req := httptest.NewRequest(http.MethodPost, "/betslip/place", jsonBody(`{"selectionIds":["s1"],"stake":1e10000000}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tokenFor(t, verifier, "user-1"))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		done <- result{code: w.Code, body: w.Body.Bytes()}
	}()

	select {
	case got := <-done:
		require.Equal(t, http.StatusBadRequest, got.code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(got.body, &body))
		require.Equal(t, "VALIDATION_ERROR", body["error"].(map[string]any)["code"])
		require.Equal(t, 0, repo.placedCount, "an out-of-range stake must never reach the repository")
	case <-time.After(2 * time.Second):
		t.Fatal("POST /betslip/place did not answer within 2s: the request reached decimal rounding")
	}
}
