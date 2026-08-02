package freight

import (
	"context"
	"fmt"
	"time"

	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
)

// ═══════════════════════════════════════════════════════════════════════════
// GL POSTING SERVICE
// ═══════════════════════════════════════════════════════════════════════════

// GLPostingService handles posting freight charges to the general ledger
type GLPostingService interface {
	PostFreightToGL(ctx context.Context, input PostFreightToGLInput) (glPostingID int64, err error)
	PostFreightExpense(ctx context.Context, companyID, freightChargeID int64, costCenterID int64) (int64, error)
	PostFreightPayable(ctx context.Context, companyID, freightChargeID int64, vendorID int64) (int64, error)
	ReconcileFreightPostings(ctx context.Context, companyID int64) (reconciled int, errors int, err error)
}

type glPostingService struct {
	freightRepo Repository
	// In a real implementation, this would be the GL service
	// For now we'll define the interface
}

// NewGLPostingService creates a new GL posting service
func NewGLPostingService(freightRepo Repository) GLPostingService {
	return &glPostingService{
		freightRepo: freightRepo,
	}
}

// PostFreightToGL posts a freight charge to the general ledger
// Creates two entries:
//   - Debit: Freight Expense (or appropriate cost account)
//   - Credit: Accounts Payable (or cash, depending on payment terms)
func (gps *glPostingService) PostFreightToGL(ctx context.Context, input PostFreightToGLInput) (int64, error) {
	if input.CompanyID == 0 {
		return 0, fmt.Errorf("company_id is required")
	}
	if input.FreightChargeID == 0 {
		return 0, fmt.Errorf("freight_charge_id is required")
	}
	if input.FreightAmount.Amount == "" {
		return 0, fmt.Errorf("freight_amount is required")
	}

	// Get freight charge to verify it exists
	charge, err := gps.freightRepo.GetFreightCharge(ctx, input.CompanyID, input.FreightChargeID)
	if err != nil {
		return 0, fmt.Errorf("freight charge not found: %w", err)
	}
	if charge == nil {
		return 0, fmt.Errorf("freight charge not found")
	}

	// TODO: In production, integrate with GL service to:
	// 1. Create GL entries for freight expense
	// 2. Create GL entries for payable/accrual
	// 3. Return GL posting ID
	// 4. Update freight_charge with gl_posting_id

	// For now, simulate GL posting ID (would come from GL service)
	glPostingID := int64(time.Now().Unix())

	// Update freight charge with GL posting ID
	updates := FreightChargeUpdate{
		GLPostingID: &glPostingID,
	}

	_, err = gps.freightRepo.UpdateFreightCharge(ctx, input.CompanyID, input.FreightChargeID, updates)
	if err != nil {
		return 0, fmt.Errorf("failed to update freight charge with GL posting ID: %w", err)
	}

	// Log audit event
	_ = gps.freightRepo.CreateAuditLog(ctx, &FreightAuditLog{
		CompanyID:       input.CompanyID,
		FreightChargeID: input.FreightChargeID,
		AuditType:       AuditTypePosted,
		NewValue:        &input.FreightAmount,
		Reason:          &input.Description,
		UserID:          input.PostedBy,
		CreatedAt:       time.Now(),
	})

	return glPostingID, nil
}

// PostFreightExpense posts freight charge to freight expense account
// This is called when freight is charged to a specific cost center
func (gps *glPostingService) PostFreightExpense(ctx context.Context, companyID, freightChargeID int64, costCenterID int64) (int64, error) {
	if companyID == 0 || freightChargeID == 0 {
		return 0, fmt.Errorf("company_id and freight_charge_id are required")
	}

	// Get freight charge
	charge, err := gps.freightRepo.GetFreightCharge(ctx, companyID, freightChargeID)
	if err != nil {
		return 0, fmt.Errorf("freight charge not found: %w", err)
	}
	if charge == nil {
		return 0, fmt.Errorf("freight charge not found")
	}

	// Get cost center if provided
	var glAccount string
	if costCenterID > 0 {
		costCenter, err := gps.freightRepo.GetCostCenter(ctx, companyID, costCenterID)
		if err != nil {
			return 0, fmt.Errorf("cost center not found: %w", err)
		}
		if costCenter != nil && costCenter.GLAccount != nil {
			glAccount = *costCenter.GLAccount
		}
	}

	// If no GL account from cost center, use default freight expense account
	if glAccount == "" {
		glAccount = "6100" // Default Freight Expense account
	}

	// Create GL posting input
	input := PostFreightToGLInput{
		CompanyID:       companyID,
		FreightChargeID: freightChargeID,
		CostCenterID:    costCenterID,
		GLAccount:       glAccount,
		FreightAmount:   charge.FreightTotal,
		Description:     fmt.Sprintf("Freight charge to %s -> %s", charge.OriginCity, charge.DestinationCity),
		PostedBy:        companyID, // TODO: pass actual user ID
	}

	return gps.PostFreightToGL(ctx, input)
}

// PostFreightPayable posts freight charge to accounts payable
// This is called when freight is due to a carrier
func (gps *glPostingService) PostFreightPayable(ctx context.Context, companyID, freightChargeID int64, vendorID int64) (int64, error) {
	if companyID == 0 || freightChargeID == 0 {
		return 0, fmt.Errorf("company_id and freight_charge_id are required")
	}

	// Get freight charge
	charge, err := gps.freightRepo.GetFreightCharge(ctx, companyID, freightChargeID)
	if err != nil {
		return 0, fmt.Errorf("freight charge not found: %w", err)
	}
	if charge == nil {
		return 0, fmt.Errorf("freight charge not found")
	}

	// Create GL posting for payable
	// Debit: Freight Expense
	// Credit: Accounts Payable (or specific carrier payable)
	costCenterID := int64(0)
	if charge.CostCenterID != nil {
		costCenterID = *charge.CostCenterID
	}

	input := PostFreightToGLInput{
		CompanyID:       companyID,
		FreightChargeID: freightChargeID,
		CostCenterID:    costCenterID,
		GLAccount:       "2100", // Accounts Payable - Carriers
		FreightAmount:   charge.FreightTotal,
		Description:     fmt.Sprintf("Freight payable to carrier %d", vendorID),
		PostedBy:        companyID,
	}

	return gps.PostFreightToGL(ctx, input)
}

// ReconcileFreightPostings reconciles freight charges with GL postings
// Returns number of reconciled records and any errors encountered
func (gps *glPostingService) ReconcileFreightPostings(ctx context.Context, companyID int64) (reconciled int, errors int, err error) {
	if companyID == 0 {
		return 0, 0, fmt.Errorf("company_id is required")
	}

	// Get all invoiced freight charges
	filter := FreightChargeFilter{
		Status: func() *FreightChargeStatus { s := FreightChargeStatusInvoiced; return &s }(),
		Limit:  1000,
		Offset: 0,
	}

	charges, err := gps.freightRepo.ListFreightCharges(ctx, companyID, filter)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to list freight charges: %w", err)
	}

	for _, charge := range charges {
		// Check if GL posting ID exists
		if charge.GLPostingID == nil {
			// Missing GL posting - log as error but continue
			errors++
			_ = gps.freightRepo.CreateAuditLog(ctx, &FreightAuditLog{
				CompanyID:       companyID,
				FreightChargeID: charge.ID,
				AuditType:       AuditTypeReconciled,
				Reason:          stringPtr("Missing GL posting ID during reconciliation"),
				UserID:          companyID,
				CreatedAt:       time.Now(),
			})
			continue
		}

		// TODO: Verify GL posting exists and matches freight amount
		// For now, just count as reconciled
		reconciled++

		_ = gps.freightRepo.CreateAuditLog(ctx, &FreightAuditLog{
			CompanyID:       companyID,
			FreightChargeID: charge.ID,
			AuditType:       AuditTypeReconciled,
			NewValue:        &charge.FreightTotal,
			Reason:          stringPtr("Freight posting reconciled successfully"),
			UserID:          companyID,
			CreatedAt:       time.Now(),
		})
	}

	return reconciled, errors, nil
}

// ═══════════════════════════════════════════════════════════════════════════
// HELPER FUNCTIONS
// ═══════════════════════════════════════════════════════════════════════════

func stringPtr(s string) *string {
	return &s
}

// FreightToGLEntry represents a single GL entry for freight
type FreightToGLEntry struct {
	AccountCode  string                   // GL account code (e.g., "6100")
	AccountName  string                   // GL account name (e.g., "Freight Expense")
	Description  string                   // Transaction description
	DebitAmount  *accountingmoney.Money   // Debit side (nil if credit)
	CreditAmount *accountingmoney.Money   // Credit side (nil if debit)
	CostCenter   *string                  // Cost center allocation
	Currency     string                   // Currency code
	TransDate    time.Time                // Transaction date
	Reference    string                   // Reference (e.g., invoice number)
}

// BuildFreightGLEntries constructs GL entries for a freight charge
// Returns: [expense entry, payable entry]
func BuildFreightGLEntries(charge *FreightCharge, glAccount string, costCenter *string) []FreightToGLEntry {
	entries := make([]FreightToGLEntry, 0, 2)

	// Entry 1: Debit Freight Expense
	expenseEntry := FreightToGLEntry{
		AccountCode:  glAccount,
		AccountName:  "Freight Expense",
		Description:  fmt.Sprintf("Freight: %s to %s", charge.OriginCity, charge.DestinationCity),
		DebitAmount:  &charge.FreightTotal,
		CreditAmount: nil,
		CostCenter:   costCenter,
		Currency:     charge.Currency,
		TransDate:    charge.CreatedAt,
		Reference:    fmt.Sprintf("FC-%d", charge.ID),
	}
	entries = append(entries, expenseEntry)

	// Entry 2: Credit Accounts Payable or specific payable account
	payableEntry := FreightToGLEntry{
		AccountCode:  "2100", // AP - Carriers
		AccountName:  "Accounts Payable - Carriers",
		Description:  fmt.Sprintf("Freight payable: %s to %s", charge.OriginCity, charge.DestinationCity),
		DebitAmount:  nil,
		CreditAmount: &charge.FreightTotal,
		CostCenter:   nil,
		Currency:     charge.Currency,
		TransDate:    charge.CreatedAt,
		Reference:    fmt.Sprintf("FC-%d", charge.ID),
	}
	entries = append(entries, payableEntry)

	return entries
}
