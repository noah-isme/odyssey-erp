package documents

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// Repository persists documents, versions, ACLs, and metadata.
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

// Classifications

func (r *Repository) ListClassifications(ctx context.Context) ([]DocumentClassification, error) {
	rows, err := r.queries.ListDocumentClassifications(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]DocumentClassification, len(rows))
	for i, row := range rows {
		items[i] = DocumentClassification{
			ID:                row.ID,
			CompanyID:         row.CompanyID,
			Code:              row.Code,
			Name:              row.Name,
			Description:       row.Description.String,
			RequiresApproval:  row.RequiresApproval,
			RequiresSignature: row.RequiresSignature,
			Active:            row.Active,
			CreatedAt:         row.CreatedAt.Time,
		}
	}
	return items, nil
}

func (r *Repository) GetClassification(ctx context.Context, id int64) (DocumentClassification, error) {
	row, err := r.queries.GetDocumentClassification(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DocumentClassification{}, ErrClassificationNotFound
		}
		return DocumentClassification{}, err
	}
	return DocumentClassification{
		ID:                row.ID,
		CompanyID:         row.CompanyID,
		Code:              row.Code,
		Name:              row.Name,
		Description:       row.Description.String,
		RequiresApproval:  row.RequiresApproval,
		RequiresSignature: row.RequiresSignature,
		Active:            row.Active,
		CreatedAt:         row.CreatedAt.Time,
	}, nil
}

// Categories

func (r *Repository) ListCategories(ctx context.Context, companyID int64) ([]DocumentCategory, error) {
	rows, err := r.queries.ListDocumentCategories(ctx, companyID)
	if err != nil {
		return nil, err
	}
	items := make([]DocumentCategory, len(rows))
	for i, row := range rows {
		var parentID *int64
		if row.ParentID.Valid {
			parentID = &row.ParentID.Int64
		}
		var defaultClassID *int64
		if row.DefaultClassificationID.Valid {
			defaultClassID = &row.DefaultClassificationID.Int64
		}
		items[i] = DocumentCategory{
			ID:                      row.ID,
			CompanyID:               row.CompanyID,
			ParentID:                parentID,
			Code:                    row.Code,
			Name:                    row.Name,
			Description:             row.Description.String,
			DefaultClassificationID: defaultClassID,
			Active:                  row.Active,
			CreatedAt:               row.CreatedAt.Time,
		}
	}
	return items, nil
}

func (r *Repository) GetCategory(ctx context.Context, id int64) (DocumentCategory, error) {
	row, err := r.queries.GetDocumentCategory(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DocumentCategory{}, ErrCategoryNotFound
		}
		return DocumentCategory{}, err
	}
	var parentID *int64
	if row.ParentID.Valid {
		parentID = &row.ParentID.Int64
	}
	var defaultClassID *int64
	if row.DefaultClassificationID.Valid {
		defaultClassID = &row.DefaultClassificationID.Int64
	}
	return DocumentCategory{
		ID:                      row.ID,
		CompanyID:               row.CompanyID,
		ParentID:                parentID,
		Code:                    row.Code,
		Name:                    row.Name,
		Description:             row.Description.String,
		DefaultClassificationID: defaultClassID,
		Active:                  row.Active,
		CreatedAt:               row.CreatedAt.Time,
	}, nil
}

func (r *Repository) InsertCategory(ctx context.Context, req CreateCategoryRequest) (DocumentCategory, error) {
	row, err := r.queries.InsertDocumentCategory(ctx, sqlc.InsertDocumentCategoryParams{
		CompanyID:   req.CompanyID,
		ParentID:    int8Ptr(req.ParentID),
		Code:        req.Code,
		Name:        req.Name,
		Description: pgtype.Text{String: req.Description, Valid: req.Description != ""},
		Active:      req.Active,
		CreatedBy:   req.ActorID,
	})
	if err != nil {
		return DocumentCategory{}, err
	}
	var parentID *int64
	if row.ParentID.Valid {
		parentID = &row.ParentID.Int64
	}
	var defaultClassID *int64
	if row.DefaultClassificationID.Valid {
		defaultClassID = &row.DefaultClassificationID.Int64
	}
	return DocumentCategory{
		ID:                      row.ID,
		CompanyID:               row.CompanyID,
		ParentID:                parentID,
		Code:                    row.Code,
		Name:                    row.Name,
		Description:             row.Description.String,
		DefaultClassificationID: defaultClassID,
		Active:                  row.Active,
		CreatedAt:               row.CreatedAt.Time,
	}, nil
}

// Numbering Rules

func (r *Repository) ListNumberingRules(ctx context.Context, companyID int64) ([]DocumentNumberingRule, error) {
	rows, err := r.queries.ListDocumentNumberingRules(ctx, companyID)
	if err != nil {
		return nil, err
	}
	items := make([]DocumentNumberingRule, len(rows))
	for i, row := range rows {
		var scopeID *int64
		if row.ScopeID.Valid {
			scopeID = &row.ScopeID.Int64
		}
		items[i] = DocumentNumberingRule{
			ID:              row.ID,
			CompanyID:       row.CompanyID,
			Code:            row.Code,
			Name:            row.Name,
			Prefix:          row.Prefix.String,
			Suffix:          row.Suffix.String,
			Pattern:         row.Pattern,
			SequenceCurrent: int64(row.SequenceCurrent),
			Scope:           row.Scope,
			ScopeID:         scopeID,
			Active:          row.Active,
			CreatedAt:       row.CreatedAt.Time,
		}
	}
	return items, nil
}

func (r *Repository) GetNumberingRuleForCategory(ctx context.Context, companyID, categoryID int64) (DocumentNumberingRule, error) {
	row, err := r.queries.GetNumberingRuleForCategory(ctx, sqlc.GetNumberingRuleForCategoryParams{
		CompanyID: companyID,
		ScopeID:   pgtype.Int8{Int64: categoryID, Valid: true},
	})
	if err != nil {
		return DocumentNumberingRule{}, err
	}
	var scopeID *int64
	if row.ScopeID.Valid {
		scopeID = &row.ScopeID.Int64
	}
	return DocumentNumberingRule{
		ID:              row.ID,
		CompanyID:       row.CompanyID,
		Code:            row.Code,
		Name:            row.Name,
		Prefix:          row.Prefix.String,
		Suffix:          row.Suffix.String,
		Pattern:         row.Pattern,
		SequenceCurrent: int64(row.SequenceCurrent),
		Scope:           row.Scope,
		ScopeID:         scopeID,
		Active:          row.Active,
		CreatedAt:       row.CreatedAt.Time,
	}, nil
}

func (r *Repository) GetDefaultNumberingRule(ctx context.Context, companyID int64) (DocumentNumberingRule, error) {
	row, err := r.queries.GetDefaultNumberingRule(ctx, companyID)
	if err != nil {
		return DocumentNumberingRule{}, err
	}
	return DocumentNumberingRule{
		ID:              row.ID,
		CompanyID:       row.CompanyID,
		Code:            row.Code,
		Name:            row.Name,
		Prefix:          row.Prefix.String,
		Suffix:          row.Suffix.String,
		Pattern:         row.Pattern,
		SequenceCurrent: int64(row.SequenceCurrent),
		Scope:           row.Scope,
		ScopeID:         nil,
		Active:          row.Active,
		CreatedAt:       row.CreatedAt.Time,
	}, nil
}

func (r *Repository) IncrementNumberingSequence(ctx context.Context, ruleID int64) error {
	return r.queries.IncrementNumberingSequence(ctx, ruleID)
} // Documents

func (r *Repository) InsertDocument(ctx context.Context, req CreateDocumentRequest, number string) (Document, error) {
	id, err := r.queries.InsertDocument(ctx, sqlc.InsertDocumentParams{
		CompanyID:        req.CompanyID,
		DocumentNumber:   number,
		Title:            req.Title,
		Description:      pgtype.Text{String: req.Description, Valid: req.Description != ""},
		CategoryID:       pgtype.Int8{Int64: req.CategoryID, Valid: req.CategoryID > 0},
		ClassificationID: req.ClassificationID,
		NumberingRuleID:  pgtype.Int8{Valid: false},
		OwnerID:          req.OwnerID,
		Status:           string(StatusDraft),
		CreatedBy:        req.ActorID,
		UpdatedBy:        req.ActorID,
	})
	if err != nil {
		return Document{}, err
	}
	return r.GetDocument(ctx, id)
}

func (r *Repository) GetDocument(ctx context.Context, id int64) (Document, error) {
	row, err := r.queries.GetDocument(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Document{}, ErrDocumentNotFound
		}
		return Document{}, err
	}
	var migrationSource, migrationSourceID *string
	if row.MigrationSource.Valid {
		migrationSource = &row.MigrationSource.String
	}
	if row.MigrationSourceID.Valid {
		migrationSourceID = &row.MigrationSourceID.String
	}
	var catID int64
	if row.CategoryID.Valid {
		catID = row.CategoryID.Int64
	}
	return Document{
		ID:                 row.ID,
		CompanyID:          row.CompanyID,
		Number:             row.DocumentNumber,
		Title:              row.Title,
		Description:        row.Description.String,
		CategoryID:         catID,
		ClassificationID:   row.ClassificationID,
		OwnerID:            row.OwnerID,
		Status:             Status(row.Status),
		CurrentVersionID:   int64Ptr(row.CurrentVersionID),
		MigrationSource:    migrationSource,
		MigrationSourceID:  migrationSourceID,
		CreatedAt:          row.CreatedAt.Time,
		UpdatedAt:          row.UpdatedAt.Time,
		CategoryName:       row.CategoryName.String,
		ClassificationName: row.ClassificationName.String,
		OwnerName:          row.OwnerName.String,
		CompanyName:        row.CompanyName.String,
	}, nil
}

func (r *Repository) ListDocuments(ctx context.Context, filter ListFilter) ([]Document, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	rows, err := r.queries.ListDocuments(ctx, sqlc.ListDocumentsParams{
		CompanyID: filter.CompanyID,
		Column2:   derefInt64(filter.CategoryID),
		Column3:   derefInt64(filter.ClassificationID),
		Column4:   derefInt64(filter.OwnerID),
		Column5:   statusStringOrEmpty(filter.Status),
		Column6:   filter.Search,
		Limit:     int32(limit),
		Offset:    int32(offset),
	})
	if err != nil {
		return nil, err
	}
	items := make([]Document, len(rows))
	for i, row := range rows {
		var catID int64
		if row.CategoryID.Valid {
			catID = row.CategoryID.Int64
		}
		items[i] = Document{
			ID:                 row.ID,
			CompanyID:          row.CompanyID,
			Number:             row.DocumentNumber,
			Title:              row.Title,
			Description:        row.Description.String,
			CategoryID:         catID,
			ClassificationID:   row.ClassificationID,
			OwnerID:            row.OwnerID,
			Status:             Status(row.Status),
			CreatedAt:          row.CreatedAt.Time,
			UpdatedAt:          row.UpdatedAt.Time,
			CategoryName:       row.CategoryName.String,
			ClassificationName: row.ClassificationName.String,
			OwnerName:          row.OwnerName.String,
			CompanyName:        row.CompanyName.String,
		}
	}
	return items, nil
}

func (r *Repository) UpdateDocument(ctx context.Context, id int64, req UpdateDocumentRequest) (Document, error) {
	err := r.queries.UpdateDocument(ctx, sqlc.UpdateDocumentParams{
		ID:               id,
		Title:            strings.TrimSpace(req.Title),
		Description:      pgtype.Text{String: req.Description, Valid: req.Description != ""},
		CategoryID:       pgtype.Int8{Int64: valueOrZero(req.CategoryID), Valid: req.CategoryID != nil},
		ClassificationID: valueOrZero(req.ClassificationID),
		OwnerID:          valueOrZero(req.OwnerID),
		UpdatedBy:        req.ActorID,
	})
	if err != nil {
		return Document{}, err
	}
	return r.GetDocument(ctx, id)
}

func (r *Repository) DeleteDocument(ctx context.Context, id int64, actorID int64) error {
	return r.queries.DeleteDocument(ctx, id)
}

// Document Versions

func (r *Repository) InsertDocumentVersion(ctx context.Context, req CreateVersionRequest) (DocumentVersion, error) {
	id, err := r.queries.InsertDocumentVersion(ctx, sqlc.InsertDocumentVersionParams{
		CompanyID:        req.CompanyID,
		DocumentID:       req.DocumentID,
		VersionNumber:    int32(req.VersionNumber),
		BlobID:           req.DocumentID, // placeholder until blob upload wired
		ClassificationID: req.ClassificationID,
		ChangeSummary:    pgtype.Text{String: req.Description, Valid: req.Description != ""},
		Status:           string(StatusDraft),
		CreatedBy:        req.ActorID,
	})
	if err != nil {
		return DocumentVersion{}, err
	}
	return r.GetDocumentVersion(ctx, id)
}

func (r *Repository) GetDocumentVersion(ctx context.Context, id int64) (DocumentVersion, error) {
	row, err := r.queries.GetDocumentVersion(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DocumentVersion{}, ErrDocumentVersionNotFound
		}
		return DocumentVersion{}, err
	}
	var approvedAt, effectiveAt, supersededAt *time.Time
	if row.ApprovedAt.Valid {
		approvedAt = &row.ApprovedAt.Time
	}
	if row.EffectiveAt.Valid {
		effectiveAt = &row.EffectiveAt.Time
	}
	if row.SupersededAt.Valid {
		supersededAt = &row.SupersededAt.Time
	}
	var approvedBy *int64
	if row.ApprovedBy.Valid {
		approvedBy = &row.ApprovedBy.Int64
	}
	var supersededByVersionID *int64
	if row.SupersededByVersionID.Valid {
		supersededByVersionID = &row.SupersededByVersionID.Int64
	}
	blobIDVal := row.BlobID
	return DocumentVersion{
		ID:                    row.ID,
		CompanyID:             row.CompanyID,
		DocumentID:            row.DocumentID,
		VersionNumber:         int(row.VersionNumber),
		VersionLabel:          row.VersionLabel.String,
		BlobID:                &blobIDVal,
		Status:                Status(row.Status),
		ClassificationID:      row.ClassificationID,
		ChangeSummary:         row.ChangeSummary.String,
		ApprovedBy:            approvedBy,
		ApprovedAt:            approvedAt,
		EffectiveAt:           effectiveAt,
		SupersededAt:          supersededAt,
		SupersededByVersionID: supersededByVersionID,
		CreatedBy:             row.CreatedBy,
		CreatedAt:             row.CreatedAt.Time,
		DocumentNumber:        row.DocumentNumber.String,
		DocumentTitle:         row.DocumentTitle.String,
		BlobStorageKey:        row.BlobStorageKey.String,
		CreatedByName:         row.CreatedByName.String,
	}, nil
}

func (r *Repository) ListDocumentVersions(ctx context.Context, filter ListVersionsFilter) ([]DocumentVersion, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	rows, err := r.queries.ListDocumentVersions(ctx, sqlc.ListDocumentVersionsParams{
		CompanyID:  filter.CompanyID,
		DocumentID: filter.DocumentID,
		Column3:    statusStringOrEmpty(filter.Status),
		Limit:      int32(limit),
		Offset:     int32(offset),
	})
	if err != nil {
		return nil, err
	}
	items := make([]DocumentVersion, len(rows))
	for i, row := range rows {
		blobIDVal := row.BlobID
		var approvedBy *int64
		if row.ApprovedBy.Valid {
			approvedBy = &row.ApprovedBy.Int64
		}
		items[i] = DocumentVersion{
			ID:               row.ID,
			CompanyID:        row.CompanyID,
			DocumentID:       row.DocumentID,
			VersionNumber:    int(row.VersionNumber),
			VersionLabel:     row.VersionLabel.String,
			BlobID:           &blobIDVal,
			Status:           Status(row.Status),
			ClassificationID: row.ClassificationID,
			ChangeSummary:    row.ChangeSummary.String,
			ApprovedBy:       approvedBy,
			CreatedBy:        row.CreatedBy,
			CreatedAt:        row.CreatedAt.Time,
			DocumentNumber:   row.DocumentNumber.String,
			DocumentTitle:    row.DocumentTitle.String,
			BlobStorageKey:   row.BlobStorageKey.String,
			CreatedByName:    row.CreatedByName.String,
		}
	}
	return items, nil
}

func (r *Repository) GetLatestVersion(ctx context.Context, documentID int64) (DocumentVersion, error) {
	row, err := r.queries.GetLatestDocumentVersion(ctx, documentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DocumentVersion{}, ErrDocumentVersionNotFound
		}
		return DocumentVersion{}, err
	}
	blobIDVal := row.BlobID
	return DocumentVersion{
		ID:               row.ID,
		CompanyID:        row.CompanyID,
		DocumentID:       row.DocumentID,
		VersionNumber:    int(row.VersionNumber),
		VersionLabel:     row.VersionLabel.String,
		BlobID:           &blobIDVal,
		Status:           Status(row.Status),
		ClassificationID: row.ClassificationID,
		ChangeSummary:    row.ChangeSummary.String,
		CreatedBy:        row.CreatedBy,
		CreatedAt:        row.CreatedAt.Time,
		DocumentNumber:   row.DocumentNumber.String,
		DocumentTitle:    row.DocumentTitle.String,
		BlobStorageKey:   row.BlobStorageKey.String,
		CreatedByName:    row.CreatedByName.String,
	}, nil
}

func (r *Repository) SetCurrentVersion(ctx context.Context, documentID, versionID int64) error {
	return r.queries.SetCurrentDocumentVersion(ctx, sqlc.SetCurrentDocumentVersionParams{
		ID:               documentID,
		CurrentVersionID: pgtype.Int8{Int64: versionID, Valid: true},
	})
}

func (r *Repository) SetVersionStatus(ctx context.Context, versionID int64, status Status, actorID int64) (DocumentVersion, error) {
	err := r.queries.UpdateDocumentVersionStatus(ctx, sqlc.UpdateDocumentVersionStatusParams{
		ID:     versionID,
		Status: string(status),
	})
	if err != nil {
		return DocumentVersion{}, err
	}
	return r.GetDocumentVersion(ctx, versionID)
}

// Blobs

func (r *Repository) InsertBlob(ctx context.Context, req CreateBlobRequest) (int64, error) {
	return r.queries.InsertStorageBlob(ctx, sqlc.InsertStorageBlobParams{
		CompanyID:           req.CompanyID,
		StorageKey:          req.StorageKey,
		StorageDriver:       req.StorageDriver,
		Bucket:              pgtype.Text{String: req.Bucket, Valid: req.Bucket != ""},
		SizeBytes:           req.SizeBytes,
		ChecksumSha256:      req.ChecksumSha256,
		DeclaredContentType: pgtype.Text{String: req.DeclaredContentType, Valid: req.DeclaredContentType != ""},
		DetectedContentType: pgtype.Text{String: req.DetectedContentType, Valid: req.DetectedContentType != ""},
		MalwareScanStatus:   req.MalwareScanStatus,
		CreatedBy:           req.CreatedBy,
	})
}

func (r *Repository) GetBlob(ctx context.Context, id int64) (Blob, error) {
	row, err := r.queries.GetStorageBlob(ctx, id)
	if err != nil {
		return Blob{}, err
	}
	return Blob{
		ID:                  row.ID,
		CompanyID:           row.CompanyID,
		StorageKey:          row.StorageKey,
		StorageDriver:       row.StorageDriver,
		Bucket:              row.Bucket.String,
		SizeBytes:           row.SizeBytes,
		ChecksumSha256:      row.ChecksumSha256,
		DeclaredContentType: row.DeclaredContentType.String,
		DetectedContentType: row.DetectedContentType.String,
		MalwareScanStatus:   row.MalwareScanStatus,
		CreatedAt:           row.CreatedAt.Time,
		CreatedBy:           row.CreatedBy,
	}, nil
}

// GetPendingDispositions returns approved requests ready for execution.
func (r *Repository) GetPendingDispositions(ctx context.Context) ([]DispositionRequest, error) {
	rows, err := r.queries.GetPendingDispositions(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]DispositionRequest, len(rows))
	for i, row := range rows {
		items[i] = DispositionRequest{
			ID:                row.ID,
			CompanyID:         row.CompanyID,
			DocumentVersionID: row.DocumentVersionID,
			RequestedBy:       row.RequestedBy,
			Status:            row.Status,
		}
	}
	return items, nil
}

// HasActiveLegalHold reports whether a company has an active legal hold.
func (r *Repository) HasActiveLegalHold(ctx context.Context, companyID int64) (bool, error) {
	return r.queries.HasActiveLegalHold(ctx, companyID)
}

// UpdateDispositionExecution records a disposition execution result.
func (r *Repository) UpdateDispositionExecution(ctx context.Context, update DispositionExecutionUpdate) error {
	var executedAt pgtype.Timestamptz
	if update.ExecutedAt != nil {
		executedAt = pgtype.Timestamptz{Time: *update.ExecutedAt, Valid: true}
	}
	var executedBy pgtype.Int8
	if update.ExecutedBy != nil {
		executedBy = pgtype.Int8{Int64: *update.ExecutedBy, Valid: true}
	}
	var errorMessage pgtype.Text
	if update.ErrorMessage != nil {
		errorMessage = pgtype.Text{String: *update.ErrorMessage, Valid: true}
	}
	return r.queries.UpdateDispositionExecution(ctx, sqlc.UpdateDispositionExecutionParams{
		ID:                update.ID,
		Status:            update.Status,
		ExecutedAt:        executedAt,
		ExecutedBy:        executedBy,
		ExecutionEvidence: update.ExecutionEvidence,
		ErrorMessage:      errorMessage,
	})
}

// ACLs

func (r *Repository) InsertACL(ctx context.Context, req CreateACLRequest) (DocumentACL, error) {
	var documentID, classificationID pgtype.Int8
	if req.DocumentID != nil {
		documentID = pgtype.Int8{Int64: *req.DocumentID, Valid: true}
	}
	if req.ClassificationID != nil {
		classificationID = pgtype.Int8{Int64: *req.ClassificationID, Valid: true}
	}
	principalID := valueOrZero(req.PrincipalID)
	effect := req.Effect
	if effect == "" {
		effect = "ALLOW"
	}
	_, err := r.queries.InsertDocumentACL(ctx, sqlc.InsertDocumentACLParams{
		CompanyID:        req.CompanyID,
		DocumentID:       documentID,
		ClassificationID: classificationID,
		PrincipalType:    req.PrincipalType,
		PrincipalID:      principalID,
		Permission:       req.Permission,
		Effect:           effect,
		GrantedBy:        req.GrantedBy,
	})
	if err != nil {
		return DocumentACL{}, err
	}
	// Return a constructed domain object (no GET query defined)
	return DocumentACL{
		CompanyID:        req.CompanyID,
		DocumentID:       req.DocumentID,
		ClassificationID: req.ClassificationID,
		PrincipalType:    req.PrincipalType,
		PrincipalID:      req.PrincipalID,
		Permission:       req.Permission,
		Effect:           effect,
		GrantedBy:        req.GrantedBy,
	}, nil
}

func (r *Repository) DeleteACL(ctx context.Context, id int64) error {
	return r.queries.DeleteDocumentACL(ctx, id)
}

func (r *Repository) ListACLs(ctx context.Context, companyID int64, documentID, classificationID *int64) ([]DocumentACL, error) {
	rows, err := r.queries.ListDocumentACLs(ctx, sqlc.ListDocumentACLsParams{
		CompanyID: companyID,
		Column2:   derefInt64(documentID),
		Column3:   derefInt64(classificationID),
	})
	if err != nil {
		return nil, err
	}
	items := make([]DocumentACL, len(rows))
	for i, row := range rows {
		var docID, classID *int64
		if row.DocumentID.Valid {
			docID = &row.DocumentID.Int64
		}
		if row.ClassificationID.Valid {
			classID = &row.ClassificationID.Int64
		}
		var expiresAt *time.Time
		if row.ExpiresAt.Valid {
			expiresAt = &row.ExpiresAt.Time
		}
		principalIDVal := row.PrincipalID
		items[i] = DocumentACL{
			ID:               row.ID,
			CompanyID:        row.CompanyID,
			DocumentID:       docID,
			ClassificationID: classID,
			PrincipalType:    row.PrincipalType,
			PrincipalID:      &principalIDVal,
			Permission:       row.Permission,
			Effect:           row.Effect,
			GrantedBy:        row.GrantedBy,
			ExpiresAt:        expiresAt,
		}
	}
	return items, nil
}

func (r *Repository) GetUserRoles(ctx context.Context, userID int64) ([]int64, error) {
	return r.queries.GetUserRoles(ctx, userID)
}

// Links

func (r *Repository) InsertLink(ctx context.Context, req CreateLinkRequest) (DocumentLink, error) {
	id, err := r.queries.InsertDocumentLink(ctx, sqlc.InsertDocumentLinkParams{
		CompanyID:         req.CompanyID,
		DocumentVersionID: req.DocumentVersionID,
		TargetModule:      req.TargetModule,
		TargetID:          req.TargetID,
		TargetCompanyID:   req.TargetCompanyID,
		LinkType:          req.LinkType,
		Description:       pgtype.Text{String: req.Description, Valid: req.Description != ""},
		CreatedBy:         req.CreatedBy,
	})
	if err != nil {
		return DocumentLink{}, err
	}
	return DocumentLink{
		ID:                id,
		CompanyID:         req.CompanyID,
		DocumentVersionID: req.DocumentVersionID,
		TargetModule:      req.TargetModule,
		TargetID:          req.TargetID,
		TargetCompanyID:   req.TargetCompanyID,
		LinkType:          req.LinkType,
		Description:       req.Description,
		CreatedBy:         req.CreatedBy,
	}, nil
}

func (r *Repository) DeleteLink(ctx context.Context, id int64) error {
	return r.queries.DeleteDocumentLink(ctx, id)
}

func (r *Repository) ListLinks(ctx context.Context, documentVersionID int64) ([]DocumentLink, error) {
	rows, err := r.queries.ListDocumentLinks(ctx, documentVersionID)
	if err != nil {
		return nil, err
	}
	items := make([]DocumentLink, len(rows))
	for i, row := range rows {
		items[i] = DocumentLink{
			ID:                row.ID,
			CompanyID:         row.CompanyID,
			DocumentVersionID: row.DocumentVersionID,
			TargetModule:      row.TargetModule,
			TargetID:          row.TargetID,
			TargetCompanyID:   row.TargetCompanyID,
			LinkType:          row.LinkType,
			Description:       row.Description.String,
			CreatedBy:         row.CreatedBy,
			CreatedAt:         row.CreatedAt.Time,
		}
	}
	return items, nil
}

// Review Steps

func (r *Repository) GetReviewStepsForDocument(ctx context.Context, documentID int64) ([]DocumentReviewStep, error) {
	rows, err := r.queries.GetDocumentReviewStepsForDocument(ctx, documentID)
	if err != nil {
		return nil, err
	}
	items := make([]DocumentReviewStep, len(rows))
	for i, row := range rows {
		var dueAt *time.Time
		if row.DueAt.Valid {
			dueAt = &row.DueAt.Time
		}
		items[i] = DocumentReviewStep{
			ID:                row.ID,
			CompanyID:         row.CompanyID,
			DocumentVersionID: row.DocumentVersionID,
			StepOrder:         int(row.StepOrder),
			Name:              row.Name,
			ReviewerRoleID:    int64Ptr(row.ReviewerRoleID),
			ReviewerUserID:    int64Ptr(row.ReviewerUserID),
			RequiredApprovals: int(row.RequiredApprovals),
			Status:            row.Status,
			DueAt:             dueAt,
			CreatedAt:         row.CreatedAt.Time,
		}
	}
	return items, nil
}

func (r *Repository) InsertReviewStep(ctx context.Context, step DocumentReviewStep) error {
	var reviewerRoleID, reviewerUserID pgtype.Int8
	if step.ReviewerRoleID != nil {
		reviewerRoleID = pgtype.Int8{Int64: *step.ReviewerRoleID, Valid: true}
	}
	if step.ReviewerUserID != nil {
		reviewerUserID = pgtype.Int8{Int64: *step.ReviewerUserID, Valid: true}
	}
	var dueAt pgtype.Timestamptz
	if step.DueAt != nil {
		dueAt = pgtype.Timestamptz{Time: *step.DueAt, Valid: true}
	}
	return r.queries.InsertDocumentReviewStep(ctx, sqlc.InsertDocumentReviewStepParams{
		CompanyID:         step.CompanyID,
		DocumentVersionID: step.DocumentVersionID,
		StepOrder:         int32(step.StepOrder),
		Name:              step.Name,
		ReviewerRoleID:    reviewerRoleID,
		ReviewerUserID:    reviewerUserID,
		RequiredApprovals: int32(step.RequiredApprovals),
		Status:            step.Status,
		DueAt:             dueAt,
	})
}

func (r *Repository) UpdateReviewStepStatus(ctx context.Context, stepID int64, decision string, decidedBy int64) error {
	return r.queries.UpdateReviewStepStatus(ctx, sqlc.UpdateReviewStepStatusParams{
		ID:     stepID,
		Status: decision,
	})
}

func (r *Repository) AreAllReviewStepsApproved(ctx context.Context, documentVersionID int64) (bool, error) {
	return r.queries.AreAllReviewStepsApproved(ctx, documentVersionID)
}

// Review Decisions

func (r *Repository) InsertReviewDecision(ctx context.Context, req ReviewDecisionRequest) (DocumentReviewDecision, error) {
	id, err := r.queries.InsertReviewDecision(ctx, sqlc.InsertReviewDecisionParams{
		CompanyID:    req.CompanyID,
		ReviewStepID: req.StepID,
		ReviewerID:   req.ReviewerID,
		Decision:     req.Decision,
		Comment:      pgtype.Text{String: req.Comments, Valid: req.Comments != ""},
	})
	if err != nil {
		return DocumentReviewDecision{}, err
	}
	return DocumentReviewDecision{
		ID:                id,
		DocumentVersionID: req.DocumentVersionID,
		StepID:            req.StepID,
		ReviewerID:        req.ReviewerID,
		Decision:          req.Decision,
		Comments:          req.Comments,
		CreatedAt:         time.Now(),
	}, nil
}

// Legal Holds

func (r *Repository) InsertLegalHold(ctx context.Context, req CreateLegalHoldRequest) (LegalHold, error) {
	id, err := r.queries.InsertLegalHold(ctx, sqlc.InsertLegalHoldParams{
		CompanyID:   req.CompanyID,
		Name:        req.Name,
		Description: pgtype.Text{String: req.Description, Valid: req.Description != ""},
		ScopeType:   req.ScopeType,
		ScopeID:     derefInt64(req.ScopeID),
		IssuedBy:    req.InitiatedBy,
	})
	if err != nil {
		return LegalHold{}, err
	}
	return LegalHold{
		ID:          id,
		CompanyID:   req.CompanyID,
		Name:        req.Name,
		Description: req.Description,
		ScopeType:   req.ScopeType,
		ScopeID:     req.ScopeID,
		Status:      "ACTIVE",
		InitiatedBy: req.InitiatedBy,
		InitiatedAt: time.Now(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}, nil
}

func (r *Repository) ReleaseLegalHold(ctx context.Context, holdID, releasedBy int64) error {
	return r.queries.ReleaseLegalHold(ctx, sqlc.ReleaseLegalHoldParams{
		ID:         holdID,
		ReleasedBy: pgtype.Int8{Int64: releasedBy, Valid: true},
	})
}

// Access Events

func (r *Repository) RecordAccessEvent(ctx context.Context, event DocumentAccessEvent) error {
	addr, _ := netip.ParseAddr(event.IPAddress)
	return r.queries.InsertDocumentAccessEvent(ctx, sqlc.InsertDocumentAccessEventParams{
		CompanyID:         event.CompanyID,
		DocumentVersionID: event.DocumentVersionID,
		ActorID:           event.ActorID,
		Action:            event.Action,
		Column5:           addr,
		UserAgent:         pgtype.Text{String: event.UserAgent, Valid: event.UserAgent != ""},
	})
}

// Helpers

var _ = statusPtr
var _ = stringPtr
var _ = categoryIDOrZero
var _ = classificationIDOrZero
var _ = ownerIDOrZero

func int8Ptr(i *int64) pgtype.Int8 {
	if i == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *i, Valid: true}
}

func statusPtr(s *Status) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: string(*s), Valid: true}
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func int64Ptr(i pgtype.Int8) *int64 {
	if !i.Valid {
		return nil
	}
	v := i.Int64
	return &v
}

func valueOrZero(i *int64) int64 {
	if i == nil {
		return 0
	}
	return *i
}

func categoryIDOrZero(i int64) int64 { return i }

func classificationIDOrZero(i int64) int64 { return i }

func ownerIDOrZero(i int64) int64 { return i }

func statusStringOrEmpty(s *Status) string {
	if s == nil {
		return ""
	}
	return string(*s)
}

func derefInt64(i *int64) int64 {
	if i == nil {
		return 0
	}
	return *i
}

// Signatures

func (r *Repository) InsertSignatureChallenge(ctx context.Context, companyID, versionID, signerID int64, expiry time.Time) (string, error) {
	var expiryTs pgtype.Timestamptz
	expiryTs.Time = expiry
	expiryTs.Valid = true

	challengeID, err := r.queries.InsertDocumentSignatureChallenge(ctx, sqlc.InsertDocumentSignatureChallengeParams{
		CompanyID:         companyID,
		DocumentVersionID: versionID,
		SignerID:          signerID,
		Expiry:            expiryTs,
	})
	if err != nil {
		return "", err
	}

	var uuidStr [16]byte = challengeID.Bytes
	// very hacky uuid to string
	return string(uuidStr[:]), nil
}

func (r *Repository) GetSignatureChallenge(ctx context.Context, challengeID string) (DocumentSignatureChallenge, error) {
	// Not implementing uuid parse here, we just pass string
	// But sqlc uses pgtype.UUID
	var pgUUID pgtype.UUID
	if err := pgUUID.Scan(challengeID); err != nil {
		return DocumentSignatureChallenge{}, err
	}

	row, err := r.queries.GetDocumentSignatureChallenge(ctx, pgUUID)
	if err != nil {
		return DocumentSignatureChallenge{}, err
	}
	return DocumentSignatureChallenge{
		ChallengeID:       challengeID,
		CompanyID:         row.CompanyID,
		DocumentVersionID: row.DocumentVersionID,
		SignerID:          row.SignerID,
		Expiry:            row.Expiry.Time,
		CreatedAt:         row.CreatedAt.Time,
	}, nil
}

func (r *Repository) InsertSignature(ctx context.Context, req SignDocumentRequest, recordVersion string, recordHash string, policyVersion int, authMethod string) (DocumentSignature, error) {
	var pgUUID pgtype.UUID
	if err := pgUUID.Scan(req.ChallengeID); err != nil {
		return DocumentSignature{}, err
	}

	row, err := r.queries.InsertDocumentSignature(ctx, sqlc.InsertDocumentSignatureParams{
		CompanyID:         req.CompanyID,
		DocumentVersionID: req.DocumentVersionID,
		ChallengeID:       pgUUID,
		SignerID:          req.SignerID,
		RecordVersion:     recordVersion,
		RecordHash:        recordHash,
		Meaning:           req.Meaning,
		PolicyVersion:     int32(policyVersion),
		AuthMethod:        authMethod,
	})
	if err != nil {
		return DocumentSignature{}, err
	}

	return DocumentSignature{
		ID:                row.ID,
		CompanyID:         row.CompanyID,
		DocumentVersionID: row.DocumentVersionID,
		ChallengeID:       req.ChallengeID,
		SignerID:          row.SignerID,
		RecordVersion:     row.RecordVersion,
		RecordHash:        row.RecordHash,
		Meaning:           row.Meaning,
		PolicyVersion:     int(row.PolicyVersion),
		AuthMethod:        row.AuthMethod,
		SignedAt:          row.SignedAt.Time,
	}, nil
}

// Retention

func (r *Repository) InsertRetention(ctx context.Context, companyID, versionID, policyID int64, triggerDate, expiryDate time.Time) error {
	var trigger, expiry pgtype.Timestamptz
	trigger.Time = triggerDate
	trigger.Valid = true
	expiry.Time = expiryDate
	expiry.Valid = true
	return r.queries.InsertDocumentRetention(ctx, sqlc.InsertDocumentRetentionParams{
		CompanyID:         companyID,
		DocumentVersionID: versionID,
		PolicyID:          policyID,
		TriggerDate:       trigger,
		ExpiryDate:        expiry,
	})
}

func (r *Repository) DeleteRetention(ctx context.Context, id, companyID int64) error {
	return r.queries.DeleteDocumentRetention(ctx, sqlc.DeleteDocumentRetentionParams{
		ID:        id,
		CompanyID: companyID,
	})
}

// =============================================================================
// Advanced Features (OCR, Collaboration, Search)
// =============================================================================

func (r *Repository) CreateOCRJob(ctx context.Context, job DocumentOCRJob) (int64, error) {
	return r.queries.CreateDocumentOCRJob(ctx, sqlc.CreateDocumentOCRJobParams{
		CompanyID:         job.CompanyID,
		DocumentVersionID: job.DocumentVersionID,
		BlobID:            job.BlobID,
		Status:            job.Status,
	})
}

func (r *Repository) UpdateOCRJob(ctx context.Context, id int64, status, text, errMsg string, completedAt *time.Time) error {
	var extracted pgtype.Text
	if text != "" {
		extracted = pgtype.Text{String: text, Valid: true}
	}
	var errorMsg pgtype.Text
	if errMsg != "" {
		errorMsg = pgtype.Text{String: errMsg, Valid: true}
	}
	var compAt pgtype.Timestamptz
	if completedAt != nil {
		compAt = pgtype.Timestamptz{Time: *completedAt, Valid: true}
	}

	return r.queries.UpdateDocumentOCRJob(ctx, sqlc.UpdateDocumentOCRJobParams{
		ID:            id,
		Status:        status,
		ExtractedText: extracted,
		ErrorMessage:  errorMsg,
		CompletedAt:   compAt,
	})
}

func (r *Repository) CreateCollaborationSession(ctx context.Context, session DocumentCollaborationSession) (int64, error) {
	return r.queries.CreateCollaborationSession(ctx, sqlc.CreateCollaborationSessionParams{
		CompanyID:         session.CompanyID,
		DocumentVersionID: session.DocumentVersionID,
		SessionToken:      session.SessionToken,
		HostUserID:        session.HostUserID,
		Active:            session.Active,
		ExpiresAt:         pgtype.Timestamptz{Time: session.ExpiresAt, Valid: true},
	})
}

func (r *Repository) IndexDocumentSearch(ctx context.Context, docID, docVersionID int64, title, content, keywords string) (int64, error) {
	var kws pgtype.Text
	if keywords != "" {
		kws = pgtype.Text{String: keywords, Valid: true}
	}
	return r.queries.IndexDocumentSearch(ctx, sqlc.IndexDocumentSearchParams{
		DocumentID:        docID,
		DocumentVersionID: docVersionID,
		Title:             title,
		Content:           content,
		Keywords:          kws,
	})
}

func (r *Repository) SearchDocumentsFullText(ctx context.Context, companyID int64, query string, limit int32) ([]Document, error) {
	rows, err := r.queries.SearchDocumentsFullText(ctx, sqlc.SearchDocumentsFullTextParams{
		CompanyID:      companyID,
		PlaintoTsquery: query,
		Limit:          limit,
	})
	if err != nil {
		return nil, err
	}
	var results []Document
	for _, row := range rows {
		catID := int64(0)
		if row.CategoryID.Valid {
			catID = row.CategoryID.Int64
		}

		results = append(results, Document{
			ID:               row.ID,
			CompanyID:        row.CompanyID,
			Number:           row.DocumentNumber,
			Title:            row.Title,
			Description:      row.Description.String,
			CategoryID:       catID,
			ClassificationID: row.ClassificationID,
			OwnerID:          row.OwnerID,
			Status:           Status(row.Status),
			CreatedAt:        row.CreatedAt.Time,
			UpdatedAt:        row.UpdatedAt.Time,
		})
	}
	return results, nil
}
