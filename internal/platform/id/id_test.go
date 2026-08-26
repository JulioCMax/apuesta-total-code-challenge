package id_test

import (
	"testing"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/platform/id"
)

// TestULIDGenerator_ReturnsParsableULIDStrings proves NewID produces a
// well-formed ULID (D9), not an arbitrary string.
func TestULIDGenerator_ReturnsParsableULIDStrings(t *testing.T) {
	gen := id.NewULIDGenerator()

	got := gen.NewID()

	_, err := ulid.Parse(got)
	require.NoError(t, err, "NewID() must return a valid ULID string, got %q", got)
}

// TestULIDGenerator_NeverRepeatsAcrossManyCalls proves consecutive IDs are
// unique even generated back-to-back in the same process.
func TestULIDGenerator_NeverRepeatsAcrossManyCalls(t *testing.T) {
	gen := id.NewULIDGenerator()

	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		got := gen.NewID()
		require.False(t, seen[got], "duplicate ULID generated: %s", got)
		seen[got] = true
	}
}

// TestULIDGenerator_IsMonotonicallyIncreasing proves consecutively-minted
// IDs sort in generation order even within the same millisecond, so the
// DynamoDB bet SK (BET#<ulid>) orders history chronologically for free
// (D9) without an extra index.
func TestULIDGenerator_IsMonotonicallyIncreasing(t *testing.T) {
	gen := id.NewULIDGenerator()

	prev := gen.NewID()
	for i := 0; i < 200; i++ {
		got := gen.NewID()
		require.True(t, got > prev, "ULID %q must sort strictly after previous %q", got, prev)
		prev = got
	}
}
