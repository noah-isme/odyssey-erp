package crm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/sales/customers"
	"github.com/odyssey-erp/odyssey-erp/internal/sales/quotations"
)

type storeFake struct {
	stages                          []Stage
	leads                           map[int64]Lead
	opportunities                   map[int64]Opportunity
	due                             []Activity
	qualifyErr                      error
	linkedCustomer, linkedQuotation int64
	reminders                       []bool
	lastScope                       Scope
}

func newStoreFake() *storeFake {
	return &storeFake{leads: map[int64]Lead{}, opportunities: map[int64]Opportunity{}}
}
func (f *storeFake) Stages(context.Context, Scope) ([]Stage, error) { return f.stages, nil }
func (f *storeFake) Pipeline(_ context.Context, scope Scope) (Pipeline, error) {
	f.lastScope = scope
	return Pipeline{Stages: f.stages}, nil
}
func (f *storeFake) CreateLead(_ context.Context, in CreateLeadInput) (Lead, error) {
	x := Lead{ID: int64(len(f.leads) + 1), CompanyID: in.CompanyID, OwnerID: in.OwnerID, Name: in.Name, Email: in.Email}
	f.leads[x.ID] = x
	return x, nil
}
func (f *storeFake) Lead(_ context.Context, _ Scope, id int64) (Lead, error) {
	x, ok := f.leads[id]
	if !ok {
		return Lead{}, ErrNotFound
	}
	return x, nil
}
func (f *storeFake) Qualify(context.Context, Scope, QualifyInput) (Opportunity, error) {
	if f.qualifyErr != nil {
		return Opportunity{}, f.qualifyErr
	}
	return Opportunity{ID: 1}, nil
}
func (f *storeFake) Opportunity(_ context.Context, _ Scope, id int64) (Opportunity, error) {
	x, ok := f.opportunities[id]
	if !ok {
		return Opportunity{}, ErrNotFound
	}
	return x, nil
}
func (f *storeFake) Move(_ context.Context, _ Scope, id int64, stage Stage, reason string, _ int64) (Opportunity, error) {
	x := f.opportunities[id]
	x.StageID = stage.ID
	x.StageName = stage.Name
	x.Status = stage.Type
	x.Reason = reason
	f.opportunities[id] = x
	return x, nil
}
func (f *storeFake) AddActivity(context.Context, Scope, ActivityInput) (Activity, error) {
	return Activity{ID: 1}, nil
}
func (f *storeFake) CompleteActivity(context.Context, Scope, int64, int64, time.Time) error {
	return nil
}
func (f *storeFake) Timeline(context.Context, Scope, string, int64) ([]Activity, []Event, error) {
	return nil, nil, nil
}
func (f *storeFake) Reassign(context.Context, Scope, string, int64, int64, int64) error { return nil }
func (f *storeFake) DueActivities(context.Context, time.Time, int) ([]Activity, error) {
	return f.due, nil
}
func (f *storeFake) MarkReminder(_ context.Context, _ int64, escalated bool, _ time.Time) error {
	f.reminders = append(f.reminders, escalated)
	return nil
}
func (f *storeFake) CustomerByEmail(context.Context, int64, string) (int64, error) {
	return 0, ErrNotFound
}
func (f *storeFake) LinkCustomer(_ context.Context, _ Scope, id, customerID, _ int64) error {
	x := f.opportunities[id]
	x.CustomerID = &customerID
	f.opportunities[id] = x
	f.linkedCustomer = customerID
	return nil
}
func (f *storeFake) LinkQuotation(_ context.Context, _ Scope, id, quotationID, _ int64) error {
	x := f.opportunities[id]
	x.QuotationID = &quotationID
	f.opportunities[id] = x
	f.linkedQuotation = quotationID
	return nil
}
func (f *storeFake) WinLoss(context.Context, Scope) (WinLoss, error) { return WinLoss{}, nil }

type customerFake struct {
	items   map[int64]*customers.Customer
	byCode  map[string]*customers.Customer
	creates int
}

func (f *customerFake) Create(_ context.Context, in customers.CreateCustomerRequest, _ int64) (*customers.Customer, error) {
	f.creates++
	x := &customers.Customer{ID: 99, CompanyID: in.CompanyID, Code: in.Code, Name: in.Name}
	f.items[x.ID] = x
	f.byCode[in.Code] = x
	return x, nil
}
func (f *customerFake) Get(_ context.Context, id int64) (*customers.Customer, error) {
	x, ok := f.items[id]
	if !ok {
		return nil, errors.New("missing")
	}
	return x, nil
}
func (f *customerFake) GetByCode(_ context.Context, _ int64, code string) (*customers.Customer, error) {
	x, ok := f.byCode[code]
	if !ok {
		return nil, customers.ErrNotFound
	}
	return x, nil
}

type quotationFake struct{ creates int }

func (f *quotationFake) Create(_ context.Context, in quotations.CreateQuotationRequest, _ int64) (*quotations.Quotation, error) {
	f.creates++
	return &quotations.Quotation{ID: 77, CompanyID: in.CompanyID, CustomerID: in.CustomerID, Lines: make([]quotations.QuotationLine, len(in.Lines))}, nil
}

type notifierFake struct{ reminders []bool }

func (f *notifierFake) Reminder(_ context.Context, _ Activity, escalated bool) error {
	f.reminders = append(f.reminders, escalated)
	return nil
}
func (*notifierFake) Reassigned(context.Context, int64, string, int64) error { return nil }

func TestStageTransitionsOnlyMoveForwardAndRequireLossReason(t *testing.T) {
	f := newStoreFake()
	f.stages = []Stage{{ID: 1, Position: 1, Type: "OPEN"}, {ID: 2, Position: 2, Type: "OPEN"}, {ID: 3, Position: 3, Type: "WON"}, {ID: 4, Position: 4, Type: "LOST"}}
	f.opportunities[8] = Opportunity{ID: 8, CompanyID: 1, OwnerID: 2, StageID: 2, Status: "OPEN"}
	svc := NewService(f, nil, nil, nil)
	scope := Scope{CompanyID: 1, UserID: 2}
	if _, err := svc.Move(context.Background(), scope, 8, 1, ""); !errors.Is(err, ErrInvalidStage) {
		t.Fatalf("backward transition err=%v", err)
	}
	if _, err := svc.Move(context.Background(), scope, 8, 4, ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("loss without reason err=%v", err)
	}
	got, err := svc.Move(context.Background(), scope, 8, 3, "competitive win")
	if err != nil || got.Status != "WON" {
		t.Fatalf("win=%+v err=%v", got, err)
	}
	if _, err = svc.Move(context.Background(), scope, 8, 4, "later loss"); !errors.Is(err, ErrInvalidStage) {
		t.Fatalf("terminal transition err=%v", err)
	}
}

func TestQualifyPropagatesDuplicateContactDetection(t *testing.T) {
	f := newStoreFake()
	f.qualifyErr = ErrDuplicateContact
	_, err := NewService(f, nil, nil, nil).Qualify(context.Background(), Scope{CompanyID: 1, UserID: 2}, QualifyInput{LeadID: 1})
	if !errors.Is(err, ErrDuplicateContact) {
		t.Fatalf("err=%v", err)
	}
}

func TestWonConversionIsIdempotentAndLinksQuotation(t *testing.T) {
	f := newStoreFake()
	leadID := int64(4)
	f.leads[leadID] = Lead{ID: leadID, CompanyID: 1, OwnerID: 2, Name: "Ayu", Organization: "Acme", Email: "ayu@example.com"}
	f.opportunities[8] = Opportunity{ID: 8, CompanyID: 1, OwnerID: 2, LeadID: &leadID, Status: "WON"}
	customersFake := &customerFake{items: map[int64]*customers.Customer{}, byCode: map[string]*customers.Customer{}}
	quotes := &quotationFake{}
	svc := NewService(f, customersFake, quotes, nil)
	scope := Scope{CompanyID: 1, UserID: 2}
	input := ConvertInput{OpportunityID: 8, Lines: []quotations.CreateQuotationLineReq{{ProductID: 3, Quantity: 1, UOM: "EA", UnitPrice: 100}}}
	first, err := svc.Convert(context.Background(), scope, input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Convert(context.Background(), scope, input)
	if err != nil {
		t.Fatal(err)
	}
	if customersFake.creates != 1 || quotes.creates != 1 || first.CustomerID != 99 || first.QuotationID != 77 || !second.Existing || f.linkedQuotation != 77 {
		t.Fatalf("first=%+v second=%+v customer creates=%d quote creates=%d", first, second, customersFake.creates, quotes.creates)
	}
}

func TestReminderDispatchSeparatesDueAndEscalated(t *testing.T) {
	now := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	reminded := now.Add(-time.Hour)
	due := now.Add(-time.Minute)
	f := newStoreFake()
	f.due = []Activity{{ID: 1, OwnerID: 2, Subject: "Call"}, {ID: 2, OwnerID: 2, Subject: "Overdue", ReminderSentAt: &reminded, DueAt: &due}}
	notifier := &notifierFake{}
	svc := NewService(f, nil, nil, notifier)
	svc.now = func() time.Time { return now }
	if err := svc.DispatchReminders(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	if len(notifier.reminders) != 2 || notifier.reminders[0] || !notifier.reminders[1] {
		t.Fatalf("notifications=%v", notifier.reminders)
	}
	if len(f.reminders) != 2 || f.reminders[0] || !f.reminders[1] {
		t.Fatalf("marks=%v", f.reminders)
	}
}

func TestOwnerCannotCreateForAnotherOwner(t *testing.T) {
	f := newStoreFake()
	_, err := NewService(f, nil, nil, nil).CreateLead(context.Background(), Scope{CompanyID: 1, UserID: 2}, CreateLeadInput{Name: "Lead", OwnerID: 3})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("err=%v", err)
	}
}
