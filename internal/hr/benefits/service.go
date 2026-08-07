package benefits

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Benefit struct {
	ID          int64
	CompanyID   int64
	Name        string
	Description string
	Provider    string
	Cost        float64
	CreatedAt   time.Time
}

type EmployeeBenefit struct {
	ID         int64
	EmployeeID int64
	BenefitID  int64
	Status     string
	EnrolledAt time.Time
}

type db interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Service struct{ pool db }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func (s *Service) ListBenefits(ctx context.Context, companyID int64) ([]Benefit, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, company_id, name, description, provider, cost, created_at FROM hr_benefits WHERE company_id = $1 ORDER BY name`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Benefit
	for rows.Next() {
		var b Benefit
		if err := rows.Scan(&b.ID, &b.CompanyID, &b.Name, &b.Description, &b.Provider, &b.Cost, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Service) CreateBenefit(ctx context.Context, b Benefit) (Benefit, error) {
	b.Name = strings.TrimSpace(b.Name)
	if b.CompanyID <= 0 || b.Name == "" || b.Provider == "" {
		return Benefit{}, errors.New("hr: invalid benefit")
	}
	err := s.pool.QueryRow(ctx, `INSERT INTO hr_benefits (company_id, name, description, provider, cost) VALUES ($1, $2, $3, $4, $5) RETURNING id, company_id, name, description, provider, cost, created_at`, 
		b.CompanyID, b.Name, b.Description, b.Provider, b.Cost).Scan(&b.ID, &b.CompanyID, &b.Name, &b.Description, &b.Provider, &b.Cost, &b.CreatedAt)
	return b, err
}

func (s *Service) EnrollEmployee(ctx context.Context, eb EmployeeBenefit) (EmployeeBenefit, error) {
	if eb.EmployeeID <= 0 || eb.BenefitID <= 0 {
		return EmployeeBenefit{}, errors.New("hr: invalid enrollment")
	}
	err := s.pool.QueryRow(ctx, `INSERT INTO hr_employee_benefits (employee_id, benefit_id) VALUES ($1, $2) RETURNING id, employee_id, benefit_id, status, enrolled_at`,
		eb.EmployeeID, eb.BenefitID).Scan(&eb.ID, &eb.EmployeeID, &eb.BenefitID, &eb.Status, &eb.EnrolledAt)
	return eb, err
}
