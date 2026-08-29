package procurement

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"time"

	"github.com/google/uuid"

	approvalengine "github.com/odyssey-erp/odyssey-erp/internal/approvals"
	"github.com/odyssey-erp/odyssey-erp/internal/fx"
	"github.com/odyssey-erp/odyssey-erp/internal/inventory"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// RepositoryPort describes repository operations used by Service.
type RepositoryPort interface {
	WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error
	GetPR(ctx context.Context, id int64) (PurchaseRequest, []PRLine, error)
	GetPO(ctx context.Context, id int64) (PurchaseOrder, []POLine, error)
	GetGRN(ctx context.Context, id int64) (GoodsReceipt, []GRNLine, error)
	ListPOs(ctx context.Context, limit, offset int, filters ListFilters) ([]POListItem, int, error)
	ListGRNs(ctx context.Context, limit, offset int, filters ListFilters) ([]GRNListItem, int, error)
	POExistsByNumber(ctx context.Context, number string) (bool, error)
}

type GoodsReturnRepositoryPort interface {
	GetGoodsReturnGRN(ctx context.Context, id int64) (GoodsReturnGRN, []GoodsReturnGRNLine, error)
	ListGoodsReturnGRNs(ctx context.Context) ([]GoodsReturnGRN, error)
}

// InventoryPort exposes required inventory integration.
type InventoryPort interface {
	PostInbound(ctx context.Context, input inventory.InboundInput) (inventory.StockCardEntry, error)
	PostAdjustment(ctx context.Context, input inventory.AdjustmentInput) (inventory.StockCardEntry, error)
}

// AuditPort reused from shared.
type AuditPort interface {
	Record(ctx context.Context, log shared.AuditLog) error
}

// Service orchestrates procurement flows.
type Service struct {
	logger         *slog.Logger
	repo           RepositoryPort
	inventory      InventoryPort
	approvals      *shared.ApprovalRecorder
	approvalEngine *approvalengine.Service
	audit          AuditPort
	idempotency    *shared.IdempotencyStore
	integration    IntegrationHandler
}

func (s *Service) SetApprovalEngine(engine *approvalengine.Service) { s.approvalEngine = engine }

// NewService constructs procurement service.
func NewService(logger *slog.Logger, repo RepositoryPort, inventory InventoryPort, approvals *shared.ApprovalRecorder, audit AuditPort, idem *shared.IdempotencyStore, integration IntegrationHandler) *Service {
	return &Service{logger: logger, repo: repo, inventory: inventory, approvals: approvals, audit: audit, idempotency: idem, integration: integration}
}

// enforceRequestCompany applies the browser tenant boundary without breaking
// the existing worker/read contract. HTTP middleware stores a Session in the
// context, so a session-bearing call must carry a complete authenticated
// identity and the loaded document must belong to that identity's company.
// Contexts without a Session are the explicit internal contract used by
// workers and cross-module jobs; those callers already receive their company
// through their own trusted event/input payloads.
func enforceRequestCompany(ctx context.Context, companyID int64) error {
	if shared.SessionFromContext(ctx) == nil {
		return nil
	}
	identity, ok := shared.IdentityFromContext(ctx)
	if !ok || identity.CompanyID <= 0 || companyID <= 0 {
		return ErrCompanyScopeRequired
	}
	if identity.CompanyID != companyID {
		return ErrCompanyScopeMismatch
	}
	return nil
}

// CreatePRInput describes creation payload.
type CreatePRInput struct {
	CompanyID  int64
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
	PRID                int64
	Number              string
	Currency            string
	ExpectedDate        time.Time
	ExpectedWarehouseID int64
	Note                string
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
	ProductID     int64
	Qty           float64
	UnitCost      float64
	LotNumber     string
	ExpiryDate    *time.Time
	SerialNumbers []string
}

// CreatePurchaseRequest persists PR header and lines.
func (s *Service) CreatePurchaseRequest(ctx context.Context, input CreatePRInput) (PurchaseRequest, error) {
	if err := enforceRequestCompany(ctx, input.CompanyID); err != nil {
		return PurchaseRequest{}, err
	}
	if len(input.Lines) == 0 {
		return PurchaseRequest{}, fmt.Errorf("procurement: minimal 1 line")
	}
	if input.Number == "" {
		input.Number = generateNumber("PR")
	}
	pr := PurchaseRequest{Number: input.Number, CompanyID: input.CompanyID, SupplierID: input.SupplierID, RequestBy: input.RequestBy, Status: PRStatusDraft, Note: input.Note}
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
	if s.logger != nil {
		s.logger.Info("created pr", slog.String("number", created.Number), slog.Int64("id", created.ID))
	}
	return created, nil
}

// SubmitPurchaseRequest transitions PR to SUBMITTED.
func (s *Service) SubmitPurchaseRequest(ctx context.Context, prID int64, actorID int64) error {
	pr, _, err := s.repo.GetPR(ctx, prID)
	if err != nil {
		return err
	}
	if err := enforceRequestCompany(ctx, pr.CompanyID); err != nil {
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
	if input.ExpectedWarehouseID <= 0 {
		return PurchaseOrder{}, ErrValidation
	}
	pr, lines, err := s.repo.GetPR(ctx, input.PRID)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if err := enforceRequestCompany(ctx, pr.CompanyID); err != nil {
		return PurchaseOrder{}, err
	}
	if pr.Status != PRStatusSubmitted {
		return PurchaseOrder{}, ErrInvalidState
	}
	if input.Number == "" {
		input.Number = generateNumber("PO")
	}

	exists, err := s.repo.POExistsByNumber(ctx, input.Number)
	if err != nil {
		return PurchaseOrder{}, err
	}
	if exists {
		return PurchaseOrder{}, fmt.Errorf("PO number %s already exists", input.Number)
	}

	po := PurchaseOrder{Number: input.Number, CompanyID: pr.CompanyID, SupplierID: pr.SupplierID, ExpectedWarehouseID: input.ExpectedWarehouseID, Status: POStatusDraft, Currency: defaultString(input.Currency, "IDR"), ExpectedDate: input.ExpectedDate, Note: input.Note}
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
		if err := tx.UpdatePRStatus(ctx, pr.ID, PRStatusClosed); err != nil {
			return err
		}
		created := PurchaseOrder{ID: poID, CompanyID: po.CompanyID, Number: po.Number, SupplierID: po.SupplierID, ExpectedWarehouseID: po.ExpectedWarehouseID, Status: po.Status, Currency: po.Currency, ExpectedDate: po.ExpectedDate, Note: po.Note}
		po = created
		return nil
	})
	if err != nil {
		return PurchaseOrder{}, err
	}
	s.recordAudit(ctx, "PO_CREATE", po.ID, map[string]any{"number": po.Number, "from_pr": input.PRID})
	if s.logger != nil {
		s.logger.Info("created po from pr", slog.String("number", po.Number), slog.Int64("id", po.ID), slog.Int64("pr_id", input.PRID))
	}
	return po, nil
}

// SubmitPurchaseOrder requests approval.
func (s *Service) SubmitPurchaseOrder(ctx context.Context, poID int64, actorID int64) error {
	po, lines, err := s.repo.GetPO(ctx, poID)
	if err != nil {
		return err
	}
	if err := enforceRequestCompany(ctx, po.CompanyID); err != nil {
		return err
	}
	if po.Status != POStatusDraft {
		return ErrInvalidState
	}
	refID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("PO:%d", poID)))
	if s.approvalEngine != nil {
		amount := 0.0
		for _, line := range lines {
			amount += line.Qty * line.Price
		}
		var companyID *int64
		if po.CompanyID > 0 {
			companyID = &po.CompanyID
		}
		if _, err := s.approvalEngine.Submit(ctx, approvalengine.Submission{Module: "PO", DocumentID: poID, RequesterID: actorID, CompanyID: companyID, Amount: amount}); err != nil {
			return err
		}
	}
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
	if err := enforceRequestCompany(ctx, po.CompanyID); err != nil {
		return err
	}
	if po.Status != POStatusApproval {
		return ErrInvalidState
	}
	now := time.Now()
	refID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("PO:%d", poID)))
	if s.approvalEngine != nil {
		_, err := s.approvalEngine.DecideDocument(ctx, "PO", poID, actorID, approvalengine.DecisionApprove, "")
		return err
	}
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

func (s *Service) RejectPurchaseOrder(ctx context.Context, poID, actorID int64, note string) error {
	po, _, err := s.repo.GetPO(ctx, poID)
	if err != nil {
		return err
	}
	if err := enforceRequestCompany(ctx, po.CompanyID); err != nil {
		return err
	}
	if po.Status != POStatusApproval {
		return ErrInvalidState
	}
	if s.approvalEngine == nil {
		return errors.New("approval engine not configured")
	}
	_, err = s.approvalEngine.DecideDocument(ctx, "PO", poID, actorID, approvalengine.DecisionReject, note)
	return err
}

func (s *Service) FinalizeApproval(ctx context.Context, request approvalengine.Request, status string, actorID int64, note string) error {
	po, _, err := s.repo.GetPO(ctx, request.DocumentID)
	if err != nil {
		return err
	}
	if err := enforceRequestCompany(ctx, po.CompanyID); err != nil {
		return err
	}
	refID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("PO:%d", po.ID)))
	return s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		if status == approvalengine.StatusApproved {
			if err := tx.UpdatePOStatus(ctx, po.ID, POStatusApproved); err != nil {
				return err
			}
			if err := tx.SetPOApproval(ctx, po.ID, actorID, time.Now()); err != nil {
				return err
			}
			if s.approvals != nil {
				_ = s.approvals.Record(ctx, shared.ApprovalLog{Module: "PO", RefID: refID, ActorID: actorID, Action: shared.ApprovalApprove, Note: note})
			}
		} else {
			if err := tx.UpdatePOStatus(ctx, po.ID, POStatusCancelled); err != nil {
				return err
			}
			if s.approvals != nil {
				_ = s.approvals.Record(ctx, shared.ApprovalLog{Module: "PO", RefID: refID, ActorID: actorID, Action: shared.ApprovalReject, Note: note})
			}
		}
		return nil
	})
}

// CreateGoodsReceipt inserts GRN and lines.
func (s *Service) CreateGoodsReceipt(ctx context.Context, input CreateGRNInput) (GoodsReceipt, error) {
	if input.Number == "" {
		input.Number = generateNumber("GRN")
	}
	po, poLines, err := s.repo.GetPO(ctx, input.POID)
	if err != nil {
		return GoodsReceipt{}, err
	}
	if err := enforceRequestCompany(ctx, po.CompanyID); err != nil {
		return GoodsReceipt{}, err
	}
	if po.Status != POStatusApproved {
		return GoodsReceipt{}, ErrInvalidState
	}
	if input.SupplierID == 0 {
		input.SupplierID = po.SupplierID
	}
	if err := validateGoodsReceiptInput(input, po, poLines); err != nil {
		return GoodsReceipt{}, err
	}
	// The receipt inherits its tenant from the approved PO. Keeping the
	// company identity on the GRN makes later workbench reads and downstream
	// matching independent of nullable legacy rows and prevents a caller from
	// choosing a different tenant on the receipt form.
	grn := GoodsReceipt{CompanyID: po.CompanyID, Number: input.Number, POID: input.POID, SupplierID: input.SupplierID, WarehouseID: input.WarehouseID, Status: GRNStatusDraft, ReceivedAt: defaultTime(input.ReceivedAt), Note: input.Note}
	withTx := s.repo.WithTx
	if repo, ok := s.repo.(GRNCreateRepositoryPort); ok {
		withTx = repo.WithGRNCreateTx
	}
	err = withTx(ctx, func(ctx context.Context, tx TxRepository) error {
		if err := tx.ValidateGRNQuantities(ctx, input.POID, input.SupplierID, input.Lines); err != nil {
			return err
		}
		grnID, err := tx.CreateGRN(ctx, grn)
		if err != nil {
			return err
		}
		grn.ID = grnID
		for _, line := range input.Lines {
			if line.ProductID == 0 || line.Qty <= 0 {
				return ErrValidation
			}
			if err := tx.InsertGRNLine(ctx, GRNLine{GRNID: grnID, ProductID: line.ProductID, Qty: line.Qty, UnitCost: line.UnitCost, LotNumber: line.LotNumber, ExpiryDate: line.ExpiryDate, SerialNumbers: line.SerialNumbers}); err != nil {
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

// validateGoodsReceiptInput keeps the receipt facts bounded by the approved
// purchase order before they are persisted. GRN lines do not carry a PO-line
// identifier, so quantities are aggregated by product; this also supports a PO
// that contains multiple lines for the same product. NUMERIC(14,4) is the
// storage contract for both PO and GRN quantities, therefore all comparisons
// are made in that exact decimal domain rather than float64.
func validateGoodsReceiptInput(input CreateGRNInput, po PurchaseOrder, poLines []POLine) error {
	if input.WarehouseID <= 0 {
		return fmt.Errorf("%w: warehouse must be set", ErrValidation)
	}
	if po.SupplierID <= 0 {
		return fmt.Errorf("%w: PO supplier must be set", ErrValidation)
	}
	if input.SupplierID != po.SupplierID {
		return fmt.Errorf("%w: GRN supplier does not match PO supplier", ErrValidation)
	}
	if len(input.Lines) == 0 {
		return fmt.Errorf("%w: at least one GRN line is required", ErrValidation)
	}
	if len(poLines) == 0 {
		return fmt.Errorf("%w: PO must contain at least one line", ErrValidation)
	}

	zero := fx.MustDecimal("0")
	ordered := make(map[int64]fx.Decimal, len(poLines))
	for _, poLine := range poLines {
		if poLine.ProductID <= 0 {
			return fmt.Errorf("%w: PO line has invalid product", ErrValidation)
		}
		qty, err := fx.FromLegacyFloat(poLine.Qty, 4)
		if err != nil || qty.Cmp(zero) <= 0 {
			return fmt.Errorf("%w: PO line for product %d has invalid quantity", ErrValidation, poLine.ProductID)
		}
		ordered[poLine.ProductID] = ordered[poLine.ProductID].Add(qty)
	}

	received := make(map[int64]fx.Decimal, len(input.Lines))
	for _, line := range input.Lines {
		if line.ProductID <= 0 {
			return fmt.Errorf("%w: GRN line has invalid product", ErrValidation)
		}
		qty, err := fx.FromLegacyFloat(line.Qty, 4)
		if err != nil || qty.Cmp(zero) <= 0 {
			return fmt.Errorf("%w: quantity for product %d must be positive", ErrValidation, line.ProductID)
		}
		unitCost, err := fx.FromLegacyFloat(line.UnitCost, 4)
		if err != nil || unitCost.Cmp(zero) < 0 {
			return fmt.Errorf("%w: unit cost for product %d must be non-negative", ErrValidation, line.ProductID)
		}
		orderedQty, ok := ordered[line.ProductID]
		if !ok {
			return fmt.Errorf("%w: product %d is not on PO", ErrValidation, line.ProductID)
		}
		received[line.ProductID] = received[line.ProductID].Add(qty)
		if received[line.ProductID].Cmp(orderedQty) > 0 {
			return fmt.Errorf("%w: quantity for product %d exceeds PO quantity", ErrValidation, line.ProductID)
		}
	}
	return nil
}

// PostGoodsReceipt posts GRN and updates inventory.
func (s *Service) PostGoodsReceipt(ctx context.Context, grnID int64) error {
	grn, lines, err := s.repo.GetGRN(ctx, grnID)
	if err != nil {
		return err
	}
	if err := enforceRequestCompany(ctx, grn.CompanyID); err != nil {
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
				Code:          fmt.Sprintf("GRN-%s-%d", grn.Number, line.ProductID),
				WarehouseID:   grn.WarehouseID,
				ProductID:     line.ProductID,
				Qty:           line.Qty,
				UnitCost:      line.UnitCost,
				Note:          fmt.Sprintf("GRN %s", grn.Number),
				ActorID:       0,
				RefModule:     "PROCUREMENT",
				RefID:         refID.String(),
				LotNumber:     line.LotNumber,
				ExpiryDate:    line.ExpiryDate,
				SerialNumbers: line.SerialNumbers,
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
	if s.integration != nil {
		evt := GRNPostedEvent{
			ID:          grn.ID,
			Number:      grn.Number,
			SupplierID:  grn.SupplierID,
			WarehouseID: grn.WarehouseID,
			ReceivedAt:  grn.ReceivedAt,
		}
		evt.Lines = make([]GRNLineEvent, 0, len(lines))
		for _, line := range lines {
			evt.Lines = append(evt.Lines, GRNLineEvent{ProductID: line.ProductID, Qty: line.Qty, UnitCost: line.UnitCost})
		}
		if err := s.integration.HandleGRNPosted(ctx, evt); err != nil {
			return err
		}
	}
	return nil
}

// GetGRNWithLines exposes GRN details for other modules (e.g. AP)
func (s *Service) GetGRNWithLines(ctx context.Context, id int64) (GoodsReceipt, []GRNLine, error) {
	grn, lines, err := s.repo.GetGRN(ctx, id)
	if err != nil {
		return GoodsReceipt{}, nil, err
	}
	if err := enforceRequestCompany(ctx, grn.CompanyID); err != nil {
		return GoodsReceipt{}, nil, err
	}
	return grn, lines, nil
}

// GetPOWithLines exposes PO details for other modules (e.g. AP)
func (s *Service) GetPOWithLines(ctx context.Context, id int64) (PurchaseOrder, []POLine, error) {
	po, lines, err := s.repo.GetPO(ctx, id)
	if err != nil {
		return PurchaseOrder{}, nil, err
	}
	if err := enforceRequestCompany(ctx, po.CompanyID); err != nil {
		return PurchaseOrder{}, nil, err
	}
	return po, lines, nil
}

// ListPOs returns paginated purchase orders.
func (s *Service) ListPOs(ctx context.Context, limit, offset int, filters ListFilters) ([]POListItem, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if err := enforceRequestCompany(ctx, filters.CompanyID); err != nil {
		return nil, 0, err
	}
	return s.repo.ListPOs(ctx, limit, offset, filters)
}

// ListGRNs returns paginated goods receipts.
func (s *Service) ListGRNs(ctx context.Context, limit, offset int, filters ListFilters) ([]GRNListItem, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if err := enforceRequestCompany(ctx, filters.CompanyID); err != nil {
		return nil, 0, err
	}
	return s.repo.ListGRNs(ctx, limit, offset, filters)
}

// CreateGoodsReturnGRN creates a goods return from a posted GRN.
func (s *Service) CreateGoodsReturnGRN(ctx context.Context, input CreateGoodsReturnGRNInput) (GoodsReturnGRN, error) {
	if len(input.Lines) == 0 {
		return GoodsReturnGRN{}, errors.New("at least one return line is required")
	}
	grn, grnLines, err := s.repo.GetGRN(ctx, input.GRNID)
	if err != nil {
		return GoodsReturnGRN{}, err
	}
	input, err = normalizeGoodsReturnInput(ctx, input, grn)
	if err != nil {
		return GoodsReturnGRN{}, err
	}
	if grn.Status != GRNStatusPosted {
		return GoodsReturnGRN{}, errors.New("GRN must be posted before creating a return")
	}
	if input.ReturnDate.IsZero() {
		input.ReturnDate = time.Now()
	}
	grnLineMap := make(map[int64]GRNLine, len(grnLines))
	for _, line := range grnLines {
		grnLineMap[line.ID] = line
	}
	// GRN quantities are NUMERIC(14,4). Keep the cumulative guard in the same
	// exact decimal domain so values such as 0.1 + 0.2 do not exceed 0.3 due to
	// binary floating-point representation before they reach PostgreSQL.
	requestedQty := make(map[int64]fx.Decimal)
	var id int64
	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		num, err := tx.GenerateGoodsReturnGRNNumber(ctx)
		if err != nil {
			return err
		}
		id, err = tx.CreateGoodsReturnGRN(ctx, GoodsReturnGRN{
			Number:      num,
			CompanyID:   input.CompanyID,
			SupplierID:  input.SupplierID,
			GRNID:       input.GRNID,
			WarehouseID: input.WarehouseID,
			ReturnDate:  input.ReturnDate,
			Status:      GoodsReturnStatusDraft,
			Reason:      input.Reason,
			Notes:       input.Notes,
			CreatedBy:   input.CreatedBy,
		})
		if err != nil {
			return err
		}
		for _, line := range input.Lines {
			originalLine, ok := grnLineMap[line.GRNLineID]
			if !ok {
				return fmt.Errorf("GRN line %d not found", line.GRNLineID)
			}
			if line.ProductID != originalLine.ProductID {
				return fmt.Errorf("product mismatch on GRN line %d", line.GRNLineID)
			}
			returnedQty, err := fx.FromLegacyFloat(line.QuantityReturned, 4)
			if err != nil || returnedQty.Cmp(fx.MustDecimal("0")) <= 0 {
				return fmt.Errorf("quantity returned must be positive")
			}
			originalQty, err := fx.FromLegacyFloat(originalLine.Qty, 4)
			if err != nil {
				return fmt.Errorf("invalid GRN line quantity")
			}
			requestedQty[line.GRNLineID] = requestedQty[line.GRNLineID].Add(returnedQty)
			if requestedQty[line.GRNLineID].Cmp(originalQty) > 0 {
				return fmt.Errorf("quantity returned exceeds GRN line quantity")
			}
			if err := tx.InsertGoodsReturnGRNLine(ctx, GoodsReturnGRNLine{
				GoodsReturnGRNID: id,
				GRNLineID:        line.GRNLineID,
				ProductID:        line.ProductID,
				QuantityReturned: line.QuantityReturned,
				UnitCost:         line.UnitCost,
				Notes:            line.Notes,
				LineOrder:        line.LineOrder,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return GoodsReturnGRN{}, err
	}
	return s.getGoodsReturnGRN(ctx, id)
}

// normalizeGoodsReturnInput binds a return to the already persisted GRN
// facts. A caller may omit supplier/warehouse/company values for convenience,
// but may not choose replacements for an existing receipt. This prevents a
// valid GRN from being used to create a return in another tenant or warehouse.
// Legacy worker callers can still process GRNs whose old rows have no company
// value; browser requests fail closed because their tenant cannot be proven.
func normalizeGoodsReturnInput(ctx context.Context, input CreateGoodsReturnGRNInput, grn GoodsReceipt) (CreateGoodsReturnGRNInput, error) {
	if grn.CompanyID > 0 {
		if input.CompanyID > 0 && input.CompanyID != grn.CompanyID {
			return CreateGoodsReturnGRNInput{}, ErrCompanyScopeMismatch
		}
		input.CompanyID = grn.CompanyID
	} else if shared.SessionFromContext(ctx) != nil {
		return CreateGoodsReturnGRNInput{}, ErrCompanyScopeRequired
	} else if input.CompanyID <= 0 {
		return CreateGoodsReturnGRNInput{}, fmt.Errorf("%w: GRN company is required", ErrValidation)
	}
	if err := enforceRequestCompany(ctx, input.CompanyID); err != nil {
		return CreateGoodsReturnGRNInput{}, err
	}
	if grn.SupplierID <= 0 {
		return CreateGoodsReturnGRNInput{}, fmt.Errorf("%w: GRN supplier is required", ErrValidation)
	}
	if input.SupplierID == 0 {
		input.SupplierID = grn.SupplierID
	}
	if input.SupplierID != grn.SupplierID {
		return CreateGoodsReturnGRNInput{}, fmt.Errorf("%w: return supplier does not match GRN supplier", ErrValidation)
	}
	if grn.WarehouseID > 0 {
		if input.WarehouseID == 0 {
			input.WarehouseID = grn.WarehouseID
		}
		if input.WarehouseID != grn.WarehouseID {
			return CreateGoodsReturnGRNInput{}, fmt.Errorf("%w: return warehouse does not match GRN warehouse", ErrValidation)
		}
	} else if input.WarehouseID <= 0 {
		return CreateGoodsReturnGRNInput{}, fmt.Errorf("%w: return warehouse is required", ErrValidation)
	}
	return input, nil
}

// ConfirmGoodsReturnGRN confirms a goods return, posts negative inventory adjustment,
// and fires the integration event.
func (s *Service) ConfirmGoodsReturnGRN(ctx context.Context, id int64, actorID int64) (GoodsReturnGRN, error) {
	ret, lines, err := s.getGoodsReturnGRNWithLines(ctx, id)
	if err != nil {
		return GoodsReturnGRN{}, err
	}
	if ret.Status != GoodsReturnStatusDraft {
		return GoodsReturnGRN{}, ErrInvalidState
	}
	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		if err := tx.ConfirmGoodsReturnGRN(ctx, id, actorID); err != nil {
			return err
		}
		for _, line := range lines {
			if s.inventory == nil {
				return errors.New("inventory integration not configured")
			}
			refID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("GRN-RET:%d:%d", ret.ID, line.ProductID)))
			_, err := s.inventory.PostAdjustment(ctx, inventory.AdjustmentInput{
				Code:        fmt.Sprintf("GRN-RET-%s-%d", ret.Number, line.ProductID),
				WarehouseID: ret.WarehouseID,
				ProductID:   line.ProductID,
				Qty:         -line.QuantityReturned,
				UnitCost:    line.UnitCost,
				Note:        fmt.Sprintf("Goods return %s", ret.Number),
				ActorID:     actorID,
				RefModule:   "PURCHASE_RETURN",
				RefID:       refID.String(),
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return GoodsReturnGRN{}, err
	}
	s.recordAudit(ctx, "GOODS_RETURN_CONFIRM", id, map[string]any{"number": ret.Number})
	if s.integration != nil {
		evt := GoodsReturnConfirmedEvent{
			ID:          ret.ID,
			Number:      ret.Number,
			SupplierID:  ret.SupplierID,
			GRNID:       ret.GRNID,
			WarehouseID: ret.WarehouseID,
			ReturnDate:  ret.ReturnDate,
		}
		for _, line := range lines {
			evt.Lines = append(evt.Lines, GoodsReturnLineEvent{
				GoodsReturnGRNLineID: line.ID,
				GRNLineID:            line.GRNLineID,
				ProductID:            line.ProductID,
				QuantityReturned:     line.QuantityReturned,
				UnitCost:             line.UnitCost,
			})
		}
		if err := s.integration.HandleGoodsReturnConfirmed(ctx, evt); err != nil {
			return GoodsReturnGRN{}, err
		}
	}
	return s.getGoodsReturnGRN(ctx, id)
}

// CancelGoodsReturnGRN cancels a draft goods return.
func (s *Service) CancelGoodsReturnGRN(ctx context.Context, id int64, actorID int64) (GoodsReturnGRN, error) {
	ret, _, err := s.getGoodsReturnGRNWithLines(ctx, id)
	if err != nil {
		return GoodsReturnGRN{}, err
	}
	if ret.Status != GoodsReturnStatusDraft {
		return GoodsReturnGRN{}, ErrInvalidState
	}
	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		return tx.CancelGoodsReturnGRN(ctx, id, actorID)
	})
	if err != nil {
		return GoodsReturnGRN{}, err
	}
	s.recordAudit(ctx, "GOODS_RETURN_CANCEL", id, map[string]any{"number": ret.Number})
	return s.getGoodsReturnGRN(ctx, id)
}

// GetGoodsReturnGRN returns a goods return with lines.
func (s *Service) GetGoodsReturnGRN(ctx context.Context, id int64) (GoodsReturnGRN, error) {
	return s.getGoodsReturnGRN(ctx, id)
}

// ListGoodsReturnGRNs returns all goods returns.
func (s *Service) ListGoodsReturnGRNs(ctx context.Context) ([]GoodsReturnGRN, error) {
	if repo, ok := s.repo.(GoodsReturnRepositoryPort); ok {
		items, err := repo.ListGoodsReturnGRNs(ctx)
		if err != nil {
			return nil, err
		}
		if shared.SessionFromContext(ctx) == nil {
			return items, nil
		}
		identity, ok := shared.IdentityFromContext(ctx)
		if !ok || identity.CompanyID <= 0 {
			return nil, ErrCompanyScopeRequired
		}
		scoped := make([]GoodsReturnGRN, 0, len(items))
		for _, item := range items {
			if item.CompanyID == identity.CompanyID {
				scoped = append(scoped, item)
			}
		}
		return scoped, nil
	}
	return nil, errors.New("goods return repository not available")
}

func (s *Service) getGoodsReturnGRN(ctx context.Context, id int64) (GoodsReturnGRN, error) {
	if repo, ok := s.repo.(GoodsReturnRepositoryPort); ok {
		ret, lines, err := repo.GetGoodsReturnGRN(ctx, id)
		if err != nil {
			return GoodsReturnGRN{}, err
		}
		if err := enforceRequestCompany(ctx, ret.CompanyID); err != nil {
			return GoodsReturnGRN{}, err
		}
		ret.Lines = lines
		return ret, nil
	}
	return GoodsReturnGRN{}, errors.New("goods return repository not available")
}

func (s *Service) getGoodsReturnGRNWithLines(ctx context.Context, id int64) (GoodsReturnGRN, []GoodsReturnGRNLine, error) {
	if repo, ok := s.repo.(GoodsReturnRepositoryPort); ok {
		ret, lines, err := repo.GetGoodsReturnGRN(ctx, id)
		if err != nil {
			return GoodsReturnGRN{}, nil, err
		}
		if err := enforceRequestCompany(ctx, ret.CompanyID); err != nil {
			return GoodsReturnGRN{}, nil, err
		}
		return ret, lines, nil
	}
	return GoodsReturnGRN{}, nil, errors.New("goods return repository not available")
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
