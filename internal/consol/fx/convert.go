package fx

import "errors"

// Converter applies FX policy rules to consolidated balances.
type Converter struct{}

// NewConverter constructs a converter instance.
func NewConverter() *Converter {
	return &Converter{}
}

// ErrConversionNotImplemented signals the FX conversion logic still needs to be delivered.
var ErrConversionNotImplemented = errors.New("consol: fx conversion not implemented")

// ConvertProfitLoss applies the configured FX policy to P&L amounts.
func (c *Converter) ConvertProfitLoss(input []Line) ([]Line, float64, error) {
	if c == nil {
		return nil, 0, ErrConversionNotImplemented
	}
	return nil, 0, ErrConversionNotImplemented
}

// ConvertBalanceSheet applies the configured FX policy to balance sheet amounts.
func (c *Converter) ConvertBalanceSheet(input []Line) ([]Line, float64, error) {
	if c == nil {
		return nil, 0, ErrConversionNotImplemented
	}
	return nil, 0, ErrConversionNotImplemented
}

// Line is a simplified representation of an amount eligible for FX conversion.
type Line struct {
	AccountCode string
	LocalAmount float64
	GroupAmount float64
}
