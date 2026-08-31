package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// seedPhase11POS seeds POS retail counters, hardware terminals, cashier sessions
// (1 closed yesterday, 1 open today), completed tickets with lines & multi-tender payments,
// refunds, loyalty members, and gift cards.
func seedPhase11POS(ctx context.Context, sctx *SeedContext) error {
	return ExecInTx(ctx, sctx.Pool, "Phase 11: Point of Sale (POS)", func(tx pgx.Tx) error {
		companyID := sctx.CompanyNTPID
		if companyID == 0 {
			var err error
			companyID, err = LookupCompanyID(ctx, tx, "NTP-HQ")
			if err != nil {
				return err
			}
			sctx.CompanyNTPID = companyID
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

		cashierID := sctx.UserIDs["hendra.wijaya@nusantarateknik.co.id"]
		if cashierID == 0 {
			cashierID = adminID
		}

		whFGID := sctx.WarehouseIDs["WH-JKT-FG"]
		if whFGID == 0 {
			var err error
			whFGID, err = LookupWarehouseID(ctx, tx, "WH-JKT-FG")
			if err != nil {
				return err
			}
			sctx.WarehouseIDs["WH-JKT-FG"] = whFGID
		}

		// Helper to resolve product ID
		getProductID := func(sku string) (int64, error) {
			if id, ok := sctx.ProductIDs[sku]; ok && id > 0 {
				return id, nil
			}
			id, err := LookupProductID(ctx, tx, sku)
			if err != nil {
				return 0, fmt.Errorf("lookup product %q: %w", sku, err)
			}
			sctx.ProductIDs[sku] = id
			return id, nil
		}

		// -------------------------------------------------------------------------
		// 1. POS Terminal (HQ Jakarta Retail & Demo Counter)
		// -------------------------------------------------------------------------
		var terminalID int64
		err := tx.QueryRow(ctx, `
			INSERT INTO pos_terminals (company_id, code, name, warehouse_id, active)
			VALUES ($1, 'POS-JKT-01', 'Kasir Penjualan Retail & Demo HQ Jakarta', $2, TRUE)
			ON CONFLICT (company_id, code) DO UPDATE SET
				name = EXCLUDED.name,
				warehouse_id = EXCLUDED.warehouse_id,
				active = TRUE
			RETURNING id`, companyID, whFGID).Scan(&terminalID)
		if err != nil {
			return fmt.Errorf("upsert pos_terminal: %w", err)
		}

		// POS Hardware Peripherals
		hardwareList := []struct {
			deviceType string
			deviceIP   string
			status     string
		}{
			{"THERMAL_PRINTER", "192.168.10.150", "ONLINE"},
			{"BARCODE_SCANNER", "192.168.10.151", "ONLINE"},
			{"CUSTOMER_DISPLAY", "192.168.10.152", "ONLINE"},
		}

		for _, h := range hardwareList {
			var hExists bool
			_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pos_hardware WHERE terminal_id = $1 AND device_type = $2)`, terminalID, h.deviceType).Scan(&hExists)
			if !hExists {
				if _, err := tx.Exec(ctx, `
					INSERT INTO pos_hardware (terminal_id, device_type, device_ip, status)
					VALUES ($1, $2, $3, $4)`,
					terminalID, h.deviceType, h.deviceIP, h.status); err != nil {
					return fmt.Errorf("insert pos_hardware %q: %w", h.deviceType, err)
				}
			}
		}

		// -------------------------------------------------------------------------
		// 2. POS Loyalty Members & Gift Cards
		// -------------------------------------------------------------------------
		loyaltyMembers := []struct {
			name   string
			phone  string
			points int64
			tier   string
		}{
			{"Ir. Bambang Trihatmojo", "+6281234567890", 1250, "PLATINUM"},
			{"Dr. Hendra Gunawan", "+6281398765432", 680, "GOLD"},
			{"Rahmat Hidayat, S.T.", "+6281755443322", 240, "STANDARD"},
		}

		for _, lm := range loyaltyMembers {
			var lmExists bool
			_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pos_loyalty_members WHERE company_id = $1 AND phone = $2)`, companyID, lm.phone).Scan(&lmExists)
			if !lmExists {
				if _, err := tx.Exec(ctx, `
					INSERT INTO pos_loyalty_members (company_id, customer_name, phone, points, tier, created_at)
					VALUES ($1, $2, $3, $4, $5, '2026-03-01 09:00:00+07')`,
					companyID, lm.name, lm.phone, lm.points, lm.tier); err != nil {
					return fmt.Errorf("insert pos_loyalty_member %q: %w", lm.name, err)
				}
			}
		}

		giftCards := []struct {
			code    string
			balance float64
			status  string
		}{
			{"GC-NTP-2026-500K", 500000.0, "ACTIVE"},
			{"GC-NTP-2026-1M", 1000000.0, "ACTIVE"},
		}

		for _, gc := range giftCards {
			var gcExists bool
			_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pos_gift_cards WHERE company_id = $1 AND code = $2)`, companyID, gc.code).Scan(&gcExists)
			if !gcExists {
				expDate := ParseDate("2026-12-31")
				if _, err := tx.Exec(ctx, `
					INSERT INTO pos_gift_cards (company_id, code, balance, currency, status, expires_at, created_at)
					VALUES ($1, $2, $3, 'IDR', $4, $5, '2026-03-01 09:00:00+07')`,
					companyID, gc.code, gc.balance, gc.status, expDate); err != nil {
					return fmt.Errorf("insert pos_gift_card %q: %w", gc.code, err)
				}
			}
		}

		// -------------------------------------------------------------------------
		// 3. Cashier Sessions (1 CLOSED Yesterday, 1 OPEN Today)
		// -------------------------------------------------------------------------
		// Session 1: Yesterday (2026-08-30) CLOSED
		var sess1ID int64
		sess1Opened := time.Date(2026, 8, 30, 8, 0, 0, 0, time.UTC)
		sess1Closed := time.Date(2026, 8, 30, 18, 0, 0, 0, time.UTC)
		sess1ClosingAmt := 12946000.00
		sess1Variance := 0.00

		err = tx.QueryRow(ctx, `
			SELECT id FROM pos_sessions
			WHERE company_id = $1 AND terminal_id = $2 AND opened_at::date = '2026-08-30'::date`,
			companyID, terminalID).Scan(&sess1ID)
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx, `
				INSERT INTO pos_sessions (
					company_id, terminal_id, cashier_id, opening_float, closing_amount,
					variance, status, opened_at, closed_at
				) VALUES (
					$1, $2, $3, 1000000.00, $4,
					$5, 'CLOSED', $6, $7
				) RETURNING id`,
				companyID, terminalID, cashierID, sess1ClosingAmt, sess1Variance, sess1Opened, sess1Closed).Scan(&sess1ID)
			if err != nil {
				return fmt.Errorf("insert pos_session 1: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("query pos_session 1: %w", err)
		}

		// Session 2: Today (2026-08-31) OPEN
		var sess2ID int64
		sess2Opened := time.Date(2026, 8, 31, 8, 30, 0, 0, time.UTC)

		err = tx.QueryRow(ctx, `
			SELECT id FROM pos_sessions
			WHERE company_id = $1 AND terminal_id = $2 AND opened_at::date = '2026-08-31'::date`,
			companyID, terminalID).Scan(&sess2ID)
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx, `
				INSERT INTO pos_sessions (
					company_id, terminal_id, cashier_id, opening_float, closing_amount,
					variance, status, opened_at, closed_at
				) VALUES (
					$1, $2, $3, 1000000.00, NULL,
					NULL, 'OPEN', $4, NULL
				) RETURNING id`,
				companyID, terminalID, cashierID, sess2Opened).Scan(&sess2ID)
			if err != nil {
				return fmt.Errorf("insert pos_session 2: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("query pos_session 2: %w", err)
		}

		// -------------------------------------------------------------------------
		// 4. POS Tickets, Lines & Payments (6 Completed/Refunded Tickets)
		// -------------------------------------------------------------------------
		type ticketLineDef struct {
			productSKU string
			qty        float64
			unitPrice  float64
			discount   float64
			taxAmt     float64
		}

		type ticketDef struct {
			number    string
			sessionID int64
			subtotal  float64
			taxAmt    float64
			total     float64
			status    string
			createdAt string
			lines     []ticketLineDef
			payMethod string
			payAmount float64
			payRef    string
		}

		tickets := []ticketDef{
			// Session 1 Tickets
			{
				number:    "POS-20260830-001",
				sessionID: sess1ID,
				subtotal:  2500000.00,
				taxAmt:    275000.00,
				total:     2775000.00,
				status:    "COMPLETED",
				createdAt: "2026-08-30 09:15:00",
				lines: []ticketLineDef{
					{"FG-IOT-FLT01", 2.0, 1250000.00, 0, 275000.00},
				},
				payMethod: "CASH",
				payAmount: 2775000.00,
				payRef:    "CASH-EXACT",
			},
			{
				number:    "POS-20260830-002",
				sessionID: sess1ID,
				subtotal:  1950000.00,
				taxAmt:    214500.00,
				total:     2164500.00,
				status:    "COMPLETED",
				createdAt: "2026-08-30 11:30:00",
				lines: []ticketLineDef{
					{"FG-IOT-PWR01", 1.0, 1950000.00, 0, 214500.00},
				},
				payMethod: "CARD",
				payAmount: 2164500.00,
				payRef:    "EDC-BCA-889123",
			},
			{
				number:    "POS-20260830-003",
				sessionID: sess1ID,
				subtotal:  6200000.00,
				taxAmt:    682000.00,
				total:     6882000.00,
				status:    "COMPLETED",
				createdAt: "2026-08-30 14:45:00",
				lines: []ticketLineDef{
					{"TRD-SW-IND08", 1.0, 6200000.00, 0, 682000.00},
				},
				payMethod: "CARD",
				payAmount: 6882000.00,
				payRef:    "EDC-MANDIRI-445112",
			},
			{
				number:    "POS-20260830-004",
				sessionID: sess1ID,
				subtotal:  1250000.00,
				taxAmt:    137500.00,
				total:     1387500.00,
				status:    "REFUNDED",
				createdAt: "2026-08-30 16:20:00",
				lines: []ticketLineDef{
					{"FG-IOT-FLT01", 1.0, 1250000.00, 0, 137500.00},
				},
				payMethod: "CASH",
				payAmount: 1387500.00,
				payRef:    "REFUND-CASH-RET",
			},

			// Session 2 Tickets
			{
				number:    "POS-20260831-001",
				sessionID: sess2ID,
				subtotal:  2450000.00,
				taxAmt:    269500.00,
				total:     2719500.00,
				status:    "COMPLETED",
				createdAt: "2026-08-31 09:45:00",
				lines: []ticketLineDef{
					{"FG-IOT-ENV01", 1.0, 2450000.00, 0, 269500.00},
				},
				payMethod: "CARD",
				payAmount: 2719500.00,
				payRef:    "EDC-BCA-990145",
			},
			{
				number:    "POS-20260831-002",
				sessionID: sess2ID,
				subtotal:  3400000.00,
				taxAmt:    374000.00,
				total:     3774000.00,
				status:    "COMPLETED",
				createdAt: "2026-08-31 11:15:00",
				lines: []ticketLineDef{
					{"TRD-UPS-IND01", 1.0, 3400000.00, 0, 374000.00},
				},
				payMethod: "CASH",
				payAmount: 3774000.00,
				payRef:    "CASH-EXACT",
			},
		}

		for _, t := range tickets {
			cTime, _ := time.Parse("2006-01-02 15:04:05", t.createdAt)

			var ticketID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO pos_tickets (
					company_id, session_id, number, currency, subtotal,
					tax_amount, total, status, created_by, created_at
				) VALUES (
					$1, $2, $3, 'IDR', $4,
					$5, $6, $7, $8, $9
				)
				ON CONFLICT (company_id, number) DO UPDATE SET
					subtotal = EXCLUDED.subtotal,
					tax_amount = EXCLUDED.tax_amount,
					total = EXCLUDED.total,
					status = EXCLUDED.status
				RETURNING id`,
				companyID, t.sessionID, t.number, t.subtotal,
				t.taxAmt, t.total, t.status, cashierID, cTime).Scan(&ticketID)
			if err != nil {
				return fmt.Errorf("upsert pos_ticket %q: %w", t.number, err)
			}

			// Insert Ticket Lines
			for _, line := range t.lines {
				prodID, err := getProductID(line.productSKU)
				if err != nil {
					return err
				}

				var lineExists bool
				_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pos_ticket_lines WHERE ticket_id = $1 AND product_id = $2)`, ticketID, prodID).Scan(&lineExists)
				if !lineExists {
					if _, err := tx.Exec(ctx, `
						INSERT INTO pos_ticket_lines (ticket_id, product_id, quantity, unit_price, discount, tax_amount)
						VALUES ($1, $2, $3, $4, $5, $6)`,
						ticketID, prodID, line.qty, line.unitPrice, line.discount, line.taxAmt); err != nil {
						return fmt.Errorf("insert pos_ticket_line for ticket %q: %w", t.number, err)
					}
				}
			}

			// Insert Payment
			idempotencyKey := fmt.Sprintf("POS-PAY-%s", t.number)
			var payExists bool
			_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pos_payments WHERE ticket_id = $1 AND idempotency_key = $2)`, ticketID, idempotencyKey).Scan(&payExists)
			if !payExists {
				if _, err := tx.Exec(ctx, `
					INSERT INTO pos_payments (ticket_id, method, amount, reference, idempotency_key)
					VALUES ($1, $2, $3, $4, $5)`,
					ticketID, t.payMethod, t.payAmount, t.payRef, idempotencyKey); err != nil {
					return fmt.Errorf("insert pos_payment for ticket %q: %w", t.number, err)
				}
			}
		}

		return nil
	})
}
