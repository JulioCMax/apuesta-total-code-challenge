package event_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/event"
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
