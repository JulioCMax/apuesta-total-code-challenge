package betslip

import (
	"context"

	domainbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/betslip"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

// CalculateCommand is the input to the Calculate use case.
type CalculateCommand struct {
	SelectionIDs []string
	Stake        money.Money
}

// ResolvedSelection is one requested selection as resolved against the
// event catalog, echoed back in CalculateResult.
type ResolvedSelection struct {
	ID      string
	EventID string
	Odds    money.Odds
}

// CalculateResult is the output of the Calculate use case: the configured
// bounds/currency, the resolved selections, and the priced Quote.
type CalculateResult struct {
	MinStake   money.Money
	MaxStake   money.Money
	Currency   string
	Stake      money.Money
	Selections []ResolvedSelection
	Quote      domainbetslip.Quote
}

// Calculate is the "bet slip calculate" use case (spec: bet-slip-
// calculation/Calculate Endpoint Response Shape).
type Calculate struct {
	catalog EventCatalog
	bounds  StakeBounds
}

// NewCalculate builds a Calculate use case backed by catalog and priced
// against bounds (read from configuration by the composition root).
func NewCalculate(catalog EventCatalog, bounds StakeBounds) *Calculate {
	return &Calculate{catalog: catalog, bounds: bounds}
}

// Execute resolves cmd's selections, prices them, and returns the result.
// A selection ID absent from the catalog or any BetSlip.Quote rule
// violation (stake bounds, same-event combo, duplicate/disabled selection,
// too many selections) surfaces as the corresponding typed domain error.
func (c *Calculate) Execute(ctx context.Context, cmd CalculateCommand) (CalculateResult, error) {
	refs, err := c.catalog.SelectionsByIDs(ctx, cmd.SelectionIDs)
	if err != nil {
		return CalculateResult{}, err
	}

	slip := domainbetslip.BetSlip{Selections: refs, Stake: cmd.Stake}
	quote, err := slip.Quote(c.bounds.MinStake, c.bounds.MaxStake, c.bounds.MaxSelections)
	if err != nil {
		return CalculateResult{}, err
	}

	selections := make([]ResolvedSelection, 0, len(refs))
	for _, ref := range refs {
		selections = append(selections, ResolvedSelection{ID: ref.ID, EventID: ref.EventID, Odds: ref.Odds})
	}

	return CalculateResult{
		MinStake:   c.bounds.MinStake,
		MaxStake:   c.bounds.MaxStake,
		Currency:   c.bounds.Currency,
		Stake:      cmd.Stake,
		Selections: selections,
		Quote:      quote,
	}, nil
}
