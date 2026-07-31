package employees

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Employee struct {
	ID, CompanyID                                                      int64
	UserID, DepartmentID, PositionID, ManagerID                        *int64
	EmployeeNumber, Name, Email, Department, Position, Manager, Status string
	HireDate                                                           time.Time
}
type CreateInput struct {
	CompanyID                                   int64
	UserID, DepartmentID, PositionID, ManagerID *int64
	EmployeeNumber, Name, Email                 string
	HireDate                                    time.Time
}
type db interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Service struct{ pool db }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }
func (s *Service) List(ctx context.Context, companyID int64) ([]Employee, error) {
	rows, err := s.pool.Query(ctx, `SELECT e.id,e.company_id,e.user_id,e.employee_number,e.name,e.email,e.department_id,e.position_id,e.manager_id,e.hire_date,e.status,COALESCE(d.name,''),COALESCE(p.name,''),COALESCE(m.name,'') FROM hr_employees e LEFT JOIN hr_departments d ON d.id=e.department_id LEFT JOIN hr_positions p ON p.id=e.position_id LEFT JOIN hr_employees m ON m.id=e.manager_id WHERE ($1=0 OR e.company_id=$1) ORDER BY e.name`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Employee
	for rows.Next() {
		var e Employee
		if err := rows.Scan(&e.ID, &e.CompanyID, &e.UserID, &e.EmployeeNumber, &e.Name, &e.Email, &e.DepartmentID, &e.PositionID, &e.ManagerID, &e.HireDate, &e.Status, &e.Department, &e.Position, &e.Manager); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
func (s *Service) Create(ctx context.Context, in CreateInput) (Employee, error) {
	in.EmployeeNumber = strings.TrimSpace(in.EmployeeNumber)
	in.Name = strings.TrimSpace(in.Name)
	in.Email = strings.TrimSpace(in.Email)
	if in.CompanyID <= 0 || in.EmployeeNumber == "" || in.Name == "" || in.Email == "" || in.HireDate.IsZero() {
		return Employee{}, errors.New("hr: invalid employee")
	}
	var e Employee
	err := s.pool.QueryRow(ctx, `INSERT INTO hr_employees(user_id,company_id,employee_number,name,email,department_id,position_id,manager_id,hire_date) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id,company_id,user_id,employee_number,name,email,department_id,position_id,manager_id,hire_date,status`, in.UserID, in.CompanyID, in.EmployeeNumber, in.Name, in.Email, in.DepartmentID, in.PositionID, in.ManagerID, in.HireDate).Scan(&e.ID, &e.CompanyID, &e.UserID, &e.EmployeeNumber, &e.Name, &e.Email, &e.DepartmentID, &e.PositionID, &e.ManagerID, &e.HireDate, &e.Status)
	return e, err
}
