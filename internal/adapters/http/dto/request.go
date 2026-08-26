package dto

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

// LoginRequest is the JSON body of POST /auth/login.
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// BetSlipRequest is the shared JSON body of POST /betslip/calculate and
// POST /betslip/place: a set of selection IDs priced against a single
// stake. Stake is bound as json.Number (not float64) so the literal digits
// the caller sent survive intact into decimal.NewFromString — a stake of
// 33.10 must never become 33.099999999999994 (spec: bet-slip-calculation/
// Potential Returns Rounding).
type BetSlipRequest struct {
	SelectionIDs []string    `json:"selectionIds" binding:"required,min=1,dive,required"`
	Stake        json.Number `json:"stake" binding:"required"`
}

// Stake magnitude bounds. These are NOT the business stake bounds
// (BETSLIP_MIN/MAX_STAKE_AMOUNT, enforced by the domain against
// configuration): they are a resource guard applied before any decimal
// arithmetic runs.
//
// shopspring/decimal accepts any exponent that fits in an int32, and
// rounding such a value to 2 decimal places materialises a big.Int with
// one digit per unit of exponent. A single unauthenticated request
// carrying "stake": 1e10000000 — ten harmless-looking bytes — costs
// seconds of CPU and hundreds of MB of allocation, which is enough to kill
// a small container or a 512MB Lambda. The guard therefore rejects the
// literal on shape alone, before decimal.Decimal.Round is ever reached.
const (
	// maxStakeLiteralLength bounds the raw numeric text itself, so a body
	// full of digits can never be fed to big.Int parsing.
	maxStakeLiteralLength = 40

	// maxStakeMagnitudeDigits bounds the number of digits before the
	// decimal point (coefficient digits + exponent). 15 leaves several
	// orders of magnitude above any configurable maximum stake.
	maxStakeMagnitudeDigits = 15

	// maxStakeDecimalPlaces bounds the digits after the decimal point.
	// Money keeps 2; anything up to 20 still rounds in constant time, and
	// beyond that the rescale itself becomes the attack.
	maxStakeDecimalPlaces = 20
)

// ErrStakeMagnitude is returned by StakeMoney when the submitted stake
// literal is too long, or its magnitude/precision is so extreme that
// rounding it would be a denial-of-service vector. Handlers map it to the
// standard VALIDATION_ERROR envelope, exactly like a malformed amount.
var ErrStakeMagnitude = errors.New("dto: stake magnitude out of accepted range")

// StakeMoney parses r.Stake into a money.Money, rejecting a malformed,
// negative, or absurdly large/precise amount. The magnitude guard runs
// BEFORE money.NewMoney so no attacker-controlled value ever reaches
// decimal rounding.
func (r BetSlipRequest) StakeMoney() (money.Money, error) {
	raw := r.Stake.String()
	if len(raw) > maxStakeLiteralLength {
		return money.Money{}, fmt.Errorf("%w: literal longer than %d characters", ErrStakeMagnitude, maxStakeLiteralLength)
	}

	d, err := decimal.NewFromString(raw)
	if err != nil {
		return money.Money{}, err
	}
	if err := checkStakeMagnitude(d); err != nil {
		return money.Money{}, err
	}

	return money.NewMoney(d)
}

// checkStakeMagnitude reports whether d's magnitude and precision are
// within the bounds above. Both Exponent and NumDigits read the decimal's
// already-parsed representation (coefficient plus base-10 exponent) and
// never materialise the full number, so this check is constant time no
// matter how extreme the literal was.
func checkStakeMagnitude(d decimal.Decimal) error {
	exp := int(d.Exponent())
	if exp < -maxStakeDecimalPlaces {
		return fmt.Errorf("%w: more than %d decimal places", ErrStakeMagnitude, maxStakeDecimalPlaces)
	}
	if exp+d.NumDigits() > maxStakeMagnitudeDigits {
		return fmt.Errorf("%w: more than %d integer digits", ErrStakeMagnitude, maxStakeMagnitudeDigits)
	}
	return nil
}
