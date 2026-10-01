package main

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SkipMaterializedViews controls whether Phase 18 refreshes analytical materialized views.
var SkipMaterializedViews bool

// journalLineDraft represents an individual debit or credit line for journal entry posting.
type journalLineDraft struct {
	AccountCode string
	Debit       float64
	Credit      float64
	CompanyID   *int64
	BranchID    *int64
	WarehouseID *int64
}

// journalEntryDraft represents a balanced double-entry General Ledger journal entry header and its lines.
type journalEntryDraft struct {
	SourceModule string
	SourceID     uuid.UUID
	Memo         string
	Date         time.Time
	PostedBy     *int64
	Lines        []journalLineDraft
}

// periodRange stores date range bounds for fiscal periods.
type periodRange struct {
	id        int64
	code      string
	startDate time.Time
	endDate   time.Time
}

// seedPhase18JournalsRefresh posts balanced double-entry General Ledger journal entries for all
// posted business documents across all domains and refreshes financial materialized views.
func seedPhase18JournalsRefresh(ctx context.Context, sctx *SeedContext) error {
	return ExecInTx(ctx, sctx.Pool, "Phase 18: General Ledger Posting & Materialized Views Refresh", func(tx pgx.Tx) error {
		adminID := sctx.UserIDs["budi.santoso@nusantarateknik.co.id"]
		if adminID == 0 {
			var err error
			adminID, err = LookupUserID(ctx, tx, "budi.santoso@nusantarateknik.co.id")
			if err != nil {
				return err
			}
			sctx.UserIDs["budi.santoso@nusantarateknik.co.id"] = adminID
		}

		companyNTP := sctx.CompanyNTPID
		if companyNTP == 0 {
			var err error
			companyNTP, err = LookupCompanyID(ctx, tx, "NTP-HQ")
			if err != nil {
				return err
			}
			sctx.CompanyNTPID = companyNTP
		}

		companyNDM := sctx.CompanyNDMID
		if companyNDM == 0 {
			companyNDM, _ = LookupCompanyID(ctx, tx, "NDM-SUB")
			sctx.CompanyNDMID = companyNDM
		}

		branchHQ := sctx.BranchIDs["BR-JKT-HQ"]
		if branchHQ == 0 {
			branchHQ, _ = LookupBranchID(ctx, tx, "BR-JKT-HQ")
			sctx.BranchIDs["BR-JKT-HQ"] = branchHQ
		}

		whFG := sctx.WarehouseIDs["WH-JKT-FG"]
		if whFG == 0 {
			whFG, _ = LookupWarehouseID(ctx, tx, "WH-JKT-FG")
			sctx.WarehouseIDs["WH-JKT-FG"] = whFG
		}

		// -------------------------------------------------------------------------
		// Cache Accounts & Periods
		// -------------------------------------------------------------------------
		accountMap := make(map[string]int64)
		accRows, err := tx.Query(ctx, `SELECT code, id FROM accounts WHERE is_active = TRUE`)
		if err != nil {
			return fmt.Errorf("load accounts: %w", err)
		}
		for accRows.Next() {
			var code string
			var id int64
			if err := accRows.Scan(&code, &id); err != nil {
				accRows.Close()
				return fmt.Errorf("scan account: %w", err)
			}
			accountMap[code] = id
		}
		accRows.Close()

		var periods []periodRange
		pRows, err := tx.Query(ctx, `SELECT id, code, start_date, end_date FROM periods ORDER BY start_date`)
		if err != nil {
			return fmt.Errorf("load periods: %w", err)
		}
		for pRows.Next() {
			var pr periodRange
			if err := pRows.Scan(&pr.id, &pr.code, &pr.startDate, &pr.endDate); err != nil {
				pRows.Close()
				return fmt.Errorf("scan period: %w", err)
			}
			periods = append(periods, pr)
		}
		pRows.Close()

		findPeriodID := func(d time.Time) (int64, error) {
			normD := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
			for _, p := range periods {
				start := time.Date(p.startDate.Year(), p.startDate.Month(), p.startDate.Day(), 0, 0, 0, 0, time.UTC)
				end := time.Date(p.endDate.Year(), p.endDate.Month(), p.endDate.Day(), 0, 0, 0, 0, time.UTC)
				if !normD.Before(start) && !normD.After(end) {
					return p.id, nil
				}
			}
			return 0, fmt.Errorf("no fiscal period found for date %s", d.Format("2006-01-02"))
		}

		// Helper to post a validated, balanced double-entry journal
		postJournal := func(draft journalEntryDraft) (int64, error) {
			if len(draft.Lines) == 0 {
				return 0, fmt.Errorf("journal entry has zero lines: %s (%s)", draft.SourceModule, draft.Memo)
			}

			periodID, err := findPeriodID(draft.Date)
			if err != nil {
				return 0, err
			}

			// Validate and sum lines
			var sumDebit, sumCredit float64
			validLines := make([]journalLineDraft, 0, len(draft.Lines))

			for _, l := range draft.Lines {
				debit := math.Round(l.Debit*100) / 100
				credit := math.Round(l.Credit*100) / 100

				if debit < 0 || credit < 0 {
					return 0, fmt.Errorf("negative amount in journal line for %s: debit=%.2f credit=%.2f", l.AccountCode, debit, credit)
				}
				if debit > 0 && credit > 0 {
					return 0, fmt.Errorf("dual-sided amount in journal line for %s: debit=%.2f credit=%.2f", l.AccountCode, debit, credit)
				}
				if debit == 0 && credit == 0 {
					continue // Skip 0-amount lines
				}

				if _, ok := accountMap[l.AccountCode]; !ok {
					return 0, fmt.Errorf("account code %q not found in accounts map", l.AccountCode)
				}

				sumDebit += debit
				sumCredit += credit
				validLines = append(validLines, journalLineDraft{
					AccountCode: l.AccountCode,
					Debit:       debit,
					Credit:      credit,
					CompanyID:   l.CompanyID,
					BranchID:    l.BranchID,
					WarehouseID: l.WarehouseID,
				})
			}

			if len(validLines) == 0 {
				return 0, fmt.Errorf("journal entry has no valid non-zero lines: %s (%s)", draft.SourceModule, draft.Memo)
			}

			// STRICT INVARIANT: SUM(debit) == SUM(credit)
			if math.Abs(sumDebit-sumCredit) > 0.005 {
				return 0, fmt.Errorf("strict GL balancing violation on %s (%s): total debit %.2f != total credit %.2f (diff %.4f)",
					draft.SourceModule, draft.Memo, sumDebit, sumCredit, sumDebit-sumCredit)
			}

			postedBy := adminID
			if draft.PostedBy != nil && *draft.PostedBy > 0 {
				postedBy = *draft.PostedBy
			}

			journalDate := time.Date(draft.Date.Year(), draft.Date.Month(), draft.Date.Day(), 0, 0, 0, 0, time.UTC)

			// Idempotent upsert of journal_entries header
			var jeID int64
			err = tx.QueryRow(ctx, `
				SELECT id FROM journal_entries
				WHERE source_module = $1 AND source_id = $2`,
				draft.SourceModule, draft.SourceID).Scan(&jeID)

			if err != nil {
				err = tx.QueryRow(ctx, `
					INSERT INTO journal_entries (
						period_id, date, source_module, source_id, memo, posted_by, posted_at, status, created_at, updated_at
					) VALUES (
						$1, $2, $3, $4, $5, $6, $7, 'POSTED', NOW(), NOW()
					) RETURNING id`,
					periodID, journalDate, draft.SourceModule, draft.SourceID, draft.Memo, postedBy, draft.Date).Scan(&jeID)
				if err != nil {
					return 0, fmt.Errorf("insert journal_entry (%s %s): %w", draft.SourceModule, draft.Memo, err)
				}
			} else {
				_, err = tx.Exec(ctx, `
					UPDATE journal_entries SET
						period_id = $1, date = $2, memo = $3, posted_by = $4, posted_at = $5, status = 'POSTED', updated_at = NOW()
					WHERE id = $6`,
					periodID, journalDate, draft.Memo, postedBy, draft.Date, jeID)
				if err != nil {
					return 0, fmt.Errorf("update journal_entry %d: %w", jeID, err)
				}
				// Remove existing lines to recreate idempotently
				_, _ = tx.Exec(ctx, `DELETE FROM journal_lines WHERE je_id = $1`, jeID)
			}

			// Insert journal_lines
			for _, l := range validLines {
				accID := accountMap[l.AccountCode]
				compID := companyNTP
				if l.CompanyID != nil && *l.CompanyID > 0 {
					compID = *l.CompanyID
				}
				var brID *int64
				if l.BranchID != nil && *l.BranchID > 0 {
					brID = l.BranchID
				} else if branchHQ > 0 {
					brID = &branchHQ
				}

				_, err = tx.Exec(ctx, `
					INSERT INTO journal_lines (
						je_id, account_id, debit, credit, dim_company_id, dim_branch_id, dim_warehouse_id, created_at, updated_at
					) VALUES (
						$1, $2, $3, $4, $5, $6, $7, NOW(), NOW()
					)`,
					jeID, accID, l.Debit, l.Credit, compID, brID, l.WarehouseID)
				if err != nil {
					return 0, fmt.Errorf("insert journal_line for acc %s (je %d): %w", l.AccountCode, jeID, err)
				}
			}

			return jeID, nil
		}

		linkSource := func(module string, refID uuid.UUID, jeID int64) error {
			_, err := tx.Exec(ctx, `
				INSERT INTO source_links (module, ref_id, je_id, created_at)
				VALUES ($1, $2, $3, NOW())
				ON CONFLICT (module, ref_id) DO UPDATE SET je_id = EXCLUDED.je_id`,
				module, refID, jeID)
			if err != nil {
				return fmt.Errorf("link source (%s, %s -> je %d): %w", module, refID, jeID, err)
			}
			return nil
		}

		// -------------------------------------------------------------------------
		// 1. Posted & Paid AR Invoices (Debit 1200 Piutang, Credit 4100 Rev, Credit 2210 PPN)
		// -------------------------------------------------------------------------
		type arInvoiceRow struct {
			id        int64
			number    string
			subtotal  float64
			taxAmount float64
			total     float64
			status    string
			txDate    time.Time
			actorID   int64
		}

		arRows, err := tx.Query(ctx, `
			SELECT id, number, subtotal, tax_amount, total, status,
			       COALESCE(posted_at, created_at) AS tx_date,
			       COALESCE(posted_by, created_by, 1) AS actor_id
			FROM ar_invoices
			WHERE status IN ('POSTED', 'PAID')
			ORDER BY id`)
		if err != nil {
			return fmt.Errorf("query posted AR invoices: %w", err)
		}

		var arInvoices []arInvoiceRow
		for arRows.Next() {
			var r arInvoiceRow
			if err := arRows.Scan(&r.id, &r.number, &r.subtotal, &r.taxAmount, &r.total, &r.status, &r.txDate, &r.actorID); err != nil {
				arRows.Close()
				return fmt.Errorf("scan AR invoice: %w", err)
			}
			arInvoices = append(arInvoices, r)
		}
		arRows.Close()

		for _, inv := range arInvoices {
			sourceID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("ARINV:%d", inv.id)))
			compID := companyNTP
			brID := branchHQ
			whID := whFG

			lines := []journalLineDraft{
				{AccountCode: "1200", Debit: inv.total, Credit: 0, CompanyID: &compID, BranchID: &brID, WarehouseID: &whID},
				{AccountCode: "4100", Debit: 0, Credit: inv.subtotal, CompanyID: &compID, BranchID: &brID, WarehouseID: &whID},
			}
			if inv.taxAmount > 0 {
				lines = append(lines, journalLineDraft{
					AccountCode: "2210",
					Debit:       0,
					Credit:      inv.taxAmount,
					CompanyID:   &compID,
					BranchID:    &brID,
					WarehouseID: &whID,
				})
			}

			jeID, err := postJournal(journalEntryDraft{
				SourceModule: "SALES.AR_INVOICE",
				SourceID:     sourceID,
				Memo:         fmt.Sprintf("AR Invoice %s", inv.number),
				Date:         inv.txDate,
				PostedBy:     &inv.actorID,
				Lines:        lines,
			})
			if err != nil {
				return err
			}

			if err := linkSource("SALES.AR_INVOICE", sourceID, jeID); err != nil {
				return err
			}
		}

		// -------------------------------------------------------------------------
		// 2. AR Payments Received (Debit 1110 Bank BCA, Credit 1200 Piutang Usaha)
		// -------------------------------------------------------------------------
		type arPaymentRow struct {
			id        int64
			number    string
			invNumber string
			amount    float64
			paidAt    time.Time
			actorID   int64
		}

		payRows, err := tx.Query(ctx, `
			SELECT p.id, p.number, i.number, p.amount, p.paid_at, COALESCE(p.created_by, 1)
			FROM ar_payments p
			JOIN ar_invoices i ON i.id = p.ar_invoice_id
			ORDER BY p.id`)
		if err != nil {
			return fmt.Errorf("query AR payments: %w", err)
		}

		var arPayments []arPaymentRow
		for payRows.Next() {
			var r arPaymentRow
			if err := payRows.Scan(&r.id, &r.number, &r.invNumber, &r.amount, &r.paidAt, &r.actorID); err != nil {
				payRows.Close()
				return fmt.Errorf("scan AR payment: %w", err)
			}
			arPayments = append(arPayments, r)
		}
		payRows.Close()

		for _, p := range arPayments {
			sourceID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("ARPAY:%d", p.id)))
			compID := companyNTP
			brID := branchHQ

			lines := []journalLineDraft{
				{AccountCode: "1110", Debit: p.amount, Credit: 0, CompanyID: &compID, BranchID: &brID},
				{AccountCode: "1200", Debit: 0, Credit: p.amount, CompanyID: &compID, BranchID: &brID},
			}

			jeID, err := postJournal(journalEntryDraft{
				SourceModule: "SALES.AR_PAYMENT",
				SourceID:     sourceID,
				Memo:         fmt.Sprintf("AR Payment %s for %s", p.number, p.invNumber),
				Date:         p.paidAt,
				PostedBy:     &p.actorID,
				Lines:        lines,
			})
			if err != nil {
				return err
			}

			if err := linkSource("SALES.AR_PAYMENT", sourceID, jeID); err != nil {
				return err
			}
		}

		// -------------------------------------------------------------------------
		// 3. AR Credit Notes (Debit 4100 Revenue, Debit 2210 PPN, Credit 1200 Piutang)
		// -------------------------------------------------------------------------
		type arCreditNoteRow struct {
			id        int64
			number    string
			subtotal  float64
			taxAmount float64
			total     float64
			txDate    time.Time
			actorID   int64
		}

		var crnRows pgx.Rows
		var crnTableExists bool
		_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'ar_credit_notes')`).Scan(&crnTableExists)
		if crnTableExists {
			crnRows, err = tx.Query(ctx, `
				SELECT id, number, subtotal, tax_amount, total,
				       COALESCE(posted_at, created_at) AS tx_date,
				       COALESCE(created_by, 1) AS actor_id
				FROM ar_credit_notes
				WHERE status = 'POSTED'
				ORDER BY id`)
			if err == nil {
				var crNotes []arCreditNoteRow
				for crnRows.Next() {
					var r arCreditNoteRow
					if err := crnRows.Scan(&r.id, &r.number, &r.subtotal, &r.taxAmount, &r.total, &r.txDate, &r.actorID); err == nil {
						crNotes = append(crNotes, r)
					}
				}
				crnRows.Close()

				for _, r := range crNotes {
					sourceID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("ARCRN:%d", r.id)))
					compID := companyNTP
					brID := branchHQ

					lines := []journalLineDraft{
						{AccountCode: "4100", Debit: r.subtotal, Credit: 0, CompanyID: &compID, BranchID: &brID},
					}
					if r.taxAmount > 0 {
						lines = append(lines, journalLineDraft{
							AccountCode: "2210",
							Debit:       r.taxAmount,
							Credit:      0,
							CompanyID:   &compID,
							BranchID:    &brID,
						})
					}
					lines = append(lines, journalLineDraft{
						AccountCode: "1200",
						Debit:       0,
						Credit:      r.total,
						CompanyID:   &compID,
						BranchID:    &brID,
					})

					jeID, err := postJournal(journalEntryDraft{
						SourceModule: "SALES.AR_CREDIT_NOTE",
						SourceID:     sourceID,
						Memo:         fmt.Sprintf("AR Credit Note %s", r.number),
						Date:         r.txDate,
						PostedBy:     &r.actorID,
						Lines:        lines,
					})
					if err == nil {
						_ = linkSource("SALES.AR_CREDIT_NOTE", sourceID, jeID)
					}
				}
			}
		}

		// -------------------------------------------------------------------------
		// 4. Posted & Paid AP Invoices (Debit 1310 Inv, Debit 1410 PPN, Credit 2100/2150 AP)
		// -------------------------------------------------------------------------
		type apInvoiceRow struct {
			id         int64
			number     string
			currency   string
			subtotal   float64
			taxAmount  float64
			total      float64
			baseAmount float64
			fxRate     float64
			txDate     time.Time
			actorID    int64
		}

		apRows, err := tx.Query(ctx, `
			SELECT id, number, currency, subtotal, tax_amount, total,
			       COALESCE(base_amount, total) AS base_amount,
			       COALESCE(fx_rate, 1.0) AS fx_rate,
			       COALESCE(posted_at, issued_at, created_at) AS tx_date,
			       COALESCE(posted_by, created_by, 1) AS actor_id
			FROM ap_invoices
			WHERE status IN ('POSTED', 'PAID')
			ORDER BY id`)
		if err != nil {
			return fmt.Errorf("query posted AP invoices: %w", err)
		}

		var apInvoices []apInvoiceRow
		for apRows.Next() {
			var r apInvoiceRow
			if err := apRows.Scan(&r.id, &r.number, &r.currency, &r.subtotal, &r.taxAmount, &r.total, &r.baseAmount, &r.fxRate, &r.txDate, &r.actorID); err != nil {
				apRows.Close()
				return fmt.Errorf("scan AP invoice: %w", err)
			}
			apInvoices = append(apInvoices, r)
		}
		apRows.Close()

		for _, inv := range apInvoices {
			sourceID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("APINV:%d", inv.id)))
			compID := companyNTP
			brID := branchHQ

			var subtotalBase, taxBase, totalBase float64
			var apAccount string

			if inv.currency == "USD" {
				rate := inv.fxRate
				if rate <= 0 {
					rate = 16000.0
				}
				subtotalBase = math.Round(inv.subtotal*rate*100) / 100
				taxBase = math.Round(inv.taxAmount*rate*100) / 100
				totalBase = subtotalBase + taxBase
				apAccount = "2150" // Hutang Usaha Valas USD
			} else {
				subtotalBase = inv.subtotal
				taxBase = inv.taxAmount
				totalBase = inv.total
				apAccount = "2100" // Hutang Usaha IDR
			}

			lines := []journalLineDraft{
				{AccountCode: "1310", Debit: subtotalBase, Credit: 0, CompanyID: &compID, BranchID: &brID},
			}
			if taxBase > 0 {
				lines = append(lines, journalLineDraft{
					AccountCode: "1410",
					Debit:       taxBase,
					Credit:      0,
					CompanyID:   &compID,
					BranchID:    &brID,
				})
			}
			lines = append(lines, journalLineDraft{
				AccountCode: apAccount,
				Debit:       0,
				Credit:      totalBase,
				CompanyID:   &compID,
				BranchID:    &brID,
			})

			jeID, err := postJournal(journalEntryDraft{
				SourceModule: "PROCUREMENT.AP_INVOICE",
				SourceID:     sourceID,
				Memo:         fmt.Sprintf("AP Invoice %s", inv.number),
				Date:         inv.txDate,
				PostedBy:     &inv.actorID,
				Lines:        lines,
			})
			if err != nil {
				return err
			}

			if err := linkSource("PROCUREMENT.AP_INVOICE", sourceID, jeID); err != nil {
				return err
			}
		}

		// -------------------------------------------------------------------------
		// 5. AP Payments Settled (Debit 2100/2150 AP, Credit 1110/1112 Bank)
		// -------------------------------------------------------------------------
		type apPaymentRow struct {
			id         int64
			number     string
			invNumber  string
			amount     float64
			currency   string
			baseAmount float64
			fxRate     float64
			paidAt     time.Time
			actorID    int64
		}

		apPayRows, err := tx.Query(ctx, `
			SELECT p.id, p.number, i.number, p.amount,
			       COALESCE(p.currency, 'IDR'),
			       COALESCE(p.base_amount, p.amount),
			       COALESCE(p.fx_rate, 1.0),
			       p.paid_at,
			       COALESCE(p.created_by, 1)
			FROM ap_payments p
			JOIN ap_invoices i ON i.id = p.ap_invoice_id
			ORDER BY p.id`)
		if err != nil {
			return fmt.Errorf("query AP payments: %w", err)
		}

		var apPayments []apPaymentRow
		for apPayRows.Next() {
			var r apPaymentRow
			if err := apPayRows.Scan(&r.id, &r.number, &r.invNumber, &r.amount, &r.currency, &r.baseAmount, &r.fxRate, &r.paidAt, &r.actorID); err != nil {
				apPayRows.Close()
				return fmt.Errorf("scan AP payment: %w", err)
			}
			apPayments = append(apPayments, r)
		}
		apPayRows.Close()

		for _, p := range apPayments {
			sourceID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("APPAY:%d", p.id)))
			compID := companyNTP
			brID := branchHQ

			var amountIDR float64
			var apAcc, bankAcc string

			if p.currency == "USD" {
				rate := p.fxRate
				if rate <= 0 {
					rate = 16000.0
				}
				amountIDR = math.Round(p.amount*rate*100) / 100
				apAcc = "2150"   // Hutang Usaha Valas USD
				bankAcc = "1112" // Kas Operasional BCA USD
			} else {
				amountIDR = p.amount
				apAcc = "2100"   // Hutang Usaha IDR
				bankAcc = "1110" // Kas Operasional BCA IDR
			}

			lines := []journalLineDraft{
				{AccountCode: apAcc, Debit: amountIDR, Credit: 0, CompanyID: &compID, BranchID: &brID},
				{AccountCode: bankAcc, Debit: 0, Credit: amountIDR, CompanyID: &compID, BranchID: &brID},
			}

			jeID, err := postJournal(journalEntryDraft{
				SourceModule: "PROCUREMENT.AP_PAYMENT",
				SourceID:     sourceID,
				Memo:         fmt.Sprintf("AP Payment %s for %s", p.number, p.invNumber),
				Date:         p.paidAt,
				PostedBy:     &p.actorID,
				Lines:        lines,
			})
			if err != nil {
				return err
			}

			if err := linkSource("PROCUREMENT.AP_PAYMENT", sourceID, jeID); err != nil {
				return err
			}
		}

		// -------------------------------------------------------------------------
		// 6. AP Debit Notes (Debit 2100 AP, Credit 1310 Inv, Credit 1410 PPN)
		// -------------------------------------------------------------------------
		type apDebitNoteRow struct {
			id        int64
			number    string
			total     float64
			txDate    time.Time
			actorID   int64
		}

		var apdnTableExists bool
		_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'ap_debit_notes')`).Scan(&apdnTableExists)
		if apdnTableExists {
			dnRows, err := tx.Query(ctx, `
				SELECT id, number, total,
				       COALESCE(posted_at, created_at) AS tx_date,
				       COALESCE(created_by, 1) AS actor_id
				FROM ap_debit_notes
				WHERE status = 'POSTED'
				ORDER BY id`)
			if err == nil {
				var debitNotes []apDebitNoteRow
				for dnRows.Next() {
					var r apDebitNoteRow
					if err := dnRows.Scan(&r.id, &r.number, &r.total, &r.txDate, &r.actorID); err == nil {
						debitNotes = append(debitNotes, r)
					}
				}
				dnRows.Close()

				for _, r := range debitNotes {
					sourceID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("APDN:%d", r.id)))
					compID := companyNTP
					brID := branchHQ

					sub := math.Round((r.total/1.11)*100) / 100
					tax := math.Round((r.total-sub)*100) / 100

					lines := []journalLineDraft{
						{AccountCode: "2100", Debit: r.total, Credit: 0, CompanyID: &compID, BranchID: &brID},
						{AccountCode: "1310", Debit: 0, Credit: sub, CompanyID: &compID, BranchID: &brID},
					}
					if tax > 0 {
						lines = append(lines, journalLineDraft{
							AccountCode: "1410",
							Debit:       0,
							Credit:      tax,
							CompanyID:   &compID,
							BranchID:    &brID,
						})
					}

					jeID, err := postJournal(journalEntryDraft{
						SourceModule: "PROCUREMENT.AP_DEBIT_NOTE",
						SourceID:     sourceID,
						Memo:         fmt.Sprintf("AP Debit Note %s", r.number),
						Date:         r.txDate,
						PostedBy:     &r.actorID,
						Lines:        lines,
					})
					if err == nil {
						_ = linkSource("PROCUREMENT.AP_DEBIT_NOTE", sourceID, jeID)
					}
				}
			}
		}

		// -------------------------------------------------------------------------
		// 7. Posted Payroll Run (Debit 5200/5210, Credit 2310/2220/2320)
		// -------------------------------------------------------------------------
		type payrollRunSummary struct {
			id                   int64
			runUUID              string
			companyID            int64
			txDate               time.Time
			actorID              int64
			totalGross           float64
			totalEmployerBPJS    float64
			totalEmployeeBPJS    float64
			totalPPh21           float64
			totalOtherDeductions float64
			totalNetPay          float64
		}

		var payrollTableExists bool
		_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'payroll_runs')`).Scan(&payrollTableExists)
		if payrollTableExists {
			prRow := tx.QueryRow(ctx, `
				SELECT r.id, r.run_uuid, r.company_id,
				       COALESCE(r.posted_at, r.created_at) AS tx_date,
				       COALESCE(r.created_by, 1) AS actor_id,
				       COALESCE(SUM(l.gross), 0) AS total_gross,
				       COALESCE(SUM(l.employer_bpjs), 0) AS total_employer_bpjs,
				       COALESCE(SUM(l.employee_bpjs), 0) AS total_employee_bpjs,
				       COALESCE(SUM(l.pph21), 0) AS total_pph21,
				       COALESCE(SUM(l.other_deductions), 0) AS total_other_deductions,
				       COALESCE(SUM(l.net_pay), 0) AS total_net_pay
				FROM payroll_runs r
				JOIN payroll_run_lines l ON l.run_id = r.id
				WHERE r.status = 'POSTED'
				GROUP BY r.id, r.run_uuid, r.company_id, r.posted_at, r.created_at, r.created_by
				LIMIT 1`)

			var prSum payrollRunSummary
			if err := prRow.Scan(&prSum.id, &prSum.runUUID, &prSum.companyID, &prSum.txDate, &prSum.actorID,
				&prSum.totalGross, &prSum.totalEmployerBPJS, &prSum.totalEmployeeBPJS, &prSum.totalPPh21,
				&prSum.totalOtherDeductions, &prSum.totalNetPay); err == nil {

				sourceID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("PAYROLL:%d", prSum.id)))
				compID := prSum.companyID
				if compID == 0 {
					compID = companyNTP
				}
				brID := branchHQ

				bpjsPayable := math.Round((prSum.totalEmployeeBPJS+prSum.totalEmployerBPJS)*100) / 100
				netPayable := math.Round((prSum.totalNetPay+prSum.totalOtherDeductions)*100) / 100

				lines := []journalLineDraft{
					{AccountCode: "5200", Debit: prSum.totalGross, Credit: 0, CompanyID: &compID, BranchID: &brID},
					{AccountCode: "5210", Debit: prSum.totalEmployerBPJS, Credit: 0, CompanyID: &compID, BranchID: &brID},
					{AccountCode: "2310", Debit: 0, Credit: netPayable, CompanyID: &compID, BranchID: &brID},
					{AccountCode: "2220", Debit: 0, Credit: prSum.totalPPh21, CompanyID: &compID, BranchID: &brID},
					{AccountCode: "2320", Debit: 0, Credit: bpjsPayable, CompanyID: &compID, BranchID: &brID},
				}

				jeID, err := postJournal(journalEntryDraft{
					SourceModule: "HR.PAYROLL",
					SourceID:     sourceID,
					Memo:         fmt.Sprintf("Payroll Run %s (July 2026)", prSum.runUUID),
					Date:         prSum.txDate,
					PostedBy:     &prSum.actorID,
					Lines:        lines,
				})
				if err != nil {
					return fmt.Errorf("post payroll journal: %w", err)
				}

				if err := linkSource("HR.PAYROLL", sourceID, jeID); err != nil {
					return err
				}
			}
		}

		// -------------------------------------------------------------------------
		// 8. Monthly Fixed Asset Depreciation (Debit 5400 Exp, Credit 1590 Accum)
		// -------------------------------------------------------------------------
		type fixedAssetRow struct {
			id        int64
			number    string
			cost      float64
			residual  float64
			usefulMos int
			inService time.Time
			status    string
		}

		var assetsTableExists bool
		_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'fixed_assets')`).Scan(&assetsTableExists)
		if assetsTableExists {
			faRows, err := tx.Query(ctx, `
				SELECT id, number, acquisition_cost, residual_value, useful_life_months, in_service_date, status
				FROM fixed_assets
				WHERE in_service_date IS NOT NULL
				ORDER BY id`)
			if err == nil {
				var faList []fixedAssetRow
				for faRows.Next() {
					var fa fixedAssetRow
					if err := faRows.Scan(&fa.id, &fa.number, &fa.cost, &fa.residual, &fa.usefulMos, &fa.inService, &fa.status); err == nil {
						faList = append(faList, fa)
					}
				}
				faRows.Close()

				monthEndDates := []string{
					"2026-03-31",
					"2026-04-30",
					"2026-05-31",
					"2026-06-30",
					"2026-07-31",
					"2026-08-31",
				}

				for _, medStr := range monthEndDates {
					med := ParseDate(medStr)
					var monthDeprTotal float64

					for _, fa := range faList {
						if fa.usefulMos <= 0 {
							continue
						}
						if fa.inService.After(med) {
							continue
						}
						// If disposed (AST-VEH-003 in July 2026), don't depreciate in July or August
						if fa.status == "DISPOSED" && medStr >= "2026-07-31" {
							continue
						}

						monthlyAmt := (fa.cost - fa.residual) / float64(fa.usefulMos)
						monthDeprTotal += monthlyAmt
					}

					monthDeprTotal = math.Round(monthDeprTotal*100) / 100
					if monthDeprTotal > 0 {
						sourceID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("DEPR:%s", medStr)))
						compID := companyNTP
						brID := branchHQ

						lines := []journalLineDraft{
							{AccountCode: "5400", Debit: monthDeprTotal, Credit: 0, CompanyID: &compID, BranchID: &brID},
							{AccountCode: "1590", Debit: 0, Credit: monthDeprTotal, CompanyID: &compID, BranchID: &brID},
						}

						jeID, err := postJournal(journalEntryDraft{
							SourceModule: "ASSETS.DEPRECIATION",
							SourceID:     sourceID,
							Memo:         fmt.Sprintf("Fixed Asset Depreciation %s", medStr[:7]),
							Date:         med,
							PostedBy:     &adminID,
							Lines:        lines,
						})
						if err != nil {
							return fmt.Errorf("post depr journal for %s: %w", medStr, err)
						}

						_ = linkSource("ASSETS.DEPRECIATION", sourceID, jeID)
					}
				}

				// Asset Disposal: AST-VEH-003 on 2026-07-15
				type disposalRow struct {
					id        int64
					assetID   int64
					date      time.Time
					proceeds  float64
					cost      float64
					accumDepr float64
					number    string
				}

				var dispTableExists bool
				_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'fixed_asset_disposals')`).Scan(&dispTableExists)
				if dispTableExists {
					dRow := tx.QueryRow(ctx, `
						SELECT d.id, d.asset_id, d.disposal_date, d.proceeds, a.acquisition_cost, a.accumulated_depreciation, a.number
						FROM fixed_asset_disposals d
						JOIN fixed_assets a ON a.id = d.asset_id
						LIMIT 1`)

					var disp disposalRow
					if err := dRow.Scan(&disp.id, &disp.assetID, &disp.date, &disp.proceeds, &disp.cost, &disp.accumDepr, &disp.number); err == nil {
						sourceID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("DISPOSAL:%d", disp.id)))
						compID := companyNTP
						brID := branchHQ

						bookValue := disp.cost - disp.accumDepr
						lossOnDisposal := math.Round((bookValue-disp.proceeds)*100) / 100

						lines := []journalLineDraft{
							{AccountCode: "1110", Debit: disp.proceeds, Credit: 0, CompanyID: &compID, BranchID: &brID},
							{AccountCode: "1590", Debit: disp.accumDepr, Credit: 0, CompanyID: &compID, BranchID: &brID},
						}
						if lossOnDisposal > 0 {
							lines = append(lines, journalLineDraft{
								AccountCode: "5500",
								Debit:       lossOnDisposal,
								Credit:      0,
								CompanyID:   &compID,
								BranchID:    &brID,
							})
						}
						lines = append(lines, journalLineDraft{
							AccountCode: "1520",
							Debit:       0,
							Credit:      disp.cost,
							CompanyID:   &compID,
							BranchID:    &brID,
						})

						jeID, err := postJournal(journalEntryDraft{
							SourceModule: "ASSETS.DISPOSAL",
							SourceID:     sourceID,
							Memo:         fmt.Sprintf("Fixed Asset Disposal %s", disp.number),
							Date:         disp.date,
							PostedBy:     &adminID,
							Lines:        lines,
						})
						if err != nil {
							return fmt.Errorf("post disposal journal: %w", err)
						}
						_, _ = tx.Exec(ctx, `UPDATE fixed_asset_disposals SET journal_entry_id = $1 WHERE id = $2`, jeID, disp.id)
						if err := linkSource("ASSETS.DISPOSAL", sourceID, jeID); err != nil {
							return err
						}
					}
				}
			}
		}

		// -------------------------------------------------------------------------
		// 9. Inventory Adjustments & Stock Takes (Debit 1300/1310/1330, Credit 5600)
		// -------------------------------------------------------------------------
		type invAdjRow struct {
			id      int64
			number  string
			whID    int64
			adjDate time.Time
			actorID int64
		}

		var adjTableExists bool
		_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'inventory_adjustments')`).Scan(&adjTableExists)
		if adjTableExists {
			adjRows, err := tx.Query(ctx, `
				SELECT id, number, warehouse_id,
				       COALESCE(posted_at, adjustment_at, created_at) AS adj_date,
				       COALESCE(posted_by, created_by, 1) AS actor_id
				FROM inventory_adjustments
				WHERE status = 'POSTED'
				ORDER BY id`)
			if err == nil {
				var adjs []invAdjRow
				for adjRows.Next() {
					var r invAdjRow
					if err := adjRows.Scan(&r.id, &r.number, &r.whID, &r.adjDate, &r.actorID); err == nil {
						adjs = append(adjs, r)
					}
				}
				adjRows.Close()

				for _, adj := range adjs {
					type lineVal struct {
						qty     float64
						avgCost float64
					}
					var linesVal []lineVal

					lRows, err := tx.Query(ctx, `
						SELECT l.qty, COALESCE(b.avg_cost, 100000.0)
						FROM inventory_adjustment_lines l
						LEFT JOIN inventory_balances b ON b.product_id = l.product_id AND b.warehouse_id = $1
						WHERE l.adjustment_id = $2`,
						adj.whID, adj.id)
					if err == nil {
						for lRows.Next() {
							var lv lineVal
							if err := lRows.Scan(&lv.qty, &lv.avgCost); err == nil {
								linesVal = append(linesVal, lv)
							}
						}
						lRows.Close()
					}

					var totalGain, totalLoss float64
					for _, lv := range linesVal {
						amt := math.Abs(lv.qty) * lv.avgCost
						if lv.qty > 0 {
							totalGain += amt
						} else {
							totalLoss += amt
						}
					}

					totalGain = math.Round(totalGain*100) / 100
					totalLoss = math.Round(totalLoss*100) / 100

					var lines []journalLineDraft
					compID := companyNTP
					brID := branchHQ
					whID := adj.whID

					if totalGain > 0 {
						lines = append(lines,
							journalLineDraft{AccountCode: "1300", Debit: totalGain, Credit: 0, CompanyID: &compID, BranchID: &brID, WarehouseID: &whID},
							journalLineDraft{AccountCode: "5600", Debit: 0, Credit: totalGain, CompanyID: &compID, BranchID: &brID, WarehouseID: &whID},
						)
					}
					if totalLoss > 0 {
						lines = append(lines,
							journalLineDraft{AccountCode: "5600", Debit: totalLoss, Credit: 0, CompanyID: &compID, BranchID: &brID, WarehouseID: &whID},
							journalLineDraft{AccountCode: "1300", Debit: 0, Credit: totalLoss, CompanyID: &compID, BranchID: &brID, WarehouseID: &whID},
						)
					}

					if len(lines) > 0 {
						sourceID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("INVADJ:%d", adj.id)))
						jeID, err := postJournal(journalEntryDraft{
							SourceModule: "INVENTORY.ADJUSTMENT",
							SourceID:     sourceID,
							Memo:         fmt.Sprintf("Inventory Adjustment %s", adj.number),
							Date:         adj.adjDate,
							PostedBy:     &adj.actorID,
							Lines:        lines,
						})
						if err != nil {
							return fmt.Errorf("post inventory adjustment journal for %s: %w", adj.number, err)
						}
						if err := linkSource("INVENTORY.ADJUSTMENT", sourceID, jeID); err != nil {
							return err
						}
					}
				}
			}
		}

		// -------------------------------------------------------------------------
		// 10. POS Completed Tickets & Refunds (Debit 1110, Credit 4200, Credit 2210)
		// -------------------------------------------------------------------------
		type posTicketRow struct {
			id        int64
			number    string
			subtotal  float64
			taxAmount float64
			total     float64
			status    string
			txDate    time.Time
			actorID   int64
		}

		var posTableExists bool
		_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'pos_tickets')`).Scan(&posTableExists)
		if posTableExists {
			tktRows, err := tx.Query(ctx, `
				SELECT id, number, subtotal, tax_amount, total, status, created_at, COALESCE(created_by, 1)
				FROM pos_tickets
				WHERE status IN ('COMPLETED', 'REFUNDED')
				ORDER BY id`)
			if err == nil {
				var tickets []posTicketRow
				for tktRows.Next() {
					var r posTicketRow
					if err := tktRows.Scan(&r.id, &r.number, &r.subtotal, &r.taxAmount, &r.total, &r.status, &r.txDate, &r.actorID); err == nil {
						tickets = append(tickets, r)
					}
				}
				tktRows.Close()

				for _, t := range tickets {
					compID := companyNTP
					brID := branchHQ
					whID := whFG

					if t.status == "COMPLETED" {
						sourceID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("POSTKT:%d", t.id)))
						lines := []journalLineDraft{
							{AccountCode: "1110", Debit: t.total, Credit: 0, CompanyID: &compID, BranchID: &brID, WarehouseID: &whID},
							{AccountCode: "4200", Debit: 0, Credit: t.subtotal, CompanyID: &compID, BranchID: &brID, WarehouseID: &whID},
						}
						if t.taxAmount > 0 {
							lines = append(lines, journalLineDraft{
								AccountCode: "2210",
								Debit:       0,
								Credit:      t.taxAmount,
								CompanyID:   &compID,
								BranchID:    &brID,
								WarehouseID: &whID,
							})
						}

						jeID, err := postJournal(journalEntryDraft{
							SourceModule: "POS.SALE",
							SourceID:     sourceID,
							Memo:         fmt.Sprintf("POS Ticket %s", t.number),
							Date:         t.txDate,
							PostedBy:     &t.actorID,
							Lines:        lines,
						})
						if err != nil {
							return fmt.Errorf("post POS sale journal for %s: %w", t.number, err)
						}
						if err := linkSource("POS.SALE", sourceID, jeID); err != nil {
							return err
						}
					} else if t.status == "REFUNDED" {
						sourceID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("POSREFUND:%d", t.id)))
						lines := []journalLineDraft{
							{AccountCode: "4200", Debit: t.subtotal, Credit: 0, CompanyID: &compID, BranchID: &brID, WarehouseID: &whID},
						}
						if t.taxAmount > 0 {
							lines = append(lines, journalLineDraft{
								AccountCode: "2210",
								Debit:       t.taxAmount,
								Credit:      0,
								CompanyID:   &compID,
								BranchID:    &brID,
								WarehouseID: &whID,
							})
						}
						lines = append(lines, journalLineDraft{
							AccountCode: "1110",
							Debit:       0,
							Credit:      t.total,
							CompanyID:   &compID,
							BranchID:    &brID,
							WarehouseID: &whID,
						})

						jeID, err := postJournal(journalEntryDraft{
							SourceModule: "POS.REFUND",
							SourceID:     sourceID,
							Memo:         fmt.Sprintf("POS Refund Ticket %s", t.number),
							Date:         t.txDate,
							PostedBy:     &t.actorID,
							Lines:        lines,
						})
						if err != nil {
							return fmt.Errorf("post POS refund journal for %s: %w", t.number, err)
						}
						if err := linkSource("POS.REFUND", sourceID, jeID); err != nil {
							return err
						}
					}
				}
			}
		}

		// -------------------------------------------------------------------------
		// 11. Intercompany Elimination Entries (Debit 4100 IC Rev, Credit 5200 IC Expense)
		// -------------------------------------------------------------------------
		type elimRunRow struct {
			id         int64
			periodCode string
			ruleID     int64
			srcCompID  int64
			tgtCompID  int64
			txDate     time.Time
			actorID    int64
		}

		var elimTableExists bool
		_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'elimination_runs')`).Scan(&elimTableExists)
		if elimTableExists {
			elimRows, err := tx.Query(ctx, `
				SELECT r.id, p.code, r.rule_id, er.source_company_id, er.target_company_id,
				       COALESCE(r.posted_at, r.created_at) AS tx_date,
				       COALESCE(r.created_by, 1) AS actor_id
				FROM elimination_runs r
				JOIN elimination_rules er ON er.id = r.rule_id
				JOIN accounting_periods ap ON ap.id = r.period_id
				JOIN periods p ON p.id = ap.period_id
				WHERE r.status = 'POSTED'
				ORDER BY r.id`)
			if err == nil {
				var elimRuns []elimRunRow
				for elimRows.Next() {
					var r elimRunRow
					if err := elimRows.Scan(&r.id, &r.periodCode, &r.ruleID, &r.srcCompID, &r.tgtCompID, &r.txDate, &r.actorID); err == nil {
						elimRuns = append(elimRuns, r)
					}
				}
				elimRows.Close()

				for _, erun := range elimRuns {
					sourceID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("ELIMRUN:%d", erun.id)))
					srcComp := erun.srcCompID
					tgtComp := erun.tgtCompID
					elimAmount := 145000000.00 // Rp 145,000,000 intercompany sales & purchases elimination

					lines := []journalLineDraft{
						{AccountCode: "4100", Debit: elimAmount, Credit: 0, CompanyID: &srcComp},
						{AccountCode: "5200", Debit: 0, Credit: elimAmount, CompanyID: &tgtComp},
					}

					jeID, err := postJournal(journalEntryDraft{
						SourceModule: "CONSOL.ELIMINATION",
						SourceID:     sourceID,
						Memo:         fmt.Sprintf("Consolidation Elimination %s Rule %d", erun.periodCode, erun.ruleID),
						Date:         erun.txDate,
						PostedBy:     &erun.actorID,
						Lines:        lines,
					})
					if err != nil {
						return fmt.Errorf("post elimination journal: %w", err)
					}
					_, _ = tx.Exec(ctx, `UPDATE elimination_runs SET journal_entry_id = $1 WHERE id = $2`, jeID, erun.id)
					if err := linkSource("CONSOL.ELIMINATION", sourceID, jeID); err != nil {
						return err
					}
				}
			}
		}

		// -------------------------------------------------------------------------
		// 12. Refresh Materialized Views
		// -------------------------------------------------------------------------
		if !SkipMaterializedViews {
			views := []string{
				"gl_balances",
				"mv_pl_monthly",
				"mv_cashflow_monthly",
				"mv_ar_aging",
				"mv_ap_aging",
				"mv_consol_balances",
			}

			for _, v := range views {
				var viewExists bool
				_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_matviews WHERE schemaname = 'public' AND matviewname = $1)`, v).Scan(&viewExists)
				if viewExists {
					if _, err := tx.Exec(ctx, fmt.Sprintf("REFRESH MATERIALIZED VIEW %s", v)); err != nil {
						return fmt.Errorf("refresh materialized view %s: %w", v, err)
					}
				}
			}
		}

		return nil
	})
}
