package cmms

import (
	"context"
	"math"
	"strings"
	"testing"
)

type predictiveFakeRepository struct {
	*Repository
	anomalies []PredictiveAnomaly
	alerts    []PredictiveAlert
	nextID    int64
}

var _ ServiceRepository = (*predictiveFakeRepository)(nil)

func (r *predictiveFakeRepository) CreatePredictiveAlert(_ context.Context, alert PredictiveAlert) (int64, error) {
	for _, existing := range r.alerts {
		if existing.CompanyID == alert.CompanyID &&
			existing.AssetID == alert.AssetID &&
			predictiveIDEqual(existing.SensorID, alert.SensorID) &&
			predictiveIDEqual(existing.ModelID, alert.ModelID) &&
			existing.ResolvedAt == nil {
			return existing.ID, nil
		}
	}
	r.nextID++
	alert.ID = r.nextID
	r.alerts = append(r.alerts, alert)
	return alert.ID, nil
}

func (r *predictiveFakeRepository) ListPredictiveAnomalies(_ context.Context, companyID int64) ([]PredictiveAnomaly, error) {
	items := make([]PredictiveAnomaly, 0, len(r.anomalies))
	for _, anomaly := range r.anomalies {
		if anomaly.CompanyID != companyID || r.hasOpenAlert(anomaly) {
			continue
		}
		items = append(items, anomaly)
	}
	return items, nil
}

func (r *predictiveFakeRepository) hasOpenAlert(anomaly PredictiveAnomaly) bool {
	for _, alert := range r.alerts {
		if alert.CompanyID == anomaly.CompanyID &&
			alert.AssetID == anomaly.AssetID &&
			predictiveIDEqual(alert.SensorID, &anomaly.SensorID) &&
			predictiveIDEqual(alert.ModelID, &anomaly.ModelID) &&
			alert.ResolvedAt == nil {
			return true
		}
	}
	return false
}

func predictiveIDEqual(left, right *int64) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func TestPredictiveThresholdUsesDefaultForDirectEvaluation(t *testing.T) {
	repo := &predictiveFakeRepository{}
	svc := NewService(repo)
	ctx := context.Background()

	for _, reading := range []float64{999, DefaultPredictiveThreshold} {
		if err := svc.EvaluatePredictiveAlerts(ctx, 1, 2, 3, 4, reading); err != nil {
			t.Fatalf("reading %.1f: %v", reading, err)
		}
	}
	if err := svc.EvaluatePredictiveAlerts(ctx, 1, 2, 3, 4, DefaultPredictiveThreshold+1); err != nil {
		t.Fatalf("above-threshold reading: %v", err)
	}
	if len(repo.alerts) != 1 {
		t.Fatalf("created %d alerts, want 1", len(repo.alerts))
	}
}

func TestPredictiveThresholdRuleVariesByAnomalyForDirectAndBatch(t *testing.T) {
	rule := func(anomaly PredictiveAnomaly) float64 {
		if anomaly.SensorID == 10 {
			return 10
		}
		return 20
	}

	directRepo := &predictiveFakeRepository{}
	direct := NewServiceWithPredictiveThreshold(directRepo, rule)
	ctx := context.Background()
	if err := direct.EvaluatePredictiveAlerts(ctx, 1, 2, 3, 10, 11); err != nil {
		t.Fatalf("direct low-threshold evaluation: %v", err)
	}
	if err := direct.EvaluatePredictiveAlerts(ctx, 1, 2, 3, 11, 11); err != nil {
		t.Fatalf("direct high-threshold evaluation: %v", err)
	}
	if len(directRepo.alerts) != 1 || *directRepo.alerts[0].SensorID != 10 {
		t.Fatalf("direct alerts=%+v, want only sensor 10", directRepo.alerts)
	}

	batchRepo := &predictiveFakeRepository{anomalies: []PredictiveAnomaly{
		{CompanyID: 1, AssetID: 2, SensorID: 10, ModelID: 3, Value: 11},
		{CompanyID: 1, AssetID: 2, SensorID: 11, ModelID: 3, Value: 11},
	}}
	batch := NewServiceWithPredictiveThreshold(batchRepo, rule)
	alerts, err := batch.EvaluatePredictiveAlertsBatch(ctx, 1)
	if err != nil {
		t.Fatalf("batch evaluation: %v", err)
	}
	if len(alerts) != 1 || *alerts[0].SensorID != 10 {
		t.Fatalf("batch alerts=%+v, want only sensor 10", alerts)
	}
}

func TestPredictiveThresholdRejectsNonFiniteReadingAndThreshold(t *testing.T) {
	ctx := context.Background()
	for _, reading := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		repo := &predictiveFakeRepository{}
		err := NewService(repo).EvaluatePredictiveAlerts(ctx, 1, 2, 3, 4, reading)
		if err == nil || !strings.Contains(err.Error(), "reading must be finite") {
			t.Errorf("reading %v error=%v, want finite-reading error", reading, err)
		}
	}

	for _, threshold := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		repo := &predictiveFakeRepository{}
		rule := func(PredictiveAnomaly) float64 { return threshold }
		err := NewServiceWithPredictiveThreshold(repo, rule).EvaluatePredictiveAlerts(ctx, 1, 2, 3, 4, 1001)
		if err == nil || !strings.Contains(err.Error(), "threshold must be finite") {
			t.Errorf("threshold %v error=%v, want finite-threshold error", threshold, err)
		}
	}

	repo := &predictiveFakeRepository{anomalies: []PredictiveAnomaly{{
		CompanyID: 1, AssetID: 2, SensorID: 4, ModelID: 3, Value: math.NaN(),
	}}}
	_, err := NewService(repo).EvaluatePredictiveAlertsBatch(ctx, 1)
	if err == nil || !strings.Contains(err.Error(), "anomaly value must be finite") {
		t.Fatalf("batch non-finite value error=%v, want finite-value error", err)
	}
}

func TestPredictiveBatchThresholdFiltersAndKeepsOpenAlertsIdempotent(t *testing.T) {
	repo := &predictiveFakeRepository{anomalies: []PredictiveAnomaly{
		{CompanyID: 1, AssetID: 2, SensorID: 4, ModelID: 3, Value: 999},
		{CompanyID: 1, AssetID: 2, SensorID: 5, ModelID: 3, Value: 1001},
	}}
	svc := NewService(repo)
	ctx := context.Background()

	first, err := svc.EvaluatePredictiveAlertsBatch(ctx, 1)
	if err != nil {
		t.Fatalf("first batch evaluation: %v", err)
	}
	second, err := svc.EvaluatePredictiveAlertsBatch(ctx, 1)
	if err != nil {
		t.Fatalf("second batch evaluation: %v", err)
	}
	if len(first) != 1 || len(second) != 0 || len(repo.alerts) != 1 {
		t.Fatalf("first=%d second=%d stored=%d, want 1/0/1", len(first), len(second), len(repo.alerts))
	}
}
