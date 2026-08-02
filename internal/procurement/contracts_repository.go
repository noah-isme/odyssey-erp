package procurement

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
)

// ContractRepository handles database operations for supplier contracts
type ContractRepository struct {
	db *pgxpool.Pool
}

// NewContractRepository creates a new contract repository
func NewContractRepository(db *pgxpool.Pool) *ContractRepository {
	return &ContractRepository{db: db}
}

// CreateContract creates a new supplier contract
func (r *ContractRepository) CreateContract(ctx context.Context, input CreateContractInput) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `
		INSERT INTO supplier_contracts (
			company_id, supplier_id, version, status, currency,
			effective_from, effective_to, payment_terms, incoterms,
			renewal_notice_days, created_by, note, created_at, updated_at
		) VALUES ($1, $2, 1, 'DRAFT', $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
		RETURNING id
	`, input.CompanyID, input.SupplierID, input.Currency, input.EffectiveFrom,
		input.EffectiveTo, input.PaymentTerms, input.Incoterms,
		input.RenewalNoticeDays, input.CreatedBy, input.Note).Scan(&id)

	if err != nil {
		return 0, fmt.Errorf("failed to create contract: %w", err)
	}
	return id, nil
}

// GetContract retrieves a contract by ID
func (r *ContractRepository) GetContract(ctx context.Context, contractID int64) (*SupplierContract, error) {
	contract := &SupplierContract{}
	err := r.db.QueryRow(ctx, `
		SELECT id, company_id, supplier_id, version, status, currency,
		       effective_from, effective_to, payment_terms, incoterms,
		       renewal_notice_days, expiry_notification_sent,
		       created_by, approved_by, approved_at, terminated_at, note,
		       created_at, updated_at
		FROM supplier_contracts
		WHERE id = $1
	`, contractID).Scan(
		&contract.ID, &contract.CompanyID, &contract.SupplierID, &contract.Version,
		&contract.Status, &contract.Currency, &contract.EffectiveFrom, &contract.EffectiveTo,
		&contract.PaymentTerms, &contract.Incoterms, &contract.RenewalNoticeDays,
		&contract.ExpiryNotificationSent, &contract.CreatedBy, &contract.ApprovedBy,
		&contract.ApprovedAt, &contract.TerminatedAt, &contract.Note,
		&contract.CreatedAt, &contract.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get contract: %w", err)
	}

	// Load price lines
	lines, err := r.GetContractPriceLines(ctx, contractID)
	if err != nil {
		return nil, err
	}
	contract.PriceLines = lines

	return contract, nil
}

// UpdateContractStatus updates the contract status
func (r *ContractRepository) UpdateContractStatus(ctx context.Context, contractID int64, status ContractStatus) error {
	_, err := r.db.Exec(ctx, `
		UPDATE supplier_contracts
		SET status = $2, updated_at = NOW()
		WHERE id = $1
	`, contractID, string(status))

	if err != nil {
		return fmt.Errorf("failed to update contract status: %w", err)
	}
	return nil
}

// ListActiveContracts retrieves all active contracts for a supplier
func (r *ContractRepository) ListActiveContracts(ctx context.Context, companyID, supplierID int64) ([]SupplierContract, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, company_id, supplier_id, version, status, currency,
		       effective_from, effective_to, payment_terms, incoterms,
		       renewal_notice_days, expiry_notification_sent,
		       created_by, approved_by, approved_at, terminated_at, note,
		       created_at, updated_at
		FROM supplier_contracts
		WHERE company_id = $1 AND supplier_id = $2 AND status = 'ACTIVE'
		  AND effective_from <= CURRENT_DATE
		  AND (effective_to IS NULL OR effective_to >= CURRENT_DATE)
		ORDER BY version DESC
	`, companyID, supplierID)

	if err != nil {
		return nil, fmt.Errorf("failed to list active contracts: %w", err)
	}
	defer rows.Close()

	var contracts []SupplierContract
	for rows.Next() {
		var c SupplierContract
		err := rows.Scan(
			&c.ID, &c.CompanyID, &c.SupplierID, &c.Version,
			&c.Status, &c.Currency, &c.EffectiveFrom, &c.EffectiveTo,
			&c.PaymentTerms, &c.Incoterms, &c.RenewalNoticeDays,
			&c.ExpiryNotificationSent, &c.CreatedBy, &c.ApprovedBy,
			&c.ApprovedAt, &c.TerminatedAt, &c.Note,
			&c.CreatedAt, &c.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan contract: %w", err)
		}
		contracts = append(contracts, c)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating contracts: %w", err)
	}

	return contracts, nil
}

// ApproveContract transitions contract to ACTIVE status
func (r *ContractRepository) ApproveContract(ctx context.Context, contractID int64, approvedBy int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE supplier_contracts
		SET status = 'ACTIVE', approved_by = $2, approved_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status = 'APPROVAL'
	`, contractID, approvedBy)

	if err != nil {
		return fmt.Errorf("failed to approve contract: %w", err)
	}
	return nil
}

// TerminateContract transitions contract to TERMINATED status
func (r *ContractRepository) TerminateContract(ctx context.Context, contractID int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE supplier_contracts
		SET status = 'TERMINATED', terminated_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status = 'ACTIVE'
	`, contractID)

	if err != nil {
		return fmt.Errorf("failed to terminate contract: %w", err)
	}
	return nil
}

// GetContractPriceLines retrieves all price tiers for a contract
func (r *ContractRepository) GetContractPriceLines(ctx context.Context, contractID int64) ([]ContractPriceLine, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, contract_id, product_id, min_quantity, unit_price, tax_rate,
		       lead_time_days, moq
		FROM contract_price_lines
		WHERE contract_id = $1
		ORDER BY product_id, min_quantity
	`, contractID)

	if err != nil {
		return nil, fmt.Errorf("failed to get contract price lines: %w", err)
	}
	defer rows.Close()

	var lines []ContractPriceLine
	for rows.Next() {
		var line ContractPriceLine
		var minQtyStr, moqStr string
		var unitPriceStr, taxRateStr string

		err := rows.Scan(
			&line.ID, &line.ContractID, &line.ProductID, &minQtyStr, &unitPriceStr, &taxRateStr,
			&line.LeadTimeDays, &moqStr,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan price line: %w", err)
		}

		// Parse Money values from NUMERIC columns
		line.MinQuantity, _ = accountingmoney.Parse(minQtyStr, 4)
		line.UnitPrice, _ = accountingmoney.Parse(unitPriceStr, 4)
		line.TaxRate, _ = accountingmoney.Parse(taxRateStr, 2)
		line.MOQ, _ = accountingmoney.Parse(moqStr, 4)

		lines = append(lines, line)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating price lines: %w", err)
	}

	return lines, nil
}

// InsertContractPriceLine adds a price tier to a contract
func (r *ContractRepository) InsertContractPriceLine(ctx context.Context, line ContractPriceLineInput) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO contract_price_lines (
			contract_id, product_id, min_quantity, unit_price, tax_rate,
			lead_time_days, moq
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, line.ContractID, line.ProductID, line.MinQuantity.String(), line.UnitPrice.String(),
		line.TaxRate.String(), line.LeadTimeDays, line.MOQ.String())

	if err != nil {
		return fmt.Errorf("failed to insert price line: %w", err)
	}
	return nil
}

// GetApplicablePriceLine retrieves the matching price tier for a product and quantity
func (r *ContractRepository) GetApplicablePriceLine(ctx context.Context, contractID, productID int64, qty accountingmoney.Money) (*ContractPriceLine, error) {
	var line ContractPriceLine
	var minQtyStr, moqStr, unitPriceStr, taxRateStr string

	err := r.db.QueryRow(ctx, `
		SELECT id, contract_id, product_id, min_quantity, unit_price, tax_rate,
		       lead_time_days, moq
		FROM contract_price_lines
		WHERE contract_id = $1 AND product_id = $2 AND min_quantity <= $3
		ORDER BY min_quantity DESC
		LIMIT 1
	`, contractID, productID, qty.String()).Scan(
		&line.ID, &line.ContractID, &line.ProductID, &minQtyStr, &unitPriceStr, &taxRateStr,
		&line.LeadTimeDays, &moqStr,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get applicable price line: %w", err)
	}

	line.MinQuantity, _ = accountingmoney.Parse(minQtyStr, 4)
	line.UnitPrice, _ = accountingmoney.Parse(unitPriceStr, 4)
	line.TaxRate, _ = accountingmoney.Parse(taxRateStr, 2)
	line.MOQ, _ = accountingmoney.Parse(moqStr, 4)

	return &line, nil
}

// RecordPriceHistory records an immutable price observation
func (r *ContractRepository) RecordPriceHistory(ctx context.Context, input RecordPriceHistoryInput) (int64, error) {
	var id int64
	var fxRateStr *string
	if input.FXRate != nil {
		s := input.FXRate.String()
		fxRateStr = &s
	}

	err := r.db.QueryRow(ctx, `
		INSERT INTO price_history (
			company_id, supplier_id, product_id, source_type, source_id,
			currency, unit_price, quantity, tax_rate, moq, lead_time_days,
			fx_rate, base_currency_price, observation_date, note, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW())
		RETURNING id
	`, input.CompanyID, input.SupplierID, input.ProductID, string(input.SourceType), input.SourceID,
		input.Currency, input.UnitPrice.String(), input.Quantity.String(), input.TaxRate.String(),
		input.MOQ.String(), input.LeadTimeDays, fxRateStr, nil, time.Now(), input.Note).Scan(&id)

	if err != nil {
		return 0, fmt.Errorf("failed to record price history: %w", err)
	}
	return id, nil
}

// ListPriceHistory retrieves price observations for a supplier/product combination
func (r *ContractRepository) ListPriceHistory(ctx context.Context, companyID, supplierID, productID int64, limit int) ([]PriceHistory, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, company_id, supplier_id, product_id, source_type, source_id,
		       currency, unit_price, quantity, tax_rate, moq, lead_time_days,
		       fx_rate, base_currency_price, observation_date, note, created_at
		FROM price_history
		WHERE company_id = $1 AND supplier_id = $2 AND product_id = $3
		ORDER BY observation_date DESC
		LIMIT $4
	`, companyID, supplierID, productID, limit)

	if err != nil {
		return nil, fmt.Errorf("failed to list price history: %w", err)
	}
	defer rows.Close()

	var records []PriceHistory
	for rows.Next() {
		var ph PriceHistory
		var sourceTypeStr string
		var unitPriceStr, qtyStr, taxRateStr, moqStr string
		var fxRateStr, baseCurrencyPriceStr *string

		err := rows.Scan(
			&ph.ID, &ph.CompanyID, &ph.SupplierID, &ph.ProductID, &sourceTypeStr, &ph.SourceID,
			&ph.Currency, &unitPriceStr, &qtyStr, &taxRateStr, &moqStr, &ph.LeadTimeDays,
			&fxRateStr, &baseCurrencyPriceStr, &ph.ObservationDate, &ph.Note, &ph.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan price history: %w", err)
		}

		ph.SourceType = PriceHistorySource(sourceTypeStr)
		ph.UnitPrice, _ = accountingmoney.Parse(unitPriceStr, 4)
		ph.Quantity, _ = accountingmoney.Parse(qtyStr, 4)
		ph.TaxRate, _ = accountingmoney.Parse(taxRateStr, 2)
		ph.MOQ, _ = accountingmoney.Parse(moqStr, 4)

		if fxRateStr != nil {
			fx, _ := accountingmoney.Parse(*fxRateStr, 6)
			ph.FXRate = &fx
		}
		if baseCurrencyPriceStr != nil {
			bcp, _ := accountingmoney.Parse(*baseCurrencyPriceStr, 4)
			ph.BaseCurrencyPrice = &bcp
		}

		records = append(records, ph)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating price history: %w", err)
	}

	return records, nil
}

// CreateScorecard creates a new supplier scorecard
func (r *ContractRepository) CreateScorecard(ctx context.Context, input CreateScorecardInput) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `
		INSERT INTO supplier_scorecards (
			company_id, supplier_id, version, period_start, period_end,
			status, created_by, created_at
		) VALUES ($1, $2, (
			SELECT COALESCE(MAX(version), 0) + 1
			FROM supplier_scorecards
			WHERE company_id = $1 AND supplier_id = $2
		), $3, $4, 'DRAFT', $5, NOW())
		RETURNING id
	`, input.CompanyID, input.SupplierID, input.PeriodStart, input.PeriodEnd, input.CreatedBy).Scan(&id)

	if err != nil {
		return 0, fmt.Errorf("failed to create scorecard: %w", err)
	}
	return id, nil
}

// GetScorecard retrieves a scorecard by ID
func (r *ContractRepository) GetScorecard(ctx context.Context, scorecardID int64) (*SupplierScorecard, error) {
	sc := &SupplierScorecard{}
	var deliveryScoreStr, qualityScoreStr, priceScoreStr, rfqScoreStr, reviewerScoreStr, overallScoreStr string

	err := r.db.QueryRow(ctx, `
		SELECT id, company_id, supplier_id, version, period_start, period_end, status,
		       delivery_otif_score, delivery_otif_weight, delivery_otif_sample_size,
		       quality_score, quality_weight, quality_sample_size,
		       price_adherence_score, price_adherence_weight, price_adherence_sample_size,
		       rfq_responsiveness_score, rfq_responsiveness_weight, rfq_responsiveness_sample_size,
		       reviewer_assessment_score, reviewer_assessment_weight,
		       overall_score, published_by, published_at, note, created_by, created_at
		FROM supplier_scorecards
		WHERE id = $1
	`, scorecardID).Scan(
		&sc.ID, &sc.CompanyID, &sc.SupplierID, &sc.Version, &sc.PeriodStart, &sc.PeriodEnd,
		&sc.Status, &deliveryScoreStr, &sc.DeliveryOTIFWeight, &sc.DeliveryOTIFSampleSize,
		&qualityScoreStr, &sc.QualityWeight, &sc.QualitySampleSize,
		&priceScoreStr, &sc.PriceAdherenceWeight, &sc.PriceAdherenceSampleSize,
		&rfqScoreStr, &sc.RFQResponsivenessWeight, &sc.RFQResponsivenessSampleSize,
		&reviewerScoreStr, &sc.ReviewerAssessmentWeight,
		&overallScoreStr, &sc.PublishedBy, &sc.PublishedAt, &sc.Note, &sc.CreatedBy, &sc.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get scorecard: %w", err)
	}

	sc.DeliveryOTIFScore, _ = accountingmoney.Parse(deliveryScoreStr, 2)
	sc.QualityScore, _ = accountingmoney.Parse(qualityScoreStr, 2)
	sc.PriceAdherenceScore, _ = accountingmoney.Parse(priceScoreStr, 2)
	sc.RFQResponsivenessScore, _ = accountingmoney.Parse(rfqScoreStr, 2)
	sc.ReviewerAssessmentScore, _ = accountingmoney.Parse(reviewerScoreStr, 2)
	sc.OverallScore, _ = accountingmoney.Parse(overallScoreStr, 2)

	return sc, nil
}

// PublishScorecard publishes a scorecard (immutable after publication)
func (r *ContractRepository) PublishScorecard(ctx context.Context, scorecardID int64, publishedBy int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE supplier_scorecards
		SET status = 'PUBLISHED', published_by = $2, published_at = NOW()
		WHERE id = $1 AND status = 'DRAFT'
	`, scorecardID, publishedBy)

	if err != nil {
		return fmt.Errorf("failed to publish scorecard: %w", err)
	}
	return nil
}

// CreatePOVariance records a purchase order variance exception
func (r *ContractRepository) CreatePOVariance(ctx context.Context, input CreatePOVarianceInput) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `
		INSERT INTO po_contract_variances (
			company_id, po_id, po_line_id, contract_id,
			variance_type, variance_percentage, variance_reason, note,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		RETURNING id
	`, input.CompanyID, input.POID, input.POLineID, input.ContractID,
		string(input.VarianceType), input.VariancePercentage, input.VarianceReason, input.Note).Scan(&id)

	if err != nil {
		return 0, fmt.Errorf("failed to create po variance: %w", err)
	}
	return id, nil
}

// GetPOVariance retrieves a variance record by ID
func (r *ContractRepository) GetPOVariance(ctx context.Context, varianceID int64) (*POContractVariance, error) {
	variance := &POContractVariance{}
	var varianceTypeStr string
	var variancePctStr *string

	err := r.db.QueryRow(ctx, `
		SELECT id, company_id, po_id, po_line_id, contract_id,
		       variance_type, variance_percentage, variance_reason,
		       approval_status, approved_by, approved_at, note,
		       created_at, updated_at
		FROM po_contract_variances
		WHERE id = $1
	`, varianceID).Scan(
		&variance.ID, &variance.CompanyID, &variance.POID, &variance.POLineID, &variance.ContractID,
		&varianceTypeStr, &variancePctStr, &variance.VarianceReason,
		&variance.ApprovalStatus, &variance.ApprovedBy, &variance.ApprovedAt, &variance.Note,
		&variance.CreatedAt, &variance.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get po variance: %w", err)
	}

	variance.VarianceType = VarianceType(varianceTypeStr)
	if variancePctStr != nil {
		pct, _ := accountingmoney.Parse(*variancePctStr, 2)
		variance.VariancePercentage = &pct
	}

	return variance, nil
}

// ApprovePOVariance approves a variance exception
func (r *ContractRepository) ApprovePOVariance(ctx context.Context, varianceID int64, approvedBy int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE po_contract_variances
		SET approval_status = 'APPROVED', approved_by = $2, approved_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1
	`, varianceID, approvedBy)

	if err != nil {
		return fmt.Errorf("failed to approve variance: %w", err)
	}
	return nil
}
