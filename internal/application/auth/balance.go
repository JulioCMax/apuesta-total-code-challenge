package auth

import (
	"context"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

// Balance is the "balance query" use case (spec: auth-and-balance/Balance
// Query).
type Balance struct {
	users UserRepository
}

// NewBalance builds a Balance use case backed by users.
func NewBalance(users UserRepository) *Balance {
	return &Balance{users: users}
}

// Execute returns the caller's current balance.
func (b *Balance) Execute(ctx context.Context, userID string) (money.Money, error) {
	return b.users.Balance(ctx, userID)
}
