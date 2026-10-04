package payroll

import (
	"context"
	"errors"
	"testing"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/stretchr/testify/require"
)

// payslipStoreFake models Repository.DeliverPayslipOnce: it invokes deliver
// only for an undelivered, unlocked payslip and marks it delivered only when
// deliver and the mark both succeed.
type payslipStoreFake struct {
	exists    bool
	delivered bool
	locked    bool
	markErr   error
	marks     int
}

func (s *payslipStoreFake) DeliverPayslipOnce(ctx context.Context, id int64, deliver func(context.Context, PayslipRecord) error) (bool, error) {
	if !s.exists {
		return false, ErrPayslipNotFound
	}
	if s.delivered || s.locked {
		return false, nil
	}
	record := PayslipRecord{ID: id, PeriodCode: "2026-07", Line: RunLine{EmployeeName: "Ayu", Email: "ayu@example.com", Result: Result{Gross: 10000000, NetPay: 9000000}}}
	if err := deliver(ctx, record); err != nil {
		return false, err
	}
	if s.markErr != nil {
		return false, s.markErr
	}
	s.delivered = true
	s.marks++
	return true, nil
}

type rendererFake struct {
	calls int
	err   error
}

func (r *rendererFake) RenderHTML(context.Context, string) ([]byte, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	return []byte("%PDF-test"), nil
}

type mailerFake struct {
	calls      int
	attachment *shared.Attachment
	to         string
	err        error
}

func (m *mailerFake) SendEmail(_ context.Context, to, subject, body string, a *shared.Attachment) error {
	m.calls++
	m.to = to
	m.attachment = a
	return m.err
}

func TestPayslipWorkerRendersAndEmailsPDF(t *testing.T) {
	store := &payslipStoreFake{exists: true}
	mail := &mailerFake{}
	err := NewPayslipProcessor(store, &rendererFake{}, mail).DeliverPayslip(context.Background(), 5)
	require.NoError(t, err)
	require.Equal(t, "ayu@example.com", mail.to)
	require.Equal(t, "application/pdf", mail.attachment.ContentType)
	require.Equal(t, "payslip-2026-07.pdf", mail.attachment.Filename)
	require.Equal(t, []byte("%PDF-test"), mail.attachment.Data)
	require.True(t, store.delivered)
}

func TestDeliverPayslipIdempotency(t *testing.T) {
	renderErr := errors.New("gotenberg down")
	sendErr := errors.New("smtp down")
	markErr := errors.New("commit failed")

	tests := []struct {
		name          string
		store         payslipStoreFake
		renderErr     error
		sendErr       error
		wantErr       error
		wantRenders   int
		wantEmails    int
		wantDelivered bool
	}{
		{name: "first delivery", store: payslipStoreFake{exists: true}, wantRenders: 1, wantEmails: 1, wantDelivered: true},
		{name: "already delivered", store: payslipStoreFake{exists: true, delivered: true}, wantDelivered: true},
		{name: "locked by another worker", store: payslipStoreFake{exists: true, locked: true}},
		{name: "not found", store: payslipStoreFake{}, wantErr: ErrPayslipNotFound},
		{name: "render failure", store: payslipStoreFake{exists: true}, renderErr: renderErr, wantErr: renderErr, wantRenders: 1},
		{name: "send failure", store: payslipStoreFake{exists: true}, sendErr: sendErr, wantErr: sendErr, wantRenders: 1, wantEmails: 1},
		// At-least-once window: the email was accepted but the mark/commit
		// failed, so the payslip stays undelivered and the retry re-sends.
		{name: "mark failure after send", store: payslipStoreFake{exists: true, markErr: markErr}, wantErr: markErr, wantRenders: 1, wantEmails: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := tc.store
			renderer := &rendererFake{err: tc.renderErr}
			mail := &mailerFake{err: tc.sendErr}
			err := NewPayslipProcessor(&store, renderer, mail).DeliverPayslip(context.Background(), 5)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.Zero(t, store.marks, "no mark on error")
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.wantRenders, renderer.calls, "renders")
			require.Equal(t, tc.wantEmails, mail.calls, "emails")
			require.Equal(t, tc.wantDelivered, store.delivered, "delivered")
		})
	}
}

func TestDeliverPayslipRequiresConfiguration(t *testing.T) {
	require.ErrorIs(t, NewPayslipProcessor(nil, &rendererFake{}, &mailerFake{}).DeliverPayslip(context.Background(), 5), ErrConfiguration)
	require.ErrorIs(t, NewPayslipProcessor(&payslipStoreFake{exists: true}, &rendererFake{}, nil).DeliverPayslip(context.Background(), 5), ErrConfiguration)
}
