package projects

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

type memoryRepo struct {
	project Project
	task    Task
	sheet   Timesheet
	member  bool
}

func (r *memoryRepo) GetProjectTask(context.Context, int64, int64) (Project, Task, error) {
	return r.project, r.task, nil
}
func (r *memoryRepo) IsProjectMember(context.Context, int64, int64, int64) (bool, error) {
	return r.member, nil
}
func (r *memoryRepo) CreateTimesheet(_ context.Context, s Timesheet) (Timesheet, error) {
	s.ID = 1
	r.sheet = s
	return s, nil
}
func (r *memoryRepo) GetTimesheet(context.Context, int64, int64) (Timesheet, error) {
	return r.sheet, nil
}
func (r *memoryRepo) UpdateTimesheet(_ context.Context, s Timesheet) error { r.sheet = s; return nil }
func TestTimesheetApprovalAndLock(t *testing.T) {
	r := &memoryRepo{project: Project{ID: 1, CompanyID: 2, ManagerID: 9}, task: Task{ID: 3, ProjectID: 1, Status: "OPEN"}, member: true}
	s := NewService(r)
	sheet, e := s.CreateTimesheet(context.Background(), Timesheet{CompanyID: 2, ProjectID: 1, TaskID: 3, EmployeeID: 5, Hours: 8})
	require.NoError(t, e)
	_, e = s.Submit(context.Background(), 2, 5, sheet.ID)
	require.NoError(t, e)
	_, e = s.Approve(context.Background(), 2, 9, sheet.ID)
	require.NoError(t, e)
	locked, e := s.Lock(context.Background(), 2, 9, sheet.ID)
	require.NoError(t, e)
	require.Equal(t, "LOCKED", locked.Status)
}
