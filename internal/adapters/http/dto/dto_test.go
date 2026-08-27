package dto_test

import (
	"encoding/json"
	"strings"
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

// TestEventSummaryFromDomain_ExposesSettingsInTheList proves the UI
// metadata flags travel with every entry of GET /events, not only with the
// detail response.
//
// The reference design renders the statistics and BetBuilder badges on the
// COLLAPSED card in the list. Serving those flags only from
// GET /events/{id} would force a client to fetch the detail of every listed
// event just to draw its badges — one request per row, for metadata the
// list already knows.
func TestEventSummaryFromDomain_ExposesSettingsInTheList(t *testing.T) {
	e := domainevent.Event{
		ID:                  "evt-settings",
		HasStatistics:       true,
		IsBetBuilderEnabled: true,
	}

	summary := dto.EventSummaryFromDomain(e)
	require.True(t, summary.Settings.HasStatistics)
	require.True(t, summary.Settings.IsBetBuilderEnabled)

	raw, err := json.Marshal(summary)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"settings"`)
	require.Contains(t, string(raw), `"hasStatistics":true`)
	require.Contains(t, string(raw), `"isBetBuilderEnabled":true`)
}

// TestEventDetailFromDomain_SettingsAppearExactlyOnce guards the embedding.
// EventDetail embeds EventSummary, so a second Settings field declared on
// the detail would put two "settings" keys in play; encoding/json would
// silently resolve the clash by depth and the losing one would vanish
// without a compile error. One key, one source.
func TestEventDetailFromDomain_SettingsAppearExactlyOnce(t *testing.T) {
	e := domainevent.Event{ID: "evt-once", HasStatistics: true}

	raw, err := json.Marshal(dto.EventDetailFromDomain(e))
	require.NoError(t, err)
	require.Equal(t, 1, strings.Count(string(raw), `"settings"`))
	require.Contains(t, string(raw), `"hasStatistics":true`)
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

// TestBetSlipRequest_IsBetBuilder_DefaultsFalseWhenOmitted proves the
// request DTO defaults isBetBuilder to false when the caller omits it
// (spec: bet-slip-calculation/Bet Builder Flag Threading (Calculate),
// "Field omitted").
func TestBetSlipRequest_IsBetBuilder_DefaultsFalseWhenOmitted(t *testing.T) {
	var req dto.BetSlipRequest
	require.NoError(t, json.Unmarshal([]byte(`{"selectionIds":["s1"],"stake":100}`), &req))

	require.False(t, req.IsBetBuilder)
}

// TestBetSlipRequest_IsBetBuilder_ParsesExplicitTrue is the triangulation
// case for the field above.
func TestBetSlipRequest_IsBetBuilder_ParsesExplicitTrue(t *testing.T) {
	var req dto.BetSlipRequest
	require.NoError(t, json.Unmarshal([]byte(`{"selectionIds":["s1"],"stake":100,"isBetBuilder":true}`), &req))

	require.True(t, req.IsBetBuilder)
}

// TestEventDetailFromDomain_OriginalOddsOmittedWhenAbsent proves a
// non-boosted selection's JSON omits originalOdds entirely rather than
// sending null (spec: events-catalog/Selection Exposes Original Odds When
// Boosted, "Non-curated selection loaded").
func TestEventDetailFromDomain_OriginalOddsOmittedWhenAbsent(t *testing.T) {
	e := domainevent.Event{
		ID: "evt-1",
		Markets: []domainevent.Market{
			{ID: "m1", TypeID: domainevent.MarketTypeMoneyline, Selections: []domainevent.Selection{
				{ID: "s1", Odds: mustOdds(t, 1.85)},
			}},
		},
	}

	raw, err := json.Marshal(dto.EventDetailFromDomain(e))
	require.NoError(t, err)
	require.NotContains(t, string(raw), `"originalOdds"`)
}

// TestEventDetailFromDomain_BoostedSelectionIncludesOriginalOdds is the
// triangulation case: a boosted selection MUST render both odds and a
// distinct originalOdds (spec: events-catalog/Selection Exposes Original
// Odds When Boosted, "Curated Super Cuota selection loaded").
func TestEventDetailFromDomain_BoostedSelectionIncludesOriginalOdds(t *testing.T) {
	original := mustOdds(t, 1.47)
	e := domainevent.Event{
		ID: "evt-1",
		Markets: []domainevent.Market{
			{ID: "m1", TypeID: domainevent.MarketTypeMoneyline, Selections: []domainevent.Selection{
				{ID: "s1", Odds: mustOdds(t, 1.60), OriginalOdds: &original},
			}},
		},
	}

	detail := dto.EventDetailFromDomain(e)
	require.NotNil(t, detail.Markets[0].Selections[0].OriginalOdds)
	require.Equal(t, "1.47", detail.Markets[0].Selections[0].OriginalOdds.String())

	raw, err := json.Marshal(detail)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"originalOdds":1.47`)
}

// TestCalculateResponseFromDomain_BoostedSelectionExposesOriginalOdds
// proves a resolved selection carrying a Super Cuota boost renders both
// values distinctly in the calculate response (spec: bet-slip-calculation/
// Boosted Selection Exposes Original Odds).
func TestCalculateResponseFromDomain_BoostedSelectionExposesOriginalOdds(t *testing.T) {
	original := mustOdds(t, 1.47)
	result := appbetslip.CalculateResult{
		MinStake: mustMoney(t, 1), MaxStake: mustMoney(t, 10000), Currency: "PEN",
		Stake:      mustMoney(t, 100),
		Selections: []appbetslip.ResolvedSelection{{ID: "s1", EventID: "e1", Odds: mustOdds(t, 1.60), OriginalOdds: &original}},
		Quote: domainbetslip.Quote{
			Singles: []domainbetslip.Leg{{SelectionIDs: []string{"s1"}, Odds: mustOdds(t, 1.60), PotentialReturns: mustMoney(t, 160)}},
		},
	}

	resp := dto.CalculateResponseFromDomain(result)

	require.NotNil(t, resp.Selections[0].OriginalOdds)
	require.Equal(t, "1.47", resp.Selections[0].OriginalOdds.String())

	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"originalOdds":1.47`)
}

// TestCalculateResponseFromDomain_NonBoostedSelectionOmitsOriginalOdds is
// the companion case: a non-boosted selection's originalOdds must be
// absent from the JSON, never null (spec: bet-slip-calculation/Boosted
// Selection Exposes Original Odds, "Non-boosted selection resolved").
func TestCalculateResponseFromDomain_NonBoostedSelectionOmitsOriginalOdds(t *testing.T) {
	result := appbetslip.CalculateResult{
		MinStake: mustMoney(t, 1), MaxStake: mustMoney(t, 10000), Currency: "PEN",
		Stake:      mustMoney(t, 100),
		Selections: []appbetslip.ResolvedSelection{{ID: "s1", EventID: "e1", Odds: mustOdds(t, 1.85)}},
		Quote: domainbetslip.Quote{
			Singles: []domainbetslip.Leg{{SelectionIDs: []string{"s1"}, Odds: mustOdds(t, 1.85), PotentialReturns: mustMoney(t, 185)}},
		},
	}

	raw, err := json.Marshal(dto.CalculateResponseFromDomain(result))
	require.NoError(t, err)
	require.NotContains(t, string(raw), `"originalOdds"`)
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

	balance := mustMoney(t, 900)
	resp := dto.PlaceResponseFromDomain(bet, &balance)

	require.Equal(t, "bet-1", resp.BetID)
	require.Equal(t, "accepted", resp.Status)
	require.NotNil(t, resp.BalanceAfter)
	require.Equal(t, "900.00", resp.BalanceAfter.String())
	require.Equal(t, []string{"s1"}, resp.Selections)

	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"balanceAfter":900.00`, "a known balance must stay an unquoted fixed-2 number")
}

// TestPlaceResponseFromDomain_UnreadableBalanceMarshalsAsNull proves the
// response still describes the committed bet when the post-placement
// balance could not be read, rendering balanceAfter as an explicit null
// rather than a misleading 0.00.
func TestPlaceResponseFromDomain_UnreadableBalanceMarshalsAsNull(t *testing.T) {
	bet := domainbetslip.Bet{ID: "bet-1", Status: domainbetslip.BetStatusAccepted, Stake: mustMoney(t, 100), CombinedOdds: mustOdds(t, 1.85), PotentialReturns: mustMoney(t, 185)}

	raw, err := json.Marshal(dto.PlaceResponseFromDomain(bet, nil))

	require.NoError(t, err)
	require.Contains(t, string(raw), `"balanceAfter":null`)
	require.Contains(t, string(raw), `"betId":"bet-1"`)
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

// TestBetSlipRequest_StakeMoney_RejectsAbsurdMagnitude proves an
// attacker-supplied exponent can never reach decimal rounding. A literal
// such as 1e10000000 is a perfectly valid JSON number that shopspring/
// decimal accepts (exponents up to int32), but rescaling it to 2 decimal
// places materialises a ten-million-digit big.Int: seconds of CPU and
// hundreds of MB of allocation per unauthenticated request. StakeMoney
// MUST reject the magnitude at the request boundary, before any rounding
// happens, and MUST return promptly.
func TestBetSlipRequest_StakeMoney_RejectsAbsurdMagnitude(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"huge positive exponent", `{"selectionIds":["s1"],"stake":1e10000}`},
		{"catastrophic positive exponent", `{"selectionIds":["s1"],"stake":1e10000000}`},
		{"catastrophic negative exponent", `{"selectionIds":["s1"],"stake":1e-10000000}`},
		{"negative sign with huge exponent", `{"selectionIds":["s1"],"stake":-1e10000000}`},
		{"absurdly long literal", `{"selectionIds":["s1"],"stake":` + strings.Repeat("9", 400) + `}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req dto.BetSlipRequest
			require.NoError(t, json.Unmarshal([]byte(tt.body), &req))

			done := make(chan error, 1)
			go func() {
				_, err := req.StakeMoney()
				done <- err
			}()

			select {
			case err := <-done:
				require.Error(t, err, "an absurd stake magnitude must be rejected")
			case <-time.After(2 * time.Second):
				t.Fatal("StakeMoney did not return within 2s: the stake magnitude reached decimal rounding")
			}
		})
	}
}

// TestBetSlipRequest_StakeMoney_AcceptsRealisticAmounts is the
// triangulation case for the magnitude guard: every amount a real caller
// can legitimately send MUST still parse exactly.
func TestBetSlipRequest_StakeMoney_AcceptsRealisticAmounts(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"minimum stake", `{"selectionIds":["s1"],"stake":1}`, "1.00"},
		{"maximum stake", `{"selectionIds":["s1"],"stake":10000}`, "10000.00"},
		{"two decimals", `{"selectionIds":["s1"],"stake":33.10}`, "33.10"},
		{"one cent", `{"selectionIds":["s1"],"stake":0.01}`, "0.01"},
		{"scientific notation", `{"selectionIds":["s1"],"stake":1e3}`, "1000.00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req dto.BetSlipRequest
			require.NoError(t, json.Unmarshal([]byte(tt.body), &req))

			got, err := req.StakeMoney()

			require.NoError(t, err)
			require.Equal(t, tt.want, got.String())
		})
	}
}
