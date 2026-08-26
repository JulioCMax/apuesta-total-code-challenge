package betslip_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	appbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/application/betslip"
	domainbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/betslip"
	domainevent "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/event"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

// raceSafeCatalog is a read-only EventCatalog stub for the concurrency
// test: unlike fakeEventCatalog (used elsewhere in this package), it
// records no call arguments, so N goroutines can call it simultaneously
// without racing on shared test-double state that has nothing to do with
// the property under test.
type raceSafeCatalog struct {
	refs []domainevent.SelectionRef
}

func (c raceSafeCatalog) SelectionsByIDs(_ context.Context, _ []string) ([]domainevent.SelectionRef, error) {
	return c.refs, nil
}

// atomicIDGenerator issues unique ids under concurrent use via an atomic
// counter — a minimal concurrency-safe stand-in for the real ULID
// generator (internal/platform/id, Phase 10), which does not exist yet.
type atomicIDGenerator struct{ counter int64 }

func (g *atomicIDGenerator) NewID() string {
	return fmt.Sprintf("bet-%d", atomic.AddInt64(&g.counter, 1))
}

// concurrentFakeBetRepository is the mutex-guarded stand-in for the real
// DynamoDB TransactWriteItems debit (D8): every Place call is serialized
// through a single lock around "check balance, then debit", exactly the
// atomicity a single conditional transaction provides in production.
type concurrentFakeBetRepository struct {
	mu      sync.Mutex
	balance money.Money
	bets    []domainbetslip.Bet
}

func newConcurrentFakeBetRepository(initialBalance money.Money) *concurrentFakeBetRepository {
	return &concurrentFakeBetRepository{balance: initialBalance}
}

func (r *concurrentFakeBetRepository) Place(_ context.Context, b domainbetslip.Bet, _ string) (domainbetslip.Bet, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.balance.LessThan(b.Stake) {
		rejected := b
		rejected.Status = domainbetslip.BetStatusRejected
		rejected.RejectionReason = domainbetslip.RejectionReasonInsufficientFunds
		r.bets = append(r.bets, rejected)
		return domainbetslip.Bet{}, false, domainbetslip.ErrInsufficientFunds{
			BetID:    rejected.ID,
			Balance:  r.balance,
			Required: b.Stake,
		}
	}

	r.balance = r.balance.Sub(b.Stake)
	accepted := b
	accepted.Status = domainbetslip.BetStatusAccepted
	r.bets = append(r.bets, accepted)
	return accepted, false, nil
}

func (r *concurrentFakeBetRepository) ListByUser(_ context.Context, _ string, _ int, _ string) ([]domainbetslip.Bet, string, error) {
	return nil, "", nil
}

func (r *concurrentFakeBetRepository) finalBalance() money.Money {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.balance
}

func (r *concurrentFakeBetRepository) storedBetCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.bets)
}

// TestPlaceBet_ConcurrentDebits_NoOverdraft proves that N concurrent
// placements against a balance covering exactly one stake result in
// exactly one accepted bet, N-1 typed insufficient-funds rejections, and an
// exact final balance — no lost updates, no double-debit, no negative
// balance (spec: bet-slip-placement/Concurrency-Safe Balance Debit). Run
// under `go test -race` to also prove no data race in Place itself.
func TestPlaceBet_ConcurrentDebits_NoOverdraft(t *testing.T) {
	const stakeAmount = "100.00"
	const n = 20

	sel := domainevent.SelectionRef{ID: "sel-1", EventID: "evt-1", Odds: mustOdds(t, "1.85")}
	catalog := raceSafeCatalog{refs: []domainevent.SelectionRef{sel}}
	repo := newConcurrentFakeBetRepository(mustMoney(t, stakeAmount)) // covers exactly one stake
	ids := &atomicIDGenerator{}
	uc := appbetslip.NewPlace(catalog, repo, ids, defaultBoundsFixture(t))

	stake := mustMoney(t, stakeAmount)

	var wg sync.WaitGroup
	errs := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := uc.Execute(context.Background(), appbetslip.PlaceCommand{
				UserID:         "user-1",
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
	require.Equal(t, "0.00", repo.finalBalance().String(), "final balance must be exact, never negative")
	require.Equal(t, n, repo.storedBetCount(), "every attempt, accepted or rejected, must be persisted")
}
