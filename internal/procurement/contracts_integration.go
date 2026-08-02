package procurement

import (
	"context"
	"fmt"

	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
)

// POCreationHook is called when a PO line is created to detect variances
// This integrates Phase 3 vendor intelligence with PO creation
func (s *ContractService) CheckAndCreatePOVariances(ctx context.Context, input CreatePOVarianceCheckInput) error {
	// Parse quantity as Money
	qty, err := accountingmoney.Parse(input.Quantity, 4)
	if err != nil {
		return fmt.Errorf("invalid quantity: %w", err)
	}

	// Detect variances
	variances, err := s.CheckPOVariances(ctx, CheckPOVariancesInput{
		CompanyID:  input.CompanyID,
		POID:       input.POID,
		POLineID:   input.POLineID,
		SupplierID: input.SupplierID,
		ProductID:  input.ProductID,
		ContractID: input.ContractID,
		Quantity:   qty,
		POPrice:    input.POPrice,
	})
	if err != nil {
		return fmt.Errorf("failed to check variances: %w", err)
	}

	// Create variance records for each detected deviation
	for _, variance := range variances {
		_, err := s.repo.CreatePOVariance(ctx, CreatePOVarianceInput{
			CompanyID:           variance.CompanyID,
			POID:                variance.POID,
			POLineID:            variance.POLineID,
			ContractID:          variance.ContractID,
			VarianceType:        variance.VarianceType,
			VariancePercentage:  variance.VariancePercentage,
			VarianceReason:      variance.VarianceReason,
			Note:                variance.Note,
		})
		if err != nil {
			return fmt.Errorf("failed to create variance: %w", err)
		}
	}

	// If variances detected, PO approval should be blocked
	// This is typically enforced at the PO service level
	if len(variances) > 0 {
		return fmt.Errorf("PO has pending variances that must be approved before approval")
	}

	return nil
}

// CreatePOVarianceCheckInput aggregates information needed for variance checking
type CreatePOVarianceCheckInput struct {
	CompanyID  int64
	POID       int64
	POLineID   int64
	SupplierID int64
	ProductID  int64
	ContractID *int64
	Quantity   string // NUMERIC as string for precision
	POPrice    string // NUMERIC as string for precision
}

// ScorecardCalculationInput bundles data for scorecard calculation
type ScorecardCalculationInput struct {
	CompanyID  int64
	SupplierID int64
	PeriodStart string // "YYYY-MM-DD"
	PeriodEnd   string // "YYYY-MM-DD"
	CreatedBy  int64
}

// POApprovalHook checks if PO can be approved based on variance status
// Called before PO approval is allowed
func (s *ContractService) CanApprovePO(ctx context.Context, poID int64, companyID int64) (bool, error) {
	// TODO: Implement with actual DB:
	// 1. Query po_contract_variances where po_id = ? AND approval_status = 'PENDING'
	// 2. If any PENDING variances exist, return false
	// 3. Return true if all variances are APPROVED or REJECTED
	
	_ = ctx
	_ = poID
	_ = companyID
	return true, nil // Placeholder
}

// RecordPOPriceObservation records the PO line price in price history
// Called after PO approval to track actual prices paid
func (s *ContractService) RecordPOPriceObservation(ctx context.Context, poID int64, poLineID int64, supplierID int64, productID int64, companyID int64, quantity string, unitPrice string, currency string) error {
	qty, err := accountingmoney.Parse(quantity, 4)
	if err != nil {
		return fmt.Errorf("invalid quantity: %w", err)
	}

	price, err := accountingmoney.Parse(unitPrice, 4)
	if err != nil {
		return fmt.Errorf("invalid unit price: %w", err)
	}

	zeroMoney, _ := accountingmoney.Parse("0", 4)

	input := RecordPriceHistoryInput{
		CompanyID:    companyID,
		SupplierID:   supplierID,
		ProductID:    productID,
		SourceType:   PriceHistorySourcePO,
		SourceID:     poID,
		Currency:     currency,
		UnitPrice:    price,
		Quantity:     qty,
		TaxRate:      zeroMoney,
		MOQ:          zeroMoney,
		LeadTimeDays: 0,
		Note:         fmt.Sprintf("PO line %d", poLineID),
	}

	_, err = s.repo.RecordPriceHistory(ctx, input)
	return err
}

// ═══════════════════════════════════════════════════════════════════════════
// SCORECARD CALCULATION INTEGRATION
// ═══════════════════════════════════════════════════════════════════════════

// BackgroundJobScheduler interface for scorecard jobs
// Typically implemented by Asynq job handler
type BackgroundJobScheduler interface {
	ScheduleMonthlyScorecardCalculation(ctx context.Context, companyID int64) error
	ScheduleContractExpiryCheck(ctx context.Context, companyID int64) error
}

// MonthlyScorecardCalculationJob handles monthly scorecard calculations
// This would be scheduled by Asynq at month-end
func (s *ScorecardService) ExecuteMonthlyScorecardCalculations(ctx context.Context, companyID int64) error {
	// TODO: Implement with actual DB:
	// 1. Get all active suppliers for company
	// 2. For each supplier:
	//    a. Check if scorecard exists for current month
	//    b. If not, execute calculation:
	//       - Call CalculateOTIFScore
	//       - Call CalculateQualityScore
	//       - Call CalculatePriceAdherenceScore
	//       - Call CalculateRFQResponsivenessScore
	//       - Allow manual reviewer assessment
	//       - Calculate weighted overall score via CalculateOverallScore
	//       - Create draft scorecard with scores
	//    c. Send notification to procurement team
	// 3. Check for scorecards >5 days in DRAFT status → send reminder to publish

	_ = ctx
	_ = companyID
	return nil // Placeholder
}

// ContractExpiryCheckJob runs daily to detect expiring contracts
// This would be scheduled by Asynq every morning
func (s *ContractService) CheckExpiringContracts(ctx context.Context, companyID int64) error {
	// TODO: Implement with actual DB:
	// 1. Find all ACTIVE contracts where:
	//    - effective_to IS NOT NULL
	//    - effective_to <= TODAY + renewal_notice_days (typically 30 days)
	//    - expiry_notification_sent = FALSE
	// 2. For each matching contract:
	//    a. Send notification to procurement team (email):
	//       - Contract number and supplier name
	//       - Current effective_to date
	//       - Suggested renewal action
	//    b. Call SetContractExpiryNotificationSent to prevent duplicate notifications
	// 3. Log all notifications for audit trail
	
	// Query template:
	// SELECT id, contract_number, supplier_id, effective_to, renewal_notice_days
	// FROM supplier_contracts
	// WHERE company_id = ? AND status = 'ACTIVE'
	//   AND effective_to IS NOT NULL
	//   AND effective_to <= CURRENT_DATE + (renewal_notice_days || ' days')::interval
	//   AND expiry_notification_sent = FALSE
	// ORDER BY effective_to ASC
	
	_ = ctx
	_ = companyID
	return nil // Placeholder
}

// ═══════════════════════════════════════════════════════════════════════════
// DOCUMENTATION: Phase 3b Integration Points
// ═══════════════════════════════════════════════════════════════════════════

/*
PHASE 3B INTEGRATION CHECKLIST

1. PO CREATION INTEGRATION
   [ ] When PO line is created:
       - Call CheckAndCreatePOVariances
       - If variances exist, set PO to PENDING_VARIANCE status
       - Send notification to approver
   [ ] When PO approval is requested:
       - Call CanApprovePO
       - If returns false (pending variances), reject approval
   [ ] When PO is approved:
       - Call RecordPOPriceObservation to add to price history
       - Record price data for trend analysis

2. SCORECARD CALCULATION
   [ ] Set up monthly Asynq job:
       - Runs on last day of month or 1st of next month
       - For each company, calls ExecuteMonthlyScorecardCalculations
       - Creates draft scorecards with calculated scores
       - Sends notifications to procurement team
   [ ] Implement scorecard calculation methods:
       - CalculateOTIFScore: Query GRN receipts, compare to promised dates
       - CalculateQualityScore: Query accepted receipts vs returns
       - CalculatePriceAdherenceScore: Query PO prices vs contract/award
       - CalculateRFQResponsivenessScore: Query RFQ invitations vs bids
   [ ] Implement CalculateOverallScore:
       - Apply weights: delivery 35%, quality 25%, price 20%, rfq 10%, reviewer 10%
       - Result should be 0-100%

3. CONTRACT EXPIRY NOTIFICATIONS
   [ ] Set up daily Asynq job:
       - Runs every morning
       - For each company, calls CheckExpiringContracts
       - Sends email notifications for expiring contracts
       - Marks notifications as sent to avoid duplicates

4. RBAC & AUDIT TRAIL
   [ ] Verify all Phase 3 operations create audit_logs entries
   [ ] Test RBAC permissions gate all variance approval operations
   [ ] Verify audit trail shows who approved/rejected each variance
   [ ] Verify scorecard publication is immutable and audited

5. COMPANY ISOLATION
   [ ] Test contract queries with multiple companies
   [ ] Test scorecard queries with multiple companies
   [ ] Test variance queries with multiple companies
   [ ] Verify no cross-company data leakage

6. EXACT ACCOUNTING
   [ ] Verify all price calculations preserve decimal precision
   [ ] Test variance percentage calculations
   [ ] Test tax calculations
   [ ] Test rounding behavior (if any) is documented

7. ERROR HANDLING & VALIDATION
   [ ] Contract effective dates: from <= to
   [ ] Contract lifecycle transitions: only valid state changes allowed
   [ ] Scorecard publication: only DRAFT → PUBLISHED allowed
   [ ] Variance approval: only PENDING → APPROVED/REJECTED allowed

8. PERFORMANCE & SCALABILITY
   [ ] Test contract tier selection with 10,000+ tiers
   [ ] Test price history queries with 1M+ observations
   [ ] Test scorecard calculations with 1000+ suppliers
   [ ] Verify index usage in all major queries

9. END-TO-END E2E TESTS
   [ ] Test 1: Contract → PO → Variance → Approval → PO Approval
   [ ] Test 2: Create Contract → Calculate Scorecard → Publish
   [ ] Test 3: Contract Expiry → Notification → New Contract Created
   [ ] Test 4: Price History Trend → Used for Next Contract Pricing

10. DOCUMENTATION & TRAINING
    [ ] Document variance approval workflow for buyers
    [ ] Document scorecard calculation methodology
    [ ] Document contract effective date best practices
    [ ] Create operational runbook for monthly scorecard review
*/
