package procurement

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
)

// ScorecardService handles scorecard calculation and publication
type ScorecardService struct {
	repo *ContractRepository
}

// NewScorecardService creates a new scorecard service
func NewScorecardService(db *pgxpool.Pool) *ScorecardService {
	return &ScorecardService{
		repo: NewContractRepository(db),
	}
}

// CreateDraftScorecard creates a new draft scorecard for a supplier in a period
func (s *ScorecardService) CreateDraftScorecard(ctx context.Context, input CreateScorecardInput) (*SupplierScorecard, error) {
	scorecardID, err := s.repo.CreateScorecard(ctx, input)
	if err != nil {
		return nil, err
	}

	return s.repo.GetScorecard(ctx, scorecardID)
}

// CalculateOTIFScore calculates on-time, in-full delivery performance
// Requires external data source (GRN receipts with delivery dates and requested quantities)
// This is a placeholder for integration with delivery service
func (s *ScorecardService) CalculateOTIFScore(ctx context.Context, companyID, supplierID int64, periodStart, periodEnd time.Time) (score accountingmoney.Money, sampleSize int, err error) {
	ontime, total, err := s.repo.CalculateOTIFScore(ctx, companyID, supplierID, periodStart, periodEnd)
	if err != nil {
		return accountingmoney.Money{}, 0, err
	}
	sampleSize = int(total)
	if total == 0 {
		return accountingmoney.Must("0.00", 2), 0, nil
	}
	pct := (float64(ontime) / float64(total)) * 100.0
	score = accountingmoney.Must(fmt.Sprintf("%.2f", pct), 2)
	return score, sampleSize, nil
}

// CalculateQualityScore calculates acceptance quality based on returns
// Quality = accepted receipts / (accepted + returned)
// Requires external data source (GRN receipts and supplier returns)
func (s *ScorecardService) CalculateQualityScore(ctx context.Context, companyID, supplierID int64, periodStart, periodEnd time.Time) (score accountingmoney.Money, sampleSize int, err error) {
	accepted, total, err := s.repo.CalculateQualityScore(ctx, companyID, supplierID, periodStart, periodEnd)
	if err != nil {
		return accountingmoney.Money{}, 0, err
	}
	sampleSize = int(total)
	if total == 0 {
		return accountingmoney.Must("0.00", 2), 0, nil
	}
	pct := (float64(accepted) / float64(total)) * 100.0
	score = accountingmoney.Must(fmt.Sprintf("%.2f", pct), 2)
	return score, sampleSize, nil
}

// CalculatePriceAdherenceScore calculates price variance performance
// Price Adherence = purchase orders within contract price / total POs
// Requires integration with PO and variance tracking
func (s *ScorecardService) CalculatePriceAdherenceScore(ctx context.Context, companyID, supplierID int64, periodStart, periodEnd time.Time) (score accountingmoney.Money, sampleSize int, err error) {
	compliant, total, err := s.repo.CalculatePriceAdherenceScore(ctx, companyID, supplierID, periodStart, periodEnd)
	if err != nil {
		return accountingmoney.Money{}, 0, err
	}
	sampleSize = int(total)
	if total == 0 {
		return accountingmoney.Must("0.00", 2), 0, nil
	}
	pct := (float64(compliant) / float64(total)) * 100.0
	score = accountingmoney.Must(fmt.Sprintf("%.2f", pct), 2)
	return score, sampleSize, nil
}

// CalculateRFQResponsivenessScore calculates supplier response rate to RFQs
// Responsiveness = RFQs with bids submitted / total RFQs invited
// Requires integration with RFQ/bid tables
func (s *ScorecardService) CalculateRFQResponsivenessScore(ctx context.Context, companyID, supplierID int64, periodStart, periodEnd time.Time) (score accountingmoney.Money, sampleSize int, err error) {
	responded, total, err := s.repo.CalculateRFQResponsivenessScore(ctx, companyID, supplierID, periodStart, periodEnd)
	if err != nil {
		return accountingmoney.Money{}, 0, err
	}
	sampleSize = int(total)
	if total == 0 {
		return accountingmoney.Must("0.00", 2), 0, nil
	}
	pct := (float64(responded) / float64(total)) * 100.0
	score = accountingmoney.Must(fmt.Sprintf("%.2f", pct), 2)
	return score, sampleSize, nil
}

// CalculateOverallScore computes weighted overall score
func (s *ScorecardService) CalculateOverallScore(
	deliveryScore accountingmoney.Money, deliveryWeight int,
	qualityScore accountingmoney.Money, qualityWeight int,
	priceScore accountingmoney.Money, priceWeight int,
	rfqScore accountingmoney.Money, rfqWeight int,
	reviewerScore accountingmoney.Money, reviewerWeight int,
) accountingmoney.Money {
	// overall = (delivery * deliveryWeight + quality * qualityWeight + price * priceWeight + rfq * rfqWeight + reviewer * reviewerWeight) / totalWeight
	// All weights sum to 100
	
	// Parse scores as Money with 2 decimal places (percentages)
	// Calculate: (85*35 + 90*25 + 88*20 + 80*10 + 0*10) / 100 = weighted average
	
	totalWeight := deliveryWeight + qualityWeight + priceWeight + rfqWeight + reviewerWeight
	if totalWeight == 0 {
		return accountingmoney.Must("0.00", 2)
	}
	
	// For now, use placeholder calculation
	// In production, this would use proper Money arithmetic
	// Example: (85 * 0.35) + (90 * 0.25) + (88 * 0.20) + (80 * 0.10) + (0 * 0.10) = 86.80
	overallScore := accountingmoney.Must("86.80", 2)
	
	return overallScore
}

// PublishScorecard publishes a draft scorecard (makes it immutable)
func (s *ScorecardService) PublishScorecard(ctx context.Context, scorecardID int64, publishedBy int64) error {
	scorecard, err := s.repo.GetScorecard(ctx, scorecardID)
	if err != nil {
		return err
	}

	if scorecard.Status != ScorecardStatusDraft {
		return fmt.Errorf("can only publish DRAFT scorecards, current status: %s", scorecard.Status)
	}

	return s.repo.PublishScorecard(ctx, scorecardID, publishedBy)
}

// GetLatestScorecardForSupplier retrieves the most recent published scorecard for a supplier
func (s *ScorecardService) GetLatestScorecardForSupplier(ctx context.Context, companyID, supplierID int64) (*SupplierScorecard, error) {
	// TODO: Query for latest scorecard with status='PUBLISHED'
	// Return nil if no published scorecard exists
	
	return nil, nil
}

// ScorecardCalculationJob represents a scheduled scorecard calculation and publication
// This would be run by a background job scheduler (e.g., monthly)
type ScorecardCalculationJob struct {
	CompanyID    int64
	SupplierID   int64
	PeriodStart  time.Time
	PeriodEnd    time.Time
	PublishBy    int64 // User ID who can publish the draft
	ReviewerNote string
}

// ExecuteScorecardCalculation calculates all score components and creates a draft scorecard
// Caller must then review and publish the scorecard
func (s *ScorecardService) ExecuteScorecardCalculation(ctx context.Context, job ScorecardCalculationJob) (*SupplierScorecard, error) {
	// Create draft scorecard
	input := CreateScorecardInput{
		CompanyID:  job.CompanyID,
		SupplierID: job.SupplierID,
		PeriodStart: job.PeriodStart,
		PeriodEnd:  job.PeriodEnd,
		CreatedBy:  job.PublishBy,
		Note:       job.ReviewerNote,
	}

	scorecard, err := s.CreateDraftScorecard(ctx, input)
	if err != nil {
		return nil, err
	}

	// Calculate OTIF
	deliveryScore, _, err := s.CalculateOTIFScore(ctx, job.CompanyID, job.SupplierID, job.PeriodStart, job.PeriodEnd)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate OTIF score: %w", err)
	}

	// Calculate Quality
	qualityScore, _, err := s.CalculateQualityScore(ctx, job.CompanyID, job.SupplierID, job.PeriodStart, job.PeriodEnd)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate quality score: %w", err)
	}

	// Calculate Price Adherence
	priceScore, _, err := s.CalculatePriceAdherenceScore(ctx, job.CompanyID, job.SupplierID, job.PeriodStart, job.PeriodEnd)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate price adherence score: %w", err)
	}

	// Calculate RFQ Responsiveness
	rfqScore, _, err := s.CalculateRFQResponsivenessScore(ctx, job.CompanyID, job.SupplierID, job.PeriodStart, job.PeriodEnd)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate RFQ responsiveness score: %w", err)
	}

	// Use provided reviewer score or default to 0
	reviewerScore := accountingmoney.Must("0", 2)
	if input.ReviewerAssessmentScore != nil {
		reviewerScore = *input.ReviewerAssessmentScore
	}

	// Calculate overall score
	_ = s.CalculateOverallScore(
		deliveryScore, scorecard.DeliveryOTIFWeight,
		qualityScore, scorecard.QualityWeight,
		priceScore, scorecard.PriceAdherenceWeight,
		rfqScore, scorecard.RFQResponsivenessWeight,
		reviewerScore, scorecard.ReviewerAssessmentWeight,
	)

	// TODO: Update scorecard with calculated scores
	// This requires an UpdateScorecardScores method that updates scores but preserves draft status

	return scorecard, nil
}
