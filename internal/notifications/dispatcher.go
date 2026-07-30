package notifications

import (
	"context"
	"crypto/sha256"
	"fmt"
)

type PreferenceStore interface {
	Channels(context.Context, int64, string) (Channels, error)
	UserEmail(context.Context, int64) (string, error)
}

type Email struct {
	To, Subject, Body string
	CorrelationID     string
}
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
	if msg.RecipientID <= 0 || msg.DedupeKey == "" || msg.Type == "" || msg.Title == "" {
		return ErrInvalidNotification
	}
	channels, err := d.prefs.Channels(ctx, msg.RecipientID, msg.Type)
	if err != nil {
		return err
	}
	var notificationID int64
	if channels.InApp {
		created, err := d.service.Create(ctx, Notification{RecipientID: msg.RecipientID, DedupeKey: msg.DedupeKey, Type: msg.Type, Title: msg.Title, Body: msg.Body, URL: msg.URL})
		if err != nil {
			return err
		}
		notificationID = created.ID
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
		correlationID := fmt.Sprintf("notification-email-%d", notificationID)
		if notificationID == 0 {
			sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%s:%s", msg.RecipientID, msg.Type, msg.DedupeKey)))
			correlationID = fmt.Sprintf("notification-email-%x", sum)
		}
		if err := d.email.EnqueueEmail(ctx, Email{To: address, Subject: msg.Title, Body: body, CorrelationID: correlationID}); err != nil {
			return err
		}
	}
	return nil
}

func InvoiceIssued(recipientID, invoiceID int64, number string) Message {
	return Message{RecipientID: recipientID, DedupeKey: fmt.Sprintf("invoice:%d", invoiceID), Type: TypeInvoiceIssued, Title: "Invoice issued", Body: fmt.Sprintf("Invoice %s has been issued.", number), URL: fmt.Sprintf("/finance/ar/invoices/%d", invoiceID)}
}
func ApprovalRequested(recipientID, poID int64, number string) Message {
	return Message{RecipientID: recipientID, DedupeKey: fmt.Sprintf("purchase-order:%d", poID), Type: TypeApprovalRequested, Title: "Approval requested", Body: fmt.Sprintf("Purchase order %s is awaiting approval.", number), URL: fmt.Sprintf("/procurement/pos/%d", poID)}
}
func ReportDelivered(recipientID, reportID int64) Message {
	return Message{RecipientID: recipientID, DedupeKey: fmt.Sprintf("report:%d", reportID), Type: TypeReportDelivered, Title: "Report delivered", Body: "Your report is ready to download.", URL: fmt.Sprintf("/board-packs/%d", reportID)}
}
func PasswordReset(recipientID int64, eventID string) Message {
	return Message{RecipientID: recipientID, DedupeKey: "password-change:" + eventID, Type: TypePasswordReset, Title: "Password changed", Body: "Your Odyssey password was changed successfully.", URL: "/settings"}
}
