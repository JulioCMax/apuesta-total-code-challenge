package auth_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/application/auth"
	domainbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/betslip"
)

// TestHistory_ScopesToCallerUserID proves the History use case passes the
// caller's own userID through to BetHistory and returns only that user's
// bets — never another user's (spec: bet-history/List Caller's Own Bets).
func TestHistory_ScopesToCallerUserID(t *testing.T) {
	repo := &fakeBetHistory{
		betsByUser: map[string][]domainbetslip.Bet{
			"user-a": {{ID: "bet-a1"}, {ID: "bet-a2"}},
			"user-b": {{ID: "bet-b1"}, {ID: "bet-b2"}, {ID: "bet-b3"}},
		},
	}
	uc := auth.NewHistory(repo)

	result, err := uc.Execute(context.Background(), auth.HistoryCommand{UserID: "user-a", Limit: 10})

	require.NoError(t, err)
	require.Equal(t, "user-a", repo.lastUserID)
	require.Len(t, result.Bets, 2)
	for _, b := range result.Bets {
		require.NotEqual(t, "bet-b1", b.ID)
		require.NotEqual(t, "bet-b2", b.ID)
		require.NotEqual(t, "bet-b3", b.ID)
	}
}

// TestHistory_CallerWithNoBetsReturnsEmptyList proves an authenticated
// caller who has placed no bets gets an empty result, not an error (spec:
// bet-history/List Caller's Own Bets, "Caller with no bets").
func TestHistory_CallerWithNoBetsReturnsEmptyList(t *testing.T) {
	repo := &fakeBetHistory{betsByUser: map[string][]domainbetslip.Bet{}}
	uc := auth.NewHistory(repo)

	result, err := uc.Execute(context.Background(), auth.HistoryCommand{UserID: "user-a", Limit: 10})

	require.NoError(t, err)
	require.Empty(t, result.Bets)
}
