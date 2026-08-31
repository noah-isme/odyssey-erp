package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// seedPhase04Procurement seeds purchase requisitions, supplier contracts, purchase orders,
// goods receipts (GRNs with lots), AP invoices, AP payments, goods returns, and AP debit notes.
func seedPhase04Procurement(ctx context.Context, sctx *SeedContext) error {
	return ExecInTx(ctx, sctx.Pool, "Phase 04: Procurement (P2P)", func(tx pgx.Tx) error {
		adminID := sctx.UserIDs["budi.santoso@nusantarateknik.co.id"]
		procManagerID := sctx.UserIDs["agus.setiawan@nusantarateknik.co.id"]
		if procManagerID == 0 {
			procManagerID = adminID
		}
		accountantID := sctx.UserIDs["siti.aminah@nusantarateknik.co.id"]
		if accountantID == 0 {
			accountantID = adminID
		}

		rawWHID := sctx.WarehouseIDs["WH-JKT-RAW"]
		ppnTaxID := sctx.TaxIDs["PPN"]
		noTaxID := sctx.TaxIDs["NO-TAX"]

		// -------------------------------------------------------------------------
		// 1. Purchase Requisitions (3 PRs: CLOSED, SUBMITTED, DRAFT)
		// -------------------------------------------------------------------------
		type prLineDef struct {
			productSKU string
			qty        float64
			note       string
		}

		type prDef struct {
			number       string
			supplierCode string
			status       string
			note         string
			createdAt    string
			lines        []prLineDef
		}

		prsData := []prDef{
			{
				number:       "PR-202603-0001",
				supplierCode: "SUP-JAYA-PCB",
				status:       "CLOSED",
				note:         "Q1 Production run PCB mainboards and enclosures for IoT Gateway",
				createdAt:    "2026-03-02 08:30:00",
				lines: []prLineDef{
					{"RM-PCB-GW01", 1000, "Mainboard PCB 4-Layer for Gateway Pro"},
					{"RM-ENC-IP67", 500, "Aluminum Enclosure IP67"},
				},
			},
			{
				number:       "PR-202605-0002",
				supplierCode: "SUP-QCT-HK",
				status:       "SUBMITTED",
				note:         "Procurement of cellular 4G modems and gas sensors for upcoming utilities tender",
				createdAt:    "2026-05-04 09:15:00",
				lines: []prLineDef{
					{"CMP-MDM-4G", 600, "Quectel 4G LTE Cat-1 modules SMD"},
					{"CMP-SEN-BME680", 500, "Bosch BME680 Environmental sensors"},
				},
			},
			{
				number:       "PR-202608-0003",
				supplierCode: "SUP-BAT-NUS",
				status:       "DRAFT",
				note:         "Buffer stock replenishment for Li-Ion battery packs and packaging boxes",
				createdAt:    "2026-08-08 14:00:00",
				lines: []prLineDef{
					{"RM-BAT-LIION", 800, "Li-Ion 18650 Battery Packs 5200mAh"},
					{"RM-PKG-BOX01", 2000, "Polyfoam & Kraft packaging boxes"},
				},
			},
		}

		for _, p := range prsData {
			var supID *int64
			if id, ok := sctx.SupplierIDs[p.supplierCode]; ok {
				supID = &id
			}
			createdTime, _ := time.Parse("2006-01-02 15:04:05", p.createdAt)

			var prID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO prs (number, supplier_id, request_by, status, note, company_id, created_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7)
				ON CONFLICT (number) DO UPDATE SET
					supplier_id = EXCLUDED.supplier_id,
					status = EXCLUDED.status,
					note = EXCLUDED.note
				RETURNING id`,
				p.number, supID, procManagerID, p.status, p.note, sctx.CompanyNTPID, createdTime).Scan(&prID)
			if err != nil {
				return fmt.Errorf("upsert PR %q: %w", p.number, err)
			}

			for _, l := range p.lines {
				prodID := sctx.ProductIDs[l.productSKU]
				_, err = tx.Exec(ctx, `
					INSERT INTO pr_lines (pr_id, product_id, qty, note)
					SELECT $1, $2, $3, $4
					WHERE NOT EXISTS (
						SELECT 1 FROM pr_lines WHERE pr_id = $1 AND product_id = $2
					)`,
					prID, prodID, l.qty, l.note)
				if err != nil {
					return fmt.Errorf("insert PR line for %q: %w", l.productSKU, err)
				}
			}
		}

		// -------------------------------------------------------------------------
		// 2. Active Supplier Contract with Volume Price Lines
		// -------------------------------------------------------------------------
		jayaSupID := sctx.SupplierIDs["SUP-JAYA-PCB"]
		contractEffFrom := ParseDate("2026-01-01")
		contractEffTo := ParseDate("2026-12-31")

		var contractID int64
		err := tx.QueryRow(ctx, `
			INSERT INTO supplier_contracts (
				company_id, supplier_id, version, status, currency,
				effective_from, effective_to, payment_terms, incoterms, renewal_notice_days,
				created_by, approved_by, approved_at, note, created_at, updated_at
			) VALUES (
				$1, $2, 1, 'ACTIVE', 'IDR',
				$3, $4, 'Net 30 Days', 'DDP Jakarta Factory', 30,
				$5, $6, '2026-01-02 10:00:00+07', 'Master Volume Agreement 2026 for PCB Mainboards',
				'2026-01-02 08:00:00+07', '2026-01-02 10:00:00+07'
			)
			ON CONFLICT (company_id, supplier_id, version) DO UPDATE SET
				status = EXCLUDED.status,
				effective_from = EXCLUDED.effective_from,
				effective_to = EXCLUDED.effective_to,
				updated_at = NOW()
			RETURNING id`,
			sctx.CompanyNTPID, jayaSupID, contractEffFrom, contractEffTo, procManagerID, adminID).Scan(&contractID)
		if err != nil {
			return fmt.Errorf("upsert supplier contract: %w", err)
		}

		pcbProdID := sctx.ProductIDs["RM-PCB-GW01"]
		priceTiers := []struct {
			minQty   float64
			price    float64
			taxRate  float64
			leadDays int
			moq      float64
		}{
			{0, 24000, 11.00, 14, 500},
			{2000, 22000, 11.00, 14, 2000},
			{5000, 20000, 11.00, 21, 5000},
		}

		for _, tier := range priceTiers {
			_, err = tx.Exec(ctx, `
				INSERT INTO contract_price_lines (contract_id, product_id, min_quantity, unit_price, tax_rate, lead_time_days, moq)
				VALUES ($1, $2, $3, $4, $5, $6, $7)
				ON CONFLICT (contract_id, product_id, min_quantity) DO UPDATE SET
					unit_price = EXCLUDED.unit_price,
					tax_rate = EXCLUDED.tax_rate,
					lead_time_days = EXCLUDED.lead_time_days,
					moq = EXCLUDED.moq`,
				contractID, pcbProdID, tier.minQty, tier.price, tier.taxRate, tier.leadDays, tier.moq)
			if err != nil {
				return fmt.Errorf("upsert contract price line: %w", err)
			}
		}

		// Record price history
		_, _ = tx.Exec(ctx, `
			INSERT INTO price_history (
				company_id, supplier_id, product_id, source_type, source_id, currency,
				unit_price, quantity, tax_rate, moq, lead_time_days, observation_date, note
			) VALUES (
				$1, $2, $3, 'CONTRACT', $4, 'IDR',
				24000, 500, 11.00, 500, 14, '2026-01-02', 'Contract base tier price'
			) ON CONFLICT DO NOTHING`,
			sctx.CompanyNTPID, jayaSupID, pcbProdID, contractID)

		// -------------------------------------------------------------------------
		// 3. Purchase Orders (10 POs: IDR & USD, across full lifecycle)
		// -------------------------------------------------------------------------
		type poLineDef struct {
			productSKU string
			qty        float64
			price      float64
			taxCode    string
			note       string
		}

		type poDef struct {
			number       string
			supplierCode string
			status       string
			currency     string
			expectedDate string
			note         string
			createdAt    string
			approved     bool
			lines        []poLineDef
		}

		posData := []poDef{
			{
				number:       "PO-202603-0001",
				supplierCode: "SUP-JAYA-PCB",
				status:       "CLOSED",
				currency:     "IDR",
				expectedDate: "2026-03-20",
				note:         "Monthly PCB Mainboard batch for IoT Gateway",
				createdAt:    "2026-03-05 09:00:00",
				approved:     true,
				lines: []poLineDef{
					{"RM-PCB-GW01", 1000, 24000, "PPN", "Mainboard PCB 4-Layer FR4 Gold"},
				},
			},
			{
				number:       "PO-202603-0002",
				supplierCode: "SUP-ALU-IND",
				status:       "CLOSED",
				currency:     "IDR",
				expectedDate: "2026-03-25",
				note:         "Die-cast aluminum enclosures for outdoor gateways",
				createdAt:    "2026-03-10 10:30:00",
				approved:     true,
				lines: []poLineDef{
					{"RM-ENC-IP67", 500, 68000, "PPN", "Die-Cast Aluminum Enclosure IP67"},
				},
			},
			{
				number:       "PO-202604-0003",
				supplierCode: "SUP-QCT-HK",
				status:       "CLOSED",
				currency:     "USD",
				expectedDate: "2026-04-20",
				note:         "Quectel 4G LTE Cat-1 cellular module direct factory import",
				createdAt:    "2026-04-02 11:00:00",
				approved:     true,
				lines: []poLineDef{
					{"CMP-MDM-4G", 500, 9.50, "NO-TAX", "Quectel 4G LTE Cat-1 Cellular Module (USD 9.50/unit)"},
				},
			},
			{
				number:       "PO-202604-0004",
				supplierCode: "SUP-ADV-TW",
				status:       "CLOSED",
				currency:     "USD",
				expectedDate: "2026-04-28",
				note:         "Advantech Industrial Edge Server IPC import for substation project",
				createdAt:    "2026-04-15 14:00:00",
				approved:     true,
				lines: []poLineDef{
					{"TRD-SVR-EDG01", 15, 1150.00, "NO-TAX", "Advantech Industrial Edge Server IPC (USD 1,150.00/unit)"},
				},
			},
			{
				number:       "PO-202605-0005",
				supplierCode: "SUP-BAT-NUS",
				status:       "APPROVED",
				currency:     "IDR",
				expectedDate: "2026-05-28",
				note:         "Li-Ion battery pack 18650 for smart environmental nodes",
				createdAt:    "2026-05-12 09:30:00",
				approved:     true,
				lines: []poLineDef{
					{"RM-BAT-LIION", 600, 58000, "PPN", "Li-Ion Battery Pack 18650 3.7V 5200mAh"},
				},
			},
			{
				number:       "PO-202605-0006",
				supplierCode: "SUP-KAB-MET",
				status:       "APPROVED",
				currency:     "IDR",
				expectedDate: "2026-06-08",
				note:         "Waterproof connectors and outdoor LoRa antennas",
				createdAt:    "2026-05-20 13:45:00",
				approved:     true,
				lines: []poLineDef{
					{"RM-CON-M12", 1000, 18000, "PPN", "M12 Waterproof 5-Pin Connector Set"},
					{"RM-ANT-LORA", 400, 75000, "PPN", "Outdoor 915MHz 5.8dBi Fiberglass Antenna"},
				},
			},
			{
				number:       "PO-202606-0007",
				supplierCode: "SUP-PKG-IND",
				status:       "APPROVED",
				currency:     "IDR",
				expectedDate: "2026-06-25",
				note:         "Polyfoam packaging kits for production packing line",
				createdAt:    "2026-06-08 10:00:00",
				approved:     true,
				lines: []poLineDef{
					{"RM-PKG-BOX01", 2000, 8500, "PPN", "Polyfoam & Master Kraft Box Packaging"},
				},
			},
			{
				number:       "PO-202607-0008",
				supplierCode: "SUP-MOX-ID",
				status:       "APPROVAL",
				currency:     "IDR",
				expectedDate: "2026-07-30",
				note:         "Moxa 8-port industrial ethernet switches for system integration",
				createdAt:    "2026-07-15 15:20:00",
				approved:     false,
				lines: []poLineDef{
					{"TRD-SW-IND08", 20, 6200000, "PPN", "Moxa 8-Port Industrial Ethernet Switch"},
				},
			},
			{
				number:       "PO-202608-0009",
				supplierCode: "SUP-JAYA-PCB",
				status:       "DRAFT",
				currency:     "IDR",
				expectedDate: "2026-08-30",
				note:         "Q3 PCB Ramp-up order under contract tier 2 pricing",
				createdAt:    "2026-08-10 11:00:00",
				approved:     false,
				lines: []poLineDef{
					{"RM-PCB-GW01", 2500, 22000, "PPN", "Mainboard PCB 4-Layer FR4 Gold (Volume Tier 2)"},
				},
			},
			{
				number:       "PO-202608-0010",
				supplierCode: "SUP-ALU-IND",
				status:       "CANCELLED",
				currency:     "IDR",
				expectedDate: "2026-08-20",
				note:         "Cancelled due to revised enclosure CNC tooling specifications",
				createdAt:    "2026-08-05 09:30:00",
				approved:     false,
				lines: []poLineDef{
					{"RM-ENC-IP67", 200, 68000, "PPN", "Die-Cast Aluminum Enclosure IP67"},
				},
			},
		}

		poIDs := make(map[string]int64)
		poLineIDs := make(map[string]int64) // "PO_NUMBER:SKU" -> po_line_id

		for _, p := range posData {
			supID := sctx.SupplierIDs[p.supplierCode]
			expDate := ParseDate(p.expectedDate)
			createdTime, _ := time.Parse("2006-01-02 15:04:05", p.createdAt)

			var approvedBy *int64
			var approvedAt *time.Time
			if p.approved {
				approvedBy = &adminID
				appTime := createdTime.Add(2 * time.Hour)
				approvedAt = &appTime
			}

			var poID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO pos (
					number, supplier_id, company_id, status, currency,
					expected_date, note, created_at, approved_by, approved_at, is_service
				) VALUES (
					$1, $2, $3, $4, $5,
					$6, $7, $8, $9, $10, FALSE
				)
				ON CONFLICT (number) DO UPDATE SET
					supplier_id = EXCLUDED.supplier_id,
					company_id = EXCLUDED.company_id,
					status = EXCLUDED.status,
					currency = EXCLUDED.currency,
					expected_date = EXCLUDED.expected_date,
					note = EXCLUDED.note,
					approved_by = EXCLUDED.approved_by,
					approved_at = EXCLUDED.approved_at
				RETURNING id`,
				p.number, supID, sctx.CompanyNTPID, p.status, p.currency,
				expDate, p.note, createdTime, approvedBy, approvedAt).Scan(&poID)
			if err != nil {
				return fmt.Errorf("upsert PO %q: %w", p.number, err)
			}
			poIDs[p.number] = poID

			for _, l := range p.lines {
				prodID := sctx.ProductIDs[l.productSKU]
				taxID := ppnTaxID
				if l.taxCode == "NO-TAX" {
					taxID = noTaxID
				}

				var poLineID int64
				err := tx.QueryRow(ctx, `
					SELECT id FROM po_lines WHERE po_id = $1 AND product_id = $2`,
					poID, prodID).Scan(&poLineID)
				if err != nil {
					err = tx.QueryRow(ctx, `
						INSERT INTO po_lines (po_id, product_id, qty, price, tax_id, note)
						VALUES ($1, $2, $3, $4, $5, $6)
						RETURNING id`,
						poID, prodID, l.qty, l.price, taxID, l.note).Scan(&poLineID)
				} else {
					_, err = tx.Exec(ctx, `
						UPDATE po_lines SET qty = $2, price = $3, tax_id = $4, note = $5
						WHERE id = $1`,
						poLineID, l.qty, l.price, taxID, l.note)
				}
				if err != nil {
					return fmt.Errorf("upsert PO line for %q: %w", l.productSKU, err)
				}
				poLineIDs[fmt.Sprintf("%s:%s", p.number, l.productSKU)] = poLineID
			}
		}

		// -------------------------------------------------------------------------
		// 4. Goods Receipts (6 GRNs with status POSTED and Lot Tracking)
		// -------------------------------------------------------------------------
		type grnLineDef struct {
			productSKU string
			qty        float64
			unitCost   float64
			lotNumber  string
			expiryDate string
		}

		type grnDef struct {
			number       string
			poNumber     string
			supplierCode string
			status       string
			receivedAt   string
			note         string
			lines        []grnLineDef
		}

		grnsData := []grnDef{
			{
				number:       "GRN-202603-0001",
				poNumber:     "PO-202603-0001",
				supplierCode: "SUP-JAYA-PCB",
				status:       "POSTED",
				receivedAt:   "2026-03-18 10:00:00",
				note:         "Incoming inspection passed. Gold plating thickness verified.",
				lines: []grnLineDef{
					{"RM-PCB-GW01", 1000, 24000, "LOT-PCB-2603-01", "2028-03-18"},
				},
			},
			{
				number:       "GRN-202603-0002",
				poNumber:     "PO-202603-0002",
				supplierCode: "SUP-ALU-IND",
				status:       "POSTED",
				receivedAt:   "2026-03-24 11:30:00",
				note:         "500 units aluminum enclosure received and dimension-checked.",
				lines: []grnLineDef{
					{"RM-ENC-IP67", 500, 68000, "", ""},
				},
			},
			{
				number:       "GRN-202604-0003",
				poNumber:     "PO-202604-0003",
				supplierCode: "SUP-QCT-HK",
				status:       "POSTED",
				receivedAt:   "2026-04-20 14:00:00",
				note:         "Quectel 4G LTE modules received from Hong Kong freight forwarder.",
				lines: []grnLineDef{
					{"CMP-MDM-4G", 500, 152000, "LOT-QCT-2604-01", "2029-04-20"},
				},
			},
			{
				number:       "GRN-202604-0004",
				poNumber:     "PO-202604-0004",
				supplierCode: "SUP-ADV-TW",
				status:       "POSTED",
				receivedAt:   "2026-04-28 15:30:00",
				note:         "Advantech Edge Server IPCs received in factory sealed crates.",
				lines: []grnLineDef{
					{"TRD-SVR-EDG01", 15, 18400000, "", ""},
				},
			},
			{
				number:       "GRN-202605-0005",
				poNumber:     "PO-202605-0005",
				supplierCode: "SUP-BAT-NUS",
				status:       "POSTED",
				receivedAt:   "2026-05-25 09:45:00",
				note:         "Li-Ion battery packs UN38.3 test certificate attached.",
				lines: []grnLineDef{
					{"RM-BAT-LIION", 600, 58000, "LOT-BAT-2605-01", "2028-05-25"},
				},
			},
			{
				number:       "GRN-202606-0006",
				poNumber:     "PO-202605-0006",
				supplierCode: "SUP-KAB-MET",
				status:       "POSTED",
				receivedAt:   "2026-06-05 13:15:00",
				note:         "M12 connectors & LoRa antennas batch received and stored in WH-RAW.",
				lines: []grnLineDef{
					{"RM-CON-M12", 1000, 18000, "", ""},
					{"RM-ANT-LORA", 400, 75000, "", ""},
				},
			},
		}

		grnIDs := make(map[string]int64)
		grnLineIDs := make(map[string]int64) // "GRN_NUMBER:SKU" -> grn_line_id

		for _, g := range grnsData {
			poID := poIDs[g.poNumber]
			supID := sctx.SupplierIDs[g.supplierCode]
			recTime, _ := time.Parse("2006-01-02 15:04:05", g.receivedAt)

			var grnID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO grns (
					number, po_id, supplier_id, warehouse_id, company_id,
					status, received_at, note, created_at
				) VALUES (
					$1, $2, $3, $4, $5,
					$6, $7, $8, $7
				)
				ON CONFLICT (number) DO UPDATE SET
					po_id = EXCLUDED.po_id,
					supplier_id = EXCLUDED.supplier_id,
					warehouse_id = EXCLUDED.warehouse_id,
					company_id = EXCLUDED.company_id,
					status = EXCLUDED.status,
					received_at = EXCLUDED.received_at,
					note = EXCLUDED.note
				RETURNING id`,
				g.number, poID, supID, rawWHID, sctx.CompanyNTPID,
				g.status, recTime, g.note).Scan(&grnID)
			if err != nil {
				return fmt.Errorf("upsert GRN %q: %w", g.number, err)
			}
			grnIDs[g.number] = grnID

			for _, l := range g.lines {
				prodID := sctx.ProductIDs[l.productSKU]
				var expDate *time.Time
				if l.expiryDate != "" {
					ed := ParseDate(l.expiryDate)
					expDate = &ed
				}

				var grnLineID int64
				err := tx.QueryRow(ctx, `
					SELECT id FROM grn_lines WHERE grn_id = $1 AND product_id = $2`,
					grnID, prodID).Scan(&grnLineID)
				if err != nil {
					err = tx.QueryRow(ctx, `
						INSERT INTO grn_lines (grn_id, product_id, qty, unit_cost, lot_number, expiry_date)
						VALUES ($1, $2, $3, $4, $5, $6)
						RETURNING id`,
						grnID, prodID, l.qty, l.unitCost, l.lotNumber, expDate).Scan(&grnLineID)
				} else {
					_, err = tx.Exec(ctx, `
						UPDATE grn_lines SET qty = $2, unit_cost = $3, lot_number = $4, expiry_date = $5
						WHERE id = $1`,
						grnLineID, l.qty, l.unitCost, l.lotNumber, expDate)
				}
				if err != nil {
					return fmt.Errorf("upsert GRN line for %q: %w", l.productSKU, err)
				}
				grnLineIDs[fmt.Sprintf("%s:%s", g.number, l.productSKU)] = grnLineID
			}
		}

		// -------------------------------------------------------------------------
		// 5. AP Invoices (8 Invoices across DRAFT, POSTED, PAID, VOID — some overdue)
		// -------------------------------------------------------------------------
		type apInvLineDef struct {
			grnKey      string // "GRN_NUMBER:SKU"
			poKey       string // "PO_NUMBER:SKU"
			productSKU  string
			description string
			qty         float64
			unitPrice   float64
			taxPct      float64
		}

		type apInvDef struct {
			number       string
			supplierCode string
			grnNumber    string
			poNumber     string
			currency     string
			status       string
			issuedAt     string
			dueAt        string
			posted       bool
			voided       bool
			voidReason   string
			lines        []apInvLineDef
		}

		apInvoicesData := []apInvDef{
			{
				number:       "APINV-202603-0001",
				supplierCode: "SUP-JAYA-PCB",
				grnNumber:    "GRN-202603-0001",
				poNumber:     "PO-202603-0001",
				currency:     "IDR",
				status:       "PAID",
				issuedAt:     "2026-03-20",
				dueAt:        "2026-04-19",
				posted:       true,
				lines: []apInvLineDef{
					{"GRN-202603-0001:RM-PCB-GW01", "PO-202603-0001:RM-PCB-GW01", "RM-PCB-GW01", "Mainboard PCB 4-Layer FR4 Gold 1000 PCS", 1000, 24000, 11.00},
				},
			},
			{
				number:       "APINV-202603-0002",
				supplierCode: "SUP-ALU-IND",
				grnNumber:    "GRN-202603-0002",
				poNumber:     "PO-202603-0002",
				currency:     "IDR",
				status:       "PAID",
				issuedAt:     "2026-03-26",
				dueAt:        "2026-04-25",
				posted:       true,
				lines: []apInvLineDef{
					{"GRN-202603-0002:RM-ENC-IP67", "PO-202603-0002:RM-ENC-IP67", "RM-ENC-IP67", "Die-Cast Aluminum Enclosure IP67 500 PCS", 500, 68000, 11.00},
				},
			},
			{
				number:       "APINV-202604-0003",
				supplierCode: "SUP-QCT-HK",
				grnNumber:    "GRN-202604-0003",
				poNumber:     "PO-202604-0003",
				currency:     "USD",
				status:       "PAID",
				issuedAt:     "2026-04-22",
				dueAt:        "2026-05-22",
				posted:       true,
				lines: []apInvLineDef{
					{"GRN-202604-0003:CMP-MDM-4G", "PO-202604-0003:CMP-MDM-4G", "CMP-MDM-4G", "Quectel 4G LTE Cat-1 Cellular Module 500 PCS (USD 9.50)", 500, 9.50, 0.00},
				},
			},
			{
				// Overdue AP Invoice for aging report testing (>60 days overdue)
				number:       "APINV-202605-0004",
				supplierCode: "SUP-ADV-TW",
				grnNumber:    "GRN-202604-0004",
				poNumber:     "PO-202604-0004",
				currency:     "USD",
				status:       "POSTED",
				issuedAt:     "2026-05-02",
				dueAt:        "2026-06-01",
				posted:       true,
				lines: []apInvLineDef{
					{"GRN-202604-0004:TRD-SVR-EDG01", "PO-202604-0004:TRD-SVR-EDG01", "TRD-SVR-EDG01", "Advantech Industrial Edge Server IPC 15 PCS (USD 1,150.00)", 15, 1150.00, 0.00},
				},
			},
			{
				// Overdue AP Invoice (>60 days overdue)
				number:       "APINV-202605-0005",
				supplierCode: "SUP-BAT-NUS",
				grnNumber:    "GRN-202605-0005",
				poNumber:     "PO-202605-0005",
				currency:     "IDR",
				status:       "POSTED",
				issuedAt:     "2026-05-26",
				dueAt:        "2026-06-25",
				posted:       true,
				lines: []apInvLineDef{
					{"GRN-202605-0005:RM-BAT-LIION", "PO-202605-0005:RM-BAT-LIION", "RM-BAT-LIION", "Li-Ion Battery Pack 18650 600 PCS", 600, 58000, 11.00},
				},
			},
			{
				// Current AP Invoice (Not overdue)
				number:       "APINV-202606-0006",
				supplierCode: "SUP-KAB-MET",
				grnNumber:    "GRN-202606-0006",
				poNumber:     "PO-202605-0006",
				currency:     "IDR",
				status:       "POSTED",
				issuedAt:     "2026-06-06",
				dueAt:        "2026-09-05",
				posted:       true,
				lines: []apInvLineDef{
					{"GRN-202606-0006:RM-CON-M12", "PO-202605-0006:RM-CON-M12", "RM-CON-M12", "M12 5-Pin Connectors 1000 SET", 1000, 18000, 11.00},
					{"GRN-202606-0006:RM-ANT-LORA", "PO-202605-0006:RM-ANT-LORA", "RM-ANT-LORA", "Outdoor 915MHz LoRa Antennas 400 PCS", 400, 75000, 11.00},
				},
			},
			{
				// Draft AP Invoice
				number:       "APINV-202608-0007",
				supplierCode: "SUP-PKG-IND",
				poNumber:     "PO-202606-0007",
				currency:     "IDR",
				status:       "DRAFT",
				issuedAt:     "2026-08-15",
				dueAt:        "2026-09-14",
				posted:       false,
				lines: []apInvLineDef{
					{"", "PO-202606-0007:RM-PKG-BOX01", "RM-PKG-BOX01", "Polyfoam Packaging Box Kits 2000 SET", 2000, 8500, 11.00},
				},
			},
			{
				// Voided AP Invoice
				number:       "APINV-202608-0008",
				supplierCode: "SUP-JAYA-PCB",
				currency:     "IDR",
				status:       "VOID",
				issuedAt:     "2026-08-01",
				dueAt:        "2026-08-31",
				posted:       false,
				voided:       true,
				voidReason:   "Duplicate vendor billing entry detected during 3-way matching",
				lines: []apInvLineDef{
					{"", "", "RM-PCB-GW01", "Mainboard PCB 4-Layer FR4 Gold (Duplicate Entry)", 1000, 24000, 11.00},
				},
			},
		}

		apInvoiceIDs := make(map[string]int64)
		apInvoiceLineIDs := make(map[string]int64)

		for _, inv := range apInvoicesData {
			supID := sctx.SupplierIDs[inv.supplierCode]
			var grnID *int64
			if id, ok := grnIDs[inv.grnNumber]; ok {
				grnID = &id
			}
			var poID *int64
			if id, ok := poIDs[inv.poNumber]; ok {
				poID = &id
			}

			issueDate := ParseDate(inv.issuedAt)
			dueDate := ParseDate(inv.dueAt)

			var subtotal, taxAmount, total float64
			for _, l := range inv.lines {
				lineSub := l.qty * l.unitPrice
				lineTax := lineSub * (l.taxPct / 100.0)
				subtotal += lineSub
				taxAmount += lineTax
				total += (lineSub + lineTax)
			}

			var postedAt *time.Time
			var postedBy *int64
			if inv.posted {
				pt := issueDate.Add(24 * time.Hour)
				postedAt = &pt
				postedBy = &accountantID
			}

			var voidedAt *time.Time
			var voidedBy *int64
			var voidReason *string
			if inv.voided {
				vt := issueDate.Add(48 * time.Hour)
				voidedAt = &vt
				voidedBy = &accountantID
				voidReason = &inv.voidReason
			}

			// FX Valuation
			var fxRate = 1.0
			baseAmount := total
			if inv.currency == "USD" {
				fxRate = 16000.0
				baseAmount = total * fxRate
			}

			var apInvID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO ap_invoices (
					number, supplier_id, grn_id, po_id, currency,
					subtotal, tax_amount, total, status, issued_at, due_at,
					company_id, posted_at, posted_by, voided_at, voided_by, void_reason,
					created_by, created_at, updated_at,
					base_currency, original_currency_amount, base_amount, fx_rate, fx_rate_date, fx_rate_source, fx_rate_locked_at
				) VALUES (
					$1, $2, $3, $4, $5,
					$6, $7, $8, $9, $10, $11,
					$12, $13, $14, $15, $16, $17,
					$18, $19, $19,
					'IDR', $8, $20, $21, $10, 'BANK_INDONESIA', $13
				)
				ON CONFLICT (number) DO UPDATE SET
					supplier_id = EXCLUDED.supplier_id,
					grn_id = EXCLUDED.grn_id,
					po_id = EXCLUDED.po_id,
					currency = EXCLUDED.currency,
					subtotal = EXCLUDED.subtotal,
					tax_amount = EXCLUDED.tax_amount,
					total = EXCLUDED.total,
					status = EXCLUDED.status,
					issued_at = EXCLUDED.issued_at,
					due_at = EXCLUDED.due_at,
					posted_at = EXCLUDED.posted_at,
					posted_by = EXCLUDED.posted_by,
					voided_at = EXCLUDED.voided_at,
					voided_by = EXCLUDED.voided_by,
					void_reason = EXCLUDED.void_reason,
					base_amount = EXCLUDED.base_amount,
					fx_rate = EXCLUDED.fx_rate,
					updated_at = NOW()
				RETURNING id`,
				inv.number, supID, grnID, poID, inv.currency,
				subtotal, taxAmount, total, inv.status, issueDate, dueDate,
				sctx.CompanyNTPID, postedAt, postedBy, voidedAt, voidedBy, voidReason,
				accountantID, issueDate,
				baseAmount, fxRate).Scan(&apInvID)
			if err != nil {
				return fmt.Errorf("upsert AP invoice %q: %w", inv.number, err)
			}
			apInvoiceIDs[inv.number] = apInvID

			for _, l := range inv.lines {
				prodID := sctx.ProductIDs[l.productSKU]
				var grnLineID *int64
				if id, ok := grnLineIDs[l.grnKey]; ok {
					grnLineID = &id
				}
				var poLineID *int64
				if id, ok := poLineIDs[l.poKey]; ok {
					poLineID = &id
				}

				lineSub := l.qty * l.unitPrice
				lineTax := lineSub * (l.taxPct / 100.0)
				lineTot := lineSub + lineTax

				var apLineID int64
				err := tx.QueryRow(ctx, `
					SELECT id FROM ap_invoice_lines WHERE ap_invoice_id = $1 AND product_id = $2`,
					apInvID, prodID).Scan(&apLineID)
				if err != nil {
					err = tx.QueryRow(ctx, `
						INSERT INTO ap_invoice_lines (
							ap_invoice_id, grn_line_id, po_line_id, product_id, description,
							quantity, unit_price, discount_pct, tax_pct, subtotal, tax_amount, total
						) VALUES (
							$1, $2, $3, $4, $5,
							$6, $7, 0, $8, $9, $10, $11
						) RETURNING id`,
						apInvID, grnLineID, poLineID, prodID, l.description,
						l.qty, l.unitPrice, l.taxPct, lineSub, lineTax, lineTot).Scan(&apLineID)
				} else {
					_, err = tx.Exec(ctx, `
						UPDATE ap_invoice_lines SET
							grn_line_id = $2, po_line_id = $3, description = $4,
							quantity = $5, unit_price = $6, tax_pct = $7, subtotal = $8, tax_amount = $9, total = $10
						WHERE id = $1`,
						apLineID, grnLineID, poLineID, l.description,
						l.qty, l.unitPrice, l.taxPct, lineSub, lineTax, lineTot)
				}
				if err != nil {
					return fmt.Errorf("upsert AP invoice line: %w", err)
				}
				apInvoiceLineIDs[fmt.Sprintf("%s:%s", inv.number, l.productSKU)] = apLineID
			}
		}

		// -------------------------------------------------------------------------
		// 6. AP Payments & Allocations (5 Payments)
		// -------------------------------------------------------------------------
		type apPayDef struct {
			number       string
			invoiceNum   string
			supplierCode string
			amount       float64
			currency     string
			paidAt       string
			method       string
			note         string
		}

		apPaymentsData := []apPayDef{
			{
				number:       "APPAY-202604-0001",
				invoiceNum:   "APINV-202603-0001",
				supplierCode: "SUP-JAYA-PCB",
				amount:       26640000,
				currency:     "IDR",
				paidAt:       "2026-04-10",
				method:       "TRANSFER",
				note:         "Payment for APINV-202603-0001 via BCA IDR Operating",
			},
			{
				number:       "APPAY-202604-0002",
				invoiceNum:   "APINV-202603-0002",
				supplierCode: "SUP-ALU-IND",
				amount:       37740000,
				currency:     "IDR",
				paidAt:       "2026-04-20",
				method:       "TRANSFER",
				note:         "Payment for APINV-202603-0002 via BCA IDR Operating",
			},
			{
				number:       "APPAY-202605-0003",
				invoiceNum:   "APINV-202604-0003",
				supplierCode: "SUP-QCT-HK",
				amount:       4750.00,
				currency:     "USD",
				paidAt:       "2026-05-15",
				method:       "TRANSFER",
				note:         "Telegraphic Transfer USD via BCA USD Account to Quectel HK",
			},
			{
				// Partial payment for PT Nusantara Power Cell
				number:       "APPAY-202607-0004",
				invoiceNum:   "APINV-202605-0005",
				supplierCode: "SUP-BAT-NUS",
				amount:       15000000,
				currency:     "IDR",
				paidAt:       "2026-07-10",
				method:       "TRANSFER",
				note:         "Partial vendor installment payment",
			},
			{
				// Partial payment for Advantech TW
				number:       "APPAY-202607-0005",
				invoiceNum:   "APINV-202605-0004",
				supplierCode: "SUP-ADV-TW",
				amount:       5000.00,
				currency:     "USD",
				paidAt:       "2026-07-20",
				method:       "TRANSFER",
				note:         "Partial payment via foreign exchange telegraphic transfer",
			},
		}

		for _, p := range apPaymentsData {
			apInvID := apInvoiceIDs[p.invoiceNum]
			supID := sctx.SupplierIDs[p.supplierCode]
			paidDate := ParseDate(p.paidAt)

			var fxRate = 1.0
			baseAmount := p.amount
			if p.currency == "USD" {
				fxRate = 16000.0
				baseAmount = p.amount * fxRate
			}

			var payID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO ap_payments (
					number, ap_invoice_id, supplier_id, amount, paid_at, method, note,
					created_by, created_at, updated_at,
					currency, original_currency_amount, base_currency, base_amount, fx_rate, fx_rate_date, fx_rate_source, fx_rate_locked_at
				) VALUES (
					$1, $2, $3, $4, $5, $6, $7,
					$8, $9, $9,
					$10, $4, 'IDR', $11, $12, $5, 'BANK_INDONESIA', $9
				)
				ON CONFLICT (number) DO UPDATE SET
					ap_invoice_id = EXCLUDED.ap_invoice_id,
					supplier_id = EXCLUDED.supplier_id,
					amount = EXCLUDED.amount,
					paid_at = EXCLUDED.paid_at,
					method = EXCLUDED.method,
					note = EXCLUDED.note,
					base_amount = EXCLUDED.base_amount,
					fx_rate = EXCLUDED.fx_rate,
					updated_at = NOW()
				RETURNING id`,
				p.number, apInvID, supID, p.amount, paidDate, p.method, p.note,
				accountantID, paidDate,
				p.currency, baseAmount, fxRate).Scan(&payID)
			if err != nil {
				return fmt.Errorf("upsert AP payment %q: %w", p.number, err)
			}

			// Payment allocation
			_, err = tx.Exec(ctx, `
				INSERT INTO ap_payment_allocations (
					ap_payment_id, ap_invoice_id, amount, created_at,
					original_currency_amount, base_amount, currency, base_currency, fx_rate, fx_rate_date, fx_rate_source, fx_rate_locked_at
				) VALUES (
					$1, $2, $3, $4,
					$3, $5, $6, 'IDR', $7, $8, 'BANK_INDONESIA', $4
				)
				ON CONFLICT DO NOTHING`,
				payID, apInvID, p.amount, paidDate,
				baseAmount, p.currency, fxRate, paidDate)
			if err != nil {
				return fmt.Errorf("insert AP payment allocation: %w", err)
			}
		}

		// -------------------------------------------------------------------------
		// 7. Goods Return GRN (1 Confirmed Return: 25 Aluminum Enclosures)
		// -------------------------------------------------------------------------
		aluSupID := sctx.SupplierIDs["SUP-ALU-IND"]
		grnAluID := grnIDs["GRN-202603-0002"]
		grnLineAluID := grnLineIDs["GRN-202603-0002:RM-ENC-IP67"]
		encProdID := sctx.ProductIDs["RM-ENC-IP67"]

		retDate := ParseDate("2026-04-05")
		retConfirmedAt := retDate.Add(4 * time.Hour)

		var returnGRNID int64
		err = tx.QueryRow(ctx, `
			INSERT INTO goods_return_grns (
				number, company_id, supplier_id, grn_id, warehouse_id,
				return_date, status, reason, notes,
				created_by, confirmed_by, confirmed_at, created_at, updated_at
			) VALUES (
				'GRN-RET-2604-00001', $1, $2, $3, $4,
				$5::date, 'CONFIRMED', 'Defective surface finish on batch', '25 enclosures returned due to anodizing scratch defects',
				$6, $6, $7::timestamptz, $8::timestamptz, $7::timestamptz
			)
			ON CONFLICT (number) DO UPDATE SET
				status = EXCLUDED.status,
				reason = EXCLUDED.reason,
				notes = EXCLUDED.notes,
				confirmed_at = EXCLUDED.confirmed_at,
				updated_at = NOW()
			RETURNING id`,
			sctx.CompanyNTPID, aluSupID, grnAluID, rawWHID,
			retDate, procManagerID, retConfirmedAt, retConfirmedAt).Scan(&returnGRNID)
		if err != nil {
			return fmt.Errorf("upsert goods return GRN: %w", err)
		}

		var returnLineID int64
		err = tx.QueryRow(ctx, `
			SELECT id FROM goods_return_grn_lines
			WHERE goods_return_grn_id = $1 AND product_id = $2`,
			returnGRNID, encProdID).Scan(&returnLineID)
		if err != nil {
			err = tx.QueryRow(ctx, `
				INSERT INTO goods_return_grn_lines (
					goods_return_grn_id, grn_line_id, product_id,
					quantity_returned, unit_cost, notes, line_order
				) VALUES (
					$1, $2, $3,
					25, 68000, '25 units aluminum enclosure with anodizing blemishes', 1
				) RETURNING id`,
				returnGRNID, grnLineAluID, encProdID).Scan(&returnLineID)
			if err != nil {
				return fmt.Errorf("insert goods return GRN line: %w", err)
			}
		}

		// -------------------------------------------------------------------------
		// 8. AP Debit Note (1 Posted Debit Note linked to Goods Return & AP Invoice)
		// -------------------------------------------------------------------------
		apInvAluID := apInvoiceIDs["APINV-202603-0002"]
		apInvLineAluID := apInvoiceLineIDs["APINV-202603-0002:RM-ENC-IP67"]

		dnDate := ParseDate("2026-04-08")
		dnPostedAt := dnDate.Add(2 * time.Hour)

		var debitNoteID int64
		err = tx.QueryRow(ctx, `
			INSERT INTO ap_debit_notes (
				number, supplier_id, ap_invoice_id, goods_return_grn_id, currency,
				reason, subtotal, tax_amount, total, status,
				posted_at, posted_by, created_by, created_at, updated_at
			) VALUES (
				'DN-2604-00001', $1, $2, $3, 'IDR',
				'Debit note for 25 returned defective enclosures', 1700000, 187000, 1887000, 'POSTED',
				$4, $5, $5, $6, $6
			)
			ON CONFLICT (number) DO UPDATE SET
				subtotal = EXCLUDED.subtotal,
				tax_amount = EXCLUDED.tax_amount,
				total = EXCLUDED.total,
				status = EXCLUDED.status,
				posted_at = EXCLUDED.posted_at,
				updated_at = NOW()
			RETURNING id`,
			aluSupID, apInvAluID, returnGRNID,
			dnPostedAt, accountantID, dnDate).Scan(&debitNoteID)
		if err != nil {
			return fmt.Errorf("upsert AP debit note: %w", err)
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO ap_debit_note_lines (
				ap_debit_note_id, ap_invoice_line_id, goods_return_grn_line_id, product_id,
				description, quantity, unit_price, discount_pct, tax_pct,
				subtotal, tax_amount, total
			)
			SELECT $1, $2, $3, $4,
				'Debit Note for 25 pcs Aluminum Enclosure IP67', 25, 68000, 0, 11.00,
				1700000, 187000, 1887000
			WHERE NOT EXISTS (
				SELECT 1 FROM ap_debit_note_lines WHERE ap_debit_note_id = $1 AND product_id = $4
			)`,
			debitNoteID, apInvLineAluID, returnLineID, encProdID)
		if err != nil {
			return fmt.Errorf("insert AP debit note line: %w", err)
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO ap_debit_note_allocations (ap_debit_note_id, ap_invoice_id, amount, created_at)
			VALUES ($1, $2, 1887000, $3)
			ON CONFLICT (ap_debit_note_id, ap_invoice_id) DO UPDATE SET
				amount = EXCLUDED.amount`,
			debitNoteID, apInvAluID, dnDate)
		if err != nil {
			return fmt.Errorf("insert AP debit note allocation: %w", err)
		}

		return nil
	})
}
