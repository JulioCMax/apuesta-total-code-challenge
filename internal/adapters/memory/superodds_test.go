package memory_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/memory/seed"
)

// TestSuperCuotaSeed_CoversRealSelections is the Super Cuota seed
// regression guard, mirroring TestGroupSeed_CoversEveryEvent: every
// curated selection ID must resolve to a real seeded selection, catching
// drift if data.json's selection IDs ever change (spec: events-catalog/
// Selection Exposes Original Odds When Boosted; design: Super Cuota).
func TestSuperCuotaSeed_CoversRealSelections(t *testing.T) {
	repo := newRepo(t)

	require.NotEmpty(t, seed.SuperCuotaOdds, "the curated Super Cuota set must not be empty")
	for selectionID := range seed.SuperCuotaOdds {
		refs, err := repo.SelectionsByIDs(context.Background(), []string{selectionID})
		require.NoErrorf(t, err, "curated Super Cuota selection %s does not resolve to a real seeded selection", selectionID)
		require.Len(t, refs, 1)
	}
}

// TestSuperCuotaSeed_EveryBoostExceedsOriginal proves every curated boost
// is strictly greater than the selection's real, unboosted TrueOdds — a
// "boost" that lowers or matches the original odds is a bug, not a
// promotion (design: Super Cuota). This also proves OriginalOdds is
// actually populated for every curated selection once the boost is
// applied (spec: events-catalog/Selection Exposes Original Odds When
// Boosted, "Curated Super Cuota selection loaded").
func TestSuperCuotaSeed_EveryBoostExceedsOriginal(t *testing.T) {
	repo := newRepo(t)

	for selectionID := range seed.SuperCuotaOdds {
		refs, err := repo.SelectionsByIDs(context.Background(), []string{selectionID})
		require.NoError(t, err)
		require.Len(t, refs, 1)

		require.NotNilf(t, refs[0].OriginalOdds, "curated selection %s must expose OriginalOdds once boosted", selectionID)
		require.Truef(t, refs[0].Odds.Decimal().GreaterThan(refs[0].OriginalOdds.Decimal()),
			"selection %s: boosted odds %s must exceed original %s", selectionID, refs[0].Odds.String(), refs[0].OriginalOdds.String())
	}
}

// TestSuperCuotaSeed_NonCuratedSelectionHasNoOriginalOdds proves a
// selection absent from the curated Super Cuota set carries a nil
// OriginalOdds — the boosted Odds value is never a second, divergent value
// (spec: events-catalog/Selection Exposes Original Odds When Boosted,
// "Non-curated selection loaded").
func TestSuperCuotaSeed_NonCuratedSelectionHasNoOriginalOdds(t *testing.T) {
	repo := newRepo(t)

	const nonCuratedSelectionID = "0ML784926076341366984D" // Empate, México vs Sudáfrica — not curated
	_, curated := seed.SuperCuotaOdds[nonCuratedSelectionID]
	require.False(t, curated, "test fixture assumption: this ID must not be curated")

	refs, err := repo.SelectionsByIDs(context.Background(), []string{nonCuratedSelectionID})
	require.NoError(t, err)
	require.Len(t, refs, 1)
	require.Nil(t, refs[0].OriginalOdds)
}
