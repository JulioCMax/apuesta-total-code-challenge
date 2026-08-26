package betslip

import (
	"errors"
	"fmt"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

var (
	// ErrEmptySlip is returned when a BetSlip has no selections.
	ErrEmptySlip = errors.New("betslip: no selections provided")

	// ErrTooManySelections is returned when a slip exceeds the configured
	// maximum selection count (BETSLIP_MAX_SELECTIONS).
	ErrTooManySelections = errors.New("betslip: too many selections")

	// ErrDuplicateSelection is returned when the same selection ID appears
	// more than once in a slip.
	ErrDuplicateSelection = errors.New("betslip: duplicate selection")

	// ErrSelectionUnavailable is returned when a requested selection is
	// disabled.
	ErrSelectionUnavailable = errors.New("betslip: selection is unavailable")

	// ErrIdempotencyKeyReuse is returned when a placement's Idempotency-Key
	// was already used with a different payload.
	ErrIdempotencyKeyReuse = errors.New("betslip: idempotency key reused with a different payload")
)

// ErrSameEventCombo is returned when 2+ selections in a combo belong to the
// same event, distinct from a request-validation (400) error.
type ErrSameEventCombo struct {
	EventID string
}

func (e ErrSameEventCombo) Error() string {
	return fmt.Sprintf("betslip: combo rejected, event %s repeated", e.EventID)
}

// StakeOutOfRangeError is returned when the requested stake falls outside
// the configured [Min, Max] bounds.
type StakeOutOfRangeError struct {
	Min money.Money
	Max money.Money
	Got money.Money
}

func (e StakeOutOfRangeError) Error() string {
	return fmt.Sprintf("betslip: stake %s out of range [%s, %s]", e.Got, e.Min, e.Max)
}

// ErrSelectionNotFound is returned when a requested selection ID does not
// exist in the event catalog.
type ErrSelectionNotFound struct {
	SelectionID string
}

func (e ErrSelectionNotFound) Error() string {
	return fmt.Sprintf("betslip: selection %s not found", e.SelectionID)
}

// ErrInsufficientFunds is returned when a placement's balance conditional
// debit fails. It carries the persisted rejected bet's identifiers (D15) so
// the caller can surface them without a second lookup.
type ErrInsufficientFunds struct {
	BetID    string
	Balance  money.Money
	Required money.Money
}

func (e ErrInsufficientFunds) Error() string {
	return fmt.Sprintf("betslip: insufficient funds: balance %s < required %s", e.Balance, e.Required)
}
