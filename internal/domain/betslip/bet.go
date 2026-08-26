package betslip

import (
	"time"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

// BetType identifies whether a placed Bet is a Single or a Combo.
type BetType string

const (
	BetTypeSingle BetType = "single"
	BetTypeCombo  BetType = "combo"
)

// BetStatus is the persisted outcome of a placement attempt. Exactly two
// values are produced today; won/lost/void are documented as the future
// settlement extension (D15).
type BetStatus string

const (
	// BetStatusAccepted means the balance was debited and the bet is live.
	BetStatusAccepted BetStatus = "accepted"
	// BetStatusRejected means the balance was left untouched; RejectionReason
	// is always set (e.g. "insufficient_funds").
	BetStatusRejected BetStatus = "rejected"
)

// RejectionReasonInsufficientFunds is the only rejection reason produced
// today: the debit's conditional check failed.
const RejectionReasonInsufficientFunds = "insufficient_funds"

// Bet is the persisted result of placing a priced BetSlip.
type Bet struct {
	ID               string
	UserID           string
	Type             BetType
	Stake            money.Money
	CombinedOdds     money.Odds
	PotentialReturns money.Money
	Status           BetStatus
	RejectionReason  string
	Selections       []string // selection IDs, in slip order
	CreatedAt        time.Time
}
