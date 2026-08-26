package money_test

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

// TestRound2_HalfUpBoundaries proves Round2 is the single half-up rounding
// function in the codebase and that it operates on exact decimal values,
// never on binary-float approximations (D6/D13).
func TestRound2_HalfUpBoundaries(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "half-cent rounds up", input: "1.005", want: "1.01"},
		{name: "exact decimal avoids binary-float drift", input: "2.675", want: "2.68"},
		{name: "smallest half-cent rounds up", input: "0.005", want: "0.01"},
		{name: "just below half-cent rounds down", input: "152.494999", want: "152.49"},
		{name: "exact half-cent rounds up at larger scale", input: "152.495", want: "152.50"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := decimal.RequireFromString(tt.input)
			got := money.Round2(in)
			want := decimal.RequireFromString(tt.want)
			require.True(t, got.Equal(want), "Round2(%s) = %s, want %s", tt.input, got.String(), want.String())
		})
	}
}

// TestMoney_JSONFixed2 proves Money marshals as an unquoted fixed-2 JSON
// number (D13), e.g. 100.00, not a quoted string and not a bare integer.
func TestMoney_JSONFixed2(t *testing.T) {
	m, err := money.NewMoneyFromFloat(100)
	require.NoError(t, err)

	raw, err := json.Marshal(m)
	require.NoError(t, err)
	require.Equal(t, "100.00", string(raw))

	m2, err := money.NewMoneyFromFloat(1234.5)
	require.NoError(t, err)
	raw2, err := json.Marshal(m2)
	require.NoError(t, err)
	require.Equal(t, "1234.50", string(raw2))
}

// TestMoney_SubDebitsExactAmount proves Sub performs exact decimal
// subtraction with no binary-float drift. Reintroduced (per the Unit 4
// trim) because the mutex-guarded fake BetRepository in the application
// layer's concurrency race test needs an exact balance debit (spec: bet-
// slip-placement/Concurrency-Safe Balance Debit).
func TestMoney_SubDebitsExactAmount(t *testing.T) {
	balance, err := money.NewMoneyFromFloat(100)
	require.NoError(t, err)
	stake, err := money.NewMoneyFromFloat(37.5)
	require.NoError(t, err)

	got := balance.Sub(stake)

	require.Equal(t, "62.50", got.String())
}

// TestOdds_CombineIsRound2Product proves combined odds are computed as the
// product of the individual odds, rounded exactly once via Round2 (D7).
func TestOdds_CombineIsRound2Product(t *testing.T) {
	a, err := money.NewOddsFromFloat(1.85)
	require.NoError(t, err)
	b, err := money.NewOddsFromFloat(2.10)
	require.NoError(t, err)

	combined := money.Combine(a, b)

	// 1.85 * 2.10 = 3.885 -> Round2 -> 3.89 (half-up)
	require.Equal(t, "3.89", combined.String())
}

// TestNewOdds_EnforcesMinimum proves Odds rejects anything below the house
// minimum of 1.01 and accepts values at or above it.
func TestNewOdds_EnforcesMinimum(t *testing.T) {
	tests := []struct {
		name    string
		odds    float64
		want    string
		wantErr bool
	}{
		{name: "below minimum is rejected", odds: 1.00, wantErr: true},
		{name: "exactly at minimum is accepted", odds: 1.01, want: "1.01"},
		{name: "well above minimum is accepted", odds: 2.50, want: "2.50"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := money.NewOddsFromFloat(tt.odds)
			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, money.ErrOddsTooLow)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got.String())
		})
	}
}
