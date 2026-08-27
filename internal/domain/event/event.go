// Package event holds the World Cup 2026 reference data types: Event,
// Market, Selection, and the enrichment types Phase and Group. All 24
// events are loaded once at boot from an embedded copy of the seed data
// (internal/adapters/memory/seed) and never mutated at runtime (D3).
package event

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

// validGroups is the set of the 12 World Cup group letters for the 2026
// format (48 teams / 12 groups).
var validGroups = map[string]bool{
	"A": true, "B": true, "C": true, "D": true,
	"E": true, "F": true, "G": true, "H": true,
	"I": true, "J": true, "K": true, "L": true,
}

// Group is a validated single-letter World Cup group (A-L). An empty Group
// denotes "unknown" (see the enrichment fallback in the in-memory event
// repository) and is always valid — resolution failure must never panic.
type Group string

// NewGroup validates s as a Group letter. An empty string is always valid
// and represents "unknown"; it is never an error and never panics.
func NewGroup(s string) (Group, error) {
	if s == "" {
		return Group(""), nil
	}
	if !validGroups[s] {
		return "", ErrInvalidGroup
	}
	return Group(s), nil
}

// IsEmpty reports whether the group is unknown.
func (g Group) IsEmpty() bool {
	return g == ""
}

// Participant is one side of an Event (home or away).
type Participant struct {
	ID   string
	Name string
}

// Event is a single World Cup 2026 fixture with its ordered markets and the
// phase/group enrichment applied at load time.
type Event struct {
	ID          string
	Name        string
	StartsAt    time.Time // UTC
	League      string
	Home        Participant
	Away        Participant
	Phase       Phase
	Group       Group
	IsLive      bool
	IsSuspended bool
	Markets     []Market

	// HasStatistics and IsBetBuilderEnabled are UI metadata flags sourced
	// unchanged from the seed data (event-level, not per-market — verified
	// against docs/data.json: both flags occur exactly once per event).
	HasStatistics       bool
	IsBetBuilderEnabled bool
}

// MarketTypeID identifies one of the four default market types kept by the
// in-memory repository's load-time filter (D5).
type MarketTypeID string

// The four default market types, in their fixed display order.
const (
	MarketTypeMoneyline      MarketTypeID = "ML0"   // 1X2
	MarketTypeTotalGoals     MarketTypeID = "OU200" // Total Goals (Over/Under)
	MarketTypeBothTeamsScore MarketTypeID = "QA158" // Both Teams to Score
	MarketTypeFirstGoal      MarketTypeID = "ML235" // First Goal
)

// Market is one bettable market within an Event (e.g. 1X2, Total Goals).
type Market struct {
	ID         string
	TypeID     MarketTypeID
	Name       string
	Order      int
	Selections []Selection
}

// Selection is one outcome within a Market, carrying its own odds.
type Selection struct {
	ID         string
	MarketID   string
	EventID    string
	Name       string
	Line       *decimal.Decimal // nil when the market has no line (e.g. 1X2)
	Odds       money.Odds
	IsDisabled bool

	// OriginalOdds is nil unless this selection is present in the curated
	// Super Cuota seed, in which case it holds the pre-boost value and Odds
	// holds the boosted one — the single value used everywhere pricing
	// occurs (design: Super Cuota).
	OriginalOdds *money.Odds
}

// SelectionRef is a lightweight, resolved reference to a single betting
// selection: exactly the fields the bet slip domain needs to build a
// Quote, without pulling in the full Event/Market/Selection tree.
type SelectionRef struct {
	ID         string
	EventID    string
	MarketID   string
	Name       string
	Odds       money.Odds
	IsDisabled bool

	// EventBetBuilderEnabled is the owning Event's IsBetBuilderEnabled flag,
	// propagated at catalog index-build time (NewSelectionRef) so
	// BetSlip.Quote can evaluate Bet Builder eligibility without
	// re-querying the full Event (spec: events-catalog/SelectionRef Carries
	// Bet Builder Eligibility).
	EventBetBuilderEnabled bool

	// OriginalOdds mirrors Selection.OriginalOdds: nil unless this
	// selection carries a Super Cuota boost.
	OriginalOdds *money.Odds
}

// NewSelectionRef builds the resolved reference sel's owning event (owner)
// needs to carry: the selection's own pricing data plus owner's Bet
// Builder eligibility, propagated here rather than re-derived by every
// caller (spec: events-catalog/SelectionRef Carries Bet Builder
// Eligibility). The caller is responsible for passing the Event that
// actually owns sel (sel.EventID == owner.ID); this function trusts that
// invariant rather than re-validating it, exactly like every other
// load-time index build in this package.
func NewSelectionRef(sel Selection, owner Event) SelectionRef {
	return SelectionRef{
		ID:                     sel.ID,
		EventID:                sel.EventID,
		MarketID:               sel.MarketID,
		Name:                   sel.Name,
		Odds:                   sel.Odds,
		IsDisabled:             sel.IsDisabled,
		EventBetBuilderEnabled: owner.IsBetBuilderEnabled,
		OriginalOdds:           sel.OriginalOdds,
	}
}
