package betslip

import (
	"context"
	"time"

	domainbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/betslip"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

// PlaceCommand is the input to the Place use case. The selection count
// determines the bet type exactly as BetSlip.Quote does: one selection is
// always a Single, 2+ distinct-event selections are always a Combo — the
// caller does not choose the type separately, it follows from the
// selections it submits.
type PlaceCommand struct {
	UserID         string
	SelectionIDs   []string
	Stake          money.Money
	IdempotencyKey string

	// IsBetBuilder is the caller's explicit Bet Builder opt-in, threaded
	// unchanged into BetSlip.AllowSameEventCombo exactly like Calculate
	// (spec: bet-slip-placement/Bet Builder Flag Threading (Place)).
	IsBetBuilder bool
}

// PlaceResult is the output of the Place use case: the persisted bet
// (accepted or rejected — see BetRepository) and whether it was returned
// via an idempotent replay rather than a fresh placement.
type PlaceResult struct {
	Bet      domainbetslip.Bet
	Replayed bool
}

// Place is the "bet slip place" use case (spec: bet-slip-placement/Atomic
// Conditional Debit and Bet Persistence).
type Place struct {
	catalog EventCatalog
	repo    BetRepository
	ids     IDGenerator
	bounds  StakeBounds
}

// NewPlace builds a Place use case backed by catalog, repo and ids, priced
// against bounds (read from configuration by the composition root).
func NewPlace(catalog EventCatalog, repo BetRepository, ids IDGenerator, bounds StakeBounds) *Place {
	return &Place{catalog: catalog, repo: repo, ids: ids, bounds: bounds}
}

// Execute prices cmd exactly like Calculate, builds the resulting Bet with a
// fresh id, and hands it to BetRepository.Place, which owns all atomicity,
// rejection-persistence and idempotent-replay semantics. Any typed domain
// error from resolution, pricing, or persistence surfaces unchanged.
func (p *Place) Execute(ctx context.Context, cmd PlaceCommand) (PlaceResult, error) {
	refs, err := p.catalog.SelectionsByIDs(ctx, cmd.SelectionIDs)
	if err != nil {
		return PlaceResult{}, err
	}

	slip := domainbetslip.BetSlip{Selections: refs, Stake: cmd.Stake, AllowSameEventCombo: cmd.IsBetBuilder}
	quote, err := slip.Quote(p.bounds.MinStake, p.bounds.MaxStake, p.bounds.MaxSelections)
	if err != nil {
		return PlaceResult{}, err
	}

	leg := quote.Singles[0]
	betType := domainbetslip.BetTypeSingle
	if quote.Combo != nil {
		leg = *quote.Combo
		betType = domainbetslip.BetTypeCombo
	}

	bet := domainbetslip.Bet{
		ID:               p.ids.NewID(),
		UserID:           cmd.UserID,
		Type:             betType,
		Stake:            cmd.Stake,
		CombinedOdds:     leg.Odds,
		PotentialReturns: leg.PotentialReturns,
		Status:           domainbetslip.BetStatusAccepted, // intended status; BetRepository may downgrade it
		Selections:       leg.SelectionIDs,
		CreatedAt:        time.Now().UTC(),
	}

	stored, replayed, err := p.repo.Place(ctx, bet, cmd.IdempotencyKey)
	if err != nil {
		return PlaceResult{}, err
	}

	return PlaceResult{Bet: stored, Replayed: replayed}, nil
}
