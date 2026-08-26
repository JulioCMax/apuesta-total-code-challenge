// Package money provides the Money and Odds value objects used throughout
// the domain. It wraps github.com/shopspring/decimal so that every monetary
// calculation in the codebase uses exact decimal arithmetic instead of
// binary floating point, and exposes Round2 as the single rounding function
// (D6): no other package may call decimal.Round directly.
package money

import (
	"errors"

	"github.com/shopspring/decimal"
)

// ErrNegativeAmount is returned when constructing a Money value from a
// negative amount.
var ErrNegativeAmount = errors.New("money: amount must be non-negative")

// Round2 rounds d to exactly 2 decimal places and is the ONLY rounding call
// site in the codebase (D6). shopspring/decimal's Round adds sign(d)*0.5
// before truncating, i.e. half-away-from-zero rounding, which is equivalent
// to HALF-UP for the non-negative values Money and Odds always hold.
func Round2(d decimal.Decimal) decimal.Decimal {
	return d.Round(2)
}

// Money represents a non-negative monetary amount, always held rounded to
// 2 decimal places.
type Money struct {
	d decimal.Decimal
}

// NewMoney builds a Money value from a decimal amount, rejecting negative
// amounts and rounding the result via Round2.
func NewMoney(amount decimal.Decimal) (Money, error) {
	if amount.IsNegative() {
		return Money{}, ErrNegativeAmount
	}
	return Money{d: Round2(amount)}, nil
}

// NewMoneyFromFloat is a convenience constructor for float64 literals
// (mainly tests and fixtures); production code reading external input
// should prefer NewMoney with a decimal parsed from the exact source text.
func NewMoneyFromFloat(amount float64) (Money, error) {
	return NewMoney(decimal.NewFromFloat(amount))
}

// Mul multiplies m by a decimal factor (e.g. an Odds value) and rounds the
// result via Round2.
func (m Money) Mul(factor decimal.Decimal) Money {
	return Money{d: Round2(m.d.Mul(factor))}
}

// LessThan reports whether m < other.
func (m Money) LessThan(other Money) bool {
	return m.d.LessThan(other.d)
}

// GreaterThan reports whether m > other.
func (m Money) GreaterThan(other Money) bool {
	return m.d.GreaterThan(other.d)
}

// String returns the fixed 2-decimal representation, e.g. "100.00".
func (m Money) String() string {
	return m.d.StringFixed(2)
}

// MarshalJSON renders Money as an unquoted fixed-2 JSON number (D13),
// e.g. 100.00, matching TrueOdds' numeric shape in the source data.
func (m Money) MarshalJSON() ([]byte, error) {
	return []byte(m.d.StringFixed(2)), nil
}
