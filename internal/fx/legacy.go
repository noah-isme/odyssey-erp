package fx

import (
	"strconv"
)

// FromLegacyFloat converts a compatibility float at an input/output boundary
// into an exact Decimal. Callers must do all accounting arithmetic afterward
// with Decimal; this is not a license to perform calculations in float64.
func FromLegacyFloat(value float64, scale int) (Decimal, error) {
	return ParseDecimal(strconv.FormatFloat(value, 'f', scale, 64))
}
