package qms

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// Repository persists QMS entities using sqlc-generated queries.
type Repository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

// NewRepository constructs a QMS repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool:    pool,
		queries: sqlc.New(pool),
	}
}

// =============================================================================
// NCRs
// =============================================================================

func (r *Repository) InsertNCR(ctx context.Context, req CreateNCRRequest, number string) (NonConformanceReport, error) {
	id, err := r.queries.InsertNCR(ctx, sqlc.InsertNCRParams{
		CompanyID:        req.CompanyID,
		Number:           number,
		Title:            req.Title,
		Description:      pgtype.Text{String: req.Description, Valid: req.Description != ""},
		SourceType:       req.SourceType,
		SourceID:         pgtype.Int8{Int64: 0, Valid: false},
		SourceReference:  pgtype.Text{String: req.SourceReference, Valid: req.SourceReference != ""},
		Category:         req.Category,
		Severity:         req.Severity,
		Status:           string(NCRStatusOpen),
		DetectedBy:       req.DetectedBy,
		DetectedAt:       pgtype.Timestamptz{Time: time.Now(), Valid: true},
		DetectedLocation: pgtype.Text{String: req.DetectedLocation, Valid: req.DetectedLocation != ""},
		ResponsiblePartyID: pgtype.Int8{Valid: false},
		AssignedTo:       pgtype.Int8{Int64: valueOrZero(req.AssignedTo), Valid: req.AssignedTo != nil},
		TargetClosureDate: pgtype.Date{Valid: false},
		CreatedBy:        req.ActorID,
	})
	if err != nil {
		return NonConformanceReport{}, err
	}
	return r.GetNCR(ctx, id)
}

func (r *Repository) GetNCR(ctx context.Context, id int64) (NonConformanceReport, error) {
	row, err := r.queries.GetNCR(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return NonConformanceReport{}, ErrNCRNotFound
		}
		return NonConformanceReport{}, err
	}
	return toNCR(row), nil
}

func (r *Repository) ListNCRs(ctx context.Context, filter ListNCRsFilter) ([]NonConformanceReport, error) {
	rows, err := r.queries.ListNCRs(ctx, sqlc.ListNCRsParams{
		CompanyID:  filter.CompanyID,
		Column2:    filter.SourceType,
		Column3:    filter.Category,
		Column4:    filter.Severity,
		Column5:    stringStatus(filter.Status),
		Column6:    valueOrZero(filter.AssignedTo),
		Column7:    pgtype.Timestamptz{Time: valueOrZeroTime(filter.DateFrom), Valid: filter.DateFrom != nil},
		Column8:    pgtype.Timestamptz{Time: valueOrZeroTime(filter.DateTo), Valid: filter.DateTo != nil},
		Limit:      int32(filter.Limit),
		Offset:     int32(filter.Offset),
	})
	if err != nil {
		return nil, err
	}
	items := make([]NonConformanceReport, len(rows))
	for i, row := range rows {
		items[i] = toNCR(row)
	}
	return items, nil
}

func (r *Repository) UpdateNCR(ctx context.Context, id int64, req UpdateNCRRequest) (NonConformanceReport, error) {
	err := r.queries.UpdateNCR(ctx, sqlc.UpdateNCRParams{
		ID:                 id,
		Title:              req.Title,
		Description:        pgtype.Text{String: req.Description, Valid: req.Description != ""},
		Category:           req.Category,
		Severity:           req.Severity,
		ResponsiblePartyID: pgtype.Int8{Int64: valueOrZero(req.ResponsiblePartyID), Valid: req.ResponsiblePartyID != nil},
		AssignedTo:         pgtype.Int8{Int64: valueOrZero(req.AssignedTo), Valid: req.AssignedTo != nil},
		TargetClosureDate:  pgtype.Date{Valid: false},
		RootCause:          pgtype.Text{String: req.RootCause, Valid: req.RootCause != ""},
		ContainmentAction:  pgtype.Text{String: req.ContainmentAction, Valid: req.ContainmentAction != ""},
	})
	if err != nil {
		return NonConformanceReport{}, err
	}
	return r.GetNCR(ctx, id)
}

func (r *Repository) UpdateNCRStatus(ctx context.Context, id int64, status Status, actorID int64) (NonConformanceReport, error) {
	err := r.queries.QMSUpdateNCRStatus(ctx, sqlc.QMSUpdateNCRStatusParams{
		ID:     id,
		Status: string(status),
	})
	if err != nil {
		return NonConformanceReport{}, err
	}
	return r.GetNCR(ctx, id)
}

func (r *Repository) CountNCRsWithPrefix(ctx context.Context, companyID int64, prefix string) (int64, error) {
	return r.queries.CountNCRsWithPrefix(ctx, sqlc.CountNCRsWithPrefixParams{
		CompanyID: companyID,
		Column2:   pgtype.Text{String: prefix, Valid: prefix != ""},
	})
}

func (r *Repository) InsertDisposition(ctx context.Context, req RecordDispositionRequest) (NCRDisposition, error) {
	id, err := r.queries.InsertNCRDisposition(ctx, sqlc.InsertNCRDispositionParams{
		NcrID:           req.NCRID,
		DispositionType: req.DispositionType,
		Description:     pgtype.Text{String: req.Description, Valid: req.Description != ""},
		ApprovedBy:      req.ApprovedBy,
		ApprovedAt:      pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		return NCRDisposition{}, err
	}
	row, err := r.queries.GetNCRDisposition(ctx, req.NCRID)
	if err != nil {
		return NCRDisposition{}, err
	}
	return NCRDisposition{
		ID:              id,
		NCRID:           row.NcrID,
		DispositionType: row.DispositionType,
		Description:     row.Description.String,
		ApprovedBy:      row.ApprovedBy,
		ApprovedAt:      row.ApprovedAt.Time,
		CreatedAt:       row.CreatedAt.Time,
	}, nil
}

// =============================================================================
// CAPAs
// =============================================================================

func (r *Repository) InsertCAPA(ctx context.Context, req CreateCAPARequest, number string) (CorrectiveAction, error) {
	id, err := r.queries.InsertCAPA(ctx, sqlc.InsertCAPAParams{
		CompanyID:       req.CompanyID,
		Number:          number,
		Title:           req.Title,
		Description:     pgtype.Text{String: req.Description, Valid: req.Description != ""},
		SourceType:      req.SourceType,
		SourceID:        pgtype.Int8{Valid: false},
		SourceReference: pgtype.Text{String: req.SourceReference, Valid: req.SourceReference != ""},
		Status:          string(CAPAStatusOpen),
		Priority:        req.Priority,
		OwnerID:         req.OwnerID,
		TeamMembers:     nil,
		RootCauseMethod: pgtype.Text{String: req.RootCauseMethod, Valid: req.RootCauseMethod != ""},
		TargetDate:      pgtype.Date{Valid: false},
		CreatedBy:       req.ActorID,
	})
	if err != nil {
		return CorrectiveAction{}, err
	}
	return r.GetCAPA(ctx, id)
}

func (r *Repository) GetCAPA(ctx context.Context, id int64) (CorrectiveAction, error) {
	row, err := r.queries.GetCAPA(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CorrectiveAction{}, ErrCAPANotFound
		}
		return CorrectiveAction{}, err
	}
	return toCAPA(row), nil
}

func (r *Repository) ListCAPAs(ctx context.Context, filter ListCAPAsFilter) ([]CorrectiveAction, error) {
	rows, err := r.queries.ListCAPAs(ctx, sqlc.ListCAPAsParams{
		CompanyID:  filter.CompanyID,
		Column2:    filter.SourceType,
		Column3:    stringStatus(filter.Status),
		Column4:    filter.Priority,
		Column5:    valueOrZero(filter.OwnerID),
		Column6:    pgtype.Timestamptz{Time: valueOrZeroTime(filter.DateFrom), Valid: filter.DateFrom != nil},
		Column7:    pgtype.Timestamptz{Time: valueOrZeroTime(filter.DateTo), Valid: filter.DateTo != nil},
		Limit:      int32(filter.Limit),
		Offset:     int32(filter.Offset),
	})
	if err != nil {
		return nil, err
	}
	items := make([]CorrectiveAction, len(rows))
	for i, row := range rows {
		items[i] = toCAPA(row)
	}
	return items, nil
}

func (r *Repository) UpdateCAPA(ctx context.Context, id int64, req UpdateCAPARequest) (CorrectiveAction, error) {
	err := r.queries.UpdateCAPA(ctx, sqlc.UpdateCAPAParams{
		ID:                 id,
		Title:              req.Title,
		Description:        pgtype.Text{String: req.Description, Valid: req.Description != ""},
		Priority:           req.Priority,
		OwnerID:            valueOrZero((*int64)(nil)),
		TeamMembers:        nil,
		RootCause:          pgtype.Text{String: req.RootCause, Valid: req.RootCause != ""},
		RootCauseMethod:    pgtype.Text{String: req.RootCauseMethod, Valid: req.RootCauseMethod != ""},
		CorrectiveAction:   pgtype.Text{String: req.CorrectiveAction, Valid: req.CorrectiveAction != ""},
		PreventiveAction:   pgtype.Text{String: req.PreventiveAction, Valid: req.PreventiveAction != ""},
		VerificationMethod: pgtype.Text{String: req.VerificationMethod, Valid: req.VerificationMethod != ""},
		VerificationResult: pgtype.Text{String: req.VerificationResult, Valid: req.VerificationResult != ""},
		EffectivenessCheck: pgtype.Text{String: req.EffectivenessCheck, Valid: req.EffectivenessCheck != ""},
		TargetDate:         pgtype.Date{Valid: false},
	})
	if err != nil {
		return CorrectiveAction{}, err
	}
	return r.GetCAPA(ctx, id)
}

func (r *Repository) UpdateCAPAStatus(ctx context.Context, id int64, status Status, actorID int64) (CorrectiveAction, error) {
	err := r.queries.QMSUpdateCAPAStatus(ctx, sqlc.QMSUpdateCAPAStatusParams{
		ID:     id,
		Status: string(status),
	})
	if err != nil {
		return CorrectiveAction{}, err
	}
	return r.GetCAPA(ctx, id)
}

func (r *Repository) CountCAPAsWithPrefix(ctx context.Context, companyID int64, prefix string) (int64, error) {
	return r.queries.CountCAPAsWithPrefix(ctx, sqlc.CountCAPAsWithPrefixParams{
		CompanyID: companyID,
		Column2:   pgtype.Text{String: prefix, Valid: prefix != ""},
	})
}

// =============================================================================
// Audits
// =============================================================================

func (r *Repository) InsertAudit(ctx context.Context, req CreateAuditRequest, number string) (Audit, error) {
	id, err := r.queries.InsertAudit(ctx, sqlc.InsertAuditParams{
		CompanyID:     req.CompanyID,
		Number:        number,
		Title:         req.Title,
		Description:   pgtype.Text{String: req.Description, Valid: req.Description != ""},
		AuditType:     req.AuditType,
		Status:        string(AuditStatusPlanned),
		Standard:      pgtype.Text{String: req.Standard, Valid: req.Standard != ""},
		Scope:         pgtype.Text{String: req.Scope, Valid: req.Scope != ""},
		LeadAuditorID: req.LeadAuditorID,
		AuditTeamIds:  nil,
		AuditeeID:     pgtype.Int8{Valid: false},
		PlannedStart:  pgtype.Date{Valid: false},
		PlannedEnd:    pgtype.Date{Valid: false},
		CreatedBy:     req.ActorID,
	})
	if err != nil {
		return Audit{}, err
	}
	return r.GetAudit(ctx, id)
}

func (r *Repository) GetAudit(ctx context.Context, id int64) (Audit, error) {
	row, err := r.queries.GetAudit(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Audit{}, ErrAuditNotFound
		}
		return Audit{}, err
	}
	return toAudit(row), nil
}

func (r *Repository) ListAudits(ctx context.Context, companyID int64, status *Status, auditType string, limit, offset int) ([]Audit, error) {
	rows, err := r.queries.ListAudits(ctx, sqlc.ListAuditsParams{
		CompanyID: companyID,
		Column2:   stringStatus(status),
		Column3:   auditType,
		Limit:     int32(limit),
		Offset:    int32(offset),
	})
	if err != nil {
		return nil, err
	}
	items := make([]Audit, len(rows))
	for i, row := range rows {
		items[i] = toAudit(row)
	}
	return items, nil
}

func (r *Repository) UpdateAudit(ctx context.Context, id int64, req UpdateAuditRequest) (Audit, error) {
	err := r.queries.UpdateAudit(ctx, sqlc.UpdateAuditParams{
		ID:           id,
		Title:        req.Title,
		Description:  pgtype.Text{String: req.Description, Valid: req.Description != ""},
		Scope:        pgtype.Text{String: req.Scope, Valid: req.Scope != ""},
		LeadAuditorID: valueOrZero((*int64)(nil)),
		AuditTeamIds: nil,
		AuditeeID:    pgtype.Int8{Valid: false},
		PlannedStart: pgtype.Date{Valid: false},
		PlannedEnd:   pgtype.Date{Valid: false},
	})
	if err != nil {
		return Audit{}, err
	}
	return r.GetAudit(ctx, id)
}

func (r *Repository) CountAuditsWithPrefix(ctx context.Context, companyID int64, prefix string) (int64, error) {
	return r.queries.CountAuditsWithPrefix(ctx, sqlc.CountAuditsWithPrefixParams{
		CompanyID: companyID,
		Column2:   pgtype.Text{String: prefix, Valid: prefix != ""},
	})
}

func (r *Repository) InsertFinding(ctx context.Context, req CreateFindingRequest) (AuditFinding, error) {
	id, err := r.queries.InsertAuditFinding(ctx, sqlc.InsertAuditFindingParams{
		AuditID:         req.AuditID,
		FindingNumber:   pgtype.Text{Valid: false},
		Category:        req.Category,
		Clause:          pgtype.Text{String: req.Clause, Valid: req.Clause != ""},
		Description:     req.Description,
		Evidence:        pgtype.Text{String: req.Evidence, Valid: req.Evidence != ""},
		Requirement:     pgtype.Text{String: req.Requirement, Valid: req.Requirement != ""},
		RiskLevel:       req.RiskLevel,
		Status:          "OPEN",
		AssignedTo:      pgtype.Int8{Valid: false},
		ResponseDueDate: pgtype.Date{Valid: false},
	})
	if err != nil {
		return AuditFinding{}, err
	}
	return r.GetFinding(ctx, id)
}

func (r *Repository) GetFinding(ctx context.Context, id int64) (AuditFinding, error) {
	row, err := r.queries.GetAuditFinding(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AuditFinding{}, errors.New("qms: finding not found")
		}
		return AuditFinding{}, err
	}
	return toFinding(row), nil
}

func (r *Repository) GetFindings(ctx context.Context, auditID int64) ([]AuditFinding, error) {
	rows, err := r.queries.ListAuditFindings(ctx, auditID)
	if err != nil {
		return nil, err
	}
	items := make([]AuditFinding, len(rows))
	for i, row := range rows {
		items[i] = toFinding(row)
	}
	return items, nil
}

func (r *Repository) UpdateFinding(ctx context.Context, id int64, req UpdateFindingRequest) (AuditFinding, error) {
	err := r.queries.UpdateAuditFinding(ctx, sqlc.UpdateAuditFindingParams{
		ID:              id,
		Category:        req.Category,
		Description:     req.Description,
		Evidence:        pgtype.Text{String: req.Evidence, Valid: req.Evidence != ""},
		Requirement:     pgtype.Text{String: req.Requirement, Valid: req.Requirement != ""},
		RiskLevel:       req.RiskLevel,
		Status:          req.Status,
		Response:        pgtype.Text{String: req.Response, Valid: req.Response != ""},
		ResponseDueDate: pgtype.Date{Valid: false},
		AssignedTo:      pgtype.Int8{Valid: false},
		VerifiedBy:      pgtype.Int8{Valid: false},
		VerifiedAt:      pgtype.Timestamptz{Valid: false},
	})
	if err != nil {
		return AuditFinding{}, err
	}
	return r.GetFinding(ctx, id)
}

// =============================================================================
// Supplier Quality
// =============================================================================

func (r *Repository) InsertSupplierQuality(ctx context.Context, req CreateSupplierQualityRequest) (SupplierQuality, error) {
	id, err := r.queries.InsertSupplierQuality(ctx, sqlc.InsertSupplierQualityParams{
		CompanyID:     req.CompanyID,
		SupplierID:    req.SupplierID,
		Status:        string(req.Status),
		QualityRating: req.QualityRating,
		RiskLevel:     req.RiskLevel,
		ApprovedDate:  pgtype.Date{Valid: false},
		ExpiryDate:    pgtype.Date{Valid: false},
		LastAuditDate: pgtype.Date{Valid: false},
		NextAuditDate: pgtype.Date{Valid: false},
		Notes:         pgtype.Text{String: req.Notes, Valid: req.Notes != ""},
		CreatedBy:     req.ActorID,
	})
	if err != nil {
		return SupplierQuality{}, err
	}
	return r.GetSupplierQuality(ctx, id)
}

func (r *Repository) GetSupplierQuality(ctx context.Context, id int64) (SupplierQuality, error) {
	row, err := r.queries.GetSupplierQuality(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SupplierQuality{}, errors.New("qms: supplier quality record not found")
		}
		return SupplierQuality{}, err
	}
	return toSupplierQuality(row), nil
}

func (r *Repository) ListSupplierQuality(ctx context.Context, companyID int64, status *Status, limit, offset int) ([]SupplierQuality, error) {
	rows, err := r.queries.ListSupplierQuality(ctx, sqlc.ListSupplierQualityParams{
		CompanyID: companyID,
		Column2:   stringStatus(status),
		Limit:     int32(limit),
		Offset:    int32(offset),
	})
	if err != nil {
		return nil, err
	}
	items := make([]SupplierQuality, len(rows))
	for i, row := range rows {
		items[i] = toSupplierQuality(row)
	}
	return items, nil
}

func (r *Repository) InsertSupplierAudit(ctx context.Context, req CreateSupplierAuditRequest) (SupplierAudit, error) {
	id, err := r.queries.InsertSupplierAudit(ctx, sqlc.InsertSupplierAuditParams{
		CompanyID:     req.CompanyID,
		SupplierID:    req.SupplierID,
		AuditNumber:   pgtype.Text{Valid: false},
		AuditType:     req.AuditType,
		Status:        string(AuditStatusPlanned),
		Standard:      pgtype.Text{String: req.Standard, Valid: req.Standard != ""},
		PlannedDate:   pgtype.Date{Valid: false},
		ActualDate:    pgtype.Date{Valid: false},
		Score:         0,
		LeadAuditorID: req.LeadAuditorID,
		ReportNumber:  pgtype.Text{Valid: false},
		CreatedBy:     req.ActorID,
	})
	if err != nil {
		return SupplierAudit{}, err
	}
	row, err := r.queries.GetSupplierAudit(ctx, id)
	if err != nil {
		return SupplierAudit{}, err
	}
	return SupplierAudit{
		ID:            row.ID,
		CompanyID:     row.CompanyID,
		SupplierID:    row.SupplierID,
		AuditType:     row.AuditType,
		Status:        Status(row.Status),
		Standard:      row.Standard.String,
		LeadAuditorID: row.LeadAuditorID,
		CreatedAt:     row.CreatedAt.Time,
	}, nil
}

// =============================================================================
// Quality Objectives
// =============================================================================

func (r *Repository) InsertQualityObjective(ctx context.Context, req CreateQualityObjectiveRequest) (QualityObjective, error) {
	id, err := r.queries.InsertQualityObjective(ctx, sqlc.InsertQualityObjectiveParams{
		CompanyID:   req.CompanyID,
		Name:        req.Name,
		Description: pgtype.Text{String: req.Description, Valid: req.Description != ""},
		MetricType:  req.MetricType,
		TargetValue: req.TargetValue,
		Unit:        pgtype.Text{String: req.Unit, Valid: req.Unit != ""},
		Frequency:   req.Frequency,
		OwnerID:     req.OwnerID,
		Status:      "ACTIVE",
		StartDate:   pgtype.Date{Time: req.StartDate, Valid: true},
		EndDate:     pgtype.Date{Valid: false},
	})
	if err != nil {
		return QualityObjective{}, err
	}
	row, err := r.queries.GetQualityObjective(ctx, id)
	if err != nil {
		return QualityObjective{}, err
	}
	return toQualityObjective(row), nil
}

func (r *Repository) InsertMeasurement(ctx context.Context, req RecordMeasurementRequest) (QualityObjectiveMeasurement, error) {
	id, err := r.queries.InsertQualityObjectiveMeasurement(ctx, sqlc.InsertQualityObjectiveMeasurementParams{
		ObjectiveID:     req.ObjectiveID,
		Value:           req.Value,
		MeasurementDate: pgtype.Date{Time: req.MeasurementDate, Valid: true},
		Notes:           pgtype.Text{String: req.Notes, Valid: req.Notes != ""},
		RecordedBy:      req.RecordedBy,
	})
	if err != nil {
		return QualityObjectiveMeasurement{}, err
	}
	row, err := r.queries.GetQualityObjectiveMeasurement(ctx, id)
	if err != nil {
		return QualityObjectiveMeasurement{}, err
	}
	return QualityObjectiveMeasurement{
		ID:              row.ID,
		ObjectiveID:     row.ObjectiveID,
		Value:           row.Value,
		MeasurementDate: row.MeasurementDate.Time,
		Notes:           row.Notes.String,
		RecordedBy:      row.RecordedBy,
		CreatedAt:       row.CreatedAt.Time,
	}, nil
}

func (r *Repository) GetMeasurements(ctx context.Context, objectiveID int64, limit int) ([]QualityObjectiveMeasurement, error) {
	rows, err := r.queries.ListQualityObjectiveMeasurements(ctx, sqlc.ListQualityObjectiveMeasurementsParams{
		ObjectiveID: objectiveID,
		Limit:       int32(limit),
	})
	if err != nil {
		return nil, err
	}
	items := make([]QualityObjectiveMeasurement, len(rows))
	for i, row := range rows {
		items[i] = QualityObjectiveMeasurement{
			ID:              row.ID,
			ObjectiveID:     row.ObjectiveID,
			Value:           row.Value,
			MeasurementDate: row.MeasurementDate.Time,
			Notes:           row.Notes.String,
			RecordedBy:      row.RecordedBy,
			CreatedAt:       row.CreatedAt.Time,
		}
	}
	return items, nil
}

// =============================================================================
// Mappers
// =============================================================================

func toNCR(row sqlc.Ncr) NonConformanceReport {
	return NonConformanceReport{
		ID:               row.ID,
		CompanyID:        row.CompanyID,
		Number:           row.Number,
		Title:            row.Title,
		Description:      row.Description.String,
		SourceType:       row.SourceType,
		SourceReference:  row.SourceReference.String,
		Category:         row.Category,
		Severity:         row.Severity,
		Status:           Status(row.Status),
		DetectedBy:       row.DetectedBy,
		DetectedAt:       row.DetectedAt.Time,
		DetectedLocation: row.DetectedLocation.String,
		RootCause:        row.RootCause.String,
		ContainmentAction: row.ContainmentAction.String,
		CreatedBy:        row.CreatedBy,
		CreatedAt:        row.CreatedAt.Time,
		UpdatedAt:        row.UpdatedAt.Time,
	}
}

func toCAPA(row sqlc.Capa) CorrectiveAction {
	return CorrectiveAction{
		ID:                 row.ID,
		CompanyID:          row.CompanyID,
		Number:             row.Number,
		Title:              row.Title,
		Description:        row.Description.String,
		SourceType:         row.SourceType,
		SourceReference:    row.SourceReference.String,
		Status:             Status(row.Status),
		Priority:           row.Priority,
		OwnerID:            row.OwnerID,
		RootCause:          row.RootCause.String,
		RootCauseMethod:    row.RootCauseMethod.String,
		CorrectiveAction:   row.CorrectiveAction.String,
		PreventiveAction:   row.PreventiveAction.String,
		VerificationMethod: row.VerificationMethod.String,
		VerificationResult: row.VerificationResult.String,
		EffectivenessCheck: row.EffectivenessCheck.String,
		CreatedBy:          row.CreatedBy,
		CreatedAt:          row.CreatedAt.Time,
		UpdatedAt:          row.UpdatedAt.Time,
	}
}

func toAudit(row sqlc.Audit) Audit {
	return Audit{
		ID:            row.ID,
		CompanyID:     row.CompanyID,
		Number:        row.Number,
		Title:         row.Title,
		Description:   row.Description.String,
		AuditType:     row.AuditType,
		Status:        Status(row.Status),
		Standard:      row.Standard.String,
		Scope:         row.Scope.String,
		LeadAuditorID: row.LeadAuditorID,
		CreatedBy:     row.CreatedBy,
		CreatedAt:     row.CreatedAt.Time,
		UpdatedAt:     row.UpdatedAt.Time,
	}
}

func toFinding(row sqlc.AuditFinding) AuditFinding {
	return AuditFinding{
		ID:          row.ID,
		AuditID:     row.AuditID,
		Category:    row.Category,
		Clause:      row.Clause.String,
		Description: row.Description,
		Evidence:    row.Evidence.String,
		Requirement: row.Requirement.String,
		RiskLevel:   row.RiskLevel,
		Status:      Status(row.Status),
		Response:    row.Response.String,
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}
}

func toSupplierQuality(row sqlc.SupplierQuality) SupplierQuality {
	return SupplierQuality{
		ID:            row.ID,
		CompanyID:     row.CompanyID,
		SupplierID:    row.SupplierID,
		Status:        Status(row.Status),
		QualityRating: row.QualityRating,
		RiskLevel:     row.RiskLevel,
		Notes:         row.Notes.String,
		CreatedBy:     row.CreatedBy,
		CreatedAt:     row.CreatedAt.Time,
		UpdatedAt:     row.UpdatedAt.Time,
	}
}


func toQualityObjective(row sqlc.QualityObjective) QualityObjective {
	return QualityObjective{
		ID:           row.ID,
		CompanyID:    row.CompanyID,
		Name:         row.Name,
		Description:  row.Description.String,
		MetricType:   row.MetricType,
		TargetValue:  row.TargetValue,
		CurrentValue: row.CurrentValue,
		Unit:         row.Unit.String,
		Frequency:    row.Frequency,
		OwnerID:      row.OwnerID,
		Status:       row.Status,
		CreatedAt:    row.CreatedAt.Time,
		UpdatedAt:    row.UpdatedAt.Time,
	}
}

// =============================================================================
// Helpers
// =============================================================================

func valueOrZero(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

func valueOrZeroTime(v *time.Time) time.Time {
	if v == nil {
		return time.Time{}
	}
	return *v
}

func stringStatus(v *Status) string {
	if v == nil {
		return ""
	}
	return string(*v)
}

// =============================================================================
// Inspections
// =============================================================================

func (r *Repository) InsertInspection(ctx context.Context, req CreateInspectionRequest) (Inspection, error) {
	id, err := r.queries.InsertQMSInspection(ctx, sqlc.InsertQMSInspectionParams{
		CompanyID:       req.CompanyID,
		Name:            req.Name,
		Description:     pgtype.Text{String: req.Description, Valid: req.Description != ""},
		ReferenceModule: pgtype.Text{String: req.ReferenceModule, Valid: req.ReferenceModule != ""},
		ReferenceID:     pgtype.Int8{Int64: valueOrZero(req.ReferenceID), Valid: req.ReferenceID != nil},
		Status:          "PLANNED",
		InspectorID:     pgtype.Int8{Int64: valueOrZero(req.InspectorID), Valid: req.InspectorID != nil},
		ScheduledAt:     pgtype.Timestamptz{Time: valueOrZeroTime(req.ScheduledAt), Valid: req.ScheduledAt != nil},
		StartedAt:       pgtype.Timestamptz{Valid: false},
		CreatedBy:       req.ActorID,
	})
	if err != nil {
		return Inspection{}, err
	}
	return r.GetInspection(ctx, id)
}

func (r *Repository) GetInspection(ctx context.Context, id int64) (Inspection, error) {
	row, err := r.queries.GetQMSInspection(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Inspection{}, ErrInspectionNotFound
		}
		return Inspection{}, err
	}
	return toInspection(row), nil
}

func (r *Repository) ListInspections(ctx context.Context, companyID int64, status, refModule string, limit, offset int32) ([]Inspection, error) {
	rows, err := r.queries.ListQMSInspections(ctx, sqlc.ListQMSInspectionsParams{
		CompanyID: companyID,
		Column2:   status,
		Column3:   refModule,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return nil, err
	}
	items := make([]Inspection, len(rows))
	for i, row := range rows {
		items[i] = toInspectionFromList(row)
	}
	return items, nil
}

func (r *Repository) UpdateInspectionStatus(ctx context.Context, id int64, status string, startedAt, completedAt *time.Time) error {
	return r.queries.UpdateQMSInspectionStatus(ctx, sqlc.UpdateQMSInspectionStatusParams{
		ID:          id,
		Status:      status,
		StartedAt:   pgtype.Timestamptz{Time: valueOrZeroTime(startedAt), Valid: startedAt != nil},
		CompletedAt: pgtype.Timestamptz{Time: valueOrZeroTime(completedAt), Valid: completedAt != nil},
	})
}

// =============================================================================
// Customer Complaints
// =============================================================================

func (r *Repository) InsertComplaint(ctx context.Context, req CreateComplaintRequest, number string) (CustomerComplaint, error) {
	id, err := r.queries.InsertCustomerComplaint(ctx, sqlc.InsertCustomerComplaintParams{
		CompanyID:       req.CompanyID,
		ComplaintNumber: number,
		CustomerID:      req.CustomerID,
		Title:           req.Title,
		Description:     req.Description,
		Status:          "RECEIVED",
		Severity:        req.Severity,
		AssignedTo:      pgtype.Int8{Int64: valueOrZero(req.AssignedTo), Valid: req.AssignedTo != nil},
		ReceivedAt:      pgtype.Timestamptz{Time: time.Now(), Valid: true},
		CreatedBy:       req.ActorID,
	})
	if err != nil {
		return CustomerComplaint{}, err
	}
	return r.GetComplaint(ctx, id)
}

func (r *Repository) GetComplaint(ctx context.Context, id int64) (CustomerComplaint, error) {
	row, err := r.queries.GetCustomerComplaint(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CustomerComplaint{}, ErrComplaintNotFound
		}
		return CustomerComplaint{}, err
	}
	return toComplaint(row), nil
}

func (r *Repository) ListComplaints(ctx context.Context, companyID int64, status, severity string, customerID *int64, limit, offset int32) ([]CustomerComplaint, error) {
	rows, err := r.queries.ListCustomerComplaints(ctx, sqlc.ListCustomerComplaintsParams{
		CompanyID: companyID,
		Column2:   status,
		Column3:   severity,
		Column4:   valueOrZero(customerID),
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return nil, err
	}

	items := make([]CustomerComplaint, len(rows))
	for i, row := range rows {
		items[i] = toComplaintFromList(row)
	}
	return items, nil
}

func (r *Repository) UpdateComplaintStatus(ctx context.Context, id int64, status string, closedAt *time.Time) error {
	var closed pgtype.Timestamptz
	if closedAt != nil {
		closed = pgtype.Timestamptz{Time: *closedAt, Valid: true}
	}
	return r.queries.UpdateCustomerComplaintStatus(ctx, sqlc.UpdateCustomerComplaintStatusParams{
		ID:       id,
		Status:   status,
		ClosedAt: closed,
	})
}

// =============================================================================
// Holds & Plans
// =============================================================================

func (r *Repository) InsertQualityHold(ctx context.Context, req CreateQualityHoldRequest) (int64, error) {
	return r.queries.InsertQMSHold(ctx, sqlc.InsertQMSHoldParams{
		CompanyID:       req.CompanyID,
		ReferenceModule: req.ReferenceModule,
		ReferenceID:     req.ReferenceID,
		Reason:          req.Reason,
		CreatedBy:       req.ActorID,
	})
}

func (r *Repository) ReleaseQualityHold(ctx context.Context, id, companyID, actorID int64) error {
	return r.queries.ReleaseQMSHold(ctx, sqlc.ReleaseQMSHoldParams{
		ID:         id,
		CompanyID:  companyID,
		ReleasedBy: pgtype.Int8{Int64: actorID, Valid: true},
	})
}

// =============================================================================
// Converters
// =============================================================================

func toInspection(row sqlc.GetQMSInspectionRow) Inspection {
	return Inspection{
		ID:              row.ID,
		CompanyID:       row.CompanyID,
		Name:            row.Name,
		Description:     row.Description.String,
		ReferenceModule: row.ReferenceModule.String,
		ReferenceID:     func() *int64 { if row.ReferenceID.Valid { return &row.ReferenceID.Int64 } else { return nil } }(),
		Status:          row.Status,
		InspectorID:     func() *int64 { if row.InspectorID.Valid { return &row.InspectorID.Int64 } else { return nil } }(),
		ScheduledAt:     func() *time.Time { if row.ScheduledAt.Valid { return &row.ScheduledAt.Time } else { return nil } }(),
		StartedAt:       func() *time.Time { if row.StartedAt.Valid { return &row.StartedAt.Time } else { return nil } }(),
		CompletedAt:     func() *time.Time { if row.CompletedAt.Valid { return &row.CompletedAt.Time } else { return nil } }(),
		CreatedBy:       row.CreatedBy,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}
}

func toInspectionFromList(row sqlc.ListQMSInspectionsRow) Inspection {
	return Inspection{
		ID:              row.ID,
		CompanyID:       row.CompanyID,
		Name:            row.Name,
		Description:     row.Description.String,
		ReferenceModule: row.ReferenceModule.String,
		ReferenceID:     func() *int64 { if row.ReferenceID.Valid { return &row.ReferenceID.Int64 } else { return nil } }(),
		Status:          row.Status,
		InspectorID:     func() *int64 { if row.InspectorID.Valid { return &row.InspectorID.Int64 } else { return nil } }(),
		ScheduledAt:     func() *time.Time { if row.ScheduledAt.Valid { return &row.ScheduledAt.Time } else { return nil } }(),
		StartedAt:       func() *time.Time { if row.StartedAt.Valid { return &row.StartedAt.Time } else { return nil } }(),
		CompletedAt:     func() *time.Time { if row.CompletedAt.Valid { return &row.CompletedAt.Time } else { return nil } }(),
		CreatedBy:       row.CreatedBy,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}
}

func toComplaint(row sqlc.GetCustomerComplaintRow) CustomerComplaint {
	return CustomerComplaint{
		ID:               row.ID,
		CompanyID:        row.CompanyID,
		ComplaintNumber:  row.ComplaintNumber,
		CustomerID:       row.CustomerID,
		Title:            row.Title,
		Description:      row.Description,
		Status:           row.Status,
		Severity:         row.Severity,
		AssignedTo:       func() *int64 { if row.AssignedTo.Valid { return &row.AssignedTo.Int64 } else { return nil } }(),
		ResponseEvidence: row.ResponseEvidence.String,
		ReceivedAt:       row.ReceivedAt.Time,
		ClosedAt:         func() *time.Time { if row.ClosedAt.Valid { return &row.ClosedAt.Time } else { return nil } }(),
		CreatedBy:        row.CreatedBy,
		CreatedAt:        row.CreatedAt.Time,
		UpdatedAt:        row.UpdatedAt.Time,
	}
}

func toComplaintFromList(row sqlc.ListCustomerComplaintsRow) CustomerComplaint {
	return CustomerComplaint{
		ID:               row.ID,
		CompanyID:        row.CompanyID,
		ComplaintNumber:  row.ComplaintNumber,
		CustomerID:       row.CustomerID,
		Title:            row.Title,
		Description:      row.Description,
		Status:           row.Status,
		Severity:         row.Severity,
		AssignedTo:       func() *int64 { if row.AssignedTo.Valid { return &row.AssignedTo.Int64 } else { return nil } }(),
		ResponseEvidence: row.ResponseEvidence.String,
		ReceivedAt:       row.ReceivedAt.Time,
		ClosedAt:         func() *time.Time { if row.ClosedAt.Valid { return &row.ClosedAt.Time } else { return nil } }(),
		CreatedBy:        row.CreatedBy,
		CreatedAt:        row.CreatedAt.Time,
		UpdatedAt:        row.UpdatedAt.Time,
	}
}

func (r *Repository) InsertInspectionPlan(ctx context.Context, req CreateInspectionPlanRequest) (InspectionPlan, error) {
	id, err := r.queries.InsertQMSInspectionPlan(ctx, sqlc.InsertQMSInspectionPlanParams{
		CompanyID:       req.CompanyID,
		Name:            req.Name,
		Description:     pgtype.Text{String: req.Description, Valid: req.Description != ""},
		ReferenceModule: pgtype.Text{String: req.ReferenceModule, Valid: req.ReferenceModule != ""},
		ReferenceID:     pgtype.Int8{Int64: req.ReferenceID, Valid: req.ReferenceID > 0},
		CreatedBy:       req.ActorID,
	})
	if err != nil {
		return InspectionPlan{}, err
	}
	return r.GetInspectionPlan(ctx, id)
}

func (r *Repository) GetInspectionPlan(ctx context.Context, id int64) (InspectionPlan, error) {
	row, err := r.queries.GetQMSInspectionPlan(ctx, id)
	if err != nil {
		return InspectionPlan{}, err
	}
	return InspectionPlan{
		ID:              row.ID,
		CompanyID:       row.CompanyID,
		Name:            row.Name,
		Description:     row.Description.String,
		ReferenceModule: row.ReferenceModule.String,
		ReferenceID:     row.ReferenceID.Int64,
		IsActive:        row.IsActive,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
		CreatedBy:       row.CreatedBy,
	}, nil
}
