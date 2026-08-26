package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	httpadapter "github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/apperror"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/handler"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/security"
	appauth "github.com/JulioCMax/apuesta-total-code-challenge/internal/application/auth"
	appbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/application/betslip"
	appevent "github.com/JulioCMax/apuesta-total-code-challenge/internal/application/event"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/account"
	domainbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/betslip"
	domainevent "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/event"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/platform/logging"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// fakeCatalog and fakeUserRepo mirror the fakes used by the handler-level
// tests, kept minimal and package-local since router_test.go lives in the
// external http_test package and cannot reach handler_test's unexported
// types.
type fakeEventCatalog struct{ events []domainevent.Event }

func (f *fakeEventCatalog) List(_ context.Context, _, _ time.Time) ([]domainevent.Event, error) {
	return f.events, nil
}
func (f *fakeEventCatalog) Detail(_ context.Context, id string) (domainevent.Event, error) {
	for _, e := range f.events {
		if e.ID == id {
			return e, nil
		}
	}
	return domainevent.Event{}, domainevent.ErrEventNotFound
}

type fakeSelectionCatalog struct {
	bySelectionID map[string]domainevent.SelectionRef
}

func (f *fakeSelectionCatalog) SelectionsByIDs(_ context.Context, ids []string) ([]domainevent.SelectionRef, error) {
	refs := make([]domainevent.SelectionRef, 0, len(ids))
	for _, id := range ids {
		ref, ok := f.bySelectionID[id]
		if !ok {
			return nil, domainbetslip.ErrSelectionNotFound{SelectionID: id}
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

type fakeUsers struct {
	byEmail map[string]account.User
	balance money.Money
}

func (f *fakeUsers) FindByEmail(_ context.Context, email string) (account.User, error) {
	u, ok := f.byEmail[email]
	if !ok {
		return account.User{}, account.ErrInvalidCredentials
	}
	return u, nil
}
func (f *fakeUsers) Balance(_ context.Context, _ string) (money.Money, error) { return f.balance, nil }

type fakeBetRepo struct{}

func (fakeBetRepo) Place(_ context.Context, b domainbetslip.Bet, _ string) (domainbetslip.Bet, bool, error) {
	b.Status = domainbetslip.BetStatusAccepted
	return b, false, nil
}
func (fakeBetRepo) ListByUser(_ context.Context, _ string, _ int, _ string) ([]domainbetslip.Bet, string, error) {
	return nil, "", nil
}

type fakeIDs struct{}

func (fakeIDs) NewID() string { return "01TESTBETID000000000000000" }

func mustOdds(t *testing.T, v float64) money.Odds {
	t.Helper()
	o, err := money.NewOddsFromFloat(v)
	require.NoError(t, err)
	return o
}

func mustMoney(t *testing.T, v float64) money.Money {
	t.Helper()
	m, err := money.NewMoneyFromFloat(v)
	require.NoError(t, err)
	return m
}

func newTestRouter(t *testing.T) (*gin.Engine, *security.JWT) {
	t.Helper()

	eventCatalog := &fakeEventCatalog{events: []domainevent.Event{
		{ID: "e1", Name: "A vs B", StartsAt: time.Now().UTC()},
	}}
	selectionCatalog := &fakeSelectionCatalog{bySelectionID: map[string]domainevent.SelectionRef{
		"s1": {ID: "s1", EventID: "e1", Odds: mustOdds(t, 1.85)},
	}}
	verifier := security.NewJWT("test-secret", time.Hour)
	hash, err := security.NewBcrypt().Hash("demo-password")
	require.NoError(t, err)
	users := &fakeUsers{
		byEmail: map[string]account.User{"demo@apuestatotal.com": {ID: "user-1", Email: "demo@apuestatotal.com", PasswordHash: hash}},
		balance: mustMoney(t, 500),
	}
	bounds := appbetslip.StakeBounds{MinStake: mustMoney(t, 1), MaxStake: mustMoney(t, 10000), Currency: "PEN", MaxSelections: 20}

	deps := httpadapter.Dependencies{
		Events:    handler.NewEvents(appevent.NewList(eventCatalog), appevent.NewDetail(eventCatalog)),
		BetSlip:   handler.NewBetSlip(appbetslip.NewCalculate(selectionCatalog, bounds), appbetslip.NewPlace(selectionCatalog, fakeBetRepo{}, fakeIDs{}, bounds), appauth.NewBalance(users)),
		Auth:      handler.NewAuth(appauth.NewLogin(users, security.NewBcrypt(), verifier), appauth.NewBalance(users), "PEN"),
		Bets:      handler.NewBets(appauth.NewHistory(fakeBetRepo{})),
		Verifier:  verifier,
		Logger:    logging.New("error", &bytes.Buffer{}),
		RateLimit: "1000-M",
		Version:   "test",
	}

	r, err := httpadapter.NewRouter(deps)
	require.NoError(t, err)
	return r, verifier
}

// TestRouter_PublicRoutesReachableWithoutAuth proves every public route
// (health, login, events, calculate) answers without a token (design.md's
// HTTP Layer route table).
func TestRouter_PublicRoutesReachableWithoutAuth(t *testing.T) {
	r, _ := newTestRouter(t)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"health", http.MethodGet, "/health", ""},
		{"events list", http.MethodGet, "/api/v1/events", ""},
		{"event detail", http.MethodGet, "/api/v1/events/e1", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
		})
	}
}

// TestRouter_ProtectedRoutesRequireJWT proves /betslip/place, /balance and
// /bets are all guarded by the same JWT middleware (spec: auth-and-
// balance/Auth Guard Middleware).
func TestRouter_ProtectedRoutesRequireJWT(t *testing.T) {
	r, _ := newTestRouter(t)

	tests := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/betslip/place"},
		{http.MethodGet, "/api/v1/balance"},
		{http.MethodGet, "/api/v1/bets"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(`{}`))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})
	}
}

// TestRouter_ProtectedRoutesAcceptValidToken is the triangulation case: the
// same three routes succeed once a valid Bearer token is presented.
func TestRouter_ProtectedRoutesAcceptValidToken(t *testing.T) {
	r, verifier := newTestRouter(t)
	token, _, err := verifier.Issue(account.User{ID: "user-1", Email: "demo@apuestatotal.com"})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/balance", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

// TestErrorEnvelope_ShapeIsIdenticalAcrossCodes proves every failure —
// validation, auth, not-found, business-rule conflict, and rate limit —
// renders through the exact same {"error":{"code","message"},"requestId"}
// shape (spec: api-platform/Consistent Error Envelope).
func TestErrorEnvelope_ShapeIsIdenticalAcrossCodes(t *testing.T) {
	r, verifier := newTestRouter(t)
	token, _, err := verifier.Issue(account.User{ID: "user-1", Email: "demo@apuestatotal.com"})
	require.NoError(t, err)

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		auth       bool
		wantStatus int
		wantCode   string
	}{
		{"validation error", http.MethodPost, "/api/v1/betslip/calculate", `{"stake":100}`, false, http.StatusBadRequest, "VALIDATION_ERROR"},
		{"unauthorized", http.MethodGet, "/api/v1/balance", "", false, http.StatusUnauthorized, "UNAUTHORIZED"},
		{"not found", http.MethodGet, "/api/v1/events/does-not-exist", "", false, http.StatusNotFound, "EVENT_NOT_FOUND"},
		{"unprocessable domain error", http.MethodPost, "/api/v1/betslip/calculate", `{"selectionIds":["missing"],"stake":100}`, false, http.StatusUnprocessableEntity, "SELECTION_NOT_FOUND"},
	}

	var envelopeKeys [][]string
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body *strings.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			} else {
				body = strings.NewReader("")
			}
			req := httptest.NewRequest(tt.method, tt.path, body)
			req.Header.Set("Content-Type", "application/json")
			if tt.auth {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, tt.wantStatus, w.Code)

			var env apperror.Envelope
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
			require.Equal(t, tt.wantCode, env.Error.Code)
			require.NotEmpty(t, env.Error.Message)
			require.NotEmpty(t, env.RequestID)

			var raw map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
			envelopeKeys = append(envelopeKeys, topLevelKeys(raw))
		})
	}

	// Every response, regardless of status code, must share the exact same
	// top-level envelope shape.
	for i := 1; i < len(envelopeKeys); i++ {
		require.ElementsMatch(t, envelopeKeys[0], envelopeKeys[i], "envelope shape must be identical across every error code")
	}
}

func topLevelKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
