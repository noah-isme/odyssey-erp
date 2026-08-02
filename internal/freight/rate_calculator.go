package freight

import (
	"context"
	"fmt"
	"math/big"
	"time"

	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
)

// ═══════════════════════════════════════════════════════════════════════════
// RATE CALCULATION SERVICE
// ═══════════════════════════════════════════════════════════════════════════

type RateCalculator interface {
	CalculateFreight(ctx context.Context, input CalculateFreightInput) (*CalculateFreightOutput, error)
	CalculateLandedCost(ctx context.Context, input CalculateLandedCostInput) (*LandedCost, error)
	ValidateRateCard(rateCard *RateCard, weight, volume *accountingmoney.Money) error
}

type rateCalculator struct {
	repo Repository
}

func NewRateCalculator(repo Repository) RateCalculator {
	return &rateCalculator{repo: repo}
}

// CalculateFreight computes total freight charge including surcharges
// Calculation: base_rate + (weight * per_kg_rate) + (volume * per_cbm_rate) + surcharges
func (rc *rateCalculator) CalculateFreight(ctx context.Context, input CalculateFreightInput) (*CalculateFreightOutput, error) {
	if input.CompanyID == 0 {
		return nil, fmt.Errorf("company_id is required")
	}
	if input.OriginCity == "" || input.DestinationCity == "" {
		return nil, fmt.Errorf("origin_city and destination_city are required")
	}

	// Find applicable rate card
	lookup := RateLookup{
		CarrierID:       input.CarrierID,
		OriginCity:      input.OriginCity,
		DestinationCity: input.DestinationCity,
		ServiceLevel:    input.ServiceLevel,
		WeightKg:        input.WeightKg,
		VolumeCbm:       input.VolumeCbm,
		AsOfDate:        time.Now(),
	}

	rateCard, err := rc.repo.GetApplicableRateCard(ctx, input.CompanyID, lookup)
	if err != nil {
		return nil, fmt.Errorf("rate card lookup failed: %w", err)
	}
	if rateCard == nil {
		return nil, fmt.Errorf("no applicable rate card found for route %s -> %s", input.OriginCity, input.DestinationCity)
	}

	// Validate rate card applies to weight/volume range
	if err := rc.ValidateRateCard(rateCard, input.WeightKg, input.VolumeCbm); err != nil {
		return nil, err
	}

	// Calculate base charge (always applies)
	baseCharge := rateCard.BaseRate

	// Calculate weight charge (weight_kg * per_kg_rate)
	var weightCharge *accountingmoney.Money
	if input.WeightKg != nil && rateCard.PerKgRate != nil {
		wc := multiplyMoney(*input.WeightKg, *rateCard.PerKgRate)
		weightCharge = &wc
	}

	// Calculate volume charge (volume_cbm * per_cbm_rate)
	var volumeCharge *accountingmoney.Money
	if input.VolumeCbm != nil && rateCard.PerCbmRate != nil {
		vc := multiplyMoney(*input.VolumeCbm, *rateCard.PerCbmRate)
		volumeCharge = &vc
	}

	// Get applicable surcharges
	surcharges, err := rc.repo.ListRateSurcharges(ctx, rateCard.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch surcharges: %w", err)
	}

	// Calculate total surcharges
	var surchargeTotal accountingmoney.Money
	surchargeBreakdown := ""

	for _, surcharge := range surcharges {
		// Check if surcharge is still effective
		if surcharge.ExpirationDate != nil && surcharge.ExpirationDate.Before(time.Now()) {
			continue
		}

		// Fixed surcharge amount
		if surcharge.SurchargeAmount != nil {
			surchargeTotal = surchargeTotal.Add(*surcharge.SurchargeAmount)
			surchargeBreakdown += fmt.Sprintf("%s: %s\n", surcharge.SurchargeName, surcharge.SurchargeAmount)
		}

		// Percentage-based surcharge
		if surcharge.SurchargePercent != nil && *surcharge.SurchargePercent > 0 {
			percentMoney := percentageOf(baseCharge, *surcharge.SurchargePercent)
			surchargeTotal = surchargeTotal.Add(percentMoney)
			surchargeBreakdown += fmt.Sprintf("%s (%.2f%%): %s\n", surcharge.SurchargeName, *surcharge.SurchargePercent, percentMoney)
		}
	}

	// Calculate total freight
	freightTotal := baseCharge
	if weightCharge != nil {
		freightTotal = freightTotal.Add(*weightCharge)
	}
	if volumeCharge != nil {
		freightTotal = freightTotal.Add(*volumeCharge)
	}
	if surchargeTotal.Amount != "" {
		freightTotal = freightTotal.Add(surchargeTotal)
	}

	// Build breakdown for audit trail
	breakdown := fmt.Sprintf("Base Rate: %s\n", baseCharge)
	if weightCharge != nil {
		breakdown += fmt.Sprintf("Weight (%.2f kg × %s/kg): %s\n", parseDecimal(input.WeightKg.Amount), rateCard.PerKgRate, weightCharge)
	}
	if volumeCharge != nil {
		breakdown += fmt.Sprintf("Volume (%.2f cbm × %s/cbm): %s\n", parseDecimal(input.VolumeCbm.Amount), rateCard.PerCbmRate, volumeCharge)
	}
	if surchargeTotal.Amount != "" {
		breakdown += fmt.Sprintf("Surcharges: %s\n%s", surchargeTotal, surchargeBreakdown)
	}
	breakdown += fmt.Sprintf("Total Freight: %s", freightTotal)

	zeroSurcharge := accountingmoney.Money{Amount: "0", Scale: baseCharge.Scale}
	if surchargeTotal.Amount == "" {
		surchargeTotal = zeroSurcharge
	}

	return &CalculateFreightOutput{
		BaseCharge:     baseCharge,
		WeightCharge:   weightCharge,
		VolumeCharge:   volumeCharge,
		SurchargeTotal: &surchargeTotal,
		FreightTotal:   freightTotal,
		Currency:       baseCharge.Amount, // Amount holds the decimal string
		Breakdown:      breakdown,
	}, nil
}

// CalculateLandedCost computes total landed cost including freight, duties, taxes, insurance
func (rc *rateCalculator) CalculateLandedCost(ctx context.Context, input CalculateLandedCostInput) (*LandedCost, error) {
	if input.CompanyID == 0 {
		return nil, fmt.Errorf("company_id is required")
	}
	if input.ShipmentID == 0 {
		return nil, fmt.Errorf("shipment_id is required")
	}
	if input.FreightChargeID == 0 {
		return nil, fmt.Errorf("freight_charge_id is required")
	}

	// Get freight charge
	freightCharge, err := rc.repo.GetFreightCharge(ctx, input.CompanyID, input.FreightChargeID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch freight charge: %w", err)
	}
	if freightCharge == nil {
		return nil, fmt.Errorf("freight charge not found")
	}

	// Calculate total landed cost
	totalLandedCost := input.ProductCost
	totalLandedCost = totalLandedCost.Add(freightCharge.FreightTotal)

	// Add optional costs
	if input.DutyCost != nil {
		totalLandedCost = totalLandedCost.Add(*input.DutyCost)
	}
	if input.TaxCost != nil {
		totalLandedCost = totalLandedCost.Add(*input.TaxCost)
	}
	if input.InsuranceCost != nil {
		totalLandedCost = totalLandedCost.Add(*input.InsuranceCost)
	}
	if input.OtherCost != nil {
		totalLandedCost = totalLandedCost.Add(*input.OtherCost)
	}

	// Calculate cost per unit (if applicable)
	var costPerUnit *accountingmoney.Money
	if freightCharge.WeightKg != nil && freightCharge.WeightKg.Amount != "" {
		// Divide landed cost by weight
		weight := parseDecimal(freightCharge.WeightKg.Amount)
		if weight > 0 {
			costStr := divideMoney(totalLandedCost, weight)
			cpu := accountingmoney.Money{Amount: costStr, Scale: totalLandedCost.Scale}
			costPerUnit = &cpu
		}
	}

	return &LandedCost{
		CompanyID:        input.CompanyID,
		ShipmentID:       input.ShipmentID,
		LoadID:           input.LoadID,
		FreightChargeID:  input.FreightChargeID,
		POID:             input.POID,
		ProductCost:      input.ProductCost,
		FreightCost:      freightCharge.FreightTotal,
		DutyCost:         input.DutyCost,
		TaxCost:          input.TaxCost,
		InsuranceCost:    input.InsuranceCost,
		OtherCost:        input.OtherCost,
		TotalLandedCost:  totalLandedCost,
		CostPerUnit:      costPerUnit,
		Currency:         input.ProductCost.Amount, // Amount holds the currency string in this context
		AllocationMethod: input.AllocationMethod,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}, nil
}

// ValidateRateCard checks if rate card applies to given weight/volume
func (rc *rateCalculator) ValidateRateCard(rateCard *RateCard, weight, volume *accountingmoney.Money) error {
	if weight == nil || weight.Amount == "" {
		return nil
	}

	weightVal := parseDecimal(weight.Amount)

	// Check weight range if specified
	if rateCard.MinWeightKg != nil && rateCard.MinWeightKg.Amount != "" {
		minVal := parseDecimal(rateCard.MinWeightKg.Amount)
		if weightVal < minVal {
			return fmt.Errorf("weight %.2f kg below minimum %.2f kg", weightVal, minVal)
		}
	}

	if rateCard.MaxWeightKg != nil && rateCard.MaxWeightKg.Amount != "" {
		maxVal := parseDecimal(rateCard.MaxWeightKg.Amount)
		if weightVal > maxVal {
			return fmt.Errorf("weight %.2f kg exceeds maximum %.2f kg", weightVal, maxVal)
		}
	}

	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// HELPER FUNCTIONS FOR MONEY ARITHMETIC
// ═══════════════════════════════════════════════════════════════════════════

// multiplyMoney multiplies two Money values using big.Rat
func multiplyMoney(a, b accountingmoney.Money) accountingmoney.Money {
	aRat := new(big.Rat)
	aRat.SetString(a.Amount)
	bRat := new(big.Rat)
	bRat.SetString(b.Amount)
	result := new(big.Rat).Mul(aRat, bRat)

	scale := a.Scale
	if b.Scale > scale {
		scale = b.Scale
	}

	return accountingmoney.Money{
		Amount: result.FloatString(scale),
		Scale:  scale,
	}
}

// percentageOf calculates a percentage of a Money value
func percentageOf(m accountingmoney.Money, percent float64) accountingmoney.Money {
	mRat := new(big.Rat)
	mRat.SetString(m.Amount)
	percentRat := new(big.Rat).SetFloat64(percent / 100)
	result := new(big.Rat).Mul(mRat, percentRat)

	return accountingmoney.Money{
		Amount: result.FloatString(m.Scale),
		Scale:  m.Scale,
	}
}

// divideMoney divides a Money value by a float
func divideMoney(m accountingmoney.Money, divisor float64) string {
	mRat := new(big.Rat)
	mRat.SetString(m.Amount)
	divisorRat := new(big.Rat).SetFloat64(divisor)
	result := new(big.Rat).Quo(mRat, divisorRat)

	return result.FloatString(m.Scale)
}

// parseDecimal converts a Money amount string to float64
func parseDecimal(amount string) float64 {
	rat := new(big.Rat)
	_, ok := rat.SetString(amount)
	if !ok {
		return 0
	}
	f64, _ := rat.Float64()
	return f64
}
