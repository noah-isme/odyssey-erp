package payroll

import (
	"errors"
	"sort"
)

var (
	ErrInvalidInput = errors.New("payroll: invalid calculation input")
	ErrMissingRule  = errors.New("payroll: required rule is missing")
)

// Money is an integer number of rupiah. Payroll never uses binary floating point.
type Money int64

type TERBracket struct {
	Category   string
	LowerBound Money
	UpperBound *Money
	RateBPS    int64
}

type BPJSRule struct {
	Program         string
	EmployeeRateBPS int64
	EmployerRateBPS int64
	WageFloor       Money
	WageCap         Money
	EmployerTaxable bool
}

type Rules struct {
	TaxVersionID, BPJSVersionID int64
	PTKPCategory                map[string]string
	TER                         []TERBracket
	BPJS                        []BPJSRule
}

type Policy struct {
	VersionID                   int64
	OvertimeDivisor             int64
	FirstHourMultiplierBPS      int64
	SubsequentHourMultiplierBPS int64
	RoundingUnit                Money
}

type Input struct {
	EmployeeID                   int64
	PTKPCode                     string
	BaseSalary, Allowances, THR  Money
	Adjustments, OtherDeductions Money
	BPJSWage                     Money
	OvertimeMinutes              int64
	AttendanceDays, LeaveDays    int64
	BPJSHealth, BPJSEmployment   bool
}

type Contribution struct {
	Program            string
	WageBase           Money
	Employee, Employer Money
	EmployerTaxable    bool
}

type Result struct {
	EmployeeID                            int64
	TaxVersionID, BPJSVersionID, PolicyID int64
	PTKPCode, TERCategory                 string
	BaseSalary, Allowances, Overtime, THR Money
	Adjustments, Gross, TaxableGross      Money
	EmployeeBPJS, EmployerBPJS, PPh21     Money
	OtherDeductions, NetPay               Money
	TERRateBPS                            int64
	Contributions                         []Contribution
	AttendanceDays, LeaveDays             int64
}

func Calculate(in Input, rules Rules, policy Policy) (Result, error) {
	category := rules.PTKPCategory[in.PTKPCode]
	if in.EmployeeID <= 0 || in.BaseSalary < 0 || in.Allowances < 0 || in.THR < 0 || in.OtherDeductions < 0 || in.OvertimeMinutes < 0 || in.BPJSWage < 0 || policy.OvertimeDivisor <= 0 || policy.RoundingUnit <= 0 {
		return Result{}, ErrInvalidInput
	}
	if category == "" || rules.TaxVersionID <= 0 || rules.BPJSVersionID <= 0 || policy.VersionID <= 0 {
		return Result{}, ErrMissingRule
	}
	overtime := OvertimePay(in.BaseSalary, in.OvertimeMinutes, policy)
	gross := Round(in.BaseSalary+in.Allowances+overtime+in.THR+in.Adjustments, policy.RoundingUnit)
	if gross < 0 {
		return Result{}, ErrInvalidInput
	}
	result := Result{EmployeeID: in.EmployeeID, TaxVersionID: rules.TaxVersionID, BPJSVersionID: rules.BPJSVersionID, PolicyID: policy.VersionID, PTKPCode: in.PTKPCode, TERCategory: category, BaseSalary: in.BaseSalary, Allowances: in.Allowances, Overtime: overtime, THR: in.THR, Adjustments: in.Adjustments, Gross: gross, OtherDeductions: in.OtherDeductions, AttendanceDays: in.AttendanceDays, LeaveDays: in.LeaveDays}
	taxableEmployer := Money(0)
	for _, rule := range rules.BPJS {
		if rule.Program == "HEALTH" && !in.BPJSHealth || rule.Program != "HEALTH" && !in.BPJSEmployment {
			continue
		}
		base := in.BPJSWage
		if base < rule.WageFloor {
			base = rule.WageFloor
		}
		if rule.WageCap > 0 && base > rule.WageCap {
			base = rule.WageCap
		}
		contribution := Contribution{Program: rule.Program, WageBase: base, Employee: Percent(base, rule.EmployeeRateBPS), Employer: Percent(base, rule.EmployerRateBPS), EmployerTaxable: rule.EmployerTaxable}
		result.EmployeeBPJS += contribution.Employee
		result.EmployerBPJS += contribution.Employer
		if rule.EmployerTaxable {
			taxableEmployer += contribution.Employer
		}
		result.Contributions = append(result.Contributions, contribution)
	}
	result.TaxableGross = gross + taxableEmployer
	result.TERRateBPS, _ = FindTERRate(category, result.TaxableGross, rules.TER)
	if result.TERRateBPS < 0 {
		return Result{}, ErrMissingRule
	}
	result.PPh21 = Percent(result.TaxableGross, result.TERRateBPS)
	result.NetPay = Round(gross-result.EmployeeBPJS-result.PPh21-in.OtherDeductions, policy.RoundingUnit)
	return result, nil
}

func FindTERRate(category string, gross Money, brackets []TERBracket) (int64, bool) {
	ordered := append([]TERBracket(nil), brackets...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].LowerBound < ordered[j].LowerBound })
	for _, bracket := range ordered {
		if bracket.Category == category && gross > bracket.LowerBound && (bracket.UpperBound == nil || gross <= *bracket.UpperBound) {
			return bracket.RateBPS, true
		}
		if bracket.Category == category && gross == 0 && bracket.LowerBound == 0 {
			return bracket.RateBPS, true
		}
	}
	return -1, false
}

func OvertimePay(base Money, minutes int64, policy Policy) Money {
	if minutes <= 0 || base <= 0 || policy.OvertimeDivisor <= 0 {
		return 0
	}
	first := minutes
	if first > 60 {
		first = 60
	}
	rest := minutes - first
	// Combine before division to retain sub-hour precision, then round once.
	numerator := int64(base) * (first*policy.FirstHourMultiplierBPS + rest*policy.SubsequentHourMultiplierBPS)
	denominator := policy.OvertimeDivisor * 60 * 10000
	return Round(Money(divRoundHalfAway(numerator, denominator)), policy.RoundingUnit)
}

func THRAmount(monthlyBase Money, serviceMonths int64) Money {
	if monthlyBase <= 0 || serviceMonths <= 0 {
		return 0
	}
	if serviceMonths >= 12 {
		return monthlyBase
	}
	return Money(divRoundHalfAway(int64(monthlyBase)*serviceMonths, 12))
}

func Percent(value Money, rateBPS int64) Money {
	return Money(divRoundHalfAway(int64(value)*rateBPS, 10000))
}

func Round(value, unit Money) Money {
	if unit <= 1 {
		return value
	}
	return Money(divRoundHalfAway(int64(value), int64(unit))) * unit
}

func divRoundHalfAway(numerator, denominator int64) int64 {
	if denominator <= 0 {
		return 0
	}
	if numerator < 0 {
		return -((-numerator + denominator/2) / denominator)
	}
	return (numerator + denominator/2) / denominator
}
