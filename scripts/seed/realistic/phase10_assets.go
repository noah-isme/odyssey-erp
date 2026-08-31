package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// seedPhase10Assets seeds fixed asset categories (with GL account mappings),
// fixed asset inventory (machinery, vehicles, IT infrastructure, office furniture),
// depreciation schedules, and asset disposal records for PT Nusantara Teknik Perkasa.
func seedPhase10Assets(ctx context.Context, sctx *SeedContext) error {
	return ExecInTx(ctx, sctx.Pool, "Phase 10: Fixed Assets", func(tx pgx.Tx) error {
		companyID := sctx.CompanyNTPID
		if companyID == 0 {
			var err error
			companyID, err = LookupCompanyID(ctx, tx, "NTP-HQ")
			if err != nil {
				return err
			}
			sctx.CompanyNTPID = companyID
		}

		// Helper to resolve GL accounts
		getAccountID := func(code string) (int64, error) {
			if id, ok := sctx.AccountIDs[code]; ok && id > 0 {
				return id, nil
			}
			id, err := LookupAccountID(ctx, tx, code)
			if err != nil {
				return 0, fmt.Errorf("lookup GL account %q: %w", code, err)
			}
			sctx.AccountIDs[code] = id
			return id, nil
		}

		// GL Accounts
		accMachID, err := getAccountID("1510") // Mesin & Peralatan Produksi SMT
		if err != nil {
			return err
		}
		accVehID, err := getAccountID("1520") // Kendaraan Operasional & Angkutan
		if err != nil {
			return err
		}
		accITFurnID, err := getAccountID("1530") // Peralatan Kantor & Komputer IT
		if err != nil {
			return err
		}
		accAccumDeprID, err := getAccountID("1590") // Akumulasi Penyusutan Aset Tetap
		if err != nil {
			return err
		}
		accDeprExpID, err := getAccountID("5400") // Beban Penyusutan Aset Tetap
		if err != nil {
			return err
		}
		accGainID, err := getAccountID("4400") // Keuntungan Selisih Kurs / Lain-lain
		if err != nil {
			return err
		}
		accLossID, err := getAccountID("5500") // Kerugian Selisih Kurs / Lain-lain
		if err != nil {
			return err
		}
		accCashID, err := getAccountID("1110") // Kas Operasional BCA IDR
		if err != nil {
			return err
		}

		// -------------------------------------------------------------------------
		// 1. Fixed Asset Categories (4 Core Categories)
		// -------------------------------------------------------------------------
		type catDef struct {
			code       string
			name       string
			assetAccID int64
			accumAccID int64
			expAccID   int64
			usefulMos  int
			residRate  float64
		}

		categories := []catDef{
			{
				code:       "FAC-MACH",
				name:       "Mesin & Peralatan Produksi SMT",
				assetAccID: accMachID,
				accumAccID: accAccumDeprID,
				expAccID:   accDeprExpID,
				usefulMos:  96, // 8 Years
				residRate:  5.0,
			},
			{
				code:       "FAC-VEH",
				name:       "Kendaraan Operasional & Angkutan",
				assetAccID: accVehID,
				accumAccID: accAccumDeprID,
				expAccID:   accDeprExpID,
				usefulMos:  60, // 5 Years
				residRate:  10.0,
			},
			{
				code:       "FAC-IT",
				name:       "Perangkat IT, Server & Alat Uji",
				assetAccID: accITFurnID,
				accumAccID: accAccumDeprID,
				expAccID:   accDeprExpID,
				usefulMos:  36, // 3 Years
				residRate:  0.0,
			},
			{
				code:       "FAC-FURN",
				name:       "Perabot & Perlengkapan Kantor",
				assetAccID: accITFurnID,
				accumAccID: accAccumDeprID,
				expAccID:   accDeprExpID,
				usefulMos:  48, // 4 Years
				residRate:  5.0,
			},
		}

		catIDMap := make(map[string]int64)
		for _, c := range categories {
			var catID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO fixed_asset_categories (
					company_id, code, name, asset_account_id, accumulated_depreciation_account_id,
					depreciation_expense_account_id, useful_life_months, residual_rate, is_active,
					disposal_gain_account_id, disposal_loss_account_id, cash_proceeds_account_id
				) VALUES (
					$1, $2, $3, $4, $5,
					$6, $7, $8, TRUE,
					$9, $10, $11
				)
				ON CONFLICT (company_id, code) DO UPDATE SET
					name = EXCLUDED.name,
					asset_account_id = EXCLUDED.asset_account_id,
					accumulated_depreciation_account_id = EXCLUDED.accumulated_depreciation_account_id,
					depreciation_expense_account_id = EXCLUDED.depreciation_expense_account_id,
					useful_life_months = EXCLUDED.useful_life_months,
					residual_rate = EXCLUDED.residual_rate,
					is_active = TRUE,
					disposal_gain_account_id = EXCLUDED.disposal_gain_account_id,
					disposal_loss_account_id = EXCLUDED.disposal_loss_account_id,
					cash_proceeds_account_id = EXCLUDED.cash_proceeds_account_id
				RETURNING id`,
				companyID, c.code, c.name, c.assetAccID, c.accumAccID,
				c.expAccID, c.usefulMos, c.residRate,
				accGainID, accLossID, accCashID).Scan(&catID)
			if err != nil {
				return fmt.Errorf("upsert fixed_asset_category %q: %w", c.code, err)
			}
			catIDMap[c.code] = catID
		}

		// -------------------------------------------------------------------------
		// 2. Fixed Assets (9 Assets: Machinery, Fleet, IT Equipment, Furniture)
		// -------------------------------------------------------------------------
		type assetDef struct {
			number       string
			name         string
			catCode      string
			acqDate      string
			inService    string
			cost         float64
			residual     float64
			usefulMos    int
			accumDepr    float64
			lastDeprDate *string
			status       string
		}

		deprJuly := "2026-07-31"
		deprJune := "2026-06-30"

		assets := []assetDef{
			{
				number:       "AST-SMT-001",
				name:         "Panasonic High-Speed Modular Mounter NPM-D3A (Line 1)",
				catCode:      "FAC-MACH",
				acqDate:      "2026-03-01",
				inService:    "2026-03-15",
				cost:         1200000000.00,
				residual:     60000000.00,
				usefulMos:    96,
				accumDepr:    59375000.00, // 5 months @ 11,875,000/mo
				lastDeprDate: &deprJuly,
				status:       "ACTIVE",
			},
			{
				number:       "AST-SMT-002",
				name:         "Heller 1809 MK5 8-Zone Forced Air Convection Reflow Oven",
				catCode:      "FAC-MACH",
				acqDate:      "2026-03-05",
				inService:    "2026-03-20",
				cost:         480000000.00,
				residual:     24000000.00,
				usefulMos:    96,
				accumDepr:    23750000.00, // 5 months @ 4,750,000/mo
				lastDeprDate: &deprJuly,
				status:       "ACTIVE",
			},
			{
				number:       "AST-AOI-001",
				name:         "Koh Young Zenith 3D Automated Optical Inspection System",
				catCode:      "FAC-MACH",
				acqDate:      "2026-03-10",
				inService:    "2026-03-25",
				cost:         650000000.00,
				residual:     32500000.00,
				usefulMos:    72,
				accumDepr:    42881944.00, // 5 months @ 8,576,388.89/mo
				lastDeprDate: &deprJuly,
				status:       "ACTIVE",
			},
			{
				number:       "AST-VEH-001",
				name:         "Isuzu Elf NMR 71 HD Box Refrigerator 6-Wheeler Logistics Truck",
				catCode:      "FAC-VEH",
				acqDate:      "2026-03-15",
				inService:    "2026-03-20",
				cost:         385000000.00,
				residual:     38500000.00,
				usefulMos:    60,
				accumDepr:    28875000.00, // 5 months @ 5,775,000/mo
				lastDeprDate: &deprJuly,
				status:       "ACTIVE",
			},
			{
				number:       "AST-VEH-002",
				name:         "Toyota Hilux 2.4 D-Cab 4x4 Field Support & Engineering Vehicle",
				catCode:      "FAC-VEH",
				acqDate:      "2026-04-01",
				inService:    "2026-04-05",
				cost:         440000000.00,
				residual:     44000000.00,
				usefulMos:    60,
				accumDepr:    26400000.00, // 4 months @ 6,600,000/mo
				lastDeprDate: &deprJuly,
				status:       "ACTIVE",
			},
			{
				number:       "AST-VEH-003",
				name:         "Daihatsu Gran Max Blind Van 1.3 Operational Courier",
				catCode:      "FAC-VEH",
				acqDate:      "2026-03-01",
				inService:    "2026-03-05",
				cost:         165000000.00,
				residual:     16500000.00,
				usefulMos:    48,
				accumDepr:    12375000.00, // 4 months @ 3,093,750/mo until disposal in July
				lastDeprDate: &deprJune,
				status:       "DISPOSED",
			},
			{
				number:       "AST-IT-001",
				name:         "Dell PowerEdge R750xs Rack Server Cluster & SAN Storage System",
				catCode:      "FAC-IT",
				acqDate:      "2026-03-01",
				inService:    "2026-03-05",
				cost:         185000000.00,
				residual:     0.00,
				usefulMos:    36,
				accumDepr:    25694444.00, // 5 months @ 5,138,888.89/mo
				lastDeprDate: &deprJuly,
				status:       "ACTIVE",
			},
			{
				number:       "AST-IT-002",
				name:         "Keysight Infiniium 4-Channel 1GHz Digital Storage Oscilloscope",
				catCode:      "FAC-IT",
				acqDate:      "2026-04-10",
				inService:    "2026-04-15",
				cost:         120000000.00,
				residual:     0.00,
				usefulMos:    48,
				accumDepr:    10000000.00, // 4 months @ 2,500,000/mo
				lastDeprDate: &deprJuly,
				status:       "ACTIVE",
			},
			{
				number:       "AST-FRN-001",
				name:         "Executive Boardroom Conference Table & Ergonomic Mesh Chairs",
				catCode:      "FAC-FURN",
				acqDate:      "2026-03-10",
				inService:    "2026-03-15",
				cost:         75000000.00,
				residual:     3750000.00,
				usefulMos:    48,
				accumDepr:    7421875.00, // 5 months @ 1,484,375/mo
				lastDeprDate: &deprJuly,
				status:       "ACTIVE",
			},
		}

		assetIDMap := make(map[string]int64)
		for _, a := range assets {
			catID := catIDMap[a.catCode]
			acqD := ParseDate(a.acqDate)
			inServD := ParseDate(a.inService)

			var lastD *time.Time
			if a.lastDeprDate != nil {
				d := ParseDate(*a.lastDeprDate)
				lastD = &d
			}

			var assetID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO fixed_assets (
					company_id, category_id, number, name, acquisition_date,
					in_service_date, acquisition_cost, residual_value, useful_life_months,
					accumulated_depreciation, last_depreciated_on, status, created_at, updated_at
				) VALUES (
					$1, $2, $3, $4, $5,
					$6, $7, $8, $9,
					$10, $11, $12, NOW(), NOW()
				)
				ON CONFLICT (company_id, number) DO UPDATE SET
					name = EXCLUDED.name,
					category_id = EXCLUDED.category_id,
					acquisition_date = EXCLUDED.acquisition_date,
					in_service_date = EXCLUDED.in_service_date,
					acquisition_cost = EXCLUDED.acquisition_cost,
					residual_value = EXCLUDED.residual_value,
					useful_life_months = EXCLUDED.useful_life_months,
					accumulated_depreciation = EXCLUDED.accumulated_depreciation,
					last_depreciated_on = EXCLUDED.last_depreciated_on,
					status = EXCLUDED.status,
					updated_at = NOW()
				RETURNING id`,
				companyID, catID, a.number, a.name, acqD,
				inServD, a.cost, a.residual, a.usefulMos,
				a.accumDepr, lastD, a.status).Scan(&assetID)
			if err != nil {
				return fmt.Errorf("upsert fixed_asset %q: %w", a.number, err)
			}
			assetIDMap[a.number] = assetID
		}

		// -------------------------------------------------------------------------
		// 3. Fixed Asset Disposal (1 Disposed Operational Vehicle)
		// -------------------------------------------------------------------------
		disposedAssetID := assetIDMap["AST-VEH-003"]
		disposalDate := ParseDate("2026-07-15")
		proceeds := 140000000.00 // Proceeds from selling the van

		var dispExists bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fixed_asset_disposals WHERE asset_id = $1)`, disposedAssetID).Scan(&dispExists)
		if !dispExists {
			if _, err := tx.Exec(ctx, `
				INSERT INTO fixed_asset_disposals (asset_id, disposal_date, proceeds, journal_entry_id, created_at)
				VALUES ($1, $2, $3, NULL, '2026-07-15 14:00:00+07')`,
				disposedAssetID, disposalDate, proceeds); err != nil {
				return fmt.Errorf("insert fixed_asset_disposal for asset %d: %w", disposedAssetID, err)
			}
		}

		return nil
	})
}
