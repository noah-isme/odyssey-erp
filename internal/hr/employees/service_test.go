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

func TestUpdateRejectsSelfManagementAndInvalidStatus(t *testing.T) {
	service := NewService(nil)
	hireDate := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	selfID := int64(8)

	// Self management
	_, err := service.Update(context.Background(), 2, 8, UpdateInput{
		ManagerID:      &selfID,
		EmployeeNumber: "E-007",
		Name:           "Ada",
		Email:          "ada@example.com",
		HireDate:       hireDate,
		Status:         "ACTIVE",
	})
	require.EqualError(t, err, "hr: employee cannot manage themselves")

	// Invalid status
	otherManager := int64(5)
	_, err = service.Update(context.Background(), 2, 8, UpdateInput{
		ManagerID:      &otherManager,
		EmployeeNumber: "E-007",
		Name:           "Ada",
		Email:          "ada@example.com",
		HireDate:       hireDate,
		Status:         "INVALID_STATUS",
	})
	require.EqualError(t, err, "hr: invalid employee status")
}

func TestUpdatePersistsChanges(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	service := &Service{pool: db}

	department, position, manager, user := int64(3), int64(4), int64(5), int64(6)
	hireDate := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	db.ExpectQuery("UPDATE hr_employees SET").
		WithArgs(&user, "E-007", "Ada Updated", "ada.new@example.com", &department, &position, &manager, hireDate, "ACTIVE", int64(8), int64(2)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "company_id", "user_id", "employee_number", "name", "email", "department_id", "position_id", "manager_id", "hire_date", "status"}).
			AddRow(int64(8), int64(2), &user, "E-007", "Ada Updated", "ada.new@example.com", &department, &position, &manager, hireDate, "ACTIVE"))

	emp, err := service.Update(context.Background(), 2, 8, UpdateInput{
		UserID:         &user,
		DepartmentID:   &department,
		PositionID:     &position,
		ManagerID:      &manager,
		EmployeeNumber: "E-007",
		Name:           "Ada Updated",
		Email:          "ada.new@example.com",
		HireDate:       hireDate,
		Status:         "ACTIVE",
	})
	require.NoError(t, err)
	require.Equal(t, "Ada Updated", emp.Name)
	require.Equal(t, "ada.new@example.com", emp.Email)
	require.NoError(t, db.ExpectationsWereMet())
}

func TestDepartmentsAndPositionsCRUD(t *testing.T) {
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer db.Close()
	service := &Service{pool: db}

	now := time.Now()
	// Create department
	db.ExpectQuery("INSERT INTO hr_departments").
		WithArgs(int64(1), "HR", "Human Resources").
		WillReturnRows(pgxmock.NewRows([]string{"id", "company_id", "code", "name", "created_at"}).
			AddRow(int64(10), int64(1), "HR", "Human Resources", now))

	dept, err := service.CreateDepartment(context.Background(), 1, "HR", "Human Resources")
	require.NoError(t, err)
	require.Equal(t, int64(10), dept.ID)

	// List departments
	db.ExpectQuery("SELECT d.id, d.company_id, d.code, d.name, COUNT").
		WithArgs(int64(1)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "company_id", "code", "name", "count", "created_at"}).
			AddRow(int64(10), int64(1), "HR", "Human Resources", 5, now))

	depts, err := service.ListDepartments(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, depts, 1)
	require.Equal(t, 5, depts[0].EmployeeCount)

	// Create position
	db.ExpectQuery("INSERT INTO hr_positions").
		WithArgs(int64(1), "DEV", "Developer").
		WillReturnRows(pgxmock.NewRows([]string{"id", "company_id", "code", "name", "created_at"}).
			AddRow(int64(20), int64(1), "DEV", "Developer", now))

	pos, err := service.CreatePosition(context.Background(), 1, "DEV", "Developer")
	require.NoError(t, err)
	require.Equal(t, int64(20), pos.ID)

	// List positions
	db.ExpectQuery("SELECT p.id, p.company_id, p.code, p.name, COUNT").
		WithArgs(int64(1)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "company_id", "code", "name", "count", "created_at"}).
			AddRow(int64(20), int64(1), "DEV", "Developer", 3, now))

	positions, err := service.ListPositions(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, positions, 1)
	require.Equal(t, 3, positions[0].EmployeeCount)

	require.NoError(t, db.ExpectationsWereMet())
}
