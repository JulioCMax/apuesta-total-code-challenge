package dto

import (
	"encoding/json"

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

// StakeMoney parses r.Stake into a money.Money, rejecting a negative or
// malformed amount.
func (r BetSlipRequest) StakeMoney() (money.Money, error) {
	d, err := decimal.NewFromString(r.Stake.String())
	if err != nil {
		return money.Money{}, err
	}
	return money.NewMoney(d)
}
