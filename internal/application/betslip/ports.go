// Package betslip holds the Calculate and Place use cases: pricing a set of
// selections and, once authenticated, persisting an atomic placement. Both
// consume caller-supplied ports (D2 — consumer-owned interfaces, no ports/
// folder) so the HTTP adapter never talks to the in-memory catalog or
// DynamoDB directly.
package betslip

import (
	"context"

	domainevent "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/event"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

// EventCatalog resolves selection IDs into the pricing data BetSlip.Quote
// needs. internal/adapters/memory.EventRepository is the production
// implementation. A missing ID yields
// domain/betslip.ErrSelectionNotFound{SelectionID: id}.
type EventCatalog interface {
	SelectionsByIDs(ctx context.Context, ids []string) ([]domainevent.SelectionRef, error)
}

// StakeBounds carries the configured stake bounds, currency, and maximum
// selection count consumed by Calculate and Place (spec: bet-slip-
// calculation/Calculate Endpoint Response Shape and /Stake Bounds
// Validation — these MUST come from configuration, never a hardcoded
// literal). internal/platform/config (Phase 10) is the production source;
// this type has no dependency on it so application/betslip stays
// framework- and infra-agnostic.
type StakeBounds struct {
	MinStake      money.Money
	MaxStake      money.Money
	Currency      string
	MaxSelections int
}
