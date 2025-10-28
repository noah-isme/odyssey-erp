package procurement

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/odyssey-erp/odyssey-erp/internal/inventory"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// RepositoryPort describes repository operations used by Service.
type RepositoryPort interface {
	WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error
	GetPR(ctx context.Context, id int64) (PurchaseRequest, []PRLine, error)
	GetPO(ctx context.Context, id int64) (PurchaseOrder, []POLine, error)
	GetGRN(ctx context.Context, id int64) (GoodsReceipt, []GRNLine, error)
	GetAPInvoice(ctx context.Context, id int64) (APInvoice, error)
	ListAPOutstanding(ctx context.Context) ([]APInvoice, error)
}

// InventoryPort exposes required inventory integration.
type InventoryPort interface {
	PostInbound(ctx context.Context, input inventory.InboundInput) (inventory.StockCardEntry, error)
}

// AuditPort reused from shared.
type AuditPort interface {
	Record(ctx context.Context, log shared.AuditLog) error
}

// Service orchestrates procurement flows.
type Service struct {
	repo        RepositoryPort
	inventory   InventoryPort
	approvals   *shared.ApprovalRecorder
	audit       AuditPort
	idempotency *shared.IdempotencyStore
}

// NewService constructs procurement service.
func NewService(repo RepositoryPort, inventory InventoryPort, approvals *shared.ApprovalRecorder, audit AuditPort, idem *shared.IdempotencyStore) *Service {
	return &Service{repo: repo, inventory: inventory, approvals: approvals, audit: audit, idempotency: idem}
}

// CreatePRInput describes creation payload.
type CreatePRInput struct {
	Number     string
	SupplierID int64
	RequestBy  int64
	Note       string
	Lines      []PRLineInput
}

// PRLineInput describes request line.
type PRLineInput struct {
	ProductID int64
	Qty       float64
	Note      string
}

// CreatePOInput defines data to create PO from PR.
type CreatePOInput struct {
	PRID         int64
	Number       string
	Currency     string
	ExpectedDate time.Time
	Note         string
}

// CreateGRNInput describes GRN creation.
type CreateGRNInput struct {
	POID        int64
	WarehouseID int64
	SupplierID  int64
	Number      string
	ReceivedAt  time.Time
	Note        string
	Lines       []GRNLineInput
}

// GRNLineInput for GRN.
type GRNLineInput struct {
	ProductID int64
	Qty       float64
	UnitCost  float64
}

// APInvoiceInput for invoice creation.
type APInvoiceInput struct {
	GRNID   int64
	Number  string
	DueDate time.Time
}

// PaymentInput describes payment info.
type PaymentInput struct {
	APInvoiceID int64
	Number      string
	Amount      float64
}

// CreatePurchaseRequest persists PR header and lines.
func (s *Service) CreatePurchaseRequest(ctx context.Context, input CreatePRInput) (PurchaseRequest, error) {
	if len(input.Lines) == 0 {
		return PurchaseRequest{}, fmt.Errorf("procurement: minimal 1 line")
	}
	if input.Number == "" {
		input.Number = generateNumber("PR")
	}
	pr := PurchaseRequest{Number: input.Number, SupplierID: input.SupplierID, RequestBy: input.RequestBy, Status: PRStatusDraft, Note: input.Note}
	var created PurchaseRequest
	err := s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		prID, err := tx.CreatePR(ctx, pr)
		if err != nil {
			return err
		}
		for _, line := range input.Lines {
			if line.ProductID == 0 || line.Qty <= 0 {
				return ErrValidation
			}
			if err := tx.InsertPRLine(ctx, PRLine{PRID: prID, ProductID: line.ProductID, Qty: line.Qty, Note: line.Note}); err != nil {
				return err
			}
		}
		created = pr
		created.ID = prID
		return nil
	})
	if err != nil {
		return PurchaseRequest{}, err
	}
	s.recordAudit(ctx, "PR_CREATE", created.ID, map[string]any{"number": created.Number})
	return created, nil
}

// SubmitPurchaseRequest transitions PR to SUBMITTED.
func (s *Service) SubmitPurchaseRequest(ctx context.Context, prID int64, actorID int64) error {
	pr, _, err := s.repo.GetPR(ctx, prID)
	if err != nil {
		return err
	}
	if pr.Status != PRStatusDraft {
		return ErrInvalidState
	}
	return s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		if err := tx.UpdatePRStatus(ctx, prID, PRStatusSubmitted); err != nil {
			return err
		}
		return nil
	})
}

// CreatePOFromPR converts PR to PO with identical lines.
func (s *Service) CreatePOFromPR(ctx context.Context, input CreatePOInput) (PurchaseOrder, error) {
	pr, lines, err := s.repo.GetPR(ctx, input.PRID)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if pr.Status == PRStatusClosed {
		return PurchaseOrder{}, ErrInvalidState
	}
	if input.Number == "" {
		input.Number = generateNumber("PO")
	}
	po := PurchaseOrder{Number: input.Number, SupplierID: pr.SupplierID, Status: POStatusDraft, Currency: defaultString(input.Currency, "IDR"), ExpectedDate: input.ExpectedDate, Note: input.Note}
	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		poID, err := tx.CreatePO(ctx, po)
		if err != nil {
			return err
		}
		for _, line := range lines {
			if err := tx.InsertPOLine(ctx, POLine{POID: poID, ProductID: line.ProductID, Qty: line.Qty, Price: 0, Note: line.Note}); err != nil {
				return err
			}
		}
		created := PurchaseOrder{ID: poID, Number: po.Number, SupplierID: po.SupplierID, Status: po.Status, Currency: po.Currency, ExpectedDate: po.ExpectedDate, Note: po.Note}
		po = created
		return nil
	})
	if err != nil {
		return PurchaseOrder{}, err
	}
	s.recordAudit(ctx, "PO_CREATE", po.ID, map[string]any{"number": po.Number, "from_pr": input.PRID})
	return po, nil
}

// SubmitPurchaseOrder requests approval.
func (s *Service) SubmitPurchaseOrder(ctx context.Context, poID int64, actorID int64) error {
	po, _, err := s.repo.GetPO(ctx, poID)
	if err != nil {
		return err
	}
	if po.Status != POStatusDraft {
		return ErrInvalidState
	}
	refID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("PO:%d", poID)))
	return s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		if err := tx.UpdatePOStatus(ctx, poID, POStatusApproval); err != nil {
			return err
		}
		if s.approvals != nil {
			_ = s.approvals.EnsureSubmit(ctx, "PO", refID, actorID, fmt.Sprintf("PO %s submitted", po.Number))
		}
		return nil
	})
}

// ApprovePurchaseOrder marks PO as approved and logs approval.
func (s *Service) ApprovePurchaseOrder(ctx context.Context, poID int64, actorID int64) error {
	po, _, err := s.repo.GetPO(ctx, poID)
	if err != nil {
		return err
	}
	if po.Status != POStatusApproval {
		return ErrInvalidState
	}
	now := time.Now()
	refID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("PO:%d", poID)))
	return s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		if err := tx.UpdatePOStatus(ctx, poID, POStatusApproved); err != nil {
			return err
		}
		if err := tx.SetPOApproval(ctx, poID, actorID, now); err != nil {
			return err
		}
		if s.approvals != nil {
			_ = s.approvals.Record(ctx, shared.ApprovalLog{Module: "PO", RefID: refID, ActorID: actorID, Action: shared.ApprovalApprove, Note: fmt.Sprintf("PO %s approved", po.Number)})
		}
		return nil
	})
}

// CreateGoodsReceipt inserts GRN and lines.
func (s *Service) CreateGoodsReceipt(ctx context.Context, input CreateGRNInput) (GoodsReceipt, error) {
	if input.Number == "" {
		input.Number = generateNumber("GRN")
	}
	if input.SupplierID == 0 {
		po, _, err := s.repo.GetPO(ctx, input.POID)
		if err != nil {
			return GoodsReceipt{}, err
		}
		input.SupplierID = po.SupplierID
	}
	if len(input.Lines) == 0 {
		return GoodsReceipt{}, ErrValidation
	}
	grn := GoodsReceipt{Number: input.Number, POID: input.POID, SupplierID: input.SupplierID, WarehouseID: input.WarehouseID, Status: GRNStatusDraft, ReceivedAt: defaultTime(input.ReceivedAt), Note: input.Note}
	err := s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		grnID, err := tx.CreateGRN(ctx, grn)
		if err != nil {
			return err
		}
		grn.ID = grnID
		for _, line := range input.Lines {
			if line.ProductID == 0 || line.Qty <= 0 {
				return ErrValidation
			}
			if err := tx.InsertGRNLine(ctx, GRNLine{GRNID: grnID, ProductID: line.ProductID, Qty: line.Qty, UnitCost: line.UnitCost}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return GoodsReceipt{}, err
	}
	s.recordAudit(ctx, "GRN_CREATE", grn.ID, map[string]any{"number": grn.Number})
	return grn, nil
}

// PostGoodsReceipt posts GRN and updates inventory.
func (s *Service) PostGoodsReceipt(ctx context.Context, grnID int64) error {
	grn, lines, err := s.repo.GetGRN(ctx, grnID)
	if err != nil {
		return err
	}
	if grn.Status != GRNStatusDraft {
		return ErrInvalidState
	}
	key := fmt.Sprintf("GRN:%s", grn.Number)
	inserted := false
	if s.idempotency != nil {
		if err := s.idempotency.CheckAndInsert(ctx, key, "procurement.grn"); err != nil {
			return err
		}
		inserted = true
	}
	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		if err := tx.UpdateGRNStatus(ctx, grnID, GRNStatusPosted); err != nil {
			return err
		}
		for _, line := range lines {
			if s.inventory == nil {
				return errors.New("inventory integration not configured")
			}
			refID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("GRN:%d:%d", grn.ID, line.ProductID)))
			_, err := s.inventory.PostInbound(ctx, inventory.InboundInput{
				Code:        fmt.Sprintf("GRN-%s-%d", grn.Number, line.ProductID),
				WarehouseID: grn.WarehouseID,
				ProductID:   line.ProductID,
				Qty:         line.Qty,
				UnitCost:    line.UnitCost,
				Note:        fmt.Sprintf("GRN %s", grn.Number),
				ActorID:     0,
				RefModule:   "PROCUREMENT",
				RefID:       refID.String(),
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if inserted {
			_ = s.idempotency.Delete(ctx, key)
		}
		return err
	}
	s.recordAudit(ctx, "GRN_POST", grnID, map[string]any{"number": grn.Number})
	return nil
}

// CreateAPInvoiceFromGRN sums GRN lines into AP invoice.
func (s *Service) CreateAPInvoiceFromGRN(ctx context.Context, input APInvoiceInput) (APInvoice, error) {
	grn, lines, err := s.repo.GetGRN(ctx, input.GRNID)
	if err != nil {
		return APInvoice{}, err
	}
	if grn.Status != GRNStatusPosted {
		return APInvoice{}, ErrInvalidState
	}
	var total float64
	for _, line := range lines {
		total += line.Qty * line.UnitCost
	}
	if total < 0 {
		total = 0
	}
	inv := APInvoice{Number: input.Number, SupplierID: grn.SupplierID, GRNID: grn.ID, Currency: "IDR", Total: math.Round(total*100) / 100, Status: APStatusDraft, DueAt: input.DueDate}
	if inv.Number == "" {
		inv.Number = generateNumber("AP")
	}
	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		id, err := tx.CreateAPInvoice(ctx, inv)
		if err != nil {
			return err
		}
		inv.ID = id
		return nil
	})
	if err != nil {
		return APInvoice{}, err
	}
	s.recordAudit(ctx, "AP_CREATE", inv.ID, map[string]any{"number": inv.Number, "total": inv.Total})
	return inv, nil
}

// PostAPInvoice changes status to POSTED.
func (s *Service) PostAPInvoice(ctx context.Context, invoiceID int64) error {
	inv, err := s.repo.GetAPInvoice(ctx, invoiceID)
	if err != nil {
		return err
	}
	if inv.Status != APStatusDraft {
		return ErrInvalidState
	}
	return s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		return tx.UpdateAPStatus(ctx, invoiceID, APStatusPosted)
	})
}

// RegisterPayment records payment and marks invoice paid if fully settled.
func (s *Service) RegisterPayment(ctx context.Context, input PaymentInput) error {
	if input.Amount <= 0 {
		return ErrValidation
	}
	target, err := s.repo.GetAPInvoice(ctx, input.APInvoiceID)
	if err != nil {
		return err
	}
	return s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		if input.Number == "" {
			input.Number = generateNumber("PAY")
		}
		if _, err := tx.CreatePayment(ctx, APPayment{Number: input.Number, APInvoiceID: input.APInvoiceID, Amount: input.Amount}); err != nil {
			return err
		}
		if input.Amount >= target.Total {
			if err := tx.UpdateAPStatus(ctx, input.APInvoiceID, APStatusPaid); err != nil {
				return err
			}
		}
		return nil
	})
}

// APAgingBucket summarises totals.
type APAgingBucket struct {
	Current   float64
	Bucket30  float64
	Bucket60  float64
	Bucket90  float64
	Bucket120 float64
}

// CalculateAPAging groups invoices by due date buckets.
func (s *Service) CalculateAPAging(ctx context.Context, asOf time.Time) (APAgingBucket, error) {
	invoices, err := s.repo.ListAPOutstanding(ctx)
	if err != nil {
		return APAgingBucket{}, err
	}
	if asOf.IsZero() {
		asOf = time.Now()
	}
	var bucket APAgingBucket
	for _, inv := range invoices {
		if inv.Status == APStatusPaid {
			continue
		}
		days := int(asOf.Sub(inv.DueAt).Hours() / 24)
		switch {
		case days <= 0:
			bucket.Current += inv.Total
		case days <= 30:
			bucket.Bucket30 += inv.Total
		case days <= 60:
			bucket.Bucket60 += inv.Total
		case days <= 90:
			bucket.Bucket90 += inv.Total
		default:
			bucket.Bucket120 += inv.Total
		}
	}
	return bucket, nil
}

func (s *Service) recordAudit(ctx context.Context, action string, entityID int64, meta map[string]any) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Record(ctx, shared.AuditLog{ActorID: 0, Action: action, Entity: "procurement", EntityID: fmt.Sprintf("%d", entityID), Meta: meta})
}

func generateNumber(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func defaultString(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func defaultTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now()
	}
	return value
}
