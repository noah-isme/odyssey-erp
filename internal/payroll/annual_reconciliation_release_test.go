package payroll

import (
	_ "embed"
	"encoding/json"
	"testing"
)

// The example is reproduced from the official PMK 168/2023 worked example;
// keeping it in testdata makes the release evidence reviewable without
// embedding a legal expectation in the calculator implementation.
//
//go:embed testdata/pph21_december_2024_tn_a.json
var pph21DecemberFixture []byte

type pph21AnnualFixture struct {
	Source                         string        `json:"source"`
	TaxYear                        int           `json:"tax_year"`
	EmployeeLabel                  string        `json:"employee_label"`
	PTKPAnnual                     Money         `json:"ptkp_annual"`
	JobExpenseRateBPS              int64         `json:"job_expense_rate_bps"`
	JobExpenseCap                  Money         `json:"job_expense_cap"`
	PensionContribution            Money         `json:"pension_contribution"`
	MandatoryReligiousContribution Money         `json:"mandatory_religious_contribution"`
	LastTaxPeriod                  int           `json:"last_tax_period"`
	Periods                        []PPh21Period `json:"periods"`
	Expected                       pph21Expected `json:"expected"`
}

type pph21Expected struct {
	GrossIncome                 Money `json:"gross_income"`
	JobExpense                  Money `json:"job_expense"`
	NetIncome                   Money `json:"net_income"`
	TaxableIncome               Money `json:"taxable_income"`
	AnnualPPh21                 Money `json:"annual_pph21"`
	WithheldBeforeLastTaxPeriod Money `json:"withheld_before_last_tax_period"`
	LastTaxPeriodPPh21          Money `json:"last_tax_period_pph21"`
}

func TestDecemberAnnualPPh21Reconciliation(t *testing.T) {
	var fixture pph21AnnualFixture
	if err := json.Unmarshal(pph21DecemberFixture, &fixture); err != nil {
		t.Fatalf("decode approved PPh 21 fixture: %v", err)
	}
	if fixture.Source == "" || fixture.TaxYear != 2024 || fixture.EmployeeLabel == "" {
		t.Fatalf("fixture is missing source identity: %+v", fixture)
	}

	upper := func(value Money) *Money { return &value }
	policy := PPh21AnnualPolicy{
		JobExpenseRateBPS:   fixture.JobExpenseRateBPS,
		JobExpenseCap:       fixture.JobExpenseCap,
		TaxableRoundingUnit: 1000,
		ProgressiveBrackets: []PPh21AnnualBracket{
			{UpperBound: upper(60000000), RateBPS: 500},
			{UpperBound: upper(250000000), RateBPS: 1500},
			{UpperBound: upper(500000000), RateBPS: 2500},
			{UpperBound: upper(5000000000), RateBPS: 3000},
			{RateBPS: 3500},
		},
	}
	got, err := CalculateAnnualPPh21(PPh21AnnualInput{
		PTKPAnnual:                     fixture.PTKPAnnual,
		Periods:                        fixture.Periods,
		PensionContribution:            fixture.PensionContribution,
		MandatoryReligiousContribution: fixture.MandatoryReligiousContribution,
		LastTaxPeriod:                  fixture.LastTaxPeriod,
	}, policy)
	if err != nil {
		t.Fatalf("annual PPh 21 calculation failed: %v", err)
	}

	if got.GrossIncome != fixture.Expected.GrossIncome || got.JobExpense != fixture.Expected.JobExpense || got.NetIncome != fixture.Expected.NetIncome || got.TaxableIncome != fixture.Expected.TaxableIncome || got.AnnualPPh21 != fixture.Expected.AnnualPPh21 || got.WithheldBeforeLastTaxPeriod != fixture.Expected.WithheldBeforeLastTaxPeriod || got.LastTaxPeriodPPh21 != fixture.Expected.LastTaxPeriodPPh21 {
		t.Fatalf("annual PPh 21 evidence mismatch: got=%+v expected=%+v", got, fixture.Expected)
	}
}

func TestAnnualPPh21AllowsRefundWhenPriorWithholdingIsHigher(t *testing.T) {
	upper := func(value Money) *Money { return &value }
	got, err := CalculateAnnualPPh21(PPh21AnnualInput{
		PTKPAnnual: 54000000,
		Periods:    []PPh21Period{{Month: 1, Gross: 5000000, Withheld: 1000}, {Month: 12, Gross: 5000000}},
	}, PPh21AnnualPolicy{
		JobExpenseRateBPS:   500,
		JobExpenseCap:       6000000,
		TaxableRoundingUnit: 1000,
		ProgressiveBrackets: []PPh21AnnualBracket{{UpperBound: upper(60000000), RateBPS: 500}, {RateBPS: 1500}},
	})
	if err != nil {
		t.Fatalf("annual PPh 21 calculation failed: %v", err)
	}
	if got.LastTaxPeriodPPh21 >= 0 {
		t.Fatalf("expected negative last-period correction, got %d", got.LastTaxPeriodPPh21)
	}
}
