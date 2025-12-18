package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	dsn := getenv("PG_DSN", "postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable")
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	// Phase 1: Core Auth & RBAC
	fmt.Println("→ Seeding users...")
	if err := seedUsers(ctx, pool); err != nil {
		log.Fatalf("seed users: %v", err)
	}
	fmt.Println("→ Seeding RBAC...")
	if err := seedRBAC(ctx, pool); err != nil {
		log.Fatalf("seed rbac: %v", err)
	}

	// Phase 2: Master Data
	fmt.Println("→ Seeding master data...")
	if err := seedMasterData(ctx, pool); err != nil {
		log.Fatalf("seed master data: %v", err)
	}

	// Phase 3: Accounting
	fmt.Println("→ Seeding accounting...")
	if err := seedAccounting(ctx, pool); err != nil {
		log.Fatalf("seed accounting: %v", err)
	}

	// Phase 4: Consolidation
	fmt.Println("→ Seeding consolidation...")
	if err := seedConsolidation(ctx, pool); err != nil {
		log.Fatalf("seed consolidation: %v", err)
	}

	// Phase 5: Operations - Procurement
	fmt.Println("→ Seeding procurement...")
	if err := seedProcurement(ctx, pool); err != nil {
		log.Fatalf("seed procurement: %v", err)
	}

	// Phase 6: Operations - Sales
	fmt.Println("→ Seeding sales...")
	if err := seedSales(ctx, pool); err != nil {
		log.Fatalf("seed sales: %v", err)
	}

	// Phase 7: Advanced Features
	fmt.Println("→ Seeding board pack templates...")
	if err := seedBoardPackTemplates(ctx, pool); err != nil {
		log.Fatalf("seed board pack templates: %v", err)
	}

	fmt.Println("✓ Seed complete at", time.Now().Format(time.RFC3339))
}

// =============================================================================
// USERS
// =============================================================================

func seedUsers(ctx context.Context, pool *pgxpool.Pool) error {
	users := []struct {
		email    string
		password string
	}{
		{"admin@odyssey.local", "admin123"},
		{"manager@odyssey.local", "manager123"},
		{"accountant@odyssey.local", "accountant123"},
	}

	for _, u := range users {
		hash, _ := bcrypt.GenerateFromPassword([]byte(u.password), bcrypt.DefaultCost)
		_, err := pool.Exec(ctx, `
			INSERT INTO users (email, password_hash, is_active, created_at, updated_at)
			VALUES ($1, $2, TRUE, NOW(), NOW())
			ON CONFLICT (email) DO NOTHING`, u.email, string(hash))
		if err != nil {
			return err
		}
	}
	return nil
}

// =============================================================================
// RBAC
// =============================================================================

func seedRBAC(ctx context.Context, pool *pgxpool.Pool) error {
	perms := []struct {
		name        string
		description string
	}{
		// Core platform permissions
		{"users.view", "View users"},
		{"users.edit", "Manage users"},
		{"roles.view", "View roles"},
		{"roles.edit", "Manage roles"},
		{"permissions.view", "View permissions"},
		{"org.view", "View organization data"},
		{"org.edit", "Manage organization data"},
		{"master.view", "View master data"},
		{"master.edit", "Manage master data"},
		{"master.import", "Import master data via CSV"},
		{"rbac.view", "View RBAC setup"},
		{"rbac.edit", "Manage RBAC configuration"},
		{"report.view", "Access reports"},
		// Inventory
		{"inventory.view", "View inventory transactions"},
		{"inventory.edit", "Post inventory transactions"},
		// Procurement
		{"procurement.view", "View procurement documents"},
		{"procurement.edit", "Manage procurement documents"},
		// Finance
		{"finance.ap.view", "View AP documents"},
		{"finance.ap.edit", "Manage AP documents"},
		{"finance.gl.view", "View General Ledger"},
		{"finance.view_analytics", "View Finance Analytics"},
		{"finance.export_analytics", "Export Finance Analytics"},
		{"finance.boardpack", "Generate Board Pack"},
		{"finance.ar.view", "View AR documents"},
		{"finance.ar.edit", "Manage AR documents"},
		// Sales
		{"sales.customer.view", "View customer data"},
		{"sales.customer.create", "Create new customers"},
		{"sales.customer.edit", "Edit customer information"},
		{"sales.quotation.view", "View sales quotations"},
		{"sales.quotation.create", "Create new quotations"},
		{"sales.quotation.edit", "Edit quotations"},
		{"sales.quotation.approve", "Approve or reject quotations"},
		{"sales.order.view", "View sales orders"},
		{"sales.order.create", "Create new sales orders"},
		{"sales.order.edit", "Edit sales orders"},
		{"sales.order.confirm", "Confirm sales orders"},
		{"sales.order.cancel", "Cancel sales orders"},
		// Consolidation
		{"finance.view_consolidation", "View consolidated financial reports"},
		{"finance.post_elimination", "Post elimination journal entries"},
		{"finance.manage_consolidation", "Manage rules and runs"},
		{"finance.export_consolidation", "Export consolidated data"},
		{"finance.period.close", "Manage period closing"},
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	for _, perm := range perms {
		if _, err := tx.Exec(ctx, `
			INSERT INTO permissions (name, description)
			VALUES ($1, $2)
			ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description`, perm.name, perm.description); err != nil {
			return err
		}
	}

	roles := []struct {
		name        string
		description string
		permissions []string
	}{
		{"admin", "Full access to all modules", []string{
			"users.view", "users.edit", "roles.view", "roles.edit", "permissions.view",
			"org.view", "org.edit", "master.view", "master.edit", "master.import",
			"rbac.view", "rbac.edit", "report.view",
			"inventory.view", "inventory.edit",
			"procurement.view", "procurement.edit",
			"finance.ap.view", "finance.ap.edit", "finance.boardpack", "finance.ar.view", "finance.ar.edit", "finance.gl.view",
			"finance.view_analytics", "finance.export_analytics",
			"sales.customer.view", "sales.customer.create", "sales.customer.edit",
			"sales.quotation.view", "sales.quotation.create", "sales.quotation.edit", "sales.quotation.approve",
			"sales.order.view", "sales.order.create", "sales.order.edit", "sales.order.confirm", "sales.order.cancel",
			"finance.view_consolidation", "finance.post_elimination", "finance.manage_consolidation", "finance.export_consolidation", "finance.period.close",
		}},
		{"manager", "Manage operations", []string{
			"org.view", "org.edit", "master.view", "master.edit", "master.import", "report.view",
			"inventory.view", "inventory.edit",
			"procurement.view", "procurement.edit",
			"finance.ap.view", "finance.boardpack", "finance.ar.view", "finance.ar.edit",
			"sales.customer.view", "sales.customer.create", "sales.customer.edit",
			"sales.quotation.view", "sales.quotation.create", "sales.quotation.edit", "sales.quotation.approve",
			"sales.order.view", "sales.order.create", "sales.order.edit", "sales.order.confirm", "sales.order.cancel",
		}},
		{"viewer", "Read-only access", []string{
			"org.view", "master.view", "report.view",
			"inventory.view", "procurement.view", "finance.ap.view",
			"sales.customer.view", "sales.quotation.view", "sales.order.view",
		}},
	}

	for _, role := range roles {
		var roleID int64
		err := tx.QueryRow(ctx, `
			INSERT INTO roles (name, description)
			VALUES ($1, $2)
			ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description, updated_at = NOW()
			RETURNING id`, role.name, role.description).Scan(&roleID)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id = $1`, roleID); err != nil {
			return err
		}
		for _, permName := range role.permissions {
			if _, err := tx.Exec(ctx, `
				INSERT INTO role_permissions (role_id, permission_id)
				SELECT $1, id FROM permissions WHERE name = $2
				ON CONFLICT DO NOTHING`, roleID, permName); err != nil {
				return err
			}
		}
	}

	// Assign roles to users
	userRoles := map[string]string{
		"admin@odyssey.local":      "admin",
		"manager@odyssey.local":    "manager",
		"accountant@odyssey.local": "viewer",
	}
	for email, roleName := range userRoles {
		var userID int64
		err := tx.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, email).Scan(&userID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1`, userID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO user_roles (user_id, role_id)
			SELECT $1, id FROM roles WHERE name = $2
			ON CONFLICT DO NOTHING`, userID, roleName); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// =============================================================================
// MASTER DATA
// =============================================================================

func seedMasterData(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Companies
	companies := []struct {
		code    string
		name    string
		address string
		taxID   string
	}{
		{"ODY-01", "PT Odyssey Utama", "Jl. Sudirman No. 100, Jakarta", "01.234.567.8-901.000"},
		{"ODY-02", "PT Odyssey Cabang", "Jl. Asia Afrika No. 50, Bandung", "02.345.678.9-012.000"},
	}
	for _, c := range companies {
		_, err := tx.Exec(ctx, `
			INSERT INTO companies (code, name, address, tax_id)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (code) DO NOTHING`, c.code, c.name, c.address, c.taxID)
		if err != nil {
			return err
		}
	}

	// Branches
	branches := []struct {
		companyCode string
		code        string
		name        string
		address     string
	}{
		{"ODY-01", "HQ-JKT", "Kantor Pusat Jakarta", "Jl. Sudirman No. 100, Jakarta"},
		{"ODY-01", "BR-SBY", "Cabang Surabaya", "Jl. Pemuda No. 45, Surabaya"},
		{"ODY-02", "BR-BDG", "Kantor Bandung", "Jl. Asia Afrika No. 50, Bandung"},
		{"ODY-02", "BR-SMG", "Cabang Semarang", "Jl. Pandanaran No. 20, Semarang"},
	}
	for _, b := range branches {
		_, err := tx.Exec(ctx, `
			INSERT INTO branches (company_id, code, name, address)
			SELECT c.id, $2, $3, $4 FROM companies c WHERE c.code = $1
			ON CONFLICT (code) DO NOTHING`, b.companyCode, b.code, b.name, b.address)
		if err != nil {
			return err
		}
	}

	// Warehouses
	warehouses := []struct {
		branchCode string
		code       string
		name       string
		address    string
	}{
		{"HQ-JKT", "WH-JKT-01", "Gudang Jakarta Pusat", "Jl. Industri No. 1, Jakarta"},
		{"BR-SBY", "WH-SBY-01", "Gudang Surabaya", "Jl. Margomulyo No. 10, Surabaya"},
		{"BR-BDG", "WH-BDG-01", "Gudang Bandung", "Jl. Soekarno Hatta No. 88, Bandung"},
		{"BR-SMG", "WH-SMG-01", "Gudang Semarang", "Jl. Siliwangi No. 55, Semarang"},
	}
	for _, w := range warehouses {
		_, err := tx.Exec(ctx, `
			INSERT INTO warehouses (branch_id, code, name, address)
			SELECT b.id, $2, $3, $4 FROM branches b WHERE b.code = $1
			ON CONFLICT (code) DO NOTHING`, w.branchCode, w.code, w.name, w.address)
		if err != nil {
			return err
		}
	}

	// Units
	units := []struct {
		code string
		name string
	}{
		{"PCS", "Pieces"},
		{"BOX", "Box"},
		{"KG", "Kilogram"},
		{"LTR", "Liter"},
		{"MTR", "Meter"},
		{"SET", "Set"},
		{"PKT", "Packet"},
	}
	for _, u := range units {
		_, err := tx.Exec(ctx, `
			INSERT INTO units (code, name)
			VALUES ($1, $2)
			ON CONFLICT (code) DO NOTHING`, u.code, u.name)
		if err != nil {
			return err
		}
	}

	// Taxes
	taxes := []struct {
		code string
		name string
		rate float64
	}{
		{"PPN", "PPN 11%", 11.00},
		{"PPH23", "PPh 23 - 2%", 2.00},
		{"NO-TAX", "Tanpa Pajak", 0.00},
	}
	for _, t := range taxes {
		_, err := tx.Exec(ctx, `
			INSERT INTO taxes (code, name, rate)
			VALUES ($1, $2, $3)
			ON CONFLICT (code) DO NOTHING`, t.code, t.name, t.rate)
		if err != nil {
			return err
		}
	}

	// Categories
	categories := []struct {
		code     string
		name     string
		parentID *int
	}{
		{"ELEC", "Electronics", nil},
		{"OFFICE", "Office Supplies", nil},
		{"RAW", "Raw Materials", nil},
		{"COMP", "Computer & Laptop", nil},
		{"FURNITURE", "Furniture", nil},
	}
	for _, c := range categories {
		_, err := tx.Exec(ctx, `
			INSERT INTO categories (code, name, parent_id)
			VALUES ($1, $2, $3)
			ON CONFLICT (code) DO NOTHING`, c.code, c.name, c.parentID)
		if err != nil {
			return err
		}
	}

	// Suppliers
	suppliers := []struct {
		code    string
		name    string
		phone   string
		email   string
		address string
	}{
		{"SUP-001", "PT Elektronik Jaya", "021-5551234", "sales@elektronikjaya.co.id", "Jl. Mangga Dua No. 10, Jakarta"},
		{"SUP-002", "CV Kertas Makmur", "022-4445678", "order@kertasmakmur.com", "Jl. Braga No. 55, Bandung"},
		{"SUP-003", "PT Baja Sentosa", "031-3339999", "info@bajasentosa.co.id", "Jl. Rungkut Industri No. 15, Surabaya"},
		{"SUP-004", "UD Mebel Indah", "024-7778888", "sales@mebelindah.com", "Jl. Pandanaran No. 30, Semarang"},
		{"SUP-005", "PT Komputer Nusantara", "021-6662222", "info@komputernusantara.co.id", "Jl. Gunung Sahari No. 88, Jakarta"},
	}
	for _, s := range suppliers {
		_, err := tx.Exec(ctx, `
			INSERT INTO suppliers (code, name, phone, email, address, is_active)
			VALUES ($1, $2, $3, $4, $5, TRUE)
			ON CONFLICT (code) DO NOTHING`, s.code, s.name, s.phone, s.email, s.address)
		if err != nil {
			return err
		}
	}

	// Products
	products := []struct {
		sku          string
		name         string
		categoryCode string
		unitCode     string
		price        float64
		taxCode      string
	}{
		{"PRD-001", "Laptop ASUS VivoBook 14", "ELEC", "PCS", 8500000, "PPN"},
		{"PRD-002", "Monitor LG 24 inch", "ELEC", "PCS", 2500000, "PPN"},
		{"PRD-003", "Kertas HVS A4 70gr", "OFFICE", "BOX", 45000, "PPN"},
		{"PRD-004", "Pulpen Pilot G2", "OFFICE", "PKT", 35000, "PPN"},
		{"PRD-005", "Baja Plat 2mm", "RAW", "KG", 18000, "PPN"},
		{"PRD-006", "Keyboard Logitech K120", "COMP", "PCS", 175000, "PPN"},
		{"PRD-007", "Mouse Wireless Logitech", "COMP", "PCS", 250000, "PPN"},
		{"PRD-008", "Meja Kerja Executive", "FURNITURE", "PCS", 3500000, "PPN"},
		{"PRD-009", "Kursi Kantor Ergonomis", "FURNITURE", "PCS", 2800000, "PPN"},
		{"PRD-010", "Printer Epson L3150", "ELEC", "PCS", 3200000, "PPN"},
	}
	for _, p := range products {
		_, err := tx.Exec(ctx, `
			INSERT INTO products (sku, name, category_id, unit_id, price, tax_id, is_active)
			SELECT $1, $2, c.id, u.id, $5, t.id, TRUE
			FROM categories c, units u, taxes t
			WHERE c.code = $3 AND u.code = $4 AND t.code = $6
			ON CONFLICT (sku) DO NOTHING`, p.sku, p.name, p.categoryCode, p.unitCode, p.price, p.taxCode)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// =============================================================================
// ACCOUNTING
// =============================================================================

func seedAccounting(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Chart of Accounts
	accounts := []struct {
		code     string
		name     string
		accType  string
		parentID *int64
	}{
		// Assets
		{"1000", "ASET", "ASSET", nil},
		{"1100", "Kas dan Bank", "ASSET", nil},
		{"1110", "Kas", "ASSET", nil},
		{"1120", "Bank BCA", "ASSET", nil},
		{"1130", "Bank Mandiri", "ASSET", nil},
		{"1200", "Piutang", "ASSET", nil},
		{"1210", "Piutang Usaha", "ASSET", nil},
		{"1220", "Piutang Karyawan", "ASSET", nil},
		{"1300", "Persediaan", "ASSET", nil},
		{"1310", "Persediaan Barang Dagang", "ASSET", nil},
		{"1400", "Aset Tetap", "ASSET", nil},
		{"1410", "Peralatan Kantor", "ASSET", nil},
		{"1420", "Kendaraan", "ASSET", nil},
		// Liabilities
		{"2000", "KEWAJIBAN", "LIABILITY", nil},
		{"2100", "Hutang Lancar", "LIABILITY", nil},
		{"2110", "Hutang Usaha", "LIABILITY", nil},
		{"2120", "Hutang Pajak", "LIABILITY", nil},
		{"2130", "Hutang Gaji", "LIABILITY", nil},
		// Equity
		{"3000", "EKUITAS", "EQUITY", nil},
		{"3100", "Modal Disetor", "EQUITY", nil},
		{"3200", "Laba Ditahan", "EQUITY", nil},
		// Revenue
		{"4000", "PENDAPATAN", "REVENUE", nil},
		{"4100", "Pendapatan Penjualan", "REVENUE", nil},
		{"4200", "Pendapatan Lain-lain", "REVENUE", nil},
		// Expenses
		{"5000", "BEBAN", "EXPENSE", nil},
		{"5100", "Beban Pokok Penjualan", "EXPENSE", nil},
		{"5200", "Beban Operasional", "EXPENSE", nil},
		{"5210", "Beban Gaji", "EXPENSE", nil},
		{"5220", "Beban Sewa", "EXPENSE", nil},
		{"5230", "Beban Listrik & Air", "EXPENSE", nil},
		{"5300", "Beban Administrasi", "EXPENSE", nil},
	}
	for _, a := range accounts {
		_, err := tx.Exec(ctx, `
			INSERT INTO accounts (code, name, type, is_active)
			VALUES ($1, $2, $3::account_type, TRUE)
			ON CONFLICT (code) DO NOTHING`, a.code, a.name, a.accType)
		if err != nil {
			return err
		}
	}

	// Periods for current year
	year := time.Now().Year()
	for month := 1; month <= 12; month++ {
		startDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
		endDate := startDate.AddDate(0, 1, -1)
		code := fmt.Sprintf("%d-%02d", year, month)

		_, err := tx.Exec(ctx, `
			INSERT INTO periods (code, start_date, end_date, status)
			VALUES ($1, $2, $3, 'OPEN')
			ON CONFLICT (code) DO NOTHING`, code, startDate, endDate)
		if err != nil {
			return err
		}
	}

	// Seed accounting_periods linked to periods
	_, err = tx.Exec(ctx, `
		INSERT INTO accounting_periods (period_id, company_id, name, start_date, end_date, status, created_at, updated_at)
		SELECT p.id, c.id, p.code || '-' || c.code, p.start_date, p.end_date, 'OPEN', NOW(), NOW()
		FROM periods p
		CROSS JOIN companies c
		ON CONFLICT DO NOTHING`)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// =============================================================================
// CONSOLIDATION
// =============================================================================

func seedConsolidation(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Consol Group
	var groupID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO consol_groups (name, reporting_currency, fx_enabled)
		VALUES ('Odyssey Group', 'IDR', FALSE)
		ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
		RETURNING id`).Scan(&groupID)
	if err != nil {
		return err
	}

	// Add all companies as members
	_, err = tx.Exec(ctx, `
		INSERT INTO consol_members (group_id, company_id, enabled)
		SELECT $1, c.id, TRUE FROM companies c
		ON CONFLICT (group_id, company_id) DO NOTHING`, groupID)
	if err != nil {
		return err
	}

	// Group-level accounts (simplified)
	groupAccounts := []struct {
		code    string
		name    string
		accType string
	}{
		{"G-1000", "Consolidated Assets", "ASSET"},
		{"G-2000", "Consolidated Liabilities", "LIABILITY"},
		{"G-3000", "Consolidated Equity", "EQUITY"},
		{"G-4000", "Consolidated Revenue", "REVENUE"},
		{"G-5000", "Consolidated Expenses", "EXPENSE"},
	}
	for _, ga := range groupAccounts {
		_, err := tx.Exec(ctx, `
			INSERT INTO consol_group_accounts (group_id, code, name, type)
			VALUES ($1, $2, $3, $4::account_type)
			ON CONFLICT (group_id, code) DO NOTHING`, groupID, ga.code, ga.name, ga.accType)
		if err != nil {
			return err
		}
	}

	// Map local accounts to group accounts by type
	_, err = tx.Exec(ctx, `
		INSERT INTO account_map (group_id, company_id, local_account_id, group_account_id)
		SELECT $1, cm.company_id, a.id, cga.id
		FROM consol_members cm
		JOIN accounts a ON TRUE
		JOIN consol_group_accounts cga ON cga.group_id = $1 AND cga.type = a.type
		WHERE cm.group_id = $1
		  AND a.code IN ('1100', '2100', '3100', '4100', '5100')
		ON CONFLICT (group_id, company_id, local_account_id) DO NOTHING`, groupID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// =============================================================================
// PROCUREMENT
// =============================================================================

func seedProcurement(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Get references
	var supplierID, warehouseID, productID, taxID int64
	err = tx.QueryRow(ctx, `SELECT id FROM suppliers WHERE code = 'SUP-001' LIMIT 1`).Scan(&supplierID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tx.Commit(ctx) // Skip if no suppliers
		}
		return err
	}
	err = tx.QueryRow(ctx, `SELECT id FROM warehouses WHERE code = 'WH-JKT-01' LIMIT 1`).Scan(&warehouseID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tx.Commit(ctx)
		}
		return err
	}
	err = tx.QueryRow(ctx, `SELECT id FROM products WHERE sku = 'PRD-001' LIMIT 1`).Scan(&productID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tx.Commit(ctx)
		}
		return err
	}
	err = tx.QueryRow(ctx, `SELECT id FROM taxes WHERE code = 'PPN' LIMIT 1`).Scan(&taxID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			taxID = 0
		}
	}

	// Purchase Orders
	pos := []struct {
		number   string
		status   string
		currency string
		note     string
	}{
		{"PO-202412-0001", "DRAFT", "IDR", "PO untuk kebutuhan kantor"},
		{"PO-202412-0002", "APPROVED", "IDR", "PO untuk restok"},
		{"PO-202412-0003", "CLOSED", "IDR", "PO sudah selesai"},
	}
	for _, po := range pos {
		var poID int64
		err := tx.QueryRow(ctx, `
			INSERT INTO pos (number, supplier_id, status, currency, note)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (number) DO UPDATE SET number = EXCLUDED.number
			RETURNING id`, po.number, supplierID, po.status, po.currency, po.note).Scan(&poID)
		if err != nil {
			return err
		}

		// PO Lines
		_, err = tx.Exec(ctx, `
			INSERT INTO po_lines (po_id, product_id, qty, price, tax_id, note)
			VALUES ($1, $2, 10, 8000000, $3, 'Item 1')
			ON CONFLICT DO NOTHING`, poID, productID, taxID)
		if err != nil {
			return err
		}
	}

	// Goods Receipt Notes
	var approvedPOID int64
	err = tx.QueryRow(ctx, `SELECT id FROM pos WHERE number = 'PO-202412-0002' LIMIT 1`).Scan(&approvedPOID)
	if err == nil {
		var grnID int64
		err = tx.QueryRow(ctx, `
			INSERT INTO grns (number, po_id, supplier_id, warehouse_id, status, note)
			VALUES ('GRN-202412-0001', $1, $2, $3, 'POSTED', 'Barang diterima lengkap')
			ON CONFLICT (number) DO UPDATE SET number = EXCLUDED.number
			RETURNING id`, approvedPOID, supplierID, warehouseID).Scan(&grnID)
		if err != nil {
			return err
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO grn_lines (grn_id, product_id, qty, unit_cost)
			VALUES ($1, $2, 10, 8000000)
			ON CONFLICT DO NOTHING`, grnID, productID)
		if err != nil {
			return err
		}

		// AP Invoice
		_, err = tx.Exec(ctx, `
			INSERT INTO ap_invoices (number, supplier_id, grn_id, currency, total, status, issued_at, due_at)
			VALUES ('INV-AP-202412-0001', $1, $2, 'IDR', 88800000, 'POSTED', CURRENT_DATE, CURRENT_DATE + 30)
			ON CONFLICT (number) DO NOTHING`, supplierID, grnID)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// =============================================================================
// SALES
// =============================================================================

func seedSales(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Get admin user and first company
	var adminID, companyID int64
	err = tx.QueryRow(ctx, `SELECT id FROM users WHERE email = 'admin@odyssey.local' LIMIT 1`).Scan(&adminID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tx.Commit(ctx)
		}
		return err
	}
	err = tx.QueryRow(ctx, `SELECT id FROM companies WHERE code = 'ODY-01' LIMIT 1`).Scan(&companyID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tx.Commit(ctx)
		}
		return err
	}

	// Seed Customers (Phase 9 version with company_id)
	customers := []struct {
		code  string
		name  string
		email string
		phone string
	}{
		{"CUST-000001", "PT Maju Bersama", "purchasing@majubersama.co.id", "021-5550001"},
		{"CUST-000002", "CV Sukses Selalu", "order@suksesselalu.com", "022-5550002"},
		{"CUST-000003", "PT Sejahtera Abadi", "procurement@sejahteraabadi.co.id", "031-5550003"},
		{"CUST-000004", "UD Makmur Jaya", "sales@makmurjaya.com", "024-5550004"},
		{"CUST-000005", "PT Global Trade", "info@globaltrade.co.id", "021-5550005"},
	}
	for _, c := range customers {
		_, err := tx.Exec(ctx, `
			INSERT INTO customers (code, name, company_id, email, phone, country, is_active, created_by)
			VALUES ($1, $2, $3, $4, $5, 'ID', TRUE, $6)
			ON CONFLICT (company_id, code) DO NOTHING`, c.code, c.name, companyID, c.email, c.phone, adminID)
		if err != nil {
			return err
		}
	}

	// Get customer and product IDs
	var customerID, productID int64
	err = tx.QueryRow(ctx, `SELECT id FROM customers WHERE code = 'CUST-000001' AND company_id = $1 LIMIT 1`, companyID).Scan(&customerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tx.Commit(ctx)
		}
		return err
	}
	err = tx.QueryRow(ctx, `SELECT id FROM products WHERE sku = 'PRD-001' LIMIT 1`).Scan(&productID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tx.Commit(ctx)
		}
		return err
	}

	// Quotations
	quotations := []struct {
		docNumber string
		status    string
		notes     string
	}{
		{"QUO-202412-0001", "DRAFT", "Quotation untuk kebutuhan kantor"},
		{"QUO-202412-0002", "APPROVED", "Quotation disetujui"},
		{"QUO-202412-0003", "CONVERTED", "Quotation sudah jadi SO"},
	}
	for _, q := range quotations {
		var quoID int64
		err := tx.QueryRow(ctx, `
			INSERT INTO quotations (doc_number, company_id, customer_id, quote_date, valid_until, status, currency, notes, created_by)
			VALUES ($1, $2, $3, CURRENT_DATE, CURRENT_DATE + 30, $4::quotation_status, 'IDR', $5, $6)
			ON CONFLICT (doc_number) DO UPDATE SET doc_number = EXCLUDED.doc_number
			RETURNING id`, q.docNumber, companyID, customerID, q.status, q.notes, adminID).Scan(&quoID)
		if err != nil {
			return err
		}

		// Quotation line
		_, err = tx.Exec(ctx, `
			INSERT INTO quotation_lines (quotation_id, product_id, description, quantity, uom, unit_price, discount_percent, tax_percent, tax_amount, line_total, line_order)
			VALUES ($1, $2, 'Laptop ASUS VivoBook 14', 5, 'PCS', 8500000, 0, 11, 4675000, 47175000, 1)
			ON CONFLICT DO NOTHING`, quoID, productID)
		if err != nil {
			return err
		}
	}

	// Sales Orders
	salesOrders := []struct {
		docNumber string
		status    string
		notes     string
	}{
		{"SO-202412-0001", "DRAFT", "Sales Order draft"},
		{"SO-202412-0002", "CONFIRMED", "Sales Order dikonfirmasi"},
		{"SO-202412-0003", "PROCESSING", "Sales Order dalam proses pengiriman"},
	}
	for _, so := range salesOrders {
		var soID int64
		err := tx.QueryRow(ctx, `
			INSERT INTO sales_orders (doc_number, company_id, customer_id, order_date, expected_delivery_date, status, currency, notes, created_by)
			VALUES ($1, $2, $3, CURRENT_DATE, CURRENT_DATE + 7, $4::sales_order_status, 'IDR', $5, $6)
			ON CONFLICT (doc_number) DO UPDATE SET doc_number = EXCLUDED.doc_number
			RETURNING id`, so.docNumber, companyID, customerID, so.status, so.notes, adminID).Scan(&soID)
		if err != nil {
			return err
		}

		// SO line
		_, err = tx.Exec(ctx, `
			INSERT INTO sales_order_lines (sales_order_id, product_id, description, quantity, uom, unit_price, discount_percent, tax_percent, tax_amount, line_total, line_order)
			VALUES ($1, $2, 'Laptop ASUS VivoBook 14', 5, 'PCS', 8500000, 0, 11, 4675000, 47175000, 1)
			ON CONFLICT DO NOTHING`, soID, productID)
		if err != nil {
			return err
		}
	}

	// Delivery Orders
	var confirmedSOID, warehouseID int64
	err = tx.QueryRow(ctx, `SELECT id FROM sales_orders WHERE doc_number = 'SO-202412-0002' LIMIT 1`).Scan(&confirmedSOID)
	if err == nil {
		err = tx.QueryRow(ctx, `SELECT id FROM warehouses WHERE code = 'WH-JKT-01' LIMIT 1`).Scan(&warehouseID)
		if err == nil {
			var doID int64
			err = tx.QueryRow(ctx, `
				INSERT INTO delivery_orders (doc_number, company_id, sales_order_id, warehouse_id, customer_id, delivery_date, status, driver_name, vehicle_number, notes, created_by)
				VALUES ('DO-202412-00001', $1, $2, $3, $4, CURRENT_DATE, 'DRAFT'::delivery_order_status, 'Budi Santoso', 'B 1234 ABC', 'Pengiriman ke Jakarta', $5)
				ON CONFLICT (doc_number) DO UPDATE SET doc_number = EXCLUDED.doc_number
				RETURNING id`, companyID, confirmedSOID, warehouseID, customerID, adminID).Scan(&doID)
			if err != nil {
				return err
			}

			// Get SO line for DO line
			var soLineID int64
			err = tx.QueryRow(ctx, `SELECT id FROM sales_order_lines WHERE sales_order_id = $1 LIMIT 1`, confirmedSOID).Scan(&soLineID)
			if err == nil {
				_, err = tx.Exec(ctx, `
					INSERT INTO delivery_order_lines (delivery_order_id, sales_order_line_id, product_id, quantity_to_deliver, uom, unit_price, line_order)
					VALUES ($1, $2, $3, 5, 'PCS', 8500000, 1)
					ON CONFLICT DO NOTHING`, doID, soLineID, productID)
				if err != nil {
					return err
				}
			}
		}
	}

	return tx.Commit(ctx)
}

// =============================================================================
// BOARD PACK TEMPLATES
// =============================================================================

func seedBoardPackTemplates(ctx context.Context, pool *pgxpool.Pool) error {
	const name = "Standard Executive Pack"
	var exists bool
	err := pool.QueryRow(ctx, `SELECT TRUE FROM board_pack_templates WHERE name = $1 LIMIT 1`, name).Scan(&exists)
	if err == nil {
		return nil // Already exists
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	var createdBy int64
	if err := pool.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, "admin@odyssey.local").Scan(&createdBy); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if err := pool.QueryRow(ctx, `SELECT id FROM users ORDER BY id LIMIT 1`).Scan(&createdBy); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	sections := []map[string]any{
		{"type": "EXEC_SUMMARY", "title": "Executive Summary"},
		{"type": "PL_SUMMARY", "title": "Profit & Loss"},
		{"type": "BS_SUMMARY", "title": "Balance Sheet"},
		{"type": "CASHFLOW_SUMMARY", "title": "Cashflow"},
		{"type": "TOP_VARIANCES", "title": "Top Variances", "options": map[string]any{"limit": 10}},
	}
	payload, err := json.Marshal(sections)
	if err != nil {
		return err
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO board_pack_templates (name, description, sections, is_default, is_active, created_by)
		VALUES ($1,$2,$3,TRUE,TRUE,$4)`, name, "Default sections covering summary, PL, BS, cashflow, dan variance.", payload, createdBy)
	return err
}

// =============================================================================
// HELPERS
// =============================================================================

func getenv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
