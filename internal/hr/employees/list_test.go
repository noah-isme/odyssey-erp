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
		WillReturnRows(employeeRows(Employee{ID: 8, CompanyID: 2, UserID: &user, EmployeeNumber: "E-007", Name: "Ada", Email: "ada@example.com",
			DepartmentID: &department, PositionID: &position, ManagerID: &manager, HireDate: hireDate, Status: "ACTIVE",
			Department: "Finance", Position: "Controller", Manager: "Manager", EmploymentType: "PERMANENT", Profile: Profile{NationalID: "3171"}}))
	employees, err := service.List(context.Background(), 2)
	if err != nil || len(employees) != 1 || employees[0].Department != "Finance" || employees[0].Manager != "Manager" || employees[0].NationalID != "3171" || employees[0].EmploymentType != "PERMANENT" {
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
