package dynamo_test

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/dynamo"
)

func codePtr(s string) *string { return &s }

// TestAsTransactionCanceled_ExtractsTheTypedException proves a wrapped
// *types.TransactionCanceledException is recoverable via errors.As-style
// extraction, and that an unrelated error is correctly reported as "not a
// cancellation".
func TestAsTransactionCanceled_ExtractsTheTypedException(t *testing.T) {
	canceled := &types.TransactionCanceledException{
		CancellationReasons: []types.CancellationReason{
			{Code: codePtr("ConditionalCheckFailed")},
			{Code: codePtr("None")},
		},
	}
	wrapped := &smithy.OperationError{Err: canceled}

	got, ok := dynamo.AsTransactionCanceled(wrapped)
	require.True(t, ok)
	require.Same(t, canceled, got)

	_, ok = dynamo.AsTransactionCanceled(errors.New("unrelated"))
	require.False(t, ok)
}

// TestConditionalCheckFailed_ReadsTheReasonAtIndex proves the helper reads
// exactly the reason code at the given index, per design.md's index-
// parallel CancellationReasons contract.
func TestConditionalCheckFailed_ReadsTheReasonAtIndex(t *testing.T) {
	reasons := []types.CancellationReason{
		{Code: codePtr("ConditionalCheckFailed")}, // idx 0: balance debit failed
		{Code: codePtr("None")},                   // idx 1: bet put not evaluated
	}

	require.True(t, dynamo.ConditionalCheckFailed(reasons, 0))
	require.False(t, dynamo.ConditionalCheckFailed(reasons, 1))
}

// TestConditionalCheckFailed_ReportsFalseForOutOfRangeIndex proves an
// index beyond the recorded reasons (e.g. no Idempotency-Key header was
// supplied, so only 2 items were ever in the transaction) never panics and
// is reported as "did not fail" rather than crashing the placement path.
func TestConditionalCheckFailed_ReportsFalseForOutOfRangeIndex(t *testing.T) {
	reasons := []types.CancellationReason{{Code: codePtr("None")}}

	require.False(t, dynamo.ConditionalCheckFailed(reasons, 2))
	require.False(t, dynamo.ConditionalCheckFailed(nil, 0))
}
