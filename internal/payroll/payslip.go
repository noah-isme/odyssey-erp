package payroll

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log/slog"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/web"
)

type PDFRenderer interface {
	RenderHTML(context.Context, string) ([]byte, error)
}
type PayslipMailer interface {
	SendEmail(context.Context, string, string, string, *shared.Attachment) error
}

type PayslipProcessor struct {
	store    PayslipStore
	renderer PDFRenderer
	mailer   PayslipMailer
}

func NewPayslipProcessor(store PayslipStore, renderer PDFRenderer, mailer PayslipMailer) *PayslipProcessor {
	return &PayslipProcessor{store: store, renderer: renderer, mailer: mailer}
}

func (p *PayslipProcessor) Render(ctx context.Context, record PayslipRecord) ([]byte, error) {
	if p.renderer == nil {
		return nil, ErrConfiguration
	}
	tpl, err := template.ParseFS(web.Templates, "templates/reports/payroll_payslip_pdf.html")
	if err != nil {
		return nil, err
	}
	var html bytes.Buffer
	if err = tpl.Execute(&html, record); err != nil {
		return nil, err
	}
	return p.renderer.RenderHTML(ctx, html.String())
}

// DeliverPayslip renders and emails a payslip exactly once per successful
// commit. A redelivered task for a payslip that is already delivered, or that
// another worker is delivering right now, returns nil without rendering or
// sending. Delivery is at-least-once: a crash or database failure after the
// SMTP server accepted the message and before the delivery commit re-sends the
// payslip on retry (see Repository.DeliverPayslipOnce).
func (p *PayslipProcessor) DeliverPayslip(ctx context.Context, payslipID int64) error {
	if p.store == nil || p.mailer == nil {
		return ErrConfiguration
	}
	delivered, err := p.store.DeliverPayslipOnce(ctx, payslipID, func(ctx context.Context, record PayslipRecord) error {
		pdf, err := p.Render(ctx, record)
		if err != nil {
			return err
		}
		attachment := &shared.Attachment{Filename: fmt.Sprintf("payslip-%s.pdf", record.PeriodCode), ContentType: "application/pdf", Data: pdf}
		return p.mailer.SendEmail(ctx, record.Line.Email, "Payslip "+record.PeriodCode, "<p>Your payslip is attached. It is confidential.</p>", attachment)
	})
	if err != nil {
		return err
	}
	if !delivered {
		slog.InfoContext(ctx, "payslip already delivered or in flight", slog.Int64("payslip_id", payslipID))
	}
	return nil
}
