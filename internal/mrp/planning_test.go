package mrp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPlanNetsSupplyAppliesSafetyStockAndLotSizing(t *testing.T) {
	asOf := time.Date(2026, time.August, 2, 14, 0, 0, 0, time.FixedZone("WIB", 7*60*60))
	key := PlanningKey{ProductID: 10, WarehouseID: 5}
	recommendations, err := Plan(PlanningInput{
		AsOf:     asOf,
		Policies: []PlanningPolicy{{PlanningKey: key, OrderType: PlanningOrderMake, LeadDays: 3, SafetyStock: 2, LotSizing: LotMultiple, LotQuantity: 5}},
		Supplies: []PlanningSupply{{PlanningKey: key, AvailableDate: asOf, Quantity: 4, SourceRef: "ON-HAND"}},
		Demands:  []PlanningDemand{{PlanningKey: key, DueDate: time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC), Quantity: 10, SourceRef: "SO-100"}},
	})
	require.NoError(t, err)
	require.Len(t, recommendations, 1)
	require.Equal(t, PlanningOrderMake, recommendations[0].OrderType)
	require.Equal(t, 10.0, recommendations[0].Quantity)
	require.Equal(t, "2026-08-07", recommendations[0].ReleaseDate.Format(time.DateOnly))
	require.False(t, recommendations[0].Late)
}

func TestPlanMarksPastDueReleaseAndRejectsMissingPolicy(t *testing.T) {
	key := PlanningKey{ProductID: 10, WarehouseID: 5}
	recommendations, err := Plan(PlanningInput{
		AsOf:     time.Date(2026, time.August, 2, 0, 0, 0, 0, time.UTC),
		Policies: []PlanningPolicy{{PlanningKey: key, OrderType: PlanningOrderBuy, LeadDays: 4, LotSizing: LotForLot}},
		Demands:  []PlanningDemand{{PlanningKey: key, DueDate: time.Date(2026, time.August, 3, 0, 0, 0, 0, time.UTC), Quantity: 2, SourceRef: "SO-101"}},
	})
	require.NoError(t, err)
	require.True(t, recommendations[0].Late)

	_, err = Plan(PlanningInput{AsOf: time.Now(), Demands: []PlanningDemand{{PlanningKey: key, DueDate: time.Now(), Quantity: 1}}})
	require.ErrorIs(t, err, ErrInvalidPlanningInput)
}
