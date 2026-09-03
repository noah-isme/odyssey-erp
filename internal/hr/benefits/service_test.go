package benefits

import (
	"context"
	"testing"
	"time"

	pgxmock "github.com/pashagolub/pgxmock/v4"
)

func TestCreateBenefitValidatesBeforeDatabase(t *testing.T) {
	service := NewService(nil)
	if _, err := service.CreateBenefit(context.Background(), Benefit{CompanyID: 1, Name: " ", Provider: "Provider"}); err == nil {
		t.Fatal("CreateBenefit() accepted a blank name")
	}
	if _, err := service.EnrollEmployee(context.Background(), EmployeeBenefit{EmployeeID: 1}); err == nil {
		t.Fatal("EnrollEmployee() accepted a missing benefit")
	}
}

func TestCreateAndEnrollBenefit(t *testing.T) {
	db, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := &Service{pool: db}

	createdAt := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	db.ExpectQuery("INSERT INTO hr_benefits").WithArgs(int64(2), "Dental", "Dental cover", "Acme Benefits", 125.5).
		WillReturnRows(pgxmock.NewRows([]string{"id", "company_id", "name", "description", "provider", "cost", "created_at"}).
			AddRow(int64(9), int64(2), "Dental", "Dental cover", "Acme Benefits", 125.5, createdAt))
	benefit, err := service.CreateBenefit(context.Background(), Benefit{CompanyID: 2, Name: " Dental ", Description: "Dental cover", Provider: "Acme Benefits", Cost: 125.5})
	if err != nil {
		t.Fatal(err)
	}
	if benefit.ID != 9 || benefit.Name != "Dental" {
		t.Fatalf("benefit = %#v", benefit)
	}

	enrolledAt := time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)
	db.ExpectQuery("INSERT INTO hr_employee_benefits").WithArgs(int64(17), int64(9)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "employee_id", "benefit_id", "status", "enrolled_at"}).
			AddRow(int64(10), int64(17), int64(9), "ACTIVE", enrolledAt))
	enrollment, err := service.EnrollEmployee(context.Background(), EmployeeBenefit{EmployeeID: 17, BenefitID: benefit.ID})
	if err != nil {
		t.Fatal(err)
	}
	if enrollment.ID != 10 || enrollment.Status != "ACTIVE" {
		t.Fatalf("enrollment = %#v", enrollment)
	}
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListBenefitsMapsRows(t *testing.T) {
	db, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := &Service{pool: db}
	createdAt := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	db.ExpectQuery("SELECT id, company_id, name, description, provider, cost, created_at FROM hr_benefits").
		WithArgs(int64(2)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "company_id", "name", "description", "provider", "cost", "created_at"}).
			AddRow(int64(1), int64(2), "Medical", "", "Provider", 50.0, createdAt))
	benefits, err := service.ListBenefits(context.Background(), 2)
	if err != nil || len(benefits) != 1 || benefits[0].Name != "Medical" {
		t.Fatalf("ListBenefits() = %#v, %v", benefits, err)
	}
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
