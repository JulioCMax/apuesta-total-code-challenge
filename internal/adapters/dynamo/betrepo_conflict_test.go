package dynamo_test

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/dynamo"
	domainbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/betslip"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

// NOTE ON WHAT THESE TESTS PROVE AND WHY THEY EXIST.
//
// Real DynamoDB cancels a TransactWriteItems call with the reason code
// "TransactionConflict" when another transaction is concurrently operating
// on one of the same items — for this service, the single
// USER#<id>/PROFILE balance item that every placement of the same user
// contends on. dynamodb-local serialises transactions internally and
// therefore NEVER produces that reason code, so no integration test in
// this package (betrepo_integration_test.go included) can exercise this
// path. These tests drive the adapter through the BetStore seam with a
// store that returns exactly the cancellation AWS would return.
//
// They prove the adapter's REACTION to that cancellation. They do not, and
// cannot, prove that real DynamoDB emits it.

// stubBetStore is a scripted BetStore: each TransactWriteItems call
// consumes the next entry of transactErrs (nil = the transaction
// committed), and calls beyond that slice succeed.
type stubBetStore struct {
	mu            sync.Mutex
	transactErrs  []error
	transactCalls int
	queryCalls    int
	items         map[string]map[string]types.AttributeValue
}

func (s *stubBetStore) TransactWriteItems(_ context.Context, _ *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := s.transactCalls
	s.transactCalls++
	if idx < len(s.transactErrs) && s.transactErrs[idx] != nil {
		return nil, s.transactErrs[idx]
	}
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

func (s *stubBetStore) GetItem(_ context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sk, _ := params.Key["SK"].(*types.AttributeValueMemberS)
	if sk == nil {
		return &dynamodb.GetItemOutput{}, nil
	}
	return &dynamodb.GetItemOutput{Item: s.items[sk.Value]}, nil
}

func (s *stubBetStore) Query(_ context.Context, _ *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.queryCalls++
	return &dynamodb.QueryOutput{}, nil
}

func (s *stubBetStore) queryCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queryCalls
}

func (s *stubBetStore) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.transactCalls
}

// transactionConflict builds the exact shape real DynamoDB returns when a
// concurrent transaction touches one of the same items: the SDK's
// operation error wrapping a TransactionCanceledException whose
// cancellation reason at the contended item's index is
// "TransactionConflict" (never "ConditionalCheckFailed" — nothing about
// the request itself was wrong).
func transactionConflict() error {
	return &smithy.OperationError{
		ServiceID:     "DynamoDB",
		OperationName: "TransactWriteItems",
		Err: &types.TransactionCanceledException{
			CancellationReasons: []types.CancellationReason{
				{Code: codePtr("TransactionConflict")},
				{Code: codePtr("None")},
				{Code: codePtr("None")},
			},
		},
	}
}

// cancellationWithCode builds a TransactionCanceledException whose
// contended item (idx 0) carries the given CancellationReason.Code — used
// to prove the adapter treats every transient capacity/contention code the
// same way it treats "TransactionConflict" (finding R2), not just that one
// literal code.
func cancellationWithCode(code string) error {
	return &smithy.OperationError{
		ServiceID:     "DynamoDB",
		OperationName: "TransactWriteItems",
		Err: &types.TransactionCanceledException{
			CancellationReasons: []types.CancellationReason{
				{Code: codePtr(code)},
				{Code: codePtr("None")},
				{Code: codePtr("None")},
			},
		},
	}
}

// emptyCancellation builds a TransactionCanceledException with a nil/empty
// CancellationReasons slice — an anomalous shape DynamoDB's own contract
// should never produce (every item gets a reason code), but one the
// adapter must still treat as transient rather than an unclassified
// failure (finding R2).
func emptyCancellation() error {
	return &smithy.OperationError{
		ServiceID:     "DynamoDB",
		OperationName: "TransactWriteItems",
		Err:           &types.TransactionCanceledException{CancellationReasons: nil},
	}
}

// instantRetry makes the retry path deterministic and instantaneous so
// these tests never sleep and never flake.
func instantRetry(maxAttempts int) dynamo.BetRepositoryOption {
	return dynamo.WithTransactionRetry(dynamo.TransactionRetryPolicy{
		MaxAttempts: maxAttempts,
		Backoff:     func(int) time.Duration { return 0 },
	})
}

func conflictTestBet(t *testing.T) domainbetslip.Bet {
	t.Helper()
	stake, err := money.NewMoneyFromFloat(100)
	require.NoError(t, err)
	odds, err := money.NewOddsFromFloat(1.85)
	require.NoError(t, err)
	returns, err := money.NewMoneyFromFloat(185)
	require.NoError(t, err)

	return domainbetslip.Bet{
		ID:               "01TESTBET000000000000000",
		UserID:           "user-1",
		Type:             domainbetslip.BetTypeSingle,
		Stake:            stake,
		CombinedOdds:     odds,
		PotentialReturns: returns,
		Status:           domainbetslip.BetStatusAccepted,
		Selections:       []string{"sel-1"},
		CreatedAt:        time.Now().UTC(),
	}
}

// TestBetRepository_Place_RetriesTransactionConflictAndSucceeds proves a
// placement cancelled by pure contention is retried rather than surfaced
// as a failure. aws-sdk-go-v2's standard retryer does NOT retry
// TransactionCanceledException, so without an application-level retry the
// caller gets a 500 with no bet persisted at all — neither accepted nor
// rejected — which breaks the "every attempt is persisted" rule.
func TestBetRepository_Place_RetriesTransactionConflictAndSucceeds(t *testing.T) {
	store := &stubBetStore{transactErrs: []error{transactionConflict(), transactionConflict(), nil}}
	repo := dynamo.NewBetRepository(store, "test-table", time.Hour, instantRetry(4))

	stored, replayed, err := repo.Place(context.Background(), conflictTestBet(t), "idem-key-1")

	require.NoError(t, err, "a transaction cancelled only by contention must be retried, not surfaced")
	require.False(t, replayed)
	require.Equal(t, domainbetslip.BetStatusAccepted, stored.Status)
	require.Equal(t, 3, store.calls(), "the placement must be retried until it commits")
}

// TestBetRepository_Place_ExhaustedTransactionConflictIsTypedNotUnclassified
// proves that when contention outlives the retry budget the adapter
// returns a TYPED error the HTTP layer can classify as a retryable 503,
// instead of a bare fmt.Errorf that falls through to an unclassified 500.
func TestBetRepository_Place_ExhaustedTransactionConflictIsTypedNotUnclassified(t *testing.T) {
	store := &stubBetStore{transactErrs: []error{
		transactionConflict(), transactionConflict(), transactionConflict(), transactionConflict(),
	}}
	repo := dynamo.NewBetRepository(store, "test-table", time.Hour, instantRetry(4))

	_, _, err := repo.Place(context.Background(), conflictTestBet(t), "idem-key-1")

	require.Error(t, err)
	require.ErrorIs(t, err, domainbetslip.ErrConcurrencyConflict,
		"an exhausted contention retry must be a typed conflict, never an unclassified internal error")
	require.Equal(t, 4, store.calls(), "the retry budget must be bounded")
}

// TestBetRepository_Place_RetriesEveryTransientCancellationReasonCode
// proves ThrottlingError, ProvisionedThroughputExceeded and
// TransactionInProgress are retried exactly like TransactionConflict,
// instead of falling through to an unclassified 500 (finding R2). NOTE:
// dynamodb-local cannot produce ANY of these reason codes (it serialises
// transactions internally), so this — like the TransactionConflict tests
// above — drives the adapter through the BetStore seam with a crafted
// TransactionCanceledException; it proves the adapter's reaction, not that
// real DynamoDB emits these codes.
func TestBetRepository_Place_RetriesEveryTransientCancellationReasonCode(t *testing.T) {
	codes := []string{"TransactionConflict", "ThrottlingError", "ProvisionedThroughputExceeded", "TransactionInProgress"}
	for _, code := range codes {
		t.Run(code, func(t *testing.T) {
			store := &stubBetStore{transactErrs: []error{cancellationWithCode(code), nil}}
			repo := dynamo.NewBetRepository(store, "test-table", time.Hour, instantRetry(4))

			stored, replayed, err := repo.Place(context.Background(), conflictTestBet(t), "idem-key-1")

			require.NoError(t, err, "a %s cancellation must be retried like TransactionConflict, not surfaced as an unclassified error", code)
			require.False(t, replayed)
			require.Equal(t, domainbetslip.BetStatusAccepted, stored.Status)
			require.Equal(t, 2, store.calls())
		})
	}
}

// TestBetRepository_Place_RetriesEmptyCancellationReasons proves an
// entirely empty CancellationReasons slice — an anomalous shape, not a
// verdict about the request — is treated as transient and retried, instead
// of falling through to an unclassified 500 (finding R2).
func TestBetRepository_Place_RetriesEmptyCancellationReasons(t *testing.T) {
	store := &stubBetStore{transactErrs: []error{emptyCancellation(), nil}}
	repo := dynamo.NewBetRepository(store, "test-table", time.Hour, instantRetry(4))

	stored, replayed, err := repo.Place(context.Background(), conflictTestBet(t), "idem-key-1")

	require.NoError(t, err, "an empty CancellationReasons slice must be retried, not surfaced as an unclassified error")
	require.False(t, replayed)
	require.Equal(t, domainbetslip.BetStatusAccepted, stored.Status)
	require.Equal(t, 2, store.calls())
}

// TestBetRepository_Place_ExhaustedTransientCancellationPreservesReasonCodes
// proves that exhausting the retry budget against a transient reason OTHER
// than TransactionConflict still returns the same typed
// ErrConcurrencyConflict (never an unclassified 500), and that the
// underlying AWS error's reason codes survive into the returned error's
// message for logs — previously dropped entirely on exhaustion (finding
// R2).
func TestBetRepository_Place_ExhaustedTransientCancellationPreservesReasonCodes(t *testing.T) {
	store := &stubBetStore{transactErrs: []error{
		cancellationWithCode("ThrottlingError"), cancellationWithCode("ThrottlingError"),
		cancellationWithCode("ThrottlingError"), cancellationWithCode("ThrottlingError"),
	}}
	repo := dynamo.NewBetRepository(store, "test-table", time.Hour, instantRetry(4))

	_, _, err := repo.Place(context.Background(), conflictTestBet(t), "idem-key-1")

	require.Error(t, err)
	require.ErrorIs(t, err, domainbetslip.ErrConcurrencyConflict,
		"an exhausted transient-cancellation retry must be a typed conflict, never an unclassified internal error")
	require.Contains(t, err.Error(), "ThrottlingError",
		"the exhausted retry error must preserve the underlying reason codes for logs")
	require.Equal(t, 4, store.calls(), "the retry budget must be bounded")
}

// TestBetRepository_Place_DoesNotRetryConditionalCheckFailure is the
// triangulation case: a ConditionalCheckFailed cancellation means the
// balance was genuinely insufficient. Retrying it would be pointless and
// would delay the rejection, so it must take the persist-rejection path on
// the first attempt.
func TestBetRepository_Place_DoesNotRetryConditionalCheckFailure(t *testing.T) {
	balanceTooLow := &smithy.OperationError{
		ServiceID:     "DynamoDB",
		OperationName: "TransactWriteItems",
		Err: &types.TransactionCanceledException{
			CancellationReasons: []types.CancellationReason{
				{
					Code: codePtr("ConditionalCheckFailed"),
					Item: map[string]types.AttributeValue{"balance": &types.AttributeValueMemberN{Value: "10.00"}},
				},
				{Code: codePtr("None")},
				{Code: codePtr("None")},
			},
		},
	}
	store := &stubBetStore{transactErrs: []error{balanceTooLow, nil}}
	repo := dynamo.NewBetRepository(store, "test-table", time.Hour, instantRetry(4))

	_, _, err := repo.Place(context.Background(), conflictTestBet(t), "idem-key-1")

	var insufficient domainbetslip.ErrInsufficientFunds
	require.ErrorAs(t, err, &insufficient)
	require.False(t, errors.Is(err, domainbetslip.ErrConcurrencyConflict))
	require.Equal(t, 2, store.calls(), "attempt 1 debits, attempt 2 persists the rejection — no retry in between")
}

// TestTransactionBackoff_ClampsExtremeAttemptsWithoutOverflowOrPanic proves
// an exported TransactionRetryPolicy with an unusually large MaxAttempts
// can never left-shift TransactionBackoff's base past time.Duration's int64
// range into a non-positive value and panic rand.Int63n (which requires
// n > 0) — a hardening item flagged alongside R1-R4.
func TestTransactionBackoff_ClampsExtremeAttemptsWithoutOverflowOrPanic(t *testing.T) {
	for _, attempt := range []int{1, 4, 40, 1000} {
		attempt := attempt
		t.Run(fmt.Sprintf("attempt=%d", attempt), func(t *testing.T) {
			var d time.Duration
			require.NotPanics(t, func() { d = dynamo.TransactionBackoff(attempt) })
			require.Greater(t, d, time.Duration(0), "backoff must always be positive, never zero or negative")
			require.LessOrEqual(t, d, 2*time.Second, "an overflowed base must be clamped, not left unbounded")
		})
	}
}

// TestBetRepository_ListByUser_MalformedCursorIsTypedValidationError proves
// a malformed pagination cursor (e.g. `?cursor=%%%`, not valid base64) is
// reported as a typed domainbetslip.ErrInvalidCursor the HTTP layer maps to
// 400 VALIDATION_ERROR, instead of an unclassified error that falls through
// to 500 (finding W-cursor).
func TestBetRepository_ListByUser_MalformedCursorIsTypedValidationError(t *testing.T) {
	store := &stubBetStore{}
	repo := dynamo.NewBetRepository(store, "test-table", time.Hour)

	_, _, err := repo.ListByUser(context.Background(), "user-1", 0, "%%%")

	require.Error(t, err)
	require.ErrorIs(t, err, domainbetslip.ErrInvalidCursor,
		"a malformed cursor must be a typed validation error, never an unclassified internal error")
}

// TestBetRepository_ListByUser_CursorValidationRejectsEmptyOrForeignKeys
// proves a cursor that is valid base64 AND valid JSON, but decodes to an
// empty key ({} or null) or to another user's partition, is rejected as
// domainbetslip.ErrInvalidCursor before it ever reaches DynamoDB (finding
// R1). Without this check such a cursor would reach Query as
// ExclusiveStartKey, DynamoDB would answer with a ValidationException, and
// that untyped error would surface as an unclassified 500 instead of 400 —
// and a foreign PK would let a caller probe another user's partition by
// watching how the response differs (500 vs a normal page), a small
// information-probe vector this same check closes.
func TestBetRepository_ListByUser_CursorValidationRejectsEmptyOrForeignKeys(t *testing.T) {
	store := &stubBetStore{}
	repo := dynamo.NewBetRepository(store, "test-table", time.Hour)

	tests := []struct {
		name   string
		cursor string
	}{
		{"empty JSON object", base64.URLEncoding.EncodeToString([]byte(`{}`))},
		{"JSON null", base64.URLEncoding.EncodeToString([]byte(`null`))},
		{"foreign user's partition", base64.URLEncoding.EncodeToString([]byte(`{"pk":"USER#other-user","sk":"BET#01ARBITRARYULID0000000000"}`))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := repo.ListByUser(context.Background(), "user-1", 0, tt.cursor)

			require.Error(t, err)
			require.ErrorIs(t, err, domainbetslip.ErrInvalidCursor,
				"a cursor decoding to an empty or foreign-partition key must never reach DynamoDB")
			require.Zero(t, store.queryCallCount(), "an invalid cursor must be rejected before any DynamoDB Query call")
		})
	}
}
