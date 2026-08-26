package betslip_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	appbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/application/betslip"
	domainbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/betslip"
	domainevent "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/event"
)

// fakeBetRepository is a test double for betslip.BetRepository, recording
// the last Bet/idempotency key it was asked to persist and returning a
// caller-supplied canned result — exactly mirroring the atomicity/replay
// contract the real DynamoDB adapter (Phase 11) implements.
type fakeBetRepository struct {
	placeResult   domainbetslip.Bet
	placeReplayed bool
	placeErr      error

	lastBet domainbetslip.Bet
	lastKey string
}

func (f *fakeBetRepository) Place(_ context.Context, b domainbetslip.Bet, idempotencyKey string) (domainbetslip.Bet, bool, error) {
	f.lastBet = b
	f.lastKey = idempotencyKey
	if f.placeErr != nil {
		return domainbetslip.Bet{}, false, f.placeErr
	}
	return f.placeResult, f.placeReplayed, nil
}

func (f *fakeBetRepository) ListByUser(_ context.Context, _ string, _ int, _ string) ([]domainbetslip.Bet, string, error) {
	return nil, "", nil
}

// fakeIDGenerator returns a fixed id, useful for asserting the exact Bet
// value handed to BetRepository.Place.
type fakeIDGenerator struct{ id string }

func (f fakeIDGenerator) NewID() string { return f.id }

// TestPlace_MapsInsufficientFunds proves a repository-side insufficient-
// funds failure surfaces as the typed domain error, carrying the persisted
// rejected bet's identifiers (spec: bet-slip-placement/Atomic Conditional
// Debit and Bet Persistence, "Insufficient balance").
func TestPlace_MapsInsufficientFunds(t *testing.T) {
	sel := domainevent.SelectionRef{ID: "sel-1", EventID: "evt-1", Odds: mustOdds(t, "1.85")}
	catalog := newFakeCatalog(sel)
	wantErr := domainbetslip.ErrInsufficientFunds{
		BetID:    "bet-1",
		Balance:  mustMoney(t, "50.00"),
		Required: mustMoney(t, "100.00"),
	}
	repo := &fakeBetRepository{placeErr: wantErr}
	uc := appbetslip.NewPlace(catalog, repo, fakeIDGenerator{id: "bet-1"}, defaultBoundsFixture(t))

	_, err := uc.Execute(context.Background(), appbetslip.PlaceCommand{
		UserID:         "user-1",
		SelectionIDs:   []string{"sel-1"},
		Stake:          mustMoney(t, "100.00"),
		IdempotencyKey: "key-1",
	})

	var insufficientErr domainbetslip.ErrInsufficientFunds
	require.ErrorAs(t, err, &insufficientErr)
	require.Equal(t, wantErr, insufficientErr)
}

// TestPlace_ReturnsReplayedBet proves a repository-side idempotent replay
// returns the recorded bet with Replayed=true and no error (spec: bet-slip-
// placement/Idempotent Placement via Idempotency-Key).
func TestPlace_ReturnsReplayedBet(t *testing.T) {
	sel := domainevent.SelectionRef{ID: "sel-1", EventID: "evt-1", Odds: mustOdds(t, "1.85")}
	catalog := newFakeCatalog(sel)
	recordedBet := domainbetslip.Bet{ID: "bet-1", Status: domainbetslip.BetStatusAccepted}
	repo := &fakeBetRepository{placeResult: recordedBet, placeReplayed: true}
	uc := appbetslip.NewPlace(catalog, repo, fakeIDGenerator{id: "would-be-a-new-id"}, defaultBoundsFixture(t))

	result, err := uc.Execute(context.Background(), appbetslip.PlaceCommand{
		UserID:         "user-1",
		SelectionIDs:   []string{"sel-1"},
		Stake:          mustMoney(t, "100.00"),
		IdempotencyKey: "key-1",
	})

	require.NoError(t, err)
	require.True(t, result.Replayed)
	require.Equal(t, recordedBet, result.Bet)
}

// TestPlace_RejectsReusedKeyWithDifferentPayload proves a repository-side
// idempotency-key/payload mismatch surfaces the typed reuse error (spec:
// bet-slip-placement/Idempotent Placement via Idempotency-Key, "Different
// key, same payload" — the mirror scenario: same key, different payload).
func TestPlace_RejectsReusedKeyWithDifferentPayload(t *testing.T) {
	sel := domainevent.SelectionRef{ID: "sel-1", EventID: "evt-1", Odds: mustOdds(t, "1.85")}
	catalog := newFakeCatalog(sel)
	repo := &fakeBetRepository{placeErr: domainbetslip.ErrIdempotencyKeyReuse}
	uc := appbetslip.NewPlace(catalog, repo, fakeIDGenerator{id: "bet-1"}, defaultBoundsFixture(t))

	_, err := uc.Execute(context.Background(), appbetslip.PlaceCommand{
		UserID:         "user-1",
		SelectionIDs:   []string{"sel-1"},
		Stake:          mustMoney(t, "50.00"),
		IdempotencyKey: "key-1",
	})

	require.ErrorIs(t, err, domainbetslip.ErrIdempotencyKeyReuse)
}

// TestPlace_ReplayOfRejectedKeyReturnsRecordedRejection proves replaying a
// key that previously recorded a rejection returns that recorded rejection
// verbatim — never an error, and never a fresh re-evaluation, even if the
// caller's balance has since changed out-of-band (D16). A genuine retry
// with a *different* stake is a different payload and MUST use a new key
// (design's D16 rationale) — it is not this scenario, so the stake here
// stays identical to a plausible original request.
func TestPlace_ReplayOfRejectedKeyReturnsRecordedRejection(t *testing.T) {
	sel := domainevent.SelectionRef{ID: "sel-1", EventID: "evt-1", Odds: mustOdds(t, "1.85")}
	catalog := newFakeCatalog(sel)
	recordedBet := domainbetslip.Bet{
		ID:              "bet-1",
		Status:          domainbetslip.BetStatusRejected,
		RejectionReason: domainbetslip.RejectionReasonInsufficientFunds,
	}
	repo := &fakeBetRepository{placeResult: recordedBet, placeReplayed: true}
	uc := appbetslip.NewPlace(catalog, repo, fakeIDGenerator{id: "would-be-a-new-id"}, defaultBoundsFixture(t))

	result, err := uc.Execute(context.Background(), appbetslip.PlaceCommand{
		UserID:         "user-1",
		SelectionIDs:   []string{"sel-1"},
		Stake:          mustMoney(t, "100.00"), // identical payload to the original rejected attempt
		IdempotencyKey: "key-1",
	})

	require.NoError(t, err)
	require.True(t, result.Replayed)
	require.Equal(t, domainbetslip.BetStatusRejected, result.Bet.Status)
	require.Equal(t, recordedBet, result.Bet)
}
