package employees

import (
	"context"
	"testing"
	"time"

	pgxmock "github.com/pashagolub/pgxmock/v4"
)

func TestListEmployeesMapsJoinedFields(t *testing.T) {
	db, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := &Service{pool: db}
	hireDate := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	department, position, manager, user := int64(3), int64(4), int64(5), int64(6)
	db.ExpectQuery("SELECT e.id,e.company_id,e.user_id,e.employee_number").WithArgs(int64(2)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "company_id", "user_id", "employee_number", "name", "email", "department_id", "position_id", "manager_id", "hire_date", "status", "department", "position", "manager"}).
			AddRow(int64(8), int64(2), &user, "E-007", "Ada", "ada@example.com", &department, &position, &manager, hireDate, "ACTIVE", "Finance", "Controller", "Manager"))
	employees, err := service.List(context.Background(), 2)
	if err != nil || len(employees) != 1 || employees[0].Department != "Finance" || employees[0].Manager != "Manager" {
		t.Fatalf("List() = %#v, %v", employees, err)
	}
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestOptionalEmployeeID(t *testing.T) {
	if opt("0") != nil || opt("not-an-id") != nil {
		t.Fatal("opt() returned a pointer for an invalid ID")
	}
	value := opt("12")
	if value == nil || *value != 12 {
		t.Fatalf("opt(12) = %v", value)
	}
}
