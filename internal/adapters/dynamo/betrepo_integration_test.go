package dynamo_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/dynamo"
	appbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/application/betslip"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/account"
	domainbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/betslip"
	domainevent "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/event"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/platform/id"
)

// stubCatalog is a minimal read-only appbetslip.EventCatalog for these
// integration tests: every requested ID resolves to the same fixed
// selection, so the test can focus entirely on the real DynamoDB
// placement path (mirroring internal/application/betslip's
// raceSafeCatalog, redeclared here since that type is unexported in
// another package).
type stubCatalog struct {
	ref domainevent.SelectionRef
}

func (c stubCatalog) SelectionsByIDs(_ context.Context, ids []string) ([]domainevent.SelectionRef, error) {
	refs := make([]domainevent.SelectionRef, 0, len(ids))
	for range ids {
		refs = append(refs, c.ref)
	}
	return refs, nil
}

func integrationBoundsFixture(t *testing.T) appbetslip.StakeBounds {
	t.Helper()
	min, err := money.NewMoneyFromFloat(1)
	require.NoError(t, err)
	max, err := money.NewMoneyFromFloat(10000)
	require.NoError(t, err)
	return appbetslip.StakeBounds{MinStake: min, MaxStake: max, Currency: "PEN", MaxSelections: 20}
}

func integrationMoney(t *testing.T, amount string) money.Money {
	t.Helper()
	m, err := money.NewMoney(decimal.RequireFromString(amount))
	require.NoError(t, err)
	return m
}

func seedBalance(t *testing.T, client *dynamodb.Client, table, userID string, amount float64) {
	t.Helper()
	repo := dynamo.NewUserRepository(client, table)
	bal, err := money.NewMoneyFromFloat(amount)
	require.NoError(t, err)
	user := account.User{
		ID:           userID,
		Email:        fmt.Sprintf("%s@apuestatotal.com", userID),
		PasswordHash: "bcrypt-hash-placeholder",
		Balance:      bal,
		Currency:     "PEN",
		CreatedAt:    time.Now().UTC(),
	}
	require.NoError(t, repo.PutUserIfAbsent(context.Background(), user))
}

func mustSelection(t *testing.T) domainevent.SelectionRef {
	t.Helper()
	o, err := money.NewOddsFromFloat(1.85)
	require.NoError(t, err)
	return domainevent.SelectionRef{ID: "sel-1", EventID: "evt-1", Odds: o}
}

// TestPlaceAtomically_NConcurrentGoroutines_LeavesExactBalance is the
// infrastructure-level mirror of application/betslip's
// TestPlaceBet_ConcurrentDebits_NoOverdraft (place_race_test.go), but
// firing N goroutines at the REAL DynamoDB-backed placement path instead
// of a mutex-guarded fake. A balance covering exactly one stake must
// result in exactly one accepted bet, N-1 typed insufficient-funds
// rejections, an exact final balance, and N stored bets — no lost
// updates, no double-debit (spec: bet-slip-placement/Concurrency-Safe
// Balance Debit).
func TestPlaceAtomically_NConcurrentGoroutines_LeavesExactBalance(t *testing.T) {
	client, table := requireDynamoLocal(t)
	const n = 12
	const userID = "user-concurrent"
	seedBalance(t, client, table, userID, 100) // covers exactly one 100.00 stake

	catalog := stubCatalog{ref: mustSelection(t)}
	repo := dynamo.NewBetRepository(client, table, time.Hour)
	uc := appbetslip.NewPlace(catalog, repo, id.NewULIDGenerator(), integrationBoundsFixture(t))
	stake := integrationMoney(t, "100.00")

	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := uc.Execute(context.Background(), appbetslip.PlaceCommand{
				UserID:         userID,
				SelectionIDs:   []string{"sel-1"},
				Stake:          stake,
				IdempotencyKey: fmt.Sprintf("key-%d", i), // distinct keys => independent placements
			})
			errs[i] = err
		}(i)
	}
	wg.Wait()

	accepted, rejected := 0, 0
	for _, err := range errs {
		if err == nil {
			accepted++
			continue
		}
		var insufficientErr domainbetslip.ErrInsufficientFunds
		require.ErrorAs(t, err, &insufficientErr)
		rejected++
	}
	require.Equal(t, 1, accepted, "exactly one placement must be accepted")
	require.Equal(t, n-1, rejected, "every other placement must be rejected")

	userRepo := dynamo.NewUserRepository(client, table)
	finalBalance, err := userRepo.Balance(context.Background(), userID)
	require.NoError(t, err)
	require.Equal(t, "0.00", finalBalance.String(), "final balance must be exact, never negative")

	bets, _, err := repo.ListByUser(context.Background(), userID, 0, "")
	require.NoError(t, err)
	require.Len(t, bets, n, "every attempt, accepted or rejected, must be persisted")

	acceptedCount, rejectedCount := 0, 0
	for _, b := range bets {
		switch b.Status {
		case domainbetslip.BetStatusAccepted:
			acceptedCount++
		case domainbetslip.BetStatusRejected:
			rejectedCount++
			require.Equal(t, domainbetslip.RejectionReasonInsufficientFunds, b.RejectionReason)
		}
	}
	require.Equal(t, 1, acceptedCount)
	require.Equal(t, n-1, rejectedCount)
}

// TestPlace_RejectedAttemptPersistsBetWithoutDebit proves an insufficient-
// balance placement is persisted with status "rejected" while the balance
// is left completely untouched (D15; spec: bet-slip-placement/Atomic
// Conditional Debit and Bet Persistence, "Insufficient balance").
func TestPlace_RejectedAttemptPersistsBetWithoutDebit(t *testing.T) {
	client, table := requireDynamoLocal(t)
	const userID = "user-rejected"
	seedBalance(t, client, table, userID, 50)

	catalog := stubCatalog{ref: mustSelection(t)}
	repo := dynamo.NewBetRepository(client, table, time.Hour)
	uc := appbetslip.NewPlace(catalog, repo, id.NewULIDGenerator(), integrationBoundsFixture(t))

	_, err := uc.Execute(context.Background(), appbetslip.PlaceCommand{
		UserID:         userID,
		SelectionIDs:   []string{"sel-1"},
		Stake:          integrationMoney(t, "100.00"),
		IdempotencyKey: "reject-key",
	})

	var insufficientErr domainbetslip.ErrInsufficientFunds
	require.ErrorAs(t, err, &insufficientErr)
	require.Equal(t, "50.00", insufficientErr.Balance.String())
	require.Equal(t, "100.00", insufficientErr.Required.String())

	userRepo := dynamo.NewUserRepository(client, table)
	balance, err := userRepo.Balance(context.Background(), userID)
	require.NoError(t, err)
	require.Equal(t, "50.00", balance.String(), "balance must remain unchanged after a rejection")

	bets, _, err := repo.ListByUser(context.Background(), userID, 0, "")
	require.NoError(t, err)
	require.Len(t, bets, 1)
	require.Equal(t, domainbetslip.BetStatusRejected, bets[0].Status)
	require.Equal(t, domainbetslip.RejectionReasonInsufficientFunds, bets[0].RejectionReason)
	require.Equal(t, insufficientErr.BetID, bets[0].ID)
}

// TestPlace_IdempotentReplay_NoSecondDebit proves retrying an accepted
// placement with the same Idempotency-Key returns the original bet without
// a second debit or a second bet (spec: bet-slip-placement/Idempotent
// Placement via Idempotency-Key, "Retry with the same Idempotency-Key").
func TestPlace_IdempotentReplay_NoSecondDebit(t *testing.T) {
	client, table := requireDynamoLocal(t)
	const userID = "user-replay-accepted"
	seedBalance(t, client, table, userID, 1000)

	catalog := stubCatalog{ref: mustSelection(t)}
	repo := dynamo.NewBetRepository(client, table, time.Hour)
	uc := appbetslip.NewPlace(catalog, repo, id.NewULIDGenerator(), integrationBoundsFixture(t))
	cmd := appbetslip.PlaceCommand{
		UserID:         userID,
		SelectionIDs:   []string{"sel-1"},
		Stake:          integrationMoney(t, "100.00"),
		IdempotencyKey: "replay-key",
	}

	first, err := uc.Execute(context.Background(), cmd)
	require.NoError(t, err)
	require.False(t, first.Replayed)
	require.Equal(t, domainbetslip.BetStatusAccepted, first.Bet.Status)

	second, err := uc.Execute(context.Background(), cmd)
	require.NoError(t, err)
	require.True(t, second.Replayed)
	require.Equal(t, first.Bet.ID, second.Bet.ID, "a replay must return the original bet, not a new one")

	userRepo := dynamo.NewUserRepository(client, table)
	balance, err := userRepo.Balance(context.Background(), userID)
	require.NoError(t, err)
	require.Equal(t, "900.00", balance.String(), "the stake must be debited exactly once")

	bets, _, err := repo.ListByUser(context.Background(), userID, 0, "")
	require.NoError(t, err)
	require.Len(t, bets, 1, "a replay must never create a second bet")
}

// TestPlace_IdempotentReplayOfRejection_CreatesNoSecondRecord proves
// replaying a key that previously recorded a rejection returns that exact
// recorded rejection — never an error, never a fresh re-evaluation, even
// though Place.Execute mints a brand-new candidate bet ID on every call
// (D16).
func TestPlace_IdempotentReplayOfRejection_CreatesNoSecondRecord(t *testing.T) {
	client, table := requireDynamoLocal(t)
	const userID = "user-replay-rejected"
	seedBalance(t, client, table, userID, 50)

	catalog := stubCatalog{ref: mustSelection(t)}
	repo := dynamo.NewBetRepository(client, table, time.Hour)
	uc := appbetslip.NewPlace(catalog, repo, id.NewULIDGenerator(), integrationBoundsFixture(t))
	cmd := appbetslip.PlaceCommand{
		UserID:         userID,
		SelectionIDs:   []string{"sel-1"},
		Stake:          integrationMoney(t, "100.00"),
		IdempotencyKey: "reject-replay-key",
	}

	_, err := uc.Execute(context.Background(), cmd)
	var insufficientErr domainbetslip.ErrInsufficientFunds
	require.ErrorAs(t, err, &insufficientErr)

	replay, err := uc.Execute(context.Background(), cmd)
	require.NoError(t, err, "a replayed rejection must never surface as an error")
	require.True(t, replay.Replayed)
	require.Equal(t, domainbetslip.BetStatusRejected, replay.Bet.Status)
	require.Equal(t, insufficientErr.BetID, replay.Bet.ID)

	userRepo := dynamo.NewUserRepository(client, table)
	balance, err := userRepo.Balance(context.Background(), userID)
	require.NoError(t, err)
	require.Equal(t, "50.00", balance.String())

	bets, _, err := repo.ListByUser(context.Background(), userID, 0, "")
	require.NoError(t, err)
	require.Len(t, bets, 1, "a replayed rejection must never create a second record")
}

// TestPlace_DifferentKeysSamePayloadAreIndependentPlacements proves two
// place requests with an identical payload but different Idempotency-Key
// values are each treated as an independent placement (spec: bet-slip-
// placement/Idempotent Placement via Idempotency-Key, "Different key, same
// payload").
func TestPlace_DifferentKeysSamePayloadAreIndependentPlacements(t *testing.T) {
	client, table := requireDynamoLocal(t)
	const userID = "user-independent"
	seedBalance(t, client, table, userID, 1000)

	catalog := stubCatalog{ref: mustSelection(t)}
	repo := dynamo.NewBetRepository(client, table, time.Hour)
	uc := appbetslip.NewPlace(catalog, repo, id.NewULIDGenerator(), integrationBoundsFixture(t))
	stake := integrationMoney(t, "100.00")

	first, err := uc.Execute(context.Background(), appbetslip.PlaceCommand{
		UserID: userID, SelectionIDs: []string{"sel-1"}, Stake: stake, IdempotencyKey: "key-a",
	})
	require.NoError(t, err)
	second, err := uc.Execute(context.Background(), appbetslip.PlaceCommand{
		UserID: userID, SelectionIDs: []string{"sel-1"}, Stake: stake, IdempotencyKey: "key-b",
	})
	require.NoError(t, err)

	require.NotEqual(t, first.Bet.ID, second.Bet.ID)
	require.False(t, first.Replayed)
	require.False(t, second.Replayed)

	userRepo := dynamo.NewUserRepository(client, table)
	balance, err := userRepo.Balance(context.Background(), userID)
	require.NoError(t, err)
	require.Equal(t, "800.00", balance.String(), "both stakes must be debited independently")

	bets, _, err := repo.ListByUser(context.Background(), userID, 0, "")
	require.NoError(t, err)
	require.Len(t, bets, 2)
}

// TestListByUser_PaginatesNewestFirstWithCursor proves ListByUser orders
// history newest-first (ULID SK sorts chronologically, ScanIndexForward
// false) and that its opaque cursor advances correctly across pages
// (design.md: "GET /bets is Query(...) with an opaque base64
// LastEvaluatedKey cursor").
func TestListByUser_PaginatesNewestFirstWithCursor(t *testing.T) {
	client, table := requireDynamoLocal(t)
	const userID = "user-history"
	seedBalance(t, client, table, userID, 1000)

	catalog := stubCatalog{ref: mustSelection(t)}
	repo := dynamo.NewBetRepository(client, table, time.Hour)
	uc := appbetslip.NewPlace(catalog, repo, id.NewULIDGenerator(), integrationBoundsFixture(t))

	var placedIDs []string
	for i := 0; i < 3; i++ {
		result, err := uc.Execute(context.Background(), appbetslip.PlaceCommand{
			UserID:         userID,
			SelectionIDs:   []string{"sel-1"},
			Stake:          integrationMoney(t, "10.00"),
			IdempotencyKey: fmt.Sprintf("history-key-%d", i),
		})
		require.NoError(t, err)
		placedIDs = append(placedIDs, result.Bet.ID)
		time.Sleep(2 * time.Millisecond) // guarantee distinct ULID timestamps
	}

	page1, cursor1, err := repo.ListByUser(context.Background(), userID, 2, "")
	require.NoError(t, err)
	require.Len(t, page1, 2)
	require.NotEmpty(t, cursor1)
	require.Equal(t, placedIDs[2], page1[0].ID, "newest bet must come first")
	require.Equal(t, placedIDs[1], page1[1].ID)

	page2, cursor2, err := repo.ListByUser(context.Background(), userID, 2, cursor1)
	require.NoError(t, err)
	require.Len(t, page2, 1)
	require.Empty(t, cursor2, "the last page must carry no further cursor")
	require.Equal(t, placedIDs[0], page2[0].ID, "the oldest bet must be on the final page")
}
