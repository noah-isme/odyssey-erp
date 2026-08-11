package distribution

import (
	"fmt"
	"math"
	"sort"

	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
)

const (
	routeEarthRadiusKm       = 6371.0088
	routeAverageSpeedKmh     = 40.0
	routeDistanceTieEpsilon  = 1e-9
	routeMetricDecimalPlaces = 2
)

// routeOptimizationPlan is calculated without external routing providers.
// Coordinates use a haversine great-circle distance as the v1 straight-line
// estimate. If any stop has a missing coordinate pair, the deterministic
// fallback is the existing manual order (stop_sequence, then stop ID); no
// unlocated stop is placed by guessing its position. Legs whose two endpoints
// have coordinates still contribute to distance, and the score is the
// percentage of route legs with a known straight-line distance.
type routeOptimizationPlan struct {
	OrderedStopIDs           []int64
	TotalDistanceKm          float64
	EstimatedDurationMinutes int
	OptimizationScore        accountingmoney.Money
}

func buildRouteOptimizationPlan(routeID, companyID int64, stops []*RouteStop) (*routeOptimizationPlan, error) {
	if routeID <= 0 {
		return nil, fmt.Errorf("route ID is required")
	}
	if len(stops) == 0 {
		return nil, fmt.Errorf("cannot optimize a route without stops")
	}

	manualOrder := append([]*RouteStop(nil), stops...)
	seenIDs := make(map[int64]struct{}, len(manualOrder))
	for _, stop := range manualOrder {
		if stop == nil {
			return nil, fmt.Errorf("route contains a nil stop")
		}
		if stop.ID <= 0 {
			return nil, fmt.Errorf("route stop ID must be positive")
		}
		if _, ok := seenIDs[stop.ID]; ok {
			return nil, fmt.Errorf("route stop IDs must be unique")
		}
		seenIDs[stop.ID] = struct{}{}
		if stop.RouteID != 0 && stop.RouteID != routeID {
			return nil, fmt.Errorf("route stop %d belongs to route %d", stop.ID, stop.RouteID)
		}
		if companyID != 0 && stop.CompanyID != 0 && stop.CompanyID != companyID {
			return nil, fmt.Errorf("route stop %d belongs to company %d", stop.ID, stop.CompanyID)
		}
		if stop.StopSequence <= 0 {
			return nil, fmt.Errorf("route stop %d has invalid sequence", stop.ID)
		}
		if hasAnyCoordinate(stop) && !hasValidCoordinates(stop) {
			return nil, fmt.Errorf("route stop %d has invalid coordinates", stop.ID)
		}
	}

	sort.SliceStable(manualOrder, func(i, j int) bool {
		if manualOrder[i].StopSequence != manualOrder[j].StopSequence {
			return manualOrder[i].StopSequence < manualOrder[j].StopSequence
		}
		return manualOrder[i].ID < manualOrder[j].ID
	})

	ordered := manualOrder
	if allStopsHaveCoordinates(manualOrder) {
		ordered = stableNearestNeighborOrder(manualOrder)
	}

	totalDistance, knownLegs := routeDistance(ordered)
	totalDistance = roundRouteMetric(totalDistance)
	estimatedDuration := 0
	if totalDistance > 0 {
		estimatedDuration = int(math.Ceil(totalDistance / routeAverageSpeedKmh * 60))
	}

	score := 100.0
	if len(ordered) > 1 {
		score = float64(knownLegs) / float64(len(ordered)-1) * 100
	}
	score = roundRouteMetric(score)

	orderedIDs := make([]int64, len(ordered))
	for i, stop := range ordered {
		orderedIDs[i] = stop.ID
	}

	return &routeOptimizationPlan{
		OrderedStopIDs:           orderedIDs,
		TotalDistanceKm:          totalDistance,
		EstimatedDurationMinutes: estimatedDuration,
		OptimizationScore:        moneyFromFloat(score, routeMetricDecimalPlaces),
	}, nil
}

func stableNearestNeighborOrder(manualOrder []*RouteStop) []*RouteStop {
	if len(manualOrder) < 2 {
		return append([]*RouteStop(nil), manualOrder...)
	}

	ordered := make([]*RouteStop, 0, len(manualOrder))
	ordered = append(ordered, manualOrder[0])
	remaining := append([]*RouteStop(nil), manualOrder[1:]...)
	for len(remaining) > 0 {
		current := ordered[len(ordered)-1]
		bestIndex := 0
		bestDistance := math.Inf(1)
		for i, candidate := range remaining {
			distance, _ := straightLineDistanceKm(current, candidate)
			// remaining retains manual order, so strict improvement preserves
			// the original order for equal or near-equal distances.
			if distance < bestDistance-routeDistanceTieEpsilon {
				bestIndex = i
				bestDistance = distance
			}
		}
		ordered = append(ordered, remaining[bestIndex])
		remaining = append(remaining[:bestIndex], remaining[bestIndex+1:]...)
	}
	return ordered
}

func routeDistance(stops []*RouteStop) (float64, int) {
	var total float64
	knownLegs := 0
	for i := 1; i < len(stops); i++ {
		distance, known := straightLineDistanceKm(stops[i-1], stops[i])
		if !known {
			continue
		}
		total += distance
		knownLegs++
	}
	return total, knownLegs
}

func straightLineDistanceKm(first, second *RouteStop) (float64, bool) {
	if !hasValidCoordinates(first) || !hasValidCoordinates(second) {
		return 0, false
	}

	latitude1 := degreesToRadians(*first.LocationLat)
	latitude2 := degreesToRadians(*second.LocationLat)
	deltaLatitude := degreesToRadians(*second.LocationLat - *first.LocationLat)
	deltaLongitude := degreesToRadians(*second.LocationLon - *first.LocationLon)

	haversine := math.Sin(deltaLatitude/2)*math.Sin(deltaLatitude/2) +
		math.Cos(latitude1)*math.Cos(latitude2)*
			math.Sin(deltaLongitude/2)*math.Sin(deltaLongitude/2)
	// Floating-point rounding can put the value just outside [0, 1].
	haversine = math.Max(0, math.Min(1, haversine))
	return routeEarthRadiusKm * 2 * math.Atan2(math.Sqrt(haversine), math.Sqrt(1-haversine)), true
}

func allStopsHaveCoordinates(stops []*RouteStop) bool {
	for _, stop := range stops {
		if !hasValidCoordinates(stop) {
			return false
		}
	}
	return true
}

func hasAnyCoordinate(stop *RouteStop) bool {
	return stop != nil && (stop.LocationLat != nil || stop.LocationLon != nil)
}

func hasValidCoordinates(stop *RouteStop) bool {
	if stop == nil || stop.LocationLat == nil || stop.LocationLon == nil {
		return false
	}
	return !math.IsNaN(*stop.LocationLat) && !math.IsInf(*stop.LocationLat, 0) &&
		!math.IsNaN(*stop.LocationLon) && !math.IsInf(*stop.LocationLon, 0) &&
		*stop.LocationLat >= -90 && *stop.LocationLat <= 90 &&
		*stop.LocationLon >= -180 && *stop.LocationLon <= 180
}

func degreesToRadians(value float64) float64 {
	return value * math.Pi / 180
}

func roundRouteMetric(value float64) float64 {
	factor := math.Pow10(routeMetricDecimalPlaces)
	return math.Round(value*factor) / factor
}
