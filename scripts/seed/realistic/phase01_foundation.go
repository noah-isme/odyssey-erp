package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"golang.org/x/crypto/bcrypt"
)

// seedPhase01Foundation seeds users, RBAC roles & permissions, companies, branches,
// warehouses, departments, cost centers, UOM units, taxes, categories, products,
// suppliers, and customers for PT Nusantara Teknik Perkasa.
func seedPhase01Foundation(ctx context.Context, sctx *SeedContext) error {
	return ExecInTx(ctx, sctx.Pool, "Phase 01: Foundation Master Data", func(tx pgx.Tx) error {
		// -------------------------------------------------------------------------
		// 1. Users (9 Indonesian Users with password123)
		// -------------------------------------------------------------------------
		users := []struct {
			email    string
			name     string
			phone    string
			roleName string
		}{
			{"budi.santoso@nusantarateknik.co.id", "Budi Santoso", "+628111223344", "admin"},
			{"siti.aminah@nusantarateknik.co.id", "Siti Aminah", "+628122334455", "accountant"},
			{"hendra.wijaya@nusantarateknik.co.id", "Hendra Wijaya", "+628133445566", "sales_manager"},
			{"agus.setiawan@nusantarateknik.co.id", "Agus Setiawan", "+628144556677", "procurement_manager"},
			{"dewi.lestari@nusantarateknik.co.id", "Dewi Lestari", "+628155667788", "warehouse_supervisor"},
			{"joko.prasetyo@nusantarateknik.co.id", "Joko Prasetyo", "+628166778899", "production_manager"},
			{"ratna.sari@nusantarateknik.co.id", "Ratna Sari", "+628177889900", "qa_manager"},
			{"bambang.pamungkas@nusantarateknik.co.id", "Bambang Pamungkas", "+628188990011", "maintenance_lead"},
			{"rina.wulandari@nusantarateknik.co.id", "Rina Wulandari", "+628199001122", "hr_manager"},
		}

		pwdHash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash password: %w", err)
		}

		for _, u := range users {
			var userID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO users (email, password_hash, is_active, name, ui_theme, ui_language, ui_notifications, phone, created_at, updated_at)
				VALUES ($1, $2, TRUE, $3, 'system', 'id', TRUE, $4, NOW(), NOW())
				ON CONFLICT (email) DO UPDATE SET
					password_hash = EXCLUDED.password_hash,
					is_active = TRUE,
					name = EXCLUDED.name,
					phone = EXCLUDED.phone,
					updated_at = NOW()
				RETURNING id`, u.email, string(pwdHash), u.name, u.phone).Scan(&userID)
			if err != nil {
				return fmt.Errorf("upsert user %q: %w", u.email, err)
			}
			sctx.UserIDs[u.email] = userID
		}

		// -------------------------------------------------------------------------
		// 2. Permissions Inventory & System Roles
		// -------------------------------------------------------------------------
		permMap := make(map[string]string)

		// Core platform permissions
		for _, p := range shared.CoreScopes() {
			permMap[p] = "Core platform permission: " + p
		}
		// Phase 1 to 6 permissions
		for _, p := range shared.Phase1To6Permissions {
			permMap[p.Name] = p.Description
		}
		// CMMS, QMS, Documents permissions
		for _, p := range shared.CMMSQMSDocumentsPermissions {
			permMap[p.Name] = p.Description
		}
		// Sourcing and Logistics permissions
		for _, p := range shared.SourcingAndLogisticsScopes() {
			permMap[p] = "Sourcing & Logistics permission: " + p
		}
		// Finance permissions
		for _, p := range shared.FinanceScopes() {
			permMap[p] = "Finance permission: " + p
		}
		// Sales and Delivery permissions
		for _, p := range shared.AllSalesDeliveryScopes() {
			permMap[p] = "Sales & Delivery permission: " + p
		}
		// Consolidation, Analytics, Insights, Audit permissions
		for _, p := range shared.FinanceConsolidationScopes() {
			permMap[p] = "Finance consolidation permission: " + p
		}
		for _, p := range shared.FinanceAnalyticsScopes() {
			permMap[p] = "Finance analytics permission: " + p
		}
		for _, p := range shared.FinanceInsightsScopes() {
			permMap[p] = "Finance insights permission: " + p
		}
		for _, p := range shared.FinanceAuditScopes() {
			permMap[p] = "Finance audit permission: " + p
		}

		// Additional domain scopes
		additionalScopes := []struct {
			name string
			desc string
		}{
			{"inventory.view", "View inventory balances and transactions"},
			{"inventory.edit", "Manage inventory transactions and adjustments"},
			{"procurement.view", "View procurement documents"},
			{"procurement.edit", "Manage procurement documents"},
			{"wms.view", "View WMS warehouse execution and bins"},
			{"wms.manage", "Manage WMS picking, putaway and bin locations"},
			{"mrp.view", "View manufacturing MRP plans and BOMs"},
			{"mrp.manage", "Manage manufacturing work orders and routings"},
			{"pos.view", "View point of sale terminal operations"},
			{"pos.manage", "Manage POS terminals and cash sessions"},
			{"projects.view", "View project plans and timesheets"},
			{"projects.manage", "Manage project tasks, budgets and timesheets"},
			{"api.manage", "Manage external API keys and webhooks"},
			{"webhooks.manage", "Manage webhook subscriptions"},
			{"portal.manage", "Manage portal customer and vendor users"},
			{"finance.fx.view", "View transaction FX rates and valuations"},
			{"finance.fx.manage", "Manage transaction FX configuration and rates"},
			{"finance.fx.revalue", "Execute FX balance revaluation"},
			{"finance.fx.override", "Approve manual FX rate overrides"},
		}
		for _, s := range additionalScopes {
			permMap[s.name] = s.desc
		}

		// Insert all permissions
		for permName, permDesc := range permMap {
			if _, err := tx.Exec(ctx, `
				INSERT INTO permissions (name, description)
				VALUES ($1, $2)
				ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description`, permName, permDesc); err != nil {
				return fmt.Errorf("upsert permission %q: %w", permName, err)
			}
		}

		// Collect all permission names for Admin
		var allPermNames []string
		for name := range permMap {
			allPermNames = append(allPermNames, name)
		}

		// Define 9 System Roles
		rolesDef := []struct {
			name        string
			description string
			permissions []string
		}{
			{
				name:        "admin",
				description: "Full administrative access across all ERP modules",
				permissions: allPermNames,
			},
			{
				name:        "accountant",
				description: "Finance, General Ledger, Taxation, Asset Depreciation & Payroll Postings",
				permissions: []string{
					"finance.gl.view", "finance.gl.edit", "finance.period.close", "finance.override.lock", "finance.boardpack",
					"finance.ar.view", "finance.ar.edit", "finance.ar.credit_note.view", "finance.ar.credit_note.create", "finance.ar.credit_note.post", "finance.ar.credit_note.void",
					"finance.ap.view", "finance.ap.edit", "finance.ap.create", "finance.ap.post", "finance.ap.void", "finance.ap.payment",
					"finance.ap.debit_note.view", "finance.ap.debit_note.create", "finance.ap.debit_note.post", "finance.ap.debit_note.void",
					"finance.view_consolidation", "finance.post_elimination", "finance.manage_consolidation", "finance.export_consolidation",
					"finance.view_analytics", "finance.export_analytics", "finance.view_insights", "finance.export_insights", "finance.view_audit",
					"finance.fx.view", "finance.fx.manage", "finance.fx.revalue", "finance.fx.override",
					"tax.view", "tax.config.manage", "tax.period.lock", "tax.document.correct", "tax.report.export",
					"payroll.view", "payroll.post", "payroll.policy.admin", "payroll.payslip.manager",
					"report.view", "master.view", "procurement.view", "sales.customer.view",
				},
			},
			{
				name:        "sales_manager",
				description: "Commercial Operations, CRM Pipeline, Quotations, Sales Orders & Deliveries",
				permissions: []string{
					"sales.customer.view", "sales.customer.create", "sales.customer.edit",
					"sales.quotation.view", "sales.quotation.create", "sales.quotation.edit", "sales.quotation.approve", "sales.quotation.convert", "sales.quotation.reject",
					"sales.order.view", "sales.order.create", "sales.order.edit", "sales.order.confirm", "sales.order.cancel",
					"delivery.order.view", "delivery.return.view", "delivery.return.create", "delivery.return.post",
					"crm.view", "crm.create", "crm.edit", "crm.convert", "crm.team.view", "crm.manage",
					"finance.ar.view", "finance.ar.credit_note.view",
					"inventory.view", "master.view", "report.view",
				},
			},
			{
				name:        "procurement_manager",
				description: "Procurement, Sourcing, RFQ Tenders, Vendor Contracts & Goods Receipts",
				permissions: []string{
					"procurement.view", "procurement.edit",
					"procurement.rfq.view", "procurement.rfq.manage", "procurement.rfq.award",
					"procurement.contract.view", "procurement.contract.manage",
					"procurement.supplier_rating.view", "procurement.supplier_rating.manage",
					"procurement.return.view", "procurement.return.create", "procurement.return.post", "procurement.return.void",
					"finance.ap.view", "finance.ap.debit_note.view",
					"inventory.view", "master.view", "report.view",
				},
			},
			{
				name:        "warehouse_supervisor",
				description: "Inventory Management, Stock Takes, WMS Execution, Picking & Logistics",
				permissions: []string{
					"inventory.view", "inventory.edit",
					"wms.view", "wms.manage",
					"delivery.order.view", "delivery.order.create", "delivery.order.edit", "delivery.order.confirm", "delivery.order.ship", "delivery.order.complete",
					"delivery.return.view", "delivery.return.create", "delivery.return.post",
					"logistics.carrier.view", "logistics.carrier.manage", "logistics.fleet.view", "logistics.fleet.manage",
					"logistics.plan.view", "logistics.plan.manage", "logistics.dispatch.manage", "logistics.freight.view",
					"procurement.view", "master.view", "report.view",
				},
			},
			{
				name:        "production_manager",
				description: "Manufacturing MRP, BOM Management, Routings, Work Orders & Operations",
				permissions: []string{
					"mrp.view", "mrp.manage",
					"inventory.view", "inventory.edit",
					"qms.inspection.view", "qms.ncr.view", "qms.ncr.create",
					"cmms.work_order.view", "cmms.request.create",
					"master.view", "report.view",
				},
			},
			{
				name:        "qa_manager",
				description: "Quality Assurance, NCR Non-Conformances, CAPA Tracking, Audits & Inspections",
				permissions: []string{
					"qms.specification.view", "qms.specification.manage",
					"qms.inspection.view", "qms.inspection.execute",
					"qms.hold.view", "qms.hold.manage",
					"qms.ncr.view", "qms.ncr.create", "qms.ncr.manage",
					"qms.capa.view", "qms.capa.create", "qms.capa.manage", "qms.capa.verify",
					"qms.audit.view", "qms.audit.manage",
					"qms.complaint.view", "qms.complaint.manage",
					"qms.supplier_quality.view", "qms.supplier_quality.manage",
					"qms.admin",
					"documents.view", "documents.upload", "documents.version", "documents.review", "documents.approve",
					"inventory.view", "mrp.view", "master.view", "report.view",
				},
			},
			{
				name:        "maintenance_lead",
				description: "CMMS Asset Maintenance, Preventive PM Schedules, Work Orders & Spare Parts",
				permissions: []string{
					"cmms.asset.view", "cmms.asset.manage",
					"cmms.request.create", "cmms.request.triage",
					"cmms.plan.view", "cmms.plan.manage",
					"cmms.work_order.view", "cmms.work_order.release", "cmms.work_order.execute", "cmms.work_order.close",
					"cmms.cost.view", "cmms.cost.approve", "cmms.admin",
					"fixedassets.location.manage", "fixedassets.maintenance.manage",
					"inventory.view", "documents.view", "master.view", "report.view",
				},
			},
			{
				name:        "hr_manager",
				description: "Human Resources, Employee Directory, Leaves, Attendance & Payroll Calculation",
				permissions: []string{
					"hr.employee.view", "hr.employee.admin",
					"hr.leave.request", "hr.leave.admin",
					"hr.attendance.import",
					"payroll.view", "payroll.process", "payroll.post", "payroll.policy.admin",
					"payroll.payslip.own", "payroll.payslip.manager",
					"users.view", "master.view", "report.view",
				},
			},
		}

		for _, r := range rolesDef {
			var roleID int64
			err := tx.QueryRow(ctx, `SELECT id FROM roles WHERE LOWER(TRIM(name)) = LOWER(TRIM($1)) ORDER BY id LIMIT 1`, r.name).Scan(&roleID)
			if errors.Is(err, pgx.ErrNoRows) {
				err = tx.QueryRow(ctx, `
					INSERT INTO roles (name, description, created_at, updated_at)
					VALUES ($1, $2, NOW(), NOW())
					RETURNING id`, r.name, r.description).Scan(&roleID)
			}
			if err != nil {
				return fmt.Errorf("upsert role %q: %w", r.name, err)
			}

			if _, err := tx.Exec(ctx, `UPDATE roles SET description = $2, updated_at = NOW() WHERE id = $1`, roleID, r.description); err != nil {
				return fmt.Errorf("update role %q: %w", r.name, err)
			}
			sctx.RoleIDs[r.name] = roleID

			for _, permName := range r.permissions {
				if _, err := tx.Exec(ctx, `
					INSERT INTO role_permissions (role_id, permission_id)
					SELECT $1, id FROM permissions WHERE name = $2
					ON CONFLICT DO NOTHING`, roleID, permName); err != nil {
					return fmt.Errorf("assign perm %q to role %q: %w", permName, r.name, err)
				}
			}
		}

		// Assign roles to users
		for _, u := range users {
			userID := sctx.UserIDs[u.email]
			roleID := sctx.RoleIDs[u.roleName]
			if userID > 0 && roleID > 0 {
				if _, err := tx.Exec(ctx, `
					INSERT INTO user_roles (user_id, role_id, created_at)
					VALUES ($1, $2, NOW())
					ON CONFLICT DO NOTHING`, userID, roleID); err != nil {
					return fmt.Errorf("assign user %q to role %q: %w", u.email, u.roleName, err)
				}
			}
		}

		// -------------------------------------------------------------------------
		// 3. Companies (Parent & Subsidiary)
		// -------------------------------------------------------------------------
		companies := []struct {
			code         string
			name         string
			address      string
			taxID        string
			baseCurrency string
		}{
			{
				code:         "NTP-HQ",
				name:         "PT Nusantara Teknik Perkasa",
				address:      "Kawasan Industri Pulogadung, Jl. Rawa Gelam IV No. 8, Jakarta Timur 13930",
				taxID:        "01.888.777.6-012.000",
				baseCurrency: "IDR",
			},
			{
				code:         "NDM-SUB",
				name:         "PT Nusantara Distribusi Mandiri",
				address:      "Kawasan Industri Jababeka II, Jl. Industri Selatan Blok JJ No. 12, Cikarang, Bekasi 17530",
				taxID:        "02.999.666.5-013.000",
				baseCurrency: "IDR",
			},
		}

		for _, c := range companies {
			var compID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO companies (code, name, address, tax_id, base_currency, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
				ON CONFLICT (code) DO UPDATE SET
					name = EXCLUDED.name,
					address = EXCLUDED.address,
					tax_id = EXCLUDED.tax_id,
					base_currency = EXCLUDED.base_currency,
					updated_at = NOW()
				RETURNING id`, c.code, c.name, c.address, c.taxID, c.baseCurrency).Scan(&compID)
			if err != nil {
				return fmt.Errorf("upsert company %q: %w", c.code, err)
			}
			if c.code == "NTP-HQ" {
				sctx.CompanyNTPID = compID
			} else if c.code == "NDM-SUB" {
				sctx.CompanyNDMID = compID
			}
		}

		// -------------------------------------------------------------------------
		// 4. Branches (4 Branches across Java)
		// -------------------------------------------------------------------------
		branches := []struct {
			companyID int64
			code      string
			name      string
			address   string
		}{
			{sctx.CompanyNTPID, "HQ-JKT", "Kantor Pusat & Pabrik Pulogadung", "Kawasan Industri Pulogadung, Jl. Rawa Gelam IV No. 8, Jakarta Timur 13930"},
			{sctx.CompanyNTPID, "BR-SBY", "Kantor Cabang Distribusi Surabaya", "Kawasan Industri Rungkut, Jl. Rungkut Industri Raya No. 45, Surabaya 60293"},
			{sctx.CompanyNDMID, "BR-CKR", "Kantor Sentral Distribusi Cikarang", "Kawasan Industri Jababeka II, Jl. Industri Selatan Blok JJ No. 12, Cikarang 17530"},
			{sctx.CompanyNDMID, "BR-BDG", "Kantor Hub Distribusi Bandung", "Jl. Soekarno Hatta No. 590, Sekejati, Buahbatu, Bandung 40286"},
		}

		for _, b := range branches {
			var branchID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO branches (company_id, code, name, address, created_at, updated_at)
				VALUES ($1, $2, $3, $4, NOW(), NOW())
				ON CONFLICT (code) DO UPDATE SET
					company_id = EXCLUDED.company_id,
					name = EXCLUDED.name,
					address = EXCLUDED.address,
					updated_at = NOW()
				RETURNING id`, b.companyID, b.code, b.name, b.address).Scan(&branchID)
			if err != nil {
				return fmt.Errorf("upsert branch %q: %w", b.code, err)
			}
			sctx.BranchIDs[b.code] = branchID
		}

		// -------------------------------------------------------------------------
		// 5. Warehouses (6 Dedicated Facilities)
		// -------------------------------------------------------------------------
		warehouses := []struct {
			branchCode string
			code       string
			name       string
			address    string
		}{
			{"HQ-JKT", "WH-JKT-RAW", "Gudang Bahan Baku & Komponen Pulogadung", "Jl. Rawa Gelam IV No. 8, Blok A, Jakarta Timur"},
			{"HQ-JKT", "WH-JKT-FG", "Gudang Produk Jadi Pulogadung", "Jl. Rawa Gelam IV No. 8, Blok B, Jakarta Timur"},
			{"HQ-JKT", "WH-JKT-WIP", "Gudang Transit & WIP Perakitan Pulogadung", "Jl. Rawa Gelam IV No. 8, Blok C, Jakarta Timur"},
			{"BR-SBY", "WH-SBY-DIST", "Gudang Distribusi Wilayah Timur Rungkut", "Jl. Rungkut Industri Raya No. 45, Surabaya"},
			{"BR-CKR", "WH-CKR-DIST", "Gudang Sentral Distribusi Cikarang", "Jl. Industri Selatan Blok JJ No. 12, Cikarang"},
			{"BR-BDG", "WH-BDG-HUB", "Gudang Hub Distribusi Bandung", "Jl. Soekarno Hatta No. 590, Bandung"},
		}

		for _, w := range warehouses {
			branchID := sctx.BranchIDs[w.branchCode]
			var whID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO warehouses (branch_id, code, name, address, created_at, updated_at)
				VALUES ($1, $2, $3, $4, NOW(), NOW())
				ON CONFLICT (code) DO UPDATE SET
					branch_id = EXCLUDED.branch_id,
					name = EXCLUDED.name,
					address = EXCLUDED.address,
					updated_at = NOW()
				RETURNING id`, branchID, w.code, w.name, w.address).Scan(&whID)
			if err != nil {
				return fmt.Errorf("upsert warehouse %q: %w", w.code, err)
			}
			sctx.WarehouseIDs[w.code] = whID
		}

		// -------------------------------------------------------------------------
		// 6. Departments & Cost Centers (9 Core Divisions)
		// -------------------------------------------------------------------------
		deptDefs := []struct {
			deptCode string
			deptName string
			ccCode   string
			ccName   string
		}{
			{"DEPT-EXEC", "Manajemen Eksekutif & Direksi", "CC-EXEC-01", "Cost Center Direksi & Manajemen"},
			{"DEPT-FIN", "Keuangan, Akuntansi & Perpajakan", "CC-FIN-01", "Cost Center Keuangan & Akuntansi"},
			{"DEPT-HR", "Human Resources & General Affairs", "CC-HR-01", "Cost Center Sumber Daya Manusia & GA"},
			{"DEPT-SLS", "Penjualan & Komersial", "CC-SLS-01", "Cost Center Penjualan & Pemasaran"},
			{"DEPT-PROC", "Pengadaan & Sourcing", "CC-PROC-01", "Cost Center Pengadaan & Pembelian"},
			{"DEPT-MFG", "Manufaktur & Produksi SMT", "CC-MFG-01", "Cost Center Pabrik & Produksi"},
			{"DEPT-LOG", "Logistik & Pergudangan", "CC-LOG-01", "Cost Center Logistik & Gudang"},
			{"DEPT-QA", "Quality Assurance & QC", "CC-QA-01", "Cost Center Penjaminan Mutu & QC"},
			{"DEPT-ENG", "Engineering & Maintenance", "CC-ENG-01", "Cost Center Rekayasa & Pemeliharaan"},
		}

		for _, d := range deptDefs {
			var deptID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO departments (company_id, code, name, is_active, created_at, updated_at)
				VALUES ($1, $2, $3, TRUE, NOW(), NOW())
				ON CONFLICT (company_id, code) DO UPDATE SET
					name = EXCLUDED.name,
					is_active = TRUE,
					updated_at = NOW()
				RETURNING id`, sctx.CompanyNTPID, d.deptCode, d.deptName).Scan(&deptID)
			if err != nil {
				return fmt.Errorf("upsert department %q: %w", d.deptCode, err)
			}
			sctx.DepartmentIDs[d.deptCode] = deptID

			var ccID int64
			err = tx.QueryRow(ctx, `
				INSERT INTO cost_centers (company_id, department_id, code, name, is_active, created_at, updated_at)
				VALUES ($1, $2, $3, $4, TRUE, NOW(), NOW())
				ON CONFLICT (company_id, code) DO UPDATE SET
					department_id = EXCLUDED.department_id,
					name = EXCLUDED.name,
					is_active = TRUE,
					updated_at = NOW()
				RETURNING id`, sctx.CompanyNTPID, deptID, d.ccCode, d.ccName).Scan(&ccID)
			if err != nil {
				return fmt.Errorf("upsert cost center %q: %w", d.ccCode, err)
			}
			sctx.CostCenterIDs[d.ccCode] = ccID
		}

		// -------------------------------------------------------------------------
		// 7. Units of Measure (7 UOM Units)
		// -------------------------------------------------------------------------
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
			var unitID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO units (code, name)
				VALUES ($1, $2)
				ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name
				RETURNING id`, u.code, u.name).Scan(&unitID)
			if err != nil {
				return fmt.Errorf("upsert unit %q: %w", u.code, err)
			}
			sctx.UnitIDs[u.code] = unitID
		}

		// -------------------------------------------------------------------------
		// 8. Tax Rates (3 Tax Definitions)
		// -------------------------------------------------------------------------
		taxes := []struct {
			code string
			name string
			rate float64
		}{
			{"PPN", "PPN 11%", 11.00},
			{"PPH23", "PPh 23 Jasa 2%", 2.00},
			{"NO-TAX", "Bebas Pajak 0%", 0.00},
		}

		for _, t := range taxes {
			var taxID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO taxes (code, name, rate)
				VALUES ($1, $2, $3)
				ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, rate = EXCLUDED.rate
				RETURNING id`, t.code, t.name, t.rate).Scan(&taxID)
			if err != nil {
				return fmt.Errorf("upsert tax %q: %w", t.code, err)
			}
			sctx.TaxIDs[t.code] = taxID
		}

		// -------------------------------------------------------------------------
		// 9. Product Categories (5 Categories)
		// -------------------------------------------------------------------------
		categories := []struct {
			code string
			name string
		}{
			{"RAW", "Bahan Baku & Mekanikal"},
			{"COMP", "Komponen & Modul Elektronik"},
			{"FG", "Produk Jadi IoT"},
			{"TRD", "Barang Dagang & Solusi Industri"},
			{"ELEC", "Perangkat Elektronik Umum"},
		}

		for _, c := range categories {
			var catID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO categories (code, name, parent_id)
				VALUES ($1, $2, NULL)
				ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name
				RETURNING id`, c.code, c.name).Scan(&catID)
			if err != nil {
				return fmt.Errorf("upsert category %q: %w", c.code, err)
			}
			sctx.CategoryIDs[c.code] = catID
		}

		// -------------------------------------------------------------------------
		// 10. Products Catalog (20 Items: 6 Raw, 4 Components, 6 FG, 4 Trading)
		// -------------------------------------------------------------------------
		products := []struct {
			sku           string
			name          string
			categoryCode  string
			unitCode      string
			price         float64
			taxCode       string
			minStock      float64
			reorderTarget float64
			costMethod    string
			trackBatch    bool
			trackSerial   bool
		}{
			// Raw Materials (6 items)
			{"RM-PCB-GW01", "Mainboard PCB 4-Layer FR4 Gold NTP-GW", "RAW", "PCS", 24000, "PPN", 500, 2000, "FIFO", true, false},
			{"RM-ENC-IP67", "Die-Cast Aluminum Enclosure IP67", "RAW", "PCS", 68000, "PPN", 300, 1000, "FIFO", false, false},
			{"RM-ANT-LORA", "Outdoor 915MHz 5.8dBi Fiberglass Antenna", "RAW", "PCS", 75000, "PPN", 200, 800, "FIFO", false, false},
			{"RM-CON-M12", "M12 Waterproof 5-Pin Connector Set", "RAW", "SET", 18000, "PPN", 400, 1500, "FIFO", false, false},
			{"RM-BAT-LIION", "Li-Ion Battery Pack 18650 3.7V 5200mAh", "RAW", "PCS", 58000, "PPN", 300, 1200, "FIFO", true, false},
			{"RM-PKG-BOX01", "Polyfoam & Master Kraft Box Packaging", "RAW", "SET", 8500, "PPN", 1000, 3000, "FIFO", false, false},

			// Components (4 items)
			{"CMP-MCU-ESP32", "Dual-Core ESP32-S3 IoT Module SMD", "COMP", "PCS", 62000, "PPN", 500, 2500, "FIFO", true, false},
			{"CMP-MDM-4G", "Quectel 4G LTE Cat-1 Cellular Module", "COMP", "PCS", 150000, "PPN", 300, 1500, "FIFO", true, false},
			{"CMP-PWR-SMPS", "Isolated DC-DC Buck Converter 24V-5V 3A", "COMP", "PCS", 28000, "PPN", 400, 1500, "FIFO", false, false},
			{"CMP-SEN-BME680", "Bosch BME680 Gas/Temp/Humidity Sensor", "COMP", "PCS", 95000, "PPN", 200, 1000, "FIFO", true, false},

			// Finished Goods (6 items)
			{"FG-IOT-GW01", "Nusantara IoT Gateway Pro 4G/LoRaWAN", "FG", "PCS", 3850000, "PPN", 50, 300, "FIFO", false, true},
			{"FG-IOT-ENV01", "Smart Environmental Monitor Industrial", "FG", "PCS", 2450000, "PPN", 30, 200, "FIFO", false, true},
			{"FG-IOT-PWR01", "3-Phase Smart Power Meter Modbus RS485", "FG", "PCS", 1950000, "PPN", 40, 250, "FIFO", false, true},
			{"FG-IOT-AGR01", "Soil & Weather Telemetry Node Solar", "FG", "PCS", 1650000, "PPN", 25, 150, "FIFO", false, true},
			{"FG-IOT-FLT01", "Fleet GPS Telematics OBD-II Tracker", "FG", "PCS", 1250000, "PPN", 50, 300, "FIFO", false, true},
			{"FG-IOT-WTR01", "Ultrasonic Water Level Sensor Transmitter", "FG", "PCS", 2850000, "PPN", 20, 120, "FIFO", false, true},

			// Trading Goods (4 items)
			{"TRD-SVR-EDG01", "Advantech Industrial Edge Server IPC", "TRD", "PCS", 18500000, "PPN", 10, 40, "FIFO", false, true},
			{"TRD-SW-IND08", "Moxa 8-Port Industrial Ethernet Switch", "TRD", "PCS", 6200000, "PPN", 15, 60, "FIFO", false, true},
			{"TRD-UPS-IND01", "Din-Rail DC-UPS 24V 10A Back-up Module", "TRD", "PCS", 3400000, "PPN", 20, 80, "FIFO", false, true},
			{"TRD-SEN-RAD01", "24GHz FMCW Radar Level Sensor 30m", "TRD", "PCS", 12500000, "PPN", 8, 30, "FIFO", false, true},
		}

		for _, p := range products {
			catID := sctx.CategoryIDs[p.categoryCode]
			unitID := sctx.UnitIDs[p.unitCode]
			taxID := sctx.TaxIDs[p.taxCode]

			var prodID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO products (sku, name, category_id, unit_id, price, tax_id, is_active, company_id, min_stock, reorder_target, cost_method, track_batch, track_serial)
				VALUES ($1, $2, $3, $4, $5, $6, TRUE, $7, $8, $9, $10, $11, $12)
				ON CONFLICT (sku) DO UPDATE SET
					name = EXCLUDED.name,
					category_id = EXCLUDED.category_id,
					unit_id = EXCLUDED.unit_id,
					price = EXCLUDED.price,
					tax_id = EXCLUDED.tax_id,
					is_active = TRUE,
					company_id = EXCLUDED.company_id,
					min_stock = EXCLUDED.min_stock,
					reorder_target = EXCLUDED.reorder_target,
					cost_method = EXCLUDED.cost_method,
					track_batch = EXCLUDED.track_batch,
					track_serial = EXCLUDED.track_serial
				RETURNING id`,
				p.sku, p.name, catID, unitID, p.price, taxID, sctx.CompanyNTPID, p.minStock, p.reorderTarget, p.costMethod, p.trackBatch, p.trackSerial).Scan(&prodID)
			if err != nil {
				return fmt.Errorf("upsert product %q: %w", p.sku, err)
			}
			sctx.ProductIDs[p.sku] = prodID
		}

		// -------------------------------------------------------------------------
		// 11. Suppliers (8 Suppliers incl. 2 USD Foreign Vendors)
		// -------------------------------------------------------------------------
		suppliers := []struct {
			code    string
			name    string
			phone   string
			email   string
			address string
			taxID   string
		}{
			{"SUP-JAYA-PCB", "PT Jaya PCB Megatama", "021-89831234", "sales@jayapcb.co.id", "Kawasan Industri Delta Silicon 3, Jl. Pinang Blok F23 No. 5, Cikarang, Bekasi", "01.234.567.8-011.000"},
			{"SUP-ALU-IND", "PT Alumindo Enclosure Perkasa", "021-5918234", "marketing@alumindo-enclosure.co.id", "Kawasan Industri Manis, Jl. Manis Raya No. 18, Tangerang, Banten", "01.345.678.9-012.000"},
			{"SUP-BAT-NUS", "PT Nusantara Power Cell", "0267-845123", "orders@nusantaracell.co.id", "Kawasan Industri KIIC, Jl. Permata Raya Lot CA-2, Karawang", "01.456.789.0-013.000"},
			{"SUP-KAB-MET", "PT Metalindo Kabel & Konektor", "021-8834567", "info@metalindokabel.co.id", "Kawasan Industri MM2100, Jl. Irian Blok EE-8, Cikarang Barat, Bekasi", "01.567.890.1-014.000"},
			{"SUP-PKG-IND", "PT Karton Presisi Indonesia", "021-4601234", "sales@kartonpresisi.co.id", "Kawasan Industri Pulogadung, Jl. Pulo Buaran III No. 4, Jakarta Timur", "01.678.901.2-015.000"},
			{"SUP-MOX-ID", "PT Surya Moxa Industrialindo", "021-6530123", "sales@suryamoxa.co.id", "Rukan Sunter Permai Blok B No. 12, Jl. Danau Sunter Utara, Jakarta Utara", "01.789.012.3-016.000"},
			{"SUP-QCT-HK", "Quectel Wireless Solutions (HK) Ltd", "+852-2889-1234", "apac.sales@quectel.com", "Unit 501, 5/F, Building 16W, Science Park West Avenue, Hong Kong", "HK-88992211"},
			{"SUP-ADV-TW", "Advantech Taiwan Corporation", "+886-2-2792-7818", "sales@advantech.tw", "No. 1, Alley 20, Lane 26, Rueiguang Road, Neihu District, Taipei", "TW-33445566"},
		}

		for _, s := range suppliers {
			var supID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO suppliers (code, name, phone, email, address, is_active, company_id, tax_id)
				VALUES ($1, $2, $3, $4, $5, TRUE, $6, $7)
				ON CONFLICT (code) DO UPDATE SET
					name = EXCLUDED.name,
					phone = EXCLUDED.phone,
					email = EXCLUDED.email,
					address = EXCLUDED.address,
					is_active = TRUE,
					company_id = EXCLUDED.company_id,
					tax_id = EXCLUDED.tax_id
				RETURNING id`, s.code, s.name, s.phone, s.email, s.address, sctx.CompanyNTPID, s.taxID).Scan(&supID)
			if err != nil {
				return fmt.Errorf("upsert supplier %q: %w", s.code, err)
			}
			sctx.SupplierIDs[s.code] = supID
		}

		// -------------------------------------------------------------------------
		// 12. Customers (10 Indonesian Enterprise Customers)
		// -------------------------------------------------------------------------
		customers := []struct {
			code             string
			name             string
			email            string
			phone            string
			taxID            string
			creditLimit      float64
			paymentTermsDays int32
			addressLine1     string
			city             string
		}{
			{"CUST-TELKOM", "PT Telkom Indonesia (Persero) Tbk", "procurement@telkom.co.id", "021-5215123", "01.000.013.1-093.000", 1500000000, 45, "Telkom Landmark Tower, Jl. Jend. Gatot Subroto Kav. 52", "Jakarta Selatan"},
			{"CUST-PLN-NUS", "PT PLN Nusantara Power", "pengadaan@plnnusantarapower.co.id", "031-8283180", "01.001.628.5-051.000", 2000000000, 60, "Jl. Ketintang Baru No. 11", "Surabaya"},
			{"CUST-PAM-JAYA", "Perumda Air Minum Jaya (PAM JAYA)", "scm@pamjaya.id", "021-5704250", "01.061.222.8-073.000", 800000000, 30, "Jl. Penjernihan II, Pejompongan", "Jakarta Pusat"},
			{"CUST-ADARO", "PT Adaro Energy Indonesia Tbk", "vendor.management@adaro.com", "021-25533000", "02.391.221.7-054.000", 1200000000, 30, "Menara Karya Lt. 23, Jl. H.R. Rasuna Said Blok X-5 Kav. 1-2", "Jakarta Selatan"},
			{"CUST-INDOFOOD", "PT Indofood CBP Sukses Makmur Tbk", "procurement.eng@icbp.indofood.co.id", "021-57958822", "01.337.914.8-062.000", 600000000, 30, "Sudirman Plaza, Indofood Tower Lt. 23, Jl. Jend. Sudirman Kav. 76-78", "Jakarta Selatan"},
			{"CUST-ASTRA", "PT Astra Otoparts Tbk", "purchasing@component.astra.co.id", "021-4603550", "01.303.491.7-052.000", 900000000, 30, "Jl. Pegangsaan Dua Km. 2.2, Kelapa Gading", "Jakarta Utara"},
			{"CUST-JAK-PRO", "PT Jakarta Propertindo (Jakpro)", "procurement@jakpro.co.id", "021-29625700", "01.764.555.2-021.000", 750000000, 45, "Gedung Thamrin City Lt. 3, Jl. Thamrin Boulevard", "Jakarta Pusat"},
			{"CUST-PETRO", "PT Petrokimia Gresik", "pengadaan.pabrik@petrokimia-gresik.com", "031-3981811", "01.000.418.2-612.000", 1000000000, 30, "Jl. Jenderal Ahmad Yani", "Gresik"},
			{"CUST-SMART-FARM", "PT Nusantara Smart Agri Mandiri", "admin@smartagri.co.id", "0251-8321234", "03.112.445.6-015.000", 300000000, 14, "Jl. Pajajaran No. 88", "Bogor"},
			{"CUST-LOG-TRANS", "PT Logistik Mega Trans", "fleet.tech@megatrans.co.id", "021-88991122", "03.223.556.7-016.000", 400000000, 14, "Kawasan Pergudangan Marunda Center Blok B No. 5", "Bekasi"},
		}

		adminID := sctx.UserIDs["budi.santoso@nusantarateknik.co.id"]

		for _, cust := range customers {
			var custID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO customers (company_id, code, name, email, phone, tax_id, credit_limit, payment_terms_days, address_line1, city, country, is_active, created_by, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'ID', TRUE, $11, NOW(), NOW())
				ON CONFLICT (company_id, code) DO UPDATE SET
					name = EXCLUDED.name,
					email = EXCLUDED.email,
					phone = EXCLUDED.phone,
					tax_id = EXCLUDED.tax_id,
					credit_limit = EXCLUDED.credit_limit,
					payment_terms_days = EXCLUDED.payment_terms_days,
					address_line1 = EXCLUDED.address_line1,
					city = EXCLUDED.city,
					is_active = TRUE,
					updated_at = NOW()
				RETURNING id`,
				sctx.CompanyNTPID, cust.code, cust.name, cust.email, cust.phone, cust.taxID, cust.creditLimit, cust.paymentTermsDays, cust.addressLine1, cust.city, adminID).Scan(&custID)
			if err != nil {
				return fmt.Errorf("upsert customer %q: %w", cust.code, err)
			}
			sctx.CustomerIDs[cust.code] = custID
		}

		return nil
	})
}
