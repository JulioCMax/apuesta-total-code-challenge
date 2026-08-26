package apperror_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/apperror"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/security"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/account"
	domainbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/betslip"
	domainevent "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/event"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newTestContext builds a *gin.Context with a request ID already stashed,
// mirroring what middleware.RequestID does in the real router (spec: api-
// platform/Consistent Error Envelope).
func newTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(apperror.RequestIDContextKey, "test-request-id")
	return c, w
}

// decodeEnvelope unmarshals w's body into apperror.Envelope, failing the
// test immediately if it does not match the single shared shape (spec: api-
// platform/Consistent Error Envelope).
func decodeEnvelope(t *testing.T, w *httptest.ResponseRecorder) apperror.Envelope {
	t.Helper()
	var env apperror.Envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	return env
}

// TestWrite_MapsTypedDomainErrorsToTheirDocumentedStatusAndCode proves every
// typed domain error from design.md's error-mapping table produces the
// exact HTTP status and code pinned there.
func TestWrite_MapsTypedDomainErrorsToTheirDocumentedStatusAndCode(t *testing.T) {
	minStake, err := money.NewMoneyFromFloat(1)
	require.NoError(t, err)
	maxStake, err := money.NewMoneyFromFloat(10000)
	require.NoError(t, err)
	stake, err := money.NewMoneyFromFloat(50000)
	require.NoError(t, err)

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"same event combo", domainbetslip.ErrSameEventCombo{EventID: "evt-1"}, http.StatusUnprocessableEntity, "SAME_EVENT_COMBO"},
		{"stake out of range", domainbetslip.StakeOutOfRangeError{Min: minStake, Max: maxStake, Got: stake}, http.StatusUnprocessableEntity, "STAKE_OUT_OF_RANGE"},
		{"selection not found", domainbetslip.ErrSelectionNotFound{SelectionID: "sel-1"}, http.StatusUnprocessableEntity, "SELECTION_NOT_FOUND"},
		{"duplicate selection", domainbetslip.ErrDuplicateSelection, http.StatusUnprocessableEntity, "DUPLICATE_SELECTION"},
		{"too many selections", domainbetslip.ErrTooManySelections, http.StatusUnprocessableEntity, "TOO_MANY_SELECTIONS"},
		{"selection unavailable", domainbetslip.ErrSelectionUnavailable, http.StatusUnprocessableEntity, "SELECTION_UNAVAILABLE"},
		{"empty slip", domainbetslip.ErrEmptySlip, http.StatusBadRequest, "VALIDATION_ERROR"},
		{"idempotency key reused", domainbetslip.ErrIdempotencyKeyReuse, http.StatusConflict, "IDEMPOTENCY_KEY_REUSED"},
		{"event not found", domainevent.ErrEventNotFound, http.StatusNotFound, "EVENT_NOT_FOUND"},
		{"invalid date range", domainevent.ErrInvalidDateRange, http.StatusBadRequest, "INVALID_DATE_RANGE"},
		{"invalid credentials", account.ErrInvalidCredentials, http.StatusUnauthorized, "INVALID_CREDENTIALS"},
		{"invalid token", security.ErrInvalidToken, http.StatusUnauthorized, "UNAUTHORIZED"},
		{"unmapped error falls back to internal", errUnmapped, http.StatusInternalServerError, "INTERNAL_ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, w := newTestContext()

			apperror.Write(c, tt.err)

			require.Equal(t, tt.wantStatus, w.Code)
			env := decodeEnvelope(t, w)
			require.Equal(t, tt.wantCode, env.Error.Code)
			require.NotEmpty(t, env.Error.Message)
			require.Equal(t, "test-request-id", env.RequestID)
		})
	}
}

var errUnmapped = &customUnmappedError{}

type customUnmappedError struct{}

func (*customUnmappedError) Error() string { return "boom" }

// TestWrite_InsufficientFundsCarriesNumericDetails proves the 409 envelope
// for a rejected placement carries betId/status/rejectionReason/balance/
// required inside details, with balance and required as unquoted numeric
// JSON (design.md's PlaceBet Transaction "Responses" example), not strings.
func TestWrite_InsufficientFundsCarriesNumericDetails(t *testing.T) {
	balance, err := money.NewMoneyFromFloat(50)
	require.NoError(t, err)
	required, err := money.NewMoneyFromFloat(100)
	require.NoError(t, err)

	c, w := newTestContext()

	apperror.Write(c, domainbetslip.ErrInsufficientFunds{BetID: "01J...", Balance: balance, Required: required})

	require.Equal(t, http.StatusConflict, w.Code)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	errObj := raw["error"].(map[string]any)
	require.Equal(t, "INSUFFICIENT_FUNDS", errObj["code"])
	details := errObj["details"].(map[string]any)
	require.Equal(t, "01J...", details["betId"])
	require.InDelta(t, 50.00, details["balance"], 0.001)
	require.InDelta(t, 100.00, details["required"], 0.001)
}

// TestWriteStatus_WritesTheSameEnvelopeShapeForNonDomainFailures proves
// WriteStatus (used by validation/auth/rate-limit/recovery middleware,
// none of which carry a Go error value) produces the identical envelope
// shape as Write (spec: api-platform/Consistent Error Envelope).
func TestWriteStatus_WritesTheSameEnvelopeShapeForNonDomainFailures(t *testing.T) {
	c, w := newTestContext()

	apperror.WriteStatus(c, http.StatusTooManyRequests, "RATE_LIMITED", "Límite excedido.")

	require.Equal(t, http.StatusTooManyRequests, w.Code)
	env := decodeEnvelope(t, w)
	require.Equal(t, "RATE_LIMITED", env.Error.Code)
	require.Equal(t, "Límite excedido.", env.Error.Message)
	require.Equal(t, "test-request-id", env.RequestID)
	require.Nil(t, env.Error.Details)
}

// TestRequestID_ReturnsEmptyWhenNeverSet proves RequestID never panics when
// no middleware has stashed a value yet (e.g. a unit test that builds a
// bare gin.Context).
func TestRequestID_ReturnsEmptyWhenNeverSet(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	require.Equal(t, "", apperror.RequestID(c))
}
