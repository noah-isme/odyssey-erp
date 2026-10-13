package performance

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v4"

	"github.com/odyssey-erp/odyssey-erp/internal/hr/employees"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

const (
	companyA      = int64(7)
	companyB      = int64(8)
	cycleID       = int64(31)
	participantID = int64(41)
	goalID        = int64(51)
	employeeID    = int64(11)
	employeeUser  = int64(42)
	managerEmpID  = int64(5)
	managerUser   = int64(77)
	adminUser     = int64(99)
	otherUser     = int64(123)
)

var (
	createdAt = time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)
	reviewCls = []string{"id", "company_id", "employee_id", "reviewer_id", "review_period", "rating", "comments", "created_at"}
)

type auditFake struct{ logs []shared.AuditLog }

func (a *auditFake) RecordTx(_ context.Context, _ pgx.Tx, log shared.AuditLog) error {
	a.logs = append(a.logs, log)
	return nil
}

func (a *auditFake) actions() []string {
	out := make([]string, 0, len(a.logs))
	for _, l := range a.logs {
		out = append(out, l.Action)
	}
	return out
}

// assertNoSensitiveMeta fails when any audit meta carries ratings, scores, or
// comments.
func (a *auditFake) assertNoSensitiveMeta(t *testing.T) {
	t.Helper()
	for _, l := range a.logs {
		for k := range l.Meta {
			if strings.Contains(k, "rating") || strings.Contains(k, "comment") || strings.Contains(k, "score") {
				t.Fatalf("audit %s meta carries %q: %#v", l.Action, k, l.Meta)
			}
		}
	}
}

type resolverFake struct {
	byEmployee map[int64]employees.JobHistory
	calls      []time.Time
	companies  []int64
}

func (r *resolverFake) AssignmentAsOf(_ context.Context, companyID, employeeID int64, asOf time.Time) (employees.JobHistory, error) {
	r.calls = append(r.calls, asOf)
	r.companies = append(r.companies, companyID)
	j, ok := r.byEmployee[employeeID]
	if !ok {
		return employees.JobHistory{}, employees.ErrNoJobAssignment
	}
	return j, nil
}

func newMock(t *testing.T) (pgxmock.PgxPoolIface, *Service, *auditFake, *resolverFake) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mock.Close)
	audit := &auditFake{}
	resolver := &resolverFake{byEmployee: map[int64]employees.JobHistory{}}
	now := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	return mock, &Service{pool: mock, employees: resolver, audit: audit, now: func() time.Time { return now }}, audit, resolver
}

func TestCreateReviewValidatesBeforeDatabase(t *testing.T) {
	service := NewService(nil, nil, nil)
	if _, err := service.CreateReview(context.Background(), 0, Review{EmployeeID: 1, ReviewerID: 2, ReviewPeriod: "2026-H1", Rating: 3}); !errors.Is(err, ErrCompanyRequired) {
		t.Fatalf("CreateReview(company 0) err = %v", err)
	}
	for _, review := range []Review{
		{ReviewerID: 2, ReviewPeriod: "2026-H1", Rating: 3},
		{EmployeeID: 1, ReviewPeriod: "2026-H1", Rating: 0},
		{EmployeeID: 1, ReviewerID: 2, ReviewPeriod: "2026-H1", Rating: 6},
	} {
		if _, err := service.CreateReview(context.Background(), companyA, review); err == nil {
			t.Fatalf("CreateReview() accepted %#v", review)
		}
	}
	if _, err := service.ListEmployeeReviews(context.Background(), 0, 4); !errors.Is(err, ErrCompanyRequired) {
		t.Fatalf("ListEmployeeReviews(company 0) err = %v", err)
	}
}

func TestCreateAndListReviewsWriteCompanyID(t *testing.T) {
	db, service, _, _ := newMock(t)
	db.ExpectQuery(regexp.QuoteMeta(`INSERT INTO hr_performance_reviews (company_id, employee_id, reviewer_id, review_period, rating, comments)`)+
		`(?s).*`+regexp.QuoteMeta(`SELECT 1 FROM hr_employees WHERE id = $2 AND company_id = $1`)+
		`.*`+regexp.QuoteMeta(`SELECT 1 FROM hr_employees WHERE id = $3 AND company_id = $1`)).
		WithArgs(companyA, int64(4), int64(8), "2026-H1", 5, "Excellent").
		WillReturnRows(pgxmock.NewRows(reviewCls).AddRow(int64(12), companyA, int64(4), int64(8), "2026-H1", 5, "Excellent", createdAt))
	review, err := service.CreateReview(context.Background(), companyA, Review{EmployeeID: 4, ReviewerID: 8, ReviewPeriod: "2026-H1", Rating: 5, Comments: "Excellent"})
	if err != nil || review.ID != 12 || review.CompanyID != companyA {
		t.Fatalf("CreateReview() = %#v, %v", review, err)
	}

	db.ExpectQuery(regexp.QuoteMeta(`FROM hr_performance_reviews WHERE company_id = $1 AND employee_id = $2`)).
		WithArgs(companyA, int64(4)).
		WillReturnRows(pgxmock.NewRows(reviewCls).AddRow(int64(12), companyA, int64(4), int64(8), "2026-H1", 5, "Excellent", createdAt))
	reviews, err := service.ListEmployeeReviews(context.Background(), companyA, 4)
	if err != nil || len(reviews) != 1 || reviews[0].Rating != 5 {
		t.Fatalf("ListEmployeeReviews() = %#v, %v", reviews, err)
	}
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateReviewRejectsCrossTenantEmployee(t *testing.T) {
	db, service, _, _ := newMock(t)
	db.ExpectQuery(regexp.QuoteMeta(`INSERT INTO hr_performance_reviews`)).
		WithArgs(companyA, int64(4), int64(8), "2026-H1", 3, "").
		WillReturnError(pgx.ErrNoRows)
	_, err := service.CreateReview(context.Background(), companyA, Review{EmployeeID: 4, ReviewerID: 8, ReviewPeriod: "2026-H1", Rating: 3})
	if !errors.Is(err, ErrEmployeeNotFound) || !errors.Is(err, ErrNotFound) {
		t.Fatalf("CreateReview(cross-tenant) err = %v", err)
	}
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
