package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/memory"
	domainbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/betslip"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/event"
)

// newRepo builds an EventRepository from the real embedded seed data
// (there is exactly one seed dataset; no test-only fixture exists), failing
// the test immediately on any load error.
func newRepo(t *testing.T) *memory.EventRepository {
	t.Helper()
	repo, err := memory.NewEventRepository()
	require.NoError(t, err)
	return repo
}

// TestList_DateRangeInclusiveBounds proves the date-range filter includes
// events whose StartsAt falls exactly on either boundary (spec:
// events-catalog/List Events by Date Range).
func TestList_DateRangeInclusiveBounds(t *testing.T) {
	repo := newRepo(t)

	from := time.Date(2026, 6, 13, 1, 0, 0, 0, time.UTC) // EE.UU. vs Paraguay
	to := time.Date(2026, 6, 13, 22, 0, 0, 0, time.UTC)  // Brasil vs Marruecos

	events, err := repo.List(context.Background(), from, to)

	require.NoError(t, err)
	require.Len(t, events, 3) // the two boundary events plus Catar vs Suiza (19:00) in between
	require.True(t, events[0].StartsAt.Equal(from), "lower boundary must be inclusive")
	require.True(t, events[len(events)-1].StartsAt.Equal(to), "upper boundary must be inclusive")
}

// TestList_EmptyRangeReturnsEmptyList proves a date range with no matching
// events returns an empty list and no error (spec: events-catalog/List
// Events by Date Range, "Empty range returns empty list, not an error").
func TestList_EmptyRangeReturnsEmptyList(t *testing.T) {
	repo := newRepo(t)

	from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)

	events, err := repo.List(context.Background(), from, to)

	require.NoError(t, err)
	require.Empty(t, events)
}

// TestList_RejectsInvertedRange proves "from" after "to" is a typed domain
// error, not a silently-empty result.
func TestList_RejectsInvertedRange(t *testing.T) {
	repo := newRepo(t)

	from := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)

	_, err := repo.List(context.Background(), from, to)

	require.ErrorIs(t, err, event.ErrInvalidDateRange)
}

// TestDetail_MarketsInDefaultOrder proves the load-time filter/sort (D5)
// always returns the four default markets in the fixed order 1X2, Total
// Goals, Both Teams to Score, First Goal, regardless of seed storage order
// (spec: events-catalog/Event Detail With Ordered Default Markets).
func TestDetail_MarketsInDefaultOrder(t *testing.T) {
	repo := newRepo(t)

	e, err := repo.Detail(context.Background(), "784926067864698880") // México vs Sudáfrica

	require.NoError(t, err)
	require.Len(t, e.Markets, 4)
	require.Equal(t, event.MarketTypeMoneyline, e.Markets[0].TypeID)
	require.Equal(t, event.MarketTypeTotalGoals, e.Markets[1].TypeID)
	require.Equal(t, event.MarketTypeBothTeamsScore, e.Markets[2].TypeID)
	require.Equal(t, event.MarketTypeFirstGoal, e.Markets[3].TypeID)
}

// TestDetail_UnknownEventIDReturnsTypedError proves an unknown event ID
// yields the typed ErrEventNotFound sentinel, not a generic error.
func TestDetail_UnknownEventIDReturnsTypedError(t *testing.T) {
	repo := newRepo(t)

	_, err := repo.Detail(context.Background(), "does-not-exist")

	require.ErrorIs(t, err, event.ErrEventNotFound)
}

// TestGroupSeed_CoversEveryEvent is the seed regression guard: every one of
// the 24 seeded events must resolve to a non-empty A-L group and the group
// stage phase (spec: events-catalog/Phase and Group Enrichment; Engram
// #1627 verified team->group map).
func TestGroupSeed_CoversEveryEvent(t *testing.T) {
	repo := newRepo(t)

	events, err := repo.List(context.Background(), time.Time{}, time.Time{})

	require.NoError(t, err)
	require.Len(t, events, 24)
	for _, e := range events {
		require.Falsef(t, e.Group.IsEmpty(), "event %s (%s) resolved to an empty group", e.ID, e.Name)
		require.Equal(t, event.PhaseGroupStage, e.Phase)
	}
}

// TestSelectionsByIDs_ResolvesRealSelectionRef proves SelectionsByIDs
// implements application/betslip.EventCatalog against the real embedded
// dataset: a known selection ID resolves to its ID/EventID/Odds/
// IsDisabled (spec: bet-slip-calculation/Selection Resolution). This is
// the port the Phase 13 composition root wires into betslip.Calculate/
// Place instead of the fakes application-layer tests use — deferred since
// Unit 5 (no RED test drove it until this real wiring needed it, per
// strict TDD's Three Laws).
func TestSelectionsByIDs_ResolvesRealSelectionRef(t *testing.T) {
	repo := newRepo(t)

	// México vs Sudáfrica's 1X2 market, "México" selection.
	const selectionID = "0ML784926076341366984H"

	refs, err := repo.SelectionsByIDs(context.Background(), []string{selectionID})

	require.NoError(t, err)
	require.Len(t, refs, 1)
	require.Equal(t, selectionID, refs[0].ID)
	require.Equal(t, "784926067864698880", refs[0].EventID)
	require.Equal(t, "1.47", refs[0].Odds.String())
	require.False(t, refs[0].IsDisabled)
}

// TestSelectionsByIDs_UnknownIDReturnsTypedError proves an unresolved
// selection ID surfaces as the typed domain error the application layer
// expects, never a generic error (spec: bet-slip-calculation/Selection
// Resolution, "Unknown selection ID").
func TestSelectionsByIDs_UnknownIDReturnsTypedError(t *testing.T) {
	repo := newRepo(t)

	_, err := repo.SelectionsByIDs(context.Background(), []string{"does-not-exist"})

	var notFound domainbetslip.ErrSelectionNotFound
	require.ErrorAs(t, err, &notFound)
	require.Equal(t, "does-not-exist", notFound.SelectionID)
}

// TestGroupSeed_UnknownTeamFallsBackToEmptyGroup proves the fallback path
// required by the design: a team pair absent from (or disagreeing in) the
// seed map never panics and never fails the request, it just yields an
// empty group.
func TestGroupSeed_UnknownTeamFallsBackToEmptyGroup(t *testing.T) {
	tests := []struct {
		name string
		home string
		away string
	}{
		{name: "both teams unseeded", home: "Nowhere", away: "Neverland"},
		{name: "one team unseeded", home: "México", away: "Neverland"},
		{name: "teams disagree on group", home: "México", away: "Brasil"}, // A vs C
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotPanics(t, func() {
				group, resolved := memory.ResolveGroup(tt.home, tt.away)
				require.False(t, resolved)
				require.True(t, group.IsEmpty())
			})
		})
	}
}

// TestDetail_MoneylineSelectionsInHomeDrawAwayOrder proves the outcomes of a
// three-way market are served local-draw-visitor, the order every betting
// UI (and the reference design) presents 1X2 in.
//
// The source dataset lists them draw-first, so this is a real reordering:
// the ordering signal is the raw Side field (1 home, 2 draw, 3 away), not
// the position the file happens to use.
func TestDetail_MoneylineSelectionsInHomeDrawAwayOrder(t *testing.T) {
	repo := newRepo(t)

	e, err := repo.Detail(context.Background(), "784926067864698880") // México vs Sudáfrica
	require.NoError(t, err)

	for _, typeID := range []event.MarketTypeID{event.MarketTypeMoneyline, event.MarketTypeFirstGoal} {
		market := marketOfType(t, e, typeID)
		require.Len(t, market.Selections, 3, "market %s", typeID)
		require.Equal(t, e.Home.Name, market.Selections[0].Name, "market %s: home goes first", typeID)
		require.Equal(t, e.Away.Name, market.Selections[2].Name, "market %s: away goes last", typeID)
	}
}

// TestDetail_EveryMoneylineMarketOrdersHomeFirst is the catalog-wide guard
// for the rule above: no seeded three-way market may present the away side
// before the home side.
func TestDetail_EveryMoneylineMarketOrdersHomeFirst(t *testing.T) {
	repo := newRepo(t)

	events, err := repo.List(context.Background(), time.Time{}, time.Time{})
	require.NoError(t, err)
	require.NotEmpty(t, events)

	checked := 0
	for _, summary := range events {
		e, err := repo.Detail(context.Background(), summary.ID)
		require.NoError(t, err)

		for _, market := range e.Markets {
			if market.TypeID != event.MarketTypeMoneyline && market.TypeID != event.MarketTypeFirstGoal {
				continue
			}
			require.Len(t, market.Selections, 3)
			require.Equal(t, e.Home.Name, market.Selections[0].Name,
				"event %s market %s", e.ID, market.TypeID)
			require.Equal(t, e.Away.Name, market.Selections[2].Name,
				"event %s market %s", e.ID, market.TypeID)
			checked++
		}
	}
	require.Positive(t, checked, "the guard must actually inspect three-way markets")
}

// TestDetail_OverUnderKeepsSourcePairing is the counterpart guard. Total
// Goals repeats the same Side across its lines (every "Más de" is side 1,
// every "Menos de" is side 3), so Side is not an ordering there: sorting by
// it would group all the overs together and break the line pairing the
// source order carries. Consecutive selections must stay paired on one
// line, with lines ascending.
func TestDetail_OverUnderKeepsSourcePairing(t *testing.T) {
	repo := newRepo(t)

	e, err := repo.Detail(context.Background(), "784926067864698880")
	require.NoError(t, err)

	market := marketOfType(t, e, event.MarketTypeTotalGoals)
	require.GreaterOrEqual(t, len(market.Selections), 4)
	require.Zero(t, len(market.Selections)%2, "over/under selections come in pairs")

	previousLine := decimal.NewFromInt(-1)
	for i := 0; i < len(market.Selections); i += 2 {
		over, under := market.Selections[i], market.Selections[i+1]
		require.NotNil(t, over.Line)
		require.NotNil(t, under.Line)
		require.True(t, over.Line.Equal(*under.Line),
			"positions %d and %d must share a line", i, i+1)
		require.True(t, over.Line.GreaterThan(previousLine), "lines must ascend")
		previousLine = *over.Line
	}
}

// marketOfType returns the event's market of the given type, failing the
// test when it is absent.
func marketOfType(t *testing.T, e event.Event, typeID event.MarketTypeID) event.Market {
	t.Helper()
	for _, market := range e.Markets {
		if market.TypeID == typeID {
			return market
		}
	}
	t.Fatalf("event %s has no market of type %s", e.ID, typeID)
	return event.Market{}
}
