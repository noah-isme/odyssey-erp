package procurement

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/inventory"
)

type memoryProcRepo struct {
	prs      map[int64]PurchaseRequest
	prLines  map[int64][]PRLine
	pos      map[int64]PurchaseOrder
	poLines  map[int64][]POLine
	grns     map[int64]GoodsReceipt
	grnLines map[int64][]GRNLine
	invoices map[int64]APInvoice
	payments map[int64][]APPayment
	nextID   int64
}

type memoryProcTx struct {
	repo *memoryProcRepo
}

func newMemoryProcRepo() *memoryProcRepo {
	return &memoryProcRepo{
		prs:      make(map[int64]PurchaseRequest),
		prLines:  make(map[int64][]PRLine),
		pos:      make(map[int64]PurchaseOrder),
		poLines:  make(map[int64][]POLine),
		grns:     make(map[int64]GoodsReceipt),
		grnLines: make(map[int64][]GRNLine),
		invoices: make(map[int64]APInvoice),
		payments: make(map[int64][]APPayment),
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
	records []inventory.InboundInput
}

func (s *stubInventory) PostInbound(ctx context.Context, input inventory.InboundInput) (inventory.StockCardEntry, error) {
	s.records = append(s.records, input)
	return inventory.StockCardEntry{TxCode: input.Code, QtyIn: input.Qty}, nil
}

func TestProcurementFlow(t *testing.T) {
	repo := newMemoryProcRepo()
	inv := &stubInventory{}
	svc := NewService(repo, inv, nil, nil, nil, nil)
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

	invoice, err := svc.CreateAPInvoiceFromGRN(ctx, APInvoiceInput{GRNID: grn.ID, DueDate: time.Now().AddDate(0, 0, 14)})
	require.NoError(t, err)
	require.NotZero(t, invoice.ID)

	require.NoError(t, svc.PostAPInvoice(ctx, invoice.ID))

	require.NoError(t, svc.RegisterPayment(ctx, PaymentInput{APInvoiceID: invoice.ID, Amount: invoice.Total}))

	aging, err := svc.CalculateAPAging(ctx, time.Now())
	require.NoError(t, err)
	require.InDelta(t, 0, aging.Current+aging.Bucket30+aging.Bucket60+aging.Bucket90+aging.Bucket120, 0.001)
}
