package procurement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SourcingRepository keeps sourcing persistence independent from sqlc while the
// schema is still evolving. Existing PR/PO sqlc contracts remain unchanged.
type SourcingRepository struct{ pool *pgxpool.Pool }

func NewSourcingRepository(pool *pgxpool.Pool) *SourcingRepository {
	return &SourcingRepository{pool: pool}
}

func (r *SourcingRepository) withTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

func (r *SourcingRepository) CreateRFQ(ctx context.Context, rfq RFQ) (RFQ, error) {
	err := r.withTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO rfqs (company_id, number, currency, response_due_at, commercial_terms, price_weight, lead_time_weight, terms_weight, supplier_rating_weight, created_by)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id, version`, rfq.CompanyID, rfq.Number, rfq.Currency, rfq.ResponseDueAt, rfq.CommercialTerms, rfq.Weights.Price, rfq.Weights.LeadTime, rfq.Weights.Terms, rfq.Weights.SupplierRating, rfq.CreatedBy).Scan(&rfq.ID, &rfq.Version); err != nil {
			return err
		}
		for _, line := range rfq.Lines {
			if err := tx.QueryRow(ctx, `INSERT INTO rfq_lines (rfq_id, pr_line_id, product_id, quantity, note, line_order) VALUES ($1,NULLIF($2,0),$3,$4,$5,$6) RETURNING id`, rfq.ID, line.PRLineID, line.ProductID, line.Quantity, line.Note, line.LineOrder).Scan(&line.ID); err != nil {
				return err
			}
		}
		for _, supplierID := range rfq.Suppliers {
			var valid bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM suppliers WHERE id=$1 AND company_id=$2 AND is_active)`, supplierID, rfq.CompanyID).Scan(&valid); err != nil {
				return err
			}
			if !valid {
				return fmt.Errorf("supplier %d is not active for this company", supplierID)
			}
			if _, err := tx.Exec(ctx, `INSERT INTO rfq_suppliers (rfq_id, supplier_id) VALUES ($1,$2)`, rfq.ID, supplierID); err != nil {
				return err
			}
		}
		return nil
	})
	return rfq, err
}

func (r *SourcingRepository) GetRFQ(ctx context.Context, id int64) (RFQ, error) {
	var rfq RFQ
	err := r.pool.QueryRow(ctx, `SELECT id, company_id, number, status, currency, response_due_at, commercial_terms, price_weight, lead_time_weight, terms_weight, supplier_rating_weight, created_by, version FROM rfqs WHERE id=$1`, id).Scan(&rfq.ID, &rfq.CompanyID, &rfq.Number, &rfq.Status, &rfq.Currency, &rfq.ResponseDueAt, &rfq.CommercialTerms, &rfq.Weights.Price, &rfq.Weights.LeadTime, &rfq.Weights.Terms, &rfq.Weights.SupplierRating, &rfq.CreatedBy, &rfq.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return RFQ{}, ErrNotFound
	}
	return rfq, err
}

func (r *SourcingRepository) UpdateRFQStatus(ctx context.Context, id int64, from, to RFQStatus) error {
	command, err := r.pool.Exec(ctx, `UPDATE rfqs SET status=$3, version=version+1, issued_at=CASE WHEN $3='ISSUED' THEN NOW() ELSE issued_at END, closed_at=CASE WHEN $3='CLOSED' THEN NOW() ELSE closed_at END, awarded_at=CASE WHEN $3='AWARDED' THEN NOW() ELSE awarded_at END, updated_at=NOW() WHERE id=$1 AND status=$2`, id, from, to)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrInvalidState
	}
	return nil
}

func (r *SourcingRepository) MarkInvitationsSent(ctx context.Context, rfqID int64, messageRef string) error {
	_, err := r.pool.Exec(ctx, `UPDATE rfq_suppliers SET status='SENT', invited_at=NOW(), issued_message_ref=$2 WHERE rfq_id=$1`, rfqID, messageRef)
	return err
}

func (r *SourcingRepository) RFQSupplierEmails(ctx context.Context, rfqID int64) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT s.email FROM rfq_suppliers rs JOIN suppliers s ON s.id=rs.supplier_id WHERE rs.rfq_id=$1 AND s.email<>'' ORDER BY s.email`, rfqID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var emails []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, err
		}
		emails = append(emails, email)
	}
	return emails, rows.Err()
}

func (r *SourcingRepository) CreateBid(ctx context.Context, input CreateBidInput, lines []storedBidLine) (int64, error) {
	var id int64
	err := r.withTx(ctx, func(tx pgx.Tx) error {
		var rfqStatus RFQStatus
		var companyID int64
		if err := tx.QueryRow(ctx, `SELECT status, company_id FROM rfqs WHERE id=$1 FOR SHARE`, input.RFQID).Scan(&rfqStatus, &companyID); err != nil {
			return err
		}
		if rfqStatus != RFQStatusIssued || companyID != input.CompanyID {
			return ErrInvalidState
		}
		var invited bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM rfq_suppliers WHERE rfq_id=$1 AND supplier_id=$2)`, input.RFQID, input.SupplierID).Scan(&invited); err != nil {
			return err
		}
		if !invited {
			return ErrValidation
		}
		if err := tx.QueryRow(ctx, `INSERT INTO rfq_bids (rfq_id, supplier_id, company_id, currency, fx_rate, fx_rate_date, payment_terms, source_reference, valid_until, created_by)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`, input.RFQID, input.SupplierID, input.CompanyID, input.Currency, input.FXRate, input.FXRateDate, input.PaymentTerms, input.SourceReference, input.ValidUntil, input.CreatedBy).Scan(&id); err != nil {
			return err
		}
		for _, line := range lines {
			var lineRFQID int64
			if err := tx.QueryRow(ctx, `SELECT rfq_id FROM rfq_lines WHERE id=$1`, line.RFQLineID).Scan(&lineRFQID); err != nil {
				return err
			}
			if lineRFQID != input.RFQID {
				return ErrValidation
			}
			_, err := tx.Exec(ctx, `INSERT INTO rfq_bid_lines (bid_id, rfq_line_id, quantity, unit_price, unit_price_base, tax_amount, freight_amount, tax_amount_base, freight_amount_base, minimum_order_quantity, lead_time_days, commercial_score, supplier_rating_score, note)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, id, line.RFQLineID, line.Quantity, line.UnitPrice, line.UnitPriceBase, line.TaxAmount, line.FreightAmount, line.TaxAmountBase, line.FreightAmountBase, line.MinimumOrderQuantity, line.LeadTimeDays, line.CommercialScore, line.SupplierRatingScore, line.Note)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return id, err
}

func (r *SourcingRepository) UpdateBidStatus(ctx context.Context, bidID int64, from, to BidStatus) error {
	command, err := r.pool.Exec(ctx, `UPDATE rfq_bids SET status=$3, submitted_at=CASE WHEN $3='SUBMITTED' THEN NOW() ELSE submitted_at END, version=version+1, updated_at=NOW() WHERE id=$1 AND status=$2`, bidID, from, to)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrInvalidState
	}
	return nil
}

func (r *SourcingRepository) ComparisonCandidates(ctx context.Context, rfqID int64) ([]comparisonCandidate, error) {
	rows, err := r.pool.Query(ctx, `SELECT l.rfq_line_id, b.id, l.id, b.supplier_id,
       ((l.quantity*l.unit_price_base)+l.tax_amount_base+l.freight_amount_base)::text,
       l.lead_time_days, l.commercial_score, l.supplier_rating_score
FROM rfq_bid_lines l JOIN rfq_bids b ON b.id=l.bid_id
WHERE b.rfq_id=$1 AND b.status='SUBMITTED' ORDER BY l.rfq_line_id, b.id`, rfqID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []comparisonCandidate
	for rows.Next() {
		var candidate comparisonCandidate
		if err := rows.Scan(&candidate.RFQLineID, &candidate.BidID, &candidate.BidLineID, &candidate.SupplierID, &candidate.TotalBaseAmount, &candidate.LeadTimeDays, &candidate.CommercialScore, &candidate.SupplierRatingScore); err != nil {
			return nil, err
		}
		result = append(result, candidate)
	}
	return result, rows.Err()
}

func (r *SourcingRepository) SaveComparison(ctx context.Context, rfqID, companyID int64, version int, createdBy int64, entries []ComparisonEntry) error {
	payload, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `INSERT INTO rfq_comparison_snapshots (rfq_id, company_id, rfq_version, comparison, created_by) VALUES ($1,$2,$3,$4,$5)`, rfqID, companyID, version, payload, createdBy)
	return err
}

func (r *SourcingRepository) CreateAward(ctx context.Context, award Award, lines []AwardLineInput) (Award, error) {
	err := r.withTx(ctx, func(tx pgx.Tx) error {
		var rfqStatus RFQStatus
		if err := tx.QueryRow(ctx, `SELECT status FROM rfqs WHERE id=$1 AND company_id=$2 FOR UPDATE`, award.RFQID, award.CompanyID).Scan(&rfqStatus); err != nil {
			return err
		}
		if rfqStatus != RFQStatusClosed {
			return ErrInvalidState
		}
		var warehouseValid bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM warehouses w JOIN branches b ON b.id=w.branch_id WHERE w.id=$1 AND b.company_id=$2)`, award.ExpectedWarehouseID, award.CompanyID).Scan(&warehouseValid); err != nil {
			return err
		}
		if !warehouseValid {
			return ErrValidation
		}
		if err := tx.QueryRow(ctx, `INSERT INTO rfq_awards (rfq_id, company_id, expected_warehouse_id, note, created_by) VALUES ($1,$2,$3,$4,$5) RETURNING id, version`, award.RFQID, award.CompanyID, award.ExpectedWarehouseID, award.Note, award.CreatedBy).Scan(&award.ID, &award.Version); err != nil {
			return err
		}
		for _, line := range lines {
			var supplierID int64
			var available, requested string
			if err := tx.QueryRow(ctx, `SELECT b.supplier_id, bl.quantity::text, rl.quantity::text FROM rfq_bid_lines bl JOIN rfq_bids b ON b.id=bl.bid_id JOIN rfq_lines rl ON rl.id=bl.rfq_line_id WHERE bl.id=$1 AND bl.rfq_line_id=$2 AND b.rfq_id=$3 AND b.status='SUBMITTED'`, line.BidLineID, line.RFQLineID, award.RFQID).Scan(&supplierID, &available, &requested); err != nil {
				return err
			}
			if !lessOrEqualDecimal(line.Quantity, available) || !lessOrEqualDecimal(line.Quantity, requested) {
				return ErrValidation
			}
			var alreadyAwarded string
			if err := tx.QueryRow(ctx, `SELECT COALESCE(SUM(quantity), 0)::text FROM rfq_award_lines WHERE rfq_line_id=$1`, line.RFQLineID).Scan(&alreadyAwarded); err != nil {
				return err
			}
			if !lessOrEqualDecimal(sumDecimal(alreadyAwarded, line.Quantity), requested) {
				return ErrValidation
			}
			_, err := tx.Exec(ctx, `INSERT INTO rfq_award_lines (award_id, rfq_line_id, bid_line_id, supplier_id, quantity, unit_price, tax_amount, freight_amount)
SELECT $1, $2, bl.id, b.supplier_id, $3, bl.unit_price, bl.tax_amount, bl.freight_amount FROM rfq_bid_lines bl JOIN rfq_bids b ON b.id=bl.bid_id WHERE bl.id=$4`, award.ID, line.RFQLineID, line.Quantity, line.BidLineID)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return award, err
}

func (r *SourcingRepository) UpdateAwardStatus(ctx context.Context, id int64, from, to AwardStatus, actorID int64) error {
	command, err := r.pool.Exec(ctx, `UPDATE rfq_awards SET status=$3, approved_by=CASE WHEN $3='APPROVED' THEN $4 ELSE approved_by END, approved_at=CASE WHEN $3='APPROVED' THEN NOW() ELSE approved_at END, version=version+1, updated_at=NOW() WHERE id=$1 AND status=$2`, id, from, to, actorID)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrInvalidState
	}
	return nil
}

func (r *SourcingRepository) GetAward(ctx context.Context, id int64) (Award, error) {
	var award Award
	err := r.pool.QueryRow(ctx, `SELECT id, rfq_id, company_id, expected_warehouse_id, status, note, created_by, version FROM rfq_awards WHERE id=$1`, id).Scan(&award.ID, &award.RFQID, &award.CompanyID, &award.ExpectedWarehouseID, &award.Status, &award.Note, &award.CreatedBy, &award.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return Award{}, ErrNotFound
	}
	return award, err
}

func (r *SourcingRepository) GenerateAwardPOs(ctx context.Context, awardID int64) error {
	return r.withTx(ctx, func(tx pgx.Tx) error {
		var companyID, warehouseID, rfqID int64
		var status AwardStatus
		if err := tx.QueryRow(ctx, `SELECT company_id, expected_warehouse_id, rfq_id, status FROM rfq_awards WHERE id=$1 FOR UPDATE`, awardID).Scan(&companyID, &warehouseID, &rfqID, &status); err != nil {
			return err
		}
		if status != AwardStatusApproved {
			return ErrInvalidState
		}
		rows, err := tx.Query(ctx, `SELECT al.id, al.supplier_id, b.currency, al.rfq_line_id, al.quantity::text, al.unit_price::text
FROM rfq_award_lines al JOIN rfq_bid_lines bl ON bl.id=al.bid_line_id JOIN rfq_bids b ON b.id=bl.bid_id
WHERE al.award_id=$1 AND al.po_line_id IS NULL ORDER BY al.supplier_id, b.currency, al.id FOR UPDATE`, awardID)
		if err != nil {
			return err
		}
		defer rows.Close()
		type awardLine struct {
			id, supplierID, rfqLineID int64
			currency, quantity, price string
		}
		groups := map[string][]awardLine{}
		for rows.Next() {
			var line awardLine
			if err := rows.Scan(&line.id, &line.supplierID, &line.currency, &line.rfqLineID, &line.quantity, &line.price); err != nil {
				return err
			}
			groups[fmt.Sprintf("%d:%s", line.supplierID, line.currency)] = append(groups[fmt.Sprintf("%d:%s", line.supplierID, line.currency)], line)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for _, lines := range groups {
			first := lines[0]
			number := fmt.Sprintf("PO-RFQ-%d-%d-%d", rfqID, first.supplierID, time.Now().UnixNano())
			var poID int64
			if err := tx.QueryRow(ctx, `INSERT INTO pos (number, supplier_id, status, currency, expected_date, expected_warehouse_id, note, company_id, created_at, rfq_award_id) VALUES ($1,$2,'DRAFT',$3,CURRENT_DATE,$4,$5,$6,NOW(),$7) RETURNING id`, number, first.supplierID, first.currency, warehouseID, fmt.Sprintf("RFQ award %d", awardID), companyID, awardID).Scan(&poID); err != nil {
				return err
			}
			for _, line := range lines {
				var productID int64
				if err := tx.QueryRow(ctx, `SELECT product_id FROM rfq_lines WHERE id=$1`, line.rfqLineID).Scan(&productID); err != nil {
					return err
				}
				var poLineID int64
				if err := tx.QueryRow(ctx, `INSERT INTO po_lines (po_id, product_id, qty, price, note, rfq_award_line_id) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`, poID, productID, line.quantity, line.price, fmt.Sprintf("RFQ award %d", awardID), line.id).Scan(&poLineID); err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `UPDATE rfq_award_lines SET po_id=$2, po_line_id=$3 WHERE id=$1`, line.id, poID, poLineID); err != nil {
					return err
				}
			}
		}
		_, err = tx.Exec(ctx, `UPDATE rfqs SET status='AWARDED', awarded_at=NOW(), version=version+1, updated_at=NOW() WHERE id=$1 AND status='CLOSED'`, rfqID)
		return err
	})
}

type storedBidLine struct {
	RFQLineID                                                                                                            int64
	Quantity, UnitPrice, UnitPriceBase, TaxAmount, FreightAmount, TaxAmountBase, FreightAmountBase, MinimumOrderQuantity string
	LeadTimeDays, CommercialScore, SupplierRatingScore                                                                   int
	Note                                                                                                                 string
}
