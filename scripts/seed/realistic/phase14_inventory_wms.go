package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// seedPhase14InventoryWMS seeds inventory balances for all 20 catalog products across the 6 warehouses,
// inventory transactions & lines, inventory card history (IN, OUT, TRANSFER, ADJUST),
// 2 stock takes (1 POSTED with variances, 1 DRAFT), 2 inventory adjustments (1 POSTED, 1 DRAFT),
// and 14+ WMS storage bins across zones A-D.
func seedPhase14InventoryWMS(ctx context.Context, sctx *SeedContext) error {
	return ExecInTx(ctx, sctx.Pool, "Phase 14: Inventory & WMS Management", func(tx pgx.Tx) error {
		adminID := sctx.UserIDs["budi.santoso@nusantarateknik.co.id"]
		if adminID == 0 {
			return fmt.Errorf("admin user budi.santoso@nusantarateknik.co.id not found")
		}
		warehouseLeadID := sctx.UserIDs["dewi.lestari@nusantarateknik.co.id"]
		if warehouseLeadID == 0 {
			warehouseLeadID = adminID
		}

		whJktRaw := sctx.WarehouseIDs["WH-JKT-RAW"]
		whJktFG := sctx.WarehouseIDs["WH-JKT-FG"]
		whJktWIP := sctx.WarehouseIDs["WH-JKT-WIP"]
		whSbyDist := sctx.WarehouseIDs["WH-SBY-DIST"]
		whCkrDist := sctx.WarehouseIDs["WH-CKR-DIST"]
		whBdgHub := sctx.WarehouseIDs["WH-BDG-HUB"]

		if whJktRaw == 0 || whJktFG == 0 || whJktWIP == 0 || whSbyDist == 0 || whCkrDist == 0 || whBdgHub == 0 {
			return fmt.Errorf("one or more warehouses not found in SeedContext")
		}

		allWarehouses := []int64{whJktRaw, whJktFG, whJktWIP, whSbyDist, whCkrDist, whBdgHub}

		// -------------------------------------------------------------------------
		// 1. Inventory Balances (All 20 Products x 6 Warehouses >= 0)
		// -------------------------------------------------------------------------
		type stockKey struct {
			whCode  string
			sku     string
			qty     float64
			avgCost float64
		}

		// Curated stock levels representing realistic distribution
		curatedStock := []stockKey{
			// Raw Materials in WH-JKT-RAW
			{"WH-JKT-RAW", "RM-ENC-IP67", 450.00, 68000.00},
			{"WH-JKT-RAW", "RM-PCB-GW01", 850.00, 24000.00},
			{"WH-JKT-RAW", "CMP-MDM-4G", 600.00, 150000.00},
			{"WH-JKT-RAW", "CMP-MCU-ESP32", 1200.00, 62000.00},
			{"WH-JKT-RAW", "CMP-PWR-SMPS", 350.00, 28000.00},
			{"WH-JKT-RAW", "RM-ANT-LORA", 500.00, 75000.00},
			{"WH-JKT-RAW", "RM-CON-M12", 400.00, 18000.00},
			{"WH-JKT-RAW", "RM-BAT-LIION", 350.00, 58000.00},
			{"WH-JKT-RAW", "RM-PKG-BOX01", 1200.00, 8500.00},
			{"WH-JKT-RAW", "CMP-SEN-BME680", 250.00, 95000.00},

			// Raw Materials & Components in WH-CKR-DIST (Cikarang Central Distribution)
			{"WH-CKR-DIST", "RM-ENC-IP67", 300.00, 68000.00},
			{"WH-CKR-DIST", "RM-PCB-GW01", 500.00, 24000.00},
			{"WH-CKR-DIST", "CMP-MDM-4G", 400.00, 150000.00},
			{"WH-CKR-DIST", "CMP-MCU-ESP32", 800.00, 62000.00},
			{"WH-CKR-DIST", "CMP-PWR-SMPS", 250.00, 28000.00},
			{"WH-CKR-DIST", "RM-ANT-LORA", 300.00, 75000.00},

			// Subassemblies & Components in WH-JKT-WIP (Pulogadung Transit & WIP)
			{"WH-JKT-WIP", "RM-PCB-GW01", 100.00, 24000.00},
			{"WH-JKT-WIP", "CMP-MCU-ESP32", 150.00, 62000.00},
			{"WH-JKT-WIP", "CMP-MDM-4G", 120.00, 150000.00},
			{"WH-JKT-WIP", "CMP-SEN-BME680", 80.00, 95000.00},

			// Finished Goods in WH-JKT-FG (Main Sales Warehouse)
			{"WH-JKT-FG", "FG-IOT-GW01", 120.00, 2250000.00},
			{"WH-JKT-FG", "FG-IOT-ENV01", 85.00, 1450000.00},
			{"WH-JKT-FG", "FG-IOT-PWR01", 160.00, 1150000.00},
			{"WH-JKT-FG", "FG-IOT-WTR01", 90.00, 1650000.00},
			{"WH-JKT-FG", "FG-IOT-AGR01", 75.00, 1950000.00},
			{"WH-JKT-FG", "FG-IOT-FLT01", 220.00, 750000.00},
			// Trading Goods in WH-JKT-FG
			{"WH-JKT-FG", "TRD-SVR-EDG01", 15.00, 14500000.00},
			{"WH-JKT-FG", "TRD-SW-IND08", 30.00, 4800000.00},
			{"WH-JKT-FG", "TRD-UPS-IND01", 20.00, 2600000.00},
			{"WH-JKT-FG", "TRD-SEN-RAD01", 10.00, 9500000.00},

			// Finished Goods & Trading in WH-CKR-DIST (Cikarang Central Distribution)
			{"WH-CKR-DIST", "FG-IOT-GW01", 60.00, 2250000.00},
			{"WH-CKR-DIST", "FG-IOT-ENV01", 40.00, 1450000.00},
			{"WH-CKR-DIST", "FG-IOT-PWR01", 75.00, 1150000.00},
			{"WH-CKR-DIST", "FG-IOT-WTR01", 45.00, 1650000.00},
			{"WH-CKR-DIST", "FG-IOT-AGR01", 40.00, 1950000.00},
			{"WH-CKR-DIST", "FG-IOT-FLT01", 100.00, 750000.00},
			{"WH-CKR-DIST", "TRD-SVR-EDG01", 8.00, 14500000.00},
			{"WH-CKR-DIST", "TRD-SW-IND08", 15.00, 4800000.00},
			{"WH-CKR-DIST", "TRD-UPS-IND01", 10.00, 2600000.00},
			{"WH-CKR-DIST", "TRD-SEN-RAD01", 5.00, 9500000.00},

			// Finished Goods & Trading in WH-SBY-DIST (East Java Regional Hub)
			{"WH-SBY-DIST", "FG-IOT-GW01", 35.00, 2250000.00},
			{"WH-SBY-DIST", "FG-IOT-ENV01", 25.00, 1450000.00},
			{"WH-SBY-DIST", "FG-IOT-PWR01", 45.00, 1150000.00},
			{"WH-SBY-DIST", "FG-IOT-WTR01", 20.00, 1650000.00},
			{"WH-SBY-DIST", "FG-IOT-AGR01", 30.00, 1950000.00},
			{"WH-SBY-DIST", "FG-IOT-FLT01", 50.00, 750000.00},
			{"WH-SBY-DIST", "TRD-SVR-EDG01", 4.00, 14500000.00},
			{"WH-SBY-DIST", "TRD-SW-IND08", 8.00, 4800000.00},
			{"WH-SBY-DIST", "TRD-UPS-IND01", 5.00, 2600000.00},
			{"WH-SBY-DIST", "TRD-SEN-RAD01", 3.00, 9500000.00},

			// Finished Goods & Trading in WH-BDG-HUB (Bandung Hub Distribution)
			{"WH-BDG-HUB", "FG-IOT-GW01", 20.00, 2250000.00},
			{"WH-BDG-HUB", "FG-IOT-ENV01", 15.00, 1450000.00},
			{"WH-BDG-HUB", "FG-IOT-PWR01", 25.00, 1150000.00},
			{"WH-BDG-HUB", "FG-IOT-WTR01", 10.00, 1650000.00},
			{"WH-BDG-HUB", "FG-IOT-AGR01", 15.00, 1950000.00},
			{"WH-BDG-HUB", "FG-IOT-FLT01", 30.00, 750000.00},
			{"WH-BDG-HUB", "TRD-SVR-EDG01", 2.00, 14500000.00},
			{"WH-BDG-HUB", "TRD-SW-IND08", 5.00, 4800000.00},
			{"WH-BDG-HUB", "TRD-UPS-IND01", 3.00, 2600000.00},
			{"WH-BDG-HUB", "TRD-SEN-RAD01", 2.00, 9500000.00},
		}

		curatedMap := make(map[string]stockKey)
		for _, cs := range curatedStock {
			key := fmt.Sprintf("%s:%s", cs.whCode, cs.sku)
			curatedMap[key] = cs
		}

		// Product default base costs
		skuBaseCosts := map[string]float64{
			"RM-PCB-GW01":    24000.00,
			"RM-ENC-IP67":    68000.00,
			"RM-ANT-LORA":    75000.00,
			"RM-CON-M12":     18000.00,
			"RM-BAT-LIION":   58000.00,
			"RM-PKG-BOX01":   8500.00,
			"CMP-MCU-ESP32":  62000.00,
			"CMP-MDM-4G":     150000.00,
			"CMP-PWR-SMPS":   28000.00,
			"CMP-SEN-BME680": 95000.00,
			"FG-IOT-GW01":    2250000.00,
			"FG-IOT-ENV01":   1450000.00,
			"FG-IOT-PWR01":   1150000.00,
			"FG-IOT-WTR01":   1650000.00,
			"FG-IOT-AGR01":   1950000.00,
			"FG-IOT-FLT01":   750000.00,
			"TRD-SVR-EDG01":  14500000.00,
			"TRD-SW-IND08":   4800000.00,
			"TRD-UPS-IND01":  2600000.00,
			"TRD-SEN-RAD01":  9500000.00,
		}

		allWHCodes := map[int64]string{
			whJktRaw:  "WH-JKT-RAW",
			whJktFG:   "WH-JKT-FG",
			whJktWIP:  "WH-JKT-WIP",
			whSbyDist: "WH-SBY-DIST",
			whCkrDist: "WH-CKR-DIST",
			whBdgHub:  "WH-BDG-HUB",
		}

		// Ensure all 20 products x 6 warehouses have valid non-negative balances
		for sku, prodID := range sctx.ProductIDs {
			baseCost := skuBaseCosts[sku]
			if baseCost == 0 {
				baseCost = 100000.00
			}

			for _, whID := range allWarehouses {
				whCode := allWHCodes[whID]
				mapKey := fmt.Sprintf("%s:%s", whCode, sku)

				qty := 10.00 // Default baseline positive quantity
				avgCost := baseCost

				if cs, exists := curatedMap[mapKey]; exists {
					qty = cs.qty
					avgCost = cs.avgCost
				}

				_, err := tx.Exec(ctx, `
					INSERT INTO inventory_balances (warehouse_id, product_id, qty, avg_cost, updated_at)
					VALUES ($1, $2, $3, $4, NOW())
					ON CONFLICT (warehouse_id, product_id) DO UPDATE SET
						qty = EXCLUDED.qty,
						avg_cost = EXCLUDED.avg_cost,
						updated_at = NOW()`,
					whID, prodID, qty, avgCost,
				)
				if err != nil {
					return fmt.Errorf("upsert inventory balance for %s at wh %d: %w", sku, whID, err)
				}
			}
		}

		// -------------------------------------------------------------------------
		// 2. Inventory Transactions (IN, OUT, TRANSFER, ADJUST)
		// -------------------------------------------------------------------------
		type txLineDef struct {
			sku      string
			qty      float64
			unitCost float64
			srcWH    *int64
			dstWH    *int64
		}

		type txDef struct {
			code     string
			txType   string
			whID     *int64
			refMod   string
			note     string
			postedAt time.Time
			lines    []txLineDef
		}

		whJktRawPtr := &whJktRaw
		whJktFGPtr := &whJktFG
		whSbyDistPtr := &whSbyDist
		whCkrDistPtr := &whCkrDist

		inventoryTxs := []txDef{
			{
				code:     "ITX-202603-0001",
				txType:   "IN",
				whID:     whJktRawPtr,
				refMod:   "PROCUREMENT.GRN",
				note:     "Goods Receipt initial batch for Quectel 4G modules & ESP32-S3 MCUs (GRN-202603-00001)",
				postedAt: time.Date(2026, 3, 5, 10, 30, 0, 0, time.UTC),
				lines: []txLineDef{
					{sku: "CMP-MDM-4G", qty: 300, unitCost: 150000.00, srcWH: nil, dstWH: whJktRawPtr},
					{sku: "CMP-MCU-ESP32", qty: 500, unitCost: 62000.00, srcWH: nil, dstWH: whJktRawPtr},
				},
			},
			{
				code:     "ITX-202603-0002",
				txType:   "OUT",
				whID:     whJktFGPtr,
				refMod:   "SALES.DO",
				note:     "Delivery Order fulfillment for PT Telkom Infrastruktur (DO-202603-00001)",
				postedAt: time.Date(2026, 3, 25, 9, 15, 0, 0, time.UTC),
				lines: []txLineDef{
					{sku: "FG-IOT-GW01", qty: 50, unitCost: 2250000.00, srcWH: whJktFGPtr, dstWH: nil},
					{sku: "RM-ANT-LORA", qty: 50, unitCost: 75000.00, srcWH: whJktFGPtr, dstWH: nil},
				},
			},
			{
				code:     "ITX-202604-0003",
				txType:   "TRANSFER",
				whID:     whJktFGPtr,
				refMod:   "LOGISTICS.TRANSFER",
				note:     "Inter-warehouse replenishment transfer Jakarta FG to Surabaya Distribution Hub",
				postedAt: time.Date(2026, 4, 15, 14, 0, 0, 0, time.UTC),
				lines: []txLineDef{
					{sku: "FG-IOT-GW01", qty: 25, unitCost: 2250000.00, srcWH: whJktFGPtr, dstWH: whSbyDistPtr},
					{sku: "FG-IOT-PWR01", qty: 30, unitCost: 1150000.00, srcWH: whJktFGPtr, dstWH: whSbyDistPtr},
				},
			},
			{
				code:     "ITX-202605-0004",
				txType:   "IN",
				whID:     whCkrDistPtr,
				refMod:   "MRP.WORK_ORDER",
				note:     "Completed manufacturing production run for IoT Gateways and Power Meters (WO-202605-001)",
				postedAt: time.Date(2026, 5, 20, 16, 45, 0, 0, time.UTC),
				lines: []txLineDef{
					{sku: "FG-IOT-GW01", qty: 40, unitCost: 2250000.00, srcWH: nil, dstWH: whCkrDistPtr},
					{sku: "FG-IOT-PWR01", qty: 60, unitCost: 1150000.00, srcWH: nil, dstWH: whCkrDistPtr},
				},
			},
			{
				code:     "ITX-202606-0005",
				txType:   "ADJUST",
				whID:     whJktFGPtr,
				refMod:   "INVENTORY.ADJUSTMENT",
				note:     "Mid-year physical stock audit variance adjustment (ADJ-202606-001)",
				postedAt: time.Date(2026, 6, 30, 17, 0, 0, 0, time.UTC),
				lines: []txLineDef{
					{sku: "FG-IOT-GW01", qty: -2, unitCost: 2250000.00, srcWH: whJktFGPtr, dstWH: nil},
					{sku: "FG-IOT-ENV01", qty: 2, unitCost: 1450000.00, srcWH: nil, dstWH: whJktFGPtr},
				},
			},
			{
				code:     "ITX-202607-0006",
				txType:   "OUT",
				whID:     whJktFGPtr,
				refMod:   "SALES.DO",
				note:     "Delivery Order fulfillment for PT PLN Nusantara Power (DO-202603-00002)",
				postedAt: time.Date(2026, 7, 10, 11, 30, 0, 0, time.UTC),
				lines: []txLineDef{
					{sku: "FG-IOT-PWR01", qty: 100, unitCost: 1150000.00, srcWH: whJktFGPtr, dstWH: nil},
					{sku: "FG-IOT-ENV01", qty: 40, unitCost: 1450000.00, srcWH: whJktFGPtr, dstWH: nil},
				},
			},
			{
				code:     "ITX-202608-0007",
				txType:   "TRANSFER",
				whID:     whJktRawPtr,
				refMod:   "LOGISTICS.TRANSFER",
				note:     "Raw materials staging transfer from Central Warehouse Jakarta to Cikarang Assembly Plant",
				postedAt: time.Date(2026, 8, 5, 9, 0, 0, 0, time.UTC),
				lines: []txLineDef{
					{sku: "RM-PCB-GW01", qty: 200, unitCost: 24000.00, srcWH: whJktRawPtr, dstWH: whCkrDistPtr},
					{sku: "CMP-MCU-ESP32", qty: 200, unitCost: 62000.00, srcWH: whJktRawPtr, dstWH: whCkrDistPtr},
				},
			},
		}

		// Track running balances for inventory cards
		runningCardBal := make(map[string]float64)

		for _, txItem := range inventoryTxs {
			var itxID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO inventory_tx (
					code, tx_type, warehouse_id, ref_module, note, posted_at, created_by, created_at
				)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $6)
				ON CONFLICT (code) DO UPDATE SET
					tx_type = EXCLUDED.tx_type,
					warehouse_id = EXCLUDED.warehouse_id,
					ref_module = EXCLUDED.ref_module,
					note = EXCLUDED.note,
					posted_at = EXCLUDED.posted_at
				RETURNING id`,
				txItem.code, txItem.txType, txItem.whID, txItem.refMod, txItem.note, txItem.postedAt, warehouseLeadID,
			).Scan(&itxID)
			if err != nil {
				return fmt.Errorf("upsert inventory_tx %q: %w", txItem.code, err)
			}

			// Clean existing lines and cards for idempotency
			_, _ = tx.Exec(ctx, `DELETE FROM inventory_tx_lines WHERE tx_id = $1`, itxID)
			_, _ = tx.Exec(ctx, `DELETE FROM inventory_cards WHERE tx_id = $1`, itxID)

			for _, l := range txItem.lines {
				prodID := sctx.ProductIDs[l.sku]
				if prodID == 0 {
					return fmt.Errorf("product %q for tx %q not found", l.sku, txItem.code)
				}

				// Insert line
				_, err := tx.Exec(ctx, `
					INSERT INTO inventory_tx_lines (
						tx_id, product_id, qty, unit_cost, src_warehouse_id, dst_warehouse_id
					)
					VALUES ($1, $2, $3, $4, $5, $6)`,
					itxID, prodID, l.qty, l.unitCost, l.srcWH, l.dstWH,
				)
				if err != nil {
					return fmt.Errorf("insert inventory_tx_lines for %s in %s: %w", l.sku, txItem.code, err)
				}

				// Generate Inventory Card Ledger entry
				targetWH := whJktFG
				if txItem.whID != nil {
					targetWH = *txItem.whID
				}
				cardKey := fmt.Sprintf("%d:%d", targetWH, prodID)
				curBal := runningCardBal[cardKey]
				if curBal == 0 {
					curBal = 100.00 // Default baseline
				}

				var qtyIn, qtyOut float64
				if l.qty >= 0 {
					if txItem.txType == "OUT" {
						qtyOut = l.qty
						curBal -= l.qty
					} else {
						qtyIn = l.qty
						curBal += l.qty
					}
				} else {
					qtyOut = -l.qty
					curBal += l.qty
				}
				runningCardBal[cardKey] = curBal
				balCost := curBal * l.unitCost

				_, err = tx.Exec(ctx, `
					INSERT INTO inventory_cards (
						warehouse_id, product_id, tx_id, tx_code, tx_type,
						qty_in, qty_out, balance_qty, unit_cost, balance_cost,
						posted_at, note, created_at
					)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW())`,
					targetWH, prodID, itxID, txItem.code, txItem.txType,
					qtyIn, qtyOut, curBal, l.unitCost, balCost,
					txItem.postedAt, txItem.note,
				)
				if err != nil {
					return fmt.Errorf("insert inventory_card for %s in %s: %w", l.sku, txItem.code, err)
				}
			}
		}

		// -------------------------------------------------------------------------
		// 3. Inventory Stock Takes (1 POSTED with Variances, 1 DRAFT)
		// -------------------------------------------------------------------------
		type stockTakeLineDef struct {
			sku         string
			systemQty   float64
			physicalQty float64
			note        string
		}

		type stockTakeDef struct {
			number   string
			whID     int64
			status   string
			note     string
			takenAt  time.Time
			postedAt *time.Time
			lines    []stockTakeLineDef
		}

		tStockPosted1 := time.Date(2026, 6, 30, 16, 0, 0, 0, time.UTC)

		stockTakes := []stockTakeDef{
			{
				number:   "STK-202606-001",
				whID:     whJktFG,
				status:   "POSTED",
				note:     "End-of-Q2 physical inventory count audit at Finished Goods Warehouse Jakarta",
				takenAt:  time.Date(2026, 6, 28, 9, 0, 0, 0, time.UTC),
				postedAt: &tStockPosted1,
				lines: []stockTakeLineDef{
					{sku: "FG-IOT-GW01", systemQty: 122.00, physicalQty: 120.00, note: "2 units missing at staging rack A-01-02 (damaged casing)"},
					{sku: "FG-IOT-ENV01", systemQty: 83.00, physicalQty: 85.00, note: "2 extra units located during recount in rack B-01-01"},
					{sku: "FG-IOT-PWR01", systemQty: 160.00, physicalQty: 160.00, note: "Physical match 100% verified"},
					{sku: "FG-IOT-WTR01", systemQty: 90.00, physicalQty: 90.00, note: "Physical match 100% verified"},
					{sku: "FG-IOT-FLT01", systemQty: 220.00, physicalQty: 220.00, note: "Physical match 100% verified"},
				},
			},
			{
				number:   "STK-202608-002",
				whID:     whCkrDist,
				status:   "DRAFT",
				note:     "Cycle count for raw material electronics buffer at Cikarang Manufacturing Plant",
				takenAt:  time.Date(2026, 8, 25, 14, 0, 0, 0, time.UTC),
				postedAt: nil,
				lines: []stockTakeLineDef{
					{sku: "RM-PCB-GW01", systemQty: 500.00, physicalQty: 498.00, note: "Count in progress on SMD reel feeder rack"},
					{sku: "CMP-MCU-ESP32", systemQty: 800.00, physicalQty: 800.00, note: "Full reels intact in moisture barrier bags"},
					{sku: "CMP-MDM-4G", systemQty: 400.00, physicalQty: 400.00, note: "Full reels intact in dry storage cabinet"},
				},
			},
		}

		for _, st := range stockTakes {
			var postedBy *int64
			if st.status == "POSTED" {
				postedBy = &warehouseLeadID
			}

			var stID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO inventory_stock_takes (
					number, warehouse_id, status, note, taken_at,
					created_by, posted_by, posted_at, created_at
				)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $5)
				ON CONFLICT (number) DO UPDATE SET
					warehouse_id = EXCLUDED.warehouse_id,
					status = EXCLUDED.status,
					note = EXCLUDED.note,
					taken_at = EXCLUDED.taken_at,
					posted_by = EXCLUDED.posted_by,
					posted_at = EXCLUDED.posted_at
				RETURNING id`,
				st.number, st.whID, st.status, st.note, st.takenAt,
				warehouseLeadID, postedBy, st.postedAt,
			).Scan(&stID)
			if err != nil {
				return fmt.Errorf("upsert inventory_stock_takes %q: %w", st.number, err)
			}

			// Clean existing lines
			_, _ = tx.Exec(ctx, `DELETE FROM inventory_stock_take_lines WHERE stock_take_id = $1`, stID)

			for _, l := range st.lines {
				prodID := sctx.ProductIDs[l.sku]
				if prodID == 0 {
					return fmt.Errorf("product %q for stock take %q not found", l.sku, st.number)
				}

				_, err := tx.Exec(ctx, `
					INSERT INTO inventory_stock_take_lines (
						stock_take_id, product_id, system_qty, physical_qty, note
					)
					VALUES ($1, $2, $3, $4, $5)`,
					stID, prodID, l.systemQty, l.physicalQty, l.note,
				)
				if err != nil {
					return fmt.Errorf("insert stock take line for %s in %s: %w", l.sku, st.number, err)
				}
			}
		}

		// -------------------------------------------------------------------------
		// 4. Inventory Adjustments (1 POSTED Write-off/Gain, 1 DRAFT)
		// -------------------------------------------------------------------------
		type adjLineDef struct {
			sku  string
			qty  float64
			note string
		}

		type adjDef struct {
			number   string
			whID     int64
			status   string
			note     string
			adjAt    time.Time
			postedAt *time.Time
			lines    []adjLineDef
		}

		tAdjPosted1 := time.Date(2026, 6, 30, 17, 0, 0, 0, time.UTC)

		adjustments := []adjDef{
			{
				number:   "ADJ-202606-001",
				whID:     whJktFG,
				status:   "POSTED",
				note:     "Reconciliation adjustment for physical variances identified during Q2 stock take STK-202606-001",
				adjAt:    time.Date(2026, 6, 30, 16, 30, 0, 0, time.UTC),
				postedAt: &tAdjPosted1,
				lines: []adjLineDef{
					{sku: "FG-IOT-GW01", qty: -2.00, note: "Write-off 2 damaged units during packing (approved by Warehouse Lead)"},
					{sku: "FG-IOT-ENV01", qty: 2.00, note: "Inventory gain 2 units found during staging recount"},
				},
			},
			{
				number:   "ADJ-202608-002",
				whID:     whJktRaw,
				status:   "DRAFT",
				note:     "Draft adjustment for raw material enclosure surface scratch during unboxing",
				adjAt:    time.Date(2026, 8, 28, 11, 0, 0, 0, time.UTC),
				postedAt: nil,
				lines: []adjLineDef{
					{sku: "RM-ENC-IP67", qty: -1.00, note: "Minor cosmetic flaw rejection pending supervisor approval"},
				},
			},
		}

		for _, adj := range adjustments {
			var postedBy *int64
			if adj.status == "POSTED" {
				postedBy = &warehouseLeadID
			}

			var adjID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO inventory_adjustments (
					number, warehouse_id, status, note, adjustment_at,
					created_by, posted_by, posted_at, created_at
				)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $5)
				ON CONFLICT (number) DO UPDATE SET
					warehouse_id = EXCLUDED.warehouse_id,
					status = EXCLUDED.status,
					note = EXCLUDED.note,
					adjustment_at = EXCLUDED.adjustment_at,
					posted_by = EXCLUDED.posted_by,
					posted_at = EXCLUDED.posted_at
				RETURNING id`,
				adj.number, adj.whID, adj.status, adj.note, adj.adjAt,
				warehouseLeadID, postedBy, adj.postedAt,
			).Scan(&adjID)
			if err != nil {
				return fmt.Errorf("upsert inventory_adjustments %q: %w", adj.number, err)
			}

			// Clean existing lines
			_, _ = tx.Exec(ctx, `DELETE FROM inventory_adjustment_lines WHERE adjustment_id = $1`, adjID)

			for _, l := range adj.lines {
				prodID := sctx.ProductIDs[l.sku]
				if prodID == 0 {
					return fmt.Errorf("product %q for adjustment %q not found", l.sku, adj.number)
				}

				_, err := tx.Exec(ctx, `
					INSERT INTO inventory_adjustment_lines (
						adjustment_id, product_id, qty, note
					)
					VALUES ($1, $2, $3, $4)`,
					adjID, prodID, l.qty, l.note,
				)
				if err != nil {
					return fmt.Errorf("insert adjustment line for %s in %s: %w", l.sku, adj.number, err)
				}
			}
		}

		// -------------------------------------------------------------------------
		// 5. WMS Storage Bins (14+ Bins Spanning Zones A-D across Warehouses)
		// -------------------------------------------------------------------------
		type binDef struct {
			companyID   int64
			warehouseID int64
			code        string
			name        string
			capacity    float64
			active      bool
		}

		wmsBins := []binDef{
			// WH-JKT-FG Bins (PT NTP)
			{sctx.CompanyNTPID, whJktFG, "BIN-JKT-A-01-01", "Zone A Rack 01 Shelf 01 - Finished Goods Staging", 500.00, true},
			{sctx.CompanyNTPID, whJktFG, "BIN-JKT-A-01-02", "Zone A Rack 01 Shelf 02 - IoT Gateways High Bay", 300.00, true},
			{sctx.CompanyNTPID, whJktFG, "BIN-JKT-A-02-01", "Zone A Rack 02 Shelf 01 - Smart Power Meters Storage", 400.00, true},
			{sctx.CompanyNTPID, whJktFG, "BIN-JKT-B-01-01", "Zone B Rack 01 Shelf 01 - Environmental Monitors", 250.00, true},
			{sctx.CompanyNTPID, whJktFG, "BIN-JKT-B-02-01", "Zone B Rack 02 Shelf 01 - Ultrasonic Water Level Sensors", 200.00, true},
			{sctx.CompanyNTPID, whJktFG, "BIN-JKT-C-01-01", "Zone C Rack 01 Shelf 01 - Advantech Industrial Edge Servers", 50.00, true},
			{sctx.CompanyNTPID, whJktFG, "BIN-JKT-C-02-01", "Zone C Rack 02 Shelf 01 - Moxa Managed Switches", 100.00, true},
			{sctx.CompanyNTPID, whJktFG, "BIN-JKT-D-01-01", "Zone D Rack 01 Shelf 01 - Bulk Pallet Heavy Storage", 2000.00, true},

			// WH-JKT-RAW Bins (PT NTP)
			{sctx.CompanyNTPID, whJktRaw, "BIN-JKT-RM-A01", "Raw Materials Zone A Shelf 01 - PCB Blank Stock", 2000.00, true},
			{sctx.CompanyNTPID, whJktRaw, "BIN-JKT-RM-B01", "Raw Materials Zone B Shelf 01 - MCU & 4G SMD Reels", 5000.00, true},
			{sctx.CompanyNTPID, whJktRaw, "BIN-JKT-RM-C01", "Raw Materials Zone C Shelf 01 - Aluminium Enclosures", 1000.00, true},

			// WH-CKR-DIST Bins (PT NDM)
			{sctx.CompanyNDMID, whCkrDist, "BIN-CKR-A-01-01", "SMT Feeder Line Component Reel Rack A1", 3000.00, true},
			{sctx.CompanyNDMID, whCkrDist, "BIN-CKR-B-01-01", "SMT Feeder Line Component Reel Rack B1", 3000.00, true},
			{sctx.CompanyNDMID, whCkrDist, "BIN-CKR-C-01-01", "Subassembly PCBA Staging Rack C1", 500.00, true},

			// WH-SBY-DIST Bins (PT NTP)
			{sctx.CompanyNTPID, whSbyDist, "BIN-SBY-A-01-01", "East Java Fast-Moving Distribution Bay A1", 600.00, true},

			// WH-BDG-HUB Bins (PT NDM)
			{sctx.CompanyNDMID, whBdgHub, "BIN-BDG-A-01-01", "Bandung Hub Distribution Bay A1", 600.00, true},
		}

		for _, b := range wmsBins {
			_, err := tx.Exec(ctx, `
				INSERT INTO wms_bins (
					company_id, warehouse_id, code, name, capacity, active,
					created_by, created_at, updated_at
				)
				VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
				ON CONFLICT (company_id, warehouse_id, code) DO UPDATE SET
					name = EXCLUDED.name,
					capacity = EXCLUDED.capacity,
					active = EXCLUDED.active,
					updated_at = NOW()`,
				b.companyID, b.warehouseID, b.code, b.name, b.capacity, b.active, adminID,
			)
			if err != nil {
				return fmt.Errorf("upsert wms_bin %q at wh %d: %w", b.code, b.warehouseID, err)
			}
		}

		return nil
	})
}
