package payroll

import "errors"

var (
	ErrInvalidAnnualInput  = errors.New("payroll: invalid annual PPh 21 input")
	ErrInvalidAnnualPolicy = errors.New("payroll: invalid annual PPh 21 policy")
)

// PPh21Period is the gross income and withholding recorded for one tax
// period. Month is the calendar month number, 1 through 12.
type PPh21Period struct {
	Month    int
	Gross    Money
	Withheld Money
}

// PPh21AnnualInput contains the evidence needed to calculate the last-tax-
// period PPh 21 amount for a permanent employee. The periods are deliberately
// supplied by the caller so a payroll run can retain the exact monthly values
// used for the annual certificate.
type PPh21AnnualInput struct {
	PTKPAnnual                     Money
	Periods                        []PPh21Period
	PensionContribution            Money
	MandatoryReligiousContribution Money
	LastTaxPeriod                  int
}

// PPh21AnnualBracket is one progressive annual PPh 21 band. A nil UpperBound
// marks the final, unlimited band. Bounds are inclusive from the tax
// authority's perspective; the implementation applies each rate to the
// amount above the previous bound.
type PPh21AnnualBracket struct {
	UpperBound *Money
	RateBPS    int64
}

// PPh21AnnualPolicy keeps volatile annual-tax rules explicit and versionable.
// It is intentionally passed to CalculateAnnualPPh21 instead of being hidden
// in the calculator, so a new reviewed rule version can be selected without
// changing historical payroll results.
type PPh21AnnualPolicy struct {
	JobExpenseRateBPS   int64
	JobExpenseCap       Money
	TaxableRoundingUnit Money
	ProgressiveBrackets []PPh21AnnualBracket
}

// PPh21AnnualResult is the auditable annual calculation and the amount due in
// the last tax period. LastTaxPeriodPPh21 may be negative when prior monthly
// withholding exceeded the annual liability and must not be clamped.
type PPh21AnnualResult struct {
	GrossIncome                    Money
	JobExpense                     Money
	PensionContribution            Money
	MandatoryReligiousContribution Money
	NetIncome                      Money
	PTKPAnnual                     Money
	TaxableIncome                  Money
	AnnualPPh21                    Money
	WithheldBeforeLastTaxPeriod    Money
	LastTaxPeriodPPh21             Money
}

// CalculateAnnualPPh21 applies the last-tax-period rule: annual tax under the
// progressive rates minus PPh 21 already withheld in earlier periods. It is
// suitable for December or an earlier termination month when the caller sets
// LastTaxPeriod to that month and supplies the corresponding period history.
func CalculateAnnualPPh21(in PPh21AnnualInput, policy PPh21AnnualPolicy) (PPh21AnnualResult, error) {
	if in.PTKPAnnual < 0 || in.PensionContribution < 0 || in.MandatoryReligiousContribution < 0 || len(in.Periods) == 0 {
		return PPh21AnnualResult{}, ErrInvalidAnnualInput
	}
	if in.LastTaxPeriod == 0 {
		in.LastTaxPeriod = 12
	}
	if in.LastTaxPeriod < 1 || in.LastTaxPeriod > 12 {
		return PPh21AnnualResult{}, ErrInvalidAnnualInput
	}
	if policy.JobExpenseRateBPS < 0 || policy.JobExpenseCap < 0 || policy.TaxableRoundingUnit <= 0 || !validAnnualBrackets(policy.ProgressiveBrackets) {
		return PPh21AnnualResult{}, ErrInvalidAnnualPolicy
	}

	var result PPh21AnnualResult
	seenMonths := make(map[int]struct{}, len(in.Periods))
	for _, period := range in.Periods {
		if period.Month < 1 || period.Month > 12 || period.Gross < 0 || period.Withheld < 0 {
			return PPh21AnnualResult{}, ErrInvalidAnnualInput
		}
		if _, exists := seenMonths[period.Month]; exists {
			return PPh21AnnualResult{}, ErrInvalidAnnualInput
		}
		seenMonths[period.Month] = struct{}{}
		result.GrossIncome += period.Gross
		if period.Month != in.LastTaxPeriod {
			result.WithheldBeforeLastTaxPeriod += period.Withheld
		}
	}
	if _, exists := seenMonths[in.LastTaxPeriod]; !exists {
		return PPh21AnnualResult{}, ErrInvalidAnnualInput
	}

	result.JobExpense = Percent(result.GrossIncome, policy.JobExpenseRateBPS)
	if policy.JobExpenseCap > 0 && result.JobExpense > policy.JobExpenseCap {
		result.JobExpense = policy.JobExpenseCap
	}
	result.PensionContribution = in.PensionContribution
	result.MandatoryReligiousContribution = in.MandatoryReligiousContribution
	result.NetIncome = result.GrossIncome - result.JobExpense - result.PensionContribution - result.MandatoryReligiousContribution
	if result.NetIncome < 0 {
		result.NetIncome = 0
	}
	result.PTKPAnnual = in.PTKPAnnual
	result.TaxableIncome = result.NetIncome - result.PTKPAnnual
	if result.TaxableIncome < 0 {
		result.TaxableIncome = 0
	}
	// PMK 168/2023 requires the annual taxable income used for the progressive
	// rates to be rounded down to a full thousand rupiah.
	result.TaxableIncome = (result.TaxableIncome / policy.TaxableRoundingUnit) * policy.TaxableRoundingUnit
	result.AnnualPPh21 = progressivePPh21(result.TaxableIncome, policy.ProgressiveBrackets)
	result.LastTaxPeriodPPh21 = result.AnnualPPh21 - result.WithheldBeforeLastTaxPeriod
	return result, nil
}

func validAnnualBrackets(brackets []PPh21AnnualBracket) bool {
	if len(brackets) == 0 {
		return false
	}
	var previous Money
	for i, bracket := range brackets {
		if bracket.RateBPS < 0 {
			return false
		}
		if bracket.UpperBound == nil {
			return i == len(brackets)-1
		}
		if *bracket.UpperBound <= previous {
			return false
		}
		previous = *bracket.UpperBound
	}
	return false
}

func progressivePPh21(taxable Money, brackets []PPh21AnnualBracket) Money {
	if taxable <= 0 {
		return 0
	}
	var tax Money
	var lower Money
	remaining := taxable
	for _, bracket := range brackets {
		band := remaining
		if bracket.UpperBound != nil {
			upper := *bracket.UpperBound
			if taxable <= lower {
				break
			}
			band = taxable - lower
			if band > upper-lower {
				band = upper - lower
			}
		}
		if band > 0 {
			tax += Percent(band, bracket.RateBPS)
			remaining -= band
		}
		if bracket.UpperBound == nil || remaining <= 0 {
			break
		}
		lower = *bracket.UpperBound
	}
	return tax
}
