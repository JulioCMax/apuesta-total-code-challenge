package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/handler"
	appbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/application/betslip"
	domainbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/betslip"
	domainevent "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/event"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

// fakeCatalog is a shared test double for application/betslip.EventCatalog,
// used by both the calculate and place handler tests.
type fakeCatalog struct {
	bySelectionID map[string]domainevent.SelectionRef
}

func newFakeCatalog(refs ...domainevent.SelectionRef) *fakeCatalog {
	byID := make(map[string]domainevent.SelectionRef, len(refs))
	for _, ref := range refs {
		byID[ref.ID] = ref
	}
	return &fakeCatalog{bySelectionID: byID}
}

func (f *fakeCatalog) SelectionsByIDs(_ context.Context, ids []string) ([]domainevent.SelectionRef, error) {
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

func testBounds(t *testing.T) appbetslip.StakeBounds {
	t.Helper()
	return appbetslip.StakeBounds{
		MinStake: mustMoney(t, 1), MaxStake: mustMoney(t, 10000), Currency: "PEN", MaxSelections: 20,
	}
}

func newCalculateRouter(catalog *fakeCatalog, bounds appbetslip.StakeBounds) *gin.Engine {
	h := handler.NewBetSlip(appbetslip.NewCalculate(catalog, bounds), nil, nil)
	r := gin.New()
	r.POST("/betslip/calculate", h.Calculate)
	return r
}

func doJSON(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestCalculate_ReturnsSinglesAndCombo proves two selections from distinct
// events return both a Single per selection AND a Combo (spec: bet-slip-
// calculation/Single and Combo Bet Generation).
func TestCalculate_ReturnsSinglesAndCombo(t *testing.T) {
	catalog := newFakeCatalog(
		domainevent.SelectionRef{ID: "s1", EventID: "e1", Odds: mustOdds(t, 1.85)},
		domainevent.SelectionRef{ID: "s2", EventID: "e2", Odds: mustOdds(t, 2.10)},
	)
	r := newCalculateRouter(catalog, testBounds(t))

	w := doJSON(t, r, http.MethodPost, "/betslip/calculate", `{"selectionIds":["s1","s2"],"stake":100}`)

	require.Equal(t, http.StatusOK, w.Code)
	// money.Money/Odds marshal as unquoted numbers with no matching
	// UnmarshalJSON (D13's minimal API — production code only ever
	// marshals them into a response, never decodes one back), so the
	// round-trip assertion below reads the raw JSON structure instead of
	// re-hydrating dto.CalculateResponse.
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body["singles"].([]any), 2)
	combo := body["combo"].(map[string]any)
	require.InDelta(t, 3.89, combo["combinedOdds"], 0.001) // Round2(1.85 * 2.10)
}

// TestCalculate_ResponseCarriesConfiguredStakeBounds proves minStake/
// maxStake come from configuration, never hardcoded literals (spec: bet-
// slip-calculation/Calculate Endpoint Response Shape).
func TestCalculate_ResponseCarriesConfiguredStakeBounds(t *testing.T) {
	catalog := newFakeCatalog(domainevent.SelectionRef{ID: "s1", EventID: "e1", Odds: mustOdds(t, 1.85)})
	bounds := appbetslip.StakeBounds{MinStake: mustMoney(t, 5), MaxStake: mustMoney(t, 500), Currency: "PEN", MaxSelections: 20}
	r := newCalculateRouter(catalog, bounds)

	w := doJSON(t, r, http.MethodPost, "/betslip/calculate", `{"selectionIds":["s1"],"stake":10}`)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.InDelta(t, 5.00, body["minStake"], 0.001)
	require.InDelta(t, 500.00, body["maxStake"], 0.001)
}

// TestCalculate_SameEventReturns422Envelope proves two selections from the
// same event surface the typed ErrSameEventCombo as a 422 in the standard
// envelope, distinct from a request-validation error (spec: bet-slip-
// calculation/Same-Event Combo Rejection).
func TestCalculate_SameEventReturns422Envelope(t *testing.T) {
	catalog := newFakeCatalog(
		domainevent.SelectionRef{ID: "s1", EventID: "e1", Odds: mustOdds(t, 1.85)},
		domainevent.SelectionRef{ID: "s2", EventID: "e1", Odds: mustOdds(t, 2.10)},
	)
	r := newCalculateRouter(catalog, testBounds(t))

	w := doJSON(t, r, http.MethodPost, "/betslip/calculate", `{"selectionIds":["s1","s2"],"stake":100}`)

	require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	errObj := body["error"].(map[string]any)
	require.Equal(t, "SAME_EVENT_COMBO", errObj["code"])
	require.Equal(t, "e1", errObj["details"].(map[string]any)["eventId"])
}

// TestCalculate_BetBuilderOptInAllowsSameEventCombo proves isBetBuilder
// threads through the HTTP layer and allows a same-event combo when the
// event itself has Bet Builder enabled (spec: bet-slip-calculation/Same-
// Event Combo Rejection, "Bet Builder opt-in on an enabled event"; /Bet
// Builder Explicit UI Affordance).
func TestCalculate_BetBuilderOptInAllowsSameEventCombo(t *testing.T) {
	catalog := newFakeCatalog(
		domainevent.SelectionRef{ID: "s1", EventID: "e1", Odds: mustOdds(t, 1.85), EventBetBuilderEnabled: true},
		domainevent.SelectionRef{ID: "s2", EventID: "e1", Odds: mustOdds(t, 2.10), EventBetBuilderEnabled: true},
	)
	r := newCalculateRouter(catalog, testBounds(t))

	w := doJSON(t, r, http.MethodPost, "/betslip/calculate", `{"selectionIds":["s1","s2"],"stake":100,"isBetBuilder":true}`)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.NotNil(t, body["combo"])
}

// TestCalculate_BetBuilderOptInOnDisabledEventReturns422BetBuilderNotAvailable
// proves the mandatory Phase 2 gap: opting into Bet Builder for an event
// whose Bet Builder is disabled classifies as the distinct
// BET_BUILDER_NOT_AVAILABLE code, never falling through to SAME_EVENT_COMBO
// or an unclassified error (design.md's apperror.Classify table;
// domain/betslip.ErrBetBuilderNotAvailable).
func TestCalculate_BetBuilderOptInOnDisabledEventReturns422BetBuilderNotAvailable(t *testing.T) {
	catalog := newFakeCatalog(
		domainevent.SelectionRef{ID: "s1", EventID: "e1", Odds: mustOdds(t, 1.85), EventBetBuilderEnabled: false},
		domainevent.SelectionRef{ID: "s2", EventID: "e1", Odds: mustOdds(t, 2.10), EventBetBuilderEnabled: false},
	)
	r := newCalculateRouter(catalog, testBounds(t))

	w := doJSON(t, r, http.MethodPost, "/betslip/calculate", `{"selectionIds":["s1","s2"],"stake":100,"isBetBuilder":true}`)

	require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	errObj := body["error"].(map[string]any)
	require.Equal(t, "BET_BUILDER_NOT_AVAILABLE", errObj["code"])
	require.Equal(t, "e1", errObj["details"].(map[string]any)["eventId"])
}

// TestCalculate_BoostedSelectionExposesOriginalOddsInResponse proves a
// resolved selection carrying a Super Cuota boost renders both odds and a
// distinct originalOdds in the JSON response (spec: bet-slip-calculation/
// Boosted Selection Exposes Original Odds).
func TestCalculate_BoostedSelectionExposesOriginalOddsInResponse(t *testing.T) {
	original := mustOdds(t, 1.47)
	catalog := newFakeCatalog(domainevent.SelectionRef{ID: "s1", EventID: "e1", Odds: mustOdds(t, 1.60), OriginalOdds: &original})
	r := newCalculateRouter(catalog, testBounds(t))

	w := doJSON(t, r, http.MethodPost, "/betslip/calculate", `{"selectionIds":["s1"],"stake":100}`)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	selections := body["selections"].([]any)
	require.Len(t, selections, 1)
	sel := selections[0].(map[string]any)
	require.InDelta(t, 1.60, sel["odds"], 0.001)
	require.InDelta(t, 1.47, sel["originalOdds"], 0.001)
}

// TestCalculate_NonBoostedSelectionOmitsOriginalOddsInResponse proves a
// non-boosted selection never renders originalOdds at all (spec: bet-slip-
// calculation/Boosted Selection Exposes Original Odds, "Non-boosted
// selection resolved").
func TestCalculate_NonBoostedSelectionOmitsOriginalOddsInResponse(t *testing.T) {
	catalog := newFakeCatalog(domainevent.SelectionRef{ID: "s1", EventID: "e1", Odds: mustOdds(t, 1.85)})
	r := newCalculateRouter(catalog, testBounds(t))

	w := doJSON(t, r, http.MethodPost, "/betslip/calculate", `{"selectionIds":["s1"],"stake":100}`)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	sel := body["selections"].([]any)[0].(map[string]any)
	require.NotContains(t, sel, "originalOdds")
}

// TestCalculate_UnknownSelectionReturns422Envelope proves an unresolved
// selection ID is a typed domain error, not a generic 400 (spec: bet-slip-
// calculation/Selection Resolution).
func TestCalculate_UnknownSelectionReturns422Envelope(t *testing.T) {
	r := newCalculateRouter(newFakeCatalog(), testBounds(t))

	w := doJSON(t, r, http.MethodPost, "/betslip/calculate", `{"selectionIds":["missing"],"stake":100}`)

	require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "SELECTION_NOT_FOUND", body["error"].(map[string]any)["code"])
}

// TestCalculate_MalformedBodyReturns400ValidationEnvelope proves a request
// that fails to bind (missing required fields) is a 400 VALIDATION_ERROR in
// the same standard envelope (spec: api-platform/Consistent Error
// Envelope).
func TestCalculate_MalformedBodyReturns400ValidationEnvelope(t *testing.T) {
	r := newCalculateRouter(newFakeCatalog(), testBounds(t))

	w := doJSON(t, r, http.MethodPost, "/betslip/calculate", `{"stake":100}`) // missing selectionIds

	require.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "VALIDATION_ERROR", body["error"].(map[string]any)["code"])
}

// TestCalculate_AbsurdStakeMagnitudeReturns400WithoutHanging proves the
// public, unauthenticated calculate endpoint rejects an attacker-supplied
// stake exponent through the standard VALIDATION_ERROR envelope, and does
// so promptly: without the request-boundary magnitude guard, rounding
// 1e10000000 to 2 decimal places burns seconds of CPU and hundreds of MB
// of allocation per request.
func TestCalculate_AbsurdStakeMagnitudeReturns400WithoutHanging(t *testing.T) {
	r := newCalculateRouter(newFakeCatalog(), testBounds(t))

	type result struct {
		code int
		body []byte
	}
	done := make(chan result, 1)
	go func() {
		w := doJSON(t, r, http.MethodPost, "/betslip/calculate", `{"selectionIds":["s1"],"stake":1e10000000}`)
		done <- result{code: w.Code, body: w.Body.Bytes()}
	}()

	select {
	case got := <-done:
		require.Equal(t, http.StatusBadRequest, got.code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(got.body, &body))
		require.Equal(t, "VALIDATION_ERROR", body["error"].(map[string]any)["code"])
	case <-time.After(2 * time.Second):
		t.Fatal("POST /betslip/calculate did not answer within 2s: an unauthenticated request reached decimal rounding")
	}
}
