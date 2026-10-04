package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type Fixtures struct {
	CompanyID          int64
	BranchID           int64
	OtherCompanyID     int64
	OtherBranchID      int64
	CustomerID         int64
	SupplierID         int64
	ProductID          int64
	WarehouseID        int64
	GRNID              int64
	DocumentCategoryID int64
	DocumentClassID    int64
	Amount             string
	AdminEmail         string
	AdminPassword      string
	BranchEmail        string
	BranchPassword     string
	NoAccessEmail      string
	NoAccessPassword   string
}

func main() {
	var (
		dsnFlag    = flag.String("dsn", "", "PostgreSQL connection string (defaults to PG_DSN env var)")
		applyGH    = flag.Bool("apply-gh", false, "Automatically set variables in GitHub staging environment via gh CLI")
		exportEnv  = flag.Bool("export", false, "Print shell export statements for fixture variables and secrets")
		jsonOutput = flag.Bool("json", false, "Print JSON output of fixtures")

		iso004        = flag.Bool("iso004", false, "Create ISO-004 worker-injection fixtures only (never runs the base seeder; requires --confirm-staging, --iso004-key and APP_ENV=staging)")
		iso004Key     = flag.String("iso004-key", "", "Run key for ISO-004 fixtures (pass the ISO-004 --run-id); required with --iso004")
		confirmTarget = flag.String("confirm-staging", "", "With --iso004: <db-name>@<db-host> that must match the DSN host and current_database()")
		denyHostRegex = flag.String("deny-host-regex", iso004DefaultDenyHosts, "With --iso004: refuse when the DSN host matches this case-insensitive regex")
		dryRun        = flag.Bool("dry-run", false, "With --iso004: run the guard and SELECT-only resolution, print the planned inserts, write nothing")
	)
	flag.Parse()

	dsn := *dsnFlag
	if dsn == "" {
		dsn = os.Getenv("PG_DSN")
	}
	explicitDSN := dsn != ""

	if *dryRun && !*iso004 {
		log.Fatalf("--dry-run is only supported with --iso004")
	}
	if *iso004 {
		runISO004Main(iso004Options{
			Guard: iso004GuardInput{
				DSN:            dsn,
				Confirm:        *confirmTarget,
				AppEnv:         os.Getenv("APP_ENV"),
				DenyHostRegex:  *denyHostRegex,
				Key:            *iso004Key,
				ExplicitDSNSet: explicitDSN,
			},
			AdminEmail: getenv("STAGING_CERT_ADMIN_EMAIL", "admin@staging.odyssey.local"),
			DryRun:     *dryRun,
		}, *applyGH, *exportEnv, *jsonOutput)
		return
	}

	if dsn == "" {
		dsn = "postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("ping postgres: %v", err)
	}

	fixtures, err := seedStagingFixtures(ctx, pool)
	if err != nil {
		log.Fatalf("seed staging fixtures: %v", err)
	}

	printSummary(fixtures, *applyGH, *exportEnv, *jsonOutput)

	if *applyGH {
		if err := applyToGitHub(fixtures); err != nil {
			log.Fatalf("apply to GitHub staging environment: %v", err)
		}
	}
}

func seedStagingFixtures(ctx context.Context, pool *pgxpool.Pool) (*Fixtures, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// 1. Ensure Companies
	var primaryCompanyID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO companies (code, name, address, tax_id)
		VALUES ('ODY-01', 'PT Odyssey Utama', 'Jl. Sudirman No. 100, Jakarta', '01.234.567.8-901.000')
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name
		RETURNING id`).Scan(&primaryCompanyID)
	if err != nil {
		return nil, fmt.Errorf("seed primary company: %w", err)
	}

	var otherCompanyID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO companies (code, name, address, tax_id)
		VALUES ('ODY-02', 'PT Odyssey Cabang', 'Jl. Asia Afrika No. 50, Bandung', '02.345.678.9-012.000')
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name
		RETURNING id`).Scan(&otherCompanyID)
	if err != nil {
		return nil, fmt.Errorf("seed other company: %w", err)
	}

	// 2. Ensure Branches
	var primaryBranchID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO branches (company_id, code, name, address)
		VALUES ($1, 'HQ-JKT', 'Kantor Pusat Jakarta', 'Jl. Sudirman No. 100, Jakarta')
		ON CONFLICT (code) DO UPDATE SET company_id = EXCLUDED.company_id, name = EXCLUDED.name
		RETURNING id`, primaryCompanyID).Scan(&primaryBranchID)
	if err != nil {
		return nil, fmt.Errorf("seed primary branch: %w", err)
	}

	var otherBranchID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO branches (company_id, code, name, address)
		VALUES ($1, 'BR-BDG', 'Kantor Bandung', 'Jl. Asia Afrika No. 50, Bandung')
		ON CONFLICT (code) DO UPDATE SET company_id = EXCLUDED.company_id, name = EXCLUDED.name
		RETURNING id`, otherCompanyID).Scan(&otherBranchID)
	if err != nil {
		return nil, fmt.Errorf("seed other branch: %w", err)
	}

	// 3. Ensure Warehouses
	var warehouseID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO warehouses (branch_id, code, name, address)
		VALUES ($1, 'WH-JKT-01', 'Gudang Jakarta Pusat', 'Jl. Industri No. 1, Jakarta')
		ON CONFLICT (code) DO UPDATE SET branch_id = EXCLUDED.branch_id, name = EXCLUDED.name
		RETURNING id`, primaryBranchID).Scan(&warehouseID)
	if err != nil {
		return nil, fmt.Errorf("seed primary warehouse: %w", err)
	}

	// 4. Ensure Units & Categories for Master Data
	var unitID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO units (code, name)
		VALUES ('EA', 'Each')
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name
		RETURNING id`).Scan(&unitID)
	if err != nil {
		return nil, fmt.Errorf("seed unit: %w", err)
	}

	var categoryID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO categories (code, name, company_id)
		VALUES ('CAT-GEN', 'General Category', $1)
		ON CONFLICT (code) DO UPDATE SET company_id = EXCLUDED.company_id
		RETURNING id`, primaryCompanyID).Scan(&categoryID)
	if err != nil {
		return nil, fmt.Errorf("seed master category: %w", err)
	}

	// 5. Ensure Product in Primary Company
	var productID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO products (sku, name, category_id, unit_id, price, is_active, company_id)
		VALUES ('PROD-STG-01', 'v0.10 Certification Product', $1, $2, 100000.00, TRUE, $3)
		ON CONFLICT (sku) DO UPDATE SET company_id = EXCLUDED.company_id, is_active = TRUE
		RETURNING id`, categoryID, unitID, primaryCompanyID).Scan(&productID)
	if err != nil {
		return nil, fmt.Errorf("seed product: %w", err)
	}

	// Inventory balance for product in warehouse (for stock take & valuation)
	_, err = tx.Exec(ctx, `
		INSERT INTO inventory_balances (warehouse_id, product_id, qty, avg_cost, updated_at)
		VALUES ($1, $2, 100.0000, 50000.0000, NOW())
		ON CONFLICT (warehouse_id, product_id) DO UPDATE
		SET avg_cost = GREATEST(inventory_balances.avg_cost, 1000.0000),
		    updated_at = NOW()`, warehouseID, productID)
	if err != nil {
		return nil, fmt.Errorf("seed inventory balance: %w", err)
	}

	// 6. Ensure Customer & Supplier in Primary Company
	var customerID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO customers (code, name, phone, email, address, is_active, company_id)
		VALUES ('CUST-STG-01', 'PT Pelanggan Sertifikasi Staging', '081234567890', 'customer.staging@odyssey.local', 'Jl. Pelanggan No. 1', TRUE, $1)
		ON CONFLICT (code) DO UPDATE SET company_id = EXCLUDED.company_id, is_active = TRUE
		RETURNING id`, primaryCompanyID).Scan(&customerID)
	if err != nil {
		return nil, fmt.Errorf("seed customer: %w", err)
	}

	var supplierID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO suppliers (code, name, phone, email, address, is_active, company_id)
		VALUES ('SUPP-STG-01', 'PT Pemasok Sertifikasi Staging', '081234567891', 'supplier.staging@odyssey.local', 'Jl. Pemasok No. 1', TRUE, $1)
		ON CONFLICT (code) DO UPDATE SET company_id = EXCLUDED.company_id, is_active = TRUE
		RETURNING id`, primaryCompanyID).Scan(&supplierID)
	if err != nil {
		return nil, fmt.Errorf("seed supplier: %w", err)
	}

	// 7. Ensure Customer & AR Invoice in Other Company (for tenant isolation test)
	var otherCustomerID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO customers (code, name, phone, email, address, is_active, company_id)
		VALUES ('CUST-OTHER-01', 'PT Pelanggan Cabang Bandung', '081234567892', 'customer.other@odyssey.local', 'Jl. Asia Afrika No. 51', TRUE, $1)
		ON CONFLICT (code) DO UPDATE SET company_id = EXCLUDED.company_id, is_active = TRUE
		RETURNING id`, otherCompanyID).Scan(&otherCustomerID)
	if err != nil {
		return nil, fmt.Errorf("seed other customer: %w", err)
	}

	var otherInvoiceCount int64
	err = tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM ar_invoices i
		JOIN customers c ON c.id = i.customer_id
		WHERE c.company_id = $1`, otherCompanyID).Scan(&otherInvoiceCount)
	if err != nil {
		return nil, fmt.Errorf("check other ar invoices: %w", err)
	}

	if otherInvoiceCount == 0 {
		arNumber := fmt.Sprintf("AR-OTHER-%d", time.Now().Unix())
		_, err = tx.Exec(ctx, `
			INSERT INTO ar_invoices (number, customer_id, currency, total, status, due_at, created_at, updated_at)
			VALUES ($1, $2, 'IDR', 100000.00, 'POSTED', NOW() + INTERVAL '30 days', NOW(), NOW())`,
			arNumber, otherCustomerID)
		if err != nil {
			return nil, fmt.Errorf("seed other ar invoice: %w", err)
		}
	}

	// 8. Ensure Un-invoiced POSTED GRN in Primary Company
	var grnID int64
	err = tx.QueryRow(ctx, `
		SELECT g.id
		FROM grns g
		WHERE g.company_id = $1 AND g.status = 'POSTED'
		  AND NOT EXISTS (SELECT 1 FROM ap_invoices ai WHERE ai.grn_id = g.id)
		ORDER BY g.id DESC
		LIMIT 1`, primaryCompanyID).Scan(&grnID)
	if err != nil && err != pgx.ErrNoRows {
		return nil, fmt.Errorf("query existing uninvoiced grn: %w", err)
	}

	if grnID == 0 {
		grnNumber := fmt.Sprintf("GRN-STG-%d", time.Now().UnixNano())
		err = tx.QueryRow(ctx, `
			INSERT INTO grns (number, supplier_id, warehouse_id, status, received_at, note, created_at, company_id)
			VALUES ($1, $2, $3, 'POSTED', NOW(), 'v0.10-core staging certification GRN', NOW(), $4)
			RETURNING id`, grnNumber, supplierID, warehouseID, primaryCompanyID).Scan(&grnID)
		if err != nil {
			return nil, fmt.Errorf("create posted grn: %w", err)
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO grn_lines (grn_id, product_id, qty, unit_cost)
			VALUES ($1, $2, 10.0000, 50000.0000)`, grnID, productID)
		if err != nil {
			return nil, fmt.Errorf("create grn line: %w", err)
		}
	}

	// 9. Users & Authentication
	adminEmail := getenv("STAGING_CERT_ADMIN_EMAIL", "admin@staging.odyssey.local")
	adminPass := getenv("STAGING_CERT_ADMIN_PASSWORD", "admin123")
	branchEmail := getenv("STAGING_CERT_BRANCH_EMAIL", "branch@staging.odyssey.local")
	branchPass := getenv("STAGING_CERT_BRANCH_PASSWORD", "branch123")
	noAccessEmail := getenv("STAGING_CERT_NO_ACCESS_EMAIL", "noaccess@staging.odyssey.local")
	noAccessPass := getenv("STAGING_CERT_NO_ACCESS_PASSWORD", "noaccess123")

	adminHash, _ := bcrypt.GenerateFromPassword([]byte(adminPass), bcrypt.DefaultCost)
	branchHash, _ := bcrypt.GenerateFromPassword([]byte(branchPass), bcrypt.DefaultCost)
	noAccessHash, _ := bcrypt.GenerateFromPassword([]byte(noAccessPass), bcrypt.DefaultCost)

	var adminUserID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, is_active, created_at, updated_at)
		VALUES ($1, $2, TRUE, NOW(), NOW())
		ON CONFLICT (email) DO UPDATE SET password_hash = EXCLUDED.password_hash, is_active = TRUE, updated_at = NOW()
		RETURNING id`, adminEmail, string(adminHash)).Scan(&adminUserID)
	if err != nil {
		return nil, fmt.Errorf("seed admin user: %w", err)
	}

	var branchUserID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, is_active, created_at, updated_at)
		VALUES ($1, $2, TRUE, NOW(), NOW())
		ON CONFLICT (email) DO UPDATE SET password_hash = EXCLUDED.password_hash, is_active = TRUE, updated_at = NOW()
		RETURNING id`, branchEmail, string(branchHash)).Scan(&branchUserID)
	if err != nil {
		return nil, fmt.Errorf("seed branch user: %w", err)
	}

	var noAccessUserID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, is_active, created_at, updated_at)
		VALUES ($1, $2, TRUE, NOW(), NOW())
		ON CONFLICT (email) DO UPDATE SET password_hash = EXCLUDED.password_hash, is_active = TRUE, updated_at = NOW()
		RETURNING id`, noAccessEmail, string(noAccessHash)).Scan(&noAccessUserID)
	if err != nil {
		return nil, fmt.Errorf("seed no-access user: %w", err)
	}

	// Find or create admin role
	var adminRoleID int64
	err = tx.QueryRow(ctx, `SELECT id FROM roles WHERE LOWER(TRIM(name)) = 'admin' LIMIT 1`).Scan(&adminRoleID)
	if err != nil {
		err = tx.QueryRow(ctx, `
			INSERT INTO roles (name, description, created_at, updated_at)
			VALUES ('admin', 'System Administrator', NOW(), NOW())
			RETURNING id`).Scan(&adminRoleID)
		if err != nil {
			return nil, fmt.Errorf("seed admin role: %w", err)
		}
	}

	// Assign admin role to admin user
	_, err = tx.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, adminUserID, adminRoleID)
	if err != nil {
		return nil, fmt.Errorf("assign admin role: %w", err)
	}

	// Ensure branch user has NO active global user_roles that grant finance permissions
	_, _ = tx.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1`, branchUserID)
	_, _ = tx.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1`, noAccessUserID)

	// 10. Expired Role Assignment and Completed REVOKE Access Review (ISO-003)
	// Seed an expired branch role assignment for the branch user
	var expiredCount int64
	err = tx.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM rbac_user_role_assignments
		WHERE company_id = $1 AND user_id = $2 AND branch_id = $3
		  AND valid_to IS NOT NULL AND valid_to <= NOW()`,
		primaryCompanyID, branchUserID, primaryBranchID).Scan(&expiredCount)
	if err != nil {
		return nil, fmt.Errorf("query expired rbac assignments: %w", err)
	}

	if expiredCount == 0 {
		_, err = tx.Exec(ctx, `
			INSERT INTO rbac_user_role_assignments (
				company_id, user_id, role_id, branch_id, valid_from, valid_to, created_at
			)
			VALUES ($1, $2, $3, $4, NOW() - INTERVAL '30 days', NOW() - INTERVAL '1 day', NOW() - INTERVAL '30 days')`,
			primaryCompanyID, branchUserID, adminRoleID, primaryBranchID)
		if err != nil {
			return nil, fmt.Errorf("insert expired rbac assignment: %w", err)
		}
	}

	// Seed completed REVOKE access review
	var reviewCount int64
	err = tx.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM rbac_access_reviews
		WHERE company_id = $1 AND subject_user_id = $2
		  AND status = 'COMPLETED' AND decision = 'REVOKE'
		  AND decided_by_user_id IS NOT NULL AND decided_at IS NOT NULL`,
		primaryCompanyID, branchUserID).Scan(&reviewCount)
	if err != nil {
		return nil, fmt.Errorf("query rbac access review: %w", err)
	}

	if reviewCount == 0 {
		_, err = tx.Exec(ctx, `
			INSERT INTO rbac_access_reviews (
				company_id, subject_user_id, review_key, status, decision,
				opened_by_user_id, decided_by_user_id, decided_at, created_at, updated_at
			)
			VALUES (
				$1, $2, 'STAGING-REVOKE-ISO003', 'COMPLETED', 'REVOKE',
				$3, $3, NOW() - INTERVAL '1 day', NOW() - INTERVAL '1 day', NOW() - INTERVAL '1 day'
			)
			ON CONFLICT (company_id, subject_user_id, review_key) DO UPDATE
			SET status = 'COMPLETED', decision = 'REVOKE',
			    decided_by_user_id = EXCLUDED.decided_by_user_id,
			    decided_at = EXCLUDED.decided_at,
			    updated_at = NOW()`,
			primaryCompanyID, branchUserID, adminUserID)
		if err != nil {
			return nil, fmt.Errorf("insert rbac access review: %w", err)
		}
	}

	// 11. Document Management & 7-Year Retention Policy (J-DOC-001)
	// Numbering rule for documents
	_, err = tx.Exec(ctx, `
		INSERT INTO document_numbering_rules (
			company_id, code, name, prefix, pattern, scope, active, created_by
		)
		VALUES ($1, 'DOC-RULE-STG', 'Staging Document Rule', 'DOC', '{PREFIX}-{YYYY}-{SEQ:04d}', 'COMPANY', TRUE, $2)
		ON CONFLICT (company_id, code) DO NOTHING`,
		primaryCompanyID, adminUserID)
	if err != nil {
		return nil, fmt.Errorf("seed document numbering rule: %w", err)
	}

	// Document Classification
	var docClassID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO document_classifications (
			company_id, code, name, active, created_by
		)
		VALUES ($1, 'CONFIDENTIAL', 'Confidential Document', TRUE, $2)
		ON CONFLICT (company_id, code) DO UPDATE SET active = TRUE
		RETURNING id`, primaryCompanyID, adminUserID).Scan(&docClassID)
	if err != nil {
		return nil, fmt.Errorf("seed document classification: %w", err)
	}

	// Document Category
	var docCategoryID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO document_categories (
			company_id, code, name, active, created_by
		)
		VALUES ($1, NULL, 'CERT-DOCS', 'Certification Documents', TRUE, $2)
		ON CONFLICT (company_id, parent_id, code) DO UPDATE SET active = TRUE
		RETURNING id`, primaryCompanyID, adminUserID).Scan(&docCategoryID)
	if err != nil {
		// Try without parent_id constraint if needed
		err = tx.QueryRow(ctx, `
			SELECT id FROM document_categories WHERE company_id = $1 AND code = 'CERT-DOCS' LIMIT 1`,
			primaryCompanyID).Scan(&docCategoryID)
		if err != nil {
			return nil, fmt.Errorf("seed document category: %w", err)
		}
	}

	// Retention Policy (COMPLIANCE 7 years = 2557 days)
	var policyID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO retention_policies (
			company_id, code, name, description, trigger_event,
			retention_period_days, classification_ids, category_ids,
			active, created_by
		)
		VALUES (
			$1, 'RET-7Y-COMPLIANCE', '7-Year Compliance Retention',
			'Statutory 7-year immutable retention policy for staging certification',
			'APPROVAL', 2557, ARRAY[$2::bigint], ARRAY[$3::bigint], TRUE, $4
		)
		ON CONFLICT (company_id, code) DO UPDATE
		SET active = TRUE,
		    retention_period_days = 2557,
		    classification_ids = ARRAY[$2::bigint],
		    category_ids = ARRAY[$3::bigint]
		RETURNING id`,
		primaryCompanyID, docClassID, docCategoryID, adminUserID).Scan(&policyID)
	if err != nil {
		return nil, fmt.Errorf("seed retention policy: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return &Fixtures{
		CompanyID:          primaryCompanyID,
		BranchID:           primaryBranchID,
		OtherCompanyID:     otherCompanyID,
		OtherBranchID:      otherBranchID,
		CustomerID:         customerID,
		SupplierID:         supplierID,
		ProductID:          productID,
		WarehouseID:        warehouseID,
		GRNID:              grnID,
		DocumentCategoryID: docCategoryID,
		DocumentClassID:    docClassID,
		Amount:             "100000",
		AdminEmail:         adminEmail,
		AdminPassword:      adminPass,
		BranchEmail:        branchEmail,
		BranchPassword:     branchPass,
		NoAccessEmail:      noAccessEmail,
		NoAccessPassword:   noAccessPass,
	}, nil
}

func printSummary(f *Fixtures, applied bool, export bool, jsonOut bool) {
	if jsonOut {
		fmt.Printf(`{
  "STAGING_CERT_COMPANY_ID": %d,
  "STAGING_CERT_BRANCH_ID": %d,
  "STAGING_CERT_OTHER_COMPANY_ID": %d,
  "STAGING_CERT_OTHER_BRANCH_ID": %d,
  "STAGING_CERT_CUSTOMER_ID": %d,
  "STAGING_CERT_SUPPLIER_ID": %d,
  "STAGING_CERT_PRODUCT_ID": %d,
  "STAGING_CERT_WAREHOUSE_ID": %d,
  "STAGING_CERT_GRN_ID": %d,
  "STAGING_CERT_DOCUMENT_CATEGORY_ID": %d,
  "STAGING_CERT_DOCUMENT_CLASSIFICATION_ID": %d,
  "STAGING_CERT_AMOUNT": "%s"
}
`,
			f.CompanyID, f.BranchID, f.OtherCompanyID, f.OtherBranchID,
			f.CustomerID, f.SupplierID, f.ProductID, f.WarehouseID,
			f.GRNID, f.DocumentCategoryID, f.DocumentClassID, f.Amount)
		return
	}

	fmt.Println("================================================================================")
	fmt.Println("             ODYSSEY v0.10-CORE STAGING CERTIFICATION FIXTURES                  ")
	fmt.Println("================================================================================")
	fmt.Printf("1.  STAGING_CERT_COMPANY_ID                 = %d\n", f.CompanyID)
	fmt.Printf("2.  STAGING_CERT_BRANCH_ID                  = %d\n", f.BranchID)
	fmt.Printf("3.  STAGING_CERT_OTHER_COMPANY_ID           = %d\n", f.OtherCompanyID)
	fmt.Printf("4.  STAGING_CERT_OTHER_BRANCH_ID            = %d\n", f.OtherBranchID)
	fmt.Printf("5.  STAGING_CERT_CUSTOMER_ID                = %d\n", f.CustomerID)
	fmt.Printf("6.  STAGING_CERT_SUPPLIER_ID                = %d\n", f.SupplierID)
	fmt.Printf("7.  STAGING_CERT_PRODUCT_ID                 = %d\n", f.ProductID)
	fmt.Printf("8.  STAGING_CERT_WAREHOUSE_ID               = %d\n", f.WarehouseID)
	fmt.Printf("9.  STAGING_CERT_GRN_ID                     = %d\n", f.GRNID)
	fmt.Printf("10. STAGING_CERT_DOCUMENT_CATEGORY_ID       = %d\n", f.DocumentCategoryID)
	fmt.Printf("11. STAGING_CERT_DOCUMENT_CLASSIFICATION_ID = %d\n", f.DocumentClassID)
	fmt.Printf("12. STAGING_CERT_AMOUNT                     = %s\n", f.Amount)
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Println("Test Identities Seeded:")
	fmt.Printf("  Admin:     %s / (configured password)\n", f.AdminEmail)
	fmt.Printf("  Branch:    %s / (configured password) [Expired ISO-003 Role + Completed Revoke]\n", f.BranchEmail)
	fmt.Printf("  No-Access: %s / (configured password) [No Company Membership]\n", f.NoAccessEmail)
	fmt.Println("================================================================================")

	if export {
		fmt.Println("\n# Shell export commands:")
		fmt.Printf("export STAGING_CERT_COMPANY_ID=\"%d\"\n", f.CompanyID)
		fmt.Printf("export STAGING_CERT_BRANCH_ID=\"%d\"\n", f.BranchID)
		fmt.Printf("export STAGING_CERT_OTHER_COMPANY_ID=\"%d\"\n", f.OtherCompanyID)
		fmt.Printf("export STAGING_CERT_OTHER_BRANCH_ID=\"%d\"\n", f.OtherBranchID)
		fmt.Printf("export STAGING_CERT_CUSTOMER_ID=\"%d\"\n", f.CustomerID)
		fmt.Printf("export STAGING_CERT_SUPPLIER_ID=\"%d\"\n", f.SupplierID)
		fmt.Printf("export STAGING_CERT_PRODUCT_ID=\"%d\"\n", f.ProductID)
		fmt.Printf("export STAGING_CERT_WAREHOUSE_ID=\"%d\"\n", f.WarehouseID)
		fmt.Printf("export STAGING_CERT_GRN_ID=\"%d\"\n", f.GRNID)
		fmt.Printf("export STAGING_CERT_DOCUMENT_CATEGORY_ID=\"%d\"\n", f.DocumentCategoryID)
		fmt.Printf("export STAGING_CERT_DOCUMENT_CLASSIFICATION_ID=\"%d\"\n", f.DocumentClassID)
		fmt.Printf("export STAGING_CERT_AMOUNT=\"%s\"\n", f.Amount)
	}

	if !applied {
		fmt.Println("\nTo apply these 12 fixture variables to GitHub staging environment:")
		fmt.Printf("gh variable set STAGING_CERT_COMPANY_ID --body \"%d\" --env staging\n", f.CompanyID)
		fmt.Printf("gh variable set STAGING_CERT_BRANCH_ID --body \"%d\" --env staging\n", f.BranchID)
		fmt.Printf("gh variable set STAGING_CERT_OTHER_COMPANY_ID --body \"%d\" --env staging\n", f.OtherCompanyID)
		fmt.Printf("gh variable set STAGING_CERT_OTHER_BRANCH_ID --body \"%d\" --env staging\n", f.OtherBranchID)
		fmt.Printf("gh variable set STAGING_CERT_CUSTOMER_ID --body \"%d\" --env staging\n", f.CustomerID)
		fmt.Printf("gh variable set STAGING_CERT_SUPPLIER_ID --body \"%d\" --env staging\n", f.SupplierID)
		fmt.Printf("gh variable set STAGING_CERT_PRODUCT_ID --body \"%d\" --env staging\n", f.ProductID)
		fmt.Printf("gh variable set STAGING_CERT_WAREHOUSE_ID --body \"%d\" --env staging\n", f.WarehouseID)
		fmt.Printf("gh variable set STAGING_CERT_GRN_ID --body \"%d\" --env staging\n", f.GRNID)
		fmt.Printf("gh variable set STAGING_CERT_DOCUMENT_CATEGORY_ID --body \"%d\" --env staging\n", f.DocumentCategoryID)
		fmt.Printf("gh variable set STAGING_CERT_DOCUMENT_CLASSIFICATION_ID --body \"%d\" --env staging\n", f.DocumentClassID)
		fmt.Printf("gh variable set STAGING_CERT_AMOUNT --body \"%s\" --env staging\n", f.Amount)
		fmt.Println("\nOr run with flag: go run ./scripts/seed/staging/main.go --apply-gh")
	}
}

func applyToGitHub(f *Fixtures) error {
	vars := map[string]string{
		"STAGING_CERT_COMPANY_ID":                 strconv.FormatInt(f.CompanyID, 10),
		"STAGING_CERT_BRANCH_ID":                  strconv.FormatInt(f.BranchID, 10),
		"STAGING_CERT_OTHER_COMPANY_ID":           strconv.FormatInt(f.OtherCompanyID, 10),
		"STAGING_CERT_OTHER_BRANCH_ID":            strconv.FormatInt(f.OtherBranchID, 10),
		"STAGING_CERT_CUSTOMER_ID":                strconv.FormatInt(f.CustomerID, 10),
		"STAGING_CERT_SUPPLIER_ID":                strconv.FormatInt(f.SupplierID, 10),
		"STAGING_CERT_PRODUCT_ID":                 strconv.FormatInt(f.ProductID, 10),
		"STAGING_CERT_WAREHOUSE_ID":               strconv.FormatInt(f.WarehouseID, 10),
		"STAGING_CERT_GRN_ID":                     strconv.FormatInt(f.GRNID, 10),
		"STAGING_CERT_DOCUMENT_CATEGORY_ID":       strconv.FormatInt(f.DocumentCategoryID, 10),
		"STAGING_CERT_DOCUMENT_CLASSIFICATION_ID": strconv.FormatInt(f.DocumentClassID, 10),
		"STAGING_CERT_AMOUNT":                     f.Amount,
	}

	fmt.Println("\n→ Applying 12 variables to GitHub environment 'staging'...")
	for name, val := range vars {
		cmd := exec.Command("gh", "variable", "set", name, "--body", val, "--env", "staging")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("gh variable set %s: %v, output: %s", name, err, string(output))
		}
		fmt.Printf("  ✓ Set %s = %s\n", name, val)
	}
	fmt.Println("All 12 variables successfully set in GitHub staging environment!")
	return nil
}

// runISO004Main runs the --iso004 extension and prints its outputs. It never
// calls seedStagingFixtures.
func runISO004Main(opts iso004Options, applyGH, exportEnv, jsonOutput bool) {
	ctx := context.Background()
	res, err := runISO004(ctx, opts)
	if err != nil {
		log.Fatalf("iso004 fixtures: %v", err)
	}
	if opts.DryRun {
		printISO004Plan(os.Stdout, res)
		return
	}
	if err := printISO004Summary(os.Stdout, os.Stderr, res, applyGH, exportEnv, jsonOutput); err != nil {
		log.Fatalf("print iso004 summary: %v", err)
	}
	if applyGH {
		progress := os.Stdout
		if jsonOutput {
			progress = os.Stderr
		}
		if err := applyISO004ToGitHub(progress, res); err != nil {
			log.Fatalf("apply ISO-004 variables to GitHub staging environment: %v", err)
		}
	}
}

func getenv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
