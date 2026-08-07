package projects

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrNotFound     = errors.New("projects: not found")
	ErrInvalidState = errors.New("projects: invalid state")
)

type Project struct {
	ID, CompanyID, ManagerID     int64
	Code, Name, Currency, Status string
}
type Task struct {
	ID, ProjectID      int64
	Code, Name, Status string
}
type Timesheet struct {
	ID, CompanyID, ProjectID, TaskID, EmployeeID int64
	WorkDate                                     string
	Currency, BaseCurrency, FXRateSource         string
	BillableRate, BaseAmount, FXRate             float64
	FXRateLockedAt                               string
	Hours                                        float64
	Description                                  string
	Billable                                     bool
	Status                                       string
}

type Repository interface {
	GetProjectTask(context.Context, int64, int64) (Project, Task, error)
	IsProjectMember(context.Context, int64, int64, int64) (bool, error)
	CreateTimesheet(context.Context, Timesheet) (Timesheet, error)
	GetTimesheet(context.Context, int64, int64) (Timesheet, error)
	UpdateTimesheet(context.Context, Timesheet) error
}

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func (s *Service) CreateTimesheet(ctx context.Context, sheet Timesheet) (Timesheet, error) {
	if sheet.CompanyID == 0 || sheet.ProjectID == 0 || sheet.TaskID == 0 || sheet.EmployeeID == 0 || sheet.Hours <= 0 || sheet.Hours > 24 {
		return Timesheet{}, errors.New("projects: invalid timesheet")
	}
	project, task, err := s.repo.GetProjectTask(ctx, sheet.ProjectID, sheet.TaskID)
	if err != nil {
		return Timesheet{}, err
	}
	if project.CompanyID != sheet.CompanyID || task.ProjectID != sheet.ProjectID || task.Status == "CANCELLED" {
		return Timesheet{}, ErrNotFound
	}
	sheet.Status = "DRAFT"
	return s.repo.CreateTimesheet(ctx, sheet)
}

func (s *Service) Submit(ctx context.Context, companyID, employeeID, id int64) (Timesheet, error) {
	return s.transition(ctx, companyID, employeeID, id, "DRAFT", "SUBMITTED")
}
func (s *Service) Reject(ctx context.Context, companyID, managerID, id int64) (Timesheet, error) {
	return s.transition(ctx, companyID, managerID, id, "SUBMITTED", "REJECTED")
}
func (s *Service) Approve(ctx context.Context, companyID, managerID, id int64) (Timesheet, error) {
	return s.transition(ctx, companyID, managerID, id, "SUBMITTED", "APPROVED")
}
func (s *Service) Lock(ctx context.Context, companyID, managerID, id int64) (Timesheet, error) {
	return s.transition(ctx, companyID, managerID, id, "APPROVED", "LOCKED")
}

func (s *Service) transition(ctx context.Context, companyID, actorID, id int64, from, to string) (Timesheet, error) {
	sheet, err := s.repo.GetTimesheet(ctx, companyID, id)
	if err != nil {
		return Timesheet{}, err
	}
	if sheet.Status != from {
		return Timesheet{}, fmt.Errorf("%w: timesheet is %s", ErrInvalidState, sheet.Status)
	}
	if from == "DRAFT" && sheet.EmployeeID != actorID {
		return Timesheet{}, ErrNotFound
	}
	if from == "DRAFT" {
		members, ok := s.repo.(interface {
			IsProjectMember(context.Context, int64, int64, int64) (bool, error)
		})
		if ok {
			member, err := members.IsProjectMember(ctx, sheet.CompanyID, sheet.ProjectID, actorID)
			if err != nil {
				return Timesheet{}, err
			}
			if !member {
				return Timesheet{}, ErrNotFound
			}
		}
	}
	if from != "DRAFT" {
		project, _, err := s.repo.GetProjectTask(ctx, sheet.ProjectID, sheet.TaskID)
		if err != nil {
			return Timesheet{}, err
		}
		if project.ManagerID != 0 && project.ManagerID != actorID {
			return Timesheet{}, ErrNotFound
		}
	}
	sheet.Status = to
	if err := s.repo.UpdateTimesheet(ctx, sheet); err != nil {
		return Timesheet{}, err
	}
	return sheet, nil
}

// =============================================================================
// Advanced Project Features (Milestones, Resource Allocation, Expenses)
// =============================================================================

type ProjectMilestone struct {
	ID          int64
	ProjectID   int64
	Name        string
	Description string
	DueDate     string
	Status      string
}

type ResourceAllocation struct {
	ID             int64
	ProjectID      int64
	EmployeeID     int64
	AllocatedHours float64
	StartDate      string
	EndDate        string
}

type ProjectExpense struct {
	ID          int64
	ProjectID   int64
	EmployeeID  int64
	Amount      float64
	Currency    string
	Description string
	ReceiptURL  string
	Status      string
}

func (s *Service) CreateMilestone(ctx context.Context, milestone ProjectMilestone) (ProjectMilestone, error) {
	if s == nil || s.repo == nil {
		return ProjectMilestone{}, errors.New("projects: repository is required")
	}
	if milestone.ProjectID == 0 || milestone.Name == "" {
		return ProjectMilestone{}, errors.New("projects: invalid milestone data")
	}
	milestone.Status = "PENDING"
	return s.repo.(AdvancedRepository).CreateMilestone(ctx, milestone)
}

func (s *Service) AllocateResource(ctx context.Context, allocation ResourceAllocation) (ResourceAllocation, error) {
	if s == nil || s.repo == nil {
		return ResourceAllocation{}, errors.New("projects: repository is required")
	}
	if allocation.ProjectID == 0 || allocation.EmployeeID == 0 || allocation.AllocatedHours <= 0 {
		return ResourceAllocation{}, errors.New("projects: invalid allocation data")
	}
	return s.repo.(AdvancedRepository).AllocateResource(ctx, allocation)
}

func (s *Service) SubmitExpense(ctx context.Context, expense ProjectExpense) (ProjectExpense, error) {
	if s == nil || s.repo == nil {
		return ProjectExpense{}, errors.New("projects: repository is required")
	}
	if expense.ProjectID == 0 || expense.EmployeeID == 0 || expense.Amount <= 0 {
		return ProjectExpense{}, errors.New("projects: invalid expense data")
	}
	expense.Status = "SUBMITTED"
	if expense.Currency == "" {
		expense.Currency = "IDR"
	}
	return s.repo.(AdvancedRepository).SubmitExpense(ctx, expense)
}

type AdvancedRepository interface {
	Repository
	CreateMilestone(context.Context, ProjectMilestone) (ProjectMilestone, error)
	AllocateResource(context.Context, ResourceAllocation) (ResourceAllocation, error)
	SubmitExpense(context.Context, ProjectExpense) (ProjectExpense, error)
}
