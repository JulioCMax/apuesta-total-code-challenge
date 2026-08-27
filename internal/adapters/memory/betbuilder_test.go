package memory_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/memory/seed"
)

// TestBetBuilderDisabledSeed_CoversRealEvents is the Bet Builder overlay
// regression guard, mirroring TestSuperCuotaSeed_CoversRealSelections:
// every event ID in the curated overlay must resolve to a real seeded
// event, catching drift if data.json's event IDs ever change (spec:
// bet-slip-calculation/Same-Event Combo Rejection).
func TestBetBuilderDisabledSeed_CoversRealEvents(t *testing.T) {
	repo := newRepo(t)

	require.NotEmpty(t, seed.BetBuilderDisabled, "the curated Bet Builder overlay must not be empty")
	for eventID := range seed.BetBuilderDisabled {
		_, err := repo.Detail(context.Background(), eventID)
		require.NoErrorf(t, err, "curated Bet Builder overlay event %s does not resolve to a real seeded event", eventID)
	}
}

// TestBetBuilderDisabledSeed_ActuallyDisablesTheEvent proves the overlay is
// not a dead entry: every curated event ID must load with
// IsBetBuilderEnabled false, and every SelectionRef the catalog resolves
// for it must carry EventBetBuilderEnabled false too — the exact
// propagation BetSlip.Quote's Bet Builder gate depends on (design: Bet
// Builder rule).
func TestBetBuilderDisabledSeed_ActuallyDisablesTheEvent(t *testing.T) {
	repo := newRepo(t)

	for eventID := range seed.BetBuilderDisabled {
		e, err := repo.Detail(context.Background(), eventID)
		require.NoError(t, err)
		require.Falsef(t, e.IsBetBuilderEnabled, "overlay event %s must load with Bet Builder disabled", eventID)

		require.NotEmpty(t, e.Markets, "overlay event %s must have at least one market to resolve a selection from", eventID)
		require.NotEmpty(t, e.Markets[0].Selections, "overlay event %s market %s must have at least one selection", eventID, e.Markets[0].ID)
		selectionID := e.Markets[0].Selections[0].ID

		refs, err := repo.SelectionsByIDs(context.Background(), []string{selectionID})
		require.NoError(t, err)
		require.Len(t, refs, 1)
		require.Falsef(t, refs[0].EventBetBuilderEnabled,
			"overlay event %s: resolved selection %s must carry EventBetBuilderEnabled false", eventID, selectionID)
	}
}

// TestBetBuilderDisabledSeed_NeverCollidesWithSuperCuota proves the two
// authored demo overlays target disjoint events: neither can silently
// override the other's curated fixture data (task requirement: pick events
// not among the five already curated for Super Cuota).
func TestBetBuilderDisabledSeed_NeverCollidesWithSuperCuota(t *testing.T) {
	repo := newRepo(t)

	for selectionID := range seed.SuperCuotaOdds {
		refs, err := repo.SelectionsByIDs(context.Background(), []string{selectionID})
		require.NoError(t, err)
		require.Len(t, refs, 1)
		require.Falsef(t, seed.BetBuilderDisabled[refs[0].EventID],
			"Super Cuota selection %s belongs to a Bet Builder-disabled overlay event %s; the two demos must never collide",
			selectionID, refs[0].EventID)
	}
}
