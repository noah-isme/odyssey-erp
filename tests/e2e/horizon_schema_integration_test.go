//go:build integration

package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/journals"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/mappings"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/periods"
	"github.com/odyssey-erp/odyssey-erp/internal/integration"
	"github.com/odyssey-erp/odyssey-erp/internal/inventory"
	"github.com/odyssey-erp/odyssey-erp/internal/pos"
	"github.com/odyssey-erp/odyssey-erp/internal/projects"
)

func TestHorizonSchemaAndAdminPermissions(t *testing.T) {
	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		t.Skip("PG_DSN is required")
	}
	applyAllMigrations(t, dsn)
	p, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	ctx := context.Background()
	for _, table := range []string{"wms_pick_waves", "mrp_boms", "pos_sessions", "projects", "api_keys", "webhook_deliveries", "portal_users", "portal_invitations"} {
		var exists bool
		if err := p.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Fatalf("missing Horizon table %s", table)
		}
	}
	var adminCount int
	if err := p.QueryRow(ctx, `SELECT COUNT(*) FROM role_permissions rp JOIN roles r ON r.id=rp.role_id JOIN permissions p ON p.id=rp.permission_id WHERE LOWER(TRIM(r.name)) IN ('admin','administrator') AND p.name IN ('wms.manage','mrp.manage','pos.manage','projects.manage','api.manage','webhooks.manage','portal.manage')`).Scan(&adminCount); err != nil {
		t.Fatal(err)
	}
	if adminCount < 7 {
		t.Fatalf("admin Horizon permissions missing: got %d", adminCount)
	}
	var uniqueIndexes int
	if err := p.QueryRow(ctx, `SELECT COUNT(*) FROM pg_indexes WHERE tablename='pos_payments' AND indexdef LIKE '%(ticket_id, idempotency_key)%'`).Scan(&uniqueIndexes); err != nil {
		t.Fatal(err)
	}
	if uniqueIndexes == 0 {
		t.Fatal("POS payment idempotency constraint missing")
	}
	var userID, companyID, projectID, taskID, sheetID int64
	if err := p.QueryRow(ctx, `INSERT INTO users(email,password_hash) VALUES('horizon-fx-'||gen_random_uuid()::text,'test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO companies(code,name,base_currency) VALUES('HFX-'||gen_random_uuid()::text,'Horizon FX','IDR') RETURNING id`).Scan(&companyID); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO projects(company_id,code,name,currency,manager_id,created_by) VALUES($1,'FX-PROJECT','FX project','USD',$2,$2) RETURNING id`, companyID, userID).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO project_tasks(project_id,code,name) VALUES($1,'TASK','Task') RETURNING id`, projectID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO fx_daily_rates(base_currency,quote_currency,rate_date,rate,source) VALUES('IDR','USD',CURRENT_DATE,15000,'HORIZON-TEST-'||gen_random_uuid()::text) RETURNING id`).Scan(new(int64)); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO timesheets(company_id,project_id,task_id,employee_id,work_date,hours,billable,billable_rate) VALUES($1,$2,$3,$4,CURRENT_DATE,2,TRUE,10) RETURNING id`, companyID, projectID, taskID, userID).Scan(&sheetID); err != nil {
		t.Fatal(err)
	}
	repo := projects.NewRepository(p)
	if err := repo.UpdateTimesheet(ctx, projects.Timesheet{ID: sheetID, CompanyID: companyID, Status: "APPROVED"}); err != nil {
		t.Fatal(err)
	}
	var baseAmount, rate float64
	if err := p.QueryRow(ctx, `SELECT base_amount,fx_rate FROM timesheets WHERE id=$1`, sheetID).Scan(&baseAmount, &rate); err != nil {
		t.Fatal(err)
	}
	if rate <= 0 || baseAmount != 2*10*rate {
		t.Fatalf("timesheet FX lock mismatch: rate=%v base=%v", rate, baseAmount)
	}
	var branchID, warehouseID, categoryID, unitID, productID int64
	if err := p.QueryRow(ctx, `INSERT INTO branches(company_id,code,name) VALUES($1,'H-BR-'||gen_random_uuid()::text,'Horizon branch') RETURNING id`, companyID).Scan(&branchID); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO warehouses(branch_id,code,name) VALUES($1,'H-WH-'||gen_random_uuid()::text,'Horizon warehouse') RETURNING id`, branchID).Scan(&warehouseID); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO categories(code,name) VALUES('H-CAT-'||gen_random_uuid()::text,'Horizon category') RETURNING id`).Scan(&categoryID); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO units(code,name) VALUES('H-UNIT-'||gen_random_uuid()::text,'Unit') RETURNING id`).Scan(&unitID); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO products(sku,name,category_id,unit_id,price) VALUES('H-SKU-'||gen_random_uuid()::text,'Horizon product',$1,$2,10) RETURNING id`, categoryID, unitID).Scan(&productID); err != nil {
		t.Fatal(err)
	}
	stock := inventory.NewService(inventory.NewRepository(p), nil, nil, inventory.ServiceConfig{}, nil)
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	if _, err := stock.PostAdjustment(ctx, inventory.AdjustmentInput{Code: "HORIZON-IN-" + suffix, WarehouseID: warehouseID, ProductID: productID, Qty: 5, UnitCost: 10, ActorID: userID, RefModule: "HORIZON"}); err != nil {
		t.Fatal(err)
	}
	if _, err := stock.PostAdjustment(ctx, inventory.AdjustmentInput{Code: "HORIZON-OUT-" + suffix, WarehouseID: warehouseID, ProductID: productID, Qty: -2, ActorID: userID, RefModule: "HORIZON"}); err != nil {
		t.Fatal(err)
	}
	var balance float64
	if err := p.QueryRow(ctx, `SELECT qty FROM inventory_balances WHERE warehouse_id=$1 AND product_id=$2`, warehouseID, productID).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if balance != 3 {
		t.Fatalf("inventory ledger balance mismatch: got %v", balance)
	}
	var cashAccount, salesAccount, periodID int64
	if err := p.QueryRow(ctx, `INSERT INTO accounts(code,name,type) VALUES('HC-'||substr(md5(gen_random_uuid()::text),1,10),'Horizon POS cash','ASSET') RETURNING id`).Scan(&cashAccount); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO accounts(code,name,type) VALUES('HS-'||substr(md5(gen_random_uuid()::text),1,10),'Horizon POS sales','REVENUE') RETURNING id`).Scan(&salesAccount); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `INSERT INTO account_mappings(module,key,account_id) VALUES('POS','pos.cash',$1),('POS','pos.sales',$2) ON CONFLICT(module,key) DO UPDATE SET account_id=EXCLUDED.account_id`, cashAccount, salesAccount); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `SELECT id FROM periods WHERE status IN ('OPEN','CLOSED') AND start_date<=CURRENT_DATE AND end_date>=CURRENT_DATE ORDER BY id LIMIT 1`).Scan(&periodID); err != nil {
		t.Fatal(err)
	}
	hooks := integration.NewHooks(journals.NewService(journals.NewRepository(p), nil, nil), periods.NewRepository(p), mappings.NewRepository(p))
	if err := hooks.HandlePOSSalePosted(ctx, pos.SalePostedEvent{TicketID: sheetID, CompanyID: companyID, ActorID: userID, Amount: 100, BaseAmount: 100, Currency: "IDR", BaseCurrency: "IDR"}); err != nil {
		t.Fatal(err)
	}
	if err := hooks.HandlePOSSalePosted(ctx, pos.SalePostedEvent{TicketID: sheetID, CompanyID: companyID, ActorID: userID, Amount: 100, BaseAmount: 100, Currency: "IDR", BaseCurrency: "IDR"}); err != nil {
		t.Fatal(err)
	}
	var journalCount, lineCount int
	sourceID := uuid.NewSHA1(uuid.Nil, []byte("POS:"+fmt.Sprint(sheetID)))
	if err := p.QueryRow(ctx, `SELECT COUNT(*) FROM journal_entries WHERE source_module='POS.SALE' AND source_id=$1`, sourceID).Scan(&journalCount); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `SELECT COUNT(*) FROM journal_lines jl JOIN journal_entries je ON je.id=jl.je_id WHERE je.source_module='POS.SALE' AND je.source_id=$1`, sourceID).Scan(&lineCount); err != nil {
		t.Fatal(err)
	}
	if journalCount != 1 || lineCount != 2 {
		t.Fatalf("POS journal idempotency/lines mismatch: journals=%d lines=%d period=%d", journalCount, lineCount, periodID)
	}
	if err := hooks.HandlePOSRefunded(ctx, pos.SalePostedEvent{TicketID: sheetID, CompanyID: companyID, ActorID: userID, Amount: 100, BaseAmount: 100, Currency: "IDR", BaseCurrency: "IDR"}); err != nil {
		t.Fatal(err)
	}
	if err := hooks.HandlePOSRefunded(ctx, pos.SalePostedEvent{TicketID: sheetID, CompanyID: companyID, ActorID: userID, Amount: 100, BaseAmount: 100, Currency: "IDR", BaseCurrency: "IDR"}); err != nil {
		t.Fatal(err)
	}
	var refundCount int
	refundSourceID := uuid.NewSHA1(uuid.Nil, []byte("POS:REFUND:"+fmt.Sprint(sheetID)))
	if err := p.QueryRow(ctx, `SELECT COUNT(*) FROM journal_entries WHERE source_module='POS.REFUND' AND source_id=$1`, refundSourceID).Scan(&refundCount); err != nil {
		t.Fatal(err)
	}
	if refundCount != 1 {
		t.Fatalf("POS refund journal idempotency mismatch: %d", refundCount)
	}
}
