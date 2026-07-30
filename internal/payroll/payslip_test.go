package payroll

import (
	"context"
	"testing"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/stretchr/testify/require"
)

type payslipStoreFake struct{ delivered int64 }

func (s *payslipStoreFake) DeliveryPayslip(context.Context, int64) (PayslipRecord, error) {
	return PayslipRecord{ID: 5, PeriodCode: "2026-07", Line: RunLine{EmployeeName: "Ayu", Email: "ayu@example.com", Result: Result{Gross: 10000000, NetPay: 9000000}}}, nil
}
func (s *payslipStoreFake) MarkPayslipDelivered(_ context.Context, id int64) error {
	s.delivered = id
	return nil
}

type rendererFake struct{}

func (rendererFake) RenderHTML(context.Context, string) ([]byte, error) {
	return []byte("%PDF-test"), nil
}

type mailerFake struct {
	attachment *shared.Attachment
	to         string
}

func (m *mailerFake) SendEmail(_ context.Context, to, subject, body string, a *shared.Attachment) error {
	m.to = to
	m.attachment = a
	return nil
}

func TestPayslipWorkerRendersAndEmailsPDF(t *testing.T) {
	store := &payslipStoreFake{}
	mail := &mailerFake{}
	err := NewPayslipProcessor(store, rendererFake{}, mail).DeliverPayslip(context.Background(), 5)
	require.NoError(t, err)
	require.Equal(t, "ayu@example.com", mail.to)
	require.Equal(t, "application/pdf", mail.attachment.ContentType)
	require.Equal(t, []byte("%PDF-test"), mail.attachment.Data)
	require.Equal(t, int64(5), store.delivered)
}
