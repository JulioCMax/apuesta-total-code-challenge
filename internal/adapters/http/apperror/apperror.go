// Package apperror renders the single error envelope every endpoint uses
// (spec: api-platform/Consistent Error Envelope), and maps every typed
// domain error listed in design.md's HTTP Layer error-mapping table to its
// documented HTTP status and code.
//
// It is a leaf package deliberately separated from adapters/http (the
// router) and adapters/http/handler: router.go must import handler to
// register routes, so handler cannot import a top-level adapters/http
// package without creating an import cycle. adapters/http/middleware also
// needs this same mapping (rate-limit, auth guard, recovery all write the
// same envelope). apperror imports neither, so both can depend on it
// freely.
package apperror

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/security"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/account"
	domainbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/betslip"
	domainevent "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/event"
)

// RequestIDContextKey is the gin.Context key middleware.RequestID stashes
// the per-request identifier under. Defined here (not in middleware) so
// this package never needs to import middleware to read it back.
const RequestIDContextKey = "request_id"

// Body is the "error" object inside Envelope.
type Body struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// Envelope is the single JSON shape every failure response uses, whether it
// is a validation error, a typed domain error, an auth failure, a rate
// limit, or an unexpected server error (spec: api-platform/Consistent Error
// Envelope).
type Envelope struct {
	Error     Body   `json:"error"`
	RequestID string `json:"requestId"`
}

// RequestID returns the request ID stashed by middleware.RequestID, or ""
// when none was set (e.g. a unit test that never wired that middleware).
func RequestID(c *gin.Context) string {
	v, ok := c.Get(RequestIDContextKey)
	if !ok {
		return ""
	}
	id, _ := v.(string)
	return id
}

// Write classifies err via Classify and writes the resulting Envelope.
func Write(c *gin.Context, err error) {
	status, code, message, details := Classify(err)
	c.JSON(status, Envelope{Error: Body{Code: code, Message: message, Details: details}, RequestID: RequestID(c)})
}

// WriteStatus writes an Envelope for a failure that never carried a Go
// error value in the first place (bind/validation failures, the rate
// limiter, the JWT guard, panic recovery).
func WriteStatus(c *gin.Context, status int, code, message string) {
	WriteDetails(c, status, code, message, nil)
}

// WriteDetails is WriteStatus plus a details payload (used by the place
// handler's 409 rejected-placement response, D15).
func WriteDetails(c *gin.Context, status int, code, message string, details map[string]any) {
	c.JSON(status, Envelope{Error: Body{Code: code, Message: message, Details: details}, RequestID: RequestID(c)})
}

// Classify maps err to its HTTP status, error code, Spanish user-facing
// message, and optional details, per design.md's HTTP Layer error table.
// An error matching none of the typed cases below is INTERNAL_ERROR/500 —
// callers must never leak an unmapped error's own message to the caller.
func Classify(err error) (status int, code, message string, details map[string]any) {
	var sameEvent domainbetslip.ErrSameEventCombo
	var betBuilderNotAvailable domainbetslip.ErrBetBuilderNotAvailable
	var stakeRange domainbetslip.StakeOutOfRangeError
	var selNotFound domainbetslip.ErrSelectionNotFound
	var insufficientFunds domainbetslip.ErrInsufficientFunds

	switch {
	case errors.As(err, &insufficientFunds):
		return http.StatusConflict, "INSUFFICIENT_FUNDS", "Saldo insuficiente para realizar la apuesta.", map[string]any{
			"betId":           insufficientFunds.BetID,
			"status":          string(domainbetslip.BetStatusRejected),
			"rejectionReason": domainbetslip.RejectionReasonInsufficientFunds,
			"balance":         insufficientFunds.Balance,
			"required":        insufficientFunds.Required,
		}
	case errors.As(err, &sameEvent):
		return http.StatusUnprocessableEntity, "SAME_EVENT_COMBO", "La combinada no puede incluir dos selecciones del mismo evento.", map[string]any{
			"eventId": sameEvent.EventID,
		}
	case errors.As(err, &betBuilderNotAvailable):
		return http.StatusUnprocessableEntity, "BET_BUILDER_NOT_AVAILABLE", "El Bet Builder no está disponible para este evento.", map[string]any{
			"eventId": betBuilderNotAvailable.EventID,
		}
	case errors.As(err, &stakeRange):
		return http.StatusUnprocessableEntity, "STAKE_OUT_OF_RANGE", "El monto de la apuesta está fuera del rango permitido.", map[string]any{
			"min": stakeRange.Min,
			"max": stakeRange.Max,
			"got": stakeRange.Got,
		}
	case errors.As(err, &selNotFound):
		return http.StatusUnprocessableEntity, "SELECTION_NOT_FOUND", "Una de las selecciones indicadas no existe.", map[string]any{
			"selectionId": selNotFound.SelectionID,
		}
	case errors.Is(err, domainbetslip.ErrDuplicateSelection):
		return http.StatusUnprocessableEntity, "DUPLICATE_SELECTION", "No se puede repetir la misma selección en la apuesta.", nil
	case errors.Is(err, domainbetslip.ErrTooManySelections):
		return http.StatusUnprocessableEntity, "TOO_MANY_SELECTIONS", "Se superó el número máximo de selecciones permitidas.", nil
	case errors.Is(err, domainbetslip.ErrSelectionUnavailable):
		return http.StatusUnprocessableEntity, "SELECTION_UNAVAILABLE", "Una de las selecciones indicadas no está disponible.", nil
	case errors.Is(err, domainbetslip.ErrEmptySlip):
		return http.StatusBadRequest, "VALIDATION_ERROR", "Debe indicar al menos una selección.", nil
	case errors.Is(err, domainbetslip.ErrInvalidCursor):
		return http.StatusBadRequest, "VALIDATION_ERROR", "El cursor de paginación no es válido.", nil
	case errors.Is(err, domainbetslip.ErrIdempotencyKeyReuse):
		return http.StatusConflict, "IDEMPOTENCY_KEY_REUSED", "La clave de idempotencia ya fue utilizada con datos diferentes.", nil
	case errors.Is(err, domainbetslip.ErrConcurrencyConflict):
		// Transient contention, not a defect: nothing was persisted and
		// nothing was debited, so the caller may safely retry the exact
		// same request. 503 says that; an unclassified 500 would not.
		return http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "El servicio está temporalmente ocupado. Vuelva a intentarlo en unos instantes.", nil
	case errors.Is(err, domainevent.ErrEventNotFound):
		return http.StatusNotFound, "EVENT_NOT_FOUND", "El evento solicitado no existe.", nil
	case errors.Is(err, domainevent.ErrInvalidDateRange):
		return http.StatusBadRequest, "INVALID_DATE_RANGE", "El rango de fechas indicado no es válido.", nil
	case errors.Is(err, account.ErrInvalidCredentials):
		return http.StatusUnauthorized, "INVALID_CREDENTIALS", "Credenciales inválidas.", nil
	case errors.Is(err, security.ErrInvalidToken):
		return http.StatusUnauthorized, "UNAUTHORIZED", "El token proporcionado no es válido o expiró.", nil
	default:
		return http.StatusInternalServerError, "INTERNAL_ERROR", "Ocurrió un error interno inesperado.", nil
	}
}
