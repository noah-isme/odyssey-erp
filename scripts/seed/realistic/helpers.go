package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SeedContext holds cached IDs and shared state across sequential seed phases.
type SeedContext struct {
	Ctx                   context.Context
	Pool                  *pgxpool.Pool
	CompanyNTPID          int64 // PT Nusantara Teknik Perkasa (Parent / HQ)
	CompanyNDMID          int64 // PT Nusantara Distribusi Mandiri (Subsidiary)
	UserIDs               map[string]int64
	RoleIDs               map[string]int64
	BranchIDs             map[string]int64
	WarehouseIDs          map[string]int64
	DepartmentIDs         map[string]int64
	CostCenterIDs         map[string]int64
	UnitIDs               map[string]int64
	TaxIDs                map[string]int64
	CategoryIDs           map[string]int64
	ProductIDs            map[string]int64
	SupplierIDs           map[string]int64
	CustomerIDs           map[string]int64
	AccountIDs            map[string]int64
	PeriodIDs             map[string]int64
	AccountingPeriodIDs   map[string]int64
	TaxRuleVersionIDs     map[string]int64
	TaxVatRateIDs         map[string]int64
	TaxWithholdingTypeIDs map[string]int64
	TaxCodeIDs            map[string]int64
	TaxPeriodIDs          map[string]int64
	BankAccountIDs        map[string]int64
}

// NewSeedContext creates an initialized SeedContext with empty ID lookup maps.
func NewSeedContext(ctx context.Context, pool *pgxpool.Pool) *SeedContext {
	return &SeedContext{
		Ctx:                   ctx,
		Pool:                  pool,
		UserIDs:               make(map[string]int64),
		RoleIDs:               make(map[string]int64),
		BranchIDs:             make(map[string]int64),
		WarehouseIDs:          make(map[string]int64),
		DepartmentIDs:         make(map[string]int64),
		CostCenterIDs:         make(map[string]int64),
		UnitIDs:               make(map[string]int64),
		TaxIDs:                make(map[string]int64),
		CategoryIDs:           make(map[string]int64),
		ProductIDs:            make(map[string]int64),
		SupplierIDs:           make(map[string]int64),
		CustomerIDs:           make(map[string]int64),
		AccountIDs:            make(map[string]int64),
		PeriodIDs:             make(map[string]int64),
		AccountingPeriodIDs:   make(map[string]int64),
		TaxRuleVersionIDs:     make(map[string]int64),
		TaxVatRateIDs:         make(map[string]int64),
		TaxWithholdingTypeIDs: make(map[string]int64),
		TaxCodeIDs:            make(map[string]int64),
		TaxPeriodIDs:          make(map[string]int64),
		BankAccountIDs:        make(map[string]int64),
	}
}

// Getenv reads an environment variable with a default fallback.
func Getenv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

// ExecInTx wraps a domain phase execution inside an isolated database transaction.
func ExecInTx(ctx context.Context, pool *pgxpool.Pool, name string, fn func(tx pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%s: begin tx: %w", name, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(tx); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("%s: commit tx: %w", name, err)
	}
	return nil
}

// ParseDate parses a YYYY-MM-DD date string into a time.Time UTC.
func ParseDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(fmt.Sprintf("invalid date format %q: %v", s, err))
	}
	return t
}

// LookupCompanyID finds a company ID by its code.
func LookupCompanyID(ctx context.Context, tx pgx.Tx, code string) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM companies WHERE code = $1`, code).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("lookup company %q: %w", code, err)
	}
	return id, nil
}

// LookupBranchID finds a branch ID by its code.
func LookupBranchID(ctx context.Context, tx pgx.Tx, code string) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM branches WHERE code = $1`, code).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("lookup branch %q: %w", code, err)
	}
	return id, nil
}

// LookupWarehouseID finds a warehouse ID by its code.
func LookupWarehouseID(ctx context.Context, tx pgx.Tx, code string) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM warehouses WHERE code = $1`, code).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("lookup warehouse %q: %w", code, err)
	}
	return id, nil
}

// LookupUserID finds a user ID by email.
func LookupUserID(ctx context.Context, tx pgx.Tx, email string) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, email).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("lookup user %q: %w", email, err)
	}
	return id, nil
}

// LookupAccountID finds an account ID by account code.
func LookupAccountID(ctx context.Context, tx pgx.Tx, code string) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM accounts WHERE code = $1`, code).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("lookup account %q: %w", code, err)
	}
	return id, nil
}

// LookupProductID finds a product ID by SKU.
func LookupProductID(ctx context.Context, tx pgx.Tx, sku string) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM products WHERE sku = $1`, sku).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("lookup product %q: %w", sku, err)
	}
	return id, nil
}

// LookupSupplierID finds a supplier ID by code.
func LookupSupplierID(ctx context.Context, tx pgx.Tx, code string) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM suppliers WHERE code = $1`, code).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("lookup supplier %q: %w", code, err)
	}
	return id, nil
}

// LookupCustomerID finds a customer ID by company ID and customer code.
func LookupCustomerID(ctx context.Context, tx pgx.Tx, companyID int64, code string) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM customers WHERE company_id = $1 AND code = $2`, companyID, code).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("lookup customer %q (company %d): %w", code, companyID, err)
	}
	return id, nil
}

// LookupPeriodID finds a period ID by period code (YYYY-MM).
func LookupPeriodID(ctx context.Context, tx pgx.Tx, code string) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM periods WHERE code = $1`, code).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("lookup period %q: %w", code, err)
	}
	return id, nil
}

// LookupAccountingPeriodID finds an accounting period ID by period ID.
func LookupAccountingPeriodID(ctx context.Context, tx pgx.Tx, periodID int64) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM accounting_periods WHERE period_id = $1`, periodID).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("lookup accounting period for period %d: %w", periodID, err)
	}
	return id, nil
}
