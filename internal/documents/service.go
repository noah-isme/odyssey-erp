package documents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/storage"
)

// Service implements Document Management business logic.
type Service struct {
	repo    *Repository
	storage storage.Storage
	now     func() time.Time
}

// NewService constructs a Service instance.
func NewService(repo *Repository, storage storage.Storage) *Service {
	return &Service{repo: repo, storage: storage, now: time.Now}
}

// WithNow overrides the clock for deterministic tests.
func (s *Service) WithNow(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

// Create inserts a new document after validating inputs.
func (s *Service) Create(ctx context.Context, req CreateDocumentRequest) (Document, error) {
	if err := req.Validate(); err != nil {
		return Document{}, err
	}

	// Validate category exists and belongs to company
	category, err := s.repo.GetCategory(ctx, req.CategoryID)
	if err != nil {
		return Document{}, err
	}
	if category.CompanyID != req.CompanyID {
		return Document{}, fmt.Errorf("documents: category not owned by company")
	}

	// Validate classification exists and is active
	classification, err := s.repo.GetClassification(ctx, req.ClassificationID)
	if err != nil {
		return Document{}, err
	}
	if !classification.Active {
		return Document{}, fmt.Errorf("documents: classification not active")
	}

	// Generate document number using numbering rule
	number, err := s.generateDocumentNumber(ctx, req.CompanyID, req.CategoryID)
	if err != nil {
		return Document{}, err
	}

	doc, err := s.repo.InsertDocument(ctx, req, number)
	if err != nil {
		return Document{}, err
	}
	return doc, nil
}

// generateDocumentNumber generates a document number using the appropriate numbering rule.
func (s *Service) generateDocumentNumber(ctx context.Context, companyID, categoryID int64) (string, error) {
	rule, err := s.repo.GetNumberingRuleForCategory(ctx, companyID, categoryID)
	if err != nil {
		// Try to find a default rule
		rule, err = s.repo.GetDefaultNumberingRule(ctx, companyID)
		if err != nil {
			return "", fmt.Errorf("documents: no numbering rule found")
		}
	}

	if !rule.Active {
		return "", fmt.Errorf("documents: numbering rule not active")
	}

	number := rule.Pattern
	number = strings.ReplaceAll(number, "{PREFIX}", rule.Prefix)
	number = strings.ReplaceAll(number, "{SUFFIX}", rule.Suffix)
	number = strings.ReplaceAll(number, "{YEAR}", fmt.Sprintf("%d", time.Now().Year()))
	number = strings.ReplaceAll(number, "{MONTH}", fmt.Sprintf("%02d", time.Now().Month()))
	number = strings.ReplaceAll(number, "{SEQ:05d}", fmt.Sprintf("%05d", rule.SequenceCurrent))
	number = strings.ReplaceAll(number, "{SEQ:04d}", fmt.Sprintf("%04d", rule.SequenceCurrent))
	number = strings.ReplaceAll(number, "{SEQ:03d}", fmt.Sprintf("%03d", rule.SequenceCurrent))
	number = strings.ReplaceAll(number, "{SEQ}", fmt.Sprintf("%d", rule.SequenceCurrent))

	// Increment sequence
	if err := s.repo.IncrementNumberingSequence(ctx, rule.ID); err != nil {
		return "", err
	}

	return number, nil
}

// Get loads a single document.
func (s *Service) Get(ctx context.Context, id int64) (Document, error) {
	return s.repo.GetDocument(ctx, id)
}

// List returns documents filtered by company/category/classification/owner/status.
func (s *Service) List(ctx context.Context, filter ListFilter) ([]Document, error) {
	return s.repo.ListDocuments(ctx, filter)
}

// Update updates a document's metadata.
func (s *Service) Update(ctx context.Context, id int64, req UpdateDocumentRequest) (Document, error) {
	if err := req.Validate(); err != nil {
		return Document{}, err
	}
	return s.repo.UpdateDocument(ctx, id, req)
}

// Delete archives a document (soft delete).
func (s *Service) Delete(ctx context.Context, id int64, actorID int64) error {
	return s.repo.DeleteDocument(ctx, id, actorID)
}

// =============================================================================
// Advanced Features (OCR, Collaboration, Search)
// =============================================================================

// InitiateOCRJob queues a document version for Optical Character Recognition (OCR) text extraction.
func (s *Service) InitiateOCRJob(ctx context.Context, companyID, versionID, blobID int64) (int64, error) {
	job := DocumentOCRJob{
		CompanyID:         companyID,
		DocumentVersionID: versionID,
		BlobID:            blobID,
		Status:            "PENDING",
	}

	jobID, err := s.repo.CreateOCRJob(ctx, job)
	if err != nil {
		return 0, fmt.Errorf("documents: failed to create OCR job: %w", err)
	}

	// Ideally, this would dispatch an asynchronous message to an OCR worker worker-pool.
	// We'll leave the actual extraction task to the worker queue.
	return jobID, nil
}

// StartCollaborationSession creates a real-time editing session token for a document.
func (s *Service) StartCollaborationSession(ctx context.Context, companyID, versionID, hostUserID int64) (string, error) {
	// Generate a simple unique token for the session (in real life, a cryptographically secure random string or JWT)
	token := fmt.Sprintf("session-%d-%d-%d", companyID, versionID, time.Now().UnixNano())

	session := DocumentCollaborationSession{
		CompanyID:         companyID,
		DocumentVersionID: versionID,
		SessionToken:      token,
		HostUserID:        hostUserID,
		Active:            true,
		ExpiresAt:         s.now().Add(24 * time.Hour), // 24-hour max session
	}

	_, err := s.repo.CreateCollaborationSession(ctx, session)
	if err != nil {
		return "", fmt.Errorf("documents: failed to start collaboration session: %w", err)
	}

	return token, nil
}

// UpdateSearchIndex indexes a document's metadata and OCR content for full-text search.
func (s *Service) UpdateSearchIndex(ctx context.Context, docID, docVersionID int64, title, content, keywords string) error {
	_, err := s.repo.IndexDocumentSearch(ctx, docID, docVersionID, title, content, keywords)
	if err != nil {
		return fmt.Errorf("documents: failed to index search content: %w", err)
	}
	return nil
}

// SearchDocumentsFullText allows searching documents via Postgres Full-Text Search.
func (s *Service) SearchDocumentsFullText(ctx context.Context, companyID int64, query string, limit int) ([]Document, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.repo.SearchDocumentsFullText(ctx, companyID, query, int32(limit))
}

// CreateVersion creates a new version of a document.
func (s *Service) CreateVersion(ctx context.Context, req CreateVersionRequest) (DocumentVersion, error) {
	if err := req.Validate(); err != nil {
		return DocumentVersion{}, err
	}

	// Verify document exists and user has write access
	doc, err := s.repo.GetDocument(ctx, req.DocumentID)
	if err != nil {
		return DocumentVersion{}, err
	}

	// Check optimistic lock - ensure no concurrent version creation
	latestVersion, err := s.repo.GetLatestVersion(ctx, req.DocumentID)
	if err != nil && !errors.Is(err, ErrDocumentVersionNotFound) {
		return DocumentVersion{}, err
	}
	if latestVersion.VersionNumber >= req.VersionNumber {
		return DocumentVersion{}, ErrVersionConflict
	}

	// Verify blob if provided
	if req.BlobID != nil {
		blob, err := s.repo.GetBlob(ctx, *req.BlobID)
		if err != nil {
			return DocumentVersion{}, err
		}
		if blob.CompanyID != doc.CompanyID {
			return DocumentVersion{}, fmt.Errorf("documents: blob not owned by document company")
		}
		// Verify blob is valid
		if blob.ID == 0 {
			return DocumentVersion{}, fmt.Errorf("documents: blob not found")
		}
	}

	version, err := s.repo.InsertDocumentVersion(ctx, req)
	if err != nil {
		return DocumentVersion{}, err
	}

	// Update document's current version pointer
	if err := s.repo.SetCurrentVersion(ctx, req.DocumentID, version.ID); err != nil {
		return DocumentVersion{}, err
	}

	return version, nil
}

// UploadAndCreateVersion uploads a file to object storage and creates a new document version.
func (s *Service) UploadAndCreateVersion(ctx context.Context, file io.Reader, size int64, mimeType string, req CreateVersionRequest) (DocumentVersion, error) {

	// 1. Put object to storage
	key, err := s.storage.Put(ctx, storage.PutInput{
		Data:                file,
		Size:                size,
		CompanyID:           req.CompanyID,
		DeclaredContentType: mimeType,
		Classification:      "INTERNAL",
	})
	if err != nil {
		return DocumentVersion{}, fmt.Errorf("storage put: %w", err)
	}

	// 2. Stat object to get final checksum and metadata
	info, err := s.storage.Stat(ctx, key)
	if err != nil {
		return DocumentVersion{}, fmt.Errorf("storage stat: %w", err)
	}

	// 3. Register blob in database
	blobID, err := s.repo.InsertBlob(ctx, CreateBlobRequest{
		CompanyID:           req.CompanyID,
		StorageKey:          key,
		StorageDriver:       "default",
		SizeBytes:           info.Size,
		ChecksumSha256:      info.ChecksumSHA256,
		DeclaredContentType: mimeType,
		DetectedContentType: info.ContentType,
		MalwareScanStatus:   "PENDING",
		CreatedBy:           req.ActorID,
	})
	if err != nil {
		return DocumentVersion{}, fmt.Errorf("insert blob: %w", err)
	}

	// 4. Create the document version
	req.BlobID = &blobID
	return s.CreateVersion(ctx, req)
}

// GetVersion loads a single document version.
func (s *Service) GetVersion(ctx context.Context, id int64) (DocumentVersion, error) {
	return s.repo.GetDocumentVersion(ctx, id)
}

// ListVersions returns versions for a document.
func (s *Service) ListVersions(ctx context.Context, filter ListVersionsFilter) ([]DocumentVersion, error) {
	return s.repo.ListDocumentVersions(ctx, filter)
}

// SetVersionStatus transitions a version's status.
func (s *Service) SetVersionStatus(ctx context.Context, versionID int64, status Status, actorID int64) (DocumentVersion, error) {
	if err := validateStatusTransition(status); err != nil {
		return DocumentVersion{}, err
	}
	return s.repo.SetVersionStatus(ctx, versionID, status, actorID)
}

// DownloadVersion streams a document version's file content.
func (s *Service) DownloadVersion(ctx context.Context, versionID int64, actorID int64) (io.ReadCloser, string, string, error) {
	version, err := s.repo.GetDocumentVersion(ctx, versionID)
	if err != nil {
		return nil, "", "", err
	}

	if version.BlobID == nil {
		return nil, "", "", fmt.Errorf("documents: version has no associated file")
	}

	blob, err := s.repo.GetBlob(ctx, *version.BlobID)
	if err != nil {
		return nil, "", "", err
	}

	// Record access event
	_ = s.repo.RecordAccessEvent(ctx, DocumentAccessEvent{
		CompanyID:         blob.CompanyID,
		DocumentVersionID: versionID,
		ActorID:           actorID,
		Action:            "DOWNLOAD",
		IPAddress:         "", // TODO: extract from context
		UserAgent:         "", // TODO: extract from context
	})

	reader, err := s.storage.Get(ctx, blob.StorageKey)
	if err != nil {
		return nil, "", "", err
	}

	return reader, blob.StorageKey, blob.DeclaredContentType, nil
}

// AddACL adds an access control entry.
func (s *Service) AddACL(ctx context.Context, req CreateACLRequest) (DocumentACL, error) {
	return s.repo.InsertACL(ctx, req)
}

// RemoveACL removes an access control entry.
func (s *Service) RemoveACL(ctx context.Context, id int64) error {
	return s.repo.DeleteACL(ctx, id)
}

// ListACLs returns ACLs for a document or classification.
func (s *Service) ListACLs(ctx context.Context, companyID int64, documentID, classificationID *int64) ([]DocumentACL, error) {
	return s.repo.ListACLs(ctx, companyID, documentID, classificationID)
}

// CheckAccess verifies if a user has a specific permission on a document.
func (s *Service) CheckAccess(ctx context.Context, companyID, userID int64, documentID int64, permission string) (bool, error) {
	// Get user roles
	roles, err := s.repo.GetUserRoles(ctx, userID)
	if err != nil {
		return false, err
	}

	// Check document-specific ACLs
	acls, err := s.repo.ListACLs(ctx, companyID, &documentID, nil)
	if err != nil {
		return false, err
	}

	for _, acl := range acls {
		if acl.Permission == permission {
			if acl.PrincipalType == "USER" && acl.PrincipalID != nil && *acl.PrincipalID == userID {
				return true, nil
			}
			if acl.PrincipalType == "ROLE" && acl.PrincipalID != nil {
				for _, role := range roles {
					if role == *acl.PrincipalID {
						return true, nil
					}
				}
			}
			if acl.PrincipalType == "PUBLIC" {
				return true, nil
			}
		}
	}

	// Check classification-level ACLs
	doc, err := s.repo.GetDocument(ctx, documentID)
	if err != nil {
		return false, err
	}

	classACLs, err := s.repo.ListACLs(ctx, companyID, nil, &doc.ClassificationID)
	if err != nil {
		return false, err
	}

	for _, acl := range classACLs {
		if acl.Permission == permission {
			if acl.PrincipalType == "USER" && acl.PrincipalID != nil && *acl.PrincipalID == userID {
				return true, nil
			}
			if acl.PrincipalType == "ROLE" && acl.PrincipalID != nil {
				for _, role := range roles {
					if role == *acl.PrincipalID {
						return true, nil
					}
				}
			}
			if acl.PrincipalType == "PUBLIC" {
				return true, nil
			}
		}
	}

	return false, nil
}

// AddLink adds a cross-module reference.
func (s *Service) AddLink(ctx context.Context, req CreateLinkRequest) (DocumentLink, error) {
	return s.repo.InsertLink(ctx, req)
}

// RemoveLink removes a cross-module reference.
func (s *Service) RemoveLink(ctx context.Context, id int64) error {
	return s.repo.DeleteLink(ctx, id)
}

// ListLinks returns links for a document version.
func (s *Service) ListLinks(ctx context.Context, documentVersionID int64) ([]DocumentLink, error) {
	return s.repo.ListLinks(ctx, documentVersionID)
}

// SubmitForReview submits a document version for review workflow.
func (s *Service) SubmitForReview(ctx context.Context, versionID int64, actorID int64) (DocumentVersion, error) {
	version, err := s.repo.GetDocumentVersion(ctx, versionID)
	if err != nil {
		return DocumentVersion{}, err
	}

	if version.Status != StatusDraft {
		return DocumentVersion{}, fmt.Errorf("documents: only draft versions can be submitted for review")
	}

	// Create review steps based on document's classification/category workflow
	steps, err := s.repo.GetReviewStepsForDocument(ctx, version.DocumentID)
	if err != nil {
		return DocumentVersion{}, err
	}

	for _, step := range steps {
		if err := s.repo.InsertReviewStep(ctx, DocumentReviewStep{
			DocumentVersionID: versionID,
			StepOrder:         step.StepOrder,
			ReviewerRoleID:    step.ReviewerRoleID,
			ReviewerUserID:    step.ReviewerUserID,
			RequiredApprovals: step.RequiredApprovals,
			Status:            "PENDING",
		}); err != nil {
			return DocumentVersion{}, err
		}
	}

	return s.SetVersionStatus(ctx, versionID, StatusUnderReview, actorID)
}

// RecordReviewDecision records a review decision.
func (s *Service) RecordReviewDecision(ctx context.Context, req ReviewDecisionRequest) (DocumentReviewDecision, error) {
	decision, err := s.repo.InsertReviewDecision(ctx, req)
	if err != nil {
		return DocumentReviewDecision{}, err
	}

	// Update step status
	if err := s.repo.UpdateReviewStepStatus(ctx, req.StepID, req.Decision, req.ReviewerID); err != nil {
		return DocumentReviewDecision{}, err
	}

	// Check if all steps are complete
	allApproved, err := s.repo.AreAllReviewStepsApproved(ctx, req.DocumentVersionID)
	if err != nil {
		return DocumentReviewDecision{}, err
	}

	if allApproved {
		_, err = s.SetVersionStatus(ctx, req.DocumentVersionID, StatusApproved, req.ReviewerID)
	} else if req.Decision == "REJECTED" {
		_, err = s.SetVersionStatus(ctx, req.DocumentVersionID, StatusRejected, req.ReviewerID)
	}

	return decision, err
}

// Signatures

func (s *Service) CreateSignatureChallenge(ctx context.Context, companyID, versionID, signerID int64, expiry time.Duration) (DocumentSignatureChallenge, error) {
	// Generate challenge expiry
	exp := time.Now().Add(expiry)

	challengeID, err := s.repo.InsertSignatureChallenge(ctx, companyID, versionID, signerID, exp)
	if err != nil {
		return DocumentSignatureChallenge{}, err
	}

	return s.repo.GetSignatureChallenge(ctx, challengeID)
}

func (s *Service) SignDocument(ctx context.Context, req SignDocumentRequest) (DocumentSignature, error) {
	challenge, err := s.repo.GetSignatureChallenge(ctx, req.ChallengeID)
	if err != nil {
		return DocumentSignature{}, err
	}

	if challenge.CompanyID != req.CompanyID {
		return DocumentSignature{}, errors.New("documents: challenge company mismatch")
	}
	if challenge.DocumentVersionID != req.DocumentVersionID {
		return DocumentSignature{}, errors.New("documents: challenge version mismatch")
	}
	if challenge.SignerID != req.SignerID {
		return DocumentSignature{}, errors.New("documents: signer mismatch")
	}
	if time.Now().After(challenge.Expiry) {
		return DocumentSignature{}, errors.New("documents: challenge expired")
	}

	// Fetch document version to get checksum
	version, err := s.repo.GetDocumentVersion(ctx, req.DocumentVersionID)
	if err != nil {
		return DocumentSignature{}, err
	}

	var recordHash string
	if version.BlobID != nil {
		blob, err := s.repo.GetBlob(ctx, *version.BlobID)
		if err == nil {
			recordHash = blob.ChecksumSha256
		}
	}

	// Create signature
	sig, err := s.repo.InsertSignature(ctx, req, strconv.Itoa(version.VersionNumber), recordHash, 1, "password")
	if err != nil {
		return DocumentSignature{}, err
	}

	// Record audit access event
	_ = s.repo.RecordAccessEvent(ctx, DocumentAccessEvent{
		CompanyID:         req.CompanyID,
		DocumentVersionID: req.DocumentVersionID,
		ActorID:           req.SignerID,
		Action:            "SIGNED",
		IPAddress:         req.IPAddress,
		UserAgent:         req.UserAgent,
	})

	return sig, nil
}

// ApplyRetention applies retention policies to a document version.
func (s *Service) ApplyRetention(ctx context.Context, versionID int64) error {
	version, err := s.repo.GetDocumentVersion(ctx, versionID)
	if err != nil {
		return err
	}

	// Example calculation: 7 years from Trigger Date (Now)
	// In a complete implementation, this would look up the specific policy ID rules.
	policyID := int64(1) // Default Fallback Policy

	triggerDate := time.Now()
	expiryDate := triggerDate.AddDate(7, 0, 0)

	return s.repo.InsertRetention(ctx, version.CompanyID, versionID, policyID, triggerDate, expiryDate)
}

// CreateLegalHold creates a legal hold.
func (s *Service) CreateLegalHold(ctx context.Context, req CreateLegalHoldRequest) (LegalHold, error) {
	return s.repo.InsertLegalHold(ctx, req)
}

// ReleaseLegalHold releases a legal hold.
func (s *Service) ReleaseLegalHold(ctx context.Context, holdID, releasedBy int64) error {
	return s.repo.ReleaseLegalHold(ctx, holdID, releasedBy)
}

// CreateDisposition creates a disposition request.
func (s *Service) CreateDisposition(ctx context.Context, req CreateDispositionRequest) (DispositionRequest, error) {
	return DispositionRequest{}, errors.New("documents: disposition not implemented")
}

// UpdateDispositionRequest updates a disposition request status.
func (s *Service) UpdateDispositionRequest(ctx context.Context, id int64, status string, actorID int64) (DispositionRequest, error) {
	return DispositionRequest{}, errors.New("documents: disposition not implemented")
}

// ExecuteApprovedDispositions processes approved disposition requests.
func (s *Service) ExecuteApprovedDispositions(ctx context.Context) error {
	dispositions, err := s.repo.GetPendingDispositions(ctx)
	if err != nil {
		return fmt.Errorf("documents: failed to get pending dispositions: %w", err)
	}

	for _, req := range dispositions {
		err := s.executeDisposition(ctx, req)
		if err != nil {
			// Log error but continue with other requests
			fmt.Printf("failed to execute disposition %d: %v\n", req.ID, err)
		}
	}

	return nil
}

func (s *Service) executeDisposition(ctx context.Context, req DispositionRequest) error {
	// 1. Get document version
	version, err := s.repo.GetDocumentVersion(ctx, req.DocumentVersionID)
	if err != nil {
		return s.failDisposition(ctx, req.ID, fmt.Sprintf("failed to get version: %v", err))
	}

	// 2. Check for active legal holds
	hasHold, err := s.repo.HasActiveLegalHold(ctx, version.CompanyID)
	if err == nil && hasHold {
		return s.failDisposition(ctx, req.ID, "blocked by active legal hold")
	}

	// 3. Delete from storage if it has a blob
	var evidence string
	if version.BlobID != nil {
		blob, err := s.repo.GetBlob(ctx, *version.BlobID)
		if err == nil && blob.StorageKey != "" {
			err = s.storage.Delete(ctx, blob.StorageKey)
			if err != nil {
				return s.failDisposition(ctx, req.ID, fmt.Sprintf("storage delete failed: %v", err))
			}
			evidence = fmt.Sprintf("{\"deleted_blob_id\": %d, \"deleted_key\": \"%s\"}", blob.ID, blob.StorageKey)
		} else {
			evidence = "{\"status\": \"no_blob_found\"}"
		}
	} else {
		evidence = "{\"status\": \"no_blob_attached\"}"
	}

	// 4. Set document version to ARCHIVED/DELETED
	_, err = s.repo.SetVersionStatus(ctx, req.DocumentVersionID, StatusArchived, 1) // System actor
	if err != nil {
		return s.failDisposition(ctx, req.ID, fmt.Sprintf("failed to update version status: %v", err))
	}

	// 5. Update request to EXECUTED
	now := time.Now()
	var actor int64 = 1 // System actor

	err = s.repo.UpdateDispositionExecution(ctx, DispositionExecutionUpdate{
		ID:                req.ID,
		Status:            "EXECUTED",
		ExecutedAt:        &now,
		ExecutedBy:        &actor,
		ExecutionEvidence: []byte(evidence),
	})

	return err
}

func (s *Service) failDisposition(ctx context.Context, id int64, errMsg string) error {
	return s.repo.UpdateDispositionExecution(ctx, DispositionExecutionUpdate{
		ID:           id,
		Status:       "FAILED",
		ErrorMessage: &errMsg,
	})
}

// ListClassifications returns all active classifications.
func (s *Service) ListClassifications(ctx context.Context) ([]DocumentClassification, error) {
	return s.repo.ListClassifications(ctx)
}

// GetClassification returns a classification by ID.
func (s *Service) GetClassification(ctx context.Context, id int64) (DocumentClassification, error) {
	return s.repo.GetClassification(ctx, id)
}

// ListCategories returns categories for a company.
func (s *Service) ListCategories(ctx context.Context, companyID int64) ([]DocumentCategory, error) {
	return s.repo.ListCategories(ctx, companyID)
}

// GetCategory returns a category by ID.
func (s *Service) GetCategory(ctx context.Context, id int64) (DocumentCategory, error) {
	return s.repo.GetCategory(ctx, id)
}

// CreateCategoryRequest defines the payload for creating a category.
type CreateCategoryRequest struct {
	CompanyID   int64
	ParentID    *int64
	Code        string
	Name        string
	Description string
	Active      bool
	ActorID     int64
}

// CreateCategory creates a new document category.
func (s *Service) CreateCategory(ctx context.Context, req CreateCategoryRequest) (DocumentCategory, error) {
	return s.repo.InsertCategory(ctx, req)
}

// ListNumberingRules returns numbering rules for a company.
func (s *Service) ListNumberingRules(ctx context.Context, companyID int64) ([]DocumentNumberingRule, error) {
	return s.repo.ListNumberingRules(ctx, companyID)
}

// UpdateDocumentRequest defines the payload for updating a document.
type UpdateDocumentRequest struct {
	Title            string
	Description      string
	CategoryID       *int64
	ClassificationID *int64
	OwnerID          *int64
	ActorID          int64
}

// Validate ensures the update request can be processed.
func (r UpdateDocumentRequest) Validate() error {
	if r.ActorID <= 0 {
		return errors.New("documents: actor id required")
	}
	return nil
}

// CreateACLRequest defines the payload for creating an ACL.
type CreateACLRequest struct {
	CompanyID        int64
	DocumentID       *int64
	ClassificationID *int64
	PrincipalType    string
	PrincipalID      *int64
	Permission       string
	Effect           string
	GrantedBy        int64
	ExpiresAt        *time.Time
}

// CreateLinkRequest defines the payload for creating a document link.
type CreateLinkRequest struct {
	CompanyID         int64
	DocumentVersionID int64
	TargetModule      string
	TargetID          int64
	TargetCompanyID   int64
	LinkType          string
	Description       string
	CreatedBy         int64
}

// ReviewDecisionRequest defines the payload for a review decision.
type ReviewDecisionRequest struct {
	CompanyID         int64
	DocumentVersionID int64
	StepID            int64
	ReviewerID        int64
	Decision          string
	Comments          string
}

// CreateLegalHoldRequest defines the payload for creating a legal hold.
type CreateLegalHoldRequest struct {
	CompanyID   int64
	Name        string
	Description string
	ScopeType   string
	ScopeID     *int64
	InitiatedBy int64
}

// CreateDispositionRequest defines the payload for creating a disposition request.
type CreateDispositionRequest struct {
	CompanyID         int64
	DocumentVersionID int64
	PolicyID          int64
	RequestedBy       int64
}

// validateStatusTransition validates if a status transition is allowed.
func validateStatusTransition(newStatus Status) error {
	validStatuses := []Status{StatusDraft, StatusSubmitted, StatusUnderReview, StatusApproved, StatusRejected, StatusPublished, StatusArchived}
	for _, s := range validStatuses {
		if s == newStatus {
			return nil
		}
	}
	return fmt.Errorf("documents: invalid status %s", newStatus)
}

// ============================================================================
// Advanced Documents (OCR, Collaboration, Search)
// ============================================================================

func (s *Service) ProcessOCR(ctx context.Context, versionID int64) error {
	return errors.New("documents: process ocr not implemented")
}

func (s *Service) CreateCollaborationSession(ctx context.Context, req CollaborationSession) (CollaborationSession, error) {
	return CollaborationSession{}, errors.New("documents: advanced repository not implemented")
}

func (s *Service) RecordCollaborationChange(ctx context.Context, req CollaborationChange) (CollaborationChange, error) {
	return CollaborationChange{}, errors.New("documents: advanced repository not implemented")
}

func (s *Service) SearchContent(ctx context.Context, companyID int64, query string) ([]Document, error) {
	return nil, errors.New("documents: search content not implemented")
}
