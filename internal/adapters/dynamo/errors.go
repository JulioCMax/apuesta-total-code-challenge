package dynamo

import (
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// conditionalCheckFailedCode is the exact CancellationReason.Code value
// DynamoDB returns for a failed ConditionExpression inside a
// TransactWriteItems call.
const conditionalCheckFailedCode = "ConditionalCheckFailed"

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
