package money

import (
	"errors"

	"github.com/shopspring/decimal"
)

// ErrOddsTooLow is returned when constructing Odds below the house minimum
// of 1.01.
var ErrOddsTooLow = errors.New("money: odds must be at least 1.01")

// minOdds is the minimum valid odds value.
var minOdds = decimal.NewFromFloat(1.01)

// Odds represents betting odds: a decimal value >= 1.01, always held
// rounded to 2 decimal places.
type Odds struct {
	d decimal.Decimal
}

// NewOdds builds an Odds value from a decimal, rounding it via Round2 and
// rejecting anything below the minimum of 1.01.
func NewOdds(value decimal.Decimal) (Odds, error) {
	rounded := Round2(value)
	if rounded.LessThan(minOdds) {
		return Odds{}, ErrOddsTooLow
	}
	return Odds{d: rounded}, nil
}

// NewOddsFromFloat is a convenience constructor for float64 literals (tests
// and fixtures); production code reading external input should prefer
// NewOdds with a decimal parsed from the exact source text.
func NewOddsFromFloat(value float64) (Odds, error) {
	return NewOdds(decimal.NewFromFloat(value))
}

// MustOdds is like NewOdds but panics on error; intended for package-level
// constants and test fixtures where the value is a known-valid literal.
func MustOdds(value decimal.Decimal) Odds {
	o, err := NewOdds(value)
	if err != nil {
		panic(err)
	}
	return o
}

// Decimal exposes the underlying decimal value.
func (o Odds) Decimal() decimal.Decimal {
	return o.d
}

// String returns the fixed 2-decimal representation, e.g. "1.85".
func (o Odds) String() string {
	return o.d.StringFixed(2)
}

// MarshalJSON renders Odds as an unquoted fixed-2 JSON number (D13),
// matching Money's shape.
func (o Odds) MarshalJSON() ([]byte, error) {
	return []byte(o.d.StringFixed(2)), nil
}

// UnmarshalJSON parses a JSON number (or numeric string) into Odds,
// enforcing the minimum of 1.01.
func (o *Odds) UnmarshalJSON(data []byte) error {
	var d decimal.Decimal
	if err := d.UnmarshalJSON(data); err != nil {
		return err
	}
	parsed, err := NewOdds(d)
	if err != nil {
		return err
	}
	*o = parsed
	return nil
}

// Combine multiplies all given odds and rounds the product exactly once via
// Round2 (D7): combinedOdds = Round2(product of odds). This is the only
// place combined odds are computed, so potentialReturns computed from the
// result always reconciles with what the caller was shown.
func Combine(odds ...Odds) Odds {
	if len(odds) == 0 {
		return Odds{d: decimal.Zero}
	}
	product := decimal.NewFromInt(1)
	for _, o := range odds {
		product = product.Mul(o.d)
	}
	return Odds{d: Round2(product)}
}
