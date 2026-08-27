package memory_test

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/betslip"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/event"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

// Real seeded event/selection IDs used only by this file's real-catalog
// Bet Builder rule proofs. Every synthetic-fixture case for these same
// rules already lives in internal/domain/betslip/betslip_test.go; this
// file exists specifically to prove the rules against selections actually
// resolved through memory.NewEventRepository, which is what caught the
// order-dependence defect the synthetic fixtures alone missed.
const (
	realStakeFixture = "100.00"

	// México vs Sudáfrica — Bet Builder enabled (data.json's own flag; no
	// overlay applies to this event).
	realEnabledSelA = "0ML784926076341366984H" // México
	realEnabledSelB = "0ML784926076341366984D" // Empate
	realEnabledEvID = "784926067864698880"

	// Catar vs Suiza — Bet Builder disabled by the authored overlay
	// (seed.BetBuilderDisabled; see internal/adapters/memory/seed/betbuilder.go).
	realDisabledSelA = "0ML784926076341379076D" // Empate
	realDisabledSelB = "0ML784926076341379076H" // Catar
	realDisabledEvID = "784926068556738560"
)

// realMustMoney is a test-only convenience wrapper around money.NewMoney,
// local to this package (internal/domain/betslip's own mustMoney lives in
// a different package and is not exported).
func realMustMoney(t *testing.T, amount string) money.Money {
	t.Helper()
	m, err := money.NewMoney(decimal.RequireFromString(amount))
	require.NoError(t, err)
	return m
}

// realSelections resolves ids against the real embedded catalog, failing
// the test immediately on any lookup error.
func realSelections(t *testing.T, repo interface {
	SelectionsByIDs(ctx context.Context, ids []string) ([]event.SelectionRef, error)
}, ids ...string) []event.SelectionRef {
	t.Helper()
	refs, err := repo.SelectionsByIDs(context.Background(), ids)
	require.NoError(t, err)
	require.Len(t, refs, len(ids))
	return refs
}

// TestRealCatalog_SameEventCombo_NoOptIn proves the separately-graded
// same-event rule against real seeded selections, not just synthetic
// fixtures: two real selections from one real event, no Bet Builder
// opt-in, must be rejected with ErrSameEventCombo (spec: bet-slip-
// calculation/Same-Event Combo Rejection).
func TestRealCatalog_SameEventCombo_NoOptIn(t *testing.T) {
	repo := newRepo(t)
	selections := realSelections(t, repo, realEnabledSelA, realEnabledSelB)

	slip := betslip.BetSlip{
		Selections: selections,
		Stake:      realMustMoney(t, realStakeFixture),
	}

	_, err := slip.Quote(realMustMoney(t, "1.00"), realMustMoney(t, "10000.00"), 20)

	require.Error(t, err)
	var sameEvent betslip.ErrSameEventCombo
	require.ErrorAs(t, err, &sameEvent)
	require.Equal(t, realEnabledEvID, sameEvent.EventID)
}

// TestRealCatalog_BetBuilderOptIn_OnOverlayDisabledEvent proves the Bet
// Builder gate against a real seeded event the authored overlay disables:
// opting in still must not bypass the rule, and the error must be the
// distinct ErrBetBuilderNotAvailable (spec: bet-slip-calculation/Same-
// Event Combo Rejection, "Bet Builder opt-in on a disabled event"; design:
// Bet Builder rule).
func TestRealCatalog_BetBuilderOptIn_OnOverlayDisabledEvent(t *testing.T) {
	repo := newRepo(t)
	selections := realSelections(t, repo, realDisabledSelA, realDisabledSelB)

	slip := betslip.BetSlip{
		Selections:          selections,
		Stake:               realMustMoney(t, realStakeFixture),
		AllowSameEventCombo: true,
	}

	_, err := slip.Quote(realMustMoney(t, "1.00"), realMustMoney(t, "10000.00"), 20)

	require.Error(t, err)
	var notAvailable betslip.ErrBetBuilderNotAvailable
	require.ErrorAs(t, err, &notAvailable)
	require.Equal(t, realDisabledEvID, notAvailable.EventID)
}

// TestRealCatalog_BetBuilderOptIn_MultiGroupDuplicate is the real-catalog
// proof of the multi-group defect FIX 1 repairs: a slip with 2+ real
// selections from a Bet Builder-enabled event AND 2+ real selections from
// an overlay-disabled event, opted in, must be refused with
// ErrBetBuilderNotAvailable naming the disabled event — in BOTH selection
// orders, so the order-dependence bug cannot regress silently.
func TestRealCatalog_BetBuilderOptIn_MultiGroupDuplicate(t *testing.T) {
	repo := newRepo(t)
	enabled := realSelections(t, repo, realEnabledSelA, realEnabledSelB)
	disabled := realSelections(t, repo, realDisabledSelA, realDisabledSelB)

	tests := []struct {
		name       string
		selections []event.SelectionRef
	}{
		{
			name:       "enabled event first",
			selections: append(append([]event.SelectionRef{}, enabled...), disabled...),
		},
		{
			name:       "disabled event first",
			selections: append(append([]event.SelectionRef{}, disabled...), enabled...),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slip := betslip.BetSlip{
				Selections:          tt.selections,
				Stake:               realMustMoney(t, realStakeFixture),
				AllowSameEventCombo: true,
			}

			_, err := slip.Quote(realMustMoney(t, "1.00"), realMustMoney(t, "10000.00"), 20)

			require.Error(t, err)
			var notAvailable betslip.ErrBetBuilderNotAvailable
			require.ErrorAs(t, err, &notAvailable)
			require.Equal(t, realDisabledEvID, notAvailable.EventID,
				"outcome must not depend on selection order")
		})
	}
}
