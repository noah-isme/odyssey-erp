package boardpack

import (
	"errors"
	"strings"
	"time"
)

// Status captures the state of a board pack record.
type Status string

const (
	StatusPending    Status = "PENDING"
	StatusInProgress Status = "IN_PROGRESS"
	StatusReady      Status = "READY"
	StatusFailed     Status = "FAILED"
)

// TemplateSectionType enumerates supported section payloads.
type TemplateSectionType string

const (
	SectionExecSummary  TemplateSectionType = "EXEC_SUMMARY"
	SectionPLSummary    TemplateSectionType = "PL_SUMMARY"
	SectionBSSummary    TemplateSectionType = "BS_SUMMARY"
	SectionCashflow     TemplateSectionType = "CASHFLOW_SUMMARY"
	SectionTopVariances TemplateSectionType = "TOP_VARIANCES"
)

// Template describes the board pack configuration stored in the database.
type Template struct {
	ID          int64
	Name        string
	Description string
	Sections    []TemplateSection
	IsDefault   bool
	IsActive    bool
	CreatedBy   int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TemplateSection configures a single section in the board pack document.
type TemplateSection struct {
	Type    TemplateSectionType `json:"type"`
	Title   string              `json:"title"`
	Options map[string]any      `json:"options"`
}

// BoardPack represents a persisted generation request/result.
type BoardPack struct {
	ID                 int64
	CompanyID          int64
	CompanyName        string
	CompanyCode        string
	PeriodID           int64
	PeriodName         string
	PeriodStart        time.Time
	PeriodEnd          time.Time
	PeriodStatus       string
	TemplateID         int64
	TemplateName       string
	Template           *Template
	VarianceSnapshotID *int64
	Status             Status
	GeneratedAt        *time.Time
	GeneratedBy        *int64
	FilePath           string
	FileSize           *int64
	PageCount          *int
	ErrorMessage       string
	Metadata           map[string]any
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Company captures the limited metadata required by the builder/UI.
type Company struct {
	ID   int64
	Code string
	Name string
}

// Period summarises the accounting period metadata used by board packs.
type Period struct {
	ID        int64
	Name      string
	StartDate time.Time
	EndDate   time.Time
	Status    string
	CompanyID int64
}

// VarianceSnapshot holds metadata for optional variance section references.
type VarianceSnapshot struct {
	ID        int64
	RuleName  string
	PeriodID  int64
	CompanyID int64
	Status    string
}

// CreateRequest defines the payload accepted by the service when creating a board pack.
type CreateRequest struct {
	CompanyID          int64
	PeriodID           int64
	TemplateID         int64
	VarianceSnapshotID *int64
	ActorID            int64
	Metadata           map[string]any
}

// Validate ensures the creation request can be processed.
func (r CreateRequest) Validate() error {
	if r.CompanyID <= 0 {
		return errors.New("boardpack: company id required")
	}
	if r.PeriodID <= 0 {
		return errors.New("boardpack: period id required")
	}
	if r.TemplateID <= 0 {
		return errors.New("boardpack: template id required")
	}
	if r.ActorID <= 0 {
		return errors.New("boardpack: actor id required")
	}
	return nil
}

// ListFilter configures ListBoardPacks queries.
type ListFilter struct {
	CompanyID int64
	PeriodID  int64
	Status    Status
	Limit     int
	Offset    int
}

// ExecSummaryData composes the meta + KPI card for the board pack.
type ExecSummaryData struct {
	Company       Company
	Period        Period
	RequestedBy   *int64
	VarianceLabel string
	KPISummary    KPISummary
	Status        Status
}

// KPISummary contains selective key metrics surfaced in the exec summary.
type KPISummary struct {
	NetProfit     float64
	Revenue       float64
	Opex          float64
	COGS          float64
	CashIn        float64
	CashOut       float64
	AROutstanding float64
	APOutstanding float64
}

// CashflowSummary highlights the cash movement for the selected period.
type CashflowSummary struct {
	CashIn  float64
	CashOut float64
	Net     float64
}

// SectionData binds template sections to concrete payloads for rendering.
type SectionData struct {
	Type       TemplateSectionType
	Title      string
	Exec       *ExecSummaryData
	Cashflow   *CashflowSummary
	Payload    any
	Limit      int
	HasContent bool
}

// DocumentData is the final view model passed to the HTML template.
type DocumentData struct {
	Pack        BoardPack
	Company     Company
	Period      Period
	Template    Template
	Sections    []SectionData
	GeneratedAt time.Time
	Warnings    []string
}

// RenderResult captures the output of the renderer.
type RenderResult struct {
	HTML   string
	PDF    []byte
	Length int64
}

var (
	ErrBoardPackNotFound = errors.New("boardpack: pack not found")
	ErrTemplateNotFound  = errors.New("boardpack: template not found")
	ErrCompanyNotFound   = errors.New("boardpack: company not found")
	ErrPeriodNotFound    = errors.New("boardpack: period not found")
	ErrInvalidStatus     = errors.New("boardpack: invalid status transition")
)

// NormaliseStatus uppercases and trims the provided status string.
func NormaliseStatus(v string) Status {
	v = strings.TrimSpace(strings.ToUpper(v))
	switch Status(v) {
	case StatusPending, StatusInProgress, StatusReady, StatusFailed:
		return Status(v)
	default:
		return StatusPending
	}
}
