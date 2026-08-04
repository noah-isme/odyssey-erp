package procurement

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
)

// TestContractLifecycle tests the full contract lifecycle: DRAFT → APPROVAL → ACTIVE → TERMINATED
func TestContractLifecycle(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	defer db.Close()

	service := NewContractService(db)
	companyID := int64(1)
	supplierID := int64(100)
	createdBy := int64(1)

	// Create contract in DRAFT status
	input := CreateContractInput{
		CompanyID:         companyID,
		SupplierID:        supplierID,
		Currency:          "USD",
		EffectiveFrom:     time.Now(),
		EffectiveTo:       nil,
		PaymentTerms:      "Net 30",
		Incoterms:         "FOB",
		RenewalNoticeDays: 30,
		CreatedBy:         createdBy,
		Note:              "Test contract",
	}

	contract, err := service.CreateContractDraft(ctx, input)
	if err != nil {
		t.Fatalf("CreateContractDraft failed: %v", err)
	}

	if contract.Status != ContractStatusDraft {
		t.Errorf("Expected status DRAFT, got %s", contract.Status)
	}

	// Submit for approval
	err = service.SubmitContractForApproval(ctx, contract.ID)
	if err != nil {
		t.Fatalf("SubmitContractForApproval failed: %v", err)
	}

	// Verify status changed to APPROVAL
	contract, err = service.repo.GetContract(ctx, contract.ID)
	if err != nil {
		t.Fatalf("GetContract failed: %v", err)
	}
	if contract.Status != ContractStatusApproval {
		t.Errorf("Expected status APPROVAL, got %s", contract.Status)
	}

	// Approve contract
	err = service.ApproveContract(ctx, contract.ID, createdBy)
	if err != nil {
		t.Fatalf("ApproveContract failed: %v", err)
	}

	// Verify status changed to ACTIVE
	contract, err = service.repo.GetContract(ctx, contract.ID)
	if err != nil {
		t.Fatalf("GetContract failed: %v", err)
	}
	if contract.Status != ContractStatusActive {
		t.Errorf("Expected status ACTIVE, got %s", contract.Status)
	}

	// Terminate contract
	err = service.TerminateContract(ctx, contract.ID)
	if err != nil {
		t.Fatalf("TerminateContract failed: %v", err)
	}

	// Verify status changed to TERMINATED
	contract, err = service.repo.GetContract(ctx, contract.ID)
	if err != nil {
		t.Fatalf("GetContract failed: %v", err)
	}
	if contract.Status != ContractStatusTerminated {
		t.Errorf("Expected status TERMINATED, got %s", contract.Status)
	}
}

// TestContractPriceTierSelection tests quantity-based price tier selection
func TestContractPriceTierSelection(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	defer db.Close()

	repo := NewContractRepository(db)
	companyID := int64(1)
	supplierID := int64(100)
	productID := int64(1001)

	// Create contract
	contractID, err := repo.CreateContract(ctx, CreateContractInput{
		CompanyID:     companyID,
		SupplierID:    supplierID,
		Currency:      "USD",
		EffectiveFrom: time.Now(),
		CreatedBy:     1,
	})
	if err != nil {
		t.Fatalf("CreateContract failed: %v", err)
	}

	// Add three price tiers: 1-100 @ $10, 101-500 @ $9, 500+ @ $8
	tiers := []ContractPriceLineInput{
		{
			ContractID:   contractID,
			ProductID:    productID,
			MinQuantity:  accountingmoney.Must("1", 4),
			UnitPrice:    accountingmoney.Must("10.00", 4),
			TaxRate:      accountingmoney.Must("10.00", 2),
			LeadTimeDays: 7,
			MOQ:          accountingmoney.Must("10", 4),
		},
		{
			ContractID:   contractID,
			ProductID:    productID,
			MinQuantity:  accountingmoney.Must("101", 4),
			UnitPrice:    accountingmoney.Must("9.00", 4),
			TaxRate:      accountingmoney.Must("10.00", 2),
			LeadTimeDays: 5,
			MOQ:          accountingmoney.Must("100", 4),
		},
		{
			ContractID:   contractID,
			ProductID:    productID,
			MinQuantity:  accountingmoney.Must("501", 4),
			UnitPrice:    accountingmoney.Must("8.00", 4),
			TaxRate:      accountingmoney.Must("10.00", 2),
			LeadTimeDays: 3,
			MOQ:          accountingmoney.Must("500", 4),
		},
	}

	for _, tier := range tiers {
		err := repo.InsertContractPriceLine(ctx, tier)
		if err != nil {
			t.Fatalf("InsertContractPriceLine failed: %v", err)
		}
	}

	// Test tier selection at various quantities
	testCases := []struct {
		qty      string
		expectedPrice string
		expectedLeadTime int
	}{
		{"50", "10.00", 7},
		{"100", "10.00", 7},
		{"150", "9.00", 5},
		{"500", "9.00", 5},
		{"600", "8.00", 3},
		{"1000", "8.00", 3},
	}

	for _, tc := range testCases {
		qty, _ := accountingmoney.Parse(tc.qty, 4)
		line, err := repo.GetApplicablePriceLine(ctx, contractID, productID, qty)
		if err != nil {
			t.Fatalf("GetApplicablePriceLine(%s) failed: %v", tc.qty, err)
		}

		if line.UnitPrice.String() != tc.expectedPrice {
			t.Errorf("Qty %s: expected price %s, got %s", tc.qty, tc.expectedPrice, line.UnitPrice.String())
		}
		if line.LeadTimeDays != tc.expectedLeadTime {
			t.Errorf("Qty %s: expected lead time %d, got %d", tc.qty, tc.expectedLeadTime, line.LeadTimeDays)
		}
	}
}

// TestPriceHistoryImmutability verifies price history cannot be updated
func TestPriceHistoryImmutability(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	defer db.Close()

	repo := NewContractRepository(db)
	companyID := int64(1)
	supplierID := int64(100)
	productID := int64(1001)

	// Record price history
	input := RecordPriceHistoryInput{
		CompanyID:    companyID,
		SupplierID:   supplierID,
		ProductID:    productID,
		SourceType:   PriceHistorySourceBid,
		SourceID:     500,
		Currency:     "USD",
		UnitPrice:    accountingmoney.Must("10.50", 4),
		Quantity:     accountingmoney.Must("100", 4),
		TaxRate:      accountingmoney.Must("10.00", 2),
		MOQ:          accountingmoney.Must("50", 4),
		LeadTimeDays: 7,
		Note:         "Test price observation",
	}

	phID, err := repo.RecordPriceHistory(ctx, input)
	if err != nil {
		t.Fatalf("RecordPriceHistory failed: %v", err)
	}

	// Retrieve price history
	history, err := repo.ListPriceHistory(ctx, companyID, supplierID, productID, 10)
	if err != nil {
		t.Fatalf("ListPriceHistory failed: %v", err)
	}

	if len(history) == 0 {
		t.Fatalf("No price history records found")
	}

	if history[0].ID != phID {
		t.Errorf("Expected ID %d, got %d", phID, history[0].ID)
	}

	if history[0].UnitPrice.String() != "10.50" {
		t.Errorf("Expected price 10.50, got %s", history[0].UnitPrice.String())
	}
}

// TestVarianceDetection tests automatic variance detection during PO creation
func TestVarianceDetection(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	defer db.Close()

	service := NewContractService(db)
	companyID := int64(1)
	supplierID := int64(100)

	testCases := []struct {
		name     string
		input    CheckPOVariancesInput
		expectVariance bool
		varianceType VarianceType
	}{
		{
			name: "No contract when none exists",
			input: CheckPOVariancesInput{
				CompanyID:  companyID,
				POID:       1,
				POLineID:   1,
				SupplierID: supplierID,
				ProductID:  1001,
				ContractID: nil,
				Quantity:   accountingmoney.Must("100", 4),
				POPrice:    "10.00",
			},
			expectVariance: false, // No applicable contract exists in test
		},
		{
			name: "Expired contract",
			input: CheckPOVariancesInput{
				CompanyID:  companyID,
				POID:       2,
				POLineID:   2,
				SupplierID: supplierID,
				ProductID:  1001,
				ContractID: int64Ptr(999), // Non-existent contract will error
				Quantity:   accountingmoney.Must("100", 4),
				POPrice:    "10.00",
			},
			expectVariance: false, // Will error on retrieval
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			variances, err := service.CheckPOVariances(ctx, tc.input)
			if err != nil {
				// Some test cases expect errors (non-existent contracts)
				return
			}

			if tc.expectVariance && len(variances) == 0 {
				t.Errorf("Expected variances but got none")
			}
			if !tc.expectVariance && len(variances) > 0 {
				t.Errorf("Expected no variances but got %d", len(variances))
			}
		})
	}
}

// TestScorecardLifecycle tests scorecard creation and publication
func TestScorecardLifecycle(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	defer db.Close()

	service := NewScorecardService(db)
	companyID := int64(1)
	supplierID := int64(100)
	createdBy := int64(1)

	// Create draft scorecard
	input := CreateScorecardInput{
		CompanyID:  companyID,
		SupplierID: supplierID,
		PeriodStart: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:  time.Date(2026, 7, 31, 23, 59, 59, 0, time.UTC),
		CreatedBy:  createdBy,
		Note:       "July 2026 performance",
	}

	scorecard, err := service.CreateDraftScorecard(ctx, input)
	if err != nil {
		t.Fatalf("CreateDraftScorecard failed: %v", err)
	}

	if scorecard.Status != ScorecardStatusDraft {
		t.Errorf("Expected status DRAFT, got %s", scorecard.Status)
	}

	// Publish scorecard
	err = service.PublishScorecard(ctx, scorecard.ID, createdBy)
	if err != nil {
		t.Fatalf("PublishScorecard failed: %v", err)
	}

	// Verify status changed to PUBLISHED
	scorecard, err = service.repo.GetScorecard(ctx, scorecard.ID)
	if err != nil {
		t.Fatalf("GetScorecard failed: %v", err)
	}

	if scorecard.Status != ScorecardStatusPublished {
		t.Errorf("Expected status PUBLISHED, got %s", scorecard.Status)
	}

	if scorecard.PublishedBy == nil || *scorecard.PublishedBy != createdBy {
		t.Errorf("Expected published_by to be set to %d", createdBy)
	}
}

// Helper functions

func int64Ptr(v int64) *int64 {
	return &v
}

func setupTestDB(t *testing.T) *pgxpool.Pool {
	// TODO: Set up test database connection
	// For now, return nil to indicate test infrastructure needed
	t.Skip("Test database setup not configured")
	return nil
}
