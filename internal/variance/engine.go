package variance

import (
	"math"
	"sort"
)

// ComputeVariance merges base & compare balances and applies threshold flags.
func ComputeVariance(base map[string]AccountBalance, compare map[string]AccountBalance, thresholdAmount, thresholdPercent *float64) []VarianceRow {
	lookup := make(map[string]VarianceRow)
	for code, bal := range base {
		lookup[code] = VarianceRow{AccountCode: code, AccountName: bal.Name, BaseAmount: round2(bal.Amount)}
	}
	for code, bal := range compare {
		row := lookup[code]
		row.AccountCode = code
		if row.AccountName == "" {
			row.AccountName = bal.Name
		}
		row.CompareAmount = round2(bal.Amount)
		lookup[code] = row
	}
	rows := make([]VarianceRow, 0, len(lookup))
	for _, row := range lookup {
		row.Variance = round2(row.BaseAmount - row.CompareAmount)
		if row.CompareAmount != 0 {
			row.VariancePct = round2((row.Variance / math.Abs(row.CompareAmount)) * 100)
		}
		row.Flagged = exceedsThreshold(row, thresholdAmount, thresholdPercent)
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		return math.Abs(rows[i].Variance) > math.Abs(rows[j].Variance)
	})
	return rows
}

// AccountBalance wraps aggregated values for an account.
type AccountBalance struct {
	Name   string
	Amount float64
}

func exceedsThreshold(row VarianceRow, amt, pct *float64) bool {
	if amt != nil && math.Abs(row.Variance) >= *amt {
		return true
	}
	if pct != nil && math.Abs(row.VariancePct) >= *pct {
		return true
	}
	return false
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
