package dto_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/http/dto"
	appbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/application/betslip"
	domainbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/betslip"
	domainevent "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/event"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

func mustMoney(t *testing.T, v float64) money.Money {
	t.Helper()
	m, err := money.NewMoneyFromFloat(v)
	require.NoError(t, err)
	return m
}

func mustOdds(t *testing.T, v float64) money.Odds {
	t.Helper()
	o, err := money.NewOddsFromFloat(v)
	require.NoError(t, err)
	return o
}

// TestBetSlipRequest_StakeMoney_ParsesExactDecimal proves the request DTO
// parses "stake" from the raw JSON number text (via encoding/json.Number),
// never through a float64 intermediate, so a value like 33.10 round-trips
// exactly instead of drifting to 33.099999999999994 (spec: bet-slip-
// calculation/Potential Returns Rounding — precision must hold from the
// request boundary onward).
func TestBetSlipRequest_StakeMoney_ParsesExactDecimal(t *testing.T) {
	var req dto.BetSlipRequest
	require.NoError(t, json.Unmarshal([]byte(`{"selectionIds":["s1"],"stake":33.10}`), &req))

	got, err := req.StakeMoney()

	require.NoError(t, err)
	require.Equal(t, "33.10", got.String())
}

// TestBetSlipRequest_StakeMoney_RejectsNegative proves a negative stake is a
// request-level validation error, not a panic or a silently-clamped value.
func TestBetSlipRequest_StakeMoney_RejectsNegative(t *testing.T) {
	var req dto.BetSlipRequest
	require.NoError(t, json.Unmarshal([]byte(`{"selectionIds":["s1"],"stake":-5}`), &req))

	_, err := req.StakeMoney()

	require.Error(t, err)
}

// TestEventSummaryFromDomain_OmitsEmptyGroup proves an unresolved group
// (event.Group("")) is omitted from the JSON output rather than sent as ""
// (design.md's in-memory event repository section: "Empty group is omitted
// from the JSON response (omitempty) rather than sent as \"\"").
func TestEventSummaryFromDomain_OmitsEmptyGroup(t *testing.T) {
	e := domainevent.Event{
		ID:       "evt-1",
		Name:     "A vs B",
		StartsAt: time.Date(2026, 6, 11, 19, 0, 0, 0, time.UTC),
		Phase:    domainevent.PhaseGroupStage,
		Group:    domainevent.Group(""),
		Home:     domainevent.Participant{Name: "A"},
		Away:     domainevent.Participant{Name: "B"},
	}

	raw, err := json.Marshal(dto.EventSummaryFromDomain(e))
	require.NoError(t, err)
	require.NotContains(t, string(raw), `"group"`)
}

// TestEventSummaryFromDomain_IncludesResolvedGroup is the companion
// (triangulation) case: a resolved group MUST appear in the output.
func TestEventSummaryFromDomain_IncludesResolvedGroup(t *testing.T) {
	group, err := domainevent.NewGroup("A")
	require.NoError(t, err)
	e := domainevent.Event{ID: "evt-2", Group: group}

	raw, err := json.Marshal(dto.EventSummaryFromDomain(e))
	require.NoError(t, err)
	require.Contains(t, string(raw), `"group":"A"`)
}

// TestEventDetailFromDomain_ExposesMarketTypeIdAndEventLevelSettings proves
// each market carries MarketType.ID and the event itself carries the UI
// metadata flags (spec: events-catalog/Market and Event Metadata Exposure).
func TestEventDetailFromDomain_ExposesMarketTypeIdAndEventLevelSettings(t *testing.T) {
	e := domainevent.Event{
		ID:                  "evt-3",
		HasStatistics:       true,
		IsBetBuilderEnabled: true,
		Markets: []domainevent.Market{
			{ID: "m1", TypeID: domainevent.MarketTypeMoneyline, Name: "1X2"},
		},
	}

	detail := dto.EventDetailFromDomain(e)

	require.True(t, detail.Settings.HasStatistics)
	require.True(t, detail.Settings.IsBetBuilderEnabled)
	require.Len(t, detail.Markets, 1)
	require.Equal(t, string(domainevent.MarketTypeMoneyline), detail.Markets[0].MarketType.ID)
}

// TestCalculateResponseFromDomain_SingleHasNoCombo proves a one-selection
// Quote maps to exactly one Single and a nil Combo (spec: bet-slip-
// calculation/Single and Combo Bet Generation).
func TestCalculateResponseFromDomain_SingleHasNoCombo(t *testing.T) {
	result := appbetslip.CalculateResult{
		MinStake: mustMoney(t, 1), MaxStake: mustMoney(t, 10000), Currency: "PEN",
		Stake:      mustMoney(t, 100),
		Selections: []appbetslip.ResolvedSelection{{ID: "s1", EventID: "e1", Odds: mustOdds(t, 1.85)}},
		Quote: domainbetslip.Quote{
			Singles: []domainbetslip.Leg{{SelectionIDs: []string{"s1"}, Odds: mustOdds(t, 1.85), PotentialReturns: mustMoney(t, 185)}},
		},
	}

	resp := dto.CalculateResponseFromDomain(result)

	require.Equal(t, "PEN", resp.Currency)
	require.Len(t, resp.Singles, 1)
	require.Equal(t, "s1", resp.Singles[0].SelectionID)
	require.Nil(t, resp.Combo)
}

// TestCalculateResponseFromDomain_TwoSelectionsProduceCombo is the
// triangulation case: 2 distinct-event selections produce 2 Singles AND a
// non-nil Combo (spec: bet-slip-calculation/Single and Combo Bet
// Generation, "Two selections from distinct events").
func TestCalculateResponseFromDomain_TwoSelectionsProduceCombo(t *testing.T) {
	result := appbetslip.CalculateResult{
		MinStake: mustMoney(t, 1), MaxStake: mustMoney(t, 10000), Currency: "PEN",
		Stake: mustMoney(t, 100),
		Selections: []appbetslip.ResolvedSelection{
			{ID: "s1", EventID: "e1", Odds: mustOdds(t, 1.85)},
			{ID: "s2", EventID: "e2", Odds: mustOdds(t, 2.10)},
		},
		Quote: domainbetslip.Quote{
			Singles: []domainbetslip.Leg{
				{SelectionIDs: []string{"s1"}, Odds: mustOdds(t, 1.85), PotentialReturns: mustMoney(t, 185)},
				{SelectionIDs: []string{"s2"}, Odds: mustOdds(t, 2.10), PotentialReturns: mustMoney(t, 210)},
			},
			Combo: &domainbetslip.Leg{SelectionIDs: []string{"s1", "s2"}, Odds: mustOdds(t, 3.89), PotentialReturns: mustMoney(t, 389)},
		},
	}

	resp := dto.CalculateResponseFromDomain(result)

	require.Len(t, resp.Singles, 2)
	require.NotNil(t, resp.Combo)
	require.Equal(t, []string{"s1", "s2"}, resp.Combo.SelectionIDs)
	require.Equal(t, "389.00", resp.Combo.PotentialReturns.String())
}

// TestPlaceResponseFromDomain_MapsBetAndBalance proves the place response
// carries every field design.md's shape requires, including balanceAfter
// read from a separate balance query (the BetRepository port has no
// "updated balance" return value).
func TestPlaceResponseFromDomain_MapsBetAndBalance(t *testing.T) {
	bet := domainbetslip.Bet{
		ID: "bet-1", Type: domainbetslip.BetTypeSingle, Stake: mustMoney(t, 100),
		CombinedOdds: mustOdds(t, 1.85), PotentialReturns: mustMoney(t, 185),
		Status: domainbetslip.BetStatusAccepted, Selections: []string{"s1"},
		CreatedAt: time.Date(2026, 6, 11, 19, 0, 0, 0, time.UTC),
	}

	resp := dto.PlaceResponseFromDomain(bet, mustMoney(t, 900))

	require.Equal(t, "bet-1", resp.BetID)
	require.Equal(t, "accepted", resp.Status)
	require.Equal(t, "900.00", resp.BalanceAfter.String())
	require.Equal(t, []string{"s1"}, resp.Selections)
}

// TestBetsResponseFromDomain_MapsItemsAndCursor proves the history response
// carries every persisted bet (including rejected ones — an audit record,
// per design.md) plus the pagination cursor.
func TestBetsResponseFromDomain_MapsItemsAndCursor(t *testing.T) {
	bets := []domainbetslip.Bet{
		{ID: "bet-1", Status: domainbetslip.BetStatusAccepted, Stake: mustMoney(t, 100), CombinedOdds: mustOdds(t, 1.5), PotentialReturns: mustMoney(t, 150)},
		{ID: "bet-2", Status: domainbetslip.BetStatusRejected, RejectionReason: domainbetslip.RejectionReasonInsufficientFunds, Stake: mustMoney(t, 50), CombinedOdds: mustOdds(t, 2), PotentialReturns: mustMoney(t, 100)},
	}

	resp := dto.BetsResponseFromDomain(bets, "next-cursor")

	require.Len(t, resp.Items, 2)
	require.Equal(t, "rejected", resp.Items[1].Status)
	require.NotNil(t, resp.NextCursor)
	require.Equal(t, "next-cursor", *resp.NextCursor)
}

// TestBetsResponseFromDomain_EmptyCursorMarshalsAsNull proves an empty
// cursor (no more pages) marshals as JSON null, not an empty string (spec:
// bet-history/List Caller's Own Bets, "Caller with no bets").
func TestBetsResponseFromDomain_EmptyCursorMarshalsAsNull(t *testing.T) {
	resp := dto.BetsResponseFromDomain(nil, "")

	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"nextCursor":null`)
	require.Contains(t, string(raw), `"items":[]`)
}
