package betslip_test

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/betslip"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/event"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

// --- shared fixtures (task 4.4: deduped across every case in this file) ---

// newSelection builds a fixture event.SelectionRef with the given odds.
func newSelection(t *testing.T, id, eventID, odds string) event.SelectionRef {
	t.Helper()
	o, err := money.NewOdds(decimal.RequireFromString(odds))
	require.NoError(t, err)
	return event.SelectionRef{ID: id, EventID: eventID, MarketID: "ML0", Odds: o}
}

// newDisabledSelection is newSelection with IsDisabled set, for the
// "unavailable selection" scenarios.
func newDisabledSelection(t *testing.T, id, eventID, odds string) event.SelectionRef {
	t.Helper()
	sel := newSelection(t, id, eventID, odds)
	sel.IsDisabled = true
	return sel
}

// mustMoney is a test-only convenience wrapper around money.NewMoney.
func mustMoney(t *testing.T, amount string) money.Money {
	t.Helper()
	m, err := money.NewMoney(decimal.RequireFromString(amount))
	require.NoError(t, err)
	return m
}

const (
	minStakeFixture      = "1.00"
	maxStakeFixture      = "10000.00"
	maxSelectionsFixture = 20
)

// TestQuote_SingleSelectionHasNoCombo proves a one-selection slip produces
// exactly one Single and no Combo (spec: Single and Combo Bet Generation).
func TestQuote_SingleSelectionHasNoCombo(t *testing.T) {
	slip := betslip.BetSlip{
		Selections: []event.SelectionRef{newSelection(t, "sel-1", "evt-1", "1.85")},
		Stake:      mustMoney(t, "100.00"),
	}

	quote, err := slip.Quote(mustMoney(t, minStakeFixture), mustMoney(t, maxStakeFixture), maxSelectionsFixture)

	require.NoError(t, err)
	require.Len(t, quote.Singles, 1)
	require.Nil(t, quote.Combo)
	require.Equal(t, "185.00", quote.Singles[0].PotentialReturns.String())
}

// TestQuote_ComboOddsAreProductRounded proves the combined odds of a
// 2+ selection combo are the rounded product of the individual odds (D7),
// and that potentialReturns is derived from that already-rounded value.
func TestQuote_ComboOddsAreProductRounded(t *testing.T) {
	slip := betslip.BetSlip{
		Selections: []event.SelectionRef{
			newSelection(t, "sel-1", "evt-1", "1.85"),
			newSelection(t, "sel-2", "evt-2", "2.10"),
		},
		Stake: mustMoney(t, "100.00"),
	}

	quote, err := slip.Quote(mustMoney(t, minStakeFixture), mustMoney(t, maxStakeFixture), maxSelectionsFixture)

	require.NoError(t, err)
	require.Len(t, quote.Singles, 2)
	require.NotNil(t, quote.Combo)
	// 1.85 * 2.10 = 3.885 -> Round2 (half-up) -> 3.89
	require.Equal(t, "3.89", quote.Combo.Odds.String())
	// 100.00 * 3.89 = 389.00
	require.Equal(t, "389.00", quote.Combo.PotentialReturns.String())
}

// TestQuote_RejectsSameEventCombo proves 2+ selections from the same event
// are rejected with a typed error carrying the offending event ID (spec:
// bet-slip-calculation/Same-Event Combo Rejection).
func TestQuote_RejectsSameEventCombo(t *testing.T) {
	slip := betslip.BetSlip{
		Selections: []event.SelectionRef{
			newSelection(t, "sel-1", "evt-1", "1.85"),
			newSelection(t, "sel-2", "evt-1", "2.10"),
		},
		Stake: mustMoney(t, "100.00"),
	}

	_, err := slip.Quote(mustMoney(t, minStakeFixture), mustMoney(t, maxStakeFixture), maxSelectionsFixture)

	require.Error(t, err)
	var sameEventErr betslip.ErrSameEventCombo
	require.ErrorAs(t, err, &sameEventErr)
	require.Equal(t, "evt-1", sameEventErr.EventID)
}

// TestQuote_RejectsTooManySelections proves a slip exceeding the caller-
// supplied selection-count limit is rejected before any pricing happens.
func TestQuote_RejectsTooManySelections(t *testing.T) {
	slip := betslip.BetSlip{
		Selections: []event.SelectionRef{
			newSelection(t, "sel-1", "evt-1", "1.85"),
			newSelection(t, "sel-2", "evt-2", "1.85"),
			newSelection(t, "sel-3", "evt-3", "1.85"),
		},
		Stake: mustMoney(t, "100.00"),
	}

	_, err := slip.Quote(mustMoney(t, minStakeFixture), mustMoney(t, maxStakeFixture), 2)

	require.ErrorIs(t, err, betslip.ErrTooManySelections)
}

// TestQuote_RejectsDuplicateSelection proves repeating the exact same
// selection ID is rejected distinctly from a same-event combo.
func TestQuote_RejectsDuplicateSelection(t *testing.T) {
	slip := betslip.BetSlip{
		Selections: []event.SelectionRef{
			newSelection(t, "sel-1", "evt-1", "1.85"),
			newSelection(t, "sel-1", "evt-1", "1.85"),
		},
		Stake: mustMoney(t, "100.00"),
	}

	_, err := slip.Quote(mustMoney(t, minStakeFixture), mustMoney(t, maxStakeFixture), maxSelectionsFixture)

	require.ErrorIs(t, err, betslip.ErrDuplicateSelection)
}

// TestQuote_RejectsDisabledSelection proves a disabled selection is
// rejected before any bet is priced.
func TestQuote_RejectsDisabledSelection(t *testing.T) {
	slip := betslip.BetSlip{
		Selections: []event.SelectionRef{newDisabledSelection(t, "sel-1", "evt-1", "1.85")},
		Stake:      mustMoney(t, "100.00"),
	}

	_, err := slip.Quote(mustMoney(t, minStakeFixture), mustMoney(t, maxStakeFixture), maxSelectionsFixture)

	require.ErrorIs(t, err, betslip.ErrSelectionUnavailable)
}

// TestQuote_StakeBounds proves the stake is validated against the
// configured [min,max] bounds passed in by the caller (spec: bet-slip-
// calculation/Stake Bounds Validation) — never a hardcoded literal.
func TestQuote_StakeBounds(t *testing.T) {
	tests := []struct {
		name    string
		stake   string
		wantErr bool
	}{
		{name: "below minimum is rejected", stake: "0.99", wantErr: true},
		{name: "exactly at minimum is accepted", stake: "1.00", wantErr: false},
		{name: "exactly at maximum is accepted", stake: "10000.00", wantErr: false},
		{name: "above maximum is rejected", stake: "10000.01", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slip := betslip.BetSlip{
				Selections: []event.SelectionRef{newSelection(t, "sel-1", "evt-1", "1.85")},
				Stake:      mustMoney(t, tt.stake),
			}

			_, err := slip.Quote(mustMoney(t, minStakeFixture), mustMoney(t, maxStakeFixture), maxSelectionsFixture)

			if tt.wantErr {
				require.Error(t, err)
				var rangeErr betslip.StakeOutOfRangeError
				require.ErrorAs(t, err, &rangeErr)
				require.Equal(t, tt.stake, rangeErr.Got.String())
				return
			}
			require.NoError(t, err)
		})
	}
}
