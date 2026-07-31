// Package money contains exact, database-safe accounting amounts.
package money

import (
	"fmt"
	"math/big"
	"strings"
)

// Money is an exact decimal amount. Amount is kept as a decimal string so a
// PostgreSQL NUMERIC value never needs to pass through float64. Scale records
// the number of fractional digits expected by the originating boundary.
type Money struct {
	Amount string
	Scale  int
}

func Parse(amount string, scale int) (Money, error) {
	amount = strings.TrimSpace(amount)
	if amount == "" {
		return Money{}, fmt.Errorf("money: empty amount")
	}
	if _, ok := new(big.Rat).SetString(amount); !ok {
		return Money{}, fmt.Errorf("money: invalid amount %q", amount)
	}
	if scale < 0 {
		return Money{}, fmt.Errorf("money: negative scale")
	}
	return Money{Amount: amount, Scale: scale}, nil
}

func Must(amount string, scale int) Money {
	m, err := Parse(amount, scale)
	if err != nil {
		panic(err)
	}
	return m
}

func (m Money) String() string {
	if m.Amount == "" {
		return "0"
	}
	return m.Amount
}

func (m Money) rat() *big.Rat {
	if m.Amount == "" {
		return new(big.Rat)
	}
	r, ok := new(big.Rat).SetString(m.Amount)
	if !ok {
		return new(big.Rat)
	}
	return r
}

func (m Money) Add(other Money) Money { return m.fromRat(new(big.Rat).Add(m.rat(), other.rat())) }
func (m Money) Sub(other Money) Money { return m.fromRat(new(big.Rat).Sub(m.rat(), other.rat())) }
func (m Money) Cmp(other Money) int   { return m.rat().Cmp(other.rat()) }

func (m Money) fromRat(r *big.Rat) Money {
	scale := m.Scale
	if scale < 0 {
		scale = 0
	}
	return Money{Amount: r.FloatString(scale), Scale: scale}
}
