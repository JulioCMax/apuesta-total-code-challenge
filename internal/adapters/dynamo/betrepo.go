package dynamo

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	domainbetslip "github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/betslip"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

// entityTypeBet is the entityType attribute value stamped on every bet
// item.
const entityTypeBet = "Bet"

// BetStore is the exact subset of *dynamodb.Client that BetRepository
// uses. Depending on the interface rather than the concrete client is what
// makes DynamoDB's own failure modes testable: dynamodb-local serialises
// transactions and can therefore NEVER emit a TransactionConflict
// cancellation, so the only way to prove the retry path is to hand this
// seam a store that returns a crafted TransactionCanceledException.
// *dynamodb.Client satisfies it as-is.
type BetStore interface {
	TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
}

// TransactionRetryPolicy bounds how often a placement transaction cancelled
// purely by contention is retried, and how long to wait between attempts.
// Backoff receives the 1-based attempt number that just failed.
type TransactionRetryPolicy struct {
	MaxAttempts int
	Backoff     func(attempt int) time.Duration
}

// defaultTransactionRetryPolicy retries a contended placement 3 times after
// the first attempt, backing off ~25ms, ~50ms, ~100ms with jitter. The
// budget is deliberately small: a placement is a synchronous request, and
// contention on a single user's profile item resolves in milliseconds or
// not at all.
func defaultTransactionRetryPolicy() TransactionRetryPolicy {
	return TransactionRetryPolicy{
		MaxAttempts: 4,
		Backoff:     TransactionBackoff,
	}
}

// maxTransactionBackoff caps the exponential backoff base TransactionBackoff
// computes, before jitter is applied.
const maxTransactionBackoff = 2 * time.Second

// TransactionBackoff computes the default jittered exponential backoff for
// the given 1-based attempt number: ~25ms, ~50ms, ~100ms, ... up to
// maxTransactionBackoff, plus up to 50% jitter. It is exported so a caller
// building a custom TransactionRetryPolicy (via WithTransactionRetry) can
// reuse this exact, already-hardened shape instead of reimplementing it.
//
// The clamp matters beyond the shipped default (MaxAttempts: 4, which never
// gets close to it): an exported TransactionRetryPolicy with an unusually
// large MaxAttempts would otherwise left-shift 25ms far enough to overflow
// time.Duration's int64 range into a non-positive value, and
// rand.Int63n panics when n <= 0.
func TransactionBackoff(attempt int) time.Duration {
	base := 25 * time.Millisecond << (attempt - 1)
	if base <= 0 || base > maxTransactionBackoff {
		base = maxTransactionBackoff
	}
	return base/2 + time.Duration(rand.Int63n(int64(base/2)+1))
}

// BetRepositoryOption customises a BetRepository at construction.
type BetRepositoryOption func(*BetRepository)

// WithTransactionRetry overrides the default transaction retry policy.
// Tests use it to make the retry path deterministic and instantaneous.
func WithTransactionRetry(policy TransactionRetryPolicy) BetRepositoryOption {
	return func(r *BetRepository) { r.retry = policy }
}

// BetRepository implements application/betslip.BetRepository: it debits
// the balance and stores the bet atomically when funds suffice (D8), or
// persists the same bet with status "rejected" — no balance update — and
// returns ErrInsufficientFunds when they do not (D15). All concurrency
// correctness lives in the single TransactWriteItems call in Place.
type BetRepository struct {
	client         BetStore
	table          string
	idempotencyTTL time.Duration
	retry          TransactionRetryPolicy
}

// NewBetRepository builds a BetRepository backed by client, targeting
// table, with idempotency records expiring after idempotencyTTL
// (IDEMPOTENCY_TTL from configuration; zero disables the expiresAt
// attribute).
func NewBetRepository(client BetStore, table string, idempotencyTTL time.Duration, opts ...BetRepositoryOption) *BetRepository {
	r := &BetRepository{
		client:         client,
		table:          table,
		idempotencyTTL: idempotencyTTL,
		retry:          defaultTransactionRetryPolicy(),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Place implements the PlaceBet Transaction from design.md: attempt 1 is a
// 3-item TransactWriteItems (conditional debit, bet Put, idempotency Put);
// attempt 2 — only reached when the debit's condition failed — persists a
// balance-free rejection. A conflict on the idempotency item always wins
// over every other condition, in either attempt: it means this exact
// request was already resolved, and D16 requires returning that recorded
// outcome untouched rather than re-deriving anything from the current
// balance.
func (r *BetRepository) Place(ctx context.Context, b domainbetslip.Bet, idempotencyKey string) (domainbetslip.Bet, bool, error) {
	pk := UserPK(b.UserID)
	reqHash := requestHash(b)

	accepted := b
	accepted.Status = domainbetslip.BetStatusAccepted
	accepted.RejectionReason = ""

	items := []types.TransactWriteItem{
		{
			Update: &types.Update{
				TableName: aws.String(r.table),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: pk},
					"SK": &types.AttributeValueMemberS{Value: ProfileSK()},
				},
				UpdateExpression:    aws.String("SET balance = balance - :stake"),
				ConditionExpression: aws.String("attribute_exists(PK) AND balance >= :stake"),
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":stake": MarshalMoney(b.Stake),
				},
				ReturnValuesOnConditionCheckFailure: types.ReturnValuesOnConditionCheckFailureAllOld,
			},
		},
		{
			Put: &types.Put{
				TableName:           aws.String(r.table),
				Item:                betItemAttrs(pk, accepted),
				ConditionExpression: aws.String("attribute_not_exists(SK)"),
			},
		},
	}
	if idempotencyKey != "" {
		items = append(items, types.TransactWriteItem{
			Put: &types.Put{
				TableName: aws.String(r.table),
				Item: idempotencyItemAttrs(
					pk, idempotencyKey, accepted.ID, string(domainbetslip.BetStatusAccepted), reqHash, accepted.CreatedAt, r.idempotencyTTL,
				),
				ConditionExpression: aws.String("attribute_not_exists(SK)"),
			},
		})
	}

	err := r.transactWithConflictRetry(ctx, items)
	if err == nil {
		return accepted, false, nil
	}

	canceled, isCanceled := AsTransactionCanceled(err)
	if !isCanceled {
		return domainbetslip.Bet{}, false, fmt.Errorf("dynamo: place: %w", err)
	}
	reasons := canceled.CancellationReasons

	// An idempotency conflict always wins: it means a concurrent or
	// earlier call already resolved this exact key, regardless of what the
	// balance condition (idx 0) reported in this attempt.
	if idempotencyKey != "" && ConditionalCheckFailed(reasons, 2) {
		return r.resolveReplay(ctx, pk, idempotencyKey, reqHash)
	}

	if ConditionalCheckFailed(reasons, 0) {
		balance, balErr := balanceFromCancellationReason(reasons, 0)
		if balErr != nil {
			return domainbetslip.Bet{}, false, fmt.Errorf("dynamo: place: read balance after conditional failure: %w", balErr)
		}
		return r.persistRejection(ctx, pk, b, idempotencyKey, reqHash, balance)
	}

	// idx 1 (bet Put) failing means a ULID collision — transient and never
	// expected in practice (design.md).
	return domainbetslip.Bet{}, false, fmt.Errorf("dynamo: place: unexpected transaction cancellation: %+v", reasons)
}

// transactWithConflictRetry runs one TransactWriteItems call, retrying
// only when DynamoDB cancelled it purely because a concurrent transaction
// held one of the same items ("TransactionConflict"). That is exactly what
// two simultaneous placements by the same user produce: both contend on
// the single USER#<id>/PROFILE balance item.
//
// aws-sdk-go-v2's standard retryer does NOT retry
// TransactionCanceledException, so without this loop a contended placement
// surfaced as an unclassified 500 with NO bet persisted — neither accepted
// nor rejected — which is precisely the outcome the concurrency
// requirement forbids.
//
// A cancellation that carries ANY ConditionalCheckFailed reason is never
// retried: that is a verdict about the request (insufficient balance, an
// already-used idempotency key) and the caller must be answered, not made
// to wait. Beyond TransactionConflict, ThrottlingError,
// ProvisionedThroughputExceeded and TransactionInProgress are retried the
// same way — none of them says anything about the request either — and so
// is an entirely empty CancellationReasons slice, an anomalous shape
// DynamoDB's own contract should never produce (finding R2). Every other
// error is returned untouched for the caller's own branching.
func (r *BetRepository) transactWithConflictRetry(ctx context.Context, items []types.TransactWriteItem) error {
	maxAttempts := r.retry.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	for attempt := 1; ; attempt++ {
		_, err := r.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: items})
		if err == nil {
			return nil
		}

		canceled, isCanceled := AsTransactionCanceled(err)
		if !isCanceled {
			return err
		}
		reasons := canceled.CancellationReasons
		transient := len(reasons) == 0 || AnyTransientCancellation(reasons)
		if !transient || AnyConditionalCheckFailed(reasons) {
			return err
		}

		if attempt >= maxAttempts {
			// err is embedded with %v, deliberately NOT %w: Place() calls
			// AsTransactionCanceled on whatever transactWithConflictRetry
			// returns, and wrapping the raw *TransactionCanceledException
			// back into the chain here would make that check see this
			// EXHAUSTED, already-classified error as "still cancelled,
			// please classify it again", discarding ErrConcurrencyConflict
			// and re-falling into the untyped "unexpected transaction
			// cancellation" branch. %v still renders err's own message (and
			// cancellationCodes(reasons) renders its reason codes
			// explicitly) into this error's text for logs — previously
			// dropped entirely — without reopening it to further typed
			// unwrapping (finding R2).
			return fmt.Errorf("dynamo: transaction still contended after %d attempts (reason codes=%v): %w (underlying: %v)",
				attempt, cancellationCodes(reasons), domainbetslip.ErrConcurrencyConflict, err)
		}
		if waitErr := sleepCtx(ctx, r.backoff(attempt)); waitErr != nil {
			return waitErr
		}
	}
}

// cancellationCodes renders a transaction cancellation's reason codes for
// diagnostics (e.g. the exhausted-retry error above): each entry is the
// item's Code, or "<nil>" when DynamoDB omitted it — should not happen per
// its own contract, but defensive since this is only for logs.
func cancellationCodes(reasons []types.CancellationReason) []string {
	codes := make([]string, len(reasons))
	for i, reason := range reasons {
		if reason.Code == nil {
			codes[i] = "<nil>"
			continue
		}
		codes[i] = *reason.Code
	}
	return codes
}

// backoff returns the configured wait before the next attempt, tolerating
// a policy that supplies no Backoff function at all.
func (r *BetRepository) backoff(attempt int) time.Duration {
	if r.retry.Backoff == nil {
		return 0
	}
	return r.retry.Backoff(attempt)
}

// sleepCtx waits for d, or aborts early with ctx's error when the caller
// gives up first — a retry loop must never outlive its request.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// persistRejection is attempt 2: a balance-free TransactWriteItems that
// records the rejected bet. Structurally incapable of moving money, since
// no Update item is ever included (D15).
func (r *BetRepository) persistRejection(
	ctx context.Context, pk string, b domainbetslip.Bet, idempotencyKey, reqHash string, balance money.Money,
) (domainbetslip.Bet, bool, error) {
	rejected := b
	rejected.Status = domainbetslip.BetStatusRejected
	rejected.RejectionReason = domainbetslip.RejectionReasonInsufficientFunds

	items := []types.TransactWriteItem{
		{
			Put: &types.Put{
				TableName:           aws.String(r.table),
				Item:                betItemAttrs(pk, rejected),
				ConditionExpression: aws.String("attribute_not_exists(SK)"),
			},
		},
	}
	if idempotencyKey != "" {
		items = append(items, types.TransactWriteItem{
			Put: &types.Put{
				TableName: aws.String(r.table),
				Item: idempotencyItemAttrs(
					pk, idempotencyKey, rejected.ID, string(domainbetslip.BetStatusRejected), reqHash, rejected.CreatedAt, r.idempotencyTTL,
				),
				ConditionExpression: aws.String("attribute_not_exists(SK)"),
			},
		})
	}

	err := r.transactWithConflictRetry(ctx, items)
	if err == nil {
		return domainbetslip.Bet{}, false, domainbetslip.ErrInsufficientFunds{
			BetID:    rejected.ID,
			Balance:  balance,
			Required: b.Stake,
		}
	}

	// If attempt 2 itself is cancelled at its idempotency Put, another
	// concurrent request already recorded this same key: resolve and
	// return that record instead (design.md's PlaceBet Transaction table).
	if canceled, isCanceled := AsTransactionCanceled(err); isCanceled && idempotencyKey != "" && ConditionalCheckFailed(canceled.CancellationReasons, 1) {
		return r.resolveReplay(ctx, pk, idempotencyKey, reqHash)
	}

	return domainbetslip.Bet{}, false, fmt.Errorf("dynamo: persist rejection: %w", err)
}

// resolveReplay reads back the idempotency record for idempotencyKey and,
// when its stored requestHash matches reqHash, returns the recorded bet
// exactly as stored — accepted or rejected — with replayed=true and no
// error (D16: never re-evaluate). A hash mismatch means the same key was
// reused with a different payload.
func (r *BetRepository) resolveReplay(ctx context.Context, pk, idempotencyKey, reqHash string) (domainbetslip.Bet, bool, error) {
	idemOut, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: IdempotencySK(idempotencyKey)},
		},
	})
	if err != nil {
		return domainbetslip.Bet{}, false, fmt.Errorf("dynamo: resolve replay: get idempotency record: %w", err)
	}
	if idemOut.Item == nil {
		return domainbetslip.Bet{}, false, fmt.Errorf("dynamo: resolve replay: idempotency record %q not found after conflict", idempotencyKey)
	}

	recordedBetID, recordedHash, ok := idempotencyRecordFromItem(idemOut.Item)
	if !ok {
		return domainbetslip.Bet{}, false, fmt.Errorf("dynamo: resolve replay: malformed idempotency record")
	}
	if recordedHash != reqHash {
		return domainbetslip.Bet{}, false, domainbetslip.ErrIdempotencyKeyReuse
	}

	betOut, err := r.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.table),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: BetSK(recordedBetID)},
		},
	})
	if err != nil {
		return domainbetslip.Bet{}, false, fmt.Errorf("dynamo: resolve replay: get recorded bet %q: %w", recordedBetID, err)
	}
	if betOut.Item == nil {
		return domainbetslip.Bet{}, false, fmt.Errorf("dynamo: resolve replay: recorded bet %q not found", recordedBetID)
	}

	bet, err := betFromItem(betOut.Item)
	if err != nil {
		return domainbetslip.Bet{}, false, fmt.Errorf("dynamo: resolve replay: decode bet: %w", err)
	}
	return bet, true, nil
}

// ListByUser queries a user's bet history newest-first (ULID SK sorts
// chronologically, so ScanIndexForward=false needs no extra sort logic),
// paginating via an opaque base64 cursor derived from DynamoDB's own
// LastEvaluatedKey. limit<=0 means "no explicit page size" (DynamoDB
// still caps a single Query at 1MB).
func (r *BetRepository) ListByUser(ctx context.Context, userID string, limit int, cursor string) ([]domainbetslip.Bet, string, error) {
	input := &dynamodb.QueryInput{
		TableName:              aws.String(r.table),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :prefix)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":     &types.AttributeValueMemberS{Value: UserPK(userID)},
			":prefix": &types.AttributeValueMemberS{Value: betSKPrefix},
		},
		ScanIndexForward: aws.Bool(false),
	}
	if limit > 0 {
		input.Limit = aws.Int32(int32(limit))
	}
	if cursor != "" {
		key, err := decodeCursor(cursor, UserPK(userID))
		if err != nil {
			// A malformed cursor (bad base64, or valid base64 that does not
			// decode to the expected JSON shape) is a caller input error,
			// not an infrastructure failure: wrap it in the typed
			// ErrInvalidCursor so the HTTP layer answers 400
			// VALIDATION_ERROR instead of an unclassified 500 (finding
			// W-cursor).
			return nil, "", fmt.Errorf("dynamo: list by user: %w: %v", domainbetslip.ErrInvalidCursor, err)
		}
		input.ExclusiveStartKey = key
	}

	out, err := r.client.Query(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("dynamo: list by user: %w", err)
	}

	bets := make([]domainbetslip.Bet, 0, len(out.Items))
	for _, item := range out.Items {
		bet, err := betFromItem(item)
		if err != nil {
			return nil, "", fmt.Errorf("dynamo: list by user: decode bet: %w", err)
		}
		bets = append(bets, bet)
	}

	nextCursor := ""
	if len(out.LastEvaluatedKey) > 0 {
		nextCursor, err = encodeCursor(out.LastEvaluatedKey)
		if err != nil {
			return nil, "", fmt.Errorf("dynamo: list by user: encode cursor: %w", err)
		}
	}

	return bets, nextCursor, nil
}

// requestHash returns the SHA-256 hex digest of b's canonical placement
// payload — every field that identifies "what the caller asked for", never
// the fields generated per attempt (ID, CreatedAt) or the outcome
// (Status). Two Place calls with the same idempotencyKey and an equal
// requestHash are the same request (D16); a different hash under the same
// key is ErrIdempotencyKeyReuse.
func requestHash(b domainbetslip.Bet) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%s|%s", b.UserID, string(b.Type), b.Stake.String(), strings.Join(b.Selections, ","))
	return hex.EncodeToString(h.Sum(nil))
}

// balanceFromCancellationReason reads the pre-transaction balance from the
// ALL_OLD item image DynamoDB returns for a failed conditional Update
// (ReturnValuesOnConditionCheckFailure).
func balanceFromCancellationReason(reasons []types.CancellationReason, idx int) (money.Money, error) {
	if idx < 0 || idx >= len(reasons) || reasons[idx].Item == nil {
		return money.Money{}, fmt.Errorf("dynamo: cancellation reason %d carries no item image", idx)
	}
	balAV, ok := reasons[idx].Item["balance"]
	if !ok {
		return money.Money{}, fmt.Errorf("dynamo: cancellation reason %d item missing balance", idx)
	}
	return UnmarshalMoney(balAV)
}

// betItemAttrs builds the bet item (PK=USER#<id>, SK=BET#<ulid>) per
// design.md's DynamoDB Single-Table Design. Selections is stored as a List
// of S (selection IDs) rather than the design table's "L of M" shorthand:
// domain/betslip.Bet.Selections is a []string, and this adapter persists
// exactly the domain aggregate's own shape, never inventing extra fields.
func betItemAttrs(pk string, b domainbetslip.Bet) map[string]types.AttributeValue {
	item := map[string]types.AttributeValue{
		"PK":               &types.AttributeValueMemberS{Value: pk},
		"SK":               &types.AttributeValueMemberS{Value: BetSK(b.ID)},
		"betId":            &types.AttributeValueMemberS{Value: b.ID},
		"type":             &types.AttributeValueMemberS{Value: string(b.Type)},
		"stake":            MarshalMoney(b.Stake),
		"combinedOdds":     MarshalOdds(b.CombinedOdds),
		"potentialReturns": MarshalMoney(b.PotentialReturns),
		"status":           &types.AttributeValueMemberS{Value: string(b.Status)},
		"selections":       stringListAttr(b.Selections),
		"createdAt":        &types.AttributeValueMemberS{Value: b.CreatedAt.Format(time.RFC3339Nano)},
		"entityType":       &types.AttributeValueMemberS{Value: entityTypeBet},
	}
	if b.RejectionReason != "" {
		item["rejectionReason"] = &types.AttributeValueMemberS{Value: b.RejectionReason}
	}
	return item
}

// betFromItem decodes a bet item back into the domain aggregate. UserID is
// derived from PK (the item stores no separate userId attribute, unlike
// the user profile item — see design.md's Bet row).
func betFromItem(item map[string]types.AttributeValue) (domainbetslip.Bet, error) {
	betID, ok := attrString(item, "betId")
	if !ok {
		return domainbetslip.Bet{}, fmt.Errorf("dynamo: bet item missing betId")
	}
	pk, _ := attrString(item, "PK")
	userID := strings.TrimPrefix(pk, "USER#")

	typeRaw, _ := attrString(item, "type")
	statusRaw, _ := attrString(item, "status")
	rejectionReason, _ := attrString(item, "rejectionReason")
	createdAtRaw, _ := attrString(item, "createdAt")

	stake, err := UnmarshalMoney(item["stake"])
	if err != nil {
		return domainbetslip.Bet{}, fmt.Errorf("dynamo: bet item %q: %w", betID, err)
	}
	combinedOdds, err := UnmarshalOdds(item["combinedOdds"])
	if err != nil {
		return domainbetslip.Bet{}, fmt.Errorf("dynamo: bet item %q: %w", betID, err)
	}
	potentialReturns, err := UnmarshalMoney(item["potentialReturns"])
	if err != nil {
		return domainbetslip.Bet{}, fmt.Errorf("dynamo: bet item %q: %w", betID, err)
	}
	createdAt, err := time.Parse(time.RFC3339Nano, createdAtRaw)
	if err != nil {
		return domainbetslip.Bet{}, fmt.Errorf("dynamo: bet item %q: invalid createdAt %q: %w", betID, createdAtRaw, err)
	}

	return domainbetslip.Bet{
		ID:               betID,
		UserID:           userID,
		Type:             domainbetslip.BetType(typeRaw),
		Stake:            stake,
		CombinedOdds:     combinedOdds,
		PotentialReturns: potentialReturns,
		Status:           domainbetslip.BetStatus(statusRaw),
		RejectionReason:  rejectionReason,
		Selections:       attrStringList(item, "selections"),
		CreatedAt:        createdAt,
	}, nil
}

// idempotencyItemAttrs builds the idempotency item (PK=USER#<id>,
// SK=IDEMP#<key>). expiresAt (TTL) is only set when ttl > 0.
func idempotencyItemAttrs(pk, key, betID, outcome, requestHash string, createdAt time.Time, ttl time.Duration) map[string]types.AttributeValue {
	item := map[string]types.AttributeValue{
		"PK":          &types.AttributeValueMemberS{Value: pk},
		"SK":          &types.AttributeValueMemberS{Value: IdempotencySK(key)},
		"betId":       &types.AttributeValueMemberS{Value: betID},
		"outcome":     &types.AttributeValueMemberS{Value: outcome},
		"requestHash": &types.AttributeValueMemberS{Value: requestHash},
		"createdAt":   &types.AttributeValueMemberS{Value: createdAt.Format(time.RFC3339Nano)},
	}
	if ttl > 0 {
		item["expiresAt"] = &types.AttributeValueMemberN{Value: strconv.FormatInt(createdAt.Add(ttl).Unix(), 10)}
	}
	return item
}

// idempotencyRecordFromItem extracts the two fields resolveReplay needs
// from an idempotency item.
func idempotencyRecordFromItem(item map[string]types.AttributeValue) (betID, requestHash string, ok bool) {
	betID, ok1 := attrString(item, "betId")
	requestHash, ok2 := attrString(item, "requestHash")
	return betID, requestHash, ok1 && ok2
}

// attrStringList extracts a List-of-S attribute as a []string, skipping
// any element that is not itself an S (defensive; every writer in this
// package only ever writes S elements).
func attrStringList(item map[string]types.AttributeValue, key string) []string {
	av, ok := item[key]
	if !ok {
		return nil
	}
	l, ok := av.(*types.AttributeValueMemberL)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(l.Value))
	for _, v := range l.Value {
		if s, ok := v.(*types.AttributeValueMemberS); ok {
			out = append(out, s.Value)
		}
	}
	return out
}

func stringListAttr(values []string) types.AttributeValue {
	l := make([]types.AttributeValue, 0, len(values))
	for _, v := range values {
		l = append(l, &types.AttributeValueMemberS{Value: v})
	}
	return &types.AttributeValueMemberL{Value: l}
}

// cursorKey is the minimal JSON shape base64-encoded into a ListByUser
// pagination cursor: exactly the two attributes this table's primary key
// needs to resume a Query via ExclusiveStartKey.
type cursorKey struct {
	PK string `json:"pk"`
	SK string `json:"sk"`
}

func encodeCursor(key map[string]types.AttributeValue) (string, error) {
	pk, _ := attrString(key, "PK")
	sk, _ := attrString(key, "SK")
	raw, err := json.Marshal(cursorKey{PK: pk, SK: sk})
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(raw), nil
}

// decodeCursor decodes cursor back into a DynamoDB key, then validates it
// against expectedPK — the calling user's own partition (UserPK(userID)) —
// before ever handing it to Query as ExclusiveStartKey. A cursor that is
// valid base64 AND valid JSON but decodes to an empty key ({} or null) or
// to a DIFFERENT partition is rejected here rather than reaching DynamoDB:
// letting it through would surface a ValidationException as an unclassified
// 500 instead of 400, and would let a caller probe another user's
// partition by watching how the response differs (finding R1).
func decodeCursor(cursor, expectedPK string) (map[string]types.AttributeValue, error) {
	raw, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, err
	}
	var ck cursorKey
	if err := json.Unmarshal(raw, &ck); err != nil {
		return nil, err
	}
	if ck.PK == "" || ck.SK == "" {
		return nil, fmt.Errorf("dynamo: cursor decodes to an empty key")
	}
	if ck.PK != expectedPK {
		return nil, fmt.Errorf("dynamo: cursor names a different partition")
	}
	return map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: ck.PK},
		"SK": &types.AttributeValueMemberS{Value: ck.SK},
	}, nil
}
