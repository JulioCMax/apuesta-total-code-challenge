package dynamo

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/shopspring/decimal"

	"github.com/JulioCMax/apuesta-total-code-challenge/internal/domain/money"
)

// MarshalMoney and UnmarshalMoney are the only place money.Money crosses
// the DynamoDB boundary. domain/money.Money deliberately does NOT
// implement attributevalue.Marshaler/Unmarshaler: doing so would require
// domain/money to import the AWS SDK's types package, breaking the stated
// dependency rule ("domain imports nothing but stdlib + shopspring/
// decimal"; "Nothing in domain/application imports ... the AWS SDK").
// These standalone functions achieve the same goal design.md calls out —
// an exact numeric N attribute, never decimal.Decimal's unexported struct
// fields — without that import. Every item is built by hand with these
// helpers (see betrepo.go), so generic reflection-based
// attributevalue.MarshalMap is never asked to encode a Money field.

// MarshalMoney returns m as an exact DynamoDB N attribute, comparable in a
// ConditionExpression (e.g. "balance >= :stake") with no float error.
func MarshalMoney(m money.Money) types.AttributeValue {
	return &types.AttributeValueMemberN{Value: m.String()}
}

// UnmarshalMoney decodes av (must be an N attribute) back into a Money
// value.
func UnmarshalMoney(av types.AttributeValue) (money.Money, error) {
	n, ok := av.(*types.AttributeValueMemberN)
	if !ok {
		return money.Money{}, fmt.Errorf("dynamo: expected N attribute for money, got %T", av)
	}
	d, err := decimal.NewFromString(n.Value)
	if err != nil {
		return money.Money{}, fmt.Errorf("dynamo: invalid money value %q: %w", n.Value, err)
	}
	m, err := money.NewMoney(d)
	if err != nil {
		return money.Money{}, fmt.Errorf("dynamo: invalid money value %q: %w", n.Value, err)
	}
	return m, nil
}

// MarshalOdds returns o as an exact DynamoDB N attribute, mirroring
// MarshalMoney for the same underlying-decimal reason.
func MarshalOdds(o money.Odds) types.AttributeValue {
	return &types.AttributeValueMemberN{Value: o.String()}
}

// UnmarshalOdds decodes av (must be an N attribute) back into an Odds
// value.
func UnmarshalOdds(av types.AttributeValue) (money.Odds, error) {
	n, ok := av.(*types.AttributeValueMemberN)
	if !ok {
		return money.Odds{}, fmt.Errorf("dynamo: expected N attribute for odds, got %T", av)
	}
	d, err := decimal.NewFromString(n.Value)
	if err != nil {
		return money.Odds{}, fmt.Errorf("dynamo: invalid odds value %q: %w", n.Value, err)
	}
	o, err := money.NewOdds(d)
	if err != nil {
		return money.Odds{}, fmt.Errorf("dynamo: invalid odds value %q: %w", n.Value, err)
	}
	return o, nil
}
