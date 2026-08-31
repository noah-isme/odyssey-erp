package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// seedPhase17Consolidation seeds the consolidation group (Odyssey Group), member entities,
// consolidated group chart of accounts, local-to-group account mappings, intercompany elimination
// rules & calculation runs, MoM variance analysis rules & snapshots, executive board pack templates & packs,
// and monthly period close runs for March through August 2026.
func seedPhase17Consolidation(ctx context.Context, sctx *SeedContext) error {
	return ExecInTx(ctx, sctx.Pool, "Phase 17: Consolidation & Financial Close", func(tx pgx.Tx) error {
		adminID := sctx.UserIDs["budi.santoso@nusantarateknik.co.id"]
		if adminID == 0 {
			return fmt.Errorf("admin user budi.santoso@nusantarateknik.co.id not found")
		}
		accountantID := sctx.UserIDs["siti.aminah@nusantarateknik.co.id"]
		if accountantID == 0 {
			accountantID = adminID
		}

		// Ensure accounting periods are cached in context or lookup directly
		type periodInfo struct {
			code       string
			periodID   int64
			accPeriodID int64
		}
		periodsList := []string{"2026-03", "2026-04", "2026-05", "2026-06", "2026-07", "2026-08"}
		periodMap := make(map[string]periodInfo)

		for _, pcode := range periodsList {
			var pid, apid int64
			err := tx.QueryRow(ctx, `SELECT id FROM periods WHERE code = $1`, pcode).Scan(&pid)
			if err != nil {
				return fmt.Errorf("lookup period %q: %w", pcode, err)
			}
			err = tx.QueryRow(ctx, `SELECT id FROM accounting_periods WHERE period_id = $1 AND (company_id = $2 OR company_id IS NULL) LIMIT 1`, pid, sctx.CompanyNTPID).Scan(&apid)
			if err != nil {
				return fmt.Errorf("lookup accounting_period for %q: %w", pcode, err)
			}
			periodMap[pcode] = periodInfo{code: pcode, periodID: pid, accPeriodID: apid}
		}

		// -------------------------------------------------------------------------
		// 1. Consolidation Group (Odyssey Group) & Members
		// -------------------------------------------------------------------------
		var groupID int64
		err := tx.QueryRow(ctx, `
			INSERT INTO consol_groups (name, reporting_currency, fx_enabled, created_at, updated_at)
			VALUES ('Odyssey Group', 'IDR', TRUE, NOW(), NOW())
			ON CONFLICT (name) DO UPDATE SET
				reporting_currency = EXCLUDED.reporting_currency,
				fx_enabled = EXCLUDED.fx_enabled,
				updated_at = NOW()
			RETURNING id`,
		).Scan(&groupID)
		if err != nil {
			return fmt.Errorf("upsert consol_group 'Odyssey Group': %w", err)
		}

		// Members: PT Nusantara Teknik Perkasa (Parent) & PT Nusantara Distribusi Mandiri (Subsidiary)
		members := []int64{sctx.CompanyNTPID, sctx.CompanyNDMID}
		for _, compID := range members {
			_, err := tx.Exec(ctx, `
				INSERT INTO consol_members (group_id, company_id, enabled, created_at, updated_at)
				VALUES ($1, $2, TRUE, NOW(), NOW())
				ON CONFLICT (group_id, company_id) DO UPDATE SET
					enabled = TRUE,
					updated_at = NOW()`,
				groupID, compID,
			)
			if err != nil {
				return fmt.Errorf("upsert consol_member company %d: %w", compID, err)
			}
		}

		// -------------------------------------------------------------------------
		// 2. Consolidated Group Chart of Accounts (consol_group_accounts)
		// -------------------------------------------------------------------------
		type groupAccDef struct {
			code       string
			name       string
			accType    string // ASSET, LIABILITY, EQUITY, REVENUE, EXPENSE
			parentCode string
		}

		groupAccounts := []groupAccDef{
			// Level 1 Parents
			{"GRP-1000", "Consolidated Assets", "ASSET", ""},
			{"GRP-2000", "Consolidated Liabilities", "LIABILITY", ""},
			{"GRP-3000", "Consolidated Equity", "EQUITY", ""},
			{"GRP-4000", "Consolidated Revenue", "REVENUE", ""},
			{"GRP-5000", "Consolidated Cost of Goods Sold", "EXPENSE", ""},
			{"GRP-6000", "Consolidated Operating Expenses", "EXPENSE", ""},

			// Level 2 Accounts
			{"GRP-1100", "Cash and Cash Equivalents", "ASSET", "GRP-1000"},
			{"GRP-1200", "Trade Receivables", "ASSET", "GRP-1000"},
			{"GRP-1300", "Inventories (Raw & Finished)", "ASSET", "GRP-1000"},
			{"GRP-1400", "Intercompany Receivables", "ASSET", "GRP-1000"},
			{"GRP-1500", "Property, Plant & Equipment", "ASSET", "GRP-1000"},

			{"GRP-2100", "Trade Payables", "LIABILITY", "GRP-2000"},
			{"GRP-2200", "Intercompany Payables", "LIABILITY", "GRP-2000"},
			{"GRP-2300", "Accrued Expenses & Tax Payables", "LIABILITY", "GRP-2000"},

			{"GRP-3100", "Share Capital", "EQUITY", "GRP-3000"},
			{"GRP-3200", "Retained Earnings", "EQUITY", "GRP-3000"},

			{"GRP-4100", "External Commercial Revenue", "REVENUE", "GRP-4000"},
			{"GRP-4200", "Intercompany Sales Revenue", "REVENUE", "GRP-4000"},

			{"GRP-5100", "Direct Materials & Manufacturing Costs", "EXPENSE", "GRP-5000"},
			{"GRP-5200", "Intercompany Cost of Goods Sold", "EXPENSE", "GRP-5000"},

			{"GRP-6100", "General & Administrative Expenses", "EXPENSE", "GRP-6000"},
			{"GRP-6200", "Selling, Marketing & Logistics", "EXPENSE", "GRP-6000"},
		}

		groupAccIDs := make(map[string]int64)

		// First pass: insert level 1 parents without parent_id
		for _, ga := range groupAccounts {
			if ga.parentCode == "" {
				var gaID int64
				err := tx.QueryRow(ctx, `
					INSERT INTO consol_group_accounts (group_id, code, name, type, parent_id, created_at, updated_at)
					VALUES ($1, $2, $3, $4::account_type, NULL, NOW(), NOW())
					ON CONFLICT (group_id, code) DO UPDATE SET
						name = EXCLUDED.name,
						type = EXCLUDED.type,
						updated_at = NOW()
					RETURNING id`,
					groupID, ga.code, ga.name, ga.accType,
				).Scan(&gaID)
				if err != nil {
					return fmt.Errorf("upsert root consol_group_account %q: %w", ga.code, err)
				}
				groupAccIDs[ga.code] = gaID
			}
		}

		// Second pass: insert child accounts with parent_id
		for _, ga := range groupAccounts {
			if ga.parentCode != "" {
				parentID := groupAccIDs[ga.parentCode]
				var gaID int64
				err := tx.QueryRow(ctx, `
					INSERT INTO consol_group_accounts (group_id, code, name, type, parent_id, created_at, updated_at)
					VALUES ($1, $2, $3, $4::account_type, $5, NOW(), NOW())
					ON CONFLICT (group_id, code) DO UPDATE SET
						name = EXCLUDED.name,
						type = EXCLUDED.type,
						parent_id = EXCLUDED.parent_id,
						updated_at = NOW()
					RETURNING id`,
					groupID, ga.code, ga.name, ga.accType, parentID,
				).Scan(&gaID)
				if err != nil {
					return fmt.Errorf("upsert child consol_group_account %q: %w", ga.code, err)
				}
				groupAccIDs[ga.code] = gaID
			}
		}

		// -------------------------------------------------------------------------
		// 3. Local-to-Group Account Mapping (account_map)
		// -------------------------------------------------------------------------
		localToGroupMap := map[string]string{
			// Assets
			"1110": "GRP-1100", // Cash & Bank BCA Operasional
			"1111": "GRP-1100", // Bank Mandiri Payroll
			"1112": "GRP-1100", // Bank BCA USD
			"1120": "GRP-1100", // Petty Cash
			"1130": "GRP-1200", // Accounts Receivable
			"1131": "GRP-1400", // Intercompany Receivables
			"1300": "GRP-1300", // Inventory Asset
			"1410": "GRP-1200", // VAT In (Pajak Masukan)
			"1500": "GRP-1500", // Fixed Assets Cost
			"1510": "GRP-1500", // Accumulated Depreciation
			// Liabilities
			"2100": "GRP-2100", // Accounts Payable
			"2110": "GRP-2200", // Intercompany Payables
			"2200": "GRP-2300", // Accrued Payroll & BPJS
			"2300": "GRP-2300", // VAT Out (Pajak Keluaran)
			"2400": "GRP-2300", // GR/IR Clearing
			// Equity
			"3100": "GRP-3100", // Modal Saham (Share Capital)
			"3200": "GRP-3200", // Laba Ditahan (Retained Earnings)
			"3300": "GRP-3200", // Laba Tahun Berjalan
			// Revenue
			"4100": "GRP-4100", // Sales Revenue
			"4200": "GRP-4200", // Intercompany Sales Revenue
			"4300": "GRP-4100", // Service & Maintenance Revenue
			"4400": "GRP-4100", // FX Gain
			// Expenses
			"5100": "GRP-5100", // COGS Raw Materials
			"5200": "GRP-5200", // Intercompany COGS
			"5300": "GRP-6100", // Operating Expenses
			"5400": "GRP-6100", // Salary & Wages
			"5500": "GRP-6100", // Depreciation Expense
			"5600": "GRP-6200", // Freight & Logistics Out
			"5700": "GRP-6100", // FX Loss
		}

		for localCode, grpCode := range localToGroupMap {
			var localAccID int64
			err := tx.QueryRow(ctx, `SELECT id FROM accounts WHERE code = $1 LIMIT 1`, localCode).Scan(&localAccID)
			if err != nil {
				continue // skip if account not in local COA
			}
			grpAccID := groupAccIDs[grpCode]
			if grpAccID == 0 {
				continue
			}

			for _, compID := range members {
				_, err = tx.Exec(ctx, `
					INSERT INTO account_map (group_id, company_id, local_account_id, group_account_id, created_at, updated_at)
					VALUES ($1, $2, $3, $4, NOW(), NOW())
					ON CONFLICT (group_id, company_id, local_account_id) DO UPDATE SET
						group_account_id = EXCLUDED.group_account_id,
						updated_at = NOW()`,
					groupID, compID, localAccID, grpAccID,
				)
				if err != nil {
					return fmt.Errorf("upsert account_map (%d, comp %d, acc %s -> %s): %w", groupID, compID, localCode, grpCode, err)
				}
			}
		}

		// -------------------------------------------------------------------------
		// 4. Intercompany Rules (ic_rules)
		// -------------------------------------------------------------------------
		arApSrc := groupAccIDs["GRP-1400"]
		arApDst := groupAccIDs["GRP-2200"]
		revCogsSrc := groupAccIDs["GRP-4200"]
		revCogsDst := groupAccIDs["GRP-5200"]

		if arApSrc > 0 && arApDst > 0 {
			_, err = tx.Exec(ctx, `
				INSERT INTO ic_rules (group_id, src_group_acc, dst_group_acc, type, enabled, created_at, updated_at)
				VALUES ($1, $2, $3, 'AR_AP', TRUE, NOW(), NOW())
				ON CONFLICT (group_id, src_group_acc, dst_group_acc, type) DO UPDATE SET
					enabled = TRUE,
					updated_at = NOW()`,
				groupID, arApSrc, arApDst,
			)
			if err != nil {
				return fmt.Errorf("upsert ic_rule AR_AP: %w", err)
			}
		}

		if revCogsSrc > 0 && revCogsDst > 0 {
			_, err = tx.Exec(ctx, `
				INSERT INTO ic_rules (group_id, src_group_acc, dst_group_acc, type, enabled, created_at, updated_at)
				VALUES ($1, $2, $3, 'REV_COGS', TRUE, NOW(), NOW())
				ON CONFLICT (group_id, src_group_acc, dst_group_acc, type) DO UPDATE SET
					enabled = TRUE,
					updated_at = NOW()`,
				groupID, revCogsSrc, revCogsDst,
			)
			if err != nil {
				return fmt.Errorf("upsert ic_rule REV_COGS: %w", err)
			}
		}

		// -------------------------------------------------------------------------
		// 5. Intercompany Elimination Rules (elimination_rules)
		// -------------------------------------------------------------------------
		type elimRuleDef struct {
			name        string
			srcCompID   int64
			tgtCompID   int64
			accSrc      string
			accTgt      string
			matchCrit   string
		}

		elimRules := []elimRuleDef{
			{
				name:      "IC Revenue & COGS Elimination (NTP - NDM)",
				srcCompID: sctx.CompanyNTPID,
				tgtCompID: sctx.CompanyNDMID,
				accSrc:    "4100",
				accTgt:    "5100",
				matchCrit: `{"type": "REV_COGS", "description": "Eliminate intercompany sales revenue against component purchases"}`,
			},
			{
				name:      "IC Balance Sheet AR & AP Elimination (NTP - NDM)",
				srcCompID: sctx.CompanyNTPID,
				tgtCompID: sctx.CompanyNDMID,
				accSrc:    "1130",
				accTgt:    "2100",
				matchCrit: `{"type": "AR_AP", "description": "Eliminate bilateral trade receivables and payables between parent and subsidiary"}`,
			},
		}

		elimRuleIDs := make([]int64, 0, len(elimRules))
		for _, er := range elimRules {
			var ruleID int64
			err := tx.QueryRow(ctx, `
				SELECT id FROM elimination_rules
				WHERE name = $1 AND source_company_id = $2 AND target_company_id = $3`,
				er.name, er.srcCompID, er.tgtCompID,
			).Scan(&ruleID)

			if err != nil {
				err = tx.QueryRow(ctx, `
					INSERT INTO elimination_rules (
						group_id, name, source_company_id, target_company_id,
						account_src, account_tgt, match_criteria, is_active,
						created_by, created_at, updated_at
					)
					VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, TRUE, $8, NOW(), NOW())
					RETURNING id`,
					groupID, er.name, er.srcCompID, er.tgtCompID,
					er.accSrc, er.accTgt, er.matchCrit, adminID,
				).Scan(&ruleID)
			} else {
				_, err = tx.Exec(ctx, `
					UPDATE elimination_rules SET
						account_src = $1, account_tgt = $2,
						match_criteria = $3::jsonb, is_active = TRUE, updated_at = NOW()
					WHERE id = $4`,
					er.accSrc, er.accTgt, er.matchCrit, ruleID,
				)
			}
			if err != nil {
				return fmt.Errorf("upsert elimination_rule %q: %w", er.name, err)
			}
			elimRuleIDs = append(elimRuleIDs, ruleID)
		}

		// -------------------------------------------------------------------------
		// 6. Elimination Calculation Runs (elimination_runs)
		// -------------------------------------------------------------------------
		for _, pcode := range periodsList {
			pInfo := periodMap[pcode]
			for idx, rID := range elimRuleIDs {
				status := "POSTED"
				if pcode == "2026-08" && idx == 1 {
					status = "SIMULATED"
				}

				tPosted := time.Date(2026, time.Month(idx+3), 28, 18, 0, 0, 0, time.UTC)
				summary := fmt.Sprintf(`{"period": "%s", "rule_id": %d, "eliminated_amount": 145000000.00, "currency": "IDR", "status": "%s"}`, pcode, rID, status)

				var runID int64
				err := tx.QueryRow(ctx, `
					SELECT id FROM elimination_runs
					WHERE period_id = $1 AND rule_id = $2`,
					pInfo.accPeriodID, rID,
				).Scan(&runID)

				if err != nil {
					_, err = tx.Exec(ctx, `
						INSERT INTO elimination_runs (
							period_id, rule_id, status, created_by, created_at,
							simulated_at, posted_at, summary
						)
						VALUES ($1, $2, $3::elimination_run_status, $4, $5, $5, $5, $6::jsonb)`,
						pInfo.accPeriodID, rID, status, accountantID, tPosted, summary,
					)
				} else {
					_, err = tx.Exec(ctx, `
						UPDATE elimination_runs SET
							status = $1::elimination_run_status,
							summary = $2::jsonb
						WHERE id = $3`,
						status, summary, runID,
					)
				}
				if err != nil {
					return fmt.Errorf("upsert elimination_run (period %s, rule %d): %w", pcode, rID, err)
				}
			}
		}

		// -------------------------------------------------------------------------
		// 7. MoM Variance Rules & Snapshots (variance_rules, variance_snapshots)
		// -------------------------------------------------------------------------
		pMarch := periodMap["2026-03"]
		pApril := periodMap["2026-04"]
		pJuly := periodMap["2026-07"]
		pAugust := periodMap["2026-08"]

		type varRuleDef struct {
			name        string
			compType    string
			baseAPID    int64
			compAPID    *int64
			threshAmt   float64
			threshPct   float64
		}

		varRules := []varRuleDef{
			{
				name:      "MoM Consolidated Commercial Revenue Variance",
				compType:  "MOM",
				baseAPID:  pJuly.accPeriodID,
				compAPID:  &pAugust.accPeriodID,
				threshAmt: 25000000.00,
				threshPct: 5.00,
			},
			{
				name:      "MoM Manufacturing Overhead & Direct Materials Variance",
				compType:  "MOM",
				baseAPID:  pMarch.accPeriodID,
				compAPID:  &pApril.accPeriodID,
				threshAmt: 15000000.00,
				threshPct: 7.50,
			},
		}

		varSnapshotIDs := make([]int64, 0)

		for _, vr := range varRules {
			var vruleID int64
			err := tx.QueryRow(ctx, `
				SELECT id FROM variance_rules
				WHERE company_id = $1 AND name = $2`,
				sctx.CompanyNTPID, vr.name,
			).Scan(&vruleID)

			if err != nil {
				err = tx.QueryRow(ctx, `
					INSERT INTO variance_rules (
						company_id, name, comparison_type, base_period_id, compare_period_id,
						dimension_filters, threshold_amount, threshold_percent, is_active,
						created_by, created_at
					)
					VALUES ($1, $2, $3, $4, $5, '{}'::jsonb, $6, $7, TRUE, $8, NOW())
					RETURNING id`,
					sctx.CompanyNTPID, vr.name, vr.compType, vr.baseAPID, vr.compAPID,
					vr.threshAmt, vr.threshPct, accountantID,
				).Scan(&vruleID)
			}
			if err != nil {
				return fmt.Errorf("upsert variance_rule %q: %w", vr.name, err)
			}

			// Generate variance snapshot
			var snapID int64
			payload := `{"analysis": "MoM variance within target tolerance (+4.2% top-line growth)", "revenue_delta": 42500000, "margin_pct": 38.4}`
			tGen := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)

			err = tx.QueryRow(ctx, `
				SELECT id FROM variance_snapshots WHERE rule_id = $1 AND period_id = $2`,
				vruleID, vr.baseAPID,
			).Scan(&snapID)

			if err != nil {
				err = tx.QueryRow(ctx, `
					INSERT INTO variance_snapshots (
						rule_id, period_id, status, generated_at, generated_by, payload, created_at, updated_at
					)
					VALUES ($1, $2, 'READY'::variance_snapshot_status, $3, $4, $5::jsonb, NOW(), NOW())
					RETURNING id`,
					vruleID, vr.baseAPID, tGen, accountantID, payload,
				).Scan(&snapID)
			}
			if err != nil {
				return fmt.Errorf("upsert variance_snapshot for rule %q: %w", vr.name, err)
			}
			varSnapshotIDs = append(varSnapshotIDs, snapID)
		}

		// -------------------------------------------------------------------------
		// 8. Board Pack Templates & Generated Board Packs (board_pack_templates, board_packs)
		// -------------------------------------------------------------------------
		sectionsDefault := `[
			{"title": "Executive Summary & Macro Outlook", "order": 1, "type": "NARRATIVE"},
			{"title": "Consolidated P&L Statement (PSAK 102)", "order": 2, "type": "FINANCIAL_PL"},
			{"title": "Consolidated Balance Sheet & Net Working Capital", "order": 3, "type": "FINANCIAL_BS"},
			{"title": "Operational KPIs: SMT Yield & Delivery OTD", "order": 4, "type": "OPERATIONS"},
			{"title": "CAPA & Enterprise Risk Register", "order": 5, "type": "RISK_AUDIT"}
		]`

		var tmplID int64
		err = tx.QueryRow(ctx, `
			SELECT id FROM board_pack_templates WHERE name = 'Executive Monthly Board Reporting Pack' LIMIT 1`,
		).Scan(&tmplID)

		if err != nil {
			err = tx.QueryRow(ctx, `
				INSERT INTO board_pack_templates (
					name, description, sections, is_default, is_active, created_by, created_at, updated_at
				)
				VALUES ($1, $2, $3::jsonb, TRUE, TRUE, $4, NOW(), NOW())
				RETURNING id`,
				"Executive Monthly Board Reporting Pack",
				"Standard comprehensive monthly board deck containing PSAK financials, KPI scorecards, and audit logs",
				sectionsDefault, adminID,
			).Scan(&tmplID)
		}
		if err != nil {
			return fmt.Errorf("upsert board_pack_template: %w", err)
		}

		// Board Packs for closed periods
		boardPacks := []struct {
			pcode     string
			filePath  string
			fileSize  int64
			pageCount int
		}{
			{"2026-06", "/reports/boardpacks/2026-06-executive-boardpack.pdf", 4850200, 36},
			{"2026-07", "/reports/boardpacks/2026-07-executive-boardpack.pdf", 5120400, 38},
		}

		for _, bp := range boardPacks {
			pInfo := periodMap[bp.pcode]
			var snapID *int64
			if len(varSnapshotIDs) > 0 {
				snapID = &varSnapshotIDs[0]
			}

			var bpID int64
			err := tx.QueryRow(ctx, `
				SELECT id FROM board_packs
				WHERE company_id = $1 AND period_id = $2 AND template_id = $3`,
				sctx.CompanyNTPID, pInfo.accPeriodID, tmplID,
			).Scan(&bpID)

			if err != nil {
				_, err = tx.Exec(ctx, `
					INSERT INTO board_packs (
						company_id, period_id, template_id, variance_snapshot_id,
						status, generated_at, generated_by, file_path, file_size, page_count,
						metadata, created_at, updated_at
					)
					VALUES ($1, $2, $3, $4, 'READY'::board_pack_status, NOW(), $5, $6, $7, $8, '{"version": "1.0", "classified": "RESTRICTED"}'::jsonb, NOW(), NOW())`,
					sctx.CompanyNTPID, pInfo.accPeriodID, tmplID, snapID,
					adminID, bp.filePath, bp.fileSize, bp.pageCount,
				)
			}
			if err != nil {
				return fmt.Errorf("upsert board_pack for %s: %w", bp.pcode, err)
			}
		}

		// -------------------------------------------------------------------------
		// 9. Period Close Runs & Checklist Items (March - August 2026)
		// -------------------------------------------------------------------------
		type checklistItemDef struct {
			code     string
			label    string
			status   string
			comment  string
		}

		defaultChecklist := []checklistItemDef{
			{"CHK-01-BANK", "Bank Reconciliation for all 3 Operating & Payroll Accounts", "DONE", "All statement lines matched with zero unreconciled variance"},
			{"CHK-02-SUBLEDGER", "Subledger Closing: AP, AR, and Inventory Transactions", "DONE", "All subledgers posted and balanced with GL control accounts"},
			{"CHK-03-DEPR", "Fixed Asset Depreciation Run & Useful Life Review", "DONE", "Automated straight-line depreciation journal entries posted"},
			{"CHK-04-ELIM", "Intercompany Transactions & Elimination Calculation Run", "DONE", "IC revenue and reciprocal balances eliminated"},
			{"CHK-05-STMT", "Financial Statements & Management Pack Generation", "DONE", "P&L, Balance Sheet, Cash Flow generated and locked"},
		}

		for _, pcode := range periodsList {
			pInfo := periodMap[pcode]
			isCurrentMonth := (pcode == "2026-08")

			runStatus := "COMPLETED"
			var completedAt *time.Time
			if isCurrentMonth {
				runStatus = "IN_PROGRESS"
				completedAt = nil
			} else {
				tComp := time.Date(2026, time.Month(len(varSnapshotIDs)+4), 1, 17, 30, 0, 0, time.UTC)
				completedAt = &tComp
			}

			note := fmt.Sprintf("Monthly financial close execution for period %s", pcode)

			var pcrID int64
			err := tx.QueryRow(ctx, `
				SELECT id FROM period_close_runs
				WHERE company_id = $1 AND period_id = $2`,
				sctx.CompanyNTPID, pInfo.accPeriodID,
			).Scan(&pcrID)

			if err != nil {
				err = tx.QueryRow(ctx, `
					INSERT INTO period_close_runs (
						company_id, period_id, status, created_by, completed_at,
						notes, created_at, updated_at
					)
					VALUES ($1, $2, $3::period_close_run_status, $4, $5, $6, NOW(), NOW())
					RETURNING id`,
					sctx.CompanyNTPID, pInfo.accPeriodID, runStatus, accountantID,
					completedAt, note,
				).Scan(&pcrID)
			} else {
				_, err = tx.Exec(ctx, `
					UPDATE period_close_runs SET
						status = $1::period_close_run_status,
						completed_at = $2, notes = $3, updated_at = NOW()
					WHERE id = $4`,
					runStatus, completedAt, note, pcrID,
				)
			}
			if err != nil {
				return fmt.Errorf("upsert period_close_run for period %s: %w", pcode, err)
			}

			// Checklist items
			for _, item := range defaultChecklist {
				itemStatus := item.status
				if isCurrentMonth && (item.code == "CHK-04-ELIM" || item.code == "CHK-05-STMT") {
					itemStatus = "PENDING"
				}

				var itemComp *time.Time
				if itemStatus == "DONE" {
					tItem := time.Date(2026, 8, 28, 14, 0, 0, 0, time.UTC)
					itemComp = &tItem
				}

				_, err = tx.Exec(ctx, `
					INSERT INTO period_close_checklist_items (
						period_close_run_id, code, label, status,
						assigned_to, completed_at, comment, created_at, updated_at
					)
					VALUES ($1, $2, $3, $4::period_close_checklist_status, $5, $6, $7, NOW(), NOW())
					ON CONFLICT (period_close_run_id, code) DO UPDATE SET
						label = EXCLUDED.label,
						status = EXCLUDED.status,
						completed_at = EXCLUDED.completed_at,
						comment = EXCLUDED.comment,
						updated_at = NOW()`,
					pcrID, item.code, item.label, itemStatus,
					accountantID, itemComp, item.comment,
				)
				if err != nil {
					return fmt.Errorf("upsert checklist item %s for period %s: %w", item.code, pcode, err)
				}
			}
		}

		return nil
	})
}
