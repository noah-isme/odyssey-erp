package customers

import (
	"context"
	"errors"
	"fmt"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, req CreateCustomerRequest, createdBy int64) (*Customer, error) {
	// Check if code already exists
	existing, err := s.repo.GetByCode(ctx, req.CompanyID, req.Code)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("check existing customer: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("%w: customer code already exists", ErrAlreadyExists)
	}

	customer := Customer{
		Code:             req.Code,
		Name:             req.Name,
		CompanyID:        req.CompanyID,
		Email:            req.Email,
		Phone:            req.Phone,
		TaxID:            req.TaxID,
		CreditLimit:      req.CreditLimit,
		PaymentTermsDays: req.PaymentTermsDays,
		AddressLine1:     req.AddressLine1,
		AddressLine2:     req.AddressLine2,
		City:             req.City,
		State:            req.State,
		PostalCode:       req.PostalCode,
		Country:          req.Country,
		IsActive:         true,
		Notes:            req.Notes,
		CreatedBy:        createdBy,
	}

	var id int64
	err = s.repo.WithTx(ctx, func(ctx context.Context, repo Repository) error {
		var err error
		id, err = repo.Create(ctx, customer)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("create customer: %w", err)
	}

	customer.ID = id
	return &customer, nil
}

func (s *Service) Update(ctx context.Context, id int64, req UpdateCustomerRequest) (*Customer, error) {
	// Check if customer exists
	existing, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get customer: %w", err)
	}

	updates := make(map[string]interface{})
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Email != nil {
		updates["email"] = *req.Email
	}
	if req.Phone != nil {
		updates["phone"] = *req.Phone
	}
	if req.TaxID != nil {
		updates["tax_id"] = *req.TaxID
	}
	if req.CreditLimit != nil {
		updates["credit_limit"] = *req.CreditLimit
	}
	if req.PaymentTermsDays != nil {
		updates["payment_terms_days"] = *req.PaymentTermsDays
	}
	if req.AddressLine1 != nil {
		updates["address_line1"] = *req.AddressLine1
	}
	if req.AddressLine2 != nil {
		updates["address_line2"] = *req.AddressLine2
	}
	if req.City != nil {
		updates["city"] = *req.City
	}
	if req.State != nil {
		updates["state"] = *req.State
	}
	if req.PostalCode != nil {
		updates["postal_code"] = *req.PostalCode
	}
	if req.Country != nil {
		updates["country"] = *req.Country
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}
	if req.Notes != nil {
		updates["notes"] = *req.Notes
	}

	if len(updates) == 0 {
		return existing, nil
	}

	err = s.repo.WithTx(ctx, func(ctx context.Context, repo Repository) error {
		return repo.Update(ctx, id, updates)
	})
	if err != nil {
		return nil, fmt.Errorf("update customer: %w", err)
	}

	return s.repo.Get(ctx, id)
}

func (s *Service) Get(ctx context.Context, id int64) (*Customer, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) List(ctx context.Context, req ListCustomersRequest) ([]Customer, int, error) {
	return s.repo.List(ctx, req)
}

func (s *Service) GenerateCode(ctx context.Context, companyID int64) (string, error) {
	return s.repo.GenerateCode(ctx, companyID)
}
