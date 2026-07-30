package notifications

import "time"

const (
	TypeInvoiceIssued     = "invoice_issued"
	TypeApprovalRequested = "approval_requested"
	TypeReportDelivered   = "report_delivered"
	TypePasswordReset     = "password_reset"
	TypeApprovalAssigned  = "approval_assigned"
	TypeApprovalEscalated = "approval_escalated"
	TypeApprovalApproved  = "approval_approved"
	TypeApprovalRejected  = "approval_rejected"
)

type Notification struct {
	ID          int64      `json:"id"`
	RecipientID int64      `json:"recipientId"`
	DedupeKey   string     `json:"-"`
	Type        string     `json:"type"`
	Title       string     `json:"title"`
	Body        string     `json:"body"`
	URL         string     `json:"url"`
	ReadAt      *time.Time `json:"readAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

type Message struct {
	RecipientID int64
	DedupeKey   string
	Type        string
	Title       string
	Body        string
	URL         string
	EmailBody   string
}

type Channels struct {
	InApp bool
	Email bool
}
