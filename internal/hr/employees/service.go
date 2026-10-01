package employees

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Employee struct {
	ID, CompanyID                                                      int64
	UserID, DepartmentID, PositionID, ManagerID                        *int64
	EmployeeNumber, Name, Email, Department, Position, Manager, Status string
	UserEmail                                                          string
	HireDate                                                           time.Time
}

type Department struct {
	ID            int64
	CompanyID     int64
	Code, Name    string
	EmployeeCount int
	CreatedAt     time.Time
}

type Position struct {
	ID            int64
	CompanyID     int64
	Code, Name    string
	EmployeeCount int
	CreatedAt     time.Time
}

type UserOption struct {
	ID    int64
	Name  string
	Email string
}

type EmployeeFilter struct {
	Search       string
	DepartmentID *int64
	Status       string
}

type CreateInput struct {
	CompanyID                                   int64
	UserID, DepartmentID, PositionID, ManagerID *int64
	EmployeeNumber, Name, Email                 string
	HireDate                                    time.Time
}

type UpdateInput struct {
	UserID, DepartmentID, PositionID, ManagerID *int64
	EmployeeNumber, Name, Email                 string
	HireDate                                    time.Time
	Status                                      string
}

type db interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Service struct{ pool db }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func (s *Service) List(ctx context.Context, companyID int64, filter ...EmployeeFilter) ([]Employee, error) {
	query := `SELECT e.id,e.company_id,e.user_id,e.employee_number,e.name,e.email,e.department_id,e.position_id,e.manager_id,e.hire_date,e.status,COALESCE(d.name,''),COALESCE(p.name,''),COALESCE(m.name,'') FROM hr_employees e LEFT JOIN hr_departments d ON d.id=e.department_id LEFT JOIN hr_positions p ON p.id=e.position_id LEFT JOIN hr_employees m ON m.id=e.manager_id WHERE ($1=0 OR e.company_id=$1)`
	args := []any{companyID}

	if len(filter) > 0 {
		f := filter[0]
		if f.DepartmentID != nil && *f.DepartmentID > 0 {
			args = append(args, *f.DepartmentID)
			query += fmt.Sprintf(" AND e.department_id=$%d", len(args))
		}
		if f.Status != "" && f.Status != "ALL" {
			args = append(args, strings.ToUpper(strings.TrimSpace(f.Status)))
			query += fmt.Sprintf(" AND e.status=$%d", len(args))
		}
		if search := strings.TrimSpace(f.Search); search != "" {
			pattern := "%" + strings.ToLower(search) + "%"
			args = append(args, pattern)
			query += fmt.Sprintf(" AND (LOWER(e.name) LIKE $%d OR LOWER(e.employee_number) LIKE $%d OR LOWER(e.email) LIKE $%d)", len(args), len(args), len(args))
		}
	}

	query += ` ORDER BY e.name`

	rows, err := s.pool.Query(ctx, query, args...)
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

func (s *Service) GetByID(ctx context.Context, companyID, id int64) (Employee, error) {
	var e Employee
	err := s.pool.QueryRow(ctx, `SELECT e.id,e.company_id,e.user_id,e.employee_number,e.name,e.email,e.department_id,e.position_id,e.manager_id,e.hire_date,e.status,COALESCE(d.name,''),COALESCE(p.name,''),COALESCE(m.name,'') FROM hr_employees e LEFT JOIN hr_departments d ON d.id=e.department_id LEFT JOIN hr_positions p ON p.id=e.position_id LEFT JOIN hr_employees m ON m.id=e.manager_id WHERE e.id=$1 AND ($2=0 OR e.company_id=$2)`, id, companyID).Scan(&e.ID, &e.CompanyID, &e.UserID, &e.EmployeeNumber, &e.Name, &e.Email, &e.DepartmentID, &e.PositionID, &e.ManagerID, &e.HireDate, &e.Status, &e.Department, &e.Position, &e.Manager)
	return e, err
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

func (s *Service) Update(ctx context.Context, companyID, id int64, in UpdateInput) (Employee, error) {
	in.EmployeeNumber = strings.TrimSpace(in.EmployeeNumber)
	in.Name = strings.TrimSpace(in.Name)
	in.Email = strings.TrimSpace(in.Email)
	in.Status = strings.ToUpper(strings.TrimSpace(in.Status))
	if in.Status == "" {
		in.Status = "ACTIVE"
	}
	if companyID <= 0 || id <= 0 || in.EmployeeNumber == "" || in.Name == "" || in.Email == "" || in.HireDate.IsZero() {
		return Employee{}, errors.New("hr: invalid employee")
	}
	if in.Status != "ACTIVE" && in.Status != "INACTIVE" {
		return Employee{}, errors.New("hr: invalid employee status")
	}
	if in.ManagerID != nil && *in.ManagerID == id {
		return Employee{}, errors.New("hr: employee cannot manage themselves")
	}

	var e Employee
	err := s.pool.QueryRow(ctx, `UPDATE hr_employees SET user_id=$1, employee_number=$2, name=$3, email=$4, department_id=$5, position_id=$6, manager_id=$7, hire_date=$8, status=$9, updated_at=NOW() WHERE id=$10 AND company_id=$11 RETURNING id,company_id,user_id,employee_number,name,email,department_id,position_id,manager_id,hire_date,status`, in.UserID, in.EmployeeNumber, in.Name, in.Email, in.DepartmentID, in.PositionID, in.ManagerID, in.HireDate, in.Status, id, companyID).Scan(&e.ID, &e.CompanyID, &e.UserID, &e.EmployeeNumber, &e.Name, &e.Email, &e.DepartmentID, &e.PositionID, &e.ManagerID, &e.HireDate, &e.Status)
	return e, err
}

func (s *Service) ListDepartments(ctx context.Context, companyID int64) ([]Department, error) {
	rows, err := s.pool.Query(ctx, `SELECT d.id, d.company_id, d.code, d.name, COUNT(e.id)::int, d.created_at FROM hr_departments d LEFT JOIN hr_employees e ON e.department_id=d.id WHERE ($1=0 OR d.company_id=$1) GROUP BY d.id, d.company_id, d.code, d.name, d.created_at ORDER BY d.name`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Department
	for rows.Next() {
		var d Department
		if err := rows.Scan(&d.ID, &d.CompanyID, &d.Code, &d.Name, &d.EmployeeCount, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Service) CreateDepartment(ctx context.Context, companyID int64, code, name string) (Department, error) {
	code = strings.TrimSpace(code)
	name = strings.TrimSpace(name)
	if companyID <= 0 || code == "" || name == "" {
		return Department{}, errors.New("hr: invalid department")
	}
	var d Department
	err := s.pool.QueryRow(ctx, `INSERT INTO hr_departments(company_id, code, name) VALUES($1, $2, $3) RETURNING id, company_id, code, name, created_at`, companyID, code, name).Scan(&d.ID, &d.CompanyID, &d.Code, &d.Name, &d.CreatedAt)
	return d, err
}

func (s *Service) ListPositions(ctx context.Context, companyID int64) ([]Position, error) {
	rows, err := s.pool.Query(ctx, `SELECT p.id, p.company_id, p.code, p.name, COUNT(e.id)::int, p.created_at FROM hr_positions p LEFT JOIN hr_employees e ON e.position_id=p.id WHERE ($1=0 OR p.company_id=$1) GROUP BY p.id, p.company_id, p.code, p.name, p.created_at ORDER BY p.name`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Position
	for rows.Next() {
		var p Position
		if err := rows.Scan(&p.ID, &p.CompanyID, &p.Code, &p.Name, &p.EmployeeCount, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Service) CreatePosition(ctx context.Context, companyID int64, code, name string) (Position, error) {
	code = strings.TrimSpace(code)
	name = strings.TrimSpace(name)
	if companyID <= 0 || code == "" || name == "" {
		return Position{}, errors.New("hr: invalid position")
	}
	var p Position
	err := s.pool.QueryRow(ctx, `INSERT INTO hr_positions(company_id, code, name) VALUES($1, $2, $3) RETURNING id, company_id, code, name, created_at`, companyID, code, name).Scan(&p.ID, &p.CompanyID, &p.Code, &p.Name, &p.CreatedAt)
	return p, err
}

func (s *Service) ListAvailableUsers(ctx context.Context) ([]UserOption, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, COALESCE(name, ''), email FROM users WHERE is_active=TRUE ORDER BY email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UserOption
	for rows.Next() {
		var u UserOption
		if err := rows.Scan(&u.ID, &u.Name, &u.Email); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
