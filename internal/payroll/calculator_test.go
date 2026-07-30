package payroll

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func money(v Money) *Money { return &v }

func testRules() Rules {
	return Rules{TaxVersionID: 1, BPJSVersionID: 2, PTKPCategory: map[string]string{"TK/0": "A", "TK/1": "A", "K/0": "A", "TK/2": "B", "TK/3": "B", "K/1": "B", "K/2": "B", "K/3": "C"}, TER: []TERBracket{
		{Category: "A", LowerBound: 0, UpperBound: money(5400000), RateBPS: 0}, {Category: "A", LowerBound: 5400000, UpperBound: money(5650000), RateBPS: 25},
		{Category: "B", LowerBound: 0, UpperBound: money(6200000), RateBPS: 0}, {Category: "B", LowerBound: 6200000, UpperBound: money(6500000), RateBPS: 25},
		{Category: "C", LowerBound: 0, UpperBound: money(6600000), RateBPS: 0}, {Category: "C", LowerBound: 6600000, UpperBound: money(6950000), RateBPS: 25},
		{Category: "A", LowerBound: 13750000, UpperBound: money(15100000), RateBPS: 600}, {Category: "A", LowerBound: 15100000, UpperBound: money(16950000), RateBPS: 700},
	}, BPJS: []BPJSRule{{Program: "HEALTH", EmployeeRateBPS: 100, EmployerRateBPS: 400, WageCap: 12000000, EmployerTaxable: true}, {Program: "JP", EmployeeRateBPS: 100, EmployerRateBPS: 200, WageCap: 11086300}, {Program: "JHT", EmployeeRateBPS: 200, EmployerRateBPS: 370}, {Program: "JKK", EmployerRateBPS: 54, EmployerTaxable: true}, {Program: "JKM", EmployerRateBPS: 30, EmployerTaxable: true}}}
}

func testPolicy() Policy {
	return Policy{VersionID: 3, OvertimeDivisor: 173, FirstHourMultiplierBPS: 15000, SubsequentHourMultiplierBPS: 20000, RoundingUnit: 1}
}

func TestTERPTKPCategoriesAndBoundary(t *testing.T) {
	expected := map[string]string{"TK/0": "A", "TK/1": "A", "K/0": "A", "TK/2": "B", "TK/3": "B", "K/1": "B", "K/2": "B", "K/3": "C"}
	for status, category := range expected {
		require.Equal(t, category, testRules().PTKPCategory[status], status)
	}
	for _, tc := range []struct {
		code, category string
		gross          Money
	}{{"TK/0", "A", 5500000}, {"TK/2", "B", 6300000}, {"K/3", "C", 6700000}} {
		t.Run(tc.category, func(t *testing.T) {
			rate, ok := FindTERRate(tc.category, tc.gross, testRules().TER)
			require.True(t, ok)
			require.Equal(t, int64(25), rate)
		})
	}
	rate, ok := FindTERRate("A", 5400000, testRules().TER)
	require.True(t, ok)
	require.Zero(t, rate)
}

func TestBPJSCapsAndCompleteBreakdown(t *testing.T) {
	result, err := Calculate(Input{EmployeeID: 7, PTKPCode: "TK/0", BaseSalary: 15000000, BPJSWage: 15000000, BPJSHealth: true, BPJSEmployment: true}, testRules(), testPolicy())
	require.NoError(t, err)
	var health, pension Contribution
	for _, c := range result.Contributions {
		if c.Program == "HEALTH" {
			health = c
		}
		if c.Program == "JP" {
			pension = c
		}
	}
	require.Equal(t, Money(12000000), health.WageBase)
	require.Equal(t, Money(11086300), pension.WageBase)
	require.Equal(t, Money(110863), pension.Employee)
	require.Positive(t, result.EmployerBPJS)
}

func TestOvertimeTHRNegativeAdjustmentAndRounding(t *testing.T) {
	policy := testPolicy()
	require.Equal(t, Money(202312), OvertimePay(10000000, 120, policy))
	require.Equal(t, Money(5000000), THRAmount(10000000, 6))
	require.Equal(t, Money(10000000), THRAmount(10000000, 12))
	result, err := Calculate(Input{EmployeeID: 1, PTKPCode: "TK/0", BaseSalary: 5000000, Adjustments: -500000, BPJSWage: 0}, testRules(), policy)
	require.NoError(t, err)
	require.Equal(t, Money(4500000), result.Gross)
	require.Equal(t, Money(4500000), result.NetPay)
	require.Equal(t, Money(1300), Round(1250, 100))
	require.Equal(t, Money(-1300), Round(-1250, 100))
}

func TestOfficialDJPExampleMonthlyTER(t *testing.T) {
	// PP 58/2023 explanation: K/0, Rp10m monthly gross uses TER A at 2%.
	rate, ok := FindTERRate("A", 10000000, append(testRules().TER, TERBracket{Category: "A", LowerBound: 9650000, UpperBound: money(10050000), RateBPS: 200}))
	require.True(t, ok)
	require.Equal(t, int64(200), rate)
	require.Equal(t, Money(200000), Percent(10000000, rate))
}
