package crm

import (
	"context"
	"errors"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/sales/customers"
	"github.com/odyssey-erp/odyssey-erp/internal/sales/quotations"
)

var (
	ErrInvalidInput     = errors.New("crm: invalid input")
	ErrNotFound         = errors.New("crm: record not found")
	ErrForbidden        = errors.New("crm: record not visible")
	ErrInvalidStage     = errors.New("crm: invalid stage transition")
	ErrDuplicateContact = errors.New("crm: contact email already exists")
	ErrNotWon           = errors.New("crm: opportunity must be won before conversion")
)

type Scope struct {
	CompanyID, UserID int64
	ViewAll           bool
}
type Stage struct {
	ID, CompanyID         int64
	Name, Type            string
	Position, Probability int
}
type Lead struct {
	ID, CompanyID, OwnerID, CreatedBy                       int64
	Source, Name, Organization, Email, Phone, Status, Notes string
	CustomerID, ContactID                                   *int64
	CreatedAt, UpdatedAt                                    time.Time
}
type Opportunity struct {
	ID, CompanyID, OwnerID, StageID, CreatedBy int64
	LeadID, ContactID, CustomerID, QuotationID *int64
	Name, Source, Status, Reason               string
	ExpectedValue                              int64
	CloseDate                                  *time.Time
	StageName                                  string
	CreatedAt, UpdatedAt                       time.Time
}
type Activity struct {
	ID, CompanyID, OwnerID, CreatedBy, EscalationRecipientID    int64
	LeadID, OpportunityID, ContactID                            *int64
	Type, Subject, Body                                         string
	DueAt, CompletedAt, ReminderAt, ReminderSentAt, EscalatedAt *time.Time
	CreatedAt                                                   time.Time
}
type Event struct {
	ID                    int64
	EntityType, EventType string
	EntityID, ActorID     int64
	Details               string
	CreatedAt             time.Time
}
type Pipeline struct {
	Stages        []Stage
	Opportunities []Opportunity
	Leads         []Lead
}
type WinLoss struct {
	WonCount, LostCount int64
	WonValue, LostValue int64
	Reasons             []ReasonTotal
}
type ReasonTotal struct {
	Reason       string
	Count, Value int64
}

type CreateLeadInput struct {
	CompanyID, OwnerID, CreatedBy                   int64
	Source, Name, Organization, Email, Phone, Notes string
}
type QualifyInput struct {
	LeadID, ActorID int64
	OpportunityName string
	ExpectedValue   int64
	CloseDate       *time.Time
}
type ActivityInput struct {
	CompanyID, OwnerID, CreatedBy    int64
	LeadID, OpportunityID, ContactID *int64
	Type, Subject, Body              string
	DueAt, ReminderAt                *time.Time
}
type ConvertInput struct {
	OpportunityID, ActorID, ExistingCustomerID int64
	CustomerName, Country                      string
	QuoteDate, ValidUntil                      time.Time
	Currency                                   string
	Lines                                      []quotations.CreateQuotationLineReq
}
type Conversion struct {
	CustomerID, QuotationID int64
	Existing                bool
}

type Store interface {
	Stages(context.Context, Scope) ([]Stage, error)
	Pipeline(context.Context, Scope) (Pipeline, error)
	CreateLead(context.Context, CreateLeadInput) (Lead, error)
	Lead(context.Context, Scope, int64) (Lead, error)
	Qualify(context.Context, Scope, QualifyInput) (Opportunity, error)
	Opportunity(context.Context, Scope, int64) (Opportunity, error)
	Move(context.Context, Scope, int64, Stage, string, int64) (Opportunity, error)
	AddActivity(context.Context, Scope, ActivityInput) (Activity, error)
	CompleteActivity(context.Context, Scope, int64, int64, time.Time) error
	Timeline(context.Context, Scope, string, int64) ([]Activity, []Event, error)
	Reassign(context.Context, Scope, string, int64, int64, int64) error
	DueActivities(context.Context, time.Time, int) ([]Activity, error)
	MarkReminder(context.Context, int64, bool, time.Time) error
	CustomerByEmail(context.Context, int64, string) (int64, error)
	LinkCustomer(context.Context, Scope, int64, int64, int64) error
	LinkQuotation(context.Context, Scope, int64, int64, int64) error
	WinLoss(context.Context, Scope) (WinLoss, error)
}

type CustomerGateway interface {
	Create(context.Context, customers.CreateCustomerRequest, int64) (*customers.Customer, error)
	Get(context.Context, int64) (*customers.Customer, error)
	GetByCode(context.Context, int64, string) (*customers.Customer, error)
}
type QuotationGateway interface {
	Create(context.Context, quotations.CreateQuotationRequest, int64) (*quotations.Quotation, error)
}
type Notifier interface {
	Reminder(context.Context, Activity, bool) error
	Reassigned(context.Context, int64, string, int64) error
}
