package betslip_test

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	appbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/application/betslip"
	domainbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/betslip"
	domainevent "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/event"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

// defaultBoundsFixture is a canary configuration deliberately different
// from any documented default, so a test asserting equality against it
// proves Calculate reads bounds from its injected config rather than
// hardcoding the spec's example literals (spec: bet-slip-calculation/
// Calculate Endpoint Response Shape).
func defaultBoundsFixture(t *testing.T) appbetslip.StakeBounds {
	t.Helper()
	return appbetslip.StakeBounds{
		MinStake:      mustMoney(t, "5.00"),
		MaxStake:      mustMoney(t, "500.00"),
		Currency:      "USD",
		MaxSelections: 20,
	}
}

func mustMoney(t *testing.T, amount string) money.Money {
	t.Helper()
	m, err := money.NewMoney(decimal.RequireFromString(amount))
	require.NoError(t, err)
	return m
}

func mustOdds(t *testing.T, value string) money.Odds {
	t.Helper()
	o, err := money.NewOdds(decimal.RequireFromString(value))
	require.NoError(t, err)
	return o
}

// TestCalculate_ResolvesSelectionsAndReturnsQuote proves Calculate resolves
// each requested selection ID through EventCatalog and returns its id,
// eventId, odds alongside the priced Quote (spec: bet-slip-calculation/
// Selection Resolution, /Single and Combo Bet Generation).
func TestCalculate_ResolvesSelectionsAndReturnsQuote(t *testing.T) {
	sel1 := domainevent.SelectionRef{ID: "sel-1", EventID: "evt-1", Odds: mustOdds(t, "1.85")}
	sel2 := domainevent.SelectionRef{ID: "sel-2", EventID: "evt-2", Odds: mustOdds(t, "2.10")}
	catalog := newFakeCatalog(sel1, sel2)
	uc := appbetslip.NewCalculate(catalog, defaultBoundsFixture(t))

	result, err := uc.Execute(context.Background(), appbetslip.CalculateCommand{
		SelectionIDs: []string{"sel-1", "sel-2"},
		Stake:        mustMoney(t, "100.00"),
	})

	require.NoError(t, err)
	require.Equal(t, []string{"sel-1", "sel-2"}, catalog.lastIDs)
	require.Len(t, result.Selections, 2)
	require.Equal(t, "sel-1", result.Selections[0].ID)
	require.Equal(t, "evt-1", result.Selections[0].EventID)
	require.Equal(t, "1.85", result.Selections[0].Odds.String())
	require.Len(t, result.Quote.Singles, 2)
	require.NotNil(t, result.Quote.Combo)
}

// TestCalculate_UnknownSelectionReturnsTypedError proves an unresolvable
// selection ID surfaces the typed domain error unchanged, not a generic
// validation failure (spec: bet-slip-calculation/Selection Resolution).
func TestCalculate_UnknownSelectionReturnsTypedError(t *testing.T) {
	catalog := newFakeCatalog() // resolves nothing
	uc := appbetslip.NewCalculate(catalog, defaultBoundsFixture(t))

	_, err := uc.Execute(context.Background(), appbetslip.CalculateCommand{
		SelectionIDs: []string{"missing-sel"},
		Stake:        mustMoney(t, "100.00"),
	})

	var notFound domainbetslip.ErrSelectionNotFound
	require.ErrorAs(t, err, &notFound)
	require.Equal(t, "missing-sel", notFound.SelectionID)
}

// TestCalculate_ResponseCarriesConfiguredBounds proves minStake/maxStake/
// currency in the result come from the injected configuration, never a
// hardcoded literal (spec: bet-slip-calculation/Calculate Endpoint Response
// Shape).
func TestCalculate_ResponseCarriesConfiguredBounds(t *testing.T) {
	sel := domainevent.SelectionRef{ID: "sel-1", EventID: "evt-1", Odds: mustOdds(t, "1.85")}
	catalog := newFakeCatalog(sel)
	bounds := defaultBoundsFixture(t)
	uc := appbetslip.NewCalculate(catalog, bounds)

	result, err := uc.Execute(context.Background(), appbetslip.CalculateCommand{
		SelectionIDs: []string{"sel-1"},
		Stake:        mustMoney(t, "10.00"),
	})

	require.NoError(t, err)
	require.Equal(t, bounds.MinStake, result.MinStake)
	require.Equal(t, bounds.MaxStake, result.MaxStake)
	require.Equal(t, bounds.Currency, result.Currency)
}

// TestCalculate_ThreadsIsBetBuilderIntoQuote proves CalculateCommand.
// IsBetBuilder threads unchanged into BetSlip.AllowSameEventCombo,
// allowing a same-event combo through when both the opt-in and the
// event's own flag are true (spec: bet-slip-calculation/Bet Builder Flag
// Threading (Calculate); design: Bet Builder rule).
func TestCalculate_ThreadsIsBetBuilderIntoQuote(t *testing.T) {
	sel1 := domainevent.SelectionRef{ID: "sel-1", EventID: "evt-1", Odds: mustOdds(t, "1.85"), EventBetBuilderEnabled: true}
	sel2 := domainevent.SelectionRef{ID: "sel-2", EventID: "evt-1", Odds: mustOdds(t, "2.10"), EventBetBuilderEnabled: true}
	catalog := newFakeCatalog(sel1, sel2)
	uc := appbetslip.NewCalculate(catalog, defaultBoundsFixture(t))

	result, err := uc.Execute(context.Background(), appbetslip.CalculateCommand{
		SelectionIDs: []string{"sel-1", "sel-2"},
		Stake:        mustMoney(t, "100.00"),
		IsBetBuilder: true,
	})

	require.NoError(t, err)
	require.NotNil(t, result.Quote.Combo)
}

// TestCalculate_IsBetBuilderOmittedStillRejectsSameEventCombo proves the
// default false value preserves the existing same-event rejection (spec:
// bet-slip-calculation/Bet Builder Flag Threading (Calculate), "Field
// omitted").
func TestCalculate_IsBetBuilderOmittedStillRejectsSameEventCombo(t *testing.T) {
	sel1 := domainevent.SelectionRef{ID: "sel-1", EventID: "evt-1", Odds: mustOdds(t, "1.85"), EventBetBuilderEnabled: true}
	sel2 := domainevent.SelectionRef{ID: "sel-2", EventID: "evt-1", Odds: mustOdds(t, "2.10"), EventBetBuilderEnabled: true}
	catalog := newFakeCatalog(sel1, sel2)
	uc := appbetslip.NewCalculate(catalog, defaultBoundsFixture(t))

	_, err := uc.Execute(context.Background(), appbetslip.CalculateCommand{
		SelectionIDs: []string{"sel-1", "sel-2"},
		Stake:        mustMoney(t, "100.00"),
	})

	var sameEvent domainbetslip.ErrSameEventCombo
	require.ErrorAs(t, err, &sameEvent)
}

// TestCalculate_BoostedSelectionExposesOriginalOdds proves a resolved
// selection carrying a Super Cuota boost echoes both Odds and OriginalOdds
// (spec: bet-slip-calculation/Boosted Selection Exposes Original Odds).
func TestCalculate_BoostedSelectionExposesOriginalOdds(t *testing.T) {
	original := mustOdds(t, "1.47")
	sel := domainevent.SelectionRef{ID: "sel-1", EventID: "evt-1", Odds: mustOdds(t, "1.60"), OriginalOdds: &original}
	catalog := newFakeCatalog(sel)
	uc := appbetslip.NewCalculate(catalog, defaultBoundsFixture(t))

	result, err := uc.Execute(context.Background(), appbetslip.CalculateCommand{
		SelectionIDs: []string{"sel-1"},
		Stake:        mustMoney(t, "100.00"),
	})

	require.NoError(t, err)
	require.Len(t, result.Selections, 1)
	require.Equal(t, "1.60", result.Selections[0].Odds.String())
	require.NotNil(t, result.Selections[0].OriginalOdds)
	require.Equal(t, "1.47", result.Selections[0].OriginalOdds.String())
}

// TestCalculate_NonBoostedSelectionHasNoOriginalOdds proves a resolved
// selection with no boost carries a nil OriginalOdds (spec: bet-slip-
// calculation/Boosted Selection Exposes Original Odds, "Non-boosted
// selection resolved").
func TestCalculate_NonBoostedSelectionHasNoOriginalOdds(t *testing.T) {
	sel := domainevent.SelectionRef{ID: "sel-1", EventID: "evt-1", Odds: mustOdds(t, "1.85")}
	catalog := newFakeCatalog(sel)
	uc := appbetslip.NewCalculate(catalog, defaultBoundsFixture(t))

	result, err := uc.Execute(context.Background(), appbetslip.CalculateCommand{
		SelectionIDs: []string{"sel-1"},
		Stake:        mustMoney(t, "100.00"),
	})

	require.NoError(t, err)
	require.Len(t, result.Selections, 1)
	require.Nil(t, result.Selections[0].OriginalOdds)
}

// TestCalculate_RoundsPotentialReturnsAtHalfCentBoundary proves the
// combined odds and potentialReturns computed through the full Calculate
// flow match BetSlip.Quote's half-up rounding exactly, with no extra
// rounding drift introduced at the application layer (spec: bet-slip-
// calculation/Potential Returns Rounding).
func TestCalculate_RoundsPotentialReturnsAtHalfCentBoundary(t *testing.T) {
	sel1 := domainevent.SelectionRef{ID: "sel-1", EventID: "evt-1", Odds: mustOdds(t, "1.85")}
	sel2 := domainevent.SelectionRef{ID: "sel-2", EventID: "evt-2", Odds: mustOdds(t, "2.10")}
	catalog := newFakeCatalog(sel1, sel2)
	uc := appbetslip.NewCalculate(catalog, defaultBoundsFixture(t))

	result, err := uc.Execute(context.Background(), appbetslip.CalculateCommand{
		SelectionIDs: []string{"sel-1", "sel-2"},
		Stake:        mustMoney(t, "100.00"),
	})

	require.NoError(t, err)
	require.NotNil(t, result.Quote.Combo)
	// 1.85 * 2.10 = 3.885 -> Round2 (half-up) -> 3.89; 100.00 * 3.89 = 389.00
	require.Equal(t, "3.89", result.Quote.Combo.Odds.String())
	require.Equal(t, "389.00", result.Quote.Combo.PotentialReturns.String())
}
