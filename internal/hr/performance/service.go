package performance

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Review struct {
	ID            int64
	EmployeeID    int64
	ReviewerID    int64
	ReviewPeriod  string
	Rating        int
	Comments      string
	CreatedAt     time.Time
}

type db interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Service struct{ pool db }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func (s *Service) ListEmployeeReviews(ctx context.Context, employeeID int64) ([]Review, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, employee_id, reviewer_id, review_period, rating, comments, created_at FROM hr_performance_reviews WHERE employee_id = $1 ORDER BY created_at DESC`, employeeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Review
	for rows.Next() {
		var r Review
		if err := rows.Scan(&r.ID, &r.EmployeeID, &r.ReviewerID, &r.ReviewPeriod, &r.Rating, &r.Comments, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Service) CreateReview(ctx context.Context, r Review) (Review, error) {
	if r.EmployeeID <= 0 || r.ReviewerID <= 0 || r.ReviewPeriod == "" || r.Rating < 1 || r.Rating > 5 {
		return Review{}, errors.New("hr: invalid performance review data")
	}
	err := s.pool.QueryRow(ctx, `INSERT INTO hr_performance_reviews (employee_id, reviewer_id, review_period, rating, comments) VALUES ($1, $2, $3, $4, $5) RETURNING id, employee_id, reviewer_id, review_period, rating, comments, created_at`,
		r.EmployeeID, r.ReviewerID, r.ReviewPeriod, r.Rating, r.Comments).Scan(&r.ID, &r.EmployeeID, &r.ReviewerID, &r.ReviewPeriod, &r.Rating, &r.Comments, &r.CreatedAt)
	return r, err
}
