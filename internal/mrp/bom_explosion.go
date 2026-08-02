package mrp

import (
	"fmt"
	"time"
)

// PlanningBOM is the effective manufacturing recipe used only to derive
// dependent demand. Work orders still retain their own released snapshot.
type PlanningBOM struct {
	ProductID int64
	ScrapPct  float64
	Lines     []PlanningBOMLine
}

type PlanningBOMLine struct {
	ComponentProductID int64
	Quantity           float64
	ScrapPct           float64
}

// ExplodeBOMDemand adds dependent material demand for MAKE items. Components
// are needed on the parent's release date, and carry the originating demand
// reference so planners can trace every recommendation back to its source.
func ExplodeBOMDemand(input PlanningInput, boms map[int64]PlanningBOM) (PlanningInput, error) {
	policies := make(map[PlanningKey]PlanningPolicy, len(input.Policies))
	for _, policy := range input.Policies {
		if !validPlanningPolicy(policy) {
			return PlanningInput{}, ErrInvalidPlanningInput
		}
		policies[policy.PlanningKey] = policy
	}
	output := input
	output.Demands = make([]PlanningDemand, 0, len(input.Demands))
	for _, root := range input.Demands {
		if err := explodeDemand(root, policies, boms, nil, &output.Demands); err != nil {
			return PlanningInput{}, err
		}
	}
	return output, nil
}

func explodeDemand(demand PlanningDemand, policies map[PlanningKey]PlanningPolicy, boms map[int64]PlanningBOM, path map[int64]bool, output *[]PlanningDemand) error {
	policy, ok := policies[demand.PlanningKey]
	if !ok {
		return fmt.Errorf("%w: missing planning policy for product %d and warehouse %d", ErrInvalidPlanningInput, demand.ProductID, demand.WarehouseID)
	}
	if demand.Quantity <= 0 || demand.DueDate.IsZero() {
		return ErrInvalidPlanningInput
	}
	*output = append(*output, demand)
	if policy.OrderType != PlanningOrderMake {
		return nil
	}
	bom, ok := boms[demand.ProductID]
	if !ok {
		return nil
	}
	if path != nil && path[demand.ProductID] {
		return fmt.Errorf("%w: BOM cycle at product %d", ErrInvalidPlanningInput, demand.ProductID)
	}
	nextPath := make(map[int64]bool, len(path)+1)
	for productID := range path {
		nextPath[productID] = true
	}
	nextPath[demand.ProductID] = true
	componentDueDate := planningDay(demand.DueDate.AddDate(0, 0, -policy.LeadDays))
	for _, line := range bom.Lines {
		if line.ComponentProductID <= 0 || line.Quantity <= 0 || line.ScrapPct < 0 || line.ScrapPct > 100 {
			return ErrInvalidPlanningInput
		}
		component := PlanningDemand{
			PlanningKey: PlanningKey{ProductID: line.ComponentProductID, WarehouseID: demand.WarehouseID},
			DueDate:     componentDueDate,
			Quantity:    demand.Quantity * line.Quantity * (1 + bom.ScrapPct/100) * (1 + line.ScrapPct/100),
			SourceRef:   fmt.Sprintf("%s->BOM-%d", demand.SourceRef, demand.ProductID),
		}
		if err := explodeDemand(component, policies, boms, nextPath, output); err != nil {
			return err
		}
	}
	return nil
}

func planningDemand(productID, warehouseID int64, dueDate time.Time, quantity float64, sourceRef string) PlanningDemand {
	return PlanningDemand{PlanningKey: PlanningKey{ProductID: productID, WarehouseID: warehouseID}, DueDate: dueDate, Quantity: quantity, SourceRef: sourceRef}
}
