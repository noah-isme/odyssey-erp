package procurement

import (
	"context"
	"fmt"
	"math/big"
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

// CalculateOTIFScore calculates on-time delivery performance from posted GRNs.
// The current schema exposes receipt timing but not a separate promised quantity
// snapshot, so the sample is receipt-level until that evidence is added.
func (s *ScorecardService) CalculateOTIFScore(ctx context.Context, companyID, supplierID int64, periodStart, periodEnd time.Time) (score accountingmoney.Money, sampleSize int, err error) {
	ontime, total, err := s.repo.CalculateOTIFScore(ctx, companyID, supplierID, periodStart, periodEnd)
	if err != nil {
		return accountingmoney.Money{}, 0, err
	}
	sampleSize = int(total)
	if total == 0 {
		return accountingmoney.Must("0.00", 2), 0, nil
	}
	score = scoreFromCounts(ontime, total)
	return score, sampleSize, nil
}

// CalculateQualityScore calculates accepted quantity as a percentage of the
// accepted and rejected/returned quantities recorded on GRN lines.
func (s *ScorecardService) CalculateQualityScore(ctx context.Context, companyID, supplierID int64, periodStart, periodEnd time.Time) (score accountingmoney.Money, sampleSize int, err error) {
	accepted, total, err := s.repo.CalculateQualityScore(ctx, companyID, supplierID, periodStart, periodEnd)
	if err != nil {
		return accountingmoney.Money{}, 0, err
	}
	sampleSize = int(total)
	if total == 0 {
		return accountingmoney.Must("0.00", 2), 0, nil
	}
	score = scoreFromCounts(accepted, total)
	return score, sampleSize, nil
}

// CalculatePriceAdherenceScore calculates the percentage of PO lines without a
// pending contract variance in the scoring period.
func (s *ScorecardService) CalculatePriceAdherenceScore(ctx context.Context, companyID, supplierID int64, periodStart, periodEnd time.Time) (score accountingmoney.Money, sampleSize int, err error) {
	compliant, total, err := s.repo.CalculatePriceAdherenceScore(ctx, companyID, supplierID, periodStart, periodEnd)
	if err != nil {
		return accountingmoney.Money{}, 0, err
	}
	sampleSize = int(total)
	if total == 0 {
		return accountingmoney.Must("0.00", 2), 0, nil
	}
	score = scoreFromCounts(compliant, total)
	return score, sampleSize, nil
}

// CalculateRFQResponsivenessScore calculates the percentage of invited RFQs
// that have a supplier bid in the scoring period.
func (s *ScorecardService) CalculateRFQResponsivenessScore(ctx context.Context, companyID, supplierID int64, periodStart, periodEnd time.Time) (score accountingmoney.Money, sampleSize int, err error) {
	responded, total, err := s.repo.CalculateRFQResponsivenessScore(ctx, companyID, supplierID, periodStart, periodEnd)
	if err != nil {
		return accountingmoney.Money{}, 0, err
	}
	sampleSize = int(total)
	if total == 0 {
		return accountingmoney.Must("0.00", 2), 0, nil
	}
	score = scoreFromCounts(responded, total)
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
	if totalWeight <= 0 {
		return accountingmoney.Must("0.00", 2)
	}

	weighted := new(big.Rat)
	addWeighted := func(score accountingmoney.Money, weight int) {
		if weight <= 0 {
			return
		}
		scoreRat, ok := new(big.Rat).SetString(score.String())
		if !ok {
			return
		}
		weighted.Add(weighted, new(big.Rat).Mul(scoreRat, big.NewRat(int64(weight), 1)))
	}
	addWeighted(deliveryScore, deliveryWeight)
	addWeighted(qualityScore, qualityWeight)
	addWeighted(priceScore, priceWeight)
	addWeighted(rfqScore, rfqWeight)
	addWeighted(reviewerScore, reviewerWeight)
	weighted.Quo(weighted, big.NewRat(int64(totalWeight), 1))
	return accountingmoney.Must(weighted.FloatString(2), 2)
}

func scoreFromCounts(successes, total int64) accountingmoney.Money {
	if total <= 0 {
		return accountingmoney.Must("0.00", 2)
	}
	ratio := new(big.Rat).Quo(big.NewRat(successes, 1), big.NewRat(total, 1))
	ratio.Mul(ratio, big.NewRat(100, 1))
	return accountingmoney.Must(ratio.FloatString(2), 2)
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
	if companyID == 0 || supplierID == 0 {
		return nil, fmt.Errorf("company ID and supplier ID are required")
	}
	return s.repo.GetLatestPublishedScorecard(ctx, companyID, supplierID)
}

// ScorecardCalculationJob represents a scheduled scorecard calculation and publication
// This would be run by a background job scheduler (e.g., monthly)
type ScorecardCalculationJob struct {
	CompanyID               int64
	SupplierID              int64
	PeriodStart             time.Time
	PeriodEnd               time.Time
	PublishBy               int64 // User ID who can publish the draft
	ReviewerNote            string
	ReviewerAssessmentScore *accountingmoney.Money
}

// ExecuteScorecardCalculation calculates all score components and creates a draft scorecard
// Caller must then review and publish the scorecard
func (s *ScorecardService) ExecuteScorecardCalculation(ctx context.Context, job ScorecardCalculationJob) (*SupplierScorecard, error) {
	// Create draft scorecard
	input := CreateScorecardInput{
		CompanyID:               job.CompanyID,
		SupplierID:              job.SupplierID,
		PeriodStart:             job.PeriodStart,
		PeriodEnd:               job.PeriodEnd,
		CreatedBy:               job.PublishBy,
		ReviewerAssessmentScore: job.ReviewerAssessmentScore,
		Note:                    job.ReviewerNote,
	}

	scorecard, err := s.CreateDraftScorecard(ctx, input)
	if err != nil {
		return nil, err
	}

	// Calculate OTIF
	deliveryScore, deliverySampleSize, err := s.CalculateOTIFScore(ctx, job.CompanyID, job.SupplierID, job.PeriodStart, job.PeriodEnd)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate OTIF score: %w", err)
	}

	// Calculate Quality
	qualityScore, qualitySampleSize, err := s.CalculateQualityScore(ctx, job.CompanyID, job.SupplierID, job.PeriodStart, job.PeriodEnd)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate quality score: %w", err)
	}

	// Calculate Price Adherence
	priceScore, priceSampleSize, err := s.CalculatePriceAdherenceScore(ctx, job.CompanyID, job.SupplierID, job.PeriodStart, job.PeriodEnd)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate price adherence score: %w", err)
	}

	// Calculate RFQ Responsiveness
	rfqScore, rfqSampleSize, err := s.CalculateRFQResponsivenessScore(ctx, job.CompanyID, job.SupplierID, job.PeriodStart, job.PeriodEnd)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate RFQ responsiveness score: %w", err)
	}

	reviewerScore := accountingmoney.Must("0", 2)
	if job.ReviewerAssessmentScore != nil {
		reviewerScore = *job.ReviewerAssessmentScore
	}

	// Categories without evidence and an omitted reviewer assessment do not
	// dilute the score; the remaining weights are renormalized by the weighted
	// average calculation.
	deliveryWeight := scorecard.DeliveryOTIFWeight
	if deliverySampleSize == 0 {
		deliveryWeight = 0
	}
	qualityWeight := scorecard.QualityWeight
	if qualitySampleSize == 0 {
		qualityWeight = 0
	}
	priceWeight := scorecard.PriceAdherenceWeight
	if priceSampleSize == 0 {
		priceWeight = 0
	}
	rfqWeight := scorecard.RFQResponsivenessWeight
	if rfqSampleSize == 0 {
		rfqWeight = 0
	}
	reviewerWeight := scorecard.ReviewerAssessmentWeight
	if job.ReviewerAssessmentScore == nil {
		reviewerWeight = 0
	}
	overallScore := s.CalculateOverallScore(
		deliveryScore, deliveryWeight,
		qualityScore, qualityWeight,
		priceScore, priceWeight,
		rfqScore, rfqWeight,
		reviewerScore, reviewerWeight,
	)
	if err := s.repo.UpdateScorecardScores(ctx, scorecard.ID,
		deliveryScore, deliverySampleSize,
		qualityScore, qualitySampleSize,
		priceScore, priceSampleSize,
		rfqScore, rfqSampleSize,
		reviewerScore, overallScore); err != nil {
		return nil, err
	}
	return s.repo.GetScorecard(ctx, scorecard.ID)
}
