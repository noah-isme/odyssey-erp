package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// seedPhase15Banking seeds 3 bank statements (BCA IDR Reconciled, Mandiri Reconciled, BCA USD Draft)
// with matched and unmatched statement lines, and 24+ direct bank transactions across
// CLEARED, RECONCILED, and PENDING statuses for PT Nusantara Teknik Perkasa.
func seedPhase15Banking(ctx context.Context, sctx *SeedContext) error {
	return ExecInTx(ctx, sctx.Pool, "Phase 15: Banking & Treasury Management", func(tx pgx.Tx) error {
		adminID := sctx.UserIDs["budi.santoso@nusantarateknik.co.id"]
		if adminID == 0 {
			return fmt.Errorf("admin user budi.santoso@nusantarateknik.co.id not found")
		}

		bcaIDR := sctx.BankAccountIDs["0088-2233-4411"]
		mandiriIDR := sctx.BankAccountIDs["1200-0011-22334"]
		bcaUSD := sctx.BankAccountIDs["0088-9988-7700"]

		if bcaIDR == 0 || mandiriIDR == 0 || bcaUSD == 0 {
			// Query directly if not populated in context
			if bcaIDR == 0 {
				_ = tx.QueryRow(ctx, `SELECT id FROM bank_accounts WHERE company_id = $1 AND account_number = '0088-2233-4411'`, sctx.CompanyNTPID).Scan(&bcaIDR)
			}
			if mandiriIDR == 0 {
				_ = tx.QueryRow(ctx, `SELECT id FROM bank_accounts WHERE company_id = $1 AND account_number = '1200-0011-22334'`, sctx.CompanyNTPID).Scan(&mandiriIDR)
			}
			if bcaUSD == 0 {
				_ = tx.QueryRow(ctx, `SELECT id FROM bank_accounts WHERE company_id = $1 AND account_number = '0088-9988-7700'`, sctx.CompanyNTPID).Scan(&bcaUSD)
			}
		}

		if bcaIDR == 0 || mandiriIDR == 0 || bcaUSD == 0 {
			return fmt.Errorf("bank accounts (BCA IDR, Mandiri IDR, BCA USD) not found")
		}

		// -------------------------------------------------------------------------
		// 1. Bank Statements & Statement Lines (3 Statements)
		// -------------------------------------------------------------------------
		type stmtLineDef struct {
			trxDate   string
			desc      string
			amount    float64
			refNum    string
			status    string // 'UNMATCHED', 'SUGGESTED', 'MATCHED'
			matchDoc  string
			matchID   *int64
		}

		type stmtDef struct {
			bankAccID  int64
			stmtDate   string
			startBal   float64
			endBal     float64
			status     string // 'DRAFT', 'RECONCILED'
			importedAt time.Time
			lines      []stmtLineDef
		}

		statements := []stmtDef{
			// Statement 1: BCA Operasional IDR (July 2026 - RECONCILED)
			{
				bankAccID:  bcaIDR,
				stmtDate:   "2026-07-31",
				startBal:   1500000000.00,
				endBal:     1825450000.00,
				status:     "RECONCILED",
				importedAt: time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC),
				lines: []stmtLineDef{
					{trxDate: "2026-07-05", desc: "CR TRSF PT TELKOM INFRASTRUKTUR PELUNASAN INV-202603-0001", amount: 213675000.00, refNum: "BCA-TRSF-20260705-01", status: "MATCHED", matchDoc: "AR_PAYMENT", matchID: nil},
					{trxDate: "2026-07-12", desc: "DB TRSF PT QUECTEL WIRELESS SOLUTIONS SUPPLIER PAYMENT", amount: -125000000.00, refNum: "BCA-TRSF-20260712-02", status: "MATCHED", matchDoc: "AP_PAYMENT", matchID: nil},
					{trxDate: "2026-07-18", desc: "CR TRSF PT PLN NUSANTARA POWER DP TERMIN 1", amount: 150000000.00, refNum: "BCA-TRSF-20260718-03", status: "MATCHED", matchDoc: "AR_PAYMENT", matchID: nil},
					{trxDate: "2026-07-25", desc: "DB TRSF GAJI & TUNJANGAN KARYAWAN JULI 2026 KE MANDIRI", amount: -88225000.00, refNum: "BCA-TRSF-20260725-04", status: "MATCHED", matchDoc: "PAYROLL_RUN", matchID: nil},
					{trxDate: "2026-07-31", desc: "CR BIAYA BUNGA JASA GIRO JULI 2026", amount: 5500000.00, refNum: "BCA-INT-20260731-05", status: "UNMATCHED", matchDoc: "", matchID: nil},
					{trxDate: "2026-07-31", desc: "DB BIAYA ADM BANK & PAJAK BUNGA JASA GIRO", amount: -500000.00, refNum: "BCA-FEE-20260731-06", status: "UNMATCHED", matchDoc: "", matchID: nil},
				},
			},
			// Statement 2: Bank Mandiri Payroll IDR (July 2026 - RECONCILED)
			{
				bankAccID:  mandiriIDR,
				stmtDate:   "2026-07-31",
				startBal:   500000000.00,
				endBal:     411775000.00,
				status:     "RECONCILED",
				importedAt: time.Date(2026, 8, 1, 8, 30, 0, 0, time.UTC),
				lines: []stmtLineDef{
					{trxDate: "2026-07-25", desc: "CR TRSF DARI BCA OPERASIONAL FUNDING PAYROLL JULI", amount: 88225000.00, refNum: "MDR-CR-20260725-01", status: "MATCHED", matchDoc: "BANK_TRANSFER", matchID: nil},
					{trxDate: "2026-07-26", desc: "DB PAYROLL BULK DISBURSEMENT 12 KARYAWAN NTP", amount: -88225000.00, refNum: "MDR-DB-20260726-02", status: "MATCHED", matchDoc: "PAYROLL_RUN", matchID: nil},
					{trxDate: "2026-07-28", desc: "DB BPJS KETENAGAKERJAAN & KESEHATAN IURAN JULI", amount: -87500000.00, refNum: "MDR-DB-20260728-03", status: "MATCHED", matchDoc: "AP_PAYMENT", matchID: nil},
					{trxDate: "2026-07-31", desc: "CR JASA GIRO REKENING PAYROLL MANDIRI", amount: 350000.00, refNum: "MDR-CR-20260731-04", status: "UNMATCHED", matchDoc: "", matchID: nil},
					{trxDate: "2026-07-31", desc: "DB BIAYA ADMINISTRASI REKENING & BUKU CEK", amount: -75000.00, refNum: "MDR-DB-20260731-05", status: "UNMATCHED", matchDoc: "", matchID: nil},
				},
			},
			// Statement 3: BCA Valas USD (August 2026 - DRAFT)
			{
				bankAccID:  bcaUSD,
				stmtDate:   "2026-08-31",
				startBal:   150000.00,
				endBal:     135500.00,
				status:     "DRAFT",
				importedAt: time.Date(2026, 8, 31, 17, 0, 0, 0, time.UTC),
				lines: []stmtLineDef{
					{trxDate: "2026-08-10", desc: "DB OUTWARD TELEGRAPHIC TRANSFER TO STMICROELECTRONICS ASIA", amount: -18500.00, refNum: "BCA-USD-20260810-01", status: "SUGGESTED", matchDoc: "AP_PAYMENT", matchID: nil},
					{trxDate: "2026-08-20", desc: "CR INWARD TELEGRAPHIC TRANSFER FROM OVERSEAS CLIENT SINGAPORE", amount: 4000.00, refNum: "BCA-USD-20260820-02", status: "UNMATCHED", matchDoc: "", matchID: nil},
					{trxDate: "2026-08-31", desc: "DB USD ACCOUNT MAINTENANCE & TELEGRAPHIC CHARGE", amount: -25.00, refNum: "BCA-USD-20260831-03", status: "UNMATCHED", matchDoc: "", matchID: nil},
					{trxDate: "2026-08-31", desc: "CR USD INTEREST ACCRUAL ON DEPOSIT", amount: 25.00, refNum: "BCA-USD-20260831-04", status: "UNMATCHED", matchDoc: "", matchID: nil},
				},
			},
		}

		for _, st := range statements {
			sDate := ParseDate(st.stmtDate)

			var stmtID int64
			// Check if statement already exists for this bank account and date
			err := tx.QueryRow(ctx, `SELECT id FROM bank_statements WHERE bank_account_id = $1 AND statement_date = $2`, st.bankAccID, sDate).Scan(&stmtID)
			if err != nil {
				err = tx.QueryRow(ctx, `
					INSERT INTO bank_statements (
						bank_account_id, statement_date, starting_balance, ending_balance,
						status, imported_at, created_at, updated_at
					)
					VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
					RETURNING id`,
					st.bankAccID, sDate, st.startBal, st.endBal, st.status, st.importedAt,
				).Scan(&stmtID)
			} else {
				_, err = tx.Exec(ctx, `
					UPDATE bank_statements SET
						starting_balance = $1, ending_balance = $2,
						status = $3, updated_at = NOW()
					WHERE id = $4`,
					st.startBal, st.endBal, st.status, stmtID,
				)
			}
			if err != nil {
				return fmt.Errorf("upsert bank_statements for account %d (%s): %w", st.bankAccID, st.stmtDate, err)
			}

			// Clean existing statement lines
			_, _ = tx.Exec(ctx, `DELETE FROM bank_statement_lines WHERE statement_id = $1`, stmtID)

			for _, l := range st.lines {
				lDate := ParseDate(l.trxDate)
				_, err := tx.Exec(ctx, `
					INSERT INTO bank_statement_lines (
						statement_id, trx_date, description, amount, reference_number,
						status, matched_doc_type, matched_doc_id, created_at, updated_at
					)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())`,
					stmtID, lDate, l.desc, l.amount, l.refNum,
					l.status, l.matchDoc, l.matchID,
				)
				if err != nil {
					return fmt.Errorf("insert bank_statement_line %s: %w", l.refNum, err)
				}
			}
		}

		// -------------------------------------------------------------------------
		// 2. Direct Bank Transactions (25+ Transactions across CLEARED/PENDING/RECONCILED)
		// -------------------------------------------------------------------------
		type bankTxDef struct {
			bankAccID int64
			date      string
			amount    float64
			desc      string
			ref       string
			status    string // 'PENDING', 'CLEARED', 'RECONCILED'
			extRef    string
			fp        string
		}

		bankTransactions := []bankTxDef{
			// March 2026 Transactions (BCA IDR)
			{bcaIDR, "2026-03-10", 213675000.00, "Incoming AR Payment: PT Telkom Infrastruktur (INV-202603-0001)", "BCA-IN-20260310-001", "RECONCILED", "EXT-BCA-20260310-001", "FP-BCA-20260310-001"},
			{bcaIDR, "2026-03-15", -95400000.00, "Outgoing AP Payment: PT Sinar Mas Paper & Packaging (INV-AP-202603-01)", "BCA-OUT-20260315-002", "RECONCILED", "EXT-BCA-20260315-002", "FP-BCA-20260315-002"},
			{bcaIDR, "2026-03-25", -88225000.00, "Funding Payroll Transfer to Mandiri Payroll Account", "BCA-OUT-20260325-003", "RECONCILED", "EXT-BCA-20260325-003", "FP-BCA-20260325-003"},
			{mandiriIDR, "2026-03-25", 88225000.00, "Payroll Transfer Funding from BCA Operasional", "MDR-IN-20260325-001", "RECONCILED", "EXT-MDR-20260325-001", "FP-MDR-20260325-001"},
			{mandiriIDR, "2026-03-26", -88225000.00, "Disbursement: Salary & Allowances 12 Employees (PR-202603)", "MDR-OUT-20260326-002", "RECONCILED", "EXT-MDR-20260326-002", "FP-MDR-20260326-002"},

			// April 2026 Transactions (BCA IDR & USD)
			{bcaIDR, "2026-04-05", 195000000.00, "Incoming AR Payment: PT PLN Nusantara Power (INV-202603-0002)", "BCA-IN-20260405-004", "RECONCILED", "EXT-BCA-20260405-004", "FP-BCA-20260405-004"},
			{bcaIDR, "2026-04-12", -65000000.00, "Outgoing AP Payment: PT Jaya Abadi Logam (PO-202603-0002)", "BCA-OUT-20260412-005", "RECONCILED", "EXT-BCA-20260412-005", "FP-BCA-20260412-005"},
			{bcaUSD, "2026-04-18", -15000.00, "Supplier Advance Wire Transfer: Quectel Wireless Solutions", "USD-OUT-20260418-001", "RECONCILED", "EXT-USD-20260418-001", "FP-USD-20260418-001"},
			{bcaIDR, "2026-04-25", -88225000.00, "Funding Payroll Transfer to Mandiri Payroll Account", "BCA-OUT-20260425-006", "RECONCILED", "EXT-BCA-20260425-006", "FP-BCA-20260425-006"},
			{mandiriIDR, "2026-04-26", -88225000.00, "Disbursement: Salary & Allowances 12 Employees (PR-202604)", "MDR-OUT-20260426-003", "RECONCILED", "EXT-MDR-20260426-003", "FP-MDR-20260426-003"},

			// May 2026 Transactions (BCA IDR)
			{bcaIDR, "2026-05-08", 228000000.00, "Incoming AR Payment: PT PAM Jaya Reservoar Telemetry (INV-202604-0003)", "BCA-IN-20260508-007", "RECONCILED", "EXT-BCA-20260508-007", "FP-BCA-20260508-007"},
			{bcaIDR, "2026-05-15", 342500000.00, "Incoming AR Payment: PT Adaro Energy Indonesia (INV-202604-0004)", "BCA-IN-20260515-008", "RECONCILED", "EXT-BCA-20260515-008", "FP-BCA-20260515-008"},
			{bcaIDR, "2026-05-20", -45000000.00, "Tax Payment: PPN Masa April 2026 via DJP Billing", "BCA-OUT-20260520-009", "RECONCILED", "EXT-BCA-20260520-009", "FP-BCA-20260520-009"},
			{mandiriIDR, "2026-05-26", -88225000.00, "Disbursement: Salary & Allowances 12 Employees (PR-202605)", "MDR-OUT-20260526-004", "RECONCILED", "EXT-MDR-20260526-004", "FP-MDR-20260526-004"},

			// June 2026 Transactions (BCA IDR & Mandiri)
			{bcaIDR, "2026-06-10", 175000000.00, "Incoming AR Payment: PT Astra Otoparts Subassembly (INV-202605-0005)", "BCA-IN-20260610-010", "RECONCILED", "EXT-BCA-20260610-010", "FP-BCA-20260610-010"},
			{bcaIDR, "2026-06-18", -52000000.00, "Outgoing AP Payment: PT Indo Fastener & Hardware (PO-202605-0004)", "BCA-OUT-20260618-011", "RECONCILED", "EXT-BCA-20260618-011", "FP-BCA-20260618-011"},
			{mandiriIDR, "2026-06-26", -88225000.00, "Disbursement: Salary & Allowances 12 Employees (PR-202606)", "MDR-OUT-20260626-005", "RECONCILED", "EXT-MDR-20260626-005", "FP-MDR-20260626-005"},
			{mandiriIDR, "2026-06-28", -87500000.00, "BPJS Ketenagakerjaan & Kesehatan Autodebet Juni 2026", "MDR-OUT-20260628-006", "RECONCILED", "EXT-MDR-20260628-006", "FP-MDR-20260628-006"},

			// July 2026 Transactions (BCA IDR, Mandiri, BCA USD)
			{bcaIDR, "2026-07-05", 213675000.00, "Incoming AR Payment: PT Telkom Infrastruktur (INV-202603-0001)", "BCA-IN-20260705-012", "RECONCILED", "EXT-BCA-20260705-012", "FP-BCA-20260705-012"},
			{bcaIDR, "2026-07-12", -125000000.00, "Outgoing AP Payment: PT Quectel Wireless Solutions", "BCA-OUT-20260712-013", "RECONCILED", "EXT-BCA-20260712-013", "FP-BCA-20260712-013"},
			{bcaIDR, "2026-07-18", 150000000.00, "Incoming AR Payment: PT PLN Nusantara Power Termin 1", "BCA-IN-20260718-014", "RECONCILED", "EXT-BCA-20260718-014", "FP-BCA-20260718-014"},
			{mandiriIDR, "2026-07-26", -88225000.00, "Disbursement: Salary & Allowances 12 Employees (PR-202607)", "MDR-OUT-20260726-007", "RECONCILED", "EXT-MDR-20260726-007", "FP-MDR-20260726-007"},

			// August 2026 Transactions (CLEARED & PENDING)
			{bcaIDR, "2026-08-08", 310000000.00, "Incoming AR Payment: PT Len Industri Smart Defense Nodes (INV-202606-0006)", "BCA-IN-20260808-015", "CLEARED", "EXT-BCA-20260808-015", "FP-BCA-20260808-015"},
			{bcaIDR, "2026-08-15", -78000000.00, "Outgoing AP Payment: PT Schneider Electric Distribution", "BCA-OUT-20260815-016", "CLEARED", "EXT-BCA-20260815-016", "FP-BCA-20260815-016"},
			{bcaUSD, "2026-08-10", -18500.00, "Telegraphic Transfer: STMicroelectronics ARM Cortex MCUs", "USD-OUT-20260810-002", "CLEARED", "EXT-USD-20260810-002", "FP-USD-20260810-002"},
			{mandiriIDR, "2026-08-26", -88225000.00, "Disbursement: Salary & Allowances 12 Employees (PR-202608)", "MDR-OUT-20260826-008", "CLEARED", "EXT-MDR-20260826-008", "FP-MDR-20260826-008"},
			{bcaIDR, "2026-08-29", 145000000.00, "Incoming Customer Deposit: PT Petrokimia Gresik IoT Sensors", "BCA-IN-20260829-017", "PENDING", "EXT-BCA-20260829-017", "FP-BCA-20260829-017"},
			{bcaIDR, "2026-08-30", -35000000.00, "Scheduled Vendor Payment: PT Graha Sarana Logistik", "BCA-OUT-20260830-018", "PENDING", "EXT-BCA-20260830-018", "FP-BCA-20260830-018"},
		}

		for _, btx := range bankTransactions {
			tDate := ParseDate(btx.date)
			_, err := tx.Exec(ctx, `
				INSERT INTO bank_transactions (
					bank_account_id, date, amount, description, reference,
					status, external_reference, fingerprint, created_at, updated_at
				)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
				ON CONFLICT (bank_account_id, external_reference) WHERE external_reference IS NOT NULL DO UPDATE SET
					date = EXCLUDED.date,
					amount = EXCLUDED.amount,
					description = EXCLUDED.description,
					reference = EXCLUDED.reference,
					status = EXCLUDED.status,
					fingerprint = EXCLUDED.fingerprint,
					updated_at = NOW()`,
				btx.bankAccID, tDate, btx.amount, btx.desc, btx.ref,
				btx.status, btx.extRef, btx.fp,
			)
			if err != nil {
				return fmt.Errorf("upsert bank_transactions %q: %w", btx.extRef, err)
			}
		}

		return nil
	})
}
