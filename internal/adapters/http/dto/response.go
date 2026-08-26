package dto

import (
	"time"

	appbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/application/betslip"
	domainbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/betslip"
	domainevent "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/event"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

// --- Events ---------------------------------------------------------------

// EventSummary is one event as it appears in both GET /events' list
// response and as the base of GET /events/:id's detail response. Group is
// omitted entirely (omitempty) when unresolved — never sent as "" (design.
// md's in-memory event repository section).
type EventSummary struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	StartsAt    time.Time `json:"startsAt"`
	League      string    `json:"league"`
	Home        string    `json:"home"`
	Away        string    `json:"away"`
	Phase       string    `json:"phase"`
	Group       string    `json:"group,omitempty"`
	IsLive      bool      `json:"isLive"`
	IsSuspended bool      `json:"isSuspended"`
}

// MarketTypeResponse carries the market type identifier every market entry
// MUST expose (spec: events-catalog/Market and Event Metadata Exposure).
type MarketTypeResponse struct {
	ID string `json:"id"`
}

// EventSelectionResponse is one betting outcome within a market of an event
// detail response.
type EventSelectionResponse struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Line       *string    `json:"line,omitempty"`
	Odds       money.Odds `json:"odds"`
	IsDisabled bool       `json:"isDisabled"`
}

// MarketResponse is one market within an event detail response, already in
// the fixed default order (D5): 1X2, Total Goals, Both Teams to Score,
// First Goal.
type MarketResponse struct {
	ID         string                   `json:"id"`
	MarketType MarketTypeResponse       `json:"marketType"`
	Name       string                   `json:"name"`
	Selections []EventSelectionResponse `json:"selections"`
}

// EventSettings carries the UI metadata flags the event itself MUST expose
// (spec: events-catalog/Market and Event Metadata Exposure — verified
// event-level, once per event, not per market).
type EventSettings struct {
	HasStatistics       bool `json:"hasStatistics"`
	IsBetBuilderEnabled bool `json:"isBetBuilderEnabled"`
}

// EventDetail is the full JSON body of GET /events/:id.
type EventDetail struct {
	EventSummary
	Settings EventSettings    `json:"settings"`
	Markets  []MarketResponse `json:"markets"`
}

// EventSummaryFromDomain converts a domain Event into its list/detail-base
// JSON shape.
func EventSummaryFromDomain(e domainevent.Event) EventSummary {
	group := ""
	if !e.Group.IsEmpty() {
		group = string(e.Group)
	}
	return EventSummary{
		ID:          e.ID,
		Name:        e.Name,
		StartsAt:    e.StartsAt,
		League:      e.League,
		Home:        e.Home.Name,
		Away:        e.Away.Name,
		Phase:       string(e.Phase),
		Group:       group,
		IsLive:      e.IsLive,
		IsSuspended: e.IsSuspended,
	}
}

// EventsFromDomain converts a slice of domain Events for GET /events.
func EventsFromDomain(events []domainevent.Event) []EventSummary {
	out := make([]EventSummary, 0, len(events))
	for _, e := range events {
		out = append(out, EventSummaryFromDomain(e))
	}
	return out
}

// EventDetailFromDomain converts a full domain Event for GET /events/:id.
func EventDetailFromDomain(e domainevent.Event) EventDetail {
	markets := make([]MarketResponse, 0, len(e.Markets))
	for _, m := range e.Markets {
		markets = append(markets, marketFromDomain(m))
	}
	return EventDetail{
		EventSummary: EventSummaryFromDomain(e),
		Settings:     EventSettings{HasStatistics: e.HasStatistics, IsBetBuilderEnabled: e.IsBetBuilderEnabled},
		Markets:      markets,
	}
}

func marketFromDomain(m domainevent.Market) MarketResponse {
	selections := make([]EventSelectionResponse, 0, len(m.Selections))
	for _, s := range m.Selections {
		selections = append(selections, selectionFromDomain(s))
	}
	return MarketResponse{
		ID:         m.ID,
		MarketType: MarketTypeResponse{ID: string(m.TypeID)},
		Name:       m.Name,
		Selections: selections,
	}
}

func selectionFromDomain(s domainevent.Selection) EventSelectionResponse {
	var line *string
	if s.Line != nil {
		v := s.Line.String()
		line = &v
	}
	return EventSelectionResponse{ID: s.ID, Name: s.Name, Line: line, Odds: s.Odds, IsDisabled: s.IsDisabled}
}

// --- BetSlip calculate ------------------------------------------------------

// ResolvedSelectionResponse is one requested selection as resolved against
// the event catalog (spec: bet-slip-calculation/Selection Resolution).
type ResolvedSelectionResponse struct {
	ID      string     `json:"id"`
	EventID string     `json:"eventId"`
	Odds    money.Odds `json:"odds"`
}

// SingleResponse is one priced Single leg.
type SingleResponse struct {
	SelectionID      string      `json:"selectionId"`
	Odds             money.Odds  `json:"odds"`
	PotentialReturns money.Money `json:"potentialReturns"`
}

// ComboResponse is the priced Combo leg, present only for 2+ selections
// spanning distinct events (spec: bet-slip-calculation/Single and Combo Bet
// Generation).
type ComboResponse struct {
	SelectionIDs     []string    `json:"selectionIds"`
	CombinedOdds     money.Odds  `json:"combinedOdds"`
	PotentialReturns money.Money `json:"potentialReturns"`
}

// CalculateResponse is the JSON body of a successful POST /betslip/
// calculate (spec: bet-slip-calculation/Calculate Endpoint Response Shape).
type CalculateResponse struct {
	MinStake   money.Money                 `json:"minStake"`
	MaxStake   money.Money                 `json:"maxStake"`
	Currency   string                      `json:"currency"`
	Stake      money.Money                 `json:"stake"`
	Selections []ResolvedSelectionResponse `json:"selections"`
	Singles    []SingleResponse            `json:"singles"`
	Combo      *ComboResponse              `json:"combo"`
}

// CalculateResponseFromDomain converts the Calculate use case's result into
// its JSON shape.
func CalculateResponseFromDomain(result appbetslip.CalculateResult) CalculateResponse {
	selections := make([]ResolvedSelectionResponse, 0, len(result.Selections))
	for _, s := range result.Selections {
		selections = append(selections, ResolvedSelectionResponse{ID: s.ID, EventID: s.EventID, Odds: s.Odds})
	}

	singles := make([]SingleResponse, 0, len(result.Quote.Singles))
	for _, leg := range result.Quote.Singles {
		selectionID := ""
		if len(leg.SelectionIDs) > 0 {
			selectionID = leg.SelectionIDs[0]
		}
		singles = append(singles, SingleResponse{SelectionID: selectionID, Odds: leg.Odds, PotentialReturns: leg.PotentialReturns})
	}

	var combo *ComboResponse
	if result.Quote.Combo != nil {
		combo = &ComboResponse{
			SelectionIDs:     result.Quote.Combo.SelectionIDs,
			CombinedOdds:     result.Quote.Combo.Odds,
			PotentialReturns: result.Quote.Combo.PotentialReturns,
		}
	}

	return CalculateResponse{
		MinStake:   result.MinStake,
		MaxStake:   result.MaxStake,
		Currency:   result.Currency,
		Stake:      result.Stake,
		Selections: selections,
		Singles:    singles,
		Combo:      combo,
	}
}

// --- BetSlip place ----------------------------------------------------------

// PlaceResponse is the JSON body of a successful POST /betslip/place
// (design.md's HTTP Layer section). BalanceAfter is read via a separate
// balance query, since BetRepository.Place returns only the stored bet.
type PlaceResponse struct {
	BetID            string      `json:"betId"`
	Type             string      `json:"type"`
	Status           string      `json:"status"`
	Stake            money.Money `json:"stake"`
	CombinedOdds     money.Odds  `json:"combinedOdds"`
	PotentialReturns money.Money `json:"potentialReturns"`
	BalanceAfter     money.Money `json:"balanceAfter"`
	CreatedAt        time.Time   `json:"createdAt"`
	Selections       []string    `json:"selections"`
}

// PlaceResponseFromDomain converts an accepted (or replayed-accepted) Bet
// plus the caller's current balance into its JSON shape.
func PlaceResponseFromDomain(bet domainbetslip.Bet, balanceAfter money.Money) PlaceResponse {
	return PlaceResponse{
		BetID:            bet.ID,
		Type:             string(bet.Type),
		Status:           string(bet.Status),
		Stake:            bet.Stake,
		CombinedOdds:     bet.CombinedOdds,
		PotentialReturns: bet.PotentialReturns,
		BalanceAfter:     balanceAfter,
		CreatedAt:        bet.CreatedAt,
		Selections:       bet.Selections,
	}
}

// --- Bet history ------------------------------------------------------------

// BetResponse is one entry in GET /bets — both accepted and rejected bets
// are listed (a rejected attempt is an audit record, design.md).
type BetResponse struct {
	BetID            string      `json:"betId"`
	Type             string      `json:"type"`
	Status           string      `json:"status"`
	RejectionReason  string      `json:"rejectionReason,omitempty"`
	Stake            money.Money `json:"stake"`
	CombinedOdds     money.Odds  `json:"combinedOdds"`
	PotentialReturns money.Money `json:"potentialReturns"`
	CreatedAt        time.Time   `json:"createdAt"`
	Selections       []string    `json:"selections"`
}

// BetsResponse is the JSON body of GET /bets.
type BetsResponse struct {
	Items      []BetResponse `json:"items"`
	NextCursor *string       `json:"nextCursor"`
}

// BetsResponseFromDomain converts a caller's bet history page into its JSON
// shape. An empty cursor marshals as JSON null (no more pages), never "".
func BetsResponseFromDomain(bets []domainbetslip.Bet, nextCursor string) BetsResponse {
	items := make([]BetResponse, 0, len(bets))
	for _, b := range bets {
		items = append(items, BetResponse{
			BetID:            b.ID,
			Type:             string(b.Type),
			Status:           string(b.Status),
			RejectionReason:  b.RejectionReason,
			Stake:            b.Stake,
			CombinedOdds:     b.CombinedOdds,
			PotentialReturns: b.PotentialReturns,
			CreatedAt:        b.CreatedAt,
			Selections:       b.Selections,
		})
	}

	var cursor *string
	if nextCursor != "" {
		cursor = &nextCursor
	}

	return BetsResponse{Items: items, NextCursor: cursor}
}

// --- Auth / balance ---------------------------------------------------------

// LoginResponse is the JSON body of a successful POST /auth/login.
type LoginResponse struct {
	Token     string `json:"token"`
	ExpiresIn int64  `json:"expiresIn"`
}

// BalanceResponse is the JSON body of GET /balance.
type BalanceResponse struct {
	Balance  money.Money `json:"balance"`
	Currency string      `json:"currency"`
}
