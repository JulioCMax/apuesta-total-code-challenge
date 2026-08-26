// Package account holds the demo User entity: identity, credential hash,
// and balance. Balance mutations happen exclusively through
// application/betslip.Place and its DynamoDB-backed BetRepository — this
// package only constructs and validates the type.
package account

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

// User is a demo account: a login identity plus its current balance.
type User struct {
	ID           string
	Email        string
	PasswordHash string
	Balance      money.Money
	Currency     string
	CreatedAt    time.Time
}

// NewUser builds a User, enforcing the non-negative balance invariant
// shared by every Money value (money.NewMoney rejects negative amounts).
func NewUser(id, email, passwordHash string, balance decimal.Decimal, currency string, createdAt time.Time) (User, error) {
	bal, err := money.NewMoney(balance)
	if err != nil {
		return User{}, fmt.Errorf("account: invalid balance: %w", err)
	}

	return User{
		ID:           id,
		Email:        email,
		PasswordHash: passwordHash,
		Balance:      bal,
		Currency:     currency,
		CreatedAt:    createdAt,
	}, nil
}
