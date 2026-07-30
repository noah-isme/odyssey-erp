package crm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/sales/customers"
	"github.com/odyssey-erp/odyssey-erp/internal/sales/quotations"
)

type Service struct {
	store      Store
	customers  CustomerGateway
	quotations QuotationGateway
	notifier   Notifier
	now        func() time.Time
}

func NewService(store Store, customers CustomerGateway, quotations QuotationGateway, notifier Notifier) *Service {
	return &Service{store: store, customers: customers, quotations: quotations, notifier: notifier, now: time.Now}
}
func validScope(s Scope) bool { return s.CompanyID > 0 && s.UserID > 0 }
func (s *Service) Pipeline(ctx context.Context, scope Scope) (Pipeline, error) {
	if !validScope(scope) {
		return Pipeline{}, ErrInvalidInput
	}
	return s.store.Pipeline(ctx, scope)
}
func (s *Service) Stages(ctx context.Context, scope Scope) ([]Stage, error) {
	if !validScope(scope) {
		return nil, ErrInvalidInput
	}
	return s.store.Stages(ctx, scope)
}
func (s *Service) CreateLead(ctx context.Context, scope Scope, in CreateLeadInput) (Lead, error) {
	if !validScope(scope) || strings.TrimSpace(in.Name) == "" {
		return Lead{}, ErrInvalidInput
	}
	in.CompanyID = scope.CompanyID
	in.CreatedBy = scope.UserID
	if in.OwnerID == 0 {
		in.OwnerID = scope.UserID
	}
	if !scope.ViewAll && in.OwnerID != scope.UserID {
		return Lead{}, ErrForbidden
	}
	if in.Source == "" {
		in.Source = "OTHER"
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	return s.store.CreateLead(ctx, in)
}
func (s *Service) Lead(ctx context.Context, scope Scope, id int64) (Lead, []Activity, []Event, error) {
	if !validScope(scope) || id <= 0 {
		return Lead{}, nil, nil, ErrInvalidInput
	}
	lead, err := s.store.Lead(ctx, scope, id)
	if err != nil {
		return Lead{}, nil, nil, err
	}
	activities, events, err := s.store.Timeline(ctx, scope, "LEAD", id)
	return lead, activities, events, err
}
func (s *Service) Qualify(ctx context.Context, scope Scope, in QualifyInput) (Opportunity, error) {
	if !validScope(scope) || in.LeadID <= 0 || in.ExpectedValue < 0 {
		return Opportunity{}, ErrInvalidInput
	}
	in.ActorID = scope.UserID
	return s.store.Qualify(ctx, scope, in)
}
func (s *Service) Opportunity(ctx context.Context, scope Scope, id int64) (Opportunity, []Activity, []Event, error) {
	if !validScope(scope) || id <= 0 {
		return Opportunity{}, nil, nil, ErrInvalidInput
	}
	opp, err := s.store.Opportunity(ctx, scope, id)
	if err != nil {
		return Opportunity{}, nil, nil, err
	}
	activities, events, err := s.store.Timeline(ctx, scope, "OPPORTUNITY", id)
	return opp, activities, events, err
}
func (s *Service) Move(ctx context.Context, scope Scope, id, stageID int64, reason string) (Opportunity, error) {
	if !validScope(scope) || id <= 0 || stageID <= 0 {
		return Opportunity{}, ErrInvalidInput
	}
	opp, err := s.store.Opportunity(ctx, scope, id)
	if err != nil {
		return Opportunity{}, err
	}
	if opp.Status != "OPEN" {
		return Opportunity{}, ErrInvalidStage
	}
	stages, err := s.store.Stages(ctx, scope)
	if err != nil {
		return Opportunity{}, err
	}
	var current, target Stage
	for _, stage := range stages {
		if stage.ID == opp.StageID {
			current = stage
		}
		if stage.ID == stageID {
			target = stage
		}
	}
	if target.ID == 0 || current.ID == 0 {
		return Opportunity{}, ErrInvalidStage
	}
	if target.Type == "OPEN" && target.Position <= current.Position {
		return Opportunity{}, ErrInvalidStage
	}
	if target.Type == "LOST" && strings.TrimSpace(reason) == "" {
		return Opportunity{}, ErrInvalidInput
	}
	return s.store.Move(ctx, scope, id, target, strings.TrimSpace(reason), scope.UserID)
}
func (s *Service) AddActivity(ctx context.Context, scope Scope, in ActivityInput) (Activity, error) {
	if !validScope(scope) || strings.TrimSpace(in.Subject) == "" || in.Type == "" || (in.LeadID == nil && in.OpportunityID == nil) {
		return Activity{}, ErrInvalidInput
	}
	if in.ReminderAt != nil && in.DueAt != nil && in.ReminderAt.After(*in.DueAt) {
		return Activity{}, ErrInvalidInput
	}
	in.CompanyID = scope.CompanyID
	in.CreatedBy = scope.UserID
	if in.OwnerID == 0 {
		in.OwnerID = scope.UserID
	}
	if !scope.ViewAll && in.OwnerID != scope.UserID {
		return Activity{}, ErrForbidden
	}
	if in.OpportunityID != nil {
		if _, err := s.store.Opportunity(ctx, scope, *in.OpportunityID); err != nil {
			return Activity{}, err
		}
	}
	if in.LeadID != nil {
		if _, err := s.store.Lead(ctx, scope, *in.LeadID); err != nil {
			return Activity{}, err
		}
	}
	return s.store.AddActivity(ctx, scope, in)
}
func (s *Service) CompleteActivity(ctx context.Context, scope Scope, id int64) error {
	if !validScope(scope) || id <= 0 {
		return ErrInvalidInput
	}
	return s.store.CompleteActivity(ctx, scope, id, scope.UserID, s.now())
}
func (s *Service) Reassign(ctx context.Context, scope Scope, entity string, id, ownerID int64) error {
	if !validScope(scope) || !scope.ViewAll || id <= 0 || ownerID <= 0 {
		return ErrForbidden
	}
	if err := s.store.Reassign(ctx, scope, entity, id, ownerID, scope.UserID); err != nil {
		return err
	}
	if s.notifier != nil {
		return s.notifier.Reassigned(ctx, ownerID, entity, id)
	}
	return nil
}

func (s *Service) Convert(ctx context.Context, scope Scope, in ConvertInput) (Conversion, error) {
	if !validScope(scope) || in.OpportunityID <= 0 || s.customers == nil || s.quotations == nil {
		return Conversion{}, ErrInvalidInput
	}
	opp, err := s.store.Opportunity(ctx, scope, in.OpportunityID)
	if err != nil {
		return Conversion{}, err
	}
	if opp.Status != "WON" {
		return Conversion{}, ErrNotWon
	}
	if opp.CustomerID != nil && opp.QuotationID != nil {
		return Conversion{CustomerID: *opp.CustomerID, QuotationID: *opp.QuotationID, Existing: true}, nil
	}
	customerID := in.ExistingCustomerID
	if opp.CustomerID != nil {
		customerID = *opp.CustomerID
	}
	if customerID == 0 && opp.ContactID != nil {
		lead, leadErr := s.store.Lead(ctx, scope, valueOrZero(opp.LeadID))
		if leadErr == nil && lead.Email != "" {
			customerID, err = s.store.CustomerByEmail(ctx, scope.CompanyID, lead.Email)
			if err != nil && !errors.Is(err, ErrNotFound) {
				return Conversion{}, err
			}
		}
	}
	if customerID != 0 {
		customer, lookupErr := s.customers.Get(ctx, customerID)
		if lookupErr != nil || customer.CompanyID != scope.CompanyID {
			return Conversion{}, ErrInvalidInput
		}
	}
	if customerID == 0 {
		lead, leadErr := s.store.Lead(ctx, scope, valueOrZero(opp.LeadID))
		if leadErr != nil {
			return Conversion{}, leadErr
		}
		code := fmt.Sprintf("CRM-L%06d", lead.ID)
		if existing, getErr := s.customers.GetByCode(ctx, scope.CompanyID, code); getErr == nil {
			customerID = existing.ID
		} else if errors.Is(getErr, customers.ErrNotFound) {
			name := strings.TrimSpace(in.CustomerName)
			if name == "" {
				name = lead.Organization
			}
			if name == "" {
				name = lead.Name
			}
			country := in.Country
			if country == "" {
				country = "ID"
			}
			email := lead.Email
			phone := lead.Phone
			created, createErr := s.customers.Create(ctx, customers.CreateCustomerRequest{Code: code, Name: name, CompanyID: scope.CompanyID, Email: stringPtr(email), Phone: stringPtr(phone), Country: country}, scope.UserID)
			if createErr != nil {
				return Conversion{}, createErr
			}
			customerID = created.ID
		} else {
			return Conversion{}, getErr
		}
	}
	if err = s.store.LinkCustomer(ctx, scope, opp.ID, customerID, scope.UserID); err != nil {
		return Conversion{}, err
	}
	if opp.QuotationID != nil {
		return Conversion{CustomerID: customerID, QuotationID: *opp.QuotationID, Existing: true}, nil
	}
	if len(in.Lines) == 0 {
		return Conversion{CustomerID: customerID}, nil
	}
	quoteDate := in.QuoteDate
	if quoteDate.IsZero() {
		quoteDate = s.now()
	}
	validUntil := in.ValidUntil
	if validUntil.IsZero() {
		validUntil = quoteDate.AddDate(0, 0, 30)
	}
	currency := in.Currency
	if currency == "" {
		currency = "IDR"
	}
	note := fmt.Sprintf("CRM opportunity #%d", opp.ID)
	quote, err := s.quotations.Create(ctx, quotations.CreateQuotationRequest{CompanyID: scope.CompanyID, CustomerID: customerID, QuoteDate: quoteDate, ValidUntil: validUntil, Currency: currency, Notes: &note, Lines: in.Lines}, scope.UserID)
	if err != nil {
		return Conversion{CustomerID: customerID}, err
	}
	if err = s.store.LinkQuotation(ctx, scope, opp.ID, quote.ID, scope.UserID); err != nil {
		return Conversion{CustomerID: customerID}, err
	}
	return Conversion{CustomerID: customerID, QuotationID: quote.ID}, nil
}
func valueOrZero(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}
func stringPtr(v string) *string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return &v
}
func (s *Service) WinLoss(ctx context.Context, scope Scope) (WinLoss, error) {
	if !validScope(scope) {
		return WinLoss{}, ErrInvalidInput
	}
	return s.store.WinLoss(ctx, scope)
}

func (s *Service) DispatchReminders(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	now := s.now()
	items, err := s.store.DueActivities(ctx, now, limit)
	if err != nil {
		return err
	}
	var failures []error
	for _, activity := range items {
		escalated := activity.DueAt != nil && activity.DueAt.Before(now)
		if s.notifier != nil {
			err = s.notifier.Reminder(ctx, activity, escalated)
		}
		if err == nil {
			err = s.store.MarkReminder(ctx, activity.ID, escalated, now)
		}
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
