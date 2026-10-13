// Package checklists manages onboarding and offboarding checklists: company
// templates, per-employee checklists with assigned tasks, the offboarding
// system actions run by CompleteOffboarding, and fixed-asset custody.
//
// Every query is company-scoped (child rows are reached through their
// company-owning parent) and every state change is audited with RecordTx
// inside the same transaction. Audit metadata carries ids, dates, kinds, and
// counts only, never profile, salary, or bank values.
package checklists

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/hr/employees"
	"github.com/odyssey-erp/odyssey-erp/internal/notifications"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// Checklist kinds (hr_checklist_templates.kind / hr_employee_checklists.kind).
const (
	KindOnboarding  = "ONBOARDING"
	KindOffboarding = "OFFBOARDING"
)

// Checklist statuses (hr_employee_checklists.status).
const (
	StatusOpen      = "OPEN"
	StatusCompleted = "COMPLETED"
	StatusCancelled = "CANCELLED"
)

// Task statuses (hr_employee_checklist_tasks.status).
const (
	TaskOpen    = "OPEN"
	TaskDone    = "DONE"
	TaskSkipped = "SKIPPED"
)

// Assignee roles (assignee_role).
const (
	RoleHR      = "HR"
	RoleIT      = "IT"
	RoleManager = "MANAGER"
)

// Action keys (action_key). They are informational markers on tasks; the
// offboarding system actions themselves run in CompleteOffboarding.
const (
	ActionKeyDeactivateUser = "DEACTIVATE_USER"
	ActionKeyReturnAssets   = "RETURN_ASSETS"
	ActionKeyCloseLeave     = "CLOSE_LEAVE"
	ActionKeyCloseBenefits  = "CLOSE_BENEFITS"
)

// NotificationTypeTaskAssigned is the notification type sent to each
// distinct assignee when a checklist starts.
const NotificationTypeTaskAssigned = "hr_checklist_task_assigned"

// Audit actions recorded by this package.
const (
	ActionTemplateCreated     = "CHECKLIST_TEMPLATE_CREATED"
	ActionTemplateUpdated     = "CHECKLIST_TEMPLATE_UPDATED"
	ActionTemplateItemAdded   = "CHECKLIST_TEMPLATE_ITEM_ADDED"
	ActionTemplateItemDeleted = "CHECKLIST_TEMPLATE_ITEM_DELETED"
	ActionChecklistStarted    = "CHECKLIST_STARTED"
	ActionTaskCompleted       = "CHECKLIST_TASK_COMPLETED"
	ActionTaskSkipped         = "CHECKLIST_TASK_SKIPPED"
	ActionChecklistCompleted  = "CHECKLIST_COMPLETED"
	ActionEmployeeOffboarded  = "EMPLOYEE_OFFBOARDED"
	ActionOffboardingCanceled = "OFFBOARDING_CANCELLED"
	ActionAssetAssigned       = "ASSET_ASSIGNED"
	ActionAssetReturned       = "ASSET_RETURNED"

	entityTemplate  = "hr_checklist_template"
	entityChecklist = "hr_employee_checklist"
	entityTask      = "hr_employee_checklist_task"
	entityEmployee  = "hr_employee"
	entityAsset     = "fixed_asset"
)

var (
	// ErrNotFound is returned (possibly wrapped) when an id does not exist in
	// the caller's company. Cross-tenant ids are indistinguishable from
	// missing ones.
	ErrNotFound          = errors.New("hr: not found")
	ErrTemplateNotFound  = fmt.Errorf("%w: checklist template", ErrNotFound)
	ErrItemNotFound      = fmt.Errorf("%w: checklist template item", ErrNotFound)
	ErrEmployeeNotFound  = fmt.Errorf("%w: employee", ErrNotFound)
	ErrChecklistNotFound = fmt.Errorf("%w: checklist", ErrNotFound)
	ErrTaskNotFound      = fmt.Errorf("%w: checklist task", ErrNotFound)
	ErrAssetNotFound     = fmt.Errorf("%w: asset", ErrNotFound)

	ErrCompanyRequired        = errors.New("hr: company required")
	ErrInvalidTemplate        = errors.New("hr: invalid checklist template")
	ErrInvalidKind            = errors.New("hr: checklist kind must be ONBOARDING or OFFBOARDING")
	ErrDuplicateCode          = errors.New("hr: checklist template code already exists")
	ErrInvalidItem            = errors.New("hr: invalid checklist item")
	ErrInvalidRole            = errors.New("hr: assignee role must be HR, IT, or MANAGER")
	ErrInvalidActionKey       = errors.New("hr: invalid checklist action key")
	ErrDuplicateSeq           = errors.New("hr: checklist item sequence already used")
	ErrAssigneeInactive       = errors.New("hr: default assignee must be an active user")
	ErrTemplateInactive       = errors.New("hr: checklist template is inactive")
	ErrTemplateEmpty          = errors.New("hr: checklist template has no items")
	ErrInvalidStart           = errors.New("hr: invalid checklist start")
	ErrTerminationRequired    = errors.New("hr: termination date is required for offboarding")
	ErrTerminationBeforeHire  = errors.New("hr: termination date is before the hire date")
	ErrEmployeeInactive       = errors.New("hr: employee is not active")
	ErrAlreadyOpen            = errors.New("hr: an open checklist of this kind already exists for the employee")
	ErrNotOpen                = errors.New("hr: checklist is not open")
	ErrTaskNotOpen            = errors.New("hr: task is not open")
	ErrForbidden              = errors.New("hr: only the assignee or a checklist admin can update this task")
	ErrWrongKind              = errors.New("hr: checklist kind does not match the action")
	ErrFutureTermination      = errors.New("hr: termination date is in the future")
	ErrJobHistoryAfterLeaving = errors.New("hr: job history has a change after the termination date; remove it first")
	ErrAssetHeldByOther       = errors.New("hr: asset is held by another employee; return it first")
	ErrAssetNotAssigned       = errors.New("hr: asset is not assigned")
	ErrAssetDisposed          = errors.New("hr: disposed assets cannot be assigned")
	errAuditNotConfigured     = errors.New("hr: checklist audit not configured")
	errEmployeesNotConfigured = errors.New("hr: checklist employee lookups not configured")
)

// Audit is satisfied by *shared.AuditLogger.
type Audit interface {
	RecordTx(context.Context, pgx.Tx, shared.AuditLog) error
}

// Notifier is satisfied by *notifications.Dispatcher.
type Notifier interface {
	Dispatch(context.Context, notifications.Message) error
}

// Employees is satisfied by *employees.Service (effective-dated job lookups).
type Employees interface {
	AssignmentAsOf(ctx context.Context, companyID, employeeID int64, asOf time.Time) (employees.JobHistory, error)
	DirectReportsAsOf(ctx context.Context, companyID, managerEmployeeID int64, asOf time.Time) ([]employees.JobHistory, error)
}

type db interface {
	Begin(context.Context) (pgx.Tx, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Service manages checklist templates, employee checklists, and asset
// custody.
type Service struct {
	pool      db
	audit     Audit
	employees Employees
	notifier  Notifier
	now       func() time.Time
}

// NewService builds a Service. notifier may be nil (no assignment
// notifications are sent then).
func NewService(pool *pgxpool.Pool, audit Audit, emp Employees, notifier Notifier) *Service {
	return &Service{pool: pool, audit: audit, employees: emp, notifier: notifier, now: time.Now}
}

func (s *Service) today() time.Time {
	if s.now == nil {
		return dateOnly(time.Now())
	}
	return dateOnly(s.now())
}

func (s *Service) inTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (s *Service) record(ctx context.Context, tx pgx.Tx, actorID int64, action, entity string, id int64, meta map[string]any) error {
	if s.audit == nil {
		return errAuditNotConfigured
	}
	if err := s.audit.RecordTx(ctx, tx, shared.AuditLog{ActorID: actorID, Action: action, Entity: entity, EntityID: strconv.FormatInt(id, 10), Meta: meta}); err != nil {
		return fmt.Errorf("audit %s: %w", action, err)
	}
	return nil
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func validKind(k string) bool { return k == KindOnboarding || k == KindOffboarding }

func validRole(r string) bool { return r == RoleHR || r == RoleIT || r == RoleManager }

func validActionKey(k string) bool {
	switch k {
	case ActionKeyDeactivateUser, ActionKeyReturnAssets, ActionKeyCloseLeave, ActionKeyCloseBenefits:
		return true
	}
	return false
}

func nullableID(id int64) *int64 {
	if id <= 0 {
		return nil
	}
	return &id
}
