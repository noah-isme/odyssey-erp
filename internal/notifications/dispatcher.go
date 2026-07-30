package notifications

import (
	"context"
	"fmt"
)

type PreferenceStore interface {
	Channels(context.Context, int64, string) (Channels, error)
	UserEmail(context.Context, int64) (string, error)
}

type Email struct{ To, Subject, Body string }
type EmailEnqueuer interface {
	EnqueueEmail(context.Context, Email) error
}

type Dispatcher struct {
	service *Service
	prefs   PreferenceStore
	email   EmailEnqueuer
}

func NewDispatcher(service *Service, prefs PreferenceStore, email EmailEnqueuer) *Dispatcher {
	return &Dispatcher{service: service, prefs: prefs, email: email}
}

func (d *Dispatcher) Dispatch(ctx context.Context, msg Message) error {
	if msg.RecipientID <= 0 || msg.Type == "" || msg.Title == "" {
		return ErrInvalidNotification
	}
	channels, err := d.prefs.Channels(ctx, msg.RecipientID, msg.Type)
	if err != nil {
		return err
	}
	if channels.InApp {
		if _, err := d.service.Create(ctx, Notification{RecipientID: msg.RecipientID, Type: msg.Type, Title: msg.Title, Body: msg.Body, URL: msg.URL}); err != nil {
			return err
		}
	}
	if channels.Email && d.email != nil {
		address, err := d.prefs.UserEmail(ctx, msg.RecipientID)
		if err != nil {
			return err
		}
		body := msg.EmailBody
		if body == "" {
			body = msg.Body
		}
		if err := d.email.EnqueueEmail(ctx, Email{To: address, Subject: msg.Title, Body: body}); err != nil {
			return err
		}
	}
	return nil
}

func InvoiceIssued(recipientID, invoiceID int64, number string) Message {
	return Message{RecipientID: recipientID, Type: TypeInvoiceIssued, Title: "Invoice issued", Body: fmt.Sprintf("Invoice %s has been issued.", number), URL: fmt.Sprintf("/finance/ar/invoices/%d", invoiceID)}
}
func ApprovalRequested(recipientID, poID int64, number string) Message {
	return Message{RecipientID: recipientID, Type: TypeApprovalRequested, Title: "Approval requested", Body: fmt.Sprintf("Purchase order %s is awaiting approval.", number), URL: fmt.Sprintf("/procurement/pos/%d", poID)}
}
func ReportDelivered(recipientID, reportID int64) Message {
	return Message{RecipientID: recipientID, Type: TypeReportDelivered, Title: "Report delivered", Body: "Your report is ready to download.", URL: fmt.Sprintf("/board-packs/%d", reportID)}
}
func PasswordReset(recipientID int64) Message {
	return Message{RecipientID: recipientID, Type: TypePasswordReset, Title: "Password changed", Body: "Your Odyssey password was changed successfully.", URL: "/settings"}
}
