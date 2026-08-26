package account_test

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/account"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

// TestNewUser_RejectsNegativeBalance proves User construction enforces the
// non-negative balance invariant shared by every Money value.
func TestNewUser_RejectsNegativeBalance(t *testing.T) {
	_, err := account.NewUser(
		"user-1", "demo@example.com", "hashed-password",
		decimal.NewFromFloat(-1), "PEN", time.Now().UTC(),
	)

	require.Error(t, err)
	require.ErrorIs(t, err, money.ErrNegativeAmount)
}

// TestNewUser_ConstructsWithValidBalance proves a non-negative balance is
// accepted and stored as a Money value, and that a distinct positive
// balance produces a distinct result (triangulation against the zero case).
func TestNewUser_ConstructsWithValidBalance(t *testing.T) {
	tests := []struct {
		name    string
		balance float64
		want    string
	}{
		{name: "zero balance is valid", balance: 0, want: "0.00"},
		{name: "positive balance is valid", balance: 1000, want: "1000.00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := account.NewUser(
				"user-1", "demo@example.com", "hashed-password",
				decimal.NewFromFloat(tt.balance), "PEN", time.Now().UTC(),
			)

			require.NoError(t, err)
			require.Equal(t, tt.want, u.Balance.String())
			require.Equal(t, "demo@example.com", u.Email)
		})
	}
}
