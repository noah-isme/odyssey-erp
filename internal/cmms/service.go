package cmms

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Service orchestrates CMMS operations.
type Service struct {
	repo *Repository
	now  func() time.Time
}

// NewService constructs a Service instance.
func NewService(repo *Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// WithNow overrides the clock for deterministic tests.
func (s *Service) WithNow(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

// CreateWorkOrder inserts a new work order after validating inputs.
func (s *Service) CreateWorkOrder(ctx context.Context, req CreateWorkOrderRequest) (WorkOrder, error) {
	if err := req.Validate(); err != nil {
		return WorkOrder{}, err
	}

	// Generate work order number
	number, err := s.generateWorkOrderNumber(ctx, req.CompanyID)
	if err != nil {
		return WorkOrder{}, err
	}

	wo, err := s.repo.InsertWorkOrder(ctx, req, number)
	if err != nil {
		return WorkOrder{}, err
	}
	return wo, nil
}

func (s *Service) generateWorkOrderNumber(ctx context.Context, companyID int64) (string, error) {
	// Simple numbering: WO-YYYY-XXXXX
	year := time.Now().Year()
	prefix := fmt.Sprintf("WO-%d-", year)
	count, err := s.repo.CountWorkOrdersWithPrefix(ctx, companyID, prefix)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%05d", prefix, count+1), nil
}

// GetWorkOrder loads a single work order.
func (s *Service) GetWorkOrder(ctx context.Context, id int64) (WorkOrder, error) {
	return s.repo.GetWorkOrder(ctx, id)
}

// ListWorkOrders returns work orders filtered by various criteria.
func (s *Service) ListWorkOrders(ctx context.Context, filter ListWorkOrdersFilter) ([]WorkOrder, error) {
	return s.repo.ListWorkOrders(ctx, filter)
}

// UpdateWorkOrder updates a work order.
func (s *Service) UpdateWorkOrder(ctx context.Context, id int64, req UpdateWorkOrderRequest) (WorkOrder, error) {
	if err := req.Validate(); err != nil {
		return WorkOrder{}, err
	}
	return s.repo.UpdateWorkOrder(ctx, id, req)
}

// UpdateWorkOrderStatus transitions a work order's status.
func (s *Service) UpdateWorkOrderStatus(ctx context.Context, id int64, status Status, actorID int64) (WorkOrder, error) {
	if err := validateWorkOrderStatusTransition(status); err != nil {
		return WorkOrder{}, err
	}
	return s.repo.UpdateWorkOrderStatus(ctx, id, status, actorID)
}

// AddTask adds a task to a work order.
func (s *Service) AddTask(ctx context.Context, req CreateTaskRequest) (WorkOrderTask, error) {
	if err := req.Validate(); err != nil {
		return WorkOrderTask{}, err
	}
	return s.repo.InsertTask(ctx, req)
}

// UpdateTask updates a work order task.
func (s *Service) UpdateTask(ctx context.Context, id int64, req UpdateTaskRequest) (WorkOrderTask, error) {
	return s.repo.UpdateTask(ctx, id, req)
}

// CompleteTask marks a task as completed.
func (s *Service) CompleteTask(ctx context.Context, id int64, actorID int64, actualHours float64) (WorkOrderTask, error) {
	return s.repo.CompleteTask(ctx, id, actorID, actualHours)
}

// CreateAsset creates a new asset.
func (s *Service) CreateAsset(ctx context.Context, req CreateAssetRequest) (Asset, error) {
	if err := req.Validate(); err != nil {
		return Asset{}, err
	}
	return s.repo.InsertAsset(ctx, req)
}

// GetAsset loads a single asset.
func (s *Service) GetAsset(ctx context.Context, id int64) (Asset, error) {
	return s.repo.GetAsset(ctx, id)
}

// ListAssets returns assets filtered by criteria.
func (s *Service) ListAssets(ctx context.Context, filter ListAssetsFilter) ([]Asset, error) {
	return s.repo.ListAssets(ctx, filter)
}

// UpdateAsset updates an asset.
func (s *Service) UpdateAsset(ctx context.Context, id int64, req UpdateAssetRequest) (Asset, error) {
	return s.repo.UpdateAsset(ctx, id, req)
}

// CreateLocation creates a new location.
func (s *Service) CreateLocation(ctx context.Context, req CreateLocationRequest) (Location, error) {
	if err := req.Validate(); err != nil {
		return Location{}, err
	}
	return s.repo.InsertLocation(ctx, req)
}

// GetLocation loads a single location.
func (s *Service) GetLocation(ctx context.Context, id int64) (Location, error) {
	return s.repo.GetLocation(ctx, id)
}

// ListLocations returns locations for a company.
func (s *Service) ListLocations(ctx context.Context, companyID int64) ([]Location, error) {
	return s.repo.ListLocations(ctx, companyID)
}

// CreatePMSchedule creates a preventive maintenance schedule.
func (s *Service) CreatePMSchedule(ctx context.Context, req CreatePMScheduleRequest) (PreventiveMaintenanceSchedule, error) {
	if err := req.Validate(); err != nil {
		return PreventiveMaintenanceSchedule{}, err
	}
	return s.repo.InsertPMSchedule(ctx, req)
}

// GetPMSchedule loads a PM schedule.
func (s *Service) GetPMSchedule(ctx context.Context, id int64) (PreventiveMaintenanceSchedule, error) {
	return s.repo.GetPMSchedule(ctx, id)
}

// ListPMSchedules returns PM schedules for an asset.
func (s *Service) ListPMSchedules(ctx context.Context, assetID int64) ([]PreventiveMaintenanceSchedule, error) {
	return s.repo.ListPMSchedules(ctx, assetID)
}

// GenerateAllPMWorkOrders generates work orders from all due PM schedules across all companies.
func (s *Service) GenerateAllPMWorkOrders(ctx context.Context) ([]WorkOrder, error) {
	schedules, err := s.repo.ListAllDuePMSchedules(ctx)
	if err != nil {
		return nil, err
	}

	var workOrders []WorkOrder
	for _, sched := range schedules {
		req := CreateWorkOrderRequest{
			CompanyID:   sched.CompanyID,
			Title:       sched.Name,
			Description: sched.Description,
			AssetID:     &sched.AssetID,
			Priority:    PriorityMedium,
			Category:    "PREVENTIVE",
			RequesterID: 1, // system user placeholder
			ActorID:     1, // system user placeholder
		}

		// Calculate next due values
		nextDate := time.Time{}
		if sched.NextDueDate != nil && sched.FrequencyType != "" && sched.FrequencyValue > 0 {
			nextDate = *sched.NextDueDate
			switch sched.FrequencyType {
			case "DAILY":
				nextDate = nextDate.AddDate(0, 0, sched.FrequencyValue)
			case "WEEKLY":
				nextDate = nextDate.AddDate(0, 0, 7*sched.FrequencyValue)
			case "MONTHLY":
				nextDate = nextDate.AddDate(0, sched.FrequencyValue, 0)
			case "YEARLY":
				nextDate = nextDate.AddDate(sched.FrequencyValue, 0, 0)
			}
		}

		nextMeter := float64(0)
		if sched.NextDueMeter != nil && sched.FrequencyType == "METER" && sched.FrequencyValue > 0 {
			nextMeter = *sched.NextDueMeter + float64(sched.FrequencyValue)
		}

		number, err := s.generateWorkOrderNumber(ctx, sched.CompanyID)
		if err != nil {
			continue // skip on failure
		}

		wo, err := s.repo.GeneratePMWorkOrderTx(ctx, req, number, sched.ID, nextDate, nextMeter)
		if err != nil {
			continue // skip on failure
		}

		workOrders = append(workOrders, wo)
	}

	return workOrders, nil
}

// RecordMeterReading records a meter reading for an asset.
func (s *Service) RecordMeterReading(ctx context.Context, req CreateMeterReadingRequest) (MeterReading, error) {
	if err := req.Validate(); err != nil {
		return MeterReading{}, err
	}
	return s.repo.InsertMeterReading(ctx, req)
}

// GetMeterReadings returns meter readings for an asset.
func (s *Service) GetMeterReadings(ctx context.Context, assetID int64, readingType string, limit int) ([]MeterReading, error) {
	return s.repo.GetMeterReadings(ctx, assetID, readingType, limit)
}

// CreateSparePart creates a new spare part.
func (s *Service) CreateSparePart(ctx context.Context, req CreateSparePartRequest) (SparePart, error) {
	if err := req.Validate(); err != nil {
		return SparePart{}, err
	}
	return s.repo.InsertSparePart(ctx, req)
}

// GetSparePart loads a spare part.
func (s *Service) GetSparePart(ctx context.Context, id int64) (SparePart, error) {
	return s.repo.GetSparePart(ctx, id)
}

// ListSpareParts returns spare parts for a company.
func (s *Service) ListSpareParts(ctx context.Context, companyID int64) ([]SparePart, error) {
	return s.repo.ListSpareParts(ctx, companyID)
}

// AddSparePartToWorkOrder adds a spare part to a work order.
func (s *Service) AddSparePartToWorkOrder(ctx context.Context, req AddSparePartRequest) (WorkOrderSparePart, error) {
	if err := req.Validate(); err != nil {
		return WorkOrderSparePart{}, err
	}
	return s.repo.InsertWorkOrderSparePart(ctx, req)
}

// IssueSparePart issues a spare part from inventory.
func (s *Service) IssueSparePart(ctx context.Context, id int64, actorID int64) (WorkOrderSparePart, error) {
	return s.repo.IssueSparePart(ctx, id, actorID)
}

// UpdateWorkOrderRequest defines the payload for updating a work order.
type UpdateWorkOrderRequest struct {
	Title           string
	Description     string
	AssetID         *int64
	LocationID      *int64
	Priority        *Priority
	Category        string
	AssigneeID      *int64
	PlannedStart    *time.Time
	PlannedEnd      *time.Time
	EstimatedHours  float64
	ActorID         int64
}

func (r UpdateWorkOrderRequest) Validate() error {
	if r.ActorID <= 0 {
		return errors.New("cmms: actor id required")
	}
	return nil
}

// CreateTaskRequest defines the payload for creating a task.
type CreateTaskRequest struct {
	WorkOrderID     int64
	Sequence        int
	Title           string
	Description     string
	AssigneeID      *int64
	EstimatedHours  float64
	ActorID         int64
}

func (r CreateTaskRequest) Validate() error {
	if r.WorkOrderID <= 0 {
		return errors.New("cmms: work order id required")
	}
	if strings.TrimSpace(r.Title) == "" {
		return errors.New("cmms: title required")
	}
	if r.ActorID <= 0 {
		return errors.New("cmms: actor id required")
	}
	return nil
}

// UpdateTaskRequest defines the payload for updating a task.
type UpdateTaskRequest struct {
	Title          string
	Description    string
	AssigneeID     *int64
	EstimatedHours float64
	ActorID        int64
}

// UpdateAssetRequest defines the payload for updating an asset.
type UpdateAssetRequest struct {
	Name           string
	Description    string
	LocationID     *int64
	Manufacturer   string
	Model          string
	SerialNumber   string
	WarrantyExpiry *time.Time
	Status         string
	Criticality    string
	ActorID        int64
}

// CreateLocationRequest defines the payload for creating a location.
type CreateLocationRequest struct {
	CompanyID   int64
	Code        string
	Name        string
	Description string
	ParentID    *int64
	Address     string
	GPSLat      *float64
	GPSLng      *float64
	Active      bool
	ActorID     int64
}

func (r CreateLocationRequest) Validate() error {
	if r.CompanyID <= 0 {
		return errors.New("cmms: company id required")
	}
	if strings.TrimSpace(r.Code) == "" {
		return errors.New("cmms: location code required")
	}
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("cmms: location name required")
	}
	if r.ActorID <= 0 {
		return errors.New("cmms: actor id required")
	}
	return nil
}

// CreatePMScheduleRequest defines the payload for creating a PM schedule.
type CreatePMScheduleRequest struct {
	CompanyID        int64
	AssetID          int64
	Name             string
	Description      string
	FrequencyType    string
	FrequencyValue   int
	MeterReadingType string
	TaskTemplateID   *int64
	Active           bool
	ActorID          int64
}

func (r CreatePMScheduleRequest) Validate() error {
	if r.CompanyID <= 0 {
		return errors.New("cmms: company id required")
	}
	if r.AssetID <= 0 {
		return errors.New("cmms: asset id required")
	}
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("cmms: name required")
	}
	if r.FrequencyValue <= 0 {
		return errors.New("cmms: frequency value required")
	}
	if r.ActorID <= 0 {
		return errors.New("cmms: actor id required")
	}
	return nil
}

// CreateMeterReadingRequest defines the payload for creating a meter reading.
type CreateMeterReadingRequest struct {
	AssetID     int64
	ReadingType string
	Value       float64
	ReadingDate time.Time
	EnteredBy   int64
	Notes       string
	ActorID     int64
}

func (r CreateMeterReadingRequest) Validate() error {
	if r.AssetID <= 0 {
		return errors.New("cmms: asset id required")
	}
	if strings.TrimSpace(r.ReadingType) == "" {
		return errors.New("cmms: reading type required")
	}
	if r.EnteredBy <= 0 {
		return errors.New("cmms: entered by required")
	}
	if r.ActorID <= 0 {
		return errors.New("cmms: actor id required")
	}
	return nil
}

// CreateSparePartRequest defines the payload for creating a spare part.
type CreateSparePartRequest struct {
	CompanyID      int64
	Code           string
	Name           string
	Description    string
	Category       string
	UnitOfMeasure  string
	MinQuantity    float64
	MaxQuantity    float64
	ReorderPoint   float64
	LeadTimeDays   int
	UnitCost       float64
	CriticalSpare  bool
	ActorID        int64
}

func (r CreateSparePartRequest) Validate() error {
	if r.CompanyID <= 0 {
		return errors.New("cmms: company id required")
	}
	if strings.TrimSpace(r.Code) == "" {
		return errors.New("cmms: code required")
	}
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("cmms: name required")
	}
	if r.ActorID <= 0 {
		return errors.New("cmms: actor id required")
	}
	return nil
}

// AddSparePartRequest defines the payload for adding a spare part to a work order.
type AddSparePartRequest struct {
	WorkOrderID int64
	SparePartID int64
	Quantity    float64
	UnitCost    float64
	ActorID     int64
}

func (r AddSparePartRequest) Validate() error {
	if r.WorkOrderID <= 0 {
		return errors.New("cmms: work order id required")
	}
	if r.SparePartID <= 0 {
		return errors.New("cmms: spare part id required")
	}
	if r.Quantity <= 0 {
		return errors.New("cmms: quantity required")
	}
	if r.ActorID <= 0 {
		return errors.New("cmms: actor id required")
	}
	return nil
}

func validateWorkOrderStatusTransition(newStatus Status) error {
	validStatuses := []Status{
		WorkOrderStatusDraft, WorkOrderStatusPlanned, WorkOrderStatusScheduled,
		WorkOrderStatusInProgress, WorkOrderStatusOnHold, WorkOrderStatusCompleted,
		WorkOrderStatusCancelled, WorkOrderStatusClosed,
	}
	for _, s := range validStatuses {
		if s == newStatus {
			return nil
		}
	}
	return fmt.Errorf("cmms: invalid status %s", newStatus)
}