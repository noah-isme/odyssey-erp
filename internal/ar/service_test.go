package ar

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type memoryARRepo struct {
	invoices       map[int64]*ARInvoice
	invoiceLines   map[int64][]ARInvoiceLine
	payments       map[int64]*ARPayment
	allocations    map[int64][]PaymentAllocationInput
	nextInvoiceID  int64
	nextPaymentID  int64
	nextLineID     int64
	invoiceCounter int64
	paymentCounter int64
}

func newMemoryARRepo() *memoryARRepo {
	return &memoryARRepo{
		invoices:     make(map[int64]*ARInvoice),
		invoiceLines: make(map[int64][]ARInvoiceLine),
		payments:     make(map[int64]*ARPayment),
		allocations:  make(map[int64][]PaymentAllocationInput),
	}
}

func (r *memoryARRepo) CreateARInvoice(ctx context.Context, input CreateARInvoiceInput) (*ARInvoice, error) {
	r.nextInvoiceID++
	inv := &ARInvoice{
		ID:              r.nextInvoiceID,
		Number:          input.Number,
		CustomerID:      input.CustomerID,
		SOID:            input.SOID,
		DeliveryOrderID: input.DeliveryOrderID,
		Currency:        input.Currency,
		Subtotal:        input.Subtotal,
		TaxAmount:       input.TaxAmount,
		Total:           input.Total,
		Status:          ARStatusDraft,
		DueAt:           input.DueDate,
		CreatedBy:       input.CreatedBy,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	r.invoices[inv.ID] = inv
	return inv, nil
}

func (r *memoryARRepo) CreateARInvoiceLine(ctx context.Context, invoiceID int64, line CreateARInvoiceLineInput) (*ARInvoiceLine, error) {
	r.nextLineID++
	l := ARInvoiceLine{
		ID:          r.nextLineID,
		ARInvoiceID: invoiceID,
		ProductID:   line.ProductID,
		Description: line.Description,
		Quantity:    line.Quantity,
		UnitPrice:   line.UnitPrice,
		DiscountPct: line.DiscountPct,
		TaxPct:      line.TaxPct,
	}
	r.invoiceLines[invoiceID] = append(r.invoiceLines[invoiceID], l)
	return &l, nil
}

func (r *memoryARRepo) GetARInvoice(ctx context.Context, id int64) (*ARInvoice, error) {
	inv, ok := r.invoices[id]
	if !ok {
		return nil, nil
	}
	return inv, nil
}

func (r *memoryARRepo) GetARInvoiceWithDetails(ctx context.Context, id int64) (*ARInvoiceWithDetails, error) {
	inv, ok := r.invoices[id]
	if !ok {
		return nil, nil
	}
	lines := r.invoiceLines[id]
	var paid float64
	for _, allocs := range r.allocations {
		for _, a := range allocs {
			if a.ARInvoiceID == id {
				paid += a.Amount
			}
		}
	}
	return &ARInvoiceWithDetails{
		ARInvoice:  *inv,
		Lines:      lines,
		PaidAmount: paid,
		Balance:    inv.Total - paid,
	}, nil
}

func (r *memoryARRepo) ListARInvoices(ctx context.Context, req ListARInvoicesRequest) ([]ARInvoice, error) {
	var out []ARInvoice
	for _, inv := range r.invoices {
		if req.Status != "" && inv.Status != req.Status {
			continue
		}
		if req.CustomerID != 0 && inv.CustomerID != req.CustomerID {
			continue
		}
		out = append(out, *inv)
	}
	return out, nil
}

func (r *memoryARRepo) ListARInvoiceLines(ctx context.Context, invoiceID int64) ([]ARInvoiceLine, error) {
	return r.invoiceLines[invoiceID], nil
}

func (r *memoryARRepo) PostARInvoice(ctx context.Context, id int64, postedBy int64) error {
	inv, ok := r.invoices[id]
	if !ok {
		return ErrInvoiceNotFound
	}
	now := time.Now()
	inv.Status = ARStatusPosted
	inv.PostedAt = &now
	inv.PostedBy = &postedBy
	return nil
}

func (r *memoryARRepo) VoidARInvoice(ctx context.Context, id int64, voidedBy int64, reason string) error {
	inv, ok := r.invoices[id]
	if !ok {
		return ErrInvoiceNotFound
	}
	now := time.Now()
	inv.Status = ARStatusVoid
	inv.VoidedAt = &now
	inv.VoidedBy = &voidedBy
	inv.VoidReason = reason
	return nil
}

func (r *memoryARRepo) GetInvoiceBalance(ctx context.Context, id int64) (total, paid, balance float64, err error) {
	inv, ok := r.invoices[id]
	if !ok {
		return 0, 0, 0, ErrInvoiceNotFound
	}
	for _, allocs := range r.allocations {
		for _, a := range allocs {
			if a.ARInvoiceID == id {
				paid += a.Amount
			}
		}
	}
	return inv.Total, paid, inv.Total - paid, nil
}

func (r *memoryARRepo) CountInvoicesByDelivery(ctx context.Context, deliveryOrderID int64) (int, error) {
	count := 0
	for _, inv := range r.invoices {
		if inv.DeliveryOrderID == deliveryOrderID {
			count++
		}
	}
	return count, nil
}

func (r *memoryARRepo) GenerateInvoiceNumber(ctx context.Context) (string, error) {
	r.invoiceCounter++
	return "INV-TEST-" + string(rune('0'+r.invoiceCounter)), nil
}

func (r *memoryARRepo) CreateARPayment(ctx context.Context, input CreateARPaymentInput) (*ARPayment, error) {
	r.nextPaymentID++
	pay := &ARPayment{
		ID:        r.nextPaymentID,
		Number:    input.Number,
		Amount:    input.Amount,
		PaidAt:    input.PaidAt,
		Method:    input.Method,
		Note:      input.Note,
		CreatedBy: input.CreatedBy,
		CreatedAt: time.Now(),
	}
	r.payments[pay.ID] = pay
	return pay, nil
}

func (r *memoryARRepo) CreatePaymentAllocation(ctx context.Context, paymentID, invoiceID int64, amount float64) error {
	r.allocations[paymentID] = append(r.allocations[paymentID], PaymentAllocationInput{
		ARInvoiceID: invoiceID,
		Amount:      amount,
	})
	return nil
}

func (r *memoryARRepo) ListARPayments(ctx context.Context) ([]ARPayment, error) {
	var out []ARPayment
	for _, p := range r.payments {
		out = append(out, *p)
	}
	return out, nil
}

func (r *memoryARRepo) ListInvoicePayments(ctx context.Context, invoiceID int64) ([]ARPaymentSummary, error) {
	var out []ARPaymentSummary
	for payID, allocs := range r.allocations {
		for _, a := range allocs {
			if a.ARInvoiceID == invoiceID {
				pay := r.payments[payID]
				out = append(out, ARPaymentSummary{
					ID:              pay.ID,
					Number:          pay.Number,
					Amount:          pay.Amount,
					AllocatedAmount: a.Amount,
					PaidAt:          pay.PaidAt,
					Method:          pay.Method,
				})
			}
		}
	}
	return out, nil
}

func (r *memoryARRepo) GeneratePaymentNumber(ctx context.Context) (string, error) {
	r.paymentCounter++
	return "PAY-TEST-" + string(rune('0'+r.paymentCounter)), nil
}

func (r *memoryARRepo) ListAROutstanding(ctx context.Context) ([]ARInvoice, error) {
	var out []ARInvoice
	for _, inv := range r.invoices {
		if inv.Status == ARStatusPosted {
			out = append(out, *inv)
		}
	}
	return out, nil
}

func TestCreateARInvoice(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryARRepo()
	svc := NewService(repo)

	dueDate := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)
	input := CreateARInvoiceInput{
		CustomerID: 100,
		SOID:       200,
		Number:     "INV-001",
		Currency:   "IDR",
		Subtotal:   1000,
		TaxAmount:  100,
		Total:      1100,
		DueDate:    dueDate,
		CreatedBy:  1,
		Lines: []CreateARInvoiceLineInput{
			{ProductID: 10, Description: "Product A", Quantity: 2, UnitPrice: 500},
		},
	}

	inv, err := svc.CreateARInvoice(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, inv)
	require.Equal(t, "INV-001", inv.Number)
	require.Equal(t, int64(100), inv.CustomerID)
	require.Equal(t, ARStatusDraft, inv.Status)
	require.Equal(t, 1100.0, inv.Total)

	lines := repo.invoiceLines[inv.ID]
	require.Len(t, lines, 1)
	require.Equal(t, "Product A", lines[0].Description)
}

func TestCreateARInvoiceRequiresCustomerID(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryARRepo()
	svc := NewService(repo)

	_, err := svc.CreateARInvoice(ctx, CreateARInvoiceInput{
		Total: 100,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "customer ID required")
}

func TestCreateARInvoiceRequiresPositiveTotal(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryARRepo()
	svc := NewService(repo)

	_, err := svc.CreateARInvoice(ctx, CreateARInvoiceInput{
		CustomerID: 100,
		Total:      0,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "total must be positive")
}

func TestPostARInvoice(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryARRepo()
	svc := NewService(repo)

	inv, _ := svc.CreateARInvoice(ctx, CreateARInvoiceInput{
		CustomerID: 100,
		Number:     "INV-002",
		Total:      500,
		CreatedBy:  1,
	})

	err := svc.PostARInvoice(ctx, PostARInvoiceInput{
		InvoiceID: inv.ID,
		PostedBy:  2,
	})
	require.NoError(t, err)

	updated, _ := repo.GetARInvoice(ctx, inv.ID)
	require.Equal(t, ARStatusPosted, updated.Status)
	require.NotNil(t, updated.PostedAt)
}

func TestPostARInvoiceInvalidStatus(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryARRepo()
	svc := NewService(repo)

	inv, _ := svc.CreateARInvoice(ctx, CreateARInvoiceInput{
		CustomerID: 100,
		Number:     "INV-003",
		Total:      500,
		CreatedBy:  1,
	})

	_ = svc.PostARInvoice(ctx, PostARInvoiceInput{InvoiceID: inv.ID, PostedBy: 1})

	err := svc.PostARInvoice(ctx, PostARInvoiceInput{InvoiceID: inv.ID, PostedBy: 1})
	require.Error(t, err)
	require.Equal(t, ErrInvalidStatus, err)
}

func TestVoidARInvoice(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryARRepo()
	svc := NewService(repo)

	inv, _ := svc.CreateARInvoice(ctx, CreateARInvoiceInput{
		CustomerID: 100,
		Number:     "INV-004",
		Total:      500,
		CreatedBy:  1,
	})

	err := svc.VoidARInvoice(ctx, VoidARInvoiceInput{
		InvoiceID:  inv.ID,
		VoidedBy:   3,
		VoidReason: "Customer cancelled",
	})
	require.NoError(t, err)

	updated, _ := repo.GetARInvoice(ctx, inv.ID)
	require.Equal(t, ARStatusVoid, updated.Status)
	require.Equal(t, "Customer cancelled", updated.VoidReason)
}

func TestRegisterARPayment(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryARRepo()
	svc := NewService(repo)

	inv, _ := svc.CreateARInvoice(ctx, CreateARInvoiceInput{
		CustomerID: 100,
		Number:     "INV-005",
		Total:      1000,
		CreatedBy:  1,
	})
	_ = svc.PostARInvoice(ctx, PostARInvoiceInput{InvoiceID: inv.ID, PostedBy: 1})

	paidAt := time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC)
	pay, err := svc.RegisterARPayment(ctx, CreateARPaymentInput{
		Number:    "PAY-001",
		Amount:    500,
		PaidAt:    paidAt,
		Method:    "transfer",
		Note:      "Partial payment",
		CreatedBy: 2,
		Allocations: []PaymentAllocationInput{
			{ARInvoiceID: inv.ID, Amount: 500},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, pay)
	require.Equal(t, "PAY-001", pay.Number)
	require.Equal(t, 500.0, pay.Amount)

	_, paid, balance, _ := repo.GetInvoiceBalance(ctx, inv.ID)
	require.Equal(t, 500.0, paid)
	require.Equal(t, 500.0, balance)
}

func TestRegisterARPaymentExceedsBalance(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryARRepo()
	svc := NewService(repo)

	inv, _ := svc.CreateARInvoice(ctx, CreateARInvoiceInput{
		CustomerID: 100,
		Number:     "INV-006",
		Total:      100,
		CreatedBy:  1,
	})
	_ = svc.PostARInvoice(ctx, PostARInvoiceInput{InvoiceID: inv.ID, PostedBy: 1})

	_, err := svc.RegisterARPayment(ctx, CreateARPaymentInput{
		Number:    "PAY-002",
		Amount:    200,
		CreatedBy: 2,
		Allocations: []PaymentAllocationInput{
			{ARInvoiceID: inv.ID, Amount: 200},
		},
	})
	require.Error(t, err)
	require.Equal(t, ErrInsufficientAmount, err)
}

func TestListARInvoices(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryARRepo()
	svc := NewService(repo)

	_, _ = svc.CreateARInvoice(ctx, CreateARInvoiceInput{CustomerID: 100, Number: "INV-A", Total: 100, CreatedBy: 1})
	inv2, _ := svc.CreateARInvoice(ctx, CreateARInvoiceInput{CustomerID: 100, Number: "INV-B", Total: 200, CreatedBy: 1})
	_ = svc.PostARInvoice(ctx, PostARInvoiceInput{InvoiceID: inv2.ID, PostedBy: 1})

	all, err := svc.ListARInvoices(ctx, ListARInvoicesRequest{})
	require.NoError(t, err)
	require.Len(t, all, 2)

	posted, err := svc.ListARInvoices(ctx, ListARInvoicesRequest{Status: ARStatusPosted})
	require.NoError(t, err)
	require.Len(t, posted, 1)
	require.Equal(t, "INV-B", posted[0].Number)
}

func TestCalculateARAging(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryARRepo()
	svc := NewService(repo)

	now := time.Now()
	current := now.AddDate(0, 0, 5)
	overdue30 := now.AddDate(0, 0, -20)
	overdue60 := now.AddDate(0, 0, -50)

	inv1, _ := svc.CreateARInvoice(ctx, CreateARInvoiceInput{CustomerID: 100, Number: "INV-C1", Total: 100, DueDate: current, CreatedBy: 1})
	inv2, _ := svc.CreateARInvoice(ctx, CreateARInvoiceInput{CustomerID: 100, Number: "INV-C2", Total: 200, DueDate: overdue30, CreatedBy: 1})
	inv3, _ := svc.CreateARInvoice(ctx, CreateARInvoiceInput{CustomerID: 100, Number: "INV-C3", Total: 300, DueDate: overdue60, CreatedBy: 1})

	_ = svc.PostARInvoice(ctx, PostARInvoiceInput{InvoiceID: inv1.ID, PostedBy: 1})
	_ = svc.PostARInvoice(ctx, PostARInvoiceInput{InvoiceID: inv2.ID, PostedBy: 1})
	_ = svc.PostARInvoice(ctx, PostARInvoiceInput{InvoiceID: inv3.ID, PostedBy: 1})

	bucket, err := svc.CalculateARAging(ctx, now)
	require.NoError(t, err)
	require.Equal(t, 100.0, bucket.Current)
	require.Equal(t, 200.0, bucket.Bucket30)
	require.Equal(t, 300.0, bucket.Bucket60)
}
