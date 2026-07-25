package fixedassets

import "testing"

func TestMonthlyDepreciation(t *testing.T) {
	tests := []struct {
		name                                    string
		cost, residual, life, accumulated, want float64
	}{
		{"straight line", 12000, 0, 12, 0, 1000},
		{"residual value", 12000, 1200, 12, 0, 900},
		{"final month capped", 12000, 0, 12, 11500, 500},
		{"fully depreciated", 12000, 0, 12, 12000, 0},
		{"invalid life", 12000, 0, 0, 0, 0},
		{"residual equals cost", 12000, 12000, 12, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := monthlyDepreciation(tt.cost, tt.residual, tt.life, tt.accumulated); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
