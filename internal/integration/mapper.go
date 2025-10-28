package integration

import "math"

func round2(value float64) float64 {
	return math.Round(value*100) / 100
}

func monetary(qty, unitCost float64) float64 {
	return round2(qty * unitCost)
}

func abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}
