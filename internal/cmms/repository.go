package cmms

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// Repository persists CMMS entities.
type Repository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

// NewRepository constructs a repository wrapper.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool:    pool,
		queries: sqlc.New(pool),
	}
}

// =============================================================================
// Work Orders
// =============================================================================

func (r *Repository) InsertWorkOrder(ctx context.Context, req CreateWorkOrderRequest, number string) (WorkOrder, error) {
	id, err := r.queries.InsertWorkOrder(ctx, sqlc.InsertWorkOrderParams{
		CompanyID:      req.CompanyID,
		Number:         number,
		Title:          req.Title,
		Description:    pgtype.Text{String: req.Description, Valid: req.Description != ""},
		AssetID:        pgtype.Int8{Int64: valueOrZero(req.AssetID), Valid: req.AssetID != nil},
		LocationID:     pgtype.Int8{Int64: valueOrZero(req.LocationID), Valid: req.LocationID != nil},
		Priority:       string(req.Priority),
		Status:         string(WorkOrderStatusDraft),
		Category:       req.Category,
		RequesterID:    req.RequesterID,
		AssigneeID:     pgtype.Int8{Int64: valueOrZero(req.AssigneeID), Valid: req.AssigneeID != nil},
		PlannedStart:   pgtype.Timestamptz{Time: valueOrZeroTime(req.PlannedStart), Valid: req.PlannedStart != nil},
		PlannedEnd:     pgtype.Timestamptz{Time: valueOrZeroTime(req.PlannedEnd), Valid: req.PlannedEnd != nil},
		EstimatedHours: req.EstimatedHours,
		CreatedBy:      req.ActorID,
	})
	if err != nil {
		return WorkOrder{}, err
	}
	return r.GetWorkOrder(ctx, id)
}

func (r *Repository) GetWorkOrder(ctx context.Context, id int64) (WorkOrder, error) {
	row, err := r.queries.GetWorkOrder(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WorkOrder{}, ErrWorkOrderNotFound
		}
		return WorkOrder{}, err
	}
	return toWorkOrder(row), nil
}

func (r *Repository) ListWorkOrders(ctx context.Context, filter ListWorkOrdersFilter) ([]WorkOrder, error) {
	rows, err := r.queries.ListWorkOrders(ctx, sqlc.ListWorkOrdersParams{
		CompanyID:   filter.CompanyID,
		Column2:     valueOrZero(filter.AssetID),
		Column3:     valueOrZero(filter.LocationID),
		Column4:     valueOrZero(filter.AssigneeID),
		Column5:     statusStringOrEmpty(filter.Status),
		Column6:     priorityStringOrEmpty(filter.Priority),
		Column7:     filter.Category,
		Column8:     pgtype.Timestamptz{Time: valueOrZeroTime(filter.DateFrom), Valid: filter.DateFrom != nil},
		Column9:     pgtype.Timestamptz{Time: valueOrZeroTime(filter.DateTo), Valid: filter.DateTo != nil},
		Limit:       int32(filter.Limit),
		Offset:      int32(filter.Offset),
	})
	if err != nil {
		return nil, err
	}
	items := make([]WorkOrder, len(rows))
	for i, row := range rows {
		items[i] = toWorkOrderFromListRow(row)
	}
	return items, nil
}

func (r *Repository) UpdateWorkOrder(ctx context.Context, id int64, req UpdateWorkOrderRequest) (WorkOrder, error) {
	err := r.queries.UpdateWorkOrder(ctx, sqlc.UpdateWorkOrderParams{
		ID:             id,
		Title:          req.Title,
		Description:    pgtype.Text{String: req.Description, Valid: req.Description != ""},
		AssetID:        pgtype.Int8{Int64: valueOrZero(req.AssetID), Valid: req.AssetID != nil},
		LocationID:     pgtype.Int8{Int64: valueOrZero(req.LocationID), Valid: req.LocationID != nil},
		Priority:       priorityStringOrEmpty(req.Priority),
		Category:       req.Category,
		AssigneeID:     pgtype.Int8{Int64: valueOrZero(req.AssigneeID), Valid: req.AssigneeID != nil},
		PlannedStart:   pgtype.Timestamptz{Time: valueOrZeroTime(req.PlannedStart), Valid: req.PlannedStart != nil},
		PlannedEnd:     pgtype.Timestamptz{Time: valueOrZeroTime(req.PlannedEnd), Valid: req.PlannedEnd != nil},
		EstimatedHours: req.EstimatedHours,
	})
	if err != nil {
		return WorkOrder{}, err
	}
	return r.GetWorkOrder(ctx, id)
}

func (r *Repository) UpdateWorkOrderStatus(ctx context.Context, id int64, status Status, actorID int64) (WorkOrder, error) {
	err := r.queries.UpdateWorkOrderStatus(ctx, sqlc.UpdateWorkOrderStatusParams{
		ID:     id,
		Status: string(status),
	})
	if err != nil {
		return WorkOrder{}, err
	}
	return r.GetWorkOrder(ctx, id)
}

func (r *Repository) CountWorkOrdersWithPrefix(ctx context.Context, companyID int64, prefix string) (int64, error) {
	return r.queries.CountWorkOrdersWithPrefix(ctx, sqlc.CountWorkOrdersWithPrefixParams{
		CompanyID: companyID,
		Column2:   pgtype.Text{String: prefix, Valid: prefix != ""},
	})
}

// =============================================================================
// Work Order Tasks
// =============================================================================

func (r *Repository) InsertTask(ctx context.Context, req CreateTaskRequest) (WorkOrderTask, error) {
	id, err := r.queries.InsertWorkOrderTask(ctx, sqlc.InsertWorkOrderTaskParams{
		WorkOrderID:     req.WorkOrderID,
		Sequence:        int32(req.Sequence),
		Title:           req.Title,
		Description:     pgtype.Text{String: req.Description, Valid: req.Description != ""},
		Status:          string(WorkOrderStatusDraft),
		AssigneeID:      pgtype.Int8{Int64: valueOrZero(req.AssigneeID), Valid: req.AssigneeID != nil},
		EstimatedHours:  req.EstimatedHours,
	})
	if err != nil {
		return WorkOrderTask{}, err
	}
	return r.GetWorkOrderTask(ctx, id)
}

func (r *Repository) GetWorkOrderTask(ctx context.Context, id int64) (WorkOrderTask, error) {
	row, err := r.queries.GetWorkOrderTask(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WorkOrderTask{}, errors.New("cmms: task not found")
		}
		return WorkOrderTask{}, err
	}
	return toWorkOrderTask(row), nil
}

func (r *Repository) ListWorkOrderTasks(ctx context.Context, workOrderID int64) ([]WorkOrderTask, error) {
	rows, err := r.queries.ListWorkOrderTasks(ctx, workOrderID)
	if err != nil {
		return nil, err
	}
	items := make([]WorkOrderTask, len(rows))
	for i, row := range rows {
		items[i] = toWorkOrderTask(row)
	}
	return items, nil
}

func (r *Repository) UpdateTask(ctx context.Context, id int64, req UpdateTaskRequest) (WorkOrderTask, error) {
	err := r.queries.UpdateWorkOrderTask(ctx, sqlc.UpdateWorkOrderTaskParams{
		ID:             id,
		Title:          req.Title,
		Description:    pgtype.Text{String: req.Description, Valid: req.Description != ""},
		AssigneeID:     pgtype.Int8{Int64: valueOrZero(req.AssigneeID), Valid: req.AssigneeID != nil},
		EstimatedHours: req.EstimatedHours,
	})
	if err != nil {
		return WorkOrderTask{}, err
	}
	return r.GetWorkOrderTask(ctx, id)
}

func (r *Repository) CompleteTask(ctx context.Context, id int64, actorID int64, actualHours float64) (WorkOrderTask, error) {
	err := r.queries.CompleteWorkOrderTask(ctx, sqlc.CompleteWorkOrderTaskParams{
		ID:            id,
		ActualHours:   actualHours,
	})
	if err != nil {
		return WorkOrderTask{}, err
	}
	return r.GetWorkOrderTask(ctx, id)
}

// =============================================================================
// Assets
// =============================================================================

func (r *Repository) InsertAsset(ctx context.Context, req CreateAssetRequest) (Asset, error) {
	id, err := r.queries.InsertAsset(ctx, sqlc.InsertAssetParams{
		CompanyID:      req.CompanyID,
		Code:           req.Code,
		Name:           req.Name,
		Description:    pgtype.Text{String: req.Description, Valid: req.Description != ""},
		AssetType:      req.AssetType,
		ParentID:       pgtype.Int8{Int64: valueOrZero(req.ParentID), Valid: req.ParentID != nil},
		LocationID:     pgtype.Int8{Int64: valueOrZero(req.LocationID), Valid: req.LocationID != nil},
		Manufacturer:   pgtype.Text{String: req.Manufacturer, Valid: req.Manufacturer != ""},
		Model:          pgtype.Text{String: req.Model, Valid: req.Model != ""},
		SerialNumber:   pgtype.Text{String: req.SerialNumber, Valid: req.SerialNumber != ""},
		InstallDate:    pgtype.Date{Time: valueOrZeroTime(req.InstallDate), Valid: req.InstallDate != nil},
		WarrantyExpiry: pgtype.Date{Time: valueOrZeroTime(req.WarrantyExpiry), Valid: req.WarrantyExpiry != nil},
		Status:         "ACTIVE",
		Criticality:    req.Criticality,
	})
	if err != nil {
		return Asset{}, err
	}
	return r.GetAsset(ctx, id)
}

func (r *Repository) GetAsset(ctx context.Context, id int64) (Asset, error) {
	row, err := r.queries.GetAsset(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Asset{}, ErrAssetNotFound
		}
		return Asset{}, err
	}
	return toAsset(row), nil
}

func (r *Repository) ListAssets(ctx context.Context, filter ListAssetsFilter) ([]Asset, error) {
	rows, err := r.queries.ListAssets(ctx, sqlc.ListAssetsParams{
		CompanyID:  filter.CompanyID,
		Column2:    valueOrZero(filter.LocationID),
		Column3:    filter.AssetType,
		Column4:    filter.Status,
		Column5:    filter.Search,
		Limit:      int32(filter.Limit),
		Offset:     int32(filter.Offset),
	})
	if err != nil {
		return nil, err
	}
	items := make([]Asset, len(rows))
	for i, row := range rows {
		items[i] = toAssetFromListRow(row)
	}
	return items, nil
}

func (r *Repository) UpdateAsset(ctx context.Context, id int64, req UpdateAssetRequest) (Asset, error) {
	err := r.queries.UpdateAsset(ctx, sqlc.UpdateAssetParams{
		ID:             id,
		Name:           req.Name,
		Description:    pgtype.Text{String: req.Description, Valid: req.Description != ""},
		LocationID:     pgtype.Int8{Int64: valueOrZero(req.LocationID), Valid: req.LocationID != nil},
		Manufacturer:   pgtype.Text{String: req.Manufacturer, Valid: req.Manufacturer != ""},
		Model:          pgtype.Text{String: req.Model, Valid: req.Model != ""},
		SerialNumber:   pgtype.Text{String: req.SerialNumber, Valid: req.SerialNumber != ""},
		WarrantyExpiry: pgtype.Date{Time: valueOrZeroTime(req.WarrantyExpiry), Valid: req.WarrantyExpiry != nil},
		Status:         req.Status,
		Criticality:    req.Criticality,
	})
	if err != nil {
		return Asset{}, err
	}
	return r.GetAsset(ctx, id)
}

// =============================================================================
// Locations
// =============================================================================

func (r *Repository) InsertLocation(ctx context.Context, req CreateLocationRequest) (Location, error) {
	id, err := r.queries.InsertLocation(ctx, sqlc.InsertLocationParams{
		CompanyID:  req.CompanyID,
		Code:       req.Code,
		Name:       req.Name,
		Description: pgtype.Text{String: req.Description, Valid: req.Description != ""},
		ParentID:   pgtype.Int8{Int64: valueOrZero(req.ParentID), Valid: req.ParentID != nil},
		Address:    pgtype.Text{String: req.Address, Valid: req.Address != ""},
		GpsLat:     pgtype.Float8{Float64: valueOrZeroFloat(req.GPSLat), Valid: req.GPSLat != nil},
		GpsLng:     pgtype.Float8{Float64: valueOrZeroFloat(req.GPSLng), Valid: req.GPSLng != nil},
		Active:     req.Active,
	})
	if err != nil {
		return Location{}, err
	}
	return r.GetLocation(ctx, id)
}

func (r *Repository) GetLocation(ctx context.Context, id int64) (Location, error) {
	row, err := r.queries.GetLocation(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Location{}, ErrLocationNotFound
		}
		return Location{}, err
	}
	return toLocation(row), nil
}

func (r *Repository) ListLocations(ctx context.Context, companyID int64) ([]Location, error) {
	rows, err := r.queries.ListLocations(ctx, companyID)
	if err != nil {
		return nil, err
	}
	items := make([]Location, len(rows))
	for i, row := range rows {
		items[i] = toLocation(row)
	}
	return items, nil
}

// =============================================================================
// PM Schedules
// =============================================================================

func (r *Repository) InsertPMSchedule(ctx context.Context, req CreatePMScheduleRequest) (PreventiveMaintenanceSchedule, error) {
	id, err := r.queries.InsertPMSchedule(ctx, sqlc.InsertPMScheduleParams{
		CompanyID:         req.CompanyID,
		AssetID:           req.AssetID,
		Name:              req.Name,
		Description:       pgtype.Text{String: req.Description, Valid: req.Description != ""},
		FrequencyType:     req.FrequencyType,
		FrequencyValue:    int32(req.FrequencyValue),
		MeterReadingType:  pgtype.Text{String: req.MeterReadingType, Valid: req.MeterReadingType != ""},
		TaskTemplateID:    pgtype.Int8{Int64: valueOrZero(req.TaskTemplateID), Valid: req.TaskTemplateID != nil},
		NextDueDate:       pgtype.Date{Time: time.Now(), Valid: false},
		NextDueMeter:      pgtype.Float8{Float64: 0, Valid: false},
		Active:            req.Active,
	})
	if err != nil {
		return PreventiveMaintenanceSchedule{}, err
	}
	return r.GetPMSchedule(ctx, id)
}

func (r *Repository) GetPMSchedule(ctx context.Context, id int64) (PreventiveMaintenanceSchedule, error) {
	row, err := r.queries.GetPMSchedule(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PreventiveMaintenanceSchedule{}, errors.New("cmms: pm schedule not found")
		}
		return PreventiveMaintenanceSchedule{}, err
	}
	return toPMSchedule(row), nil
}

func (r *Repository) ListPMSchedules(ctx context.Context, assetID int64) ([]PreventiveMaintenanceSchedule, error) {
	rows, err := r.queries.ListPMSchedules(ctx, assetID)
	if err != nil {
		return nil, err
	}
	items := make([]PreventiveMaintenanceSchedule, len(rows))
	for i, row := range rows {
		items[i] = toPMScheduleFromListRow(row)
	}
	return items, nil
}

func (r *Repository) ListDuePMSchedules(ctx context.Context, companyID int64) ([]PreventiveMaintenanceSchedule, error) {
	rows, err := r.queries.ListDuePMSchedules(ctx, companyID)
	if err != nil {
		return nil, err
	}
	items := make([]PreventiveMaintenanceSchedule, len(rows))
	for i, row := range rows {
		items[i] = toPMScheduleFromDueListRow(row)
	}
	return items, nil
}

// =============================================================================
// Task Templates
// =============================================================================

func (r *Repository) InsertTaskTemplate(ctx context.Context, companyID int64, name, description, category string, estimatedHours float64, instructions, safetyNotes string, actorID int64) (TaskTemplate, error) {
	id, err := r.queries.InsertTaskTemplate(ctx, sqlc.InsertTaskTemplateParams{
		CompanyID:       companyID,
		Name:            name,
		Description:     pgtype.Text{String: description, Valid: description != ""},
		Category:        pgtype.Text{String: category, Valid: category != ""},
		EstimatedHours:  estimatedHours,
		Instructions:    pgtype.Text{String: instructions, Valid: instructions != ""},
		SafetyNotes:     pgtype.Text{String: safetyNotes, Valid: safetyNotes != ""},
	})
	if err != nil {
		return TaskTemplate{}, err
	}
	return r.GetTaskTemplate(ctx, id)
}

func (r *Repository) GetTaskTemplate(ctx context.Context, id int64) (TaskTemplate, error) {
	row, err := r.queries.GetTaskTemplate(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TaskTemplate{}, errors.New("cmms: task template not found")
		}
		return TaskTemplate{}, err
	}
	return toTaskTemplate(row), nil
}

func (r *Repository) ListTaskTemplates(ctx context.Context, companyID int64) ([]TaskTemplate, error) {
	rows, err := r.queries.ListTaskTemplates(ctx, companyID)
	if err != nil {
		return nil, err
	}
	items := make([]TaskTemplate, len(rows))
	for i, row := range rows {
		items[i] = toTaskTemplate(row)
	}
	return items, nil
}

func (r *Repository) InsertTaskTemplateStep(ctx context.Context, taskTemplateID int64, sequence int, title, description string, estimatedHours float64, instructions string) error {
	return r.queries.InsertTaskTemplateStep(ctx, sqlc.InsertTaskTemplateStepParams{
		TaskTemplateID:  taskTemplateID,
		Sequence:        int32(sequence),
		Title:           title,
		Description:     pgtype.Text{String: description, Valid: description != ""},
		EstimatedHours:  estimatedHours,
		Instructions:    pgtype.Text{String: instructions, Valid: instructions != ""},
	})
}

func (r *Repository) ListTaskTemplateSteps(ctx context.Context, taskTemplateID int64) ([]TaskTemplateStep, error) {
	rows, err := r.queries.ListTaskTemplateSteps(ctx, taskTemplateID)
	if err != nil {
		return nil, err
	}
	items := make([]TaskTemplateStep, len(rows))
	for i, row := range rows {
		items[i] = toTaskTemplateStep(row)
	}
	return items, nil
}

// =============================================================================
// Spare Parts
// =============================================================================

func (r *Repository) InsertSparePart(ctx context.Context, req CreateSparePartRequest) (SparePart, error) {
	id, err := r.queries.InsertSparePart(ctx, sqlc.InsertSparePartParams{
		CompanyID:       req.CompanyID,
		Code:            req.Code,
		Name:            req.Name,
		Description:     pgtype.Text{String: req.Description, Valid: req.Description != ""},
		Category:        pgtype.Text{String: req.Category, Valid: req.Category != ""},
		UnitOfMeasure:   req.UnitOfMeasure,
		MinQuantity:     req.MinQuantity,
		MaxQuantity:     req.MaxQuantity,
		ReorderPoint:    req.ReorderPoint,
		LeadTimeDays:    int32(req.LeadTimeDays),
		UnitCost:        req.UnitCost,
		CriticalSpare:   req.CriticalSpare,
	})
	if err != nil {
		return SparePart{}, err
	}
	return r.GetSparePart(ctx, id)
}

func (r *Repository) GetSparePart(ctx context.Context, id int64) (SparePart, error) {
	row, err := r.queries.GetSparePart(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SparePart{}, errors.New("cmms: spare part not found")
		}
		return SparePart{}, err
	}
	return toSparePart(row), nil
}

func (r *Repository) ListSpareParts(ctx context.Context, companyID int64) ([]SparePart, error) {
	rows, err := r.queries.ListSpareParts(ctx, companyID)
	if err != nil {
		return nil, err
	}
	items := make([]SparePart, len(rows))
	for i, row := range rows {
		items[i] = toSparePart(row)
	}
	return items, nil
}

// =============================================================================
// Work Order Spare Parts
// =============================================================================

func (r *Repository) InsertWorkOrderSparePart(ctx context.Context, req AddSparePartRequest) (WorkOrderSparePart, error) {
	id, err := r.queries.InsertWorkOrderSparePart(ctx, sqlc.InsertWorkOrderSparePartParams{
		WorkOrderID: req.WorkOrderID,
		SparePartID: req.SparePartID,
		Quantity:    req.Quantity,
		UnitCost:    req.UnitCost,
		TotalCost:   req.Quantity * req.UnitCost,
	})
	if err != nil {
		return WorkOrderSparePart{}, err
	}
	return r.GetWorkOrderSparePart(ctx, id)
}

func (r *Repository) GetWorkOrderSparePart(ctx context.Context, id int64) (WorkOrderSparePart, error) {
	row, err := r.queries.GetWorkOrderSparePart(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WorkOrderSparePart{}, errors.New("cmms: work order spare part not found")
		}
		return WorkOrderSparePart{}, err
	}
	return toWorkOrderSparePart(row), nil
}

func (r *Repository) ListWorkOrderSpareParts(ctx context.Context, workOrderID int64) ([]WorkOrderSparePart, error) {
	rows, err := r.queries.ListWorkOrderSpareParts(ctx, workOrderID)
	if err != nil {
		return nil, err
	}
	items := make([]WorkOrderSparePart, len(rows))
	for i, row := range rows {
		items[i] = toWorkOrderSparePart(row)
	}
	return items, nil
}

func (r *Repository) IssueSparePart(ctx context.Context, id int64, actorID int64) (WorkOrderSparePart, error) {
	err := r.queries.IssueWorkOrderSparePart(ctx, sqlc.IssueWorkOrderSparePartParams{
		ID:       id,
		IssuedBy: pgtype.Int8{Int64: actorID, Valid: true},
	})
	if err != nil {
		return WorkOrderSparePart{}, err
	}
	return r.GetWorkOrderSparePart(ctx, id)
}

// =============================================================================
// Meter Readings
// =============================================================================

func (r *Repository) InsertMeterReading(ctx context.Context, req CreateMeterReadingRequest) (MeterReading, error) {
	id, err := r.queries.InsertMeterReading(ctx, sqlc.InsertMeterReadingParams{
		AssetID:      req.AssetID,
		ReadingType:  req.ReadingType,
		Value:        req.Value,
		ReadingDate:  pgtype.Date{Time: req.ReadingDate, Valid: !req.ReadingDate.IsZero()},
		EnteredBy:    req.EnteredBy,
		Notes:        pgtype.Text{String: req.Notes, Valid: req.Notes != ""},
	})
	if err != nil {
		return MeterReading{}, err
	}
	return r.GetMeterReading(ctx, id)
}

func (r *Repository) GetMeterReading(ctx context.Context, id int64) (MeterReading, error) {
	row, err := r.queries.GetMeterReading(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MeterReading{}, errors.New("cmms: meter reading not found")
		}
		return MeterReading{}, err
	}
	return toMeterReading(row), nil
}

func (r *Repository) GetMeterReadings(ctx context.Context, assetID int64, readingType string, limit int) ([]MeterReading, error) {
	rows, err := r.queries.ListMeterReadings(ctx, sqlc.ListMeterReadingsParams{
		AssetID:     assetID,
		Column2:     readingType,
		Limit:       int32(limit),
	})
	if err != nil {
		return nil, err
	}
	items := make([]MeterReading, len(rows))
	for i, row := range rows {
		items[i] = toMeterReading(row)
	}
	return items, nil
}

// =============================================================================
// Helpers for conversion
// =============================================================================

func toWorkOrder(row sqlc.GetWorkOrderRow) WorkOrder {
	var assetID, locationID, assigneeID *int64
	if row.AssetID.Valid {
		assetID = &row.AssetID.Int64
	}
	if row.LocationID.Valid {
		locationID = &row.LocationID.Int64
	}
	if row.AssigneeID.Valid {
		assigneeID = &row.AssigneeID.Int64
	}
	var plannedStart, plannedEnd, actualStart, actualEnd *time.Time
	if row.PlannedStart.Valid {
		plannedStart = &row.PlannedStart.Time
	}
	if row.PlannedEnd.Valid {
		plannedEnd = &row.PlannedEnd.Time
	}
	if row.ActualStart.Valid {
		actualStart = &row.ActualStart.Time
	}
	if row.ActualEnd.Valid {
		actualEnd = &row.ActualEnd.Time
	}
	return WorkOrder{
		ID:             row.ID,
		CompanyID:      row.CompanyID,
		Number:         row.Number,
		Title:          row.Title,
		Description:    row.Description.String,
		AssetID:        assetID,
		AssetName:      row.AssetName.String,
		LocationID:     locationID,
		LocationName:   row.LocationName.String,
		Priority:       Priority(row.Priority),
		Status:         Status(row.Status),
		Category:       row.Category,
		RequesterID:    row.RequesterID,
		AssigneeID:     assigneeID,
		PlannedStart:   plannedStart,
		PlannedEnd:     plannedEnd,
		ActualStart:    actualStart,
		ActualEnd:      actualEnd,
		EstimatedHours: row.EstimatedHours,
		ActualHours:    row.ActualHours,
		CreatedBy:      row.CreatedBy,
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
	}
}

func toWorkOrderFromListRow(row sqlc.ListWorkOrdersRow) WorkOrder {
	var assetID *int64
	if row.AssetID.Valid {
		assetID = &row.AssetID.Int64
	}
	var locationID *int64
	if row.LocationID.Valid {
		locationID = &row.LocationID.Int64
	}
	var assigneeID *int64
	if row.AssigneeID.Valid {
		assigneeID = &row.AssigneeID.Int64
	}
	var plannedStart *time.Time
	if row.PlannedStart.Valid {
		plannedStart = &row.PlannedStart.Time
	}
	var plannedEnd *time.Time
	if row.PlannedEnd.Valid {
		plannedEnd = &row.PlannedEnd.Time
	}

	return WorkOrder{
		ID:             row.ID,
		CompanyID:      row.CompanyID,
		Number:         row.Number,
		Title:          row.Title,
		Description:    row.Description.String,
		AssetID:        assetID,
		LocationID:     locationID,
		Priority:       Priority(row.Priority),
		Status:         Status(row.Status),
		Category:       row.Category,
		AssigneeID:     assigneeID,
		PlannedStart:   plannedStart,
		PlannedEnd:     plannedEnd,
		EstimatedHours: row.EstimatedHours,
		ActualHours:    row.ActualHours,
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
	}
}

func toWorkOrderTask(row sqlc.WorkOrderTask) WorkOrderTask {
	var assigneeID *int64
	if row.AssigneeID.Valid {
		assigneeID = &row.AssigneeID.Int64
	}
	var completedAt *time.Time
	if row.CompletedAt.Valid {
		completedAt = &row.CompletedAt.Time
	}
	return WorkOrderTask{
		ID:              row.ID,
		WorkOrderID:     row.WorkOrderID,
		Sequence:        int(row.Sequence),
		Title:           row.Title,
		Description:     row.Description.String,
		Status:          Status(row.Status),
		AssigneeID:      assigneeID,
		EstimatedHours:  row.EstimatedHours,
		ActualHours:     row.ActualHours,
		CompletedAt:     completedAt,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}
}

func toAsset(row sqlc.GetAssetRow) Asset {
	var parentID, locationID *int64
	if row.ParentID.Valid {
		parentID = &row.ParentID.Int64
	}
	if row.LocationID.Valid {
		locationID = &row.LocationID.Int64
	}
	var installDate, warrantyExpiry *time.Time
	if row.InstallDate.Valid {
		installDate = &row.InstallDate.Time
	}
	if row.WarrantyExpiry.Valid {
		warrantyExpiry = &row.WarrantyExpiry.Time
	}
	return Asset{
		ID:              row.ID,
		CompanyID:       row.CompanyID,
		Code:            row.Code,
		Name:            row.Name,
		Description:     row.Description.String,
		AssetType:       row.AssetType,
		ParentID:        parentID,
		LocationID:      locationID,
		Manufacturer:    row.Manufacturer.String,
		Model:           row.Model.String,
		SerialNumber:    row.SerialNumber.String,
		InstallDate:     installDate,
		WarrantyExpiry:  warrantyExpiry,
		Status:          row.Status,
		Criticality:     row.Criticality,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}
}

func toLocation(row sqlc.Location) Location {
	var parentID *int64
	if row.ParentID.Valid {
		parentID = &row.ParentID.Int64
	}
	var gpsLat, gpsLng *float64
	if row.GpsLat.Valid {
		gpsLat = &row.GpsLat.Float64
	}
	if row.GpsLng.Valid {
		gpsLng = &row.GpsLng.Float64
	}
	return Location{
		ID:          row.ID,
		CompanyID:   row.CompanyID,
		Code:        row.Code,
		Name:        row.Name,
		Description: row.Description.String,
		ParentID:    parentID,
		Address:     row.Address.String,
		GPSLat:      gpsLat,
		GPSLng:      gpsLng,
		Active:      row.Active,
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}
}

func toPMSchedule(row sqlc.GetPMScheduleRow) PreventiveMaintenanceSchedule {
	var taskTemplateID *int64
	if row.TaskTemplateID.Valid {
		taskTemplateID = &row.TaskTemplateID.Int64
	}
	var nextDueDate *time.Time
	if row.NextDueDate.Valid {
		nextDueDate = &row.NextDueDate.Time
	}
	var nextDueMeter *float64
	if row.NextDueMeter.Valid {
		nextDueMeter = &row.NextDueMeter.Float64
	}
	return PreventiveMaintenanceSchedule{
		ID:                row.ID,
		CompanyID:         row.CompanyID,
		AssetID:           row.AssetID,
		Name:              row.Name,
		Description:       row.Description.String,
		FrequencyType:     row.FrequencyType,
		FrequencyValue:    int(row.FrequencyValue),
		MeterReadingType:  row.MeterReadingType.String,
		TaskTemplateID:    taskTemplateID,
		NextDueDate:       nextDueDate,
		NextDueMeter:      nextDueMeter,
		Active:            row.Active,
		CreatedAt:         row.CreatedAt.Time,
		UpdatedAt:         row.UpdatedAt.Time,
	}
}

func toAssetFromListRow(row sqlc.ListAssetsRow) Asset {
	var parentID *int64
	if row.ParentID.Valid {
		parentID = &row.ParentID.Int64
	}
	var locationID *int64
	if row.LocationID.Valid {
		locationID = &row.LocationID.Int64
	}
	var installDate *time.Time
	if row.InstallDate.Valid {
		installDate = &row.InstallDate.Time
	}
	var warrantyExpiry *time.Time
	if row.WarrantyExpiry.Valid {
		warrantyExpiry = &row.WarrantyExpiry.Time
	}
	return Asset{
		ID:             row.ID,
		CompanyID:      row.CompanyID,
		Code:           row.Code,
		Name:           row.Name,
		Description:    row.Description.String,
		AssetType:      row.AssetType,
		ParentID:       parentID,
		LocationID:     locationID,
		Manufacturer:   row.Manufacturer.String,
		Model:          row.Model.String,
		SerialNumber:   row.SerialNumber.String,
		InstallDate:    installDate,
		WarrantyExpiry: warrantyExpiry,
		Status:         row.Status,
		Criticality:    row.Criticality,
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
	}
}

func toTaskTemplate(row sqlc.TaskTemplate) TaskTemplate {
	return TaskTemplate{
		ID:              row.ID,
		CompanyID:       row.CompanyID,
		Name:            row.Name,
		Description:     row.Description.String,
		Category:        row.Category.String,
		EstimatedHours:  row.EstimatedHours,
		Instructions:    row.Instructions.String,
		SafetyNotes:     row.SafetyNotes.String,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}
}

func toTaskTemplateStep(row sqlc.TaskTemplateStep) TaskTemplateStep {
	return TaskTemplateStep{
		ID:               row.ID,
		TaskTemplateID:   row.TaskTemplateID,
		Sequence:         int(row.Sequence),
		Title:            row.Title,
		Description:      row.Description.String,
		EstimatedHours:   row.EstimatedHours,
		Instructions:     row.Instructions.String,
		CreatedAt:        row.CreatedAt.Time,
	}
}

func toPMScheduleFromListRow(row sqlc.ListPMSchedulesRow) PreventiveMaintenanceSchedule {
	var taskTemplateID *int64
	if row.TaskTemplateID.Valid {
		taskTemplateID = &row.TaskTemplateID.Int64
	}
	var nextDueDate *time.Time
	if row.NextDueDate.Valid {
		nextDueDate = &row.NextDueDate.Time
	}
	var nextDueMeter *float64
	if row.NextDueMeter.Valid {
		nextDueMeter = &row.NextDueMeter.Float64
	}

	return PreventiveMaintenanceSchedule{
		ID:               row.ID,
		CompanyID:        row.CompanyID,
		AssetID:          row.AssetID,
		Name:             row.Name,
		Description:      row.Description.String,
		FrequencyType:    row.FrequencyType,
		FrequencyValue:   int(row.FrequencyValue),
		MeterReadingType: row.MeterReadingType.String,
		TaskTemplateID:   taskTemplateID,
		NextDueDate:      nextDueDate,
		NextDueMeter:     nextDueMeter,
		Active:           row.Active,
		CreatedAt:        row.CreatedAt.Time,
		UpdatedAt:        row.UpdatedAt.Time,
	}
}

func toPMScheduleFromDueListRow(row sqlc.ListDuePMSchedulesRow) PreventiveMaintenanceSchedule {
	var taskTemplateID *int64
	if row.TaskTemplateID.Valid {
		taskTemplateID = &row.TaskTemplateID.Int64
	}
	var nextDueDate *time.Time
	if row.NextDueDate.Valid {
		nextDueDate = &row.NextDueDate.Time
	}
	var nextDueMeter *float64
	if row.NextDueMeter.Valid {
		nextDueMeter = &row.NextDueMeter.Float64
	}

	return PreventiveMaintenanceSchedule{
		ID:               row.ID,
		CompanyID:        row.CompanyID,
		AssetID:          row.AssetID,
		Name:             row.Name,
		Description:      row.Description.String,
		FrequencyType:    row.FrequencyType,
		FrequencyValue:   int(row.FrequencyValue),
		MeterReadingType: row.MeterReadingType.String,
		TaskTemplateID:   taskTemplateID,
		NextDueDate:      nextDueDate,
		NextDueMeter:     nextDueMeter,
		Active:           row.Active,
		CreatedAt:        row.CreatedAt.Time,
		UpdatedAt:        row.UpdatedAt.Time,
	}
}

func toSparePart(row sqlc.SparePart) SparePart {
	return SparePart{
		ID:             row.ID,
		CompanyID:      row.CompanyID,
		Code:           row.Code,
		Name:           row.Name,
		Description:    row.Description.String,
		Category:       row.Category.String,
		UnitOfMeasure:  row.UnitOfMeasure,
		MinQuantity:    row.MinQuantity,
		MaxQuantity:    row.MaxQuantity,
		ReorderPoint:   row.ReorderPoint,
		LeadTimeDays:   int(row.LeadTimeDays),
		UnitCost:       row.UnitCost,
		CriticalSpare:  row.CriticalSpare,
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
	}
}

func toWorkOrderSparePart(row sqlc.WorkOrderSparePart) WorkOrderSparePart {
	var issuedAt *time.Time
	if row.IssuedAt.Valid {
		issuedAt = &row.IssuedAt.Time
	}
	var issuedBy *int64
	if row.IssuedBy.Valid {
		issuedBy = &row.IssuedBy.Int64
	}
	return WorkOrderSparePart{
		ID:          row.ID,
		WorkOrderID: row.WorkOrderID,
		SparePartID: row.SparePartID,
		Quantity:    row.Quantity,
		UnitCost:    row.UnitCost,
		TotalCost:   row.TotalCost,
		IssuedAt:    issuedAt,
		IssuedBy:    issuedBy,
		CreatedAt:   row.CreatedAt.Time,
	}
}

func toMeterReading(row sqlc.MeterReading) MeterReading {
	return MeterReading{
		ID:           row.ID,
		AssetID:      row.AssetID,
		ReadingType:  row.ReadingType,
		Value:        row.Value,
		ReadingDate:  row.ReadingDate.Time,
		EnteredBy:    row.EnteredBy,
		Notes:        row.Notes.String,
		CreatedAt:    row.CreatedAt.Time,
	}
}

// Helper functions
func valueOrZero(ptr *int64) int64 {
	if ptr == nil {
		return 0
	}
	return *ptr
}

func valueOrZeroTime(ptr *time.Time) time.Time {
	if ptr == nil {
		return time.Time{}
	}
	return *ptr
}

func valueOrZeroFloat(ptr *float64) float64 {
	if ptr == nil {
		return 0
	}
	return *ptr
}



func statusStringOrEmpty(s *Status) string {
	if s == nil {
		return ""
	}
	return string(*s)
}

func priorityStringOrEmpty(p *Priority) string {
	if p == nil {
		return ""
	}
	return string(*p)
}
func (r *Repository) ListAllDuePMSchedules(ctx context.Context) ([]PreventiveMaintenanceSchedule, error) {
	rows, err := r.queries.ListAllDuePMSchedules(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]PreventiveMaintenanceSchedule, len(rows))
	for i, row := range rows {
		items[i] = toPMScheduleFromAllDueListRow(row)
	}
	return items, nil
}

func (r *Repository) GeneratePMWorkOrderTx(ctx context.Context, req CreateWorkOrderRequest, number string, schedID int64, nextDate time.Time, nextMeter float64) (WorkOrder, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return WorkOrder{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.queries.WithTx(tx)

	woID, err := qtx.InsertWorkOrder(ctx, sqlc.InsertWorkOrderParams{
		CompanyID:      req.CompanyID,
		Number:         number,
		Title:          req.Title,
		Description:    pgtype.Text{String: req.Description, Valid: req.Description != ""},
		AssetID:        pgtype.Int8{Int64: valueOrZero(req.AssetID), Valid: req.AssetID != nil},
		LocationID:     pgtype.Int8{Int64: valueOrZero(req.LocationID), Valid: req.LocationID != nil},
		Priority:       string(req.Priority),
		Category:       req.Category,
		Status:         "REQUESTED",
		RequesterID:    req.RequesterID,
		AssigneeID:     pgtype.Int8{Int64: valueOrZero(req.AssigneeID), Valid: req.AssigneeID != nil},
		PlannedStart:   pgtype.Timestamptz{Time: valueOrZeroTime(req.PlannedStart), Valid: req.PlannedStart != nil},
		PlannedEnd:     pgtype.Timestamptz{Time: valueOrZeroTime(req.PlannedEnd), Valid: req.PlannedEnd != nil},
		EstimatedHours: req.EstimatedHours,
		CreatedBy:      req.ActorID,
	})
	if err != nil {
		return WorkOrder{}, err
	}

	var nextDatePg pgtype.Date
	if !nextDate.IsZero() {
		nextDatePg = pgtype.Date{Time: nextDate, Valid: true}
	}
	var nextMeterPg pgtype.Float8
	if nextMeter > 0 {
		nextMeterPg = pgtype.Float8{Float64: nextMeter, Valid: true}
	}

	if err := qtx.UpdatePMScheduleNextDue(ctx, sqlc.UpdatePMScheduleNextDueParams{
		ID:           schedID,
		NextDueDate:  nextDatePg,
		NextDueMeter: nextMeterPg,
	}); err != nil {
		return WorkOrder{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return WorkOrder{}, err
	}

	return r.GetWorkOrder(ctx, woID)
}

func (r *Repository) UpdatePMScheduleNextDue(ctx context.Context, id int64, nextDate time.Time, nextMeter float64) error {
	var nextDatePg pgtype.Date
	if !nextDate.IsZero() {
		nextDatePg = pgtype.Date{Time: nextDate, Valid: true}
	}
	var nextMeterPg pgtype.Float8
	if nextMeter > 0 {
		nextMeterPg = pgtype.Float8{Float64: nextMeter, Valid: true}
	}
	
	return r.queries.UpdatePMScheduleNextDue(ctx, sqlc.UpdatePMScheduleNextDueParams{
		ID:           id,
		NextDueDate:  nextDatePg,
		NextDueMeter: nextMeterPg,
	})
}

func toPMScheduleFromAllDueListRow(row sqlc.ListAllDuePMSchedulesRow) PreventiveMaintenanceSchedule {
	return PreventiveMaintenanceSchedule{
		ID:               row.ID,
		CompanyID:        row.CompanyID,
		AssetID:          row.AssetID,
		Name:             row.Name,
		Description:      row.Description.String,
		FrequencyType:    row.FrequencyType,
		FrequencyValue:   int(row.FrequencyValue),
		MeterReadingType: row.MeterReadingType.String,
		TaskTemplateID:   &row.TaskTemplateID.Int64, // simplified handling for brevity
		NextDueDate:      func() *time.Time { if row.NextDueDate.Valid { return &row.NextDueDate.Time } else { return nil } }(),
		NextDueMeter:     func() *float64 { if row.NextDueMeter.Valid { return &row.NextDueMeter.Float64 } else { return nil } }(),
		Active:           row.Active,
		CreatedAt:        row.CreatedAt.Time,
		UpdatedAt:        row.UpdatedAt.Time,
	}
}

// =============================================================================
// Advanced CMMS Features (Predictive Maintenance, IoT)
// =============================================================================

func (r *Repository) CreateIoTSensor(ctx context.Context, sensor IoTSensor) (int64, error) {
	return r.queries.CreateIoTSensor(ctx, sqlc.CreateIoTSensorParams{
		CompanyID:  sensor.CompanyID,
		AssetID:    sensor.AssetID,
		SensorCode: sensor.SensorCode,
		SensorType: sensor.SensorType,
		Status:     sensor.Status,
	})
}

func (r *Repository) InsertIoTReading(ctx context.Context, sensorID int64, value float64) (int64, error) {
	val := pgtype.Numeric{}
	val.Scan(fmt.Sprintf("%f", value))
	return r.queries.InsertIoTReading(ctx, sqlc.InsertIoTReadingParams{
		SensorID: sensorID,
		Value:    val,
	})
}

func (r *Repository) UpdateIoTSensorReading(ctx context.Context, sensorID int64, val float64, ts time.Time) error {
	numericVal := pgtype.Numeric{}
	numericVal.Scan(fmt.Sprintf("%f", val))
	var lastReading pgtype.Timestamptz
	lastReading.Time = ts
	lastReading.Valid = true
	return r.queries.UpdateIoTSensorReading(ctx, sqlc.UpdateIoTSensorReadingParams{
		ID:               sensorID,
		LastReadingAt:    lastReading,
		LastReadingValue: numericVal,
	})
}

func (r *Repository) CreatePredictiveModel(ctx context.Context, model PredictiveModel) (int64, error) {
	acc := pgtype.Numeric{}
	acc.Scan(fmt.Sprintf("%f", model.Accuracy))
	return r.queries.CreatePredictiveModel(ctx, sqlc.CreatePredictiveModelParams{
		CompanyID: model.CompanyID,
		AssetType: model.AssetType,
		ModelName: model.ModelName,
		Version:   model.Version,
		Accuracy:  acc,
		IsActive:  model.IsActive,
	})
}

func (r *Repository) CreatePredictiveAlert(ctx context.Context, alert PredictiveAlert) (int64, error) {
	var sensorID pgtype.Int8
	if alert.SensorID != nil {
		sensorID = pgtype.Int8{Int64: *alert.SensorID, Valid: true}
	}
	var modelID pgtype.Int8
	if alert.ModelID != nil {
		modelID = pgtype.Int8{Int64: *alert.ModelID, Valid: true}
	}
	return r.queries.CreatePredictiveAlert(ctx, sqlc.CreatePredictiveAlertParams{
		CompanyID:   alert.CompanyID,
		AssetID:     alert.AssetID,
		SensorID:    sensorID,
		ModelID:     modelID,
		Severity:    alert.Severity,
		Description: alert.Description,
	})
}

func (r *Repository) ResolvePredictiveAlert(ctx context.Context, id int64) error {
	return r.queries.ResolvePredictiveAlert(ctx, id)
}
