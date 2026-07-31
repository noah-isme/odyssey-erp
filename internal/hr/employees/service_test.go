package employees

import (
	"context"
	"testing"
	"time"

	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

func TestCreateRejectsIncompleteEmployeeBeforeDatabaseAccess(t *testing.T) {
	service := NewService(nil)
	base := CreateInput{CompanyID: 1, EmployeeNumber: "E-1", Name: "Ada", Email: "ada@example.com", HireDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}

	for name, input := range map[string]CreateInput{
		"company":   func() CreateInput { x := base; x.CompanyID = 0; return x }(),
		"number":    func() CreateInput { x := base; x.EmployeeNumber = " "; return x }(),
		"name":      func() CreateInput { x := base; x.Name = " "; return x }(),
		"email":     func() CreateInput { x := base; x.Email = " "; return x }(),
		"hire date": func() CreateInput { x := base; x.HireDate = time.Time{}; return x }(),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := service.Create(context.Background(), input)
			require.EqualError(t, err, "hr: invalid employee")
		})
	}
}

func TestCreatePersistsEmployeeRelationships(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	service := &Service{pool: db}
	department, position, manager, user := int64(3), int64(4), int64(5), int64(6)
	hireDate := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	db.ExpectQuery("INSERT INTO hr_employees").
		WithArgs(&user, int64(2), "E-007", "Ada Lovelace", "ada@example.com", &department, &position, &manager, hireDate).
		WillReturnRows(pgxmock.NewRows([]string{"id", "company_id", "user_id", "employee_number", "name", "email", "department_id", "position_id", "manager_id", "hire_date", "status"}).
			AddRow(int64(8), int64(2), &user, "E-007", "Ada Lovelace", "ada@example.com", &department, &position, &manager, hireDate, "ACTIVE"))

	employee, err := service.Create(context.Background(), CreateInput{
		CompanyID: 2, UserID: &user, DepartmentID: &department, PositionID: &position, ManagerID: &manager,
		EmployeeNumber: " E-007 ", Name: " Ada Lovelace ", Email: " ada@example.com ", HireDate: hireDate,
	})
	require.NoError(t, err)
	require.Equal(t, int64(8), employee.ID)
	require.Equal(t, "E-007", employee.EmployeeNumber)
	require.NoError(t, db.ExpectationsWereMet())
}
