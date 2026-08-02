package mrp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestExplodeBOMDemandUsesParentReleaseDateAndScrap(t *testing.T) {
	date := time.Date(2026, time.August, 20, 0, 0, 0, 0, time.UTC)
	input := PlanningInput{
		AsOf: date,
		Policies: []PlanningPolicy{
			{PlanningKey: PlanningKey{ProductID: 100, WarehouseID: 1}, OrderType: PlanningOrderMake, LeadDays: 3, LotSizing: LotForLot},
			{PlanningKey: PlanningKey{ProductID: 200, WarehouseID: 1}, OrderType: PlanningOrderBuy, LotSizing: LotForLot},
		},
		Demands: []PlanningDemand{planningDemand(100, 1, date, 10, "SO-LINE-1")},
	}
	exploded, err := ExplodeBOMDemand(input, map[int64]PlanningBOM{
		100: {ProductID: 100, ScrapPct: 5, Lines: []PlanningBOMLine{{ComponentProductID: 200, Quantity: 2, ScrapPct: 10}}},
	})
	require.NoError(t, err)
	require.Len(t, exploded.Demands, 2)
	require.Equal(t, "2026-08-17", exploded.Demands[1].DueDate.Format(time.DateOnly))
	require.InDelta(t, 23.1, exploded.Demands[1].Quantity, 0.000001)
	require.Equal(t, "SO-LINE-1->BOM-100", exploded.Demands[1].SourceRef)
}

func TestExplodeBOMDemandRejectsCycles(t *testing.T) {
	input := PlanningInput{
		AsOf: time.Now(),
		Policies: []PlanningPolicy{
			{PlanningKey: PlanningKey{ProductID: 1, WarehouseID: 1}, OrderType: PlanningOrderMake, LotSizing: LotForLot},
			{PlanningKey: PlanningKey{ProductID: 2, WarehouseID: 1}, OrderType: PlanningOrderMake, LotSizing: LotForLot},
		},
		Demands: []PlanningDemand{planningDemand(1, 1, time.Now(), 1, "SO-LINE-1")},
	}
	_, err := ExplodeBOMDemand(input, map[int64]PlanningBOM{
		1: {ProductID: 1, Lines: []PlanningBOMLine{{ComponentProductID: 2, Quantity: 1}}},
		2: {ProductID: 2, Lines: []PlanningBOMLine{{ComponentProductID: 1, Quantity: 1}}},
	})
	require.ErrorIs(t, err, ErrInvalidPlanningInput)
}
