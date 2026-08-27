package event_test

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/event"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

// TestNewGroup_ValidatesLetterRange proves Group only accepts the 12 World
// Cup group letters (A-L) or an empty string, and rejects everything else,
// per specs/events-catalog "Phase and Group Enrichment".
func TestNewGroup_ValidatesLetterRange(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "first valid letter", input: "A", wantErr: false},
		{name: "last valid letter", input: "L", wantErr: false},
		{name: "middle valid letter", input: "F", wantErr: false},
		{name: "empty string is unknown, not an error", input: "", wantErr: false},
		{name: "lowercase letter rejected", input: "a", wantErr: true},
		{name: "letter beyond L rejected", input: "M", wantErr: true},
		{name: "multi-character string rejected", input: "AB", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := event.NewGroup(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, event.ErrInvalidGroup)
				return
			}
			require.NoError(t, err)
			require.Equal(t, event.Group(tt.input), got)
		})
	}
}

// TestNewGroup_EmptyNeverPanics proves the "unknown group" fallback path is
// always safe to call, matching the design's "never a panic" requirement.
func TestNewGroup_EmptyNeverPanics(t *testing.T) {
	require.NotPanics(t, func() {
		g, err := event.NewGroup("")
		require.NoError(t, err)
		require.True(t, g.IsEmpty())
	})
}

// TestPhaseGroupStage_IsTheOnlySeedPhase pins the Phase enum value used by
// the 2026 sample dataset; knockout phases extend this constant list
// without touching callers.
func TestPhaseGroupStage_IsTheOnlySeedPhase(t *testing.T) {
	require.Equal(t, event.Phase("group_stage"), event.PhaseGroupStage)
}

// TestNewSelectionRef_PropagatesBetBuilderEligibility proves
// SelectionRef.EventBetBuilderEnabled is propagated from the owning
// Event's own flag, not left at its zero value (spec: events-catalog/
// SelectionRef Carries Bet Builder Eligibility, both scenarios). All 24
// real seeded events carry the flag true; that scenario is proven here
// against a directly-constructed fixture shaped like one, since domain
// tests never load the adapter-owned seed data.
func TestNewSelectionRef_PropagatesBetBuilderEligibility(t *testing.T) {
	tests := []struct {
		name   string
		owner  event.Event
		wantBB bool
	}{
		{
			name:   "synthetic disabled event",
			owner:  event.Event{ID: "evt-1", IsBetBuilderEnabled: false},
			wantBB: false,
		},
		{
			name:   "seeded-style enabled event (all 24 real events carry this flag)",
			owner:  event.Event{ID: "evt-2", IsBetBuilderEnabled: true},
			wantBB: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sel := event.Selection{ID: "sel-1", EventID: tt.owner.ID, Odds: mustTestOdds(t, "1.85")}

			ref := event.NewSelectionRef(sel, tt.owner)

			require.Equal(t, sel.ID, ref.ID)
			require.Equal(t, sel.EventID, ref.EventID)
			require.Equal(t, tt.wantBB, ref.EventBetBuilderEnabled)
		})
	}
}

// TestNewSelectionRef_OriginalOddsNilByDefault proves a selection with no
// Super Cuota boost propagates a nil OriginalOdds into its SelectionRef
// (design: Domain Model Additions, Selection.OriginalOdds / SelectionRef.
// OriginalOdds).
func TestNewSelectionRef_OriginalOddsNilByDefault(t *testing.T) {
	sel := event.Selection{ID: "sel-1", EventID: "evt-1", Odds: mustTestOdds(t, "1.85")}

	ref := event.NewSelectionRef(sel, event.Event{ID: "evt-1"})

	require.Nil(t, ref.OriginalOdds)
}

// TestNewSelectionRef_PropagatesOriginalOddsWhenBoosted is the
// triangulation case: a boosted Selection's OriginalOdds pointer survives
// into its SelectionRef unchanged.
func TestNewSelectionRef_PropagatesOriginalOddsWhenBoosted(t *testing.T) {
	original := mustTestOdds(t, "1.47")
	sel := event.Selection{ID: "sel-1", EventID: "evt-1", Odds: mustTestOdds(t, "1.60"), OriginalOdds: &original}

	ref := event.NewSelectionRef(sel, event.Event{ID: "evt-1"})

	require.NotNil(t, ref.OriginalOdds)
	require.Equal(t, "1.47", ref.OriginalOdds.String())
}

// mustTestOdds is a test-only convenience wrapper around money.NewOdds.
func mustTestOdds(t *testing.T, value string) money.Odds {
	t.Helper()
	o, err := money.NewOdds(decimal.RequireFromString(value))
	require.NoError(t, err)
	return o
}
