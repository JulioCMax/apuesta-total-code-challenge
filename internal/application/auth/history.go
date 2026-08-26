package auth

import (
	"context"

	domainbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/betslip"
)

// HistoryCommand is the input to the History use case.
type HistoryCommand struct {
	UserID string
	Limit  int
	Cursor string
}

// HistoryResult is the output of the History use case.
type HistoryResult struct {
	Bets       []domainbetslip.Bet
	NextCursor string
}

// History is the "list caller's own bets" use case (spec: bet-history/List
// Caller's Own Bets).
type History struct {
	bets BetHistory
}

// NewHistory builds a History use case backed by bets.
func NewHistory(bets BetHistory) *History {
	return &History{bets: bets}
}

// Execute returns cmd.UserID's own bets. Scoping to the caller is enforced
// by BetHistory (a DynamoDB Query against that user's partition); this use
// case never accepts or forwards any other user's ID than the one the HTTP
// layer derived from the caller's own verified JWT.
func (h *History) Execute(ctx context.Context, cmd HistoryCommand) (HistoryResult, error) {
	bets, next, err := h.bets.ListByUser(ctx, cmd.UserID, cmd.Limit, cmd.Cursor)
	if err != nil {
		return HistoryResult{}, err
	}
	return HistoryResult{Bets: bets, NextCursor: next}, nil
}
