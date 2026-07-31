package procurement

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/inventory"
	_ "github.com/odyssey-erp/odyssey-erp/testing"
)

type memoryProcRepo struct {
	prs         map[int64]PurchaseRequest
	prLines     map[int64][]PRLine
	pos         map[int64]PurchaseOrder
	poLines     map[int64][]POLine
	grns        map[int64]GoodsReceipt
	grnLines    map[int64][]GRNLine
	returns     map[int64]GoodsReturnGRN
	returnLines map[int64][]GoodsReturnGRNLine
	invoices    map[int64]APInvoice
	payments    map[int64][]APPayment
	nextID      int64
}

type memoryProcTx struct {
	repo *memoryProcRepo
}

func newMemoryProcRepo() *memoryProcRepo {
	return &memoryProcRepo{
		prs:         make(map[int64]PurchaseRequest),
		prLines:     make(map[int64][]PRLine),
		pos:         make(map[int64]PurchaseOrder),
		poLines:     make(map[int64][]POLine),
		grns:        make(map[int64]GoodsReceipt),
		grnLines:    make(map[int64][]GRNLine),
		returns:     make(map[int64]GoodsReturnGRN),
		returnLines: make(map[int64][]GoodsReturnGRNLine),
		invoices:    make(map[int64]APInvoice),
		payments:    make(map[int64][]APPayment),
	}
}

func (r *memoryProcRepo) WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error {
	tx := &memoryProcTx{repo: r}
	return fn(ctx, tx)
}

func (r *memoryProcRepo) GetPR(ctx context.Context, id int64) (PurchaseRequest, []PRLine, error) {
	pr, ok := r.prs[id]
	if !ok {
		return PurchaseRequest{}, nil, ErrNotFound
	}
	return pr, append([]PRLine(nil), r.prLines[id]...), nil
}

func (r *memoryProcRepo) GetPO(ctx context.Context, id int64) (PurchaseOrder, []POLine, error) {
	po, ok := r.pos[id]
	if !ok {
		return PurchaseOrder{}, nil, ErrNotFound
	}
	return po, append([]POLine(nil), r.poLines[id]...), nil
}

func (r *memoryProcRepo) GetGRN(ctx context.Context, id int64) (GoodsReceipt, []GRNLine, error) {
	grn, ok := r.grns[id]
	if !ok {
		return GoodsReceipt{}, nil, ErrNotFound
	}
	return grn, append([]GRNLine(nil), r.grnLines[id]...), nil
}

func (r *memoryProcRepo) ListPOs(ctx context.Context, limit, offset int, filters ListFilters) ([]POListItem, int, error) {
	return nil, 0, nil
}

func (r *memoryProcRepo) ListGRNs(ctx context.Context, limit, offset int, filters ListFilters) ([]GRNListItem, int, error) {
	return nil, 0, nil
}

func (r *memoryProcRepo) GetAPInvoice(ctx context.Context, id int64) (APInvoice, error) {
	inv, ok := r.invoices[id]
	if !ok {
		return APInvoice{}, ErrNotFound
	}
	return inv, nil
}

func (r *memoryProcRepo) ListAPOutstanding(ctx context.Context) ([]APInvoice, error) {
	invoices := make([]APInvoice, 0, len(r.invoices))
	for _, inv := range r.invoices {
		invoices = append(invoices, inv)
	}
	return invoices, nil
}

func (r *memoryProcRepo) POExistsByNumber(ctx context.Context, number string) (bool, error) {
	for _, po := range r.pos {
		if po.Number == number {
			return true, nil
		}
	}
	return false, nil
}

func (tx *memoryProcTx) nextID() int64 {
	tx.repo.nextID++
	return tx.repo.nextID
}

func (tx *memoryProcTx) CreatePR(ctx context.Context, pr PurchaseRequest) (int64, error) {
	id := tx.nextID()
	pr.ID = id
	tx.repo.prs[id] = pr
	return id, nil
}

func (tx *memoryProcTx) InsertPRLine(ctx context.Context, line PRLine) error {
	line.ID = tx.nextID()
	tx.repo.prLines[line.PRID] = append(tx.repo.prLines[line.PRID], line)
	return nil
}

func (tx *memoryProcTx) UpdatePRStatus(ctx context.Context, id int64, status PRStatus) error {
	pr := tx.repo.prs[id]
	pr.Status = status
	tx.repo.prs[id] = pr
	return nil
}

func (tx *memoryProcTx) CreatePO(ctx context.Context, po PurchaseOrder) (int64, error) {
	id := tx.nextID()
	po.ID = id
	tx.repo.pos[id] = po
	return id, nil
}

func (tx *memoryProcTx) InsertPOLine(ctx context.Context, line POLine) error {
	line.ID = tx.nextID()
	tx.repo.poLines[line.POID] = append(tx.repo.poLines[line.POID], line)
	return nil
}

func (tx *memoryProcTx) UpdatePOStatus(ctx context.Context, id int64, status POStatus) error {
	po := tx.repo.pos[id]
	po.Status = status
	tx.repo.pos[id] = po
	return nil
}

func (tx *memoryProcTx) SetPOApproval(ctx context.Context, id int64, approvedBy int64, approvedAt time.Time) error {
	po := tx.repo.pos[id]
	po.Status = POStatusApproved
	tx.repo.pos[id] = po
	return nil
}

func (tx *memoryProcTx) CreateGRN(ctx context.Context, grn GoodsReceipt) (int64, error) {
	id := tx.nextID()
	grn.ID = id
	tx.repo.grns[id] = grn
	return id, nil
}

func (tx *memoryProcTx) InsertGRNLine(ctx context.Context, line GRNLine) error {
	line.ID = tx.nextID()
	tx.repo.grnLines[line.GRNID] = append(tx.repo.grnLines[line.GRNID], line)
	return nil
}

func (tx *memoryProcTx) UpdateGRNStatus(ctx context.Context, id int64, status GRNStatus) error {
	grn := tx.repo.grns[id]
	grn.Status = status
	tx.repo.grns[id] = grn
	return nil
}

func (tx *memoryProcTx) CreateGoodsReturnGRN(ctx context.Context, ret GoodsReturnGRN) (int64, error) {
	id := tx.nextID()
	ret.ID = id
	tx.repo.returns[id] = ret
	return id, nil
}

func (tx *memoryProcTx) InsertGoodsReturnGRNLine(ctx context.Context, line GoodsReturnGRNLine) error {
	line.ID = tx.nextID()
	tx.repo.returnLines[line.GoodsReturnGRNID] = append(tx.repo.returnLines[line.GoodsReturnGRNID], line)
	return nil
}

func (tx *memoryProcTx) ConfirmGoodsReturnGRN(ctx context.Context, id int64, actorID int64) error {
	ret := tx.repo.returns[id]
	ret.Status = GoodsReturnStatusConfirmed
	tx.repo.returns[id] = ret
	return nil
}

func (tx *memoryProcTx) CancelGoodsReturnGRN(ctx context.Context, id int64, actorID int64) error {
	ret := tx.repo.returns[id]
	ret.Status = GoodsReturnStatusCancelled
	tx.repo.returns[id] = ret
	return nil
}

func (tx *memoryProcTx) GenerateGoodsReturnGRNNumber(ctx context.Context) (string, error) {
	return "GRN-RET-TEST", nil
}

func (r *memoryProcRepo) GetGoodsReturnGRN(ctx context.Context, id int64) (GoodsReturnGRN, []GoodsReturnGRNLine, error) {
	ret, ok := r.returns[id]
	if !ok {
		return GoodsReturnGRN{}, nil, ErrGoodsReturnNotFound
	}
	return ret, append([]GoodsReturnGRNLine(nil), r.returnLines[id]...), nil
}

func (r *memoryProcRepo) ListGoodsReturnGRNs(ctx context.Context) ([]GoodsReturnGRN, error) {
	returns := make([]GoodsReturnGRN, 0, len(r.returns))
	for _, ret := range r.returns {
		returns = append(returns, ret)
	}
	return returns, nil
}

func (tx *memoryProcTx) CreateAPInvoice(ctx context.Context, inv APInvoice) (int64, error) {
	id := tx.nextID()
	inv.ID = id
	tx.repo.invoices[id] = inv
	return id, nil
}

func (tx *memoryProcTx) UpdateAPStatus(ctx context.Context, id int64, status APInvoiceStatus) error {
	inv := tx.repo.invoices[id]
	inv.Status = status
	tx.repo.invoices[id] = inv
	return nil
}

func (tx *memoryProcTx) CreatePayment(ctx context.Context, payment APPayment) (int64, error) {
	id := tx.nextID()
	payment.ID = id
	tx.repo.payments[payment.APInvoiceID] = append(tx.repo.payments[payment.APInvoiceID], payment)
	return id, nil
}

type stubInventory struct {
	records     []inventory.InboundInput
	adjustments []inventory.AdjustmentInput
}

func (s *stubInventory) PostInbound(ctx context.Context, input inventory.InboundInput) (inventory.StockCardEntry, error) {
	s.records = append(s.records, input)
	return inventory.StockCardEntry{TxCode: input.Code, QtyIn: input.Qty}, nil
}

func (s *stubInventory) PostAdjustment(ctx context.Context, input inventory.AdjustmentInput) (inventory.StockCardEntry, error) {
	s.adjustments = append(s.adjustments, input)
	return inventory.StockCardEntry{TxCode: input.Code, QtyOut: -input.Qty}, nil
}

func TestProcurementFlow(t *testing.T) {
	repo := newMemoryProcRepo()
	inv := &stubInventory{}
	svc := NewService(nil, repo, inv, nil, nil, nil, nil)
	ctx := context.Background()

	pr, err := svc.CreatePurchaseRequest(ctx, CreatePRInput{
		SupplierID: 1,
		RequestBy:  99,
		Lines:      []PRLineInput{{ProductID: 11, Qty: 5}},
	})
	require.NoError(t, err)
	require.NotZero(t, pr.ID)

	require.NoError(t, svc.SubmitPurchaseRequest(ctx, pr.ID, 100))

	po, err := svc.CreatePOFromPR(ctx, CreatePOInput{PRID: pr.ID, Currency: "IDR", ExpectedDate: time.Now()})
	require.NoError(t, err)
	require.NotZero(t, po.ID)

	require.NoError(t, svc.SubmitPurchaseOrder(ctx, po.ID, 100))
	require.NoError(t, svc.ApprovePurchaseOrder(ctx, po.ID, 200))

	grn, err := svc.CreateGoodsReceipt(ctx, CreateGRNInput{
		POID:        po.ID,
		WarehouseID: 2,
		SupplierID:  1,
		Lines:       []GRNLineInput{{ProductID: 11, Qty: 5, UnitCost: 10000}},
	})
	require.NoError(t, err)
	require.NotZero(t, grn.ID)

	require.NoError(t, svc.PostGoodsReceipt(ctx, grn.ID))
	require.Len(t, inv.records, 1)
	require.Equal(t, 5.0, inv.records[0].Qty)
}

func TestGRNPostPropagatesTraceabilityToInventory(t *testing.T) {
	repo := newMemoryProcRepo()
	inv := &stubInventory{}
	svc := NewService(nil, repo, inv, nil, nil, nil, nil)
	ctx := context.Background()

	poID := int64(1)
	repo.pos[poID] = PurchaseOrder{ID: poID, Number: "PO-TRACE", SupplierID: 1, Status: POStatusApproved}
	expiry := time.Date(2027, time.January, 31, 0, 0, 0, 0, time.UTC)
	grn, err := svc.CreateGoodsReceipt(ctx, CreateGRNInput{POID: poID, WarehouseID: 2, SupplierID: 1, Lines: []GRNLineInput{{ProductID: 11, Qty: 2, UnitCost: 10000, LotNumber: "LOT-2026-001", ExpiryDate: &expiry, SerialNumbers: []string{"SN-001", "SN-002"}}}})
	require.NoError(t, err)
	require.NoError(t, svc.PostGoodsReceipt(ctx, grn.ID))
	require.Len(t, inv.records, 1)
	require.Equal(t, "LOT-2026-001", inv.records[0].LotNumber)
	require.Equal(t, expiry, *inv.records[0].ExpiryDate)
	require.Equal(t, []string{"SN-001", "SN-002"}, inv.records[0].SerialNumbers)
}

func TestCreateGoodsReturnPreventsDuplicateQuantities(t *testing.T) {
	repo := newMemoryProcRepo()
	repo.grns[1] = GoodsReceipt{ID: 1, SupplierID: 7, POID: 22, Status: GRNStatusPosted}
	repo.grnLines[1] = []GRNLine{{ID: 9, ProductID: 100, Qty: 5, UnitCost: 12}}
	svc := NewService(nil, repo, nil, nil, nil, nil, nil)

	_, err := svc.CreateGoodsReturnGRN(context.Background(), CreateGoodsReturnGRNInput{
		GRNID:       1,
		CompanyID:   3,
		SupplierID:  7,
		WarehouseID: 4,
		Reason:      "damaged",
		Lines: []GoodsReturnGRNLineInput{
			{GRNLineID: 9, ProductID: 100, QuantityReturned: 3, UnitCost: 12},
			{GRNLineID: 9, ProductID: 100, QuantityReturned: 3, UnitCost: 12},
		},
	})
	require.ErrorContains(t, err, "quantity returned exceeds GRN line quantity")
}

func TestGoodsReturnLifecycleTransitions(t *testing.T) {
	repo := newMemoryProcRepo()
	repo.grns[1] = GoodsReceipt{ID: 1, SupplierID: 7, POID: 22, Status: GRNStatusPosted}
	repo.grnLines[1] = []GRNLine{{ID: 9, ProductID: 100, Qty: 5, UnitCost: 12}}
	svc := NewService(nil, repo, &stubInventory{}, nil, nil, nil, nil)

	ret, err := svc.CreateGoodsReturnGRN(context.Background(), CreateGoodsReturnGRNInput{
		GRNID:       1,
		CompanyID:   3,
		SupplierID:  7,
		WarehouseID: 4,
		Reason:      "damaged",
		Lines:       []GoodsReturnGRNLineInput{{GRNLineID: 9, ProductID: 100, QuantityReturned: 2, UnitCost: 12}},
	})
	require.NoError(t, err)
	require.Equal(t, GoodsReturnStatusDraft, ret.Status)

	confirmed, err := svc.ConfirmGoodsReturnGRN(context.Background(), ret.ID, 11)
	require.NoError(t, err)
	require.Equal(t, GoodsReturnStatusConfirmed, confirmed.Status)

	_, err = svc.ConfirmGoodsReturnGRN(context.Background(), ret.ID, 11)
	require.ErrorIs(t, err, ErrInvalidState)

	cancelRepo := newMemoryProcRepo()
	cancelRepo.returns[2] = GoodsReturnGRN{ID: 2, Number: "GRN-RET-2", Status: GoodsReturnStatusCancelled}
	cancelSvc := NewService(nil, cancelRepo, nil, nil, nil, nil, nil)
	_, err = cancelSvc.CancelGoodsReturnGRN(context.Background(), 2, 11)
	require.ErrorIs(t, err, ErrInvalidState)
}
