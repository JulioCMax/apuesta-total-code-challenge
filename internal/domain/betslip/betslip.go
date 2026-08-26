// Package betslip holds the BetSlip aggregate (pricing a set of selections
// into Singles and an optional Combo) and the Bet entity that results from
// placing a priced slip.
package betslip

import (
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/event"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

// BetSlip is the aggregate that prices a set of resolved selections against
// a single stake.
type BetSlip struct {
	Selections []event.SelectionRef
	Stake      money.Money
}

// Leg is one priced outcome within a Quote: a Single carries exactly one
// selection ID and that selection's own odds; the Combo carries every
// selection ID and the combined odds (D7).
type Leg struct {
	SelectionIDs     []string
	Odds             money.Odds
	PotentialReturns money.Money
}

// Quote is the result of pricing a BetSlip: one Single per selection, plus
// a Combo when 2+ selections span distinct events (nil otherwise).
type Quote struct {
	Singles []Leg
	Combo   *Leg
}

// Quote prices the slip's selections against the caller-supplied stake
// bounds and selection-count limit (read from configuration by the
// application layer, never hardcoded here — spec: Calculate Endpoint
// Response Shape / Stake Bounds Validation).
func (b BetSlip) Quote(minStake, maxStake money.Money, maxSelections int) (Quote, error) {
	if len(b.Selections) == 0 {
		return Quote{}, ErrEmptySlip
	}
	if maxSelections > 0 && len(b.Selections) > maxSelections {
		return Quote{}, ErrTooManySelections
	}
	if b.Stake.LessThan(minStake) || b.Stake.GreaterThan(maxStake) {
		return Quote{}, StakeOutOfRangeError{Min: minStake, Max: maxStake, Got: b.Stake}
	}

	seenSelections := make(map[string]bool, len(b.Selections))
	seenEvents := make(map[string]bool, len(b.Selections))
	var duplicateEventID string

	for _, sel := range b.Selections {
		if seenSelections[sel.ID] {
			return Quote{}, ErrDuplicateSelection
		}
		seenSelections[sel.ID] = true

		if sel.IsDisabled {
			return Quote{}, ErrSelectionUnavailable
		}

		if seenEvents[sel.EventID] && duplicateEventID == "" {
			duplicateEventID = sel.EventID
		}
		seenEvents[sel.EventID] = true
	}

	singles := make([]Leg, 0, len(b.Selections))
	odds := make([]money.Odds, 0, len(b.Selections))
	selectionIDs := make([]string, 0, len(b.Selections))
	for _, sel := range b.Selections {
		singles = append(singles, Leg{
			SelectionIDs:     []string{sel.ID},
			Odds:             sel.Odds,
			PotentialReturns: b.Stake.Mul(sel.Odds.Decimal()),
		})
		odds = append(odds, sel.Odds)
		selectionIDs = append(selectionIDs, sel.ID)
	}

	quote := Quote{Singles: singles}

	if len(b.Selections) == 1 {
		return quote, nil
	}

	if duplicateEventID != "" {
		return Quote{}, ErrSameEventCombo{EventID: duplicateEventID}
	}

	combinedOdds := money.Combine(odds...)
	quote.Combo = &Leg{
		SelectionIDs:     selectionIDs,
		Odds:             combinedOdds,
		PotentialReturns: b.Stake.Mul(combinedOdds.Decimal()),
	}

	return quote, nil
}
