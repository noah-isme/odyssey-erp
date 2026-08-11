package distribution

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
)

func TestOptimizeRouteUsesStableNearestNeighborOrderAndPersistsMetrics(t *testing.T) {
	repo := newFakeDistributionRepository()
	const routeID = int64(11)
	repo.routes[routeID] = &DeliveryRoute{ID: routeID, CompanyID: 7, Status: RouteStatusDraft}
	repo.stops[routeID] = []*RouteStop{
		optimizationStop(102, routeID, 7, 2, 0, 2),
		optimizationStop(104, routeID, 7, 4, 0, 1.1),
		optimizationStop(101, routeID, 7, 1, 0, 0),
		optimizationStop(103, routeID, 7, 3, 0, 1),
	}

	if err := NewServiceWithDependencies(repo, Dependencies{}).OptimizeRoute(context.Background(), routeID); err != nil {
		t.Fatalf("optimize route: %v", err)
	}

	if got, want := optimizedStopOrder(repo.stops[routeID]), []int64{101, 103, 104, 102}; !equalInt64s(got, want) {
		t.Fatalf("optimized order=%v want %v", got, want)
	}
	if got := repo.routes[routeID].Status; got != RouteStatusOptimized {
		t.Fatalf("route status=%s want %s", got, RouteStatusOptimized)
	}
	if distance := *repo.routes[routeID].TotalDistanceKm; distance < 222 || distance > 223 {
		t.Fatalf("route distance=%v want roughly 222.39 km", distance)
	}
	if got := *repo.routes[routeID].EstimatedDurationMinutes; got != 334 {
		t.Fatalf("estimated duration=%d want 334 minutes", got)
	}
	if score := moneyFloat(*repo.routes[routeID].OptimizationScore); math.Abs(score-100) > 0.001 {
		t.Fatalf("optimization score=%v want 100", score)
	}
}

func TestOptimizeRoutePreservesManualOrderForEqualNearestStops(t *testing.T) {
	repo := newFakeDistributionRepository()
	const routeID = int64(12)
	repo.routes[routeID] = &DeliveryRoute{ID: routeID, CompanyID: 7, Status: RouteStatusDraft}
	repo.stops[routeID] = []*RouteStop{
		optimizationStop(123, routeID, 7, 3, -1, 0),
		optimizationStop(121, routeID, 7, 1, 0, 0),
		optimizationStop(122, routeID, 7, 2, 1, 0),
	}

	if err := NewServiceWithDependencies(repo, Dependencies{}).OptimizeRoute(context.Background(), routeID); err != nil {
		t.Fatalf("optimize route: %v", err)
	}

	if got, want := optimizedStopOrder(repo.stops[routeID]), []int64{121, 122, 123}; !equalInt64s(got, want) {
		t.Fatalf("optimized order=%v want stable tie order %v", got, want)
	}
}

func TestOptimizeRouteUsesDeterministicFallbackWhenCoordinatesAreMissing(t *testing.T) {
	repo := newFakeDistributionRepository()
	const routeID = int64(13)
	repo.routes[routeID] = &DeliveryRoute{ID: routeID, CompanyID: 7, Status: RouteStatusDraft}
	repo.stops[routeID] = []*RouteStop{
		optimizationStop(134, routeID, 7, 4, 0, 2),
		optimizationStopWithoutCoordinates(132, routeID, 7, 2),
		optimizationStop(131, routeID, 7, 1, 0, 0),
		optimizationStop(133, routeID, 7, 3, 0, 1),
	}

	if err := NewServiceWithDependencies(repo, Dependencies{}).OptimizeRoute(context.Background(), routeID); err != nil {
		t.Fatalf("optimize route: %v", err)
	}

	if got, want := optimizedStopOrder(repo.stops[routeID]), []int64{131, 132, 133, 134}; !equalInt64s(got, want) {
		t.Fatalf("fallback order=%v want manual order %v", got, want)
	}
	if distance := *repo.routes[routeID].TotalDistanceKm; distance < 111 || distance > 112 {
		t.Fatalf("fallback distance=%v want known-leg distance roughly 111.20 km", distance)
	}
	if got := *repo.routes[routeID].EstimatedDurationMinutes; got != 167 {
		t.Fatalf("fallback estimated duration=%d want 167 minutes", got)
	}
	if score := moneyFloat(*repo.routes[routeID].OptimizationScore); math.Abs(score-33.33) > 0.001 {
		t.Fatalf("fallback score=%v want 33.33 known-leg coverage", score)
	}
}

func TestOptimizeRouteRejectsInvalidAndEmptyRoutes(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid route id", func(t *testing.T) {
		if err := (&Service{}).OptimizeRoute(ctx, 0); err == nil || !strings.Contains(err.Error(), "route ID is required") {
			t.Fatalf("error=%v want route ID validation", err)
		}
	})

	t.Run("missing route", func(t *testing.T) {
		repo := newFakeDistributionRepository()
		err := NewServiceWithDependencies(repo, Dependencies{}).OptimizeRoute(ctx, 20)
		if err == nil || !strings.Contains(err.Error(), "route not found") {
			t.Fatalf("error=%v want route not found", err)
		}
	})

	t.Run("non draft route", func(t *testing.T) {
		repo := newFakeDistributionRepository()
		repo.routes[21] = &DeliveryRoute{ID: 21, CompanyID: 7, Status: RouteStatusOptimized}
		err := NewServiceWithDependencies(repo, Dependencies{}).OptimizeRoute(ctx, 21)
		if err == nil || !strings.Contains(err.Error(), "can only optimize DRAFT routes") {
			t.Fatalf("error=%v want lifecycle guard", err)
		}
	})

	t.Run("empty route", func(t *testing.T) {
		repo := newFakeDistributionRepository()
		repo.routes[22] = &DeliveryRoute{ID: 22, CompanyID: 7, Status: RouteStatusDraft}
		err := NewServiceWithDependencies(repo, Dependencies{}).OptimizeRoute(ctx, 22)
		if err == nil || !strings.Contains(err.Error(), "without stops") {
			t.Fatalf("error=%v want empty route validation", err)
		}
	})

	t.Run("invalid stop", func(t *testing.T) {
		repo := newFakeDistributionRepository()
		repo.routes[23] = &DeliveryRoute{ID: 23, CompanyID: 7, Status: RouteStatusDraft}
		repo.stops[23] = []*RouteStop{optimizationStop(231, 23, 7, 0, 0, 0)}
		err := NewServiceWithDependencies(repo, Dependencies{}).OptimizeRoute(ctx, 23)
		if err == nil || !strings.Contains(err.Error(), "invalid sequence") {
			t.Fatalf("error=%v want invalid stop validation", err)
		}
	})
}

func TestOptimizeRouteDoesNotPartiallyUpdateWhenRepositoryFails(t *testing.T) {
	repo := newFakeDistributionRepository()
	const routeID = int64(24)
	repo.routes[routeID] = &DeliveryRoute{ID: routeID, CompanyID: 7, Status: RouteStatusDraft}
	repo.stops[routeID] = []*RouteStop{
		optimizationStop(242, routeID, 7, 2, 0, 2),
		optimizationStop(241, routeID, 7, 1, 0, 0),
	}
	repo.routeOptimizationErr = errors.New("injected update failure")

	err := NewServiceWithDependencies(repo, Dependencies{}).OptimizeRoute(context.Background(), routeID)
	if err == nil || !strings.Contains(err.Error(), "failed to persist route optimization") {
		t.Fatalf("error=%v want update failure", err)
	}
	if got := repo.routes[routeID].Status; got != RouteStatusDraft {
		t.Fatalf("route status=%s changed after failed update", got)
	}
	if got, want := optimizedStopOrder(repo.stops[routeID]), []int64{241, 242}; !equalInt64s(got, want) {
		t.Fatalf("stop order=%v changed after failed update; want original sequence order %v", got, want)
	}
	if repo.routes[routeID].TotalDistanceKm != nil || repo.routes[routeID].EstimatedDurationMinutes != nil || repo.routes[routeID].OptimizationScore != nil {
		t.Fatalf("route metrics changed after failed update: %+v", repo.routes[routeID])
	}
}

func optimizationStop(id, routeID, companyID int64, sequence int, latitude, longitude float64) *RouteStop {
	return &RouteStop{
		ID:           id,
		RouteID:      routeID,
		CompanyID:    companyID,
		StopSequence: sequence,
		StopType:     StopTypeCustomer,
		LocationLat:  &latitude,
		LocationLon:  &longitude,
	}
}

func optimizationStopWithoutCoordinates(id, routeID, companyID int64, sequence int) *RouteStop {
	return &RouteStop{
		ID:           id,
		RouteID:      routeID,
		CompanyID:    companyID,
		StopSequence: sequence,
		StopType:     StopTypeCustomer,
	}
}

func optimizedStopOrder(stops []*RouteStop) []int64 {
	ordered := append([]*RouteStop(nil), stops...)
	for i := 0; i < len(ordered); i++ {
		for j := i + 1; j < len(ordered); j++ {
			if ordered[j].StopSequence < ordered[i].StopSequence {
				ordered[i], ordered[j] = ordered[j], ordered[i]
			}
		}
	}
	result := make([]int64, len(ordered))
	for i, stop := range ordered {
		result[i] = stop.ID
	}
	return result
}

func equalInt64s(left, right []int64) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
