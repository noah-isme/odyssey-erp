package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// seedPhase06MRP seeds manufacturing master data (work centers, BOMs, routings, operations),
// WIP staging locations, and production work orders with operation tracking and material movements.
func seedPhase06MRP(ctx context.Context, sctx *SeedContext) error {
	return ExecInTx(ctx, sctx.Pool, "Phase 06: Manufacturing MRP", func(tx pgx.Tx) error {
		companyID := sctx.CompanyNTPID
		if companyID == 0 {
			var err error
			companyID, err = LookupCompanyID(ctx, tx, "NTP-HQ")
			if err != nil {
				return err
			}
			sctx.CompanyNTPID = companyID
		}

		// Lookup key user IDs
		prodMgrID := sctx.UserIDs["joko.prasetyo@nusantarateknik.co.id"]
		if prodMgrID == 0 {
			var err error
			prodMgrID, err = LookupUserID(ctx, tx, "joko.prasetyo@nusantarateknik.co.id")
			if err != nil {
				return err
			}
			sctx.UserIDs["joko.prasetyo@nusantarateknik.co.id"] = prodMgrID
		}

		adminID := sctx.UserIDs["budi.santoso@nusantarateknik.co.id"]
		if adminID == 0 {
			var err error
			adminID, err = LookupUserID(ctx, tx, "budi.santoso@nusantarateknik.co.id")
			if err != nil {
				return err
			}
			sctx.UserIDs["budi.santoso@nusantarateknik.co.id"] = adminID
		}

		qaMgrID := sctx.UserIDs["ratna.sari@nusantarateknik.co.id"]
		if qaMgrID == 0 {
			var err error
			qaMgrID, err = LookupUserID(ctx, tx, "ratna.sari@nusantarateknik.co.id")
			if err != nil {
				return err
			}
			sctx.UserIDs["ratna.sari@nusantarateknik.co.id"] = qaMgrID
		}

		// Helper to resolve product ID by SKU
		getProdID := func(sku string) (int64, error) {
			if id, ok := sctx.ProductIDs[sku]; ok && id > 0 {
				return id, nil
			}
			id, err := LookupProductID(ctx, tx, sku)
			if err != nil {
				return 0, err
			}
			sctx.ProductIDs[sku] = id
			return id, nil
		}

		// Helper to resolve warehouse ID by code
		getWhID := func(code string) (int64, error) {
			if id, ok := sctx.WarehouseIDs[code]; ok && id > 0 {
				return id, nil
			}
			id, err := LookupWarehouseID(ctx, tx, code)
			if err != nil {
				return 0, err
			}
			sctx.WarehouseIDs[code] = id
			return id, nil
		}

		whRawID, err := getWhID("WH-JKT-RAW")
		if err != nil {
			return err
		}
		whFGID, err := getWhID("WH-JKT-FG")
		if err != nil {
			return err
		}
		whWIPID, err := getWhID("WH-JKT-WIP")
		if err != nil {
			return err
		}

		// -------------------------------------------------------------------------
		// 1. Work Centers (3 Core Electronics Production Work Centers)
		// -------------------------------------------------------------------------
		workCenters := []struct {
			code     string
			name     string
			capacity float64
		}{
			{"WC-SMT-01", "SMT Pick-and-Place Line 1 (Cleanroom)", 16.0},
			{"WC-ASY-01", "Sub-assembly & Hand Soldering Line", 16.0},
			{"WC-TST-01", "Final QA & 24h Burn-in Chamber Station", 16.0},
		}

		wcMap := make(map[string]int64)
		for _, wc := range workCenters {
			var wcID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO mrp_work_centers (company_id, code, name, capacity_hours_per_day, active, created_by, created_at, updated_at)
				VALUES ($1, $2, $3, $4, TRUE, $5, NOW(), NOW())
				ON CONFLICT (company_id, code) DO UPDATE SET
					name = EXCLUDED.name,
					capacity_hours_per_day = EXCLUDED.capacity_hours_per_day,
					active = TRUE,
					updated_at = NOW()
				RETURNING id`, companyID, wc.code, wc.name, wc.capacity, prodMgrID).Scan(&wcID)
			if err != nil {
				return fmt.Errorf("upsert mrp_work_center %q: %w", wc.code, err)
			}
			wcMap[wc.code] = wcID
		}

		// -------------------------------------------------------------------------
		// 2. WIP Staging Locations (Linking Raw Material, WIP Warehouse & Work Centers)
		// -------------------------------------------------------------------------
		wipLocations := []struct {
			wcCode string
			name   string
		}{
			{"WC-SMT-01", "SMT Line Raw-to-WIP Buffer"},
			{"WC-ASY-01", "Manual Assembly WIP Staging Area"},
			{"WC-TST-01", "Burn-in Chamber & Testing Buffer"},
		}

		for _, wl := range wipLocations {
			wcID := wcMap[wl.wcCode]
			if _, err := tx.Exec(ctx, `
				INSERT INTO mrp_wip_locations (company_id, warehouse_id, wip_warehouse_id, work_center_id, name, active, created_by)
				VALUES ($1, $2, $3, $4, $5, TRUE, $6)
				ON CONFLICT (company_id, warehouse_id, work_center_id) DO UPDATE SET
					wip_warehouse_id = EXCLUDED.wip_warehouse_id,
					name = EXCLUDED.name,
					active = TRUE`, companyID, whRawID, whWIPID, wcID, wl.name, prodMgrID); err != nil {
				return fmt.Errorf("upsert mrp_wip_location for wc %q: %w", wl.wcCode, err)
			}
		}

		// -------------------------------------------------------------------------
		// 3. Routings & Operation Sequences (3 Finished Goods Products)
		// -------------------------------------------------------------------------
		type routingOpDef struct {
			sequence int
			code     string
			name     string
			wcCode   string
			setupMin float64
			runMin   float64
			yieldPct float64
		}

		routingsDef := []struct {
			productSKU string
			code       string
			version    string
			operations []routingOpDef
		}{
			{
				productSKU: "FG-IOT-GW01",
				code:       "RT-IOT-GW01",
				version:    "v1.0",
				operations: []routingOpDef{
					{10, "OP-SMT-GW01", "High-Speed SMT Surface Mount Component Assembly", "WC-SMT-01", 30.0, 15.0, 99.5},
					{20, "OP-ASY-GW01", "Through-Hole Soldering & LoRa/4G Module Integration", "WC-ASY-01", 15.0, 25.0, 99.0},
					{30, "OP-ENC-GW01", "IP67 Die-Cast Aluminum Enclosure Assembly & Potting", "WC-ASY-01", 10.0, 20.0, 99.5},
					{40, "OP-TST-GW01", "24-Hour Thermal Burn-in Chamber & RF Transceiver Automated Test", "WC-TST-01", 20.0, 60.0, 98.5},
				},
			},
			{
				productSKU: "FG-IOT-ENV01",
				code:       "RT-IOT-ENV01",
				version:    "v1.0",
				operations: []routingOpDef{
					{10, "OP-SMT-ENV01", "SMT Environmental Sensor Mainboard Assembly", "WC-SMT-01", 25.0, 12.0, 99.5},
					{20, "OP-ASY-ENV01", "BME680 Multi-Gas Sensor & Battery Pack Assembly", "WC-ASY-01", 15.0, 20.0, 99.0},
					{30, "OP-TST-ENV01", "Environmental Chamber Multi-Point Calibration & Burn-in", "WC-TST-01", 20.0, 45.0, 99.0},
				},
			},
			{
				productSKU: "FG-IOT-PWR01",
				code:       "RT-IOT-PWR01",
				version:    "v1.0",
				operations: []routingOpDef{
					{10, "OP-SMT-PWR01", "SMT Power Meter Processing Board Assembly", "WC-SMT-01", 25.0, 10.0, 99.5},
					{20, "OP-ASY-PWR01", "High-Voltage Isolation Transformer & Terminal Soldering", "WC-ASY-01", 15.0, 15.0, 99.5},
					{30, "OP-TST-PWR01", "3-Phase 3kV Dielectric Insulation & Modbus RS485 Calibration", "WC-TST-01", 15.0, 30.0, 99.0},
				},
			},
		}

		routingMap := make(map[string]int64)
		routingOpMap := make(map[string]map[int]int64)

		for _, rDef := range routingsDef {
			prodID, err := getProdID(rDef.productSKU)
			if err != nil {
				return err
			}

			var rID int64
			err = tx.QueryRow(ctx, `
				INSERT INTO mrp_routings (company_id, product_id, code, version, active, created_by, created_at, updated_at)
				VALUES ($1, $2, $3, $4, TRUE, $5, NOW(), NOW())
				ON CONFLICT (company_id, code, version) DO UPDATE SET
					product_id = EXCLUDED.product_id,
					active = TRUE,
					updated_at = NOW()
				RETURNING id`, companyID, prodID, rDef.code, rDef.version, prodMgrID).Scan(&rID)
			if err != nil {
				return fmt.Errorf("upsert mrp_routing %q: %w", rDef.code, err)
			}
			routingMap[rDef.productSKU] = rID
			routingOpMap[rDef.productSKU] = make(map[int]int64)

			for _, op := range rDef.operations {
				wcID := wcMap[op.wcCode]
				var opID int64
				err := tx.QueryRow(ctx, `
					INSERT INTO mrp_routing_operations (routing_id, work_center_id, sequence, code, name, setup_minutes, run_minutes, yield_pct)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
					ON CONFLICT (routing_id, sequence) DO UPDATE SET
						work_center_id = EXCLUDED.work_center_id,
						code = EXCLUDED.code,
						name = EXCLUDED.name,
						setup_minutes = EXCLUDED.setup_minutes,
						run_minutes = EXCLUDED.run_minutes,
						yield_pct = EXCLUDED.yield_pct
					RETURNING id`, rID, wcID, op.sequence, op.code, op.name, op.setupMin, op.runMin, op.yieldPct).Scan(&opID)
				if err != nil {
					return fmt.Errorf("upsert mrp_routing_operation %q (seq %d): %w", op.code, op.sequence, err)
				}
				routingOpMap[rDef.productSKU][op.sequence] = opID
			}
		}

		// -------------------------------------------------------------------------
		// 4. Bills of Materials (BOMs) with 4-6 Components Each & Revision Control
		// -------------------------------------------------------------------------
		type bomLineDef struct {
			componentSKU string
			quantity     float64
			scrapPct     float64
		}

		bomsDef := []struct {
			productSKU string
			version    string
			effective  string
			scrapPct   float64
			lines      []bomLineDef
		}{
			{
				productSKU: "FG-IOT-GW01",
				version:    "v1.0",
				effective:  "2026-03-01",
				scrapPct:   1.50,
				lines: []bomLineDef{
					{"RM-PCB-GW01", 1.0, 1.0},
					{"RM-ENC-IP67", 1.0, 0.5},
					{"RM-ANT-LORA", 1.0, 0.5},
					{"RM-CON-M12", 2.0, 1.0},
					{"CMP-MCU-ESP32", 1.0, 1.0},
					{"CMP-MDM-4G", 1.0, 0.5},
					{"RM-PKG-BOX01", 1.0, 0.0},
				},
			},
			{
				productSKU: "FG-IOT-ENV01",
				version:    "v1.0",
				effective:  "2026-03-01",
				scrapPct:   1.00,
				lines: []bomLineDef{
					{"RM-PCB-GW01", 1.0, 1.0},
					{"RM-ENC-IP67", 1.0, 0.5},
					{"CMP-MCU-ESP32", 1.0, 1.0},
					{"CMP-SEN-BME680", 1.0, 1.0},
					{"RM-BAT-LIION", 1.0, 0.5},
					{"RM-PKG-BOX01", 1.0, 0.0},
				},
			},
			{
				productSKU: "FG-IOT-PWR01",
				version:    "v1.0",
				effective:  "2026-03-01",
				scrapPct:   1.00,
				lines: []bomLineDef{
					{"RM-PCB-GW01", 1.0, 1.0},
					{"CMP-MCU-ESP32", 1.0, 1.0},
					{"CMP-PWR-SMPS", 1.0, 0.5},
					{"RM-CON-M12", 2.0, 1.0},
					{"RM-PKG-BOX01", 1.0, 0.0},
				},
			},
		}

		bomMap := make(map[string]int64)

		for _, bDef := range bomsDef {
			prodID, err := getProdID(bDef.productSKU)
			if err != nil {
				return err
			}
			effDate := ParseDate(bDef.effective)

			var bomID int64
			var revStatus string
			err = tx.QueryRow(ctx, `
				SELECT id, revision_status FROM mrp_boms
				WHERE company_id = $1 AND product_id = $2 AND version = $3`, companyID, prodID, bDef.version).Scan(&bomID, &revStatus)

			if errors.Is(err, pgx.ErrNoRows) {
				// Insert new BOM in DRAFT state first to allow child lines insertion
				err = tx.QueryRow(ctx, `
					INSERT INTO mrp_boms (company_id, product_id, version, effective_from, scrap_pct, active, revision_status, created_by)
					VALUES ($1, $2, $3, $4, $5, TRUE, 'DRAFT', $6)
					RETURNING id`, companyID, prodID, bDef.version, effDate, bDef.scrapPct, prodMgrID).Scan(&bomID)
				if err != nil {
					return fmt.Errorf("insert draft mrp_bom for %q: %w", bDef.productSKU, err)
				}
				revStatus = "DRAFT"
			} else if err != nil {
				return fmt.Errorf("lookup mrp_bom for %q: %w", bDef.productSKU, err)
			}

			// If in DRAFT status, populate lines and promote to APPROVED
			if revStatus == "DRAFT" {
				for _, line := range bDef.lines {
					cmpID, err := getProdID(line.componentSKU)
					if err != nil {
						return err
					}
					if _, err := tx.Exec(ctx, `
						INSERT INTO mrp_bom_lines (bom_id, component_product_id, quantity, scrap_pct)
						VALUES ($1, $2, $3, $4)
						ON CONFLICT (bom_id, component_product_id) DO UPDATE SET
							quantity = EXCLUDED.quantity,
							scrap_pct = EXCLUDED.scrap_pct`, bomID, cmpID, line.quantity, line.scrapPct); err != nil {
						return fmt.Errorf("upsert bom line %q in bom %d: %w", line.componentSKU, bomID, err)
					}
				}

				// Finalize revision approval
				approvedAt := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
				if _, err := tx.Exec(ctx, `
					UPDATE mrp_boms
					SET revision_status = 'APPROVED',
						approved_by = $1,
						approved_at = $2,
						change_reason = 'Official Production Baseline Revision v1.0'
					WHERE id = $3`, adminID, approvedAt, bomID); err != nil {
					return fmt.Errorf("approve mrp_bom %d: %w", bomID, err)
				}
			}

			bomMap[bDef.productSKU] = bomID
		}

		// -------------------------------------------------------------------------
		// 5. Work Orders (8 Work Orders covering DRAFT, RELEASED, IN_PROGRESS, COMPLETED, CLOSED, CANCELLED)
		// -------------------------------------------------------------------------
		workOrders := []struct {
			number       string
			productSKU   string
			plannedQty   float64
			completedQty float64
			status       string
			startDate    string
			dueDate      string
			completedAt  *string
		}{
			{
				number:       "WO-202603-001",
				productSKU:   "FG-IOT-GW01",
				plannedQty:   200.0,
				completedQty: 200.0,
				status:       "CLOSED",
				startDate:    "2026-03-05",
				dueDate:      "2026-03-15",
			},
			{
				number:       "WO-202604-002",
				productSKU:   "FG-IOT-ENV01",
				plannedQty:   150.0,
				completedQty: 150.0,
				status:       "COMPLETED",
				startDate:    "2026-04-10",
				dueDate:      "2026-04-20",
			},
			{
				number:       "WO-202605-003",
				productSKU:   "FG-IOT-PWR01",
				plannedQty:   100.0,
				completedQty: 100.0,
				status:       "CLOSED",
				startDate:    "2026-05-05",
				dueDate:      "2026-05-15",
			},
			{
				number:       "WO-202606-004",
				productSKU:   "FG-IOT-GW01",
				plannedQty:   300.0,
				completedQty: 300.0,
				status:       "COMPLETED",
				startDate:    "2026-06-12",
				dueDate:      "2026-06-25",
			},
			{
				number:       "WO-202607-005",
				productSKU:   "FG-IOT-ENV01",
				plannedQty:   200.0,
				completedQty: 120.0,
				status:       "IN_PROGRESS",
				startDate:    "2026-07-15",
				dueDate:      "2026-08-05",
			},
			{
				number:       "WO-202608-006",
				productSKU:   "FG-IOT-PWR01",
				plannedQty:   250.0,
				completedQty: 0.0,
				status:       "RELEASED",
				startDate:    "2026-08-10",
				dueDate:      "2026-08-28",
			},
			{
				number:       "WO-202608-007",
				productSKU:   "FG-IOT-GW01",
				plannedQty:   100.0,
				completedQty: 0.0,
				status:       "DRAFT",
				startDate:    "2026-08-25",
				dueDate:      "2026-08-31",
			},
			{
				number:       "WO-202605-008",
				productSKU:   "FG-IOT-ENV01",
				plannedQty:   50.0,
				completedQty: 0.0,
				status:       "CANCELLED",
				startDate:    "2026-05-20",
				dueDate:      "2026-05-25",
			},
		}

		for _, wo := range workOrders {
			prodID, err := getProdID(wo.productSKU)
			if err != nil {
				return err
			}
			bomID := bomMap[wo.productSKU]
			routingID := routingMap[wo.productSKU]
			startDate := ParseDate(wo.startDate)
			dueDate := ParseDate(wo.dueDate)
			createdAt := startDate.Add(8 * time.Hour)

			var woID int64
			err = tx.QueryRow(ctx, `
				INSERT INTO mrp_work_orders (company_id, number, product_id, bom_id, routing_id, warehouse_id, planned_qty, completed_qty, status, planned_start_date, planned_due_date, created_by, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $13)
				ON CONFLICT (company_id, number) DO UPDATE SET
					product_id = EXCLUDED.product_id,
					bom_id = EXCLUDED.bom_id,
					routing_id = EXCLUDED.routing_id,
					warehouse_id = EXCLUDED.warehouse_id,
					planned_qty = EXCLUDED.planned_qty,
					completed_qty = EXCLUDED.completed_qty,
					status = EXCLUDED.status,
					planned_start_date = EXCLUDED.planned_start_date,
					planned_due_date = EXCLUDED.planned_due_date,
					updated_at = EXCLUDED.updated_at
				RETURNING id`,
				companyID, wo.number, prodID, bomID, routingID, whFGID, wo.plannedQty, wo.completedQty, wo.status, startDate, dueDate, prodMgrID, createdAt).Scan(&woID)
			if err != nil {
				return fmt.Errorf("upsert mrp_work_order %q: %w", wo.number, err)
			}

			// Seed Operation Tracking for each work order
			for _, rDef := range routingsDef {
				if rDef.productSKU != wo.productSKU {
					continue
				}

				for _, op := range rDef.operations {
					routingOpID := routingOpMap[wo.productSKU][op.sequence]
					wcID := wcMap[op.wcCode]

					var opStatus string
					var goodQty, scrapQty float64
					var actualSetup, actualRun float64
					var startedAt, completedAt *time.Time

					switch wo.status {
					case "COMPLETED", "CLOSED":
						opStatus = "COMPLETED"
						goodQty = wo.plannedQty
						scrapQty = 2.0
						actualSetup = op.setupMin
						actualRun = op.runMin * (wo.plannedQty / 10.0)
						st := startDate.Add(time.Duration(op.sequence*6) * time.Hour)
						ct := st.Add(time.Duration(actualSetup+actualRun) * time.Minute)
						startedAt = &st
						completedAt = &ct

					case "IN_PROGRESS":
						if op.sequence == 10 {
							opStatus = "COMPLETED"
							goodQty = wo.completedQty
							scrapQty = 1.0
							actualSetup = op.setupMin
							actualRun = op.runMin * (wo.completedQty / 10.0)
							st := startDate.Add(8 * time.Hour)
							ct := st.Add(12 * time.Hour)
							startedAt = &st
							completedAt = &ct
						} else if op.sequence == 20 {
							opStatus = "IN_PROGRESS"
							goodQty = 50.0
							scrapQty = 0.0
							actualSetup = op.setupMin
							actualRun = 60.0
							st := startDate.Add(24 * time.Hour)
							startedAt = &st
						} else {
							opStatus = "PENDING"
						}

					case "RELEASED":
						if op.sequence == 10 {
							opStatus = "READY"
						} else {
							opStatus = "PENDING"
						}

					case "DRAFT", "CANCELLED":
						opStatus = "PENDING"
					}

					plannedRun := op.runMin * (wo.plannedQty / 10.0)

					if _, err := tx.Exec(ctx, `
						INSERT INTO mrp_work_order_operations (
							company_id, work_order_id, routing_operation_id, work_center_id,
							sequence, code, name, status,
							planned_setup_minutes, planned_run_minutes,
							actual_setup_minutes, actual_run_minutes,
							good_quantity, scrap_quantity,
							operator_id, started_at, completed_at, created_at, updated_at
						) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $18)
						ON CONFLICT (work_order_id, sequence) DO UPDATE SET
							routing_operation_id = EXCLUDED.routing_operation_id,
							work_center_id = EXCLUDED.work_center_id,
							code = EXCLUDED.code,
							name = EXCLUDED.name,
							status = EXCLUDED.status,
							planned_setup_minutes = EXCLUDED.planned_setup_minutes,
							planned_run_minutes = EXCLUDED.planned_run_minutes,
							actual_setup_minutes = EXCLUDED.actual_setup_minutes,
							actual_run_minutes = EXCLUDED.actual_run_minutes,
							good_quantity = EXCLUDED.good_quantity,
							scrap_quantity = EXCLUDED.scrap_quantity,
							operator_id = EXCLUDED.operator_id,
							started_at = EXCLUDED.started_at,
							completed_at = EXCLUDED.completed_at,
							updated_at = NOW()`,
						companyID, woID, routingOpID, wcID,
						op.sequence, op.code, op.name, opStatus,
						op.setupMin, plannedRun,
						actualSetup, actualRun,
						goodQty, scrapQty,
						prodMgrID, startedAt, completedAt, createdAt); err != nil {
						return fmt.Errorf("upsert mrp_work_order_operation for wo %q seq %d: %w", wo.number, op.sequence, err)
					}
				}
			}

			// Record Material Issue Movements for in-progress and completed orders
			if wo.status == "COMPLETED" || wo.status == "CLOSED" || wo.status == "IN_PROGRESS" {
				for _, bDef := range bomsDef {
					if bDef.productSKU != wo.productSKU {
						continue
					}
					for _, bLine := range bDef.lines {
						cmpID, err := getProdID(bLine.componentSKU)
						if err != nil {
							return err
						}
						issueQty := bLine.quantity * wo.plannedQty
						idempotencyKey := fmt.Sprintf("ISSUE-%s-%s", wo.number, bLine.componentSKU)

						if _, err := tx.Exec(ctx, `
							INSERT INTO mrp_material_movements (
								company_id, work_order_id, product_id, source_warehouse_id, destination_warehouse_id,
								quantity, movement_type, idempotency_key, created_by, created_at
							) VALUES ($1, $2, $3, $4, $5, $6, 'ISSUE', $7, $8, $9)
							ON CONFLICT (company_id, work_order_id, movement_type, idempotency_key) DO UPDATE SET
								quantity = EXCLUDED.quantity`,
							companyID, woID, cmpID, whRawID, whWIPID, issueQty, idempotencyKey, prodMgrID, createdAt); err != nil {
							return fmt.Errorf("insert mrp_material_movement for wo %q sku %q: %w", wo.number, bLine.componentSKU, err)
						}
					}
				}
			}
		}

		return nil
	})
}
