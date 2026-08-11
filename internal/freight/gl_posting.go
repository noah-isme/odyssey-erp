package freight

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/journals"
	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
	accountingshared "github.com/odyssey-erp/odyssey-erp/internal/accounting/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/fx"
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

// AccountingJournalPoster is the small accounting boundary required by
// freight. journals.Service satisfies it directly, while an application
// adapter can resolve account codes and periods before delegating to it.
type AccountingJournalPoster interface {
	PostJournal(ctx context.Context, input journals.PostingInput) (journals.JournalEntry, error)
}

var (
	// ErrAccountingNotConfigured is returned by the legacy constructor until
	// the application supplies the accounting journal adapter.
	ErrAccountingNotConfigured = errors.New("freight: accounting journal poster is not configured")
	// ErrGLPostingIdentityMismatch protects an existing freight-to-journal link
	// from being overwritten by a different accounting identity.
	ErrGLPostingIdentityMismatch = errors.New("freight: GL posting identity mismatch")
)

const (
	freightSourceModule       = "FREIGHT.CHARGE"
	freightPayableAccountCode = "2100"
	freightExpenseAccountCode = "6100"
)

type glPostingService struct {
	freightRepo Repository
	accounting  AccountingJournalPoster
	posted      sync.Map
}

// NewGLPostingService creates a new GL posting service. The variadic form
// preserves the original constructor for callers that have not wired
// accounting yet; posting then fails closed with ErrAccountingNotConfigured.
func NewGLPostingService(freightRepo Repository, accounting ...AccountingJournalPoster) GLPostingService {
	var poster AccountingJournalPoster
	if len(accounting) > 0 {
		poster = accounting[0]
	}
	return &glPostingService{
		freightRepo: freightRepo,
		accounting:  poster,
	}
}

// NewGLPostingServiceWithAccounting constructs the production-ready freight
// posting service with an injected accounting journal poster.
func NewGLPostingServiceWithAccounting(freightRepo Repository, accounting AccountingJournalPoster) GLPostingService {
	return NewGLPostingService(freightRepo, accounting)
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
	if gps == nil || gps.accounting == nil {
		return 0, ErrAccountingNotConfigured
	}

	amount, err := fx.ParseDecimal(input.FreightAmount.Amount)
	if err != nil {
		return 0, fmt.Errorf("freight_amount is invalid: %w", err)
	}
	debitAccountID, err := parseGLAccountID(input.GLAccount)
	if err != nil {
		return 0, fmt.Errorf("freight GL account is invalid: %w", err)
	}
	creditAccountID, err := parseGLAccountID(freightPayableAccountCode)
	if err != nil {
		return 0, fmt.Errorf("freight payable account is invalid: %w", err)
	}

	// Get freight charge to verify it exists
	charge, err := gps.freightRepo.GetFreightCharge(ctx, input.CompanyID, input.FreightChargeID)
	if err != nil {
		return 0, fmt.Errorf("freight charge not found: %w", err)
	}
	if charge == nil {
		return 0, fmt.Errorf("freight charge not found")
	}

	key := freightPostingKey{companyID: input.CompanyID, chargeID: input.FreightChargeID}
	if cached, ok := gps.posted.Load(key); ok {
		postingID := cached.(int64)
		if charge.GLPostingID == nil || *charge.GLPostingID != postingID {
			return 0, fmt.Errorf("%w: cached journal %d does not match freight charge journal", ErrGLPostingIdentityMismatch, postingID)
		}
		return postingID, nil
	}

	companyID := input.CompanyID
	debitLine := journals.PostingLineInput{
		AccountID:    debitAccountID,
		DebitDecimal: amount,
		CompanyID:    &companyID,
	}
	if input.CostCenterID > 0 {
		costCenterID := input.CostCenterID
		debitLine.CostCenterID = &costCenterID
	}
	creditLine := journals.PostingLineInput{
		AccountID:     creditAccountID,
		CreditDecimal: amount,
		CompanyID:     &companyID,
	}
	postingDate := charge.CreatedAt
	if postingDate.IsZero() {
		postingDate = time.Now().UTC()
	}
	description := input.Description
	if description == "" {
		description = fmt.Sprintf("Freight charge %d", input.FreightChargeID)
	}
	sourceID := freightSourceID(input.CompanyID, input.FreightChargeID)
	entry, err := gps.accounting.PostJournal(ctx, journals.PostingInput{
		PeriodID:     input.PeriodID,
		Date:         postingDate,
		SourceModule: freightSourceModule,
		SourceID:     sourceID,
		Memo:         description,
		PostedBy:     input.PostedBy,
		Lines:        []journals.PostingLineInput{debitLine, creditLine},
	})
	if err != nil {
		if isSourceAlreadyLinked(err) && charge.GLPostingID != nil {
			postingID := *charge.GLPostingID
			gps.posted.Store(key, postingID)
			return postingID, nil
		}
		return 0, fmt.Errorf("failed to post freight journal: %w", err)
	}
	if entry.ID <= 0 {
		return 0, errors.New("freight: accounting journal returned no posting ID")
	}
	if entry.SourceID != uuid.Nil && entry.SourceID != sourceID {
		return 0, fmt.Errorf("%w: journal source ID does not identify freight charge", ErrGLPostingIdentityMismatch)
	}
	if entry.SourceModule != "" && entry.SourceModule != freightSourceModule {
		return 0, fmt.Errorf("%w: journal source module %q is not %q", ErrGLPostingIdentityMismatch, entry.SourceModule, freightSourceModule)
	}
	if charge.GLPostingID != nil {
		if *charge.GLPostingID != entry.ID {
			return 0, fmt.Errorf("%w: freight charge has %d, accounting returned %d", ErrGLPostingIdentityMismatch, *charge.GLPostingID, entry.ID)
		}
		gps.posted.Store(key, entry.ID)
		return entry.ID, nil
	}

	glPostingID := entry.ID
	updated, err := gps.freightRepo.UpdateFreightCharge(ctx, input.CompanyID, input.FreightChargeID, FreightChargeUpdate{GLPostingID: &glPostingID})
	if err != nil {
		return 0, fmt.Errorf("failed to update freight charge with GL posting ID: %w", err)
	}
	if updated != nil && updated.GLPostingID != nil && *updated.GLPostingID != glPostingID {
		return 0, fmt.Errorf("%w: freight charge update returned %d, accounting returned %d", ErrGLPostingIdentityMismatch, *updated.GLPostingID, glPostingID)
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

	gps.posted.Store(key, glPostingID)
	return glPostingID, nil
}

type freightPostingKey struct {
	companyID int64
	chargeID  int64
}

func freightSourceID(companyID, chargeID int64) uuid.UUID {
	return uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("%s:%d:%d", freightSourceModule, companyID, chargeID)))
}

func parseGLAccountID(account string) (int64, error) {
	account = strings.TrimSpace(account)
	if account == "" {
		return 0, errors.New("account is required")
	}
	id, err := strconv.ParseInt(account, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("account %q must be a positive numeric account ID", account)
	}
	return id, nil
}

func isSourceAlreadyLinked(err error) bool {
	return errors.Is(err, accountingshared.ErrSourceAlreadyLinked) || strings.Contains(err.Error(), "uq_source_links")
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

	glAccount, err := gps.resolveFreightExpenseAccount(ctx, companyID, costCenterID)
	if err != nil {
		return 0, err
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
	glAccount, err := gps.resolveFreightExpenseAccount(ctx, companyID, costCenterID)
	if err != nil {
		return 0, err
	}

	input := PostFreightToGLInput{
		CompanyID:       companyID,
		FreightChargeID: freightChargeID,
		CostCenterID:    costCenterID,
		GLAccount:       glAccount,
		FreightAmount:   charge.FreightTotal,
		Description:     fmt.Sprintf("Freight payable to carrier %d", vendorID),
		PostedBy:        companyID,
	}

	return gps.PostFreightToGL(ctx, input)
}

func (gps *glPostingService) resolveFreightExpenseAccount(ctx context.Context, companyID, costCenterID int64) (string, error) {
	if costCenterID <= 0 {
		return freightExpenseAccountCode, nil
	}
	costCenter, err := gps.freightRepo.GetCostCenter(ctx, companyID, costCenterID)
	if err != nil {
		return "", fmt.Errorf("cost center not found: %w", err)
	}
	if costCenter != nil && costCenter.GLAccount != nil && strings.TrimSpace(*costCenter.GLAccount) != "" {
		return *costCenter.GLAccount, nil
	}
	return freightExpenseAccountCode, nil
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
	AccountCode  string                 // GL account code (e.g., "6100")
	AccountName  string                 // GL account name (e.g., "Freight Expense")
	Description  string                 // Transaction description
	DebitAmount  *accountingmoney.Money // Debit side (nil if credit)
	CreditAmount *accountingmoney.Money // Credit side (nil if debit)
	CostCenter   *string                // Cost center allocation
	Currency     string                 // Currency code
	TransDate    time.Time              // Transaction date
	Reference    string                 // Reference (e.g., invoice number)
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
