package payroll

import (
	"errors"
	"math"
)

// TER (Tarif Efektif Rata-Rata) Categories for Indonesian PPh 21 (2026 regulations)
type TERCategory string

const (
	TERCategoryA TERCategory = "A" // Single, no dependents (TK/0), TK/1, K/0
	TERCategoryB TERCategory = "B" // TK/2, TK/3, K/1, K/2
	TERCategoryC TERCategory = "C" // K/3
)

// CalculateBPJS computes BPJS Kesehatan and Ketenagakerjaan deductions based on Indonesian law.
// It returns (employeeDeduction, employerContribution).
func CalculateBPJS(grossSalary float64) (employee float64, employer float64) {
	// BPJS Kesehatan Cap (Rp 12,000,000 max base)
	kesehatanBase := grossSalary
	if kesehatanBase > 12000000 {
		kesehatanBase = 12000000
	}
	bpjsKesEmployee := kesehatanBase * 0.01 // 1% paid by employee
	bpjsKesEmployer := kesehatanBase * 0.04 // 4% paid by employer

	// BPJS Ketenagakerjaan
	// JHT (Jaminan Hari Tua)
	jhtEmployee := grossSalary * 0.02 // 2%
	jhtEmployer := grossSalary * 0.037 // 3.7%
	
	// JP (Jaminan Pensiun) - Capped at around ~Rp 10,042,300 (adjusting for inflation)
	jpBase := grossSalary
	if jpBase > 10042300 {
		jpBase = 10042300
	}
	jpEmployee := jpBase * 0.01 // 1%
	jpEmployer := jpBase * 0.02 // 2%

	// JKK (Jaminan Kecelakaan Kerja) - 0.24% default risk
	jkkEmployer := grossSalary * 0.0024

	// JKM (Jaminan Kematian) - 0.3%
	jkmEmployer := grossSalary * 0.003

	employee = bpjsKesEmployee + jhtEmployee + jpEmployee
	employer = bpjsKesEmployer + jhtEmployer + jpEmployer + jkkEmployer + jkmEmployer

	return math.Round(employee), math.Round(employer)
}

// CalculatePPh21TER calculates monthly income tax deduction using the TER (Tarif Efektif Rata-Rata) method.
func CalculatePPh21TER(grossSalary float64, category TERCategory) (float64, error) {
	// 2026 TER simplified tables (mocked brackets for brevity)
	var rate float64

	switch category {
	case TERCategoryA:
		if grossSalary <= 5400000 {
			rate = 0.0
		} else if grossSalary <= 5650000 {
			rate = 0.0025
		} else if grossSalary <= 5950000 {
			rate = 0.005
		} else if grossSalary <= 15000000 {
			rate = 0.05
		} else {
			rate = 0.15 // Progressive in higher brackets
		}
	case TERCategoryB:
		if grossSalary <= 6200000 {
			rate = 0.0
		} else if grossSalary <= 15000000 {
			rate = 0.04
		} else {
			rate = 0.12
		}
	case TERCategoryC:
		if grossSalary <= 6600000 {
			rate = 0.0
		} else if grossSalary <= 15000000 {
			rate = 0.03
		} else {
			rate = 0.10
		}
	default:
		return 0, errors.New("invalid TER Category")
	}

	return math.Round(grossSalary * rate), nil
}
