package mrp

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrNotFound     = errors.New("mrp: not found")
	ErrInvalidState = errors.New("mrp: invalid state")
)

type BOMLine struct {
	ProductID          int64
	Quantity, ScrapPct float64
}
type BOM struct {
	ID, CompanyID, ProductID, CreatedBy int64
	Version                             string
	Lines                               []BOMLine
	Active                              bool
}
type WorkOrder struct {
	ID, CompanyID, ProductID, BOMID, WarehouseID, CreatedBy int64
	PlannedQty, CompletedQty                                float64
	Status                                                  string
}

type Repository interface {
	CreateBOM(context.Context, BOM) (BOM, error)
	CreateWorkOrder(context.Context, WorkOrder) (WorkOrder, error)
	GetBOM(context.Context, int64, int64) (BOM, error)
	GetWorkOrder(context.Context, int64, int64) (WorkOrder, error)
	UpdateWorkOrder(context.Context, WorkOrder) error
}

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func (s *Service) CreateBOM(ctx context.Context, bom BOM) (BOM, error) {
	if bom.CompanyID == 0 || bom.ProductID == 0 || bom.CreatedBy == 0 || bom.Version == "" || len(bom.Lines) == 0 {
		return BOM{}, ErrInvalidState
	}
	for _, line := range bom.Lines {
		if line.ProductID == 0 || line.Quantity <= 0 || line.ScrapPct < 0 || line.ScrapPct > 100 {
			return BOM{}, ErrInvalidState
		}
	}
	bom.Active = true
	return s.repo.CreateBOM(ctx, bom)
}

func (s *Service) CreateWorkOrder(ctx context.Context, order WorkOrder) (WorkOrder, error) {
	if order.CompanyID == 0 || order.ProductID == 0 || order.BOMID == 0 || order.CreatedBy == 0 || order.PlannedQty <= 0 {
		return WorkOrder{}, ErrInvalidState
	}
	if _, err := s.repo.GetBOM(ctx, order.CompanyID, order.BOMID); err != nil {
		return WorkOrder{}, err
	}
	order.Status = "DRAFT"
	return s.repo.CreateWorkOrder(ctx, order)
}

func (s *Service) Release(ctx context.Context, companyID, orderID int64) (WorkOrder, error) {
	order, err := s.repo.GetWorkOrder(ctx, companyID, orderID)
	if err != nil {
		return WorkOrder{}, err
	}
	if order.Status != "DRAFT" || order.BOMID == 0 {
		return WorkOrder{}, fmt.Errorf("%w: work order must have a BOM and be draft", ErrInvalidState)
	}
	if _, err := s.repo.GetBOM(ctx, companyID, order.BOMID); err != nil {
		return WorkOrder{}, err
	}
	order.Status = "RELEASED"
	if err := s.repo.UpdateWorkOrder(ctx, order); err != nil {
		return WorkOrder{}, err
	}
	return order, nil
}

func (s *Service) Start(ctx context.Context, companyID, orderID int64) (WorkOrder, error) {
	return s.transition(ctx, companyID, orderID, "RELEASED", "IN_PROGRESS")
}
func (s *Service) Complete(ctx context.Context, companyID, orderID int64, quantity float64) (WorkOrder, error) {
	order, err := s.repo.GetWorkOrder(ctx, companyID, orderID)
	if err != nil {
		return WorkOrder{}, err
	}
	if order.Status != "IN_PROGRESS" || quantity <= 0 || order.CompletedQty+quantity > order.PlannedQty {
		return WorkOrder{}, ErrInvalidState
	}
	order.CompletedQty += quantity
	if order.CompletedQty == order.PlannedQty {
		order.Status = "COMPLETED"
	}
	if err := s.repo.UpdateWorkOrder(ctx, order); err != nil {
		return WorkOrder{}, err
	}
	return order, nil
}
func (s *Service) transition(ctx context.Context, companyID, orderID int64, from, to string) (WorkOrder, error) {
	order, err := s.repo.GetWorkOrder(ctx, companyID, orderID)
	if err != nil {
		return WorkOrder{}, err
	}
	if order.Status != from {
		return WorkOrder{}, ErrInvalidState
	}
	order.Status = to
	if err := s.repo.UpdateWorkOrder(ctx, order); err != nil {
		return WorkOrder{}, err
	}
	return order, nil
}
