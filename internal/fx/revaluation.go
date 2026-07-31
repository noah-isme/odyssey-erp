package fx

import (
	"fmt"
	"strings"
)

type DocumentType string

const (
	ARInvoice DocumentType = "AR_INVOICE"
	APInvoice DocumentType = "AP_INVOICE"
)

type RevaluationInput struct {
	DocumentType                                     DocumentType
	OriginalBalance, PreviousBaseAmount, ClosingRate Decimal
}
type RevaluationResult struct{ ClosingBaseAmount, Difference Decimal }

// CalculateRevaluation computes only the period delta. It never changes the
// invoice's locked carrying value; the caller persists the detail row and a
// separate journal entry.
func CalculateRevaluation(in RevaluationInput) (RevaluationResult, error) {
	if in.DocumentType != ARInvoice && in.DocumentType != APInvoice {
		return RevaluationResult{}, fmt.Errorf("invalid document type %q", in.DocumentType)
	}
	closing, err := CalculateBaseAmount(in.OriginalBalance, in.ClosingRate)
	if err != nil {
		return RevaluationResult{}, err
	}
	return RevaluationResult{ClosingBaseAmount: closing, Difference: closing.Sub(in.PreviousBaseAmount).Round(2)}, nil
}

// RevaluationPolicy documents the chosen next-period policy in code and
// prevents callers from accidentally mixing cumulative and reversal behavior.
const RevaluationPolicy = "REVERSAL"

func ValidateRevaluationPolicy(policy string) error {
	if strings.ToUpper(policy) != RevaluationPolicy {
		return fmt.Errorf("unsupported FX revaluation policy %q", policy)
	}
	return nil
}
