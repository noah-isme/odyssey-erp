package mrp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrNotFound               = errors.New("mrp: not found")
	ErrInvalidState           = errors.New("mrp: invalid state")
	ErrIdempotencyKeyRequired = errors.New("mrp: idempotency key required")
)

type BOMLine struct {
	ProductID          int64
	Quantity, ScrapPct float64
}
type BOM struct {
	ID, CompanyID, ProductID, CreatedBy   int64
	ApprovedBy                            *int64
	Version, RevisionStatus, ChangeReason string
	Lines                                 []BOMLine
	Active                                bool
	ScrapPct                              float64
	EffectiveFrom, EffectiveTo            time.Time
	ApprovedAt                            *time.Time
}

const (
	BOMRevisionDraft      = "DRAFT"
	BOMRevisionApproved   = "APPROVED"
	BOMRevisionSuperseded = "SUPERSEDED"
)

func (b BOM) IsApprovedEffective(on time.Time) bool {
	if b.RevisionStatus != BOMRevisionApproved || !b.Active || b.EffectiveFrom.IsZero() {
		return false
	}
	on = planningDay(on)
	return !on.Before(planningDay(b.EffectiveFrom)) && (b.EffectiveTo.IsZero() || !on.After(planningDay(b.EffectiveTo)))
}

type BOMRevisionInput struct {
	Version       string    `json:"version"`
	EffectiveFrom time.Time `json:"effective_from"`
	ChangeReason  string    `json:"change_reason"`
}
type WorkOrder struct {
	ID, CompanyID, ProductID, BOMID, WarehouseID, CreatedBy int64
	PlannedQty, CompletedQty                                float64
	Status                                                  string
}

type WorkOrderOperation struct {
	ID, CompanyID, WorkOrderID, RoutingOperationID, WorkCenterID int64
	Sequence                                                     int
	Code, Name, Status                                           string
	PlannedSetupMinutes, PlannedRunMinutes                       float64
	ActualSetupMinutes, ActualRunMinutes                         float64
	GoodQuantity, ScrapQuantity                                  float64
	OperatorID                                                   *int64
	ScheduledStart, ScheduledEnd                                 time.Time
	ScheduleManual                                               bool
}

type WIPLocation struct {
	ID, CompanyID, WarehouseID, WIPWarehouseID int64
	WorkCenterID                               *int64
	Name                                       string
	Active                                     bool
	CreatedBy                                  int64
}

type WorkCenter struct {
	ID, CompanyID, CreatedBy int64
	Code, Name               string
	CapacityHoursPerDay      float64
	Active                   bool
}

type RoutingOperation struct {
	ID, RoutingID, WorkCenterID int64
	Sequence                    int
	Code, Name                  string
	SetupMinutes, RunMinutes    float64
	YieldPct                    float64
}

type Routing struct {
	ID, CompanyID, ProductID, CreatedBy int64
	Code, Version                       string
	Active                              bool
	Operations                          []RoutingOperation
}

type Repository interface {
	CreateBOM(context.Context, BOM) (BOM, error)
	CreateBOMRevision(context.Context, int64, int64, int64, BOMRevisionInput) (BOM, error)
	ApproveBOM(context.Context, int64, int64, int64, string) (BOM, error)
	ListBOMRevisions(context.Context, int64, int64) ([]BOM, error)
	CreateWorkOrder(context.Context, WorkOrder) (WorkOrder, error)
	GetBOM(context.Context, int64, int64) (BOM, error)
	GetWorkOrder(context.Context, int64, int64) (WorkOrder, error)
	UpdateWorkOrder(context.Context, WorkOrder) error
	GenerateWorkOrderOperations(context.Context, WorkOrder) error
	CreateWIPLocation(context.Context, WIPLocation) (WIPLocation, error)
	ListWIPLocations(context.Context, int64) ([]WIPLocation, error)
	DeactivateWIPLocation(context.Context, int64, int64) error
	ResolveWIPLocation(context.Context, int64, int64, int64) (WIPLocation, error)
	CreateWorkCenter(context.Context, WorkCenter) (WorkCenter, error)
	CreateRouting(context.Context, Routing) (Routing, error)
}

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func (s *Service) GetWorkOrder(ctx context.Context, companyID, orderID int64) (WorkOrder, error) {
	return s.repo.GetWorkOrder(ctx, companyID, orderID)
}

func (s *Service) CreateBOM(ctx context.Context, bom BOM) (BOM, error) {
	if bom.CompanyID == 0 || bom.ProductID == 0 || bom.CreatedBy == 0 || strings.TrimSpace(bom.Version) == "" || len(bom.Lines) == 0 || bom.ScrapPct < 0 || bom.ScrapPct > 100 {
		return BOM{}, ErrInvalidState
	}
	for _, line := range bom.Lines {
		if line.ProductID == 0 || line.ProductID == bom.ProductID || line.Quantity <= 0 || line.ScrapPct < 0 || line.ScrapPct > 100 {
			return BOM{}, ErrInvalidState
		}
	}
	bom.Active = true
	bom.RevisionStatus = BOMRevisionDraft
	if bom.EffectiveFrom.IsZero() {
		bom.EffectiveFrom = planningDay(time.Now())
	}
	return s.repo.CreateBOM(ctx, bom)
}

func (s *Service) CreateBOMRevision(ctx context.Context, companyID, sourceBOMID, actorID int64, in BOMRevisionInput) (BOM, error) {
	if companyID <= 0 || sourceBOMID <= 0 || actorID <= 0 || strings.TrimSpace(in.Version) == "" || in.EffectiveFrom.IsZero() || strings.TrimSpace(in.ChangeReason) == "" {
		return BOM{}, ErrInvalidState
	}
	return s.repo.CreateBOMRevision(ctx, companyID, sourceBOMID, actorID, in)
}

func (s *Service) ApproveBOM(ctx context.Context, companyID, bomID, actorID int64, reason string) (BOM, error) {
	if companyID <= 0 || bomID <= 0 || actorID <= 0 || strings.TrimSpace(reason) == "" {
		return BOM{}, ErrInvalidState
	}
	return s.repo.ApproveBOM(ctx, companyID, bomID, actorID, reason)
}

func (s *Service) ListBOMRevisions(ctx context.Context, companyID, productID int64) ([]BOM, error) {
	if companyID <= 0 || productID <= 0 {
		return nil, ErrInvalidState
	}
	return s.repo.ListBOMRevisions(ctx, companyID, productID)
}

func (s *Service) CreateWIPLocation(ctx context.Context, location WIPLocation) (WIPLocation, error) {
	if location.CompanyID <= 0 || location.WarehouseID <= 0 || location.WIPWarehouseID <= 0 || location.WarehouseID == location.WIPWarehouseID || location.CreatedBy <= 0 || strings.TrimSpace(location.Name) == "" {
		return WIPLocation{}, ErrInvalidState
	}
	location.Active = true
	return s.repo.CreateWIPLocation(ctx, location)
}
func (s *Service) ListWIPLocations(ctx context.Context, companyID int64) ([]WIPLocation, error) {
	if companyID <= 0 {
		return nil, ErrInvalidState
	}
	return s.repo.ListWIPLocations(ctx, companyID)
}
func (s *Service) DeactivateWIPLocation(ctx context.Context, companyID, id int64) error {
	if companyID <= 0 || id <= 0 {
		return ErrInvalidState
	}
	return s.repo.DeactivateWIPLocation(ctx, companyID, id)
}

func (s *Service) CreateWorkOrder(ctx context.Context, order WorkOrder) (WorkOrder, error) {
	if order.CompanyID == 0 || order.ProductID == 0 || order.BOMID == 0 || order.WarehouseID == 0 || order.CreatedBy == 0 || order.PlannedQty <= 0 {
		return WorkOrder{}, ErrInvalidState
	}
	bom, err := s.repo.GetBOM(ctx, order.CompanyID, order.BOMID)
	if err != nil {
		return WorkOrder{}, err
	}
	if bom.ProductID != order.ProductID || !bom.IsApprovedEffective(time.Now()) {
		return WorkOrder{}, ErrInvalidState
	}
	order.Status = "DRAFT"
	return s.repo.CreateWorkOrder(ctx, order)
}

func (s *Service) CreateWorkCenter(ctx context.Context, center WorkCenter) (WorkCenter, error) {
	if center.CompanyID <= 0 || center.CreatedBy <= 0 || strings.TrimSpace(center.Code) == "" || strings.TrimSpace(center.Name) == "" || center.CapacityHoursPerDay <= 0 {
		return WorkCenter{}, ErrInvalidState
	}
	center.Active = true
	return s.repo.CreateWorkCenter(ctx, center)
}

func (s *Service) CreateRouting(ctx context.Context, routing Routing) (Routing, error) {
	if routing.CompanyID <= 0 || routing.ProductID <= 0 || routing.CreatedBy <= 0 || strings.TrimSpace(routing.Code) == "" || strings.TrimSpace(routing.Version) == "" || len(routing.Operations) == 0 {
		return Routing{}, ErrInvalidState
	}
	lastSequence := 0
	for _, operation := range routing.Operations {
		if operation.WorkCenterID <= 0 || operation.Sequence <= lastSequence || strings.TrimSpace(operation.Code) == "" || strings.TrimSpace(operation.Name) == "" || operation.SetupMinutes < 0 || operation.RunMinutes < 0 || operation.YieldPct <= 0 || operation.YieldPct > 100 {
			return Routing{}, ErrInvalidState
		}
		lastSequence = operation.Sequence
	}
	routing.Active = true
	return s.repo.CreateRouting(ctx, routing)
}

func (s *Service) Release(ctx context.Context, companyID, orderID int64) (WorkOrder, error) {
	order, err := s.repo.GetWorkOrder(ctx, companyID, orderID)
	if err != nil {
		return WorkOrder{}, err
	}
	if order.Status != "DRAFT" || order.BOMID == 0 {
		return WorkOrder{}, fmt.Errorf("%w: work order must have a BOM and be draft", ErrInvalidState)
	}
	bom, err := s.repo.GetBOM(ctx, companyID, order.BOMID)
	if err != nil {
		return WorkOrder{}, err
	}
	if bom.ProductID != order.ProductID || !bom.IsApprovedEffective(time.Now()) {
		return WorkOrder{}, ErrInvalidState
	}
	if err := s.repo.GenerateWorkOrderOperations(ctx, order); err != nil {
		return WorkOrder{}, err
	}
	order.Status = "RELEASED"
	if err := s.repo.UpdateWorkOrder(ctx, order); err != nil {
		return WorkOrder{}, err
	}
	return order, nil
}

func validateCompletionInput(input CompletionInput) error {
	if input.CompanyID <= 0 || input.ActorID <= 0 || input.WorkOrderID <= 0 || input.Quantity <= 0 {
		return ErrInvalidState
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" {
		return ErrIdempotencyKeyRequired
	}
	return nil
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
