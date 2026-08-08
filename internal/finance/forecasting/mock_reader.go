package forecasting

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
)

// MockReader is a stub implementation of SourceReader for testing.
type MockReader struct {
	sourceType SourceType
	name       string
	isOutflow  bool
}

func NewMockReader(name string, sourceType SourceType, isOutflow bool) *MockReader {
	return &MockReader{
		name:       name,
		sourceType: sourceType,
		isOutflow:  isOutflow,
	}
}

func (r *MockReader) Name() string {
	return r.name
}

func (r *MockReader) ReadExpectedFlows(ctx context.Context, companyID int64, fromDate, toDate time.Time) ([]ExpectedCashFlow, error) {
	var flows []ExpectedCashFlow
	
	// Generate some dummy data across the date range
	current := fromDate
	for current.Before(toDate) {
		// Random chance to have an event on this day
		if rand.Float32() < 0.2 { // 20% chance
			amt := automation.MustParseExact(fmt.Sprintf("%d.00", rand.Intn(5000)+100))
			if r.isOutflow {
				amt = amt.Mul(automation.MustParseExact("-1"))
			}

			flows = append(flows, ExpectedCashFlow{
				SourceType: r.sourceType,
				SourceRef:  fmt.Sprintf("%s-dummy-%d", r.sourceType, current.Unix()),
				Amount:     amt,
				Currency:   "USD", // Hardcoded currency for dummy
				Date:       current,
				Certainty:  CertaintyProbable,
			})
		}
		current = current.Add(24 * time.Hour)
	}

	return flows, nil
}
