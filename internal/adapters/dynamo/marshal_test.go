package dynamo_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/adapters/dynamo"
	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

// TestMarshalMoney_ProducesExactNumericAttribute proves Money marshals to
// a DynamoDB N attribute holding the exact fixed-2 decimal string, never
// the Decimal type's unexported struct fields (design.md's "Non-obvious
// adapter detail").
func TestMarshalMoney_ProducesExactNumericAttribute(t *testing.T) {
	m, err := money.NewMoneyFromFloat(1234.5)
	require.NoError(t, err)

	av := dynamo.MarshalMoney(m)

	n, ok := av.(*types.AttributeValueMemberN)
	require.True(t, ok, "money must marshal to an N attribute, got %T", av)
	require.Equal(t, "1234.50", n.Value)
}

// TestUnmarshalMoney_RoundTripsExactValue proves a marshaled Money value
// unmarshals back to the exact same amount, with no float drift.
func TestUnmarshalMoney_RoundTripsExactValue(t *testing.T) {
	original, err := money.NewMoneyFromFloat(9999.99)
	require.NoError(t, err)

	got, err := dynamo.UnmarshalMoney(dynamo.MarshalMoney(original))
	require.NoError(t, err)

	require.Equal(t, original.String(), got.String())
}

// TestUnmarshalMoney_RejectsNonNumericAttribute proves a malformed stored
// attribute (wrong type) is a typed decode error, never a silent zero
// value.
func TestUnmarshalMoney_RejectsNonNumericAttribute(t *testing.T) {
	_, err := dynamo.UnmarshalMoney(&types.AttributeValueMemberS{Value: "not-a-number"})
	require.Error(t, err)
}

// TestMarshalOdds_ProducesExactNumericAttribute proves Odds marshals the
// same way Money does (same underlying decimal representation issue).
func TestMarshalOdds_ProducesExactNumericAttribute(t *testing.T) {
	o, err := money.NewOddsFromFloat(1.85)
	require.NoError(t, err)

	av := dynamo.MarshalOdds(o)

	n, ok := av.(*types.AttributeValueMemberN)
	require.True(t, ok, "odds must marshal to an N attribute, got %T", av)
	require.Equal(t, "1.85", n.Value)
}

// TestUnmarshalOdds_RoundTripsExactValue proves a marshaled Odds value
// unmarshals back to the exact same value.
func TestUnmarshalOdds_RoundTripsExactValue(t *testing.T) {
	original, err := money.NewOddsFromFloat(3.89)
	require.NoError(t, err)

	got, err := dynamo.UnmarshalOdds(dynamo.MarshalOdds(original))
	require.NoError(t, err)

	require.Equal(t, original.String(), got.String())
}
