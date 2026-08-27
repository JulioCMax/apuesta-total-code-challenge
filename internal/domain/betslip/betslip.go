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

	// AllowSameEventCombo is the caller's explicit Bet Builder opt-in
	// (design D26). Default false preserves every existing same-event
	// rejection unchanged. A same-event combo is only ever allowed through
	// when this is true AND every selection belonging to the duplicated
	// event has EventBetBuilderEnabled — opt-in alone on a disabled event
	// is refused with the distinct ErrBetBuilderNotAvailable, never
	// silently downgraded to ErrSameEventCombo (spec: bet-slip-
	// calculation/Same-Event Combo Rejection).
	AllowSameEventCombo bool
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
	// betBuilderEnabledByEvent records each selection's own
	// EventBetBuilderEnabled flag, keyed by event ID. Every selection
	// belonging to the same event carries the same value (propagated from
	// one owning Event at catalog build time), so a later write never
	// disagrees with an earlier one for the same key.
	betBuilderEnabledByEvent := make(map[string]bool, len(b.Selections))
	// duplicateEventIDs collects EVERY event that has 2+ selections in this
	// slip, in stable selection order (first-detected-duplicate order), not
	// just the first one encountered. A slip can legitimately combine
	// same-event pairs from more than one event, and each one must be
	// checked independently — consulting only the first duplicate found the
	// outcome selection-order dependent and let a later disabled event's
	// pair slip through unchecked.
	var duplicateEventIDs []string
	seenDuplicateEvent := make(map[string]bool, len(b.Selections))

	for _, sel := range b.Selections {
		if seenSelections[sel.ID] {
			return Quote{}, ErrDuplicateSelection
		}
		seenSelections[sel.ID] = true

		if sel.IsDisabled {
			return Quote{}, ErrSelectionUnavailable
		}

		if seenEvents[sel.EventID] && !seenDuplicateEvent[sel.EventID] {
			duplicateEventIDs = append(duplicateEventIDs, sel.EventID)
			seenDuplicateEvent[sel.EventID] = true
		}
		seenEvents[sel.EventID] = true
		betBuilderEnabledByEvent[sel.EventID] = sel.EventBetBuilderEnabled
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

	if len(duplicateEventIDs) > 0 {
		if !b.AllowSameEventCombo {
			// No opt-in: the pre-existing rule, unchanged regardless of any
			// event's own flag. Names the first duplicated event in
			// selection order — deterministic even when more than one event
			// repeats.
			return Quote{}, ErrSameEventCombo{EventID: duplicateEventIDs[0]}
		}

		// Opted in: EVERY duplicated event must itself allow Bet Builder,
		// not just the first one encountered — a slip can legitimately
		// combine same-event pairs from more than one event, and any single
		// disabled one must still refuse the whole combo. duplicateEventIDs
		// is walked in selection order (not e.g. sorted or map-iteration
		// order), so the offending event named below is deliberately the
		// first OFFENDING one in selection order: the outcome never depends
		// on which duplicated event happens to be checked first (design:
		// Bet Builder rule).
		for _, eventID := range duplicateEventIDs {
			if !betBuilderEnabledByEvent[eventID] {
				// Opted in, but this event itself has Bet Builder disabled:
				// the caller asked explicitly and deserves a distinct
				// answer, never the generic same-event rejection (design:
				// Bet Builder rule).
				return Quote{}, ErrBetBuilderNotAvailable{EventID: eventID}
			}
		}
		// Opted in AND every duplicated event allows it: fall through and
		// price the combo exactly like any other, via money.Combine below.
	}

	combinedOdds := money.Combine(odds...)
	quote.Combo = &Leg{
		SelectionIDs:     selectionIDs,
		Odds:             combinedOdds,
		PotentialReturns: b.Stake.Mul(combinedOdds.Decimal()),
	}

	return quote, nil
}
