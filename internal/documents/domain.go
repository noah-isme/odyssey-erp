package documents

import (
	"errors"
	"strings"
	"time"
)

// Status captures the state of a document.
type Status string

const (
	StatusDraft       Status = "DRAFT"
	StatusSubmitted   Status = "SUBMITTED"
	StatusUnderReview Status = "UNDER_REVIEW"
	StatusApproved    Status = "APPROVED"
	StatusRejected    Status = "REJECTED"
	StatusPublished   Status = "PUBLISHED"
	StatusArchived    Status = "ARCHIVED"
)

// ClassificationLevel represents document sensitivity classification.
type ClassificationLevel string

const (
	ClassificationPublic       ClassificationLevel = "PUBLIC"
	ClassificationInternal     ClassificationLevel = "INTERNAL"
	ClassificationConfidential ClassificationLevel = "CONFIDENTIAL"
	ClassificationRestricted   ClassificationLevel = "RESTRICTED"
)

// DocumentClassification represents a document classification definition.
type DocumentClassification struct {
	ID                int64
	CompanyID         int64
	Code              string
	Name              string
	Description       string
	RequiresApproval  bool
	RequiresSignature bool
	Active            bool
	CreatedAt         time.Time
}

// DocumentCategory represents a document category for organization.
type DocumentCategory struct {
	ID                      int64
	CompanyID               int64
	ParentID                *int64
	Code                    string
	Name                    string
	Description             string
	DefaultClassificationID *int64
	Active                  bool
	CreatedAt               time.Time
}

// DocumentNumberingRule represents a document numbering rule.
type DocumentNumberingRule struct {
	ID              int64
	CompanyID       int64
	Code            string
	Name            string
	Prefix          string
	Suffix          string
	Pattern         string // e.g., "{PREFIX}-{YYYY}-{SEQ:04d}"
	SequenceCurrent int64
	Scope           string
	ScopeID         *int64
	Active          bool
	CreatedAt       time.Time
}

// Document represents a document header record.
type Document struct {
	ID                int64
	CompanyID         int64
	Number            string
	Title             string
	Description       string
	CategoryID        int64
	ClassificationID  int64
	OwnerID           int64
	Status            Status
	CurrentVersionID  *int64
	MigrationSource   *string
	MigrationSourceID *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	// Joined fields
	CategoryName       string
	ClassificationName string
	OwnerName          string
	CompanyName        string
}

// Blob represents a storage object registered for a document.
//
// This is deliberately storage/database neutral so document services do not
// depend on generated SQLC or pgtype values.
type Blob struct {
	ID                  int64
	CompanyID           int64
	StorageKey          string
	StorageDriver       string
	Bucket              string
	SizeBytes           int64
	ChecksumSha256      string
	DeclaredContentType string
	DetectedContentType string
	MalwareScanStatus   string
	CreatedAt           time.Time
	CreatedBy           int64
}

// CreateBlobRequest contains the values needed to register a storage object.
type CreateBlobRequest struct {
	CompanyID           int64
	StorageKey          string
	StorageDriver       string
	Bucket              string
	SizeBytes           int64
	ChecksumSha256      string
	DeclaredContentType string
	DetectedContentType string
	MalwareScanStatus   string
	CreatedBy           int64
}

// DocumentVersion represents a version of a document.
type DocumentVersion struct {
	ID                    int64
	CompanyID             int64
	DocumentID            int64
	VersionNumber         int
	VersionLabel          string
	BlobID                *int64
	Status                Status
	ClassificationID      int64
	ChangeSummary         string
	ApprovedBy            *int64
	ApprovedAt            *time.Time
	EffectiveAt           *time.Time
	SupersededAt          *time.Time
	SupersededByVersionID *int64
	CreatedBy             int64
	CreatedAt             time.Time
	// Joined fields
	DocumentNumber string
	DocumentTitle  string
	BlobStorageKey string
	CreatedByName  string
}

// DocumentACL represents an access control entry for a document.
type DocumentACL struct {
	ID               int64
	CompanyID        int64
	DocumentID       *int64
	ClassificationID *int64
	PrincipalType    string // USER, ROLE
	PrincipalID      *int64
	Permission       string // READ, WRITE, ADMIN, APPROVE, SIGN
	Effect           string // ALLOW, DENY
	GrantedBy        int64
	ExpiresAt        *time.Time
}

// DocumentLink represents a cross-module reference from a document version.
type DocumentLink struct {
	ID                int64
	CompanyID         int64
	DocumentVersionID int64
	TargetModule      string // e.g., "sales", "procurement", "cmms", "qms"
	TargetID          int64
	TargetCompanyID   int64
	LinkType          string // REFERENCE, ATTACHMENT, EVIDENCE
	Description       string
	CreatedBy         int64
	CreatedAt         time.Time
}

// DocumentReviewStep represents a step in a document review workflow.
type DocumentReviewStep struct {
	ID                int64
	CompanyID         int64
	DocumentVersionID int64
	StepOrder         int
	Name              string
	ReviewerRoleID    *int64
	ReviewerUserID    *int64
	RequiredApprovals int
	Status            string // PENDING, APPROVED, REJECTED, SKIPPED
	DueAt             *time.Time
	CreatedAt         time.Time
}

// DocumentReviewDecision represents a review decision record.
type DocumentReviewDecision struct {
	ID                int64
	DocumentVersionID int64
	StepID            int64
	ReviewerID        int64
	Decision          string // APPROVED, REJECTED
	Comments          string
	CreatedAt         time.Time
}

// DocumentSignatureChallenge represents a signature challenge for e-signatures.
type DocumentSignatureChallenge struct {
	ChallengeID       string
	CompanyID         int64
	DocumentVersionID int64
	SignerID          int64
	Expiry            time.Time
	CreatedAt         time.Time
}

// DocumentSignature represents an electronic signature on a document.
type DocumentSignature struct {
	ID                int64
	CompanyID         int64
	DocumentVersionID int64
	ChallengeID       string
	SignerID          int64
	RecordVersion     string
	RecordHash        string
	Meaning           string
	PolicyVersion     int
	AuthMethod        string
	SignedAt          time.Time
}

// SignDocumentRequest defines the payload for executing a signature.
type SignDocumentRequest struct {
	CompanyID         int64
	DocumentVersionID int64
	ChallengeID       string
	SignerID          int64
	Meaning           string
	IPAddress         string
	UserAgent         string
}

// RetentionPolicy represents a document retention policy.
type RetentionPolicy struct {
	ID                int64
	CompanyID         int64
	ClassificationID  *int64
	CategoryID        *int64
	Name              string
	Description       string
	RetentionDays     int
	DispositionAction string // DELETE, ARCHIVE, LEGAL_HOLD
	Active            bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// DocumentRetention represents a document's retention schedule.
type DocumentRetention struct {
	ID                int64
	CompanyID         int64
	DocumentVersionID int64
	PolicyID          int64
	TriggerDate       time.Time
	ExpiryDate        time.Time
	Status            string
	CalculatedAt      time.Time
}

// LegalHold represents a legal hold on documents.
type LegalHold struct {
	ID          int64
	CompanyID   int64
	Name        string
	Description string
	ScopeType   string // DOCUMENT, CATEGORY, CLASSIFICATION, ALL
	ScopeID     *int64
	Status      string // ACTIVE, RELEASED
	InitiatedBy int64
	InitiatedAt time.Time
	ReleasedBy  *int64
	ReleasedAt  *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Advanced Documents Features (Collaboration)

type CollaborationSession struct {
	ID        int64
	CompanyID int64
	VersionID int64
	Status    string // ACTIVE, CLOSED
	CreatedAt time.Time
}

type CollaborationChange struct {
	ID        int64
	SessionID int64
	ActorID   int64
	Operation string // INSERT, DELETE, REPLACE
	Payload   string
	Timestamp time.Time
}

// LegalHoldReference represents a document under legal hold.
type LegalHoldReference struct {
	ID                int64
	LegalHoldID       int64
	DocumentVersionID int64
	CreatedAt         time.Time
}

// DispositionRequest represents a request to dispose of documents.
type DispositionRequest struct {
	ID                int64
	CompanyID         int64
	DocumentVersionID int64
	RequestedBy       int64
	Status            string
}

// DispositionExecutionUpdate records the outcome of executing a disposition.
// Nil optional fields are persisted as SQL NULL by the repository.
type DispositionExecutionUpdate struct {
	ID                int64
	Status            string
	ExecutedAt        *time.Time
	ExecutedBy        *int64
	ExecutionEvidence []byte
	ErrorMessage      *string
}

// DocumentAccessEvent represents an audit event for document access.
type DocumentAccessEvent struct {
	ID                int64
	CompanyID         int64
	DocumentVersionID int64
	ActorID           int64
	Action            string // VIEW, DOWNLOAD, SHARE, PRINT
	ShareToken        *string
	IPAddress         string
	UserAgent         string
	CreatedAt         time.Time
}

// CreateDocumentRequest defines the payload for creating a document.
type CreateDocumentRequest struct {
	CompanyID        int64
	Title            string
	Description      string
	CategoryID       int64
	ClassificationID int64
	OwnerID          int64
	ActorID          int64
	Metadata         map[string]any
}

// Validate ensures the creation request can be processed.
func (r CreateDocumentRequest) Validate() error {
	if r.CompanyID <= 0 {
		return errors.New("documents: company id required")
	}
	if strings.TrimSpace(r.Title) == "" {
		return errors.New("documents: title required")
	}
	if r.CategoryID <= 0 {
		return errors.New("documents: category id required")
	}
	if r.ClassificationID <= 0 {
		return errors.New("documents: classification id required")
	}
	if r.OwnerID <= 0 {
		return errors.New("documents: owner id required")
	}
	if r.ActorID <= 0 {
		return errors.New("documents: actor id required")
	}
	return nil
}

// CreateVersionRequest defines the payload for creating a document version.
type CreateVersionRequest struct {
	DocumentID       int64
	CompanyID        int64
	VersionNumber    int
	Description      string // stored as change_summary
	BlobID           *int64
	ClassificationID int64
	ActorID          int64
}

// Validate ensures the version creation request can be processed.
func (r CreateVersionRequest) Validate() error {
	if r.DocumentID <= 0 {
		return errors.New("documents: document id required")
	}
	if r.VersionNumber <= 0 {
		return errors.New("documents: version number required")
	}
	if r.ActorID <= 0 {
		return errors.New("documents: actor id required")
	}
	return nil
}

// ListFilter configures ListDocuments queries.
type ListFilter struct {
	CompanyID        int64
	CategoryID       *int64
	ClassificationID *int64
	OwnerID          *int64
	Status           *Status
	Search           string
	Limit            int
	Offset           int
}

// ListVersionsFilter configures ListDocumentVersions queries.
type ListVersionsFilter struct {
	CompanyID  int64
	DocumentID int64
	Status     *Status
	Limit      int
	Offset     int
}

// DocumentOCRJob represents a background job to extract text from a document blob.
type DocumentOCRJob struct {
	ID                int64
	CompanyID         int64
	DocumentVersionID int64
	BlobID            int64
	Status            string // PENDING, PROCESSING, COMPLETED, FAILED
	ExtractedText     string
	ErrorMessage      string
	CreatedAt         time.Time
	CompletedAt       *time.Time
}

// DocumentCollaborationSession represents a real-time editing/viewing session.
type DocumentCollaborationSession struct {
	ID                int64
	CompanyID         int64
	DocumentVersionID int64
	SessionToken      string
	HostUserID        int64
	Active            bool
	CreatedAt         time.Time
	ExpiresAt         time.Time
}

// DocumentSearchIndex represents a full-text search index for a document.
type DocumentSearchIndex struct {
	ID                int64
	DocumentID        int64
	DocumentVersionID int64
	Title             string
	Content           string // Includes OCR extracted text + user inputs
	Keywords          string
	IndexedAt         time.Time
}

var (
	ErrDocumentNotFound        = errors.New("documents: document not found")
	ErrDocumentVersionNotFound = errors.New("documents: document version not found")
	ErrClassificationNotFound  = errors.New("documents: classification not found")
	ErrCategoryNotFound        = errors.New("documents: category not found")
	ErrNumberingRuleNotFound   = errors.New("documents: numbering rule not found")
	ErrInvalidStatus           = errors.New("documents: invalid status transition")
	ErrAccessDenied            = errors.New("documents: access denied")
	ErrVersionConflict         = errors.New("documents: version conflict (optimistic lock)")
)

// NormaliseStatus uppercases and trims the provided status string.
func NormaliseStatus(v string) Status {
	v = strings.TrimSpace(strings.ToUpper(v))
	switch Status(v) {
	case StatusDraft, StatusSubmitted, StatusUnderReview, StatusApproved, StatusRejected, StatusPublished, StatusArchived:
		return Status(v)
	default:
		return StatusDraft
	}
}
