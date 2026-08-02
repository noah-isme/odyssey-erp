package procurement

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
	approvalengine "github.com/odyssey-erp/odyssey-erp/internal/approvals"
	"github.com/odyssey-erp/odyssey-erp/internal/fx"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// RFQEmailQueue is deliberately small so issuing an RFQ is testable without
// coupling the sourcing domain to Asynq.
type RFQEmailQueue interface {
	EnqueueRFQ(context.Context, []string, string, string) error
}

type SourcingService struct {
	repo        *SourcingRepository
	approvals   *approvalengine.Service
	audit       AuditPort
	idempotency *shared.IdempotencyStore
	emails      RFQEmailQueue
}

func NewSourcingService(repo *SourcingRepository, approvals *approvalengine.Service, audit AuditPort, idempotency *shared.IdempotencyStore, emails RFQEmailQueue) *SourcingService {
	return &SourcingService{repo: repo, approvals: approvals, audit: audit, idempotency: idempotency, emails: emails}
}

func (s *SourcingService) CreateRFQ(ctx context.Context, input CreateRFQInput) (RFQ, error) {
	if s == nil || s.repo == nil || input.CompanyID <= 0 || input.CreatedBy <= 0 || input.ResponseDueAt.IsZero() || len(input.Lines) == 0 || len(input.SupplierIDs) == 0 {
		return RFQ{}, ErrValidation
	}
	if input.ResponseDueAt.Before(time.Now()) {
		return RFQ{}, ErrValidation
	}
	if input.Weights == (RFQWeights{}) {
		input.Weights = DefaultRFQWeights()
	}
	if !input.Weights.Valid() {
		return RFQ{}, ErrValidation
	}
	currency, err := fx.Currency(input.Currency)
	if err != nil {
		return RFQ{}, ErrValidation
	}
	seenSuppliers := map[int64]struct{}{}
	rfq := RFQ{CompanyID: input.CompanyID, Number: strings.TrimSpace(input.Number), Currency: currency, Status: RFQStatusDraft, ResponseDueAt: input.ResponseDueAt, CommercialTerms: strings.TrimSpace(input.CommercialTerms), Weights: input.Weights, CreatedBy: input.CreatedBy}
	if rfq.Number == "" {
		rfq.Number = generateNumber("RFQ")
	}
	for index, line := range input.Lines {
		if line.ProductID <= 0 || !positiveDecimal(line.Quantity) {
			return RFQ{}, ErrValidation
		}
		rfq.Lines = append(rfq.Lines, RFQLine{PRLineID: line.PRLineID, ProductID: line.ProductID, Quantity: canonicalDecimal(line.Quantity, 4), Note: strings.TrimSpace(line.Note), LineOrder: index + 1})
	}
	for _, supplierID := range input.SupplierIDs {
		if supplierID <= 0 {
			return RFQ{}, ErrValidation
		}
		if _, exists := seenSuppliers[supplierID]; exists {
			return RFQ{}, ErrValidation
		}
		seenSuppliers[supplierID] = struct{}{}
		rfq.Suppliers = append(rfq.Suppliers, supplierID)
	}
	created, err := s.repo.CreateRFQ(ctx, rfq)
	if err != nil {
		return RFQ{}, err
	}
	s.record(ctx, "RFQ_CREATE", created.ID, map[string]any{"number": created.Number, "supplier_count": len(created.Suppliers)})
	return created, nil
}

func (s *SourcingService) IssueRFQ(ctx context.Context, rfqID, actorID int64) error {
	rfq, err := s.repo.GetRFQ(ctx, rfqID)
	if err != nil {
		return err
	}
	if rfq.Status != RFQStatusDraft || actorID <= 0 {
		return ErrInvalidState
	}
	emails, err := s.repo.RFQSupplierEmails(ctx, rfqID)
	if err != nil {
		return err
	}
	messageRef := fmt.Sprintf("rfq-%d-v%d", rfq.ID, rfq.Version+1)
	if s.emails != nil && len(emails) > 0 {
		if err := s.emails.EnqueueRFQ(ctx, emails, fmt.Sprintf("Request for quotation %s", rfq.Number), fmt.Sprintf("RFQ %s is due on %s.", rfq.Number, rfq.ResponseDueAt.Format("2006-01-02 15:04 MST"))); err != nil {
			return err
		}
	}
	if err := s.repo.UpdateRFQStatus(ctx, rfqID, RFQStatusDraft, RFQStatusIssued); err != nil {
		return err
	}
	if err := s.repo.MarkInvitationsSent(ctx, rfqID, messageRef); err != nil {
		return err
	}
	s.record(ctx, "RFQ_ISSUE", rfqID, map[string]any{"message_ref": messageRef, "recipients": len(emails), "actor_id": actorID})
	return nil
}

func (s *SourcingService) CreateBid(ctx context.Context, input CreateBidInput) (int64, error) {
	if s == nil || s.repo == nil || input.RFQID <= 0 || input.SupplierID <= 0 || input.CompanyID <= 0 || input.CreatedBy <= 0 || len(input.Lines) == 0 || input.FXRateDate.IsZero() {
		return 0, ErrValidation
	}
	currency, err := fx.Currency(input.Currency)
	if err != nil {
		return 0, ErrValidation
	}
	rate, err := fx.ParseDecimal(input.FXRate)
	if err != nil || rate.Cmp(fx.MustDecimal("0")) <= 0 {
		return 0, ErrValidation
	}
	input.Currency = currency
	stored := make([]storedBidLine, 0, len(input.Lines))
	seen := map[int64]struct{}{}
	for _, line := range input.Lines {
		if line.RFQLineID <= 0 || !positiveDecimal(line.Quantity) || line.LeadTimeDays < 0 || line.CommercialScore < 0 || line.CommercialScore > 100 || line.SupplierRatingScore < 0 || line.SupplierRatingScore > 100 {
			return 0, ErrValidation
		}
		if _, exists := seen[line.RFQLineID]; exists {
			return 0, ErrValidation
		}
		seen[line.RFQLineID] = struct{}{}
		if err := validateMoney(line.UnitPrice); err != nil {
			return 0, err
		}
		if err := validateMoney(line.TaxAmount); err != nil {
			return 0, err
		}
		if err := validateMoney(line.FreightAmount); err != nil {
			return 0, err
		}
		unitBase, err := moneyBase(line.UnitPrice, rate, 4)
		if err != nil {
			return 0, err
		}
		taxBase, err := moneyBase(line.TaxAmount, rate, 4)
		if err != nil {
			return 0, err
		}
		freightBase, err := moneyBase(line.FreightAmount, rate, 4)
		if err != nil {
			return 0, err
		}
		moq := "0"
		if strings.TrimSpace(line.MinimumOrderQuantity) != "" {
			if !nonNegativeDecimal(line.MinimumOrderQuantity) {
				return 0, ErrValidation
			}
			moq = canonicalDecimal(line.MinimumOrderQuantity, 4)
		}
		stored = append(stored, storedBidLine{RFQLineID: line.RFQLineID, Quantity: canonicalDecimal(line.Quantity, 4), UnitPrice: line.UnitPrice.String(), UnitPriceBase: unitBase, TaxAmount: line.TaxAmount.String(), FreightAmount: line.FreightAmount.String(), TaxAmountBase: taxBase, FreightAmountBase: freightBase, MinimumOrderQuantity: moq, LeadTimeDays: line.LeadTimeDays, CommercialScore: line.CommercialScore, SupplierRatingScore: line.SupplierRatingScore, Note: strings.TrimSpace(line.Note)})
	}
	id, err := s.repo.CreateBid(ctx, input, stored)
	if err != nil {
		return 0, err
	}
	s.record(ctx, "RFQ_BID_CREATE", id, map[string]any{"rfq_id": input.RFQID, "supplier_id": input.SupplierID, "source_reference": input.SourceReference})
	return id, nil
}

func (s *SourcingService) SubmitBid(ctx context.Context, bidID, actorID int64) error {
	if actorID <= 0 {
		return ErrValidation
	}
	if err := s.repo.UpdateBidStatus(ctx, bidID, BidStatusDraft, BidStatusSubmitted); err != nil {
		return err
	}
	s.record(ctx, "RFQ_BID_SUBMIT", bidID, map[string]any{"actor_id": actorID})
	return nil
}

func (s *SourcingService) CloseRFQ(ctx context.Context, rfqID, actorID int64) error {
	if actorID <= 0 {
		return ErrValidation
	}
	if err := s.repo.UpdateRFQStatus(ctx, rfqID, RFQStatusIssued, RFQStatusClosed); err != nil {
		return err
	}
	s.record(ctx, "RFQ_CLOSE", rfqID, map[string]any{"actor_id": actorID})
	return nil
}

func (s *SourcingService) SnapshotComparison(ctx context.Context, rfqID, actorID int64) ([]ComparisonEntry, error) {
	rfq, err := s.repo.GetRFQ(ctx, rfqID)
	if err != nil {
		return nil, err
	}
	if rfq.Status != RFQStatusClosed || actorID <= 0 {
		return nil, ErrInvalidState
	}
	candidates, err := s.repo.ComparisonCandidates(ctx, rfqID)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, ErrValidation
	}
	entries, err := calculateComparison(rfq.Weights, candidates)
	if err != nil {
		return nil, err
	}
	if err := s.repo.SaveComparison(ctx, rfqID, rfq.CompanyID, rfq.Version, actorID, entries); err != nil {
		return nil, err
	}
	s.record(ctx, "RFQ_COMPARE", rfqID, map[string]any{"entries": len(entries), "actor_id": actorID})
	return entries, nil
}

func (s *SourcingService) CreateAward(ctx context.Context, input CreateAwardInput) (Award, error) {
	if input.RFQID <= 0 || input.CompanyID <= 0 || input.ExpectedWarehouseID <= 0 || input.CreatedBy <= 0 || len(input.Lines) == 0 {
		return Award{}, ErrValidation
	}
	seen := map[string]struct{}{}
	for _, line := range input.Lines {
		if line.RFQLineID <= 0 || line.BidLineID <= 0 || !positiveDecimal(line.Quantity) {
			return Award{}, ErrValidation
		}
		key := fmt.Sprintf("%d:%d", line.RFQLineID, line.BidLineID)
		if _, exists := seen[key]; exists {
			return Award{}, ErrValidation
		}
		seen[key] = struct{}{}
	}
	award, err := s.repo.CreateAward(ctx, Award{RFQID: input.RFQID, CompanyID: input.CompanyID, ExpectedWarehouseID: input.ExpectedWarehouseID, Status: AwardStatusDraft, Note: strings.TrimSpace(input.Note), CreatedBy: input.CreatedBy}, input.Lines)
	if err != nil {
		return Award{}, err
	}
	s.record(ctx, "RFQ_AWARD_CREATE", award.ID, map[string]any{"rfq_id": award.RFQID, "actor_id": input.CreatedBy})
	return award, nil
}

func (s *SourcingService) SubmitAward(ctx context.Context, awardID, actorID int64) error {
	award, err := s.repo.GetAward(ctx, awardID)
	if err != nil {
		return err
	}
	if award.Status != AwardStatusDraft || actorID <= 0 {
		return ErrInvalidState
	}
	if s.approvals != nil {
		companyID := award.CompanyID
		// Exact award amounts remain in sourcing records. The shared approval
		// engine currently accepts float64 thresholds, so a zero amount routes
		// through a module/company policy without creating a lossy money boundary.
		if _, err := s.approvals.Submit(ctx, approvalengine.Submission{Module: "RFQ_AWARD", DocumentID: awardID, RequesterID: actorID, CompanyID: &companyID, Amount: 0}); err != nil {
			return err
		}
	}
	if err := s.repo.UpdateAwardStatus(ctx, awardID, AwardStatusDraft, AwardStatusApproval, 0); err != nil {
		return err
	}
	s.record(ctx, "RFQ_AWARD_SUBMIT", awardID, map[string]any{"actor_id": actorID})
	return nil
}

func (s *SourcingService) FinalizeApproval(ctx context.Context, request approvalengine.Request, status string, actorID int64, note string) error {
	if request.Module != "RFQ_AWARD" {
		return errors.New("unsupported approval module")
	}
	if status == approvalengine.StatusApproved {
		if err := s.repo.UpdateAwardStatus(ctx, request.DocumentID, AwardStatusApproval, AwardStatusApproved, actorID); err != nil {
			return err
		}
		key := fmt.Sprintf("rfq-award-po:%d", request.DocumentID)
		inserted := false
		if s.idempotency != nil {
			if err := s.idempotency.CheckAndInsert(ctx, key, "procurement.rfq_award"); err != nil {
				return err
			}
			inserted = true
		}
		if err := s.repo.GenerateAwardPOs(ctx, request.DocumentID); err != nil {
			if inserted {
				_ = s.idempotency.Delete(ctx, key)
			}
			return err
		}
		s.record(ctx, "RFQ_AWARD_APPROVE", request.DocumentID, map[string]any{"actor_id": actorID, "note": note})
		return nil
	}
	if err := s.repo.UpdateAwardStatus(ctx, request.DocumentID, AwardStatusApproval, AwardStatusRejected, actorID); err != nil {
		return err
	}
	s.record(ctx, "RFQ_AWARD_REJECT", request.DocumentID, map[string]any{"actor_id": actorID, "note": note})
	return nil
}

func (s *SourcingService) record(ctx context.Context, action string, id int64, meta map[string]any) {
	if s.audit != nil {
		_ = s.audit.Record(ctx, shared.AuditLog{Action: action, Entity: "procurement_rfq", EntityID: fmt.Sprintf("%d", id), Meta: meta})
	}
}

func validateMoney(value accountingmoney.Money) error {
	if _, err := accountingmoney.Parse(value.String(), value.Scale); err != nil {
		return ErrValidation
	}
	if !nonNegativeDecimal(value.String()) {
		return ErrValidation
	}
	return nil
}

func moneyBase(value accountingmoney.Money, rate fx.Decimal, scale int) (string, error) {
	amount, err := fx.ParseDecimal(value.String())
	if err != nil {
		return "", err
	}
	if amount.Cmp(fx.MustDecimal("0")) < 0 {
		return "", ErrValidation
	}
	return amount.Mul(rate).Round(scale).String(), nil
}

func positiveDecimal(value string) bool          { return compareDecimal(value, "0") > 0 }
func nonNegativeDecimal(value string) bool       { return compareDecimal(value, "0") >= 0 }
func lessOrEqualDecimal(left, right string) bool { return compareDecimal(left, right) <= 0 }
func compareDecimal(left, right string) int {
	l, lok := new(big.Rat).SetString(strings.TrimSpace(left))
	r, rok := new(big.Rat).SetString(strings.TrimSpace(right))
	if !lok || !rok {
		return -1
	}
	return l.Cmp(r)
}
func canonicalDecimal(value string, scale int) string {
	decimal, err := fx.ParseDecimal(value)
	if err != nil {
		return ""
	}
	return decimal.Round(scale).String()
}

func sumDecimal(left, right string) string {
	a, ok := new(big.Rat).SetString(strings.TrimSpace(left))
	if !ok {
		return ""
	}
	b, ok := new(big.Rat).SetString(strings.TrimSpace(right))
	if !ok {
		return ""
	}
	return new(big.Rat).Add(a, b).FloatString(10)
}

var _ approvalengine.Finalizer = (*SourcingService)(nil)
