package ar

import (
	"context"
	"errors"
	"time"
)

// RepositoryPort defines data access methods for AR.
type RepositoryPort interface {
	CreateARInvoice(ctx context.Context, input ARInvoiceInput) (*ARInvoice, error)
	CreateARPayment(ctx context.Context, input ARPaymentInput) (*ARPayment, error)
	ListARInvoices(ctx context.Context) ([]ARInvoice, error)
	ListARPayments(ctx context.Context) ([]ARPayment, error)
	ListAROutstanding(ctx context.Context) ([]ARInvoice, error)
}

// Service handles AR business logic.
type Service struct {
	repo RepositoryPort
}

// NewService builds Service instance.
func NewService(repo RepositoryPort) *Service {
	return &Service{repo: repo}
}

// CreateARInvoiceFromSO creates an AR invoice from a sales order.
func (s *Service) CreateARInvoiceFromSO(ctx context.Context, input ARInvoiceInput) (*ARInvoice, error) {
	if input.CustomerID == 0 {
		return nil, errors.New("customer ID required")
	}
	if input.SOID == 0 {
		return nil, errors.New("sales order ID required")
	}
	if input.Total <= 0 {
		return nil, errors.New("total must be positive")
	}
	now := time.Now()
	input.CreatedAt = now
	input.UpdatedAt = now
	return s.repo.CreateARInvoice(ctx, input)
}

// RegisterARPayment records a payment against an AR invoice.
func (s *Service) RegisterARPayment(ctx context.Context, input ARPaymentInput) (*ARPayment, error) {
	if input.ARInvoiceID == 0 {
		return nil, errors.New("AR invoice ID required")
	}
	if input.Amount <= 0 {
		return nil, errors.New("amount must be positive")
	}
	now := time.Now()
	input.CreatedAt = now
	input.UpdatedAt = now
	return s.repo.CreateARPayment(ctx, input)
}

// GetARInvoices returns all AR invoices.
func (s *Service) GetARInvoices(ctx context.Context) ([]ARInvoice, error) {
	return s.repo.ListARInvoices(ctx)
}

// GetARPayments returns all AR payments.
func (s *Service) GetARPayments(ctx context.Context) ([]ARPayment, error) {
	return s.repo.ListARPayments(ctx)
}

// CalculateARAging groups invoices by due date buckets.
func (s *Service) CalculateARAging(ctx context.Context, asOf time.Time) (ARAgingBucket, error) {
	invoices, err := s.repo.ListAROutstanding(ctx)
	if err != nil {
		return ARAgingBucket{}, err
	}
	if asOf.IsZero() {
		asOf = time.Now()
	}
	var bucket ARAgingBucket
	for _, inv := range invoices {
		if inv.Status == ARStatusPaid {
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
