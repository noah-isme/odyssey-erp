package elimination

import "math"

// ComputeElimination derives summary values from balances.
func ComputeElimination(sourceBalance, targetBalance float64) SimulationSummary {
	amount := math.Min(math.Abs(sourceBalance), math.Abs(targetBalance))
	if math.IsNaN(amount) || math.IsInf(amount, 0) {
		amount = 0
	}
	return SimulationSummary{
		SourceBalance: round2(sourceBalance),
		TargetBalance: round2(targetBalance),
		Eliminated:    round2(amount),
	}
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
