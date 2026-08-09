package freight

import (
	"context"
	"testing"
	"time"

	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
)

// ═══════════════════════════════════════════════════════════════════════════
// FREIGHT SERVICE INTEGRATION TESTS
// ═══════════════════════════════════════════════════════════════════════════

func TestCalculateFreightCharge_BasicCalculation(t *testing.T) {
	repo := NewMockRepository()
	svc := NewFreightService(repo)
	ctx := context.Background()

	// Create rate card
	baseRate := accountingmoney.Must("500.00", 2)
	perKgRate := accountingmoney.Must("0.25", 2)
	perCbmRate := accountingmoney.Must("2.00", 2)

	rateInput := CreateRateCardInput{
		CompanyID:          1,
		OriginCity:         "New York",
		OriginCountry:      "USA",
		DestinationCity:    "Los Angeles",
		DestinationCountry: "USA",
		ServiceLevel:       ServiceLevelStandard,
		BaseRate:           baseRate,
		PerKgRate:          &perKgRate,
		PerCbmRate:         &perCbmRate,
		Currency:           "USD",
		EffectiveDate:      time.Now(),
		CreatedBy:          1,
	}

	rateCard, err := svc.CreateRateCard(ctx, rateInput)
	if err != nil {
		t.Fatalf("Failed to create rate card: %v", err)
	}
	if rateCard == nil {
		t.Fatal("Rate card is nil")
	}

	// Calculate freight
	weight := accountingmoney.Must("1000", 2)
	volume := accountingmoney.Must("50", 2)

	calcInput := CalculateFreightInput{
		CompanyID:       1,
		OriginCity:      "New York",
		DestinationCity: "Los Angeles",
		ServiceLevel:    ServiceLevelStandard,
		WeightKg:        &weight,
		VolumeCbm:       &volume,
	}

	charge, err := svc.CalculateAndCreateFreightCharge(ctx, calcInput)
	if err != nil {
		t.Fatalf("Failed to calculate freight: %v", err)
	}

	if charge == nil {
		t.Fatal("Freight charge is nil")
	}

	if charge.Status != FreightChargeStatusCalculated {
		t.Errorf("Expected status CALCULATED, got %v", charge.Status)
	}

	// Verify: base(500) + weight(250) + volume(100) = 850
	expected := accountingmoney.Must("850.00", 2)
	if charge.FreightTotal.Amount != expected.Amount {
		t.Errorf("Expected freight total %s, got %s", expected.Amount, charge.FreightTotal.Amount)
	}
}

func TestStatusTransition_CalculatedToInvoiced(t *testing.T) {
	repo := NewMockRepository()
	svc := NewFreightService(repo)
	ctx := context.Background()

	// Create rate card and charge
	baseRate := accountingmoney.Must("500.00", 2)
	rateInput := CreateRateCardInput{
		CompanyID:          1,
		OriginCity:         "New York",
		OriginCountry:      "USA",
		DestinationCity:    "Los Angeles",
		DestinationCountry: "USA",
		ServiceLevel:       ServiceLevelStandard,
		BaseRate:           baseRate,
		Currency:           "USD",
		EffectiveDate:      time.Now(),
		CreatedBy:          1,
	}

	_, _ = svc.CreateRateCard(ctx, rateInput)

	calcInput := CalculateFreightInput{
		CompanyID:       1,
		OriginCity:      "New York",
		DestinationCity: "Los Angeles",
		ServiceLevel:    ServiceLevelStandard,
	}

	charge, _ := svc.CalculateAndCreateFreightCharge(ctx, calcInput)

	// Mark as invoiced
	updated, err := svc.UpdateFreightChargeInvoice(ctx, 1, charge.ID, "INV-001", time.Now())
	if err != nil {
		t.Fatalf("Failed to mark as invoiced: %v", err)
	}

	if updated.Status != FreightChargeStatusInvoiced {
		t.Errorf("Expected status INVOICED, got %v", updated.Status)
	}

	if updated.InvoiceNumber == nil || *updated.InvoiceNumber != "INV-001" {
		t.Errorf("Expected invoice number INV-001")
	}
}

func TestStatusTransition_InvoicedToPaid(t *testing.T) {
	repo := NewMockRepository()
	svc := NewFreightService(repo)
	ctx := context.Background()

	// Setup
	baseRate := accountingmoney.Must("500.00", 2)
	rateInput := CreateRateCardInput{
		CompanyID:          1,
		OriginCity:         "New York",
		OriginCountry:      "USA",
		DestinationCity:    "Los Angeles",
		DestinationCountry: "USA",
		ServiceLevel:       ServiceLevelStandard,
		BaseRate:           baseRate,
		Currency:           "USD",
		EffectiveDate:      time.Now(),
		CreatedBy:          1,
	}

	_, _ = svc.CreateRateCard(ctx, rateInput)

	calcInput := CalculateFreightInput{
		CompanyID:       1,
		OriginCity:      "New York",
		DestinationCity: "Los Angeles",
		ServiceLevel:    ServiceLevelStandard,
	}

	charge, _ := svc.CalculateAndCreateFreightCharge(ctx, calcInput)
	_, _ = svc.UpdateFreightChargeInvoice(ctx, 1, charge.ID, "INV-001", time.Now())

	// Mark as paid
	updated, err := svc.MarkFreightChargePaid(ctx, 1, charge.ID)
	if err != nil {
		t.Fatalf("Failed to mark as paid: %v", err)
	}

	if updated.Status != FreightChargeStatusPaid {
		t.Errorf("Expected status PAID, got %v", updated.Status)
	}
}

func TestStatusTransitionsDoNotMoveChargesBackwards(t *testing.T) {
	repo := NewMockRepository()
	svc := NewFreightService(repo)
	ctx := context.Background()
	baseRate := accountingmoney.Must("100.00", 2)
	_, err := svc.CreateRateCard(ctx, CreateRateCardInput{
		CompanyID:       1,
		OriginCity:      "A",
		DestinationCity: "B",
		ServiceLevel:    ServiceLevelStandard,
		BaseRate:        baseRate,
		Currency:        "USD",
		EffectiveDate:   time.Now(),
	})
	if err != nil {
		t.Fatalf("CreateRateCard failed: %v", err)
	}
	charge, err := svc.CalculateAndCreateFreightCharge(ctx, CalculateFreightInput{
		CompanyID:       1,
		OriginCity:      "A",
		DestinationCity: "B",
		ServiceLevel:    ServiceLevelStandard,
	})
	if err != nil {
		t.Fatalf("CalculateAndCreateFreightCharge failed: %v", err)
	}
	if _, err := svc.MarkFreightChargePaid(ctx, 1, charge.ID); err == nil {
		t.Fatal("expected payment before invoicing to fail")
	}
	if _, err := svc.UpdateFreightChargeInvoice(ctx, 1, charge.ID, "", time.Time{}); err == nil {
		t.Fatal("expected empty invoice number to fail")
	}
}

func TestRateCardValidation_WeightBounds(t *testing.T) {
	repo := NewMockRepository()
	calc := NewRateCalculator(repo)

	// Create rate card with weight bounds
	minWeight := accountingmoney.Must("100", 2)
	maxWeight := accountingmoney.Must("5000", 2)

	rateCard := &RateCard{
		ID:          1,
		BaseRate:    accountingmoney.Must("500.00", 2),
		MinWeightKg: &minWeight,
		MaxWeightKg: &maxWeight,
	}

	// Test valid weight
	validWeight := accountingmoney.Must("2500", 2)
	err := calc.ValidateRateCard(rateCard, &validWeight, nil)
	if err != nil {
		t.Errorf("Valid weight rejected: %v", err)
	}

	// Test underweight
	underWeight := accountingmoney.Must("50", 2)
	err = calc.ValidateRateCard(rateCard, &underWeight, nil)
	if err == nil {
		t.Error("Expected error for underweight, got nil")
	}

	// Test overweight
	overWeight := accountingmoney.Must("10000", 2)
	err = calc.ValidateRateCard(rateCard, &overWeight, nil)
	if err == nil {
		t.Error("Expected error for overweight, got nil")
	}
}

func TestCostCenterCreation(t *testing.T) {
	repo := NewMockRepository()
	svc := NewFreightService(repo)
	ctx := context.Background()

	// Create cost center
	ccInput := CreateCostCenterInput{
		CostCenterCode: "WH-NYC",
		CostCenterName: "NYC Warehouse",
		CostCenterType: CostCenterTypeWarehouse,
		GLAccount:      moneyToStrPtr("6100"),
	}

	costCenter, err := svc.CreateCostCenter(ctx, 1, ccInput)
	if err != nil {
		t.Fatalf("Failed to create cost center: %v", err)
	}

	if costCenter.CostCenterCode != "WH-NYC" {
		t.Errorf("Expected cost center code WH-NYC, got %s", costCenter.CostCenterCode)
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// HELPER FUNCTIONS
// ═══════════════════════════════════════════════════════════════════════════

func moneyToStrPtr(s string) *string {
	return &s
}
