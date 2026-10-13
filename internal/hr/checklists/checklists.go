package checklists

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/hr/employees"
	"github.com/odyssey-erp/odyssey-erp/internal/notifications"
)

// Checklist maps an hr_employee_checklists row plus display fields and,
// from Get, its tasks.
type Checklist struct {
	ID                int64
	CompanyID         int64
	EmployeeID        int64
	EmployeeName      string
	EmployeeNumber    string
	TemplateID        int64
	TemplateName      string
	Kind              string
	Status            string
	StartedOn         time.Time
	TerminationDate   *time.Time
	TerminationReason string
	CreatedBy         *int64
	CompletedBy       *int64
	CompletedAt       *time.Time
	CreatedAt         time.Time
	OpenTasks         int
	Tasks             []Task
}

// Task maps an hr_employee_checklist_tasks row plus display fields.
type Task struct {
	ID             int64
	CompanyID      int64
	ChecklistID    int64
	Seq            int
	Title          string
	Description    string
	AssigneeRole   string
	AssigneeUserID *int64
	AssigneeName   string
	DueDate        *time.Time
	Status         string
	ActionKey      string
	Notes          string
	CompletedBy    *int64
	CompletedAt    *time.Time
	// Set by MyTasks.
	EmployeeID   int64
	EmployeeName string
	Kind         string
}

// Overdue reports whether an OPEN task is past its due date on day.
func (t Task) Overdue(day time.Time) bool {
	return t.Status == TaskOpen && t.DueDate != nil && dateOnly(*t.DueDate).Before(dateOnly(day))
}

// StartInput starts a checklist from a template for an employee.
// TerminationDate is required for OFFBOARDING templates and ignored for
// ONBOARDING ones.
type StartInput struct {
	CompanyID         int64
	EmployeeID        int64
	TemplateID        int64
	ActorID           int64
	TerminationDate   *time.Time
	TerminationReason string
}

// Filter narrows List.
type Filter struct {
	Status     string
	Kind       string
	EmployeeID *int64
}

const checklistSelect = `SELECT c.id,c.company_id,c.employee_id,e.name,e.employee_number,c.template_id,t.name,c.kind,c.status,c.started_on,c.termination_date,c.termination_reason,c.created_by,c.completed_by,c.completed_at,c.created_at,
(SELECT COUNT(*) FROM hr_employee_checklist_tasks k WHERE k.checklist_id=c.id AND k.status='OPEN')
FROM hr_employee_checklists c
JOIN hr_employees e ON e.id=c.employee_id AND e.company_id=c.company_id
JOIN hr_checklist_templates t ON t.id=c.template_id AND t.company_id=c.company_id
WHERE c.company_id=$1`

func scanChecklist(row pgx.Row) (Checklist, error) {
	var c Checklist
	err := row.Scan(&c.ID, &c.CompanyID, &c.EmployeeID, &c.EmployeeName, &c.EmployeeNumber, &c.TemplateID, &c.TemplateName, &c.Kind, &c.Status, &c.StartedOn,
		&c.TerminationDate, &c.TerminationReason, &c.CreatedBy, &c.CompletedBy, &c.CompletedAt, &c.CreatedAt, &c.OpenTasks)
	return c, err
}

// taskSelect reaches tasks through their company-owning checklist.
const taskSelect = `SELECT k.id,c.company_id,k.checklist_id,k.seq,k.title,k.description,k.assignee_role,k.assignee_user_id,COALESCE(u.name,''),k.due_date,k.status,COALESCE(k.action_key,''),k.notes,k.completed_by,k.completed_at,
c.employee_id,e.name,c.kind
FROM hr_employee_checklist_tasks k
JOIN hr_employee_checklists c ON c.id=k.checklist_id AND c.company_id=$1
JOIN hr_employees e ON e.id=c.employee_id AND e.company_id=c.company_id
LEFT JOIN users u ON u.id=k.assignee_user_id`

func scanTask(row pgx.Row) (Task, error) {
	var t Task
	err := row.Scan(&t.ID, &t.CompanyID, &t.ChecklistID, &t.Seq, &t.Title, &t.Description, &t.AssigneeRole, &t.AssigneeUserID, &t.AssigneeName, &t.DueDate, &t.Status, &t.ActionKey, &t.Notes, &t.CompletedBy, &t.CompletedAt,
		&t.EmployeeID, &t.EmployeeName, &t.Kind)
	return t, err
}

func collectTasks(rows pgx.Rows) ([]Task, error) {
	defer rows.Close()
	var out []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Get returns a company checklist with its tasks in seq order.
func (s *Service) Get(ctx context.Context, companyID, checklistID int64) (Checklist, error) {
	if companyID <= 0 {
		return Checklist{}, ErrCompanyRequired
	}
	c, err := scanChecklist(s.pool.QueryRow(ctx, checklistSelect+` AND c.id=$2`, companyID, checklistID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Checklist{}, ErrChecklistNotFound
	}
	if err != nil {
		return Checklist{}, fmt.Errorf("get checklist: %w", err)
	}
	rows, err := s.pool.Query(ctx, taskSelect+` WHERE k.checklist_id=$2 ORDER BY k.seq, k.id`, companyID, checklistID)
	if err != nil {
		return Checklist{}, fmt.Errorf("get checklist tasks: %w", err)
	}
	if c.Tasks, err = collectTasks(rows); err != nil {
		return Checklist{}, err
	}
	return c, nil
}

// List returns company checklists, newest first.
func (s *Service) List(ctx context.Context, companyID int64, f Filter) ([]Checklist, error) {
	if companyID <= 0 {
		return nil, ErrCompanyRequired
	}
	query := checklistSelect
	args := []any{companyID}
	if st := strings.ToUpper(strings.TrimSpace(f.Status)); st != "" {
		args = append(args, st)
		query += fmt.Sprintf(` AND c.status=$%d`, len(args))
	}
	if k := strings.ToUpper(strings.TrimSpace(f.Kind)); k != "" {
		args = append(args, k)
		query += fmt.Sprintf(` AND c.kind=$%d`, len(args))
	}
	if f.EmployeeID != nil {
		args = append(args, *f.EmployeeID)
		query += fmt.Sprintf(` AND c.employee_id=$%d`, len(args))
	}
	rows, err := s.pool.Query(ctx, query+` ORDER BY c.created_at DESC, c.id DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("list checklists: %w", err)
	}
	defer rows.Close()
	var out []Checklist
	for rows.Next() {
		c, err := scanChecklist(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// MyTasks returns the user's OPEN tasks on OPEN company checklists, by due
// date.
func (s *Service) MyTasks(ctx context.Context, companyID, userID int64) ([]Task, error) {
	if companyID <= 0 {
		return nil, ErrCompanyRequired
	}
	if userID <= 0 {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, taskSelect+` WHERE k.assignee_user_id=$2 AND k.status='OPEN' AND c.status='OPEN' ORDER BY k.due_date NULLS LAST, k.id`, companyID, userID)
	if err != nil {
		return nil, fmt.Errorf("my checklist tasks: %w", err)
	}
	return collectTasks(rows)
}

// IsTaskAssignee reports whether the task belongs to a checklist of the company
// and is assigned to the user.
func (s *Service) IsTaskAssignee(ctx context.Context, companyID, taskID, userID int64) (bool, error) {
	if companyID <= 0 || taskID <= 0 || userID <= 0 {
		return false, nil
	}
	var ok bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM hr_employee_checklist_tasks k JOIN hr_employee_checklists c ON c.id=k.checklist_id AND c.company_id=$2 WHERE k.id=$1 AND k.assignee_user_id=$3)`, taskID, companyID, userID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("check task assignee: %w", err)
	}
	return ok, nil
}

type startEmployee struct {
	name     string
	status   string
	hireDate time.Time
	manager  *int64
}

type pendingTask struct {
	id       int64
	assignee *int64
}

// Start instantiates a checklist from a template in one transaction:
// template and employee are validated in the company, MANAGER tasks go to
// the manager's user as of today (unassigned when the manager has no active
// user), HR/IT tasks to the item's default assignee (which must be an active
// user; unassigned when the item has none), due_date = started_on +
// due_offset_days. OFFBOARDING writes hr_employees.termination_date/reason
// without changing the employee status. After commit one notification is
// sent per distinct assignee.
func (s *Service) Start(ctx context.Context, in StartInput) (Checklist, error) {
	if in.CompanyID <= 0 {
		return Checklist{}, ErrCompanyRequired
	}
	if in.EmployeeID <= 0 || in.TemplateID <= 0 {
		return Checklist{}, ErrInvalidStart
	}
	if s.audit == nil {
		return Checklist{}, errAuditNotConfigured
	}
	if s.employees == nil {
		return Checklist{}, errEmployeesNotConfigured
	}
	today := s.today()
	in.TerminationReason = strings.TrimSpace(in.TerminationReason)

	var out Checklist
	var created []pendingTask
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		tpl, err := scanTemplate(tx.QueryRow(ctx, `SELECT `+templateCols+` FROM hr_checklist_templates t WHERE t.id=$1 AND t.company_id=$2`, in.TemplateID, in.CompanyID))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTemplateNotFound
		}
		if err != nil {
			return fmt.Errorf("load checklist template: %w", err)
		}
		if !tpl.IsActive {
			return ErrTemplateInactive
		}

		var emp startEmployee
		err = tx.QueryRow(ctx, `SELECT name, status, hire_date, manager_id FROM hr_employees WHERE id=$1 AND company_id=$2 FOR UPDATE`, in.EmployeeID, in.CompanyID).Scan(&emp.name, &emp.status, &emp.hireDate, &emp.manager)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrEmployeeNotFound
		}
		if err != nil {
			return fmt.Errorf("lock employee: %w", err)
		}
		if emp.status != employees.StatusActive {
			return ErrEmployeeInactive
		}

		var termination *time.Time
		if tpl.Kind == KindOffboarding {
			if in.TerminationDate == nil || in.TerminationDate.IsZero() {
				return ErrTerminationRequired
			}
			d := dateOnly(*in.TerminationDate)
			if d.Before(dateOnly(emp.hireDate)) {
				return ErrTerminationBeforeHire
			}
			termination = &d
		}

		itemRows, err := tx.Query(ctx, itemSelect+` WHERE i.template_id=$2 ORDER BY i.seq`, in.CompanyID, in.TemplateID)
		if err != nil {
			return fmt.Errorf("load checklist items: %w", err)
		}
		items, err := collectItems(itemRows)
		if err != nil {
			return fmt.Errorf("load checklist items: %w", err)
		}
		if len(items) == 0 {
			return ErrTemplateEmpty
		}

		assignees, err := s.resolveAssignees(ctx, tx, in.CompanyID, in.EmployeeID, emp.manager, today, items)
		if err != nil {
			return err
		}

		reason := ""
		if termination != nil {
			reason = in.TerminationReason
		}
		out = Checklist{CompanyID: in.CompanyID, EmployeeID: in.EmployeeID, EmployeeName: emp.name, TemplateID: tpl.ID, TemplateName: tpl.Name, Kind: tpl.Kind, Status: StatusOpen,
			StartedOn: today, TerminationDate: termination, TerminationReason: reason, CreatedBy: nullableID(in.ActorID)}
		err = tx.QueryRow(ctx, `INSERT INTO hr_employee_checklists(company_id,employee_id,template_id,kind,status,started_on,termination_date,termination_reason,created_by)
VALUES($1,$2,$3,$4,'OPEN',$5,$6,$7,$8) RETURNING id,created_at`, in.CompanyID, in.EmployeeID, tpl.ID, tpl.Kind, today, termination, reason, out.CreatedBy).Scan(&out.ID, &out.CreatedAt)
		if isUniqueViolation(err) {
			return ErrAlreadyOpen
		}
		if err != nil {
			return fmt.Errorf("insert checklist: %w", err)
		}

		for i, it := range items {
			due := today.AddDate(0, 0, it.DueOffsetDays)
			var actionKey *string
			if it.ActionKey != "" {
				ak := it.ActionKey
				actionKey = &ak
			}
			task := Task{CompanyID: in.CompanyID, ChecklistID: out.ID, Seq: it.Seq, Title: it.Title, Description: it.Description, AssigneeRole: it.AssigneeRole,
				AssigneeUserID: assignees[i], DueDate: &due, Status: TaskOpen, ActionKey: it.ActionKey, EmployeeID: in.EmployeeID, EmployeeName: emp.name, Kind: tpl.Kind}
			if err := tx.QueryRow(ctx, `INSERT INTO hr_employee_checklist_tasks(company_id,checklist_id,seq,title,description,assignee_role,assignee_user_id,due_date,status,action_key)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,'OPEN',$9) RETURNING id`, in.CompanyID, out.ID, it.Seq, it.Title, it.Description, it.AssigneeRole, assignees[i], due, actionKey).Scan(&task.ID); err != nil {
				return fmt.Errorf("insert checklist task: %w", err)
			}
			out.Tasks = append(out.Tasks, task)
			created = append(created, pendingTask{id: task.ID, assignee: task.AssigneeUserID})
		}
		out.OpenTasks = len(out.Tasks)

		if termination != nil {
			if _, err := tx.Exec(ctx, `UPDATE hr_employees SET termination_date=$1, termination_reason=$2, updated_at=NOW() WHERE id=$3 AND company_id=$4`, *termination, reason, in.EmployeeID, in.CompanyID); err != nil {
				return fmt.Errorf("record termination date: %w", err)
			}
		}

		meta := map[string]any{"employee_id": in.EmployeeID, "template_id": tpl.ID, "kind": tpl.Kind, "tasks": len(out.Tasks)}
		if termination != nil {
			meta["termination_date"] = termination.Format("2006-01-02")
		}
		return s.record(ctx, tx, in.ActorID, ActionChecklistStarted, entityChecklist, out.ID, meta)
	})
	if err != nil {
		return Checklist{}, err
	}
	s.notifyAssignees(ctx, out, created)
	return out, nil
}

// resolveAssignees returns the assignee user id for each item (nil when
// unassigned).
func (s *Service) resolveAssignees(ctx context.Context, tx pgx.Tx, companyID, employeeID int64, currentManager *int64, today time.Time, items []Item) ([]*int64, error) {
	out := make([]*int64, len(items))
	var managerUser *int64
	managerResolved := false
	for i, it := range items {
		switch it.AssigneeRole {
		case RoleManager:
			if !managerResolved {
				u, err := s.managerUserAsOf(ctx, tx, companyID, employeeID, currentManager, today)
				if err != nil {
					return nil, err
				}
				managerUser, managerResolved = u, true
			}
			out[i] = managerUser
		default:
			if it.DefaultAssigneeUserID == nil {
				continue
			}
			if err := requireActiveUser(ctx, tx, *it.DefaultAssigneeUserID); err != nil {
				return nil, fmt.Errorf("%w (item %d)", err, it.Seq)
			}
			id := *it.DefaultAssigneeUserID
			out[i] = &id
		}
	}
	return out, nil
}

// managerUserAsOf resolves the manager on the employee's job record
// effective on day (falling back to hr_employees.manager_id when no record
// is effective) and returns the manager's active user id, or nil.
func (s *Service) managerUserAsOf(ctx context.Context, tx pgx.Tx, companyID, employeeID int64, currentManager *int64, day time.Time) (*int64, error) {
	managerID := currentManager
	jh, err := s.employees.AssignmentAsOf(ctx, companyID, employeeID, day)
	switch {
	case err == nil:
		managerID = jh.ManagerID
	case errors.Is(err, employees.ErrNotFound):
		// no effective record: keep the current manager column
	default:
		return nil, fmt.Errorf("resolve manager: %w", err)
	}
	if managerID == nil {
		return nil, nil
	}
	var userID *int64
	var active *bool
	err = tx.QueryRow(ctx, `SELECT m.user_id, u.is_active FROM hr_employees m LEFT JOIN users u ON u.id=m.user_id WHERE m.id=$1 AND m.company_id=$2`, *managerID, companyID).Scan(&userID, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve manager user: %w", err)
	}
	if userID == nil || active == nil || !*active {
		return nil, nil
	}
	return userID, nil
}

// notifyAssignees sends one message per distinct assignee, keyed by that
// assignee's first task (tasks arrive in seq order). Delivery is best effort.
func (s *Service) notifyAssignees(ctx context.Context, c Checklist, tasks []pendingTask) {
	if s.notifier == nil {
		return
	}
	type group struct {
		first pendingTask
		count int
	}
	groups := map[int64]*group{}
	var order []int64
	for _, t := range tasks {
		if t.assignee == nil || *t.assignee <= 0 {
			continue
		}
		g, ok := groups[*t.assignee]
		if !ok {
			g = &group{first: t}
			groups[*t.assignee] = g
			order = append(order, *t.assignee)
		}
		g.count++
	}
	ctx = context.WithoutCancel(ctx)
	kind := strings.ToLower(c.Kind)
	for _, recipient := range order {
		g := groups[recipient]
		noun := "task"
		if g.count != 1 {
			noun = "tasks"
		}
		_ = s.notifier.Dispatch(ctx, notifications.Message{
			RecipientID: recipient,
			DedupeKey:   fmt.Sprintf("hr-checklist-task-%d", g.first.id),
			Type:        NotificationTypeTaskAssigned,
			Title:       "Checklist tasks assigned",
			Body:        fmt.Sprintf("You have %d %s assigned on the %s checklist for %s.", g.count, noun, kind, c.EmployeeName),
			URL:         fmt.Sprintf("/hr/checklists/%d", c.ID),
		})
	}
}

// CompleteTask marks an OPEN task DONE. Only the assignee or a checklist
// admin may do so, and only while the checklist is OPEN.
func (s *Service) CompleteTask(ctx context.Context, companyID, taskID, actorID int64, isAdmin bool, notes string) error {
	return s.closeTask(ctx, companyID, taskID, actorID, isAdmin, notes, TaskDone, ActionTaskCompleted)
}

// SkipTask marks an OPEN task SKIPPED (same rules as CompleteTask).
func (s *Service) SkipTask(ctx context.Context, companyID, taskID, actorID int64, isAdmin bool, notes string) error {
	return s.closeTask(ctx, companyID, taskID, actorID, isAdmin, notes, TaskSkipped, ActionTaskSkipped)
}

func (s *Service) closeTask(ctx context.Context, companyID, taskID, actorID int64, isAdmin bool, notes, status, action string) error {
	if companyID <= 0 {
		return ErrCompanyRequired
	}
	if taskID <= 0 || actorID <= 0 {
		return ErrTaskNotFound
	}
	notes = strings.TrimSpace(notes)
	return s.inTx(ctx, func(tx pgx.Tx) error {
		var checklistID int64
		var taskStatus, checklistStatus string
		var assignee *int64
		err := tx.QueryRow(ctx, `SELECT k.checklist_id, k.status, k.assignee_user_id, c.status FROM hr_employee_checklist_tasks k JOIN hr_employee_checklists c ON c.id=k.checklist_id AND c.company_id=$2 WHERE k.id=$1 FOR UPDATE OF k`, taskID, companyID).
			Scan(&checklistID, &taskStatus, &assignee, &checklistStatus)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTaskNotFound
		}
		if err != nil {
			return fmt.Errorf("lock checklist task: %w", err)
		}
		if !isAdmin && (assignee == nil || *assignee != actorID) {
			return ErrForbidden
		}
		if checklistStatus != StatusOpen {
			return ErrNotOpen
		}
		if taskStatus != TaskOpen {
			return ErrTaskNotOpen
		}
		if _, err := tx.Exec(ctx, `UPDATE hr_employee_checklist_tasks SET status=$2, notes=$3, completed_by=$4, completed_at=NOW() WHERE id=$1`, taskID, status, notes, actorID); err != nil {
			return fmt.Errorf("update checklist task: %w", err)
		}
		return s.record(ctx, tx, actorID, action, entityTask, taskID, map[string]any{"checklist_id": checklistID})
	})
}
