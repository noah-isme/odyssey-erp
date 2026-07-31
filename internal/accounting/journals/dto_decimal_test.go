package journals

import (
	"testing"

	"github.com/google/uuid"
	"github.com/odyssey-erp/odyssey-erp/internal/fx"
)

func TestPostingValidationAcceptsExactDecimalLines(t *testing.T) {
	in := PostingInput{PeriodID: 1, SourceModule: "FX", SourceID: uuid.New(), Lines: []PostingLineInput{
		{AccountID: 1, DebitDecimal: fx.MustDecimal("0.10")},
		{AccountID: 2, CreditDecimal: fx.MustDecimal("0.10")},
	}}
	if err := in.Validate(); err != nil {
		t.Fatal(err)
	}
}
