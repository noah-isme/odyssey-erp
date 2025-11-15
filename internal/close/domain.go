package close

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// PeriodStatus enumerates accounting period lifecycle stages.
type PeriodStatus string

const (
	PeriodStatusOpen       PeriodStatus = "OPEN"
	PeriodStatusSoftClosed PeriodStatus = "SOFT_CLOSED"
	PeriodStatusHardClosed PeriodStatus = "HARD_CLOSED"
)

// RunStatus captures the lifecycle of a close run.
type RunStatus string

const (
	RunStatusDraft      RunStatus = "DRAFT"
	RunStatusInProgress RunStatus = "IN_PROGRESS"
	RunStatusCompleted  RunStatus = "COMPLETED"
	RunStatusCancelled  RunStatus = "CANCELLED"
)

// ChecklistStatus describes checklist progress.
type ChecklistStatus string

const (
	ChecklistStatusPending    ChecklistStatus = "PENDING"
	ChecklistStatusInProgress ChecklistStatus = "IN_PROGRESS"
	ChecklistStatusDone       ChecklistStatus = "DONE"
	ChecklistStatusSkipped    ChecklistStatus = "SKIPPED"
)

// Period encapsulates metadata for a fiscal period scoped to a company.
type Period struct {
	ID           int64
	PeriodID     int64
	CompanyID    int64
	Name         string
	StartDate    time.Time
	EndDate      time.Time
	Status       PeriodStatus
	SoftClosedBy *int64
	SoftClosedAt *time.Time
	ClosedBy     *int64
	ClosedAt     *time.Time
	Metadata     map[string]any
	LatestRunID  int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// CloseRun represents a single execution of a close checklist.
type CloseRun struct {
	ID          int64
	CompanyID   int64
	PeriodID    int64
	Status      RunStatus
	CreatedBy   int64
	CreatedAt   time.Time
	CompletedAt *time.Time
	Notes       string
	Checklist   []ChecklistItem
}

// ChecklistItem captures user-facing tasks required to complete a close run.
type ChecklistItem struct {
	ID          int64
	RunID       int64
	Code        string
	Label       string
	Status      ChecklistStatus
	AssignedTo  *int64
	CompletedAt *time.Time
	Comment     string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ChecklistDefinition describes seed checklist entries.
type ChecklistDefinition struct {
	Code  string
	Label string
}

// CreatePeriodInput captures validation rules for new periods.
type CreatePeriodInput struct {
	CompanyID int64
	Name      string
	StartDate time.Time
	EndDate   time.Time
	ActorID   int64
	Metadata  map[string]any
}

// Validate ensures the create period input is coherent.
func (in CreatePeriodInput) Validate() error {
	if in.CompanyID == 0 {
		return errors.New("close: company id required")
	}
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("close: name required")
	}
	if in.StartDate.IsZero() || in.EndDate.IsZero() {
		return errors.New("close: start and end date required")
	}
	if in.StartDate.After(in.EndDate) {
		return errors.New("close: start date cannot be after end date")
	}
	return nil
}

// StartCloseRunInput bundles parameters for opening a new close run.
type StartCloseRunInput struct {
	CompanyID int64
	PeriodID  int64
	ActorID   int64
	Notes     string
}

// ChecklistUpdateInput controls checklist status changes.
type ChecklistUpdateInput struct {
	ItemID  int64
	Status  ChecklistStatus
	ActorID int64
	Comment string
}

// ErrPeriodHardClosed is returned when writing to a hard closed period.
var ErrPeriodHardClosed = errors.New("close: period already hard closed")

// ErrChecklistLocked indicates updates are not permitted.
var ErrChecklistLocked = errors.New("close: checklist cannot be updated in current state")

// ErrRunNotFound indicates a close run could not be loaded.
var ErrRunNotFound = fmt.Errorf("close: run not found")

// ErrPeriodOverlap indicates the requested period conflicts with an existing range.
var ErrPeriodOverlap = errors.New("close: period overlaps existing range")

// ErrInvalidChecklistStatus indicates an unsupported transition.
var ErrInvalidChecklistStatus = errors.New("close: invalid checklist status")

// ErrChecklistIncomplete is returned when trying to hard close before completing the checklist.
var ErrChecklistIncomplete = errors.New("close: checklist not complete")

// ErrActiveRunExists indicates a run already exists for the period.
var ErrActiveRunExists = errors.New("close: close run already active for this period")
