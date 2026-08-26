// Package betslip holds the Calculate and Place use cases: pricing a set of
// selections and, once authenticated, persisting an atomic placement. Both
// consume caller-supplied ports (D2 — consumer-owned interfaces, no ports/
// folder) so the HTTP adapter never talks to the in-memory catalog or
// DynamoDB directly.
package betslip

import (
	"context"

	domainbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/betslip"
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

// BetRepository persists a placement as a single atomic operation: it
// debits the balance and stores the bet when funds suffice, or persists the
// same bet with status "rejected" (no balance update) and returns
// ErrInsufficientFunds carrying that record when they do not (D8/D15).
//
// replayed=true means idempotencyKey was already used with an identical
// payload; the recorded outcome (accepted or rejected) is returned as-is,
// never re-evaluated (D16). ErrIdempotencyKeyReuse means the same key was
// used with a different payload. internal/adapters/dynamo.BetRepository is
// the production implementation; it owns all DynamoDB error translation —
// this use case never sees an AWS type.
type BetRepository interface {
	Place(ctx context.Context, b domainbetslip.Bet, idempotencyKey string) (stored domainbetslip.Bet, replayed bool, err error)
	ListByUser(ctx context.Context, userID string, limit int, cursor string) ([]domainbetslip.Bet, string, error)
}

// IDGenerator issues a fresh unique bet identifier per placement (D9 — a
// ULID, so the persisted SK sorts chronologically for free).
// internal/platform/id.ULID (Phase 10) is the production implementation.
type IDGenerator interface {
	NewID() string
}
