package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// seedPhase02Finance seeds the PSAK Chart of Accounts, 12 fiscal periods, 28 account mappings,
// 3 bank accounts, 184 daily FX rates (IDR/USD), Indonesian tax compliance setup, and consolidation rules.
func seedPhase02Finance(ctx context.Context, sctx *SeedContext) error {
	return ExecInTx(ctx, sctx.Pool, "Phase 02: Finance & Accounting", func(tx pgx.Tx) error {
		adminID := sctx.UserIDs["budi.santoso@nusantarateknik.co.id"]

		// -------------------------------------------------------------------------
		// 1. PSAK Chart of Accounts (Full 4-Digit Indonesian Hierarchy)
		// -------------------------------------------------------------------------
		type coaItem struct {
			code       string
			name       string
			accType    string
			parentCode string
		}

		accountsTree := []coaItem{
			// ASSETS (1000 Series)
			{"1000", "ASET", "ASSET", ""},
			{"1100", "Kas dan Setara Kas", "ASSET", "1000"},
			{"1110", "Kas Operasional BCA IDR", "ASSET", "1100"},
			{"1111", "Kas Payroll Mandiri IDR", "ASSET", "1100"},
			{"1112", "Kas Operasional BCA USD", "ASSET", "1100"},
			{"1120", "Kas Kecil (Petty Cash)", "ASSET", "1100"},
			{"1200", "Piutang Usaha IDR", "ASSET", "1000"},
			{"1250", "Piutang Usaha Valas USD", "ASSET", "1000"},
			{"1300", "Persediaan Barang", "ASSET", "1000"},
			{"1310", "Persediaan Bahan Baku", "ASSET", "1300"},
			{"1320", "Persediaan Barang Dalam Proses (WIP)", "ASSET", "1300"},
			{"1330", "Persediaan Barang Jadi", "ASSET", "1300"},
			{"1340", "Persediaan Barang Dagang", "ASSET", "1300"},
			{"1400", "Uang Muka & Pajak Dibayar Dimuka", "ASSET", "1000"},
			{"1410", "Pajak Masukan (PPN Masukan 11%)", "ASSET", "1400"},
			{"1420", "Uang Muka Pajak PPh 23", "ASSET", "1400"},
			{"1500", "Aset Tetap", "ASSET", "1000"},
			{"1510", "Mesin & Peralatan Produksi SMT", "ASSET", "1500"},
			{"1520", "Kendaraan Operasional & Angkutan", "ASSET", "1500"},
			{"1530", "Peralatan Kantor & Komputer IT", "ASSET", "1500"},
			{"1590", "Akumulasi Penyusutan Aset Tetap", "ASSET", "1500"},

			// LIABILITIES (2000 Series)
			{"2000", "KEWAJIBAN & LIABILITAS", "LIABILITY", ""},
			{"2100", "Hutang Usaha IDR", "LIABILITY", "2000"},
			{"2150", "Hutang Usaha Valas USD", "LIABILITY", "2000"},
			{"2200", "Hutang Pajak", "LIABILITY", "2000"},
			{"2210", "Hutang Pajak PPN Keluaran 11%", "LIABILITY", "2200"},
			{"2220", "Hutang Pajak PPh 21 Karyawan", "LIABILITY", "2200"},
			{"2230", "Hutang Pajak PPh 23 Jasa", "LIABILITY", "2200"},
			{"2300", "Hutang Gaji & Manfaat Karyawan", "LIABILITY", "2000"},
			{"2310", "Hutang Gaji & Upah (Payroll Payable)", "LIABILITY", "2300"},
			{"2320", "Hutang BPJS Kesehatan & Ketenagakerjaan", "LIABILITY", "2300"},
			{"2400", "Hutang Penerimaan Barang Belum Ditagih (GR/IR)", "LIABILITY", "2000"},

			// EQUITY (3000 Series)
			{"3000", "EKUITAS", "EQUITY", ""},
			{"3100", "Modal Saham Disetor", "EQUITY", "3000"},
			{"3200", "Saldo Laba Ditahan (Retained Earnings)", "EQUITY", "3000"},
			{"3300", "Laba/Rugi Periode Berjalan", "EQUITY", "3000"},

			// REVENUE (4000 Series)
			{"4000", "PENDAPATAN", "REVENUE", ""},
			{"4100", "Penjualan Produk IoT (Manufacturing)", "REVENUE", "4000"},
			{"4200", "Penjualan Perangkat Distribusi (Trading)", "REVENUE", "4000"},
			{"4300", "Pendapatan Jasa Engineering & Instalasi", "REVENUE", "4000"},
			{"4400", "Keuntungan Selisih Kurs (FX Gain)", "REVENUE", "4000"},

			// EXPENSES & COGS (5000 Series)
			{"5000", "BEBAN & HPP", "EXPENSE", ""},
			{"5100", "Beban Pokok Penjualan (COGS)", "EXPENSE", "5000"},
			{"5200", "Beban Gaji & Tunjangan Karyawan", "EXPENSE", "5000"},
			{"5210", "Beban BPJS Porsi Pemberi Kerja", "EXPENSE", "5000"},
			{"5300", "Beban Operasional, Pemasaran & Administrasi", "EXPENSE", "5000"},
			{"5400", "Beban Penyusutan Aset Tetap", "EXPENSE", "5000"},
			{"5500", "Kerugian Selisih Kurs (FX Loss)", "EXPENSE", "5000"},
			{"5600", "Selisih & Penyesuaian Persediaan (Stock Gain/Loss)", "EXPENSE", "5000"},
		}

		for _, a := range accountsTree {
			var parentID *int64
			if a.parentCode != "" {
				if pid, ok := sctx.AccountIDs[a.parentCode]; ok {
					parentID = &pid
				} else {
					var fetchedID int64
					err := tx.QueryRow(ctx, `SELECT id FROM accounts WHERE code = $1`, a.parentCode).Scan(&fetchedID)
					if err != nil {
						return fmt.Errorf("lookup parent account %q: %w", a.parentCode, err)
					}
					parentID = &fetchedID
				}
			}

			var accID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO accounts (code, name, type, parent_id, is_active, company_id, created_at, updated_at)
				VALUES ($1, $2, $3::account_type, $4, TRUE, $5, NOW(), NOW())
				ON CONFLICT (code) DO UPDATE SET
					name = EXCLUDED.name,
					type = EXCLUDED.type,
					parent_id = EXCLUDED.parent_id,
					is_active = TRUE,
					company_id = EXCLUDED.company_id,
					updated_at = NOW()
				RETURNING id`, a.code, a.name, a.accType, parentID, sctx.CompanyNTPID).Scan(&accID)
			if err != nil {
				return fmt.Errorf("upsert account %q: %w", a.code, err)
			}
			sctx.AccountIDs[a.code] = accID
		}

		// -------------------------------------------------------------------------
		// 2. Accounting Periods (12 Monthly Periods for 2026)
		// -------------------------------------------------------------------------
		year := 2026
		for month := 1; month <= 12; month++ {
			startDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
			endDate := startDate.AddDate(0, 1, -1)
			code := fmt.Sprintf("%d-%02d", year, month)

			var periodID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO periods (code, start_date, end_date, status, created_at, updated_at)
				VALUES ($1, $2, $3, 'OPEN', NOW(), NOW())
				ON CONFLICT (code) DO UPDATE SET
					start_date = EXCLUDED.start_date,
					end_date = EXCLUDED.end_date,
					status = 'OPEN',
					updated_at = NOW()
				RETURNING id`, code, startDate, endDate).Scan(&periodID)
			if err != nil {
				return fmt.Errorf("upsert period %q: %w", code, err)
			}
			sctx.PeriodIDs[code] = periodID

			// Seed company-specific accounting period linked to periods(id)
			periodName := fmt.Sprintf("%s-%s", code, "NTP-HQ")
			var apID int64
			err = tx.QueryRow(ctx, `
				INSERT INTO accounting_periods (period_id, company_id, name, start_date, end_date, status, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, 'OPEN', NOW(), NOW())
				ON CONFLICT (period_id) DO UPDATE SET
					company_id = EXCLUDED.company_id,
					name = EXCLUDED.name,
					start_date = EXCLUDED.start_date,
					end_date = EXCLUDED.end_date,
					status = 'OPEN',
					updated_at = NOW()
				RETURNING id`, periodID, sctx.CompanyNTPID, periodName, startDate, endDate).Scan(&apID)
			if err != nil {
				return fmt.Errorf("upsert accounting_period for %q: %w", code, err)
			}
			sctx.AccountingPeriodIDs[code] = apID
		}

		// -------------------------------------------------------------------------
		// 3. System Account Mappings (28 Event Keys)
		// -------------------------------------------------------------------------
		mappings := map[string]string{
			"grn.inventory":                  "1300", // Raw & finished inventory asset
			"grn.grir":                       "2400", // GR/IR Clearing liability
			"ap.invoice.ap":                  "2100", // Accounts Payable IDR
			"ap.invoice.inventory":           "1300", // Inventory stock
			"ap.invoice.expense":             "5300", // Operating expense
			"ap.invoice.tax_input":           "1410", // PPN Masukan 11%
			"ap.payment.cash":                "1110", // Bank Operasional BCA
			"ap.payment.ap":                  "2100", // AP IDR
			"ap.payment.fx_gain":             "4400", // FX Gain
			"ap.payment.fx_loss":             "5500", // FX Loss
			"inventory.adjustment.gain":      "5600", // Stock gain
			"inventory.adjustment.loss":      "5600", // Stock loss
			"inventory.adjustment.inventory": "1300", // Inventory asset
			"ar.invoice.ar":                  "1200", // Accounts Receivable IDR
			"ar.invoice.revenue":             "4100", // Manufacturing IoT Revenue
			"ar.invoice.tax":                 "2210", // PPN Keluaran 11%
			"ar.payment.fx_gain":             "4400", // FX Gain
			"ar.payment.fx_loss":             "5500", // FX Loss
			"fx.realized.gain":               "4400", // Realized FX Gain
			"fx.realized.loss":               "5500", // Realized FX Loss
			"fx.revaluation.gain":            "4400", // Revaluation Gain
			"fx.revaluation.loss":            "5500", // Revaluation Loss
			"ar.credit_note.ar":              "1200", // AR IDR
			"ar.credit_note.revenue":         "4100", // Sales Revenue
			"ar.credit_note.tax":             "2210", // PPN Keluaran
			"ar.return.cogs":                 "5100", // COGS reversal
			"pos.cash":                        "1110", // Cash/Bank
			"pos.sales":                       "4200", // Trading Sales Revenue
		}

		for key, accCode := range mappings {
			parts := strings.SplitN(key, ".", 2)
			module := strings.ToUpper(parts[0])
			accID := sctx.AccountIDs[accCode]
			if accID == 0 {
				return fmt.Errorf("account %q for mapping key %q not found", accCode, key)
			}

			if _, err := tx.Exec(ctx, `
				INSERT INTO account_mappings (module, key, account_id, company_id, created_at, updated_at)
				VALUES ($1, $2, $3, $4, NOW(), NOW())
				ON CONFLICT (module, key) DO UPDATE SET
					account_id = EXCLUDED.account_id,
					company_id = EXCLUDED.company_id,
					updated_at = NOW()`, module, key, accID, sctx.CompanyNTPID); err != nil {
				return fmt.Errorf("upsert account mapping %q -> %q: %w", key, accCode, err)
			}
		}

		// -------------------------------------------------------------------------
		// 4. Bank Accounts (3 Core Corporate Accounts)
		// -------------------------------------------------------------------------
		bankAccounts := []struct {
			name           string
			accountNumber  string
			currency       string
			glAccountCode  string
			initialBalance float64
		}{
			{
				name:           "BCA Operasional IDR",
				accountNumber:  "0088-2233-4411",
				currency:       "IDR",
				glAccountCode:  "1110",
				initialBalance: 1500000000.00,
			},
			{
				name:           "Mandiri Payroll IDR",
				accountNumber:  "1200-0011-22334",
				currency:       "IDR",
				glAccountCode:  "1111",
				initialBalance: 500000000.00,
			},
			{
				name:           "BCA Valas USD",
				accountNumber:  "0088-9988-7700",
				currency:       "USD",
				glAccountCode:  "1112",
				initialBalance: 150000.00,
			},
		}

		for _, ba := range bankAccounts {
			glAccID := sctx.AccountIDs[ba.glAccountCode]
			if glAccID == 0 {
				return fmt.Errorf("gl account %q for bank account %q not found", ba.glAccountCode, ba.name)
			}

			var bankAccID int64
			err := tx.QueryRow(ctx, `SELECT id FROM bank_accounts WHERE company_id = $1 AND account_number = $2`, sctx.CompanyNTPID, ba.accountNumber).Scan(&bankAccID)
			if errors.Is(err, pgx.ErrNoRows) {
				err = tx.QueryRow(ctx, `
					INSERT INTO bank_accounts (company_id, name, account_number, currency, gl_account_id, initial_balance, is_active, created_at, updated_at)
					VALUES ($1, $2, $3, $4, $5, $6, TRUE, NOW(), NOW())
					RETURNING id`, sctx.CompanyNTPID, ba.name, ba.accountNumber, ba.currency, glAccID, ba.initialBalance).Scan(&bankAccID)
			}
			if err != nil {
				return fmt.Errorf("upsert bank account %q: %w", ba.name, err)
			}
			sctx.BankAccountIDs[ba.accountNumber] = bankAccID
		}

		// -------------------------------------------------------------------------
		// 5. Daily FX Rates (184 Days: March 1, 2026 – August 31, 2026)
		// -------------------------------------------------------------------------
		// Smooth trajectory representing realistic 2026 Bank Indonesia JISDOR rate dynamics (15,500 – 16,200 IDR/USD).
		fxStart := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
		fxEnd := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)

		totalDays := int(fxEnd.Sub(fxStart).Hours()/24) + 1
		for i := 0; i < totalDays; i++ {
			rateDate := fxStart.AddDate(0, 0, i)
			dayIndex := float64(i)

			// Upward curve March-July (15,520 -> 16,180) then slight cooling in August (-> 15,920)
			progress := dayIndex / float64(totalDays-1)
			baseRate := 15520.0 + 660.0*math.Sin(progress*math.Pi*0.95)
			// Small deterministically calculated daily fluctuation (+/- 25 IDR)
			dailyNoise := float64((i*17)%50) - 25.0
			rate := math.Round((baseRate+dailyNoise)*100) / 100

			if _, err := tx.Exec(ctx, `
				INSERT INTO fx_daily_rates (base_currency, quote_currency, rate_date, rate, source, source_reference, fetched_at)
				VALUES ('IDR', 'USD', $1, $2, 'BI', 'Bank Indonesia JISDOR', NOW())
				ON CONFLICT (base_currency, quote_currency, rate_date, source) DO UPDATE SET
					rate = EXCLUDED.rate,
					fetched_at = NOW()`, rateDate, rate); err != nil {
				return fmt.Errorf("insert fx_daily_rate for %s: %w", rateDate.Format("2006-01-02"), err)
			}
		}

		// -------------------------------------------------------------------------
		// 6. Tax Compliance & e-Faktur Configuration (Indonesian Tax Standard)
		// -------------------------------------------------------------------------
		// Company Tax Identities
		compTaxIdentities := []struct {
			companyID         int64
			legalName         string
			npwp              string
			nitku             string
			pkpNumber         string
			registeredAddress string
		}{
			{
				companyID:         sctx.CompanyNTPID,
				legalName:         "PT Nusantara Teknik Perkasa",
				npwp:              "01.888.777.6-012.000",
				nitku:             "0188877760120000000000",
				pkpNumber:         "PKP-018887776-012",
				registeredAddress: "Kawasan Industri Pulogadung, Jl. Rawa Gelam IV No. 8, Jakarta Timur 13930",
			},
			{
				companyID:         sctx.CompanyNDMID,
				legalName:         "PT Nusantara Distribusi Mandiri",
				npwp:              "02.999.666.5-013.000",
				nitku:             "0299966650130000000000",
				pkpNumber:         "PKP-029996665-013",
				registeredAddress: "Kawasan Industri Jababeka II, Jl. Industri Selatan Blok JJ No. 12, Cikarang 17530",
			},
		}

		for _, cti := range compTaxIdentities {
			var existingID int64
			err := tx.QueryRow(ctx, `SELECT id FROM company_tax_identities WHERE company_id = $1 LIMIT 1`, cti.companyID).Scan(&existingID)
			if errors.Is(err, pgx.ErrNoRows) {
				_, err = tx.Exec(ctx, `
					INSERT INTO company_tax_identities (company_id, legal_name, npwp, nitku, pkp_number, registered_address, effective_from, created_by, created_at)
					VALUES ($1, $2, $3, $4, $5, $6, '2026-01-01', $7, NOW())`,
					cti.companyID, cti.legalName, cti.npwp, cti.nitku, cti.pkpNumber, cti.registeredAddress, adminID)
			}
			if err != nil {
				return fmt.Errorf("upsert company_tax_identity for company %d: %w", cti.companyID, err)
			}
		}

		// Tax Rule Versions (VAT, Withholding, Tax Codes)
		taxRuleVersions := []struct {
			ruleKind       string
			versionCode    string
			effectiveFrom  string
			sourceURL      string
			sourceChecksum string
		}{
			{"VAT", "VAT-UU-HPP-2022", "2022-04-01", "https://jdih.kemenkeu.go.id/uu-hpp-7-2021", "sha256:4d8a1c9e8b23f1a6c7e4d5b6a7c8e9f0123456789abcdef0123456789abcdef0"},
			{"WITHHOLDING", "WHT-PPH23-2022", "2022-04-01", "https://jdih.kemenkeu.go.id/pmk-141-2015", "sha256:9b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c"},
			{"TAX_CODE", "TC-DJP-2024", "2024-01-01", "https://pajak.go.id/coretax-tax-codes", "sha256:7f1e2d3c4b5a6f7e8d9c0b1a2f3e4d5c6b7a8f9e0d1c2b3a4f5e6d7c8b9a0f1e"},
		}

		reviewedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		for _, trv := range taxRuleVersions {
			var trvID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO tax_rule_versions (rule_kind, version_code, effective_from, source_url, source_checksum, reviewed_by, reviewed_at, created_by, created_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $6, NOW())
				ON CONFLICT (rule_kind, version_code) DO UPDATE SET
					effective_from = EXCLUDED.effective_from,
					source_url = EXCLUDED.source_url,
					source_checksum = EXCLUDED.source_checksum
				RETURNING id`, trv.ruleKind, trv.versionCode, trv.effectiveFrom, trv.sourceURL, trv.sourceChecksum, adminID, reviewedAt).Scan(&trvID)
			if err != nil {
				return fmt.Errorf("upsert tax_rule_version %q: %w", trv.versionCode, err)
			}
			sctx.TaxRuleVersionIDs[trv.versionCode] = trvID
		}

		// Tax VAT Rates (11% Standard Rate)
		vatRuleID := sctx.TaxRuleVersionIDs["VAT-UU-HPP-2022"]
		var vatRateID int64
		err := tx.QueryRow(ctx, `
			INSERT INTO tax_vat_rates (rule_version_id, code, name, rate_bps, dpp_numerator, dpp_denominator, luxury_only)
			VALUES ($1, 'PPN11', 'PPN Standar 11%', 1100, 1, 1, FALSE)
			ON CONFLICT (rule_version_id, code) DO UPDATE SET
				name = EXCLUDED.name,
				rate_bps = EXCLUDED.rate_bps
			RETURNING id`, vatRuleID).Scan(&vatRateID)
		if err != nil {
			return fmt.Errorf("upsert tax_vat_rate PPN11: %w", err)
		}
		sctx.TaxVatRateIDs["PPN11"] = vatRateID

		// Tax Withholding Types (PPh 23 Jasa 2%)
		whtRuleID := sctx.TaxRuleVersionIDs["WHT-PPH23-2022"]
		var whtTypeID int64
		err = tx.QueryRow(ctx, `
			INSERT INTO tax_withholding_types (rule_version_id, code, article, name, recognition_event, rate_bps, tax_base)
			VALUES ($1, 'PPH23-SRV', 'PPh23', 'PPh 23 Jasa Teknik & Manajemen', 'INVOICE', 200, 'GROSS')
			ON CONFLICT (rule_version_id, code) DO UPDATE SET
				name = EXCLUDED.name,
				rate_bps = EXCLUDED.rate_bps
			RETURNING id`, whtRuleID).Scan(&whtTypeID)
		if err != nil {
			return fmt.Errorf("upsert tax_withholding_type PPH23-SRV: %w", err)
		}
		sctx.TaxWithholdingTypeIDs["PPH23-SRV"] = whtTypeID

		// Tax Codes (DJP Standard e-Faktur & Withholding Codes)
		tcRuleID := sctx.TaxRuleVersionIDs["TC-DJP-2024"]
		taxCodes := []struct {
			code               string
			name               string
			taxKind            string
			officialObjectCode string
			vatRateID          *int64
			whtTypeID          *int64
		}{
			{"PPN-OUT-11", "PPN Keluaran 11%", "VAT_OUTPUT", "010", &vatRateID, nil},
			{"PPN-IN-11", "PPN Masukan 11%", "VAT_INPUT", "B2", &vatRateID, nil},
			{"WHT-23-SRV", "PPh 23 Pemotongan Jasa 2%", "PPh23", "24-104-01", nil, &whtTypeID},
		}

		for _, tc := range taxCodes {
			var tcID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO tax_codes (rule_version_id, code, name, tax_kind, official_object_code, vat_rate_id, withholding_type_id)
				VALUES ($1, $2, $3, $4, $5, $6, $7)
				ON CONFLICT (rule_version_id, code) DO UPDATE SET
					name = EXCLUDED.name,
					tax_kind = EXCLUDED.tax_kind,
					official_object_code = EXCLUDED.official_object_code,
					vat_rate_id = EXCLUDED.vat_rate_id,
					withholding_type_id = EXCLUDED.withholding_type_id
				RETURNING id`, tcRuleID, tc.code, tc.name, tc.taxKind, tc.officialObjectCode, tc.vatRateID, tc.whtTypeID).Scan(&tcID)
			if err != nil {
				return fmt.Errorf("upsert tax_code %q: %w", tc.code, err)
			}
			sctx.TaxCodeIDs[tc.code] = tcID
		}

		// DJP e-Faktur NSFP Number Ranges (Nomor Seri Faktur Pajak)
		for _, compID := range []int64{sctx.CompanyNTPID, sctx.CompanyNDMID} {
			if _, err := tx.Exec(ctx, `
				INSERT INTO tax_invoice_number_ranges (company_id, prefix, range_start, range_end, next_number, effective_from, created_by, created_at)
				VALUES ($1, '010', 2600000001, 2600010000, 2600000001, '2026-01-01', $2, NOW())
				ON CONFLICT (company_id, prefix, range_start, range_end) DO NOTHING`, compID, adminID); err != nil {
				return fmt.Errorf("upsert tax_invoice_number_ranges for company %d: %w", compID, err)
			}
		}

		// Tax Periods (March–August 2026)
		for month := 3; month <= 8; month++ {
			code := fmt.Sprintf("2026-%02d", month)
			apID := sctx.AccountingPeriodIDs[code]
			if apID > 0 {
				var tpID int64
				err := tx.QueryRow(ctx, `
					INSERT INTO tax_periods (company_id, accounting_period_id, status, created_at)
					VALUES ($1, $2, 'OPEN', NOW())
					ON CONFLICT (company_id, accounting_period_id) DO NOTHING
					RETURNING id`, sctx.CompanyNTPID, apID).Scan(&tpID)
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return fmt.Errorf("upsert tax_period for %s: %w", code, err)
				}
				if tpID > 0 {
					sctx.TaxPeriodIDs[code] = tpID
				}
			}
		}

		// -------------------------------------------------------------------------
		// 7. Consolidation Group, Accounts & Elimination Rules
		// -------------------------------------------------------------------------
		var groupID int64
		err = tx.QueryRow(ctx, `
			INSERT INTO consol_groups (name, reporting_currency, fx_enabled, created_at, updated_at)
			VALUES ('Odyssey Group', 'IDR', TRUE, NOW(), NOW())
			ON CONFLICT (name) DO UPDATE SET
				reporting_currency = EXCLUDED.reporting_currency,
				fx_enabled = EXCLUDED.fx_enabled,
				updated_at = NOW()
			RETURNING id`).Scan(&groupID)
		if err != nil {
			return fmt.Errorf("upsert consol_group: %w", err)
		}

		// Group Members (Parent NTP and Subsidiary NDM)
		for _, compID := range []int64{sctx.CompanyNTPID, sctx.CompanyNDMID} {
			if _, err := tx.Exec(ctx, `
				INSERT INTO consol_members (group_id, company_id, enabled, created_at, updated_at)
				VALUES ($1, $2, TRUE, NOW(), NOW())
				ON CONFLICT (group_id, company_id) DO UPDATE SET
					enabled = TRUE,
					updated_at = NOW()`, groupID, compID); err != nil {
				return fmt.Errorf("upsert consol_member for company %d: %w", compID, err)
			}
		}

		// Consol Group Accounts
		groupAccounts := []struct {
			code    string
			name    string
			accType string
		}{
			{"1000", "Consolidated Assets", "ASSET"},
			{"1100", "Consolidated Cash and Cash Equivalents", "ASSET"},
			{"1200", "Consolidated Trade Receivables", "ASSET"},
			{"1300", "Consolidated Inventories", "ASSET"},
			{"2000", "Consolidated Liabilities", "LIABILITY"},
			{"2100", "Consolidated Trade Payables", "LIABILITY"},
			{"3000", "Consolidated Equity", "EQUITY"},
			{"4000", "Consolidated Revenues", "REVENUE"},
			{"5000", "Consolidated Operating Expenses", "EXPENSE"},
		}

		groupAccMap := make(map[string]int64)
		for _, ga := range groupAccounts {
			var gaID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO consol_group_accounts (group_id, code, name, type, created_at, updated_at)
				VALUES ($1, $2, $3, $4::account_type, NOW(), NOW())
				ON CONFLICT (group_id, code) DO UPDATE SET
					name = EXCLUDED.name,
					type = EXCLUDED.type,
					updated_at = NOW()
				RETURNING id`, groupID, ga.code, ga.name, ga.accType).Scan(&gaID)
			if err != nil {
				return fmt.Errorf("upsert consol_group_account %q: %w", ga.code, err)
			}
			groupAccMap[ga.code] = gaID
		}

		// Account Mapping to Consolidation Group Accounts
		for localCode, localID := range sctx.AccountIDs {
			prefix := localCode[:1] + "000"
			if groupAccID, ok := groupAccMap[prefix]; ok {
				for _, compID := range []int64{sctx.CompanyNTPID, sctx.CompanyNDMID} {
					if _, err := tx.Exec(ctx, `
						INSERT INTO account_map (group_id, company_id, local_account_id, group_account_id, created_at, updated_at)
						VALUES ($1, $2, $3, $4, NOW(), NOW())
						ON CONFLICT (group_id, company_id, local_account_id) DO UPDATE SET
							group_account_id = EXCLUDED.group_account_id,
							updated_at = NOW()`, groupID, compID, localID, groupAccID); err != nil {
						return fmt.Errorf("upsert account_map for local account %q (company %d): %w", localCode, compID, err)
					}
				}
			}
		}

		// Consolidation Month-End FX Rates (USD/IDR)
		monthEndFX := []struct {
			asOfDate    string
			pair        string
			averageRate float64
			closingRate float64
		}{
			{"2026-03-31", "USD/IDR", 15580.000000, 15640.000000},
			{"2026-04-30", "USD/IDR", 15710.000000, 15780.000000},
			{"2026-05-31", "USD/IDR", 15850.000000, 15910.000000},
			{"2026-06-30", "USD/IDR", 16000.000000, 16080.000000},
			{"2026-07-31", "USD/IDR", 16140.000000, 16190.000000},
			{"2026-08-31", "USD/IDR", 16020.000000, 15950.000000},
		}

		for _, fx := range monthEndFX {
			if _, err := tx.Exec(ctx, `
				INSERT INTO fx_rates (as_of_date, pair, average_rate, closing_rate, created_at)
				VALUES ($1, $2, $3, $4, NOW())
				ON CONFLICT (as_of_date, pair) DO UPDATE SET
					average_rate = EXCLUDED.average_rate,
					closing_rate = EXCLUDED.closing_rate`, fx.asOfDate, fx.pair, fx.averageRate, fx.closingRate); err != nil {
				return fmt.Errorf("upsert consolidation fx_rate for %s: %w", fx.asOfDate, err)
			}
		}

		return nil
	})
}
