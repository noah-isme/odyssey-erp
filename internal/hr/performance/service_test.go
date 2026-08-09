package performance

import (
	"context"
	"testing"
	"time"

	pgxmock "github.com/pashagolub/pgxmock/v4"
)

func TestCreateReviewValidatesBeforeDatabase(t *testing.T) {
	service := NewService(nil)
	for _, review := range []Review{
		{ReviewerID: 2, ReviewPeriod: "2026-H1", Rating: 3},
		{EmployeeID: 1, ReviewPeriod: "2026-H1", Rating: 0},
		{EmployeeID: 1, ReviewerID: 2, ReviewPeriod: "2026-H1", Rating: 6},
	} {
		if _, err := service.CreateReview(context.Background(), review); err == nil {
			t.Fatalf("CreateReview() accepted %#v", review)
		}
	}
}

func TestCreateAndListReviews(t *testing.T) {
	db, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := &Service{pool: db}
	createdAt := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)
	db.ExpectQuery("INSERT INTO hr_performance_reviews").WithArgs(int64(4), int64(8), "2026-H1", 5, "Excellent").
		WillReturnRows(pgxmock.NewRows([]string{"id", "employee_id", "reviewer_id", "review_period", "rating", "comments", "created_at"}).
			AddRow(int64(12), int64(4), int64(8), "2026-H1", 5, "Excellent", createdAt))
	review, err := service.CreateReview(context.Background(), Review{EmployeeID: 4, ReviewerID: 8, ReviewPeriod: "2026-H1", Rating: 5, Comments: "Excellent"})
	if err != nil || review.ID != 12 {
		t.Fatalf("CreateReview() = %#v, %v", review, err)
	}

	db.ExpectQuery("SELECT id, employee_id, reviewer_id, review_period, rating, comments, created_at FROM hr_performance_reviews").
		WithArgs(int64(4)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "employee_id", "reviewer_id", "review_period", "rating", "comments", "created_at"}).
			AddRow(int64(12), int64(4), int64(8), "2026-H1", 5, "Excellent", createdAt))
	reviews, err := service.ListEmployeeReviews(context.Background(), 4)
	if err != nil || len(reviews) != 1 || reviews[0].Rating != 5 {
		t.Fatalf("ListEmployeeReviews() = %#v, %v", reviews, err)
	}
	if err := db.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
