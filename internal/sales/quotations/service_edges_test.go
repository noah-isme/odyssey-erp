package quotations

import (
	"context"
	"testing"
	"time"
)

func TestCreateQuotationRejectsBackwardsValidityWindow(t *testing.T) {
	quoteDate := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	_, err := NewService(newMemoryRepo(), &mockCustomerRepo{}).Create(context.Background(), CreateQuotationRequest{
		CompanyID: 1, CustomerID: 1, QuoteDate: quoteDate, ValidUntil: quoteDate.Add(-time.Hour),
	}, 1)
	if err == nil || err.Error() != "valid_until must be after quote_date" {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestApproveAndRejectRequireSubmittedQuotation(t *testing.T) {
	for _, action := range []struct {
		name string
		call func(*Service) error
	}{
		{name: "approve", call: func(s *Service) error { _, err := s.Approve(context.Background(), 1, 2); return err }},
		{name: "reject", call: func(s *Service) error { _, err := s.Reject(context.Background(), 1, 2, "not ready"); return err }},
	} {
		t.Run(action.name, func(t *testing.T) {
			repo := newMemoryRepo()
			repo.quotations[1] = &Quotation{ID: 1, Status: QuotationStatusDraft}
			if err := action.call(NewService(repo, &mockCustomerRepo{})); err == nil {
				t.Fatal("action accepted a draft quotation")
			}
		})
	}
}
