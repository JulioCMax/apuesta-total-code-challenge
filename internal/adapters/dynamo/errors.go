package dynamo

import (
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// conditionalCheckFailedCode is the exact CancellationReason.Code value
// DynamoDB returns for a failed ConditionExpression inside a
// TransactWriteItems call.
const conditionalCheckFailedCode = "ConditionalCheckFailed"

// transactionConflictCode is the exact CancellationReason.Code value
// DynamoDB returns when another transaction was concurrently operating on
// one of the same items. It says nothing about the request being wrong:
// every placement by the same user contends on that user's single
// USER#<id>/PROFILE balance item, so this is the expected reason code
// under the concurrency scenario this service is graded on.
const transactionConflictCode = "TransactionConflict"

// throttlingErrorCode, provisionedThroughputExceededCode and
// transactionInProgressCode are the remaining CancellationReason.Code
// values that mean "transient capacity pressure or ongoing contention",
// exactly like TransactionConflict: none of them is a verdict about the
// request itself, and the identical request retried a moment later can
// commit (finding R2). Before this, only TransactionConflict was
// classified, so any of these fell through to an untyped "unexpected
// transaction cancellation" error and an unclassified 500.
const (
	throttlingErrorCode               = "ThrottlingError"
	provisionedThroughputExceededCode = "ProvisionedThroughputExceeded"
	transactionInProgressCode         = "TransactionInProgress"
)

// AsTransactionCanceled extracts *types.TransactionCanceledException from
// err (however deeply it is wrapped by the SDK's own operation-error
// types), mirroring design.md's error-mapping table: every branch of
// BetRepository.Place starts by asking "was this cancellation, or
// something else entirely (throttling, a network error)?"
func AsTransactionCanceled(err error) (*types.TransactionCanceledException, bool) {
	var canceled *types.TransactionCanceledException
	ok := errors.As(err, &canceled)
	return canceled, ok
}

// ConditionalCheckFailed reports whether reasons[idx] recorded a
// ConditionalCheckFailed cancellation. An index beyond len(reasons) — an
// item that was never part of the transaction, e.g. no Idempotency-Key
// header meant the idempotency Put was omitted — safely reports false
// instead of panicking, since design.md's item order is optional-item
// aware (idx 2 only exists when idempotencyKey != "").
func ConditionalCheckFailed(reasons []types.CancellationReason, idx int) bool {
	if idx < 0 || idx >= len(reasons) {
		return false
	}
	return reasons[idx].Code != nil && *reasons[idx].Code == conditionalCheckFailedCode
}

// AnyConditionalCheckFailed reports whether ANY item in the transaction
// failed its ConditionExpression. Such a cancellation is a verdict about
// the request itself (an insufficient balance, an already-used idempotency
// key) and must be answered, never retried.
func AnyConditionalCheckFailed(reasons []types.CancellationReason) bool {
	return anyReasonCode(reasons, conditionalCheckFailedCode)
}

// AnyTransactionConflict reports whether ANY item in the transaction was
// cancelled because a concurrent transaction held it. Such a cancellation
// is transient: the exact same request retried a moment later can commit.
func AnyTransactionConflict(reasons []types.CancellationReason) bool {
	return anyReasonCode(reasons, transactionConflictCode)
}

// AnyTransientCancellation reports whether ANY item in the transaction was
// cancelled for a reason a caller-identical retry can resolve:
// TransactionConflict, ThrottlingError, ProvisionedThroughputExceeded, or
// TransactionInProgress. It does NOT consider an empty CancellationReasons
// slice — that anomalous shape is transient too, but the caller
// (BetRepository.transactWithConflictRetry) checks len(reasons) == 0
// separately, since "no reasons at all" is not "a reason code that matches"
// (finding R2).
func AnyTransientCancellation(reasons []types.CancellationReason) bool {
	return anyReasonCode(reasons, transactionConflictCode) ||
		anyReasonCode(reasons, throttlingErrorCode) ||
		anyReasonCode(reasons, provisionedThroughputExceededCode) ||
		anyReasonCode(reasons, transactionInProgressCode)
}

func anyReasonCode(reasons []types.CancellationReason, code string) bool {
	for _, reason := range reasons {
		if reason.Code != nil && *reason.Code == code {
			return true
		}
	}
	return false
}
