package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// seedPhase05Sales seeds quotations, sales orders, delivery orders, AR invoices,
// AR payments, return delivery orders, and AR credit notes for PT Nusantara Teknik Perkasa.
func seedPhase05Sales(ctx context.Context, sctx *SeedContext) error {
	return ExecInTx(ctx, sctx.Pool, "Phase 05: Sales (O2C)", func(tx pgx.Tx) error {
		adminID := sctx.UserIDs["budi.santoso@nusantarateknik.co.id"]
		salesManagerID := sctx.UserIDs["hendra.wijaya@nusantarateknik.co.id"]
		if salesManagerID == 0 {
			salesManagerID = adminID
		}
		warehouseLeadID := sctx.UserIDs["dewi.lestari@nusantarateknik.co.id"]
		if warehouseLeadID == 0 {
			warehouseLeadID = adminID
		}
		accountantID := sctx.UserIDs["siti.aminah@nusantarateknik.co.id"]
		if accountantID == 0 {
			accountantID = adminID
		}

		fgWHID := sctx.WarehouseIDs["WH-JKT-FG"]

		// -------------------------------------------------------------------------
		// 1. Quotations (10 Quotations: CONVERTED, APPROVED, REJECTED, DRAFT)
		// -------------------------------------------------------------------------
		type quoLineDef struct {
			productSKU  string
			description string
			qty         float64
			uom         string
			unitPrice   float64
			discountPct float64
			taxPct      float64
		}

		type quoDef struct {
			docNumber    string
			customerCode string
			quoteDate    string
			validUntil   string
			status       string
			currency     string
			notes        string
			rejected     bool
			rejectReason string
			lines        []quoLineDef
		}

		quotationsData := []quoDef{
			{
				docNumber:    "QUO-202603-0001",
				customerCode: "CUST-TELKOM",
				quoteDate:    "2026-03-05",
				validUntil:   "2026-04-05",
				status:       "CONVERTED",
				currency:     "IDR",
				notes:        "Official quotation for 50 IoT Gateway Pro & LoRa antennas",
				lines: []quoLineDef{
					{"FG-IOT-GW01", "Nusantara IoT Gateway Pro 4G/LoRaWAN", 50, "PCS", 3850000, 0, 11.00},
					{"RM-ANT-LORA", "Outdoor 915MHz 5.8dBi Fiberglass Antenna", 50, "PCS", 120000, 0, 11.00},
				},
			},
			{
				docNumber:    "QUO-202603-0002",
				customerCode: "CUST-PLN-NUS",
				quoteDate:    "2026-03-12",
				validUntil:   "2026-04-12",
				status:       "CONVERTED",
				currency:     "IDR",
				notes:        "Power meter and environmental monitors for substation telemetry",
				lines: []quoLineDef{
					{"FG-IOT-PWR01", "3-Phase Smart Power Meter Modbus RS485", 100, "PCS", 1950000, 0, 11.00},
					{"FG-IOT-ENV01", "Smart Environmental Monitor Industrial", 40, "PCS", 2450000, 0, 11.00},
				},
			},
			{
				docNumber:    "QUO-202604-0003",
				customerCode: "CUST-PAM-JAYA",
				quoteDate:    "2026-04-02",
				validUntil:   "2026-05-02",
				status:       "CONVERTED",
				currency:     "IDR",
				notes:        "Ultrasonic water level telemetry nodes for reservoir flood monitoring",
				lines: []quoLineDef{
					{"FG-IOT-WTR01", "Ultrasonic Water Level Sensor Transmitter", 80, "PCS", 2850000, 0, 11.00},
					{"FG-IOT-GW01", "Nusantara IoT Gateway Pro 4G/LoRaWAN", 20, "PCS", 3850000, 0, 11.00},
				},
			},
			{
				docNumber:    "QUO-202604-0004",
				customerCode: "CUST-ADARO",
				quoteDate:    "2026-04-18",
				validUntil:   "2026-05-18",
				status:       "CONVERTED",
				currency:     "IDR",
				notes:        "Mining heavy haul fleet OBD-II GPS telematics & edge servers",
				lines: []quoLineDef{
					{"FG-IOT-FLT01", "Fleet GPS Telematics OBD-II Tracker", 200, "PCS", 1250000, 0, 11.00},
					{"TRD-SVR-EDG01", "Advantech Industrial Edge Server IPC", 5, "PCS", 18500000, 0, 11.00},
				},
			},
			{
				docNumber:    "QUO-202605-0005",
				customerCode: "CUST-INDOFOOD",
				quoteDate:    "2026-05-10",
				validUntil:   "2026-06-10",
				status:       "APPROVED",
				currency:     "IDR",
				notes:        "Food processing environmental monitor mesh & industrial ethernet switches",
				lines: []quoLineDef{
					{"FG-IOT-ENV01", "Smart Environmental Monitor Industrial", 50, "PCS", 2450000, 0, 11.00},
					{"TRD-SW-IND08", "Moxa 8-Port Industrial Ethernet Switch", 10, "PCS", 6200000, 0, 11.00},
				},
			},
			{
				docNumber:    "QUO-202605-0006",
				customerCode: "CUST-ASTRA",
				quoteDate:    "2026-05-22",
				validUntil:   "2026-06-22",
				status:       "APPROVED",
				currency:     "IDR",
				notes:        "Plant 4 energy management smart power meters",
				lines: []quoLineDef{
					{"FG-IOT-PWR01", "3-Phase Smart Power Meter Modbus RS485", 80, "PCS", 1950000, 0, 11.00},
				},
			},
			{
				docNumber:    "QUO-202606-0007",
				customerCode: "CUST-JAK-PRO",
				quoteDate:    "2026-06-15",
				validUntil:   "2026-07-15",
				status:       "APPROVED",
				currency:     "IDR",
				notes:        "Smart city gateway & radar sensors for LRT infrastructure",
				lines: []quoLineDef{
					{"FG-IOT-GW01", "Nusantara IoT Gateway Pro 4G/LoRaWAN", 30, "PCS", 3850000, 0, 11.00},
					{"TRD-SEN-RAD01", "24GHz FMCW Radar Level Sensor 30m", 8, "PCS", 12500000, 0, 11.00},
				},
			},
			{
				docNumber:    "QUO-202607-0008",
				customerCode: "CUST-PETRO",
				quoteDate:    "2026-07-08",
				validUntil:   "2026-08-08",
				status:       "REJECTED",
				currency:     "IDR",
				notes:        "Industrial edge servers for chemical plant process monitoring",
				rejected:     true,
				rejectReason: "Client budget ceiling limit exceeded by 15%",
				lines: []quoLineDef{
					{"TRD-SVR-EDG01", "Advantech Industrial Edge Server IPC", 10, "PCS", 18500000, 0, 11.00},
				},
			},
			{
				docNumber:    "QUO-202608-0009",
				customerCode: "CUST-SMART-FARM",
				quoteDate:    "2026-08-05",
				validUntil:   "2026-09-05",
				status:       "DRAFT",
				currency:     "IDR",
				notes:        "Agricultural telemetry nodes for oil palm plantation trial",
				lines: []quoLineDef{
					{"FG-IOT-AGR01", "Soil & Weather Telemetry Node Solar", 60, "PCS", 1650000, 0, 11.00},
				},
			},
			{
				docNumber:    "QUO-202608-0010",
				customerCode: "CUST-LOG-TRANS",
				quoteDate:    "2026-08-12",
				validUntil:   "2026-09-12",
				status:       "DRAFT",
				currency:     "IDR",
				notes:        "Fleet telematics units for logistics freight trucks",
				lines: []quoLineDef{
					{"FG-IOT-FLT01", "Fleet GPS Telematics OBD-II Tracker", 120, "PCS", 1250000, 0, 11.00},
				},
			},
		}

		quotationIDs := make(map[string]int64)

		for _, q := range quotationsData {
			custID := sctx.CustomerIDs[q.customerCode]
			qDate := ParseDate(q.quoteDate)
			vDate := ParseDate(q.validUntil)

			var subtotal, taxAmount, total float64
			for _, l := range q.lines {
				sub := l.qty * l.unitPrice
				disc := sub * (l.discountPct / 100.0)
				taxable := sub - disc
				tax := taxable * (l.taxPct / 100.0)
				subtotal += taxable
				taxAmount += tax
				total += (taxable + tax)
			}

			var approvedBy *int64
			var approvedAt *time.Time
			if q.status == "APPROVED" || q.status == "CONVERTED" {
				approvedBy = &salesManagerID
				appTime := qDate.Add(24 * time.Hour)
				approvedAt = &appTime
			}

			var rejectedBy *int64
			var rejectedAt *time.Time
			var rejectReason *string
			if q.rejected {
				rejectedBy = &salesManagerID
				rejTime := qDate.Add(48 * time.Hour)
				rejectedAt = &rejTime
				rejectReason = &q.rejectReason
			}

			var quoID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO quotations (
					doc_number, company_id, customer_id, quote_date, valid_until,
					status, currency, subtotal, tax_amount, total_amount, notes,
					created_by, approved_by, approved_at, rejected_by, rejected_at, rejection_reason,
					created_at, updated_at
				) VALUES (
					$1, $2, $3, $4::date, $5::date,
					$6, $7, $8, $9, $10, $11,
					$12, $13, $14, $15, $16, $17,
					$4::timestamptz, $4::timestamptz
				)
				ON CONFLICT (doc_number) DO UPDATE SET
					customer_id = EXCLUDED.customer_id,
					quote_date = EXCLUDED.quote_date,
					valid_until = EXCLUDED.valid_until,
					status = EXCLUDED.status,
					currency = EXCLUDED.currency,
					subtotal = EXCLUDED.subtotal,
					tax_amount = EXCLUDED.tax_amount,
					total_amount = EXCLUDED.total_amount,
					notes = EXCLUDED.notes,
					approved_by = EXCLUDED.approved_by,
					approved_at = EXCLUDED.approved_at,
					rejected_by = EXCLUDED.rejected_by,
					rejected_at = EXCLUDED.rejected_at,
					rejection_reason = EXCLUDED.rejection_reason,
					updated_at = NOW()
				RETURNING id`,
				q.docNumber, sctx.CompanyNTPID, custID, qDate, vDate,
				q.status, q.currency, subtotal, taxAmount, total, q.notes,
				salesManagerID, approvedBy, approvedAt, rejectedBy, rejectedAt, rejectReason).Scan(&quoID)
			if err != nil {
				return fmt.Errorf("upsert quotation %q: %w", q.docNumber, err)
			}
			quotationIDs[q.docNumber] = quoID

			for i, l := range q.lines {
				prodID := sctx.ProductIDs[l.productSKU]
				sub := l.qty * l.unitPrice
				disc := sub * (l.discountPct / 100.0)
				taxable := sub - disc
				tax := taxable * (l.taxPct / 100.0)
				lineTot := taxable + tax

				var lineID int64
				err := tx.QueryRow(ctx, `
					SELECT id FROM quotation_lines WHERE quotation_id = $1 AND product_id = $2`,
					quoID, prodID).Scan(&lineID)
				if err != nil {
					err = tx.QueryRow(ctx, `
						INSERT INTO quotation_lines (
							quotation_id, product_id, description, quantity, uom,
							unit_price, discount_percent, discount_amount, tax_percent, tax_amount,
							line_total, notes, line_order, created_at, updated_at
						) VALUES (
							$1, $2, $3, $4, $5,
							$6, $7, $8, $9, $10,
							$11, '', $12, $13, $13
						) RETURNING id`,
						quoID, prodID, l.description, l.qty, l.uom,
						l.unitPrice, l.discountPct, disc, l.taxPct, tax,
						lineTot, i+1, qDate).Scan(&lineID)
				} else {
					_, err = tx.Exec(ctx, `
						UPDATE quotation_lines SET
							description = $2, quantity = $3, uom = $4, unit_price = $5,
							discount_percent = $6, discount_amount = $7, tax_percent = $8, tax_amount = $9, line_total = $10
						WHERE id = $1`,
						lineID, l.description, l.qty, l.uom, l.unitPrice,
						l.discountPct, disc, l.taxPct, tax, lineTot)
				}
				if err != nil {
					return fmt.Errorf("upsert quotation line: %w", err)
				}
			}
		}

		// -------------------------------------------------------------------------
		// 2. Sales Orders (10 Sales Orders across full lifecycle)
		// -------------------------------------------------------------------------
		type soLineDef struct {
			productSKU   string
			description  string
			qty          float64
			qtyDelivered float64
			qtyInvoiced  float64
			uom          string
			unitPrice    float64
			taxPct       float64
		}

		type soDef struct {
			docNumber    string
			customerCode string
			quoDocNumber string
			orderDate    string
			expDelivery  string
			status       string
			currency     string
			notes        string
			confirmed    bool
			cancelled    bool
			cancelReason string
			lines        []soLineDef
		}

		salesOrdersData := []soDef{
			{
				docNumber:    "SO-202603-0001",
				customerCode: "CUST-TELKOM",
				quoDocNumber: "QUO-202603-0001",
				orderDate:    "2026-03-10",
				expDelivery:  "2026-03-25",
				status:       "COMPLETED",
				currency:     "IDR",
				notes:        "Telkom Smart Grid Phase 1 project order",
				confirmed:    true,
				lines: []soLineDef{
					{"FG-IOT-GW01", "Nusantara IoT Gateway Pro 4G/LoRaWAN", 50, 50, 50, "PCS", 3850000, 11.00},
					{"RM-ANT-LORA", "Outdoor 915MHz 5.8dBi Fiberglass Antenna", 50, 50, 50, "PCS", 120000, 11.00},
				},
			},
			{
				docNumber:    "SO-202603-0002",
				customerCode: "CUST-PLN-NUS",
				quoDocNumber: "QUO-202603-0002",
				orderDate:    "2026-03-18",
				expDelivery:  "2026-03-30",
				status:       "COMPLETED",
				currency:     "IDR",
				notes:        "PLN Ketintang Substation power meter rollout",
				confirmed:    true,
				lines: []soLineDef{
					{"FG-IOT-PWR01", "3-Phase Smart Power Meter Modbus RS485", 100, 100, 100, "PCS", 1950000, 11.00},
					{"FG-IOT-ENV01", "Smart Environmental Monitor Industrial", 40, 40, 40, "PCS", 2450000, 11.00},
				},
			},
			{
				docNumber:    "SO-202604-0003",
				customerCode: "CUST-PAM-JAYA",
				quoDocNumber: "QUO-202604-0003",
				orderDate:    "2026-04-08",
				expDelivery:  "2026-04-20",
				status:       "COMPLETED",
				currency:     "IDR",
				notes:        "PAM Jaya reservoir level monitoring project",
				confirmed:    true,
				lines: []soLineDef{
					{"FG-IOT-WTR01", "Ultrasonic Water Level Sensor Transmitter", 80, 80, 80, "PCS", 2850000, 11.00},
					{"FG-IOT-GW01", "Nusantara IoT Gateway Pro 4G/LoRaWAN", 20, 20, 20, "PCS", 3850000, 11.00},
				},
			},
			{
				docNumber:    "SO-202604-0004",
				customerCode: "CUST-ADARO",
				quoDocNumber: "QUO-202604-0004",
				orderDate:    "2026-04-22",
				expDelivery:  "2026-05-05",
				status:       "COMPLETED",
				currency:     "IDR",
				notes:        "Adaro coal mining telematics & edge computing",
				confirmed:    true,
				lines: []soLineDef{
					{"FG-IOT-FLT01", "Fleet GPS Telematics OBD-II Tracker", 200, 200, 200, "PCS", 1250000, 11.00},
					{"TRD-SVR-EDG01", "Advantech Industrial Edge Server IPC", 5, 5, 5, "PCS", 18500000, 11.00},
				},
			},
			{
				docNumber:    "SO-202605-0005",
				customerCode: "CUST-INDOFOOD",
				quoDocNumber: "QUO-202605-0005",
				orderDate:    "2026-05-18",
				expDelivery:  "2026-06-05",
				status:       "CONFIRMED",
				currency:     "IDR",
				notes:        "Indofood CBP food processing environmental monitors",
				confirmed:    true,
				lines: []soLineDef{
					{"FG-IOT-ENV01", "Smart Environmental Monitor Industrial", 50, 0, 0, "PCS", 2450000, 11.00},
					{"TRD-SW-IND08", "Moxa 8-Port Industrial Ethernet Switch", 10, 0, 0, "PCS", 6200000, 11.00},
				},
			},
			{
				docNumber:    "SO-202605-0006",
				customerCode: "CUST-ASTRA",
				quoDocNumber: "QUO-202605-0006",
				orderDate:    "2026-05-28",
				expDelivery:  "2026-06-15",
				status:       "PROCESSING",
				currency:     "IDR",
				notes:        "Astra Plant 4 energy management partial delivery",
				confirmed:    true,
				lines: []soLineDef{
					{"FG-IOT-PWR01", "3-Phase Smart Power Meter Modbus RS485", 80, 40, 40, "PCS", 1950000, 11.00},
				},
			},
			{
				docNumber:    "SO-202606-0007",
				customerCode: "CUST-JAK-PRO",
				quoDocNumber: "QUO-202606-0007",
				orderDate:    "2026-06-20",
				expDelivery:  "2026-07-10",
				status:       "CONFIRMED",
				currency:     "IDR",
				notes:        "Jakarta Propertindo LRT smart infrastructure",
				confirmed:    true,
				lines: []soLineDef{
					{"FG-IOT-GW01", "Nusantara IoT Gateway Pro 4G/LoRaWAN", 30, 0, 0, "PCS", 3850000, 11.00},
					{"TRD-SEN-RAD01", "24GHz FMCW Radar Level Sensor 30m", 8, 0, 0, "PCS", 12500000, 11.00},
				},
			},
			{
				docNumber:    "SO-202607-0008",
				customerCode: "CUST-PETRO",
				orderDate:    "2026-07-15",
				expDelivery:  "2026-08-15",
				status:       "DRAFT",
				currency:     "IDR",
				notes:        "Petrokimia Gresik UPS backup systems proposal order",
				lines: []soLineDef{
					{"TRD-UPS-IND01", "Din-Rail DC-UPS 24V 10A Back-up Module", 25, 0, 0, "PCS", 3400000, 11.00},
				},
			},
			{
				docNumber:    "SO-202608-0009",
				customerCode: "CUST-SMART-FARM",
				orderDate:    "2026-08-10",
				expDelivery:  "2026-08-25",
				status:       "DRAFT",
				currency:     "IDR",
				notes:        "Nusantara Smart Agri pilot project order",
				lines: []soLineDef{
					{"FG-IOT-AGR01", "Soil & Weather Telemetry Node Solar", 40, 0, 0, "PCS", 1650000, 11.00},
				},
			},
			{
				docNumber:    "SO-202608-0010",
				customerCode: "CUST-LOG-TRANS",
				orderDate:    "2026-08-02",
				expDelivery:  "2026-08-18",
				status:       "CANCELLED",
				currency:     "IDR",
				notes:        "Cancelled due to customer fleet restructuring",
				cancelled:    true,
				cancelReason: "Customer requested scope and hardware cancellation",
				lines: []soLineDef{
					{"FG-IOT-FLT01", "Fleet GPS Telematics OBD-II Tracker", 50, 0, 0, "PCS", 1250000, 11.00},
				},
			},
		}

		salesOrderIDs := make(map[string]int64)
		salesOrderLineIDs := make(map[string]int64) // "SO_DOC:SKU" -> so_line_id

		for _, s := range salesOrdersData {
			custID := sctx.CustomerIDs[s.customerCode]
			var quoID *int64
			if id, ok := quotationIDs[s.quoDocNumber]; ok {
				quoID = &id
			}
			oDate := ParseDate(s.orderDate)
			expDate := ParseDate(s.expDelivery)

			var subtotal, taxAmount, total float64
			for _, l := range s.lines {
				sub := l.qty * l.unitPrice
				tax := sub * (l.taxPct / 100.0)
				subtotal += sub
				taxAmount += tax
				total += (sub + tax)
			}

			var confirmedBy *int64
			var confirmedAt *time.Time
			if s.confirmed {
				confirmedBy = &salesManagerID
				cTime := oDate.Add(24 * time.Hour)
				confirmedAt = &cTime
			}

			var cancelledBy *int64
			var cancelledAt *time.Time
			var cancelReason *string
			if s.cancelled {
				cancelledBy = &salesManagerID
				cnTime := oDate.Add(48 * time.Hour)
				cancelledAt = &cnTime
				cancelReason = &s.cancelReason
			}

			var soID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO sales_orders (
					doc_number, company_id, customer_id, quotation_id, order_date, expected_delivery_date,
					status, currency, subtotal, tax_amount, total_amount, notes,
					created_by, confirmed_by, confirmed_at, cancelled_by, cancelled_at, cancellation_reason,
					created_at, updated_at
				) VALUES (
					$1, $2, $3, $4, $5::date, $6::date,
					$7, $8, $9, $10, $11, $12,
					$13, $14, $15, $16, $17, $18,
					$5::timestamptz, $5::timestamptz
				)
				ON CONFLICT (doc_number) DO UPDATE SET
					customer_id = EXCLUDED.customer_id,
					quotation_id = EXCLUDED.quotation_id,
					order_date = EXCLUDED.order_date,
					expected_delivery_date = EXCLUDED.expected_delivery_date,
					status = EXCLUDED.status,
					currency = EXCLUDED.currency,
					subtotal = EXCLUDED.subtotal,
					tax_amount = EXCLUDED.tax_amount,
					total_amount = EXCLUDED.total_amount,
					notes = EXCLUDED.notes,
					confirmed_by = EXCLUDED.confirmed_by,
					confirmed_at = EXCLUDED.confirmed_at,
					cancelled_by = EXCLUDED.cancelled_by,
					cancelled_at = EXCLUDED.cancelled_at,
					cancellation_reason = EXCLUDED.cancellation_reason,
					updated_at = NOW()
				RETURNING id`,
				s.docNumber, sctx.CompanyNTPID, custID, quoID, oDate, expDate,
				s.status, s.currency, subtotal, taxAmount, total, s.notes,
				salesManagerID, confirmedBy, confirmedAt, cancelledBy, cancelledAt, cancelReason).Scan(&soID)
			if err != nil {
				return fmt.Errorf("upsert sales order %q: %w", s.docNumber, err)
			}
			salesOrderIDs[s.docNumber] = soID

			for i, l := range s.lines {
				prodID := sctx.ProductIDs[l.productSKU]
				sub := l.qty * l.unitPrice
				tax := sub * (l.taxPct / 100.0)
				lineTot := sub + tax

				var lineID int64
				err := tx.QueryRow(ctx, `
					SELECT id FROM sales_order_lines WHERE sales_order_id = $1 AND product_id = $2`,
					soID, prodID).Scan(&lineID)
				if err != nil {
					err = tx.QueryRow(ctx, `
						INSERT INTO sales_order_lines (
							sales_order_id, product_id, description, quantity, quantity_delivered, quantity_invoiced,
							uom, unit_price, discount_percent, discount_amount, tax_percent, tax_amount,
							line_total, notes, line_order, created_at, updated_at
						) VALUES (
							$1, $2, $3, $4, $5, $6,
							$7, $8, 0, 0, $9, $10,
							$11, '', $12, $13, $13
						) RETURNING id`,
						soID, prodID, l.description, l.qty, l.qtyDelivered, l.qtyInvoiced,
						l.uom, l.unitPrice, l.taxPct, tax,
						lineTot, i+1, oDate).Scan(&lineID)
				} else {
					_, err = tx.Exec(ctx, `
						UPDATE sales_order_lines SET
							description = $2, quantity = $3, quantity_delivered = $4, quantity_invoiced = $5,
							uom = $6, unit_price = $7, tax_percent = $8, tax_amount = $9, line_total = $10
						WHERE id = $1`,
						lineID, l.description, l.qty, l.qtyDelivered, l.qtyInvoiced,
						l.uom, l.unitPrice, l.taxPct, tax, lineTot)
				}
				if err != nil {
					return fmt.Errorf("upsert sales order line: %w", err)
				}
				salesOrderLineIDs[fmt.Sprintf("%s:%s", s.docNumber, l.productSKU)] = lineID
			}
		}

		// -------------------------------------------------------------------------
		// 3. Delivery Orders (7 Delivery Orders across full lifecycle)
		// -------------------------------------------------------------------------
		type doLineDef struct {
			soKey       string // "SO_DOC:SKU"
			productSKU  string
			qtyToDeliv  float64
			qtyDeliv    float64
			uom         string
			unitPrice   float64
		}

		type doDef struct {
			docNumber    string
			soDocNumber  string
			customerCode string
			deliveryDate string
			status       string
			driverName   string
			vehicleNum   string
			trackingNum  string
			delivered    bool
			lines        []doLineDef
		}

		deliveryOrdersData := []doDef{
			{
				docNumber:    "DO-202603-00001",
				soDocNumber:  "SO-202603-0001",
				customerCode: "CUST-TELKOM",
				deliveryDate: "2026-03-22",
				status:       "DELIVERED",
				driverName:   "Ahmad Supriyadi",
				vehicleNum:   "B 9123 TXR",
				trackingNum:  "TRK-NTP-202603-01",
				delivered:    true,
				lines: []doLineDef{
					{"SO-202603-0001:FG-IOT-GW01", "FG-IOT-GW01", 50, 50, "PCS", 3850000},
					{"SO-202603-0001:RM-ANT-LORA", "RM-ANT-LORA", 50, 50, "PCS", 120000},
				},
			},
			{
				docNumber:    "DO-202603-00002",
				soDocNumber:  "SO-202603-0002",
				customerCode: "CUST-PLN-NUS",
				deliveryDate: "2026-03-28",
				status:       "DELIVERED",
				driverName:   "Bambang Setiawan",
				vehicleNum:   "B 9456 TYU",
				trackingNum:  "TRK-NTP-202603-02",
				delivered:    true,
				lines: []doLineDef{
					{"SO-202603-0002:FG-IOT-PWR01", "FG-IOT-PWR01", 100, 100, "PCS", 1950000},
					{"SO-202603-0002:FG-IOT-ENV01", "FG-IOT-ENV01", 40, 40, "PCS", 2450000},
				},
			},
			{
				docNumber:    "DO-202604-00003",
				soDocNumber:  "SO-202604-0003",
				customerCode: "CUST-PAM-JAYA",
				deliveryDate: "2026-04-18",
				status:       "DELIVERED",
				driverName:   "Ahmad Supriyadi",
				vehicleNum:   "B 9123 TXR",
				trackingNum:  "TRK-NTP-202604-03",
				delivered:    true,
				lines: []doLineDef{
					{"SO-202604-0003:FG-IOT-WTR01", "FG-IOT-WTR01", 80, 80, "PCS", 2850000},
					{"SO-202604-0003:FG-IOT-GW01", "FG-IOT-GW01", 20, 20, "PCS", 3850000},
				},
			},
			{
				docNumber:    "DO-202604-00004",
				soDocNumber:  "SO-202604-0004",
				customerCode: "CUST-ADARO",
				deliveryDate: "2026-04-30",
				status:       "DELIVERED",
				driverName:   "Bambang Setiawan",
				vehicleNum:   "B 9456 TYU",
				trackingNum:  "TRK-NTP-202604-04",
				delivered:    true,
				lines: []doLineDef{
					{"SO-202604-0004:FG-IOT-FLT01", "FG-IOT-FLT01", 200, 200, "PCS", 1250000},
					{"SO-202604-0004:TRD-SVR-EDG01", "TRD-SVR-EDG01", 5, 5, "PCS", 18500000},
				},
			},
			{
				// In-transit DO
				docNumber:    "DO-202605-00005",
				soDocNumber:  "SO-202605-0006",
				customerCode: "CUST-ASTRA",
				deliveryDate: "2026-05-30",
				status:       "IN_TRANSIT",
				driverName:   "Ahmad Supriyadi",
				vehicleNum:   "B 9123 TXR",
				trackingNum:  "TRK-NTP-202605-05",
				delivered:    false,
				lines: []doLineDef{
					{"SO-202605-0006:FG-IOT-PWR01", "FG-IOT-PWR01", 40, 40, "PCS", 1950000},
				},
			},
			{
				// Draft DO
				docNumber:    "DO-202606-00006",
				soDocNumber:  "SO-202606-0007",
				customerCode: "CUST-JAK-PRO",
				deliveryDate: "2026-06-25",
				status:       "DRAFT",
				driverName:   "Bambang Setiawan",
				vehicleNum:   "B 9456 TYU",
				trackingNum:  "",
				delivered:    false,
				lines: []doLineDef{
					{"SO-202606-0007:FG-IOT-GW01", "FG-IOT-GW01", 30, 0, "PCS", 3850000},
					{"SO-202606-0007:TRD-SEN-RAD01", "TRD-SEN-RAD01", 8, 0, "PCS", 12500000},
				},
			},
			{
				// Cancelled DO
				docNumber:    "DO-202607-00007",
				soDocNumber:  "SO-202605-0005",
				customerCode: "CUST-INDOFOOD",
				deliveryDate: "2026-07-05",
				status:       "CANCELLED",
				driverName:   "",
				vehicleNum:   "",
				trackingNum:  "",
				delivered:    false,
				lines: []doLineDef{
					{"SO-202605-0005:FG-IOT-ENV01", "FG-IOT-ENV01", 50, 0, "PCS", 2450000},
					{"SO-202605-0005:TRD-SW-IND08", "TRD-SW-IND08", 10, 0, "PCS", 6200000},
				},
			},
		}

		deliveryOrderIDs := make(map[string]int64)
		deliveryOrderLineIDs := make(map[string]int64) // "DO_DOC:SKU" -> do_line_id

		for _, d := range deliveryOrdersData {
			soID := salesOrderIDs[d.soDocNumber]
			custID := sctx.CustomerIDs[d.customerCode]
			dDate := ParseDate(d.deliveryDate)

			var confirmedBy *int64
			var confirmedAt *time.Time
			var deliveredAt *time.Time
			if d.status != "DRAFT" && d.status != "CANCELLED" {
				confirmedBy = &warehouseLeadID
				cfTime := dDate.Add(4 * time.Hour)
				confirmedAt = &cfTime
				if d.delivered {
					delTime := dDate.Add(8 * time.Hour)
					deliveredAt = &delTime
				}
			}

			var doID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO delivery_orders (
					doc_number, company_id, sales_order_id, warehouse_id, customer_id,
					delivery_date, status, driver_name, vehicle_number, tracking_number, notes,
					created_by, confirmed_by, confirmed_at, delivered_at,
					created_at, updated_at
				) VALUES (
					$1, $2, $3, $4, $5,
					$6::date, $7, $8, $9, $10, 'Standard delivery dispatch',
					$11, $12, $13, $14,
					$6::timestamptz, $6::timestamptz
				)
				ON CONFLICT (doc_number) DO UPDATE SET
					sales_order_id = EXCLUDED.sales_order_id,
					warehouse_id = EXCLUDED.warehouse_id,
					customer_id = EXCLUDED.customer_id,
					delivery_date = EXCLUDED.delivery_date,
					status = EXCLUDED.status,
					driver_name = EXCLUDED.driver_name,
					vehicle_number = EXCLUDED.vehicle_number,
					tracking_number = EXCLUDED.tracking_number,
					confirmed_by = EXCLUDED.confirmed_by,
					confirmed_at = EXCLUDED.confirmed_at,
					delivered_at = EXCLUDED.delivered_at,
					updated_at = NOW()
				RETURNING id`,
				d.docNumber, sctx.CompanyNTPID, soID, fgWHID, custID,
				dDate, d.status, d.driverName, d.vehicleNum, d.trackingNum,
				warehouseLeadID, confirmedBy, confirmedAt, deliveredAt).Scan(&doID)
			if err != nil {
				return fmt.Errorf("upsert delivery order %q: %w", d.docNumber, err)
			}
			deliveryOrderIDs[d.docNumber] = doID

			for i, l := range d.lines {
				soLineID := salesOrderLineIDs[l.soKey]
				prodID := sctx.ProductIDs[l.productSKU]

				var doLineID int64
				err := tx.QueryRow(ctx, `
					SELECT id FROM delivery_order_lines WHERE delivery_order_id = $1 AND product_id = $2`,
					doID, prodID).Scan(&doLineID)
				if err != nil {
					err = tx.QueryRow(ctx, `
						INSERT INTO delivery_order_lines (
							delivery_order_id, sales_order_line_id, product_id,
							quantity_to_deliver, quantity_delivered, uom, unit_price,
							notes, line_order, created_at, updated_at
						) VALUES (
							$1, $2, $3,
							$4, $5, $6, $7,
							'', $8, $9, $9
						) RETURNING id`,
						doID, soLineID, prodID,
						l.qtyToDeliv, l.qtyDeliv, l.uom, l.unitPrice,
						i+1, dDate).Scan(&doLineID)
				} else {
					_, err = tx.Exec(ctx, `
						UPDATE delivery_order_lines SET
							sales_order_line_id = $2, quantity_to_deliver = $3, quantity_delivered = $4,
							uom = $5, unit_price = $6
						WHERE id = $1`,
						doLineID, soLineID, l.qtyToDeliv, l.qtyDeliv, l.uom, l.unitPrice)
				}
				if err != nil {
					return fmt.Errorf("upsert delivery order line: %w", err)
				}
				deliveryOrderLineIDs[fmt.Sprintf("%s:%s", d.docNumber, l.productSKU)] = doLineID
			}
		}

		// -------------------------------------------------------------------------
		// 4. AR Invoices (8 Invoices across full lifecycle — some overdue for aging)
		// -------------------------------------------------------------------------
		type arInvLineDef struct {
			doKey       string // "DO_DOC:SKU"
			productSKU  string
			description string
			qty         float64
			unitPrice   float64
			taxPct      float64
		}

		type arInvDef struct {
			number       string
			customerCode string
			soDocNumber  string
			doDocNumber  string
			status       string
			issuedAt     string
			dueAt        string
			posted       bool
			voided       bool
			voidReason   string
			lines        []arInvLineDef
		}

		arInvoicesData := []arInvDef{
			{
				number:       "INV-202603-00001",
				customerCode: "CUST-TELKOM",
				soDocNumber:  "SO-202603-0001",
				doDocNumber:  "DO-202603-00001",
				status:       "PAID",
				issuedAt:     "2026-03-25",
				dueAt:        "2026-05-09",
				posted:       true,
				lines: []arInvLineDef{
					{"DO-202603-00001:FG-IOT-GW01", "FG-IOT-GW01", "Nusantara IoT Gateway Pro 4G/LoRaWAN 50 PCS", 50, 3850000, 11.00},
					{"DO-202603-00001:RM-ANT-LORA", "RM-ANT-LORA", "Outdoor 915MHz Fiberglass Antennas 50 PCS", 50, 120000, 11.00},
				},
			},
			{
				number:       "INV-202603-00002",
				customerCode: "CUST-PLN-NUS",
				soDocNumber:  "SO-202603-0002",
				doDocNumber:  "DO-202603-00002",
				status:       "PAID",
				issuedAt:     "2026-03-30",
				dueAt:        "2026-05-29",
				posted:       true,
				lines: []arInvLineDef{
					{"DO-202603-00002:FG-IOT-PWR01", "FG-IOT-PWR01", "3-Phase Smart Power Meter Modbus RS485 100 PCS", 100, 1950000, 11.00},
					{"DO-202603-00002:FG-IOT-ENV01", "FG-IOT-ENV01", "Smart Environmental Monitor Industrial 40 PCS", 40, 2450000, 11.00},
				},
			},
			{
				number:       "INV-202604-00003",
				customerCode: "CUST-PAM-JAYA",
				soDocNumber:  "SO-202604-0003",
				doDocNumber:  "DO-202604-00003",
				status:       "PAID",
				issuedAt:     "2026-04-20",
				dueAt:        "2026-05-20",
				posted:       true,
				lines: []arInvLineDef{
					{"DO-202604-00003:FG-IOT-WTR01", "FG-IOT-WTR01", "Ultrasonic Water Level Sensor Transmitter 80 PCS", 80, 2850000, 11.00},
					{"DO-202604-00003:FG-IOT-GW01", "FG-IOT-GW01", "Nusantara IoT Gateway Pro 4G/LoRaWAN 20 PCS", 20, 3850000, 11.00},
				},
			},
			{
				// Overdue AR Invoice (>90 days overdue)
				number:       "INV-202604-00004",
				customerCode: "CUST-ADARO",
				soDocNumber:  "SO-202604-0004",
				doDocNumber:  "DO-202604-00004",
				status:       "POSTED",
				issuedAt:     "2026-04-30",
				dueAt:        "2026-05-30",
				posted:       true,
				lines: []arInvLineDef{
					{"DO-202604-00004:FG-IOT-FLT01", "FG-IOT-FLT01", "Fleet GPS Telematics OBD-II Tracker 200 PCS", 200, 1250000, 11.00},
					{"DO-202604-00004:TRD-SVR-EDG01", "TRD-SVR-EDG01", "Advantech Industrial Edge Server IPC 5 PCS", 5, 18500000, 11.00},
				},
			},
			{
				// Overdue AR Invoice (>60 days overdue)
				number:       "INV-202605-00005",
				customerCode: "CUST-ASTRA",
				soDocNumber:  "SO-202605-0006",
				doDocNumber:  "DO-202605-00005",
				status:       "POSTED",
				issuedAt:     "2026-05-31",
				dueAt:        "2026-06-30",
				posted:       true,
				lines: []arInvLineDef{
					{"DO-202605-00005:FG-IOT-PWR01", "FG-IOT-PWR01", "3-Phase Smart Power Meter Modbus RS485 40 PCS", 40, 1950000, 11.00},
				},
			},
			{
				// Current AR Invoice (Not overdue)
				number:       "INV-202606-00006",
				customerCode: "CUST-JAK-PRO",
				soDocNumber:  "SO-202606-0007",
				status:       "POSTED",
				issuedAt:     "2026-06-22",
				dueAt:        "2026-08-05",
				posted:       true,
				lines: []arInvLineDef{
					{"", "FG-IOT-GW01", "Nusantara IoT Gateway Pro 4G/LoRaWAN 30 PCS", 30, 3850000, 11.00},
					{"", "TRD-SEN-RAD01", "24GHz FMCW Radar Level Sensor 8 PCS", 8, 12500000, 11.00},
				},
			},
			{
				// Draft AR Invoice
				number:       "INV-202608-00007",
				customerCode: "CUST-SMART-FARM",
				soDocNumber:  "SO-202608-0009",
				status:       "DRAFT",
				issuedAt:     "2026-08-15",
				dueAt:        "2026-08-29",
				posted:       false,
				lines: []arInvLineDef{
					{"", "FG-IOT-AGR01", "Soil & Weather Telemetry Node Solar 40 PCS", 40, 1650000, 11.00},
				},
			},
			{
				// Voided AR Invoice
				number:       "INV-202608-00008",
				customerCode: "CUST-LOG-TRANS",
				soDocNumber:  "SO-202608-0010",
				status:       "VOID",
				issuedAt:     "2026-08-05",
				dueAt:        "2026-08-19",
				posted:       false,
				voided:       true,
				voidReason:   "Customer sales order cancelled prior to factory dispatch",
				lines: []arInvLineDef{
					{"", "FG-IOT-FLT01", "Fleet GPS Telematics OBD-II Tracker 50 PCS (Cancelled)", 50, 1250000, 11.00},
				},
			},
		}

		arInvoiceIDs := make(map[string]int64)
		arInvoiceLineIDs := make(map[string]int64) // "INV_NUM:SKU" -> ar_line_id

		for _, inv := range arInvoicesData {
			custID := sctx.CustomerIDs[inv.customerCode]
			var soID *int64
			if id, ok := salesOrderIDs[inv.soDocNumber]; ok {
				soID = &id
			}
			var doID *int64
			if id, ok := deliveryOrderIDs[inv.doDocNumber]; ok {
				doID = &id
			}

			issueDate := ParseDate(inv.issuedAt)
			dueDate := ParseDate(inv.dueAt)

			var subtotal, taxAmount, total float64
			for _, l := range inv.lines {
				sub := l.qty * l.unitPrice
				tax := sub * (l.taxPct / 100.0)
				subtotal += sub
				taxAmount += tax
				total += (sub + tax)
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

			var arInvID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO ar_invoices (
					number, customer_id, so_id, delivery_order_id, currency,
					subtotal, tax_amount, total, status, due_at,
					posted_at, posted_by, voided_at, voided_by, void_reason, created_by,
					created_at, updated_at,
					base_currency, original_currency_amount, base_amount, fx_rate, fx_rate_date, fx_rate_source, fx_rate_locked_at
				) VALUES (
					$1, $2, $3, $4, 'IDR',
					$5, $6, $7, $8, $9::timestamptz,
					$10::timestamptz, $11, $12::timestamptz, $13, $14, $15,
					$16::timestamptz, $16::timestamptz,
					'IDR', $7, $7, 1.0, $16::date, 'BANK_INDONESIA', $10::timestamptz
				)
				ON CONFLICT (number) DO UPDATE SET
					customer_id = EXCLUDED.customer_id,
					so_id = EXCLUDED.so_id,
					delivery_order_id = EXCLUDED.delivery_order_id,
					subtotal = EXCLUDED.subtotal,
					tax_amount = EXCLUDED.tax_amount,
					total = EXCLUDED.total,
					status = EXCLUDED.status,
					due_at = EXCLUDED.due_at,
					posted_at = EXCLUDED.posted_at,
					posted_by = EXCLUDED.posted_by,
					voided_at = EXCLUDED.voided_at,
					voided_by = EXCLUDED.voided_by,
					void_reason = EXCLUDED.void_reason,
					base_amount = EXCLUDED.base_amount,
					updated_at = NOW()
				RETURNING id`,
				inv.number, custID, soID, doID,
				subtotal, taxAmount, total, inv.status, dueDate,
				postedAt, postedBy, voidedAt, voidedBy, voidReason, accountantID,
				issueDate).Scan(&arInvID)
			if err != nil {
				return fmt.Errorf("upsert AR invoice %q: %w", inv.number, err)
			}
			arInvoiceIDs[inv.number] = arInvID

			for _, l := range inv.lines {
				prodID := sctx.ProductIDs[l.productSKU]
				var doLineID *int64
				if id, ok := deliveryOrderLineIDs[l.doKey]; ok {
					doLineID = &id
				}

				sub := l.qty * l.unitPrice
				tax := sub * (l.taxPct / 100.0)
				lineTot := sub + tax

				var arLineID int64
				err := tx.QueryRow(ctx, `
					SELECT id FROM ar_invoice_lines WHERE ar_invoice_id = $1 AND product_id = $2`,
					arInvID, prodID).Scan(&arLineID)
				if err != nil {
					err = tx.QueryRow(ctx, `
						INSERT INTO ar_invoice_lines (
							ar_invoice_id, delivery_order_line_id, product_id, description,
							quantity, unit_price, discount_pct, tax_pct, subtotal, tax_amount, total, created_at
						) VALUES (
							$1, $2, $3, $4,
							$5, $6, 0, $7, $8, $9, $10, $11
						) RETURNING id`,
						arInvID, doLineID, prodID, l.description,
						l.qty, l.unitPrice, l.taxPct, sub, tax, lineTot, issueDate).Scan(&arLineID)
				} else {
					_, err = tx.Exec(ctx, `
						UPDATE ar_invoice_lines SET
							delivery_order_line_id = $2, description = $3,
							quantity = $4, unit_price = $5, tax_pct = $6, subtotal = $7, tax_amount = $8, total = $9
						WHERE id = $1`,
						arLineID, doLineID, l.description,
						l.qty, l.unitPrice, l.taxPct, sub, tax, lineTot)
				}
				if err != nil {
					return fmt.Errorf("upsert AR invoice line: %w", err)
				}
				arInvoiceLineIDs[fmt.Sprintf("%s:%s", inv.number, l.productSKU)] = arLineID
			}
		}

		// -------------------------------------------------------------------------
		// 5. AR Payments & Allocations (5 Payments)
		// -------------------------------------------------------------------------
		type arPayDef struct {
			number       string
			invoiceNum   string
			amount       float64
			paidAt       string
			method       string
			note         string
		}

		arPaymentsData := []arPayDef{
			{
				number:     "PAY-202604-00001",
				invoiceNum: "INV-202603-00001",
				amount:     220335000,
				paidAt:     "2026-04-28",
				method:     "TRANSFER",
				note:       "Full customer settlement via BCA IDR Operating Account",
			},
			{
				number:     "PAY-202605-00002",
				invoiceNum: "INV-202603-00002",
				amount:     325230000,
				paidAt:     "2026-05-18",
				method:     "TRANSFER",
				note:       "PLN Nusantara Power invoice settlement",
			},
			{
				number:     "PAY-202605-00003",
				invoiceNum: "INV-202604-00003",
				amount:     338550000,
				paidAt:     "2026-05-12",
				method:     "TRANSFER",
				note:       "PAM Jaya reservoir monitoring payment received",
			},
			{
				// Partial payment from Adaro Energy
				number:     "PAY-202606-00004",
				invoiceNum: "INV-202604-00004",
				amount:     150000000,
				paidAt:     "2026-06-15",
				method:     "TRANSFER",
				note:       "Partial milestone customer payment from Adaro Energy",
			},
			{
				// Partial payment from Astra Otoparts
				number:     "PAY-202607-00005",
				invoiceNum: "INV-202605-00005",
				amount:     40000000,
				paidAt:     "2026-07-20",
				method:     "TRANSFER",
				note:       "Partial invoice down payment from Astra Otoparts",
			},
		}

		for _, p := range arPaymentsData {
			arInvID := arInvoiceIDs[p.invoiceNum]
			paidDate := ParseDate(p.paidAt)

			var payID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO ar_payments (
					number, ar_invoice_id, amount, paid_at, method, note,
					created_by, created_at, updated_at,
					currency, original_currency_amount, base_currency, base_amount, fx_rate, fx_rate_date, fx_rate_source, fx_rate_locked_at
				) VALUES (
					$1, $2, $3, $4::timestamptz, $5, $6,
					$7, $4::timestamptz, $4::timestamptz,
					'IDR', $3, 'IDR', $3, 1.0, $4::date, 'BANK_INDONESIA', $4::timestamptz
				)
				ON CONFLICT (number) DO UPDATE SET
					ar_invoice_id = EXCLUDED.ar_invoice_id,
					amount = EXCLUDED.amount,
					paid_at = EXCLUDED.paid_at,
					method = EXCLUDED.method,
					note = EXCLUDED.note,
					base_amount = EXCLUDED.base_amount,
					updated_at = NOW()
				RETURNING id`,
				p.number, arInvID, p.amount, paidDate, p.method, p.note,
				accountantID).Scan(&payID)
			if err != nil {
				return fmt.Errorf("upsert AR payment %q: %w", p.number, err)
			}

			// Payment allocation
			_, err = tx.Exec(ctx, `
				INSERT INTO ar_payment_allocations (
					ar_payment_id, ar_invoice_id, amount, created_at,
					original_currency_amount, base_amount, currency, base_currency, fx_rate, fx_rate_date, fx_rate_source, fx_rate_locked_at
				)
				SELECT $1, $2, $3, $4::timestamptz, $3, $3, 'IDR', 'IDR', 1.0, $4::date, 'BANK_INDONESIA', $4::timestamptz
				WHERE NOT EXISTS (
					SELECT 1 FROM ar_payment_allocations WHERE ar_payment_id = $1 AND ar_invoice_id = $2
				)`,
				payID, arInvID, p.amount, paidDate)
			if err != nil {
				return fmt.Errorf("insert AR payment allocation: %w", err)
			}
		}

		// -------------------------------------------------------------------------
		// 6. Return Delivery Order (1 Confirmed RDO: 2 Ultrasonic Water Level Sensors)
		// -------------------------------------------------------------------------
		pamCustID := sctx.CustomerIDs["CUST-PAM-JAYA"]
		doPamID := deliveryOrderIDs["DO-202604-00003"]
		doLinePamID := deliveryOrderLineIDs["DO-202604-00003:FG-IOT-WTR01"]
		wtrProdID := sctx.ProductIDs["FG-IOT-WTR01"]

		rdoDate := ParseDate("2026-04-25")
		rdoConfirmedAt := rdoDate.Add(4 * time.Hour)

		var rdoID int64
		err := tx.QueryRow(ctx, `
			INSERT INTO return_delivery_orders (
				number, company_id, customer_id, original_delivery_order_id, warehouse_id,
				return_date, status, reason, notes,
				created_by, confirmed_by, confirmed_at, created_at, updated_at
			) VALUES (
				'RDO-202604-00001', $1, $2, $3, $4,
				$5::date, 'CONFIRMED', 'Sensor calibration recalibration adjustment', '2 water level sensors returned from Pejompongan pumping station for sensitivity adjustment',
				$6, $6, $7::timestamptz, $5::timestamptz, $7::timestamptz
			)
			ON CONFLICT (number) DO UPDATE SET
				status = EXCLUDED.status,
				reason = EXCLUDED.reason,
				notes = EXCLUDED.notes,
				confirmed_at = EXCLUDED.confirmed_at,
				updated_at = NOW()
			RETURNING id`,
			sctx.CompanyNTPID, pamCustID, doPamID, fgWHID,
			rdoDate, warehouseLeadID, rdoConfirmedAt).Scan(&rdoID)
		if err != nil {
			return fmt.Errorf("upsert return delivery order: %w", err)
		}

		var rdoLineID int64
		err = tx.QueryRow(ctx, `
			SELECT id FROM return_delivery_order_lines
			WHERE return_delivery_order_id = $1 AND product_id = $2`,
			rdoID, wtrProdID).Scan(&rdoLineID)
		if err != nil {
			err = tx.QueryRow(ctx, `
				INSERT INTO return_delivery_order_lines (
					return_delivery_order_id, delivery_order_line_id, product_id,
					quantity_returned, unit_price, restock_warehouse_id, notes, line_order
				) VALUES (
					$1, $2, $3,
					2, 2850000, $4, '2 units Ultrasonic Water Level Sensor Transmitter', 1
				) RETURNING id`,
				rdoID, doLinePamID, wtrProdID, fgWHID).Scan(&rdoLineID)
			if err != nil {
				return fmt.Errorf("insert return delivery order line: %w", err)
			}
		}

		// -------------------------------------------------------------------------
		// 7. AR Credit Note (1 Posted Credit Note linked to RDO & AR Invoice)
		// -------------------------------------------------------------------------
		arInvPamID := arInvoiceIDs["INV-202604-00003"]
		arInvLinePamID := arInvoiceLineIDs["INV-202604-00003:FG-IOT-WTR01"]

		cnDate := ParseDate("2026-04-28")
		cnPostedAt := cnDate.Add(2 * time.Hour)

		var creditNoteID int64
		err = tx.QueryRow(ctx, `
			INSERT INTO ar_credit_notes (
				number, customer_id, ar_invoice_id, return_delivery_order_id, currency,
				reason, subtotal, tax_amount, total, status,
				posted_at, posted_by, created_by, created_at, updated_at
			) VALUES (
				'CN-2604-00001', $1, $2, $3, 'IDR',
				'Credit adjustment for 2 returned ultrasonic water level sensors (RDO-202604-00001)',
				5700000, 627000, 6327000, 'POSTED',
				$4::timestamptz, $5, $5, $6::timestamptz, $6::timestamptz
			)
			ON CONFLICT (number) DO UPDATE SET
				subtotal = EXCLUDED.subtotal,
				tax_amount = EXCLUDED.tax_amount,
				total = EXCLUDED.total,
				status = EXCLUDED.status,
				posted_at = EXCLUDED.posted_at,
				updated_at = NOW()
			RETURNING id`,
			pamCustID, arInvPamID, rdoID,
			cnPostedAt, accountantID, cnDate).Scan(&creditNoteID)
		if err != nil {
			return fmt.Errorf("upsert AR credit note: %w", err)
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO ar_credit_note_lines (
				ar_credit_note_id, ar_invoice_line_id, return_delivery_order_line_id, product_id,
				description, quantity, unit_price, discount_pct, tax_pct,
				subtotal, tax_amount, total
			)
			SELECT $1, $2, $3, $4,
				'Credit Note for 2 pcs Ultrasonic Water Level Sensor Transmitter', 2, 2850000, 0, 11.00,
				5700000, 627000, 6327000
			WHERE NOT EXISTS (
				SELECT 1 FROM ar_credit_note_lines WHERE ar_credit_note_id = $1 AND product_id = $4
			)`,
			creditNoteID, arInvLinePamID, rdoLineID, wtrProdID)
		if err != nil {
			return fmt.Errorf("insert AR credit note line: %w", err)
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO ar_credit_note_allocations (ar_credit_note_id, ar_invoice_id, amount, created_at)
			VALUES ($1, $2, 6327000, $3)
			ON CONFLICT (ar_credit_note_id, ar_invoice_id) DO UPDATE SET
				amount = EXCLUDED.amount`,
			creditNoteID, arInvPamID, cnDate)
		if err != nil {
			return fmt.Errorf("insert AR credit note allocation: %w", err)
		}

		return nil
	})
}
