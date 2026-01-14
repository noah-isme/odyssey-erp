package ap

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/procurement"
)

type memoryAPRepo struct {
	invoices     map[int64]APInvoice
	lines        map[int64][]APInvoiceLine
	payments     map[int64]APPayment
	allocations  map[int64][]APPaymentAllocation
	nextID       int64
	nextLineID   int64
	nextPayID    int64
	nextAllocID  int64
	numberCursor int64
}

type memoryAPTx struct {
	repo *memoryAPRepo
}

func newMemoryAPRepo() *memoryAPRepo {
	return &memoryAPRepo{
		invoices:    make(map[int64]APInvoice),
		lines:       make(map[int64][]APInvoiceLine),
		payments:    make(map[int64]APPayment),
		allocations: make(map[int64][]APPaymentAllocation),
	}
}

func (r *memoryAPRepo) WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error {
	return fn(ctx, &memoryAPTx{repo: r})
}

func (r *memoryAPRepo) GetAPInvoice(ctx context.Context, id int64) (APInvoice, error) {
	inv, ok := r.invoices[id]
	if !ok {
		return APInvoice{}, ErrInvoiceNotFound
	}
	return inv, nil
}

func (r *memoryAPRepo) GetAPInvoiceWithDetails(ctx context.Context, id int64) (APInvoiceWithDetails, error) {
	inv, ok := r.invoices[id]
	if !ok {
		return APInvoiceWithDetails{}, ErrInvoiceNotFound
	}
	lines := append([]APInvoiceLine(nil), r.lines[id]...)
	allocs := r.allocations[id]
	var payments []APPaymentSummary
	var paid float64
	for _, alloc := range allocs {
		pay := r.payments[alloc.APPaymentID]
		payments = append(payments, APPaymentSummary{
			ID:              pay.ID,
			Number:          pay.Number,
			Amount:          pay.Amount,
			AllocatedAmount: alloc.Amount,
			PaidAt:          pay.PaidAt,
			Method:          pay.Method,
			Note:            pay.Note,
		})
		paid += alloc.Amount
	}
	return APInvoiceWithDetails{
		APInvoice:  inv,
		Lines:      lines,
		Payments:   payments,
		PaidAmount: paid,
		Balance:    inv.Total - paid,
	}, nil
}

func (r *memoryAPRepo) ListAPInvoices(ctx context.Context, req ListAPInvoicesRequest) ([]APInvoice, error) {
	var out []APInvoice
	for _, inv := range r.invoices {
		if req.Status != "" && inv.Status != req.Status {
			continue
		}
		if req.SupplierID != 0 && inv.SupplierID != req.SupplierID {
			continue
		}
		out = append(out, inv)
	}
	return out, nil
}

func (r *memoryAPRepo) CountInvoicesByGRN(ctx context.Context, grnID int64) (int, error) {
	count := 0
	for _, inv := range r.invoices {
		if inv.GRNID != nil && *inv.GRNID == grnID {
			count++
		}
	}
	return count, nil
}

func (r *memoryAPRepo) ListAPPayments(ctx context.Context) ([]APPayment, error) {
	var out []APPayment
	for _, pay := range r.payments {
		out = append(out, pay)
	}
	return out, nil
}

func (r *memoryAPRepo) GetAPPaymentWithDetails(ctx context.Context, id int64) (APPaymentWithDetails, error) {
	pay, ok := r.payments[id]
	if !ok {
		return APPaymentWithDetails{}, ErrPaymentNotFound
	}
	var allocations []APPaymentAllocationDetail
	var totalAllocated float64
	for invoiceID, allocs := range r.allocations {
		inv, ok := r.invoices[invoiceID]
		if !ok {
			continue
		}
		for _, alloc := range allocs {
			if alloc.APPaymentID != id {
				continue
			}
			allocations = append(allocations, APPaymentAllocationDetail{
				ID:            alloc.ID,
				APPaymentID:   alloc.APPaymentID,
				APInvoiceID:   alloc.APInvoiceID,
				InvoiceNumber: inv.Number,
				POID:          inv.POID,
				InvoiceStatus: inv.Status,
				InvoiceTotal:  inv.Total,
				DueAt:         inv.DueAt,
				Amount:        alloc.Amount,
			})
			totalAllocated += alloc.Amount
		}
	}
	unallocated := pay.Amount - totalAllocated
	if unallocated < 0 {
		unallocated = 0
	}
	return APPaymentWithDetails{
		APPayment:      pay,
		Allocations:    allocations,
		TotalAllocated: totalAllocated,
		Unallocated:    unallocated,
		LedgerPosted:   false,
	}, nil
}

func (r *memoryAPRepo) GetAPInvoiceBalancesBatch(ctx context.Context) ([]APInvoiceBalance, error) {
	var balances []APInvoiceBalance
	for id, inv := range r.invoices {
		if inv.Status != APStatusPosted {
			continue
		}
		var paid float64
		for _, alloc := range r.allocations[id] {
			paid += alloc.Amount
		}
		balance := inv.Total - paid
		if balance > 0 {
			balances = append(balances, APInvoiceBalance{
				ID:         inv.ID,
				DueAt:      inv.DueAt,
				Total:      inv.Total,
				PaidAmount: paid,
				Balance:    balance,
			})
		}
	}
	return balances, nil
}

func (tx *memoryAPTx) CreateAPInvoice(ctx context.Context, input CreateAPInvoiceInput) (int64, error) {
	tx.repo.nextID++
	id := tx.repo.nextID
	now := time.Now()
	inv := APInvoice{
		ID:         id,
		Number:     input.Number,
		SupplierID: input.SupplierID,
		GRNID:      input.GRNID,
		POID:       input.POID,
		Currency:   input.Currency,
		Subtotal:   input.Subtotal,
		TaxAmount:  input.TaxAmount,
		Total:      input.Total,
		Status:     APStatusDraft,
		DueAt:      input.DueDate,
		CreatedBy:  input.CreatedBy,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	tx.repo.invoices[id] = inv
	return id, nil
}

func (tx *memoryAPTx) CreateAPInvoiceLine(ctx context.Context, input CreateAPInvoiceLineInput, invoiceID int64) error {
	tx.repo.nextLineID++
	lineSubtotal := input.Quantity * input.UnitPrice * (1 - (input.DiscountPct / 100))
	lineTax := lineSubtotal * (input.TaxPct / 100)
	lineTotal := lineSubtotal + lineTax
	line := APInvoiceLine{
		ID:          tx.repo.nextLineID,
		APInvoiceID: invoiceID,
		GRNLineID:   input.GRNLineID,
		ProductID:   input.ProductID,
		Description: input.Description,
		Quantity:    input.Quantity,
		UnitPrice:   input.UnitPrice,
		DiscountPct: input.DiscountPct,
		TaxPct:      input.TaxPct,
		Subtotal:    lineSubtotal,
		TaxAmount:   lineTax,
		Total:       lineTotal,
		CreatedAt:   time.Now(),
	}
	tx.repo.lines[invoiceID] = append(tx.repo.lines[invoiceID], line)
	return nil
}

func (tx *memoryAPTx) UpdateAPStatus(ctx context.Context, id int64, status APInvoiceStatus) error {
	inv, ok := tx.repo.invoices[id]
	if !ok {
		return ErrInvoiceNotFound
	}
	inv.Status = status
	inv.UpdatedAt = time.Now()
	tx.repo.invoices[id] = inv
	return nil
}

func (tx *memoryAPTx) PostAPInvoice(ctx context.Context, input PostAPInvoiceInput) error {
	inv, ok := tx.repo.invoices[input.InvoiceID]
	if !ok {
		return ErrInvoiceNotFound
	}
	now := time.Now()
	inv.Status = APStatusPosted
	inv.PostedAt = &now
	inv.PostedBy = &input.PostedBy
	inv.UpdatedAt = now
	tx.repo.invoices[input.InvoiceID] = inv
	return nil
}

func (tx *memoryAPTx) VoidAPInvoice(ctx context.Context, input VoidAPInvoiceInput) error {
	inv, ok := tx.repo.invoices[input.InvoiceID]
	if !ok {
		return ErrInvoiceNotFound
	}
	now := time.Now()
	inv.Status = APStatusVoid
	inv.VoidedAt = &now
	inv.VoidedBy = &input.VoidedBy
	inv.VoidReason = &input.VoidReason
	inv.UpdatedAt = now
	tx.repo.invoices[input.InvoiceID] = inv
	return nil
}

func (tx *memoryAPTx) CreateAPPayment(ctx context.Context, input CreateAPPaymentInput) (int64, error) {
	tx.repo.nextPayID++
	id := tx.repo.nextPayID
	var apInvoiceID *int64
	if len(input.Allocations) > 0 {
		first := input.Allocations[0].APInvoiceID
		apInvoiceID = &first
	}
	payment := APPayment{
		ID:          id,
		Number:      input.Number,
		APInvoiceID: apInvoiceID,
		SupplierID:  input.SupplierID,
		Amount:      input.Amount,
		PaidAt:      input.PaidAt,
		Method:      input.Method,
		Note:        input.Note,
		CreatedBy:   input.CreatedBy,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	tx.repo.payments[id] = payment
	return id, nil
}

func (tx *memoryAPTx) CreatePaymentAllocation(ctx context.Context, input PaymentAllocationInput, paymentID int64) error {
	tx.repo.nextAllocID++
	alloc := APPaymentAllocation{
		ID:          tx.repo.nextAllocID,
		APPaymentID: paymentID,
		APInvoiceID: input.APInvoiceID,
		Amount:      input.Amount,
		CreatedAt:   time.Now(),
	}
	tx.repo.allocations[input.APInvoiceID] = append(tx.repo.allocations[input.APInvoiceID], alloc)
	return nil
}

func (tx *memoryAPTx) GenerateAPInvoiceNumber(ctx context.Context) (string, error) {
	tx.repo.numberCursor++
	return "INV-TEST-" + time.Now().Format("20060102") + "-" + fmtInt(tx.repo.numberCursor), nil
}

func (tx *memoryAPTx) GenerateAPPaymentNumber(ctx context.Context) (string, error) {
	tx.repo.numberCursor++
	return "PAY-TEST-" + time.Now().Format("20060102") + "-" + fmtInt(tx.repo.numberCursor), nil
}

type stubProcRepo struct {
	grns     map[int64]procurement.GoodsReceipt
	grnLines map[int64][]procurement.GRNLine
	pos      map[int64]procurement.PurchaseOrder
	poLines  map[int64][]procurement.POLine
}

func newStubProcRepo() *stubProcRepo {
	return &stubProcRepo{
		grns:     make(map[int64]procurement.GoodsReceipt),
		grnLines: make(map[int64][]procurement.GRNLine),
		pos:      make(map[int64]procurement.PurchaseOrder),
		poLines:  make(map[int64][]procurement.POLine),
	}
}

func (s *stubProcRepo) WithTx(ctx context.Context, fn func(context.Context, procurement.TxRepository) error) error {
	return errors.New("not implemented")
}

func (s *stubProcRepo) GetPR(ctx context.Context, id int64) (procurement.PurchaseRequest, []procurement.PRLine, error) {
	return procurement.PurchaseRequest{}, nil, procurement.ErrNotFound
}

func (s *stubProcRepo) GetPO(ctx context.Context, id int64) (procurement.PurchaseOrder, []procurement.POLine, error) {
	po, ok := s.pos[id]
	if !ok {
		return procurement.PurchaseOrder{}, nil, procurement.ErrNotFound
	}
	return po, append([]procurement.POLine(nil), s.poLines[id]...), nil
}

func (s *stubProcRepo) GetGRN(ctx context.Context, id int64) (procurement.GoodsReceipt, []procurement.GRNLine, error) {
	grn, ok := s.grns[id]
	if !ok {
		return procurement.GoodsReceipt{}, nil, procurement.ErrNotFound
	}
	return grn, append([]procurement.GRNLine(nil), s.grnLines[id]...), nil
}

func (s *stubProcRepo) ListPOs(ctx context.Context, limit, offset int, filters procurement.ListFilters) ([]procurement.POListItem, int, error) {
	return nil, 0, nil
}

func (s *stubProcRepo) ListGRNs(ctx context.Context, limit, offset int, filters procurement.ListFilters) ([]procurement.GRNListItem, int, error) {
	return nil, 0, nil
}

func TestCreateAPInvoiceFromGRN(t *testing.T) {
	ctx := context.Background()
	apRepo := newMemoryAPRepo()
	procRepo := newStubProcRepo()
	procRepo.grns[1] = procurement.GoodsReceipt{
		ID:         1,
		SupplierID: 10,
		POID:       22,
		Status:     procurement.GRNStatusPosted,
	}
	procRepo.grnLines[1] = []procurement.GRNLine{
		{ID: 1, ProductID: 100, Qty: 2, UnitCost: 50},
		{ID: 2, ProductID: 101, Qty: 1, UnitCost: 25},
	}
	procRepo.pos[22] = procurement.PurchaseOrder{
		ID:         22,
		SupplierID: 10,
		Status:     procurement.POStatusApproved,
		Currency:   "USD",
	}
	procSvc := procurement.NewService(procRepo, nil, nil, nil, nil, nil)
	svc := NewService(apRepo, procSvc)

	dueDate := time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)
	inv, err := svc.CreateAPInvoiceFromGRN(ctx, CreateAPInvoiceFromGRNInput{
		GRNID:     1,
		DueDate:   dueDate,
		CreatedBy: 5,
		Number:    "INV-1",
	})
	require.NoError(t, err)
	require.Equal(t, int64(10), inv.SupplierID)
	require.NotNil(t, inv.GRNID)
	require.NotNil(t, inv.POID)
	require.Equal(t, int64(1), *inv.GRNID)
	require.Equal(t, int64(22), *inv.POID)
	require.Equal(t, "USD", inv.Currency)
	require.InDelta(t, 125.0, inv.Total, 0.001)
	require.Len(t, apRepo.lines[inv.ID], 2)
}

func TestCreateAPInvoiceFromPO(t *testing.T) {
	ctx := context.Background()
	apRepo := newMemoryAPRepo()
	procRepo := newStubProcRepo()
	procRepo.pos[10] = procurement.PurchaseOrder{
		ID:         10,
		SupplierID: 7,
		Status:     procurement.POStatusApproved,
		Currency:   "IDR",
	}
	procRepo.poLines[10] = []procurement.POLine{
		{ID: 1, ProductID: 500, Qty: 3, Price: 40, Note: "Test line"},
	}
	procSvc := procurement.NewService(procRepo, nil, nil, nil, nil, nil)
	svc := NewService(apRepo, procSvc)

	inv, err := svc.CreateAPInvoiceFromPO(ctx, CreateAPInvoiceFromPOInput{
		POID:      10,
		DueDate:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		CreatedBy: 1,
		Number:    "INV-PO-1",
	})
	require.NoError(t, err)
	require.Equal(t, int64(7), inv.SupplierID)
	require.Nil(t, inv.GRNID)
	require.NotNil(t, inv.POID)
	require.Equal(t, int64(10), *inv.POID)
	require.Equal(t, "IDR", inv.Currency)
	require.InDelta(t, 120.0, inv.Total, 0.001)
}

func TestRegisterAPPaymentMultiAllocation(t *testing.T) {
	ctx := context.Background()
	apRepo := newMemoryAPRepo()
	procRepo := newStubProcRepo()
	procSvc := procurement.NewService(procRepo, nil, nil, nil, nil, nil)
	svc := NewService(apRepo, procSvc)

	apRepo.invoices[1] = APInvoice{ID: 1, SupplierID: 10, Total: 100, Status: APStatusPosted}
	apRepo.invoices[2] = APInvoice{ID: 2, SupplierID: 10, Total: 200, Status: APStatusPosted}

	_, err := svc.RegisterAPPayment(ctx, CreateAPPaymentInput{
		Amount:    150,
		PaidAt:    time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
		Method:    "TRANSFER",
		CreatedBy: 1,
		Allocations: []PaymentAllocationInput{
			{APInvoiceID: 1, Amount: 100},
			{APInvoiceID: 2, Amount: 50},
		},
	})
	require.NoError(t, err)
	require.Equal(t, APStatusPaid, apRepo.invoices[1].Status)
	require.Equal(t, APStatusPosted, apRepo.invoices[2].Status)
	require.NotEmpty(t, apRepo.payments)

	apRepo.invoices[3] = APInvoice{ID: 3, SupplierID: 99, Total: 50, Status: APStatusPosted}
	_, err = svc.RegisterAPPayment(ctx, CreateAPPaymentInput{
		Amount:    150,
		PaidAt:    time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC),
		Method:    "TRANSFER",
		CreatedBy: 1,
		Allocations: []PaymentAllocationInput{
			{APInvoiceID: 1, Amount: 100},
			{APInvoiceID: 3, Amount: 50},
		},
	})
	require.Error(t, err)
}

func fmtInt(val int64) string {
	return strconv.FormatInt(val, 10)
}
