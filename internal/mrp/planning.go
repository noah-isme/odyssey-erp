package mrp

import (
	"errors"
	"math"
	"sort"
	"time"
)

var ErrInvalidPlanningInput = errors.New("mrp: invalid planning input")

type PlanningOrderType string

const (
	PlanningOrderBuy  PlanningOrderType = "BUY"
	PlanningOrderMake PlanningOrderType = "MAKE"
)

type LotSizingRule string

const (
	LotForLot   LotSizingRule = "LOT_FOR_LOT"
	LotMinimum  LotSizingRule = "MINIMUM"
	LotFixed    LotSizingRule = "FIXED"
	LotMultiple LotSizingRule = "MULTIPLE"
)

type PlanningKey struct {
	ProductID   int64
	WarehouseID int64
}

type PlanningDemand struct {
	PlanningKey
	DueDate   time.Time
	Quantity  float64
	SourceRef string
}

type PlanningSupply struct {
	PlanningKey
	AvailableDate time.Time
	Quantity      float64
	SourceRef     string
}

type PlanningPolicy struct {
	PlanningKey
	OrderType   PlanningOrderType
	LeadDays    int
	SafetyStock float64
	LotSizing   LotSizingRule
	LotQuantity float64
}

type PlanningInput struct {
	AsOf     time.Time
	Demands  []PlanningDemand
	Supplies []PlanningSupply
	Policies []PlanningPolicy
}

type PlanningRecommendation struct {
	PlanningKey
	OrderType       PlanningOrderType
	Quantity        float64
	ReleaseDate     time.Time
	DueDate         time.Time
	DemandSourceRef string
	Late            bool
}

// Plan nets dated demand against available supply. It is deliberately pure so
// persistence, asynchronous execution, and approval workflows cannot change
// the result of a planning run for the same input snapshot.
func Plan(input PlanningInput) ([]PlanningRecommendation, error) {
	asOf := planningDay(input.AsOf)
	if asOf.IsZero() {
		return nil, ErrInvalidPlanningInput
	}

	policies := make(map[PlanningKey]PlanningPolicy, len(input.Policies))
	for _, policy := range input.Policies {
		if policy.ProductID <= 0 || policy.WarehouseID <= 0 || policy.LeadDays < 0 || policy.SafetyStock < 0 || policy.OrderType == "" || policy.LotSizing == "" || policy.LotQuantity < 0 {
			return nil, ErrInvalidPlanningInput
		}
		if (policy.LotSizing == LotMinimum || policy.LotSizing == LotFixed || policy.LotSizing == LotMultiple) && policy.LotQuantity <= 0 {
			return nil, ErrInvalidPlanningInput
		}
		if _, exists := policies[policy.PlanningKey]; exists {
			return nil, ErrInvalidPlanningInput
		}
		policies[policy.PlanningKey] = policy
	}

	demands := make(map[PlanningKey][]PlanningDemand)
	for _, demand := range input.Demands {
		if demand.ProductID <= 0 || demand.WarehouseID <= 0 || demand.Quantity <= 0 || planningDay(demand.DueDate).IsZero() {
			return nil, ErrInvalidPlanningInput
		}
		if _, exists := policies[demand.PlanningKey]; !exists {
			return nil, ErrInvalidPlanningInput
		}
		demand.DueDate = planningDay(demand.DueDate)
		demands[demand.PlanningKey] = append(demands[demand.PlanningKey], demand)
	}

	supplies := make(map[PlanningKey][]PlanningSupply)
	for _, supply := range input.Supplies {
		if supply.ProductID <= 0 || supply.WarehouseID <= 0 || supply.Quantity <= 0 {
			return nil, ErrInvalidPlanningInput
		}
		if _, exists := policies[supply.PlanningKey]; !exists {
			return nil, ErrInvalidPlanningInput
		}
		if supply.AvailableDate.IsZero() {
			supply.AvailableDate = asOf
		} else {
			supply.AvailableDate = planningDay(supply.AvailableDate)
		}
		supplies[supply.PlanningKey] = append(supplies[supply.PlanningKey], supply)
	}

	keys := make([]PlanningKey, 0, len(demands))
	for key := range demands {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].ProductID == keys[j].ProductID {
			return keys[i].WarehouseID < keys[j].WarehouseID
		}
		return keys[i].ProductID < keys[j].ProductID
	})

	recommendations := make([]PlanningRecommendation, 0)
	for _, key := range keys {
		demandRows := demands[key]
		supplyRows := supplies[key]
		sort.Slice(demandRows, func(i, j int) bool {
			if demandRows[i].DueDate.Equal(demandRows[j].DueDate) {
				return demandRows[i].SourceRef < demandRows[j].SourceRef
			}
			return demandRows[i].DueDate.Before(demandRows[j].DueDate)
		})
		sort.Slice(supplyRows, func(i, j int) bool {
			if supplyRows[i].AvailableDate.Equal(supplyRows[j].AvailableDate) {
				return supplyRows[i].SourceRef < supplyRows[j].SourceRef
			}
			return supplyRows[i].AvailableDate.Before(supplyRows[j].AvailableDate)
		})

		available, nextSupply := 0.0, 0
		policy := policies[key]
		for _, demand := range demandRows {
			for nextSupply < len(supplyRows) && !supplyRows[nextSupply].AvailableDate.After(demand.DueDate) {
				available += supplyRows[nextSupply].Quantity
				nextSupply++
			}
			available -= demand.Quantity
			required := policy.SafetyStock - available
			if required <= 0 {
				continue
			}
			quantity := lotSizedQuantity(required, policy)
			available += quantity
			releaseDate := demand.DueDate.AddDate(0, 0, -policy.LeadDays)
			recommendations = append(recommendations, PlanningRecommendation{
				PlanningKey:     key,
				OrderType:       policy.OrderType,
				Quantity:        quantity,
				ReleaseDate:     releaseDate,
				DueDate:         demand.DueDate,
				DemandSourceRef: demand.SourceRef,
				Late:            releaseDate.Before(asOf),
			})
		}
	}
	return recommendations, nil
}

func lotSizedQuantity(required float64, policy PlanningPolicy) float64 {
	switch policy.LotSizing {
	case LotMinimum:
		return math.Max(required, policy.LotQuantity)
	case LotFixed:
		return math.Ceil(required/policy.LotQuantity) * policy.LotQuantity
	case LotMultiple:
		return math.Ceil(required/policy.LotQuantity) * policy.LotQuantity
	default:
		return required
	}
}

func planningDay(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}
