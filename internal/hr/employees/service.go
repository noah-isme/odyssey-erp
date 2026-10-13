package employees

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/hr/compensation"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// Audit actions recorded by this package. Audit metadata only ever carries
// field names and ids, never profile, salary, or bank values.
const (
	ActionEmployeeCreated        = "EMPLOYEE_CREATED"
	ActionEmployeeProfileUpdated = "EMPLOYEE_PROFILE_UPDATED"
	ActionEmployeeJobChanged     = "EMPLOYEE_JOB_CHANGED"
	ActionEmployeeRehired        = "EMPLOYEE_REHIRED"
	ActionEmployeeContactUpsert  = "EMPLOYEE_CONTACT_UPSERTED"
	ActionEmployeeContactDeleted = "EMPLOYEE_CONTACT_DELETED"
	auditEntity                  = "hr_employee"
)

// Employment types and statuses accepted by hr_employees / hr_employee_job_history.
const (
	EmploymentPermanent = "PERMANENT"
	EmploymentContract  = "CONTRACT"
	EmploymentProbation = "PROBATION"
	EmploymentIntern    = "INTERN"
	StatusActive        = "ACTIVE"
	StatusInactive      = "INACTIVE"
)

var (
	errCompanyRequired    = errors.New("hr: company required")
	errAuditNotConfigured = errors.New("hr: employee audit not configured")

	// ErrNotFound is returned (possibly wrapped) when an id does not exist in
	// the caller's company. Cross-tenant ids are indistinguishable from
	// missing ones.
	ErrNotFound              = errors.New("hr: not found")
	ErrDepartmentNotFound    = fmt.Errorf("%w: department", ErrNotFound)
	ErrPositionNotFound      = fmt.Errorf("%w: position", ErrNotFound)
	ErrManagerNotFound       = fmt.Errorf("%w: manager", ErrNotFound)
	ErrEmployeeNotFound      = fmt.Errorf("%w: employee", ErrNotFound)
	ErrContactNotFound       = fmt.Errorf("%w: emergency contact", ErrNotFound)
	ErrNoJobAssignment       = fmt.Errorf("%w: job assignment as of date", ErrNotFound)
	ErrInvalidEmployee       = errors.New("hr: invalid employee")
	ErrInvalidStatus         = errors.New("hr: invalid employee status")
	ErrInvalidEmploymentType = errors.New("hr: invalid employment type")
	ErrInvalidGender         = errors.New("hr: invalid gender")
	ErrInvalidMaritalStatus  = errors.New("hr: invalid marital status")
	ErrInvalidDependents     = errors.New("hr: dependents count cannot be negative")
	ErrInvalidContractDates  = errors.New("hr: contract end date is before contract start date")
	ErrSelfManager           = errors.New("hr: employee cannot manage themselves")
	ErrCompensationDisabled  = errors.New("hr: compensation is not configured")
)

// Profile holds the personal and contract fields from migration 000130.
// Gender and MaritalStatus are "" when unset (NULL in the database).
// NationalID, TaxID, DateOfBirth, and the address fields are sensitive; see
// MaskSensitive.
type Profile struct {
	DateOfBirth       *time.Time
	Gender            string
	NationalID        string
	TaxID             string
	MaritalStatus     string
	DependentsCount   int
	Phone             string
	AddressLine1      string
	AddressLine2      string
	City              string
	Province          string
	PostalCode        string
	Country           string
	ContractStartDate *time.Time
	ContractEndDate   *time.Time
	ProbationEndDate  *time.Time
}

type Employee struct {
	ID, CompanyID                                                      int64
	UserID, DepartmentID, PositionID, ManagerID                        *int64
	EmployeeNumber, Name, Email, Department, Position, Manager, Status string
	UserEmail                                                          string
	HireDate                                                           time.Time
	Profile
	EmploymentType    string
	TerminationDate   *time.Time
	TerminationReason string
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

// CreateInput creates an employee, its initial job history row, and
// optionally its first compensation assignment, in one transaction.
type CreateInput struct {
	CompanyID                                   int64
	UserID, DepartmentID, PositionID, ManagerID *int64
	EmployeeNumber, Name, Email                 string
	HireDate                                    time.Time
	Profile
	EmploymentType string // default PERMANENT
	Grade          string
	// Compensation, when set, creates the first payroll_compensation_assignments
	// row. EffectiveFrom defaults to HireDate, Reason to "Hired", ActorID to
	// CreateInput.ActorID.
	Compensation *compensation.ChangeInput
	ActorID      int64
}

// UpdateInput changes identity and profile fields. Department, position,
// manager, status, and employment type are job fields and change only through
// RecordJobChange. Sensitive fields (NationalID, TaxID, DateOfBirth, address)
// are written only when IncludeSensitive is true; otherwise the stored values
// are kept.
type UpdateInput struct {
	UserID                      *int64
	EmployeeNumber, Name, Email string
	HireDate                    time.Time
	Profile
	IncludeSensitive bool
	ActorID          int64
}

// DBTX is the query surface shared by the pool and transactions.
type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type db interface {
	DBTX
	Begin(context.Context) (pgx.Tx, error)
}

// Audit is satisfied by *shared.AuditLogger.
type Audit interface {
	RecordTx(context.Context, pgx.Tx, shared.AuditLog) error
}

// CompensationCreator creates the first compensation assignment inside the
// employee-creation transaction. *compensation.Service satisfies it.
type CompensationCreator interface {
	CreateInitialTx(ctx context.Context, tx compensation.DBTX, companyID, employeeID int64, in compensation.ChangeInput) (compensation.Assignment, error)
}

var _ CompensationCreator = (*compensation.Service)(nil)

type Service struct {
	pool         db
	audit        Audit
	compensation CompensationCreator
}

func NewService(pool *pgxpool.Pool, audit Audit) *Service {
	s := &Service{audit: audit}
	if pool != nil {
		s.pool = pool
	}
	return s
}

// SetCompensationCreator enables CreateInput.Compensation.
func (s *Service) SetCompensationCreator(c CompensationCreator) { s.compensation = c }

const employeeSelect = `SELECT e.id,e.company_id,e.user_id,e.employee_number,e.name,e.email,e.department_id,e.position_id,e.manager_id,e.hire_date,e.status,COALESCE(d.name,''),COALESCE(p.name,''),COALESCE(m.name,''),
e.date_of_birth,COALESCE(e.gender,''),e.national_id,e.tax_id,COALESCE(e.marital_status,''),e.dependents_count,e.phone,e.address_line1,e.address_line2,e.city,e.province,e.postal_code,e.country,
e.employment_type,e.contract_start_date,e.contract_end_date,e.probation_end_date,e.termination_date,e.termination_reason
FROM hr_employees e LEFT JOIN hr_departments d ON d.id=e.department_id LEFT JOIN hr_positions p ON p.id=e.position_id LEFT JOIN hr_employees m ON m.id=e.manager_id`

func scanEmployee(row pgx.Row) (Employee, error) {
	var e Employee
	err := row.Scan(&e.ID, &e.CompanyID, &e.UserID, &e.EmployeeNumber, &e.Name, &e.Email, &e.DepartmentID, &e.PositionID, &e.ManagerID, &e.HireDate, &e.Status, &e.Department, &e.Position, &e.Manager,
		&e.DateOfBirth, &e.Gender, &e.NationalID, &e.TaxID, &e.MaritalStatus, &e.DependentsCount, &e.Phone, &e.AddressLine1, &e.AddressLine2, &e.City, &e.Province, &e.PostalCode, &e.Country,
		&e.EmploymentType, &e.ContractStartDate, &e.ContractEndDate, &e.ProbationEndDate, &e.TerminationDate, &e.TerminationReason)
	return e, err
}

// MaskSensitive blanks national_id, tax_id, date of birth, and address fields.
// Callers without hr.employee.sensitive must call it before rendering or
// exporting an Employee.
func MaskSensitive(e *Employee) {
	if e == nil {
		return
	}
	e.NationalID = ""
	e.TaxID = ""
	e.DateOfBirth = nil
	e.AddressLine1 = ""
	e.AddressLine2 = ""
	e.City = ""
	e.Province = ""
	e.PostalCode = ""
	e.Country = ""
}

func (s *Service) List(ctx context.Context, companyID int64, filter ...EmployeeFilter) ([]Employee, error) {
	if companyID <= 0 {
		return nil, errCompanyRequired
	}
	query := employeeSelect + ` WHERE e.company_id=$1`
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
		e, err := scanEmployee(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Service) GetByID(ctx context.Context, companyID, id int64) (Employee, error) {
	if companyID <= 0 {
		return Employee{}, errCompanyRequired
	}
	return scanEmployee(s.pool.QueryRow(ctx, employeeSelect+` WHERE e.id=$1 AND e.company_id=$2`, id, companyID))
}

func (s *Service) Create(ctx context.Context, in CreateInput) (Employee, error) {
	in.EmployeeNumber = strings.TrimSpace(in.EmployeeNumber)
	in.Name = strings.TrimSpace(in.Name)
	in.Email = strings.TrimSpace(in.Email)
	in.Grade = strings.TrimSpace(in.Grade)
	in.EmploymentType = strings.ToUpper(strings.TrimSpace(in.EmploymentType))
	if in.EmploymentType == "" {
		in.EmploymentType = EmploymentPermanent
	}
	if in.CompanyID <= 0 || in.EmployeeNumber == "" || in.Name == "" || in.Email == "" || in.HireDate.IsZero() {
		return Employee{}, ErrInvalidEmployee
	}
	if !validEmploymentType(in.EmploymentType) {
		return Employee{}, ErrInvalidEmploymentType
	}
	if err := normalizeProfile(&in.Profile); err != nil {
		return Employee{}, err
	}
	var comp compensation.ChangeInput
	if in.Compensation != nil {
		if s.compensation == nil {
			return Employee{}, ErrCompensationDisabled
		}
		comp = *in.Compensation
		if comp.EffectiveFrom.IsZero() {
			comp.EffectiveFrom = in.HireDate
		}
		if strings.TrimSpace(comp.Reason) == "" {
			comp.Reason = "Hired"
		}
		if comp.ActorID == 0 {
			comp.ActorID = in.ActorID
		}
	}
	if s.audit == nil {
		return Employee{}, errAuditNotConfigured
	}

	var e Employee
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if err := validateRefs(ctx, tx, in.CompanyID, in.DepartmentID, in.PositionID, in.ManagerID); err != nil {
			return err
		}
		p := in.Profile
		err := tx.QueryRow(ctx, `INSERT INTO hr_employees(user_id,company_id,employee_number,name,email,department_id,position_id,manager_id,hire_date,
date_of_birth,gender,national_id,tax_id,marital_status,dependents_count,phone,address_line1,address_line2,city,province,postal_code,country,
employment_type,contract_start_date,contract_end_date,probation_end_date)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,''),$12,$13,NULLIF($14,''),$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26)
RETURNING id,status`,
			in.UserID, in.CompanyID, in.EmployeeNumber, in.Name, in.Email, in.DepartmentID, in.PositionID, in.ManagerID, in.HireDate,
			p.DateOfBirth, p.Gender, p.NationalID, p.TaxID, p.MaritalStatus, p.DependentsCount, p.Phone, p.AddressLine1, p.AddressLine2, p.City, p.Province, p.PostalCode, p.Country,
			in.EmploymentType, p.ContractStartDate, p.ContractEndDate, p.ProbationEndDate).Scan(&e.ID, &e.Status)
		if err != nil {
			return fmt.Errorf("insert employee: %w", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO hr_employee_job_history(company_id,employee_id,department_id,position_id,manager_id,status,employment_type,grade,valid_from,reason,created_by)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'Hired',$10)`,
			in.CompanyID, e.ID, in.DepartmentID, in.PositionID, in.ManagerID, e.Status, in.EmploymentType, in.Grade, in.HireDate, nullableID(in.ActorID)); err != nil {
			return fmt.Errorf("insert initial job history: %w", err)
		}
		if in.Compensation != nil {
			if _, err := s.compensation.CreateInitialTx(ctx, tx, in.CompanyID, e.ID, comp); err != nil {
				return err
			}
		}
		return s.recordTx(ctx, tx, in.ActorID, ActionEmployeeCreated, e.ID, map[string]any{
			"fields":       createdFields(in),
			"compensation": in.Compensation != nil,
		})
	})
	if err != nil {
		return Employee{}, err
	}
	e.CompanyID = in.CompanyID
	e.UserID, e.DepartmentID, e.PositionID, e.ManagerID = in.UserID, in.DepartmentID, in.PositionID, in.ManagerID
	e.EmployeeNumber, e.Name, e.Email, e.HireDate = in.EmployeeNumber, in.Name, in.Email, in.HireDate
	e.Profile = in.Profile
	e.EmploymentType = in.EmploymentType
	return e, nil
}

// Update changes identity and profile fields and audits the names of the
// fields that changed. It never changes job fields; use RecordJobChange.
func (s *Service) Update(ctx context.Context, companyID, id int64, in UpdateInput) (Employee, error) {
	if err := s.validateUpdate(companyID, id, &in); err != nil {
		return Employee{}, err
	}
	var out Employee
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = s.updateTx(ctx, tx, companyID, id, in)
		return err
	})
	if err != nil {
		return Employee{}, err
	}
	return out, nil
}

// validateUpdate normalizes and validates an update without touching the
// database.
func (s *Service) validateUpdate(companyID, id int64, in *UpdateInput) error {
	in.EmployeeNumber = strings.TrimSpace(in.EmployeeNumber)
	in.Name = strings.TrimSpace(in.Name)
	in.Email = strings.TrimSpace(in.Email)
	if companyID <= 0 || id <= 0 || in.EmployeeNumber == "" || in.Name == "" || in.Email == "" || in.HireDate.IsZero() {
		return ErrInvalidEmployee
	}
	if err := normalizeProfile(&in.Profile); err != nil {
		return err
	}
	if s.audit == nil {
		return errAuditNotConfigured
	}
	return nil
}

// updateTx is Update inside a caller-owned transaction; in must have passed
// validateUpdate.
func (s *Service) updateTx(ctx context.Context, tx pgx.Tx, companyID, id int64, in UpdateInput) (Employee, error) {
	old, err := scanEmployee(tx.QueryRow(ctx, employeeSelect+` WHERE e.id=$1 AND e.company_id=$2 FOR UPDATE OF e`, id, companyID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Employee{}, ErrEmployeeNotFound
	}
	if err != nil {
		return Employee{}, fmt.Errorf("lock employee: %w", err)
	}
	next := old
	next.UserID = in.UserID
	next.EmployeeNumber, next.Name, next.Email, next.HireDate = in.EmployeeNumber, in.Name, in.Email, in.HireDate
	applyProfile(&next.Profile, in.Profile, in.IncludeSensitive)
	p := next.Profile
	if _, err := tx.Exec(ctx, `UPDATE hr_employees SET user_id=$1, employee_number=$2, name=$3, email=$4, hire_date=$5,
date_of_birth=$6, gender=NULLIF($7,''), national_id=$8, tax_id=$9, marital_status=NULLIF($10,''), dependents_count=$11, phone=$12,
address_line1=$13, address_line2=$14, city=$15, province=$16, postal_code=$17, country=$18,
contract_start_date=$19, contract_end_date=$20, probation_end_date=$21, updated_at=NOW()
WHERE id=$22 AND company_id=$23`,
		next.UserID, next.EmployeeNumber, next.Name, next.Email, next.HireDate,
		p.DateOfBirth, p.Gender, p.NationalID, p.TaxID, p.MaritalStatus, p.DependentsCount, p.Phone,
		p.AddressLine1, p.AddressLine2, p.City, p.Province, p.PostalCode, p.Country,
		p.ContractStartDate, p.ContractEndDate, p.ProbationEndDate, id, companyID); err != nil {
		return Employee{}, fmt.Errorf("update employee: %w", err)
	}
	if changed := changedEmployeeFields(old, next); len(changed) > 0 {
		if err := s.recordTx(ctx, tx, in.ActorID, ActionEmployeeProfileUpdated, id, map[string]any{"fields": changed}); err != nil {
			return Employee{}, err
		}
	}
	return next, nil
}

// inTx runs fn in a transaction, committing on success.
func (s *Service) inTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (s *Service) recordTx(ctx context.Context, tx pgx.Tx, actorID int64, action string, employeeID int64, meta map[string]any) error {
	if s.audit == nil {
		return errAuditNotConfigured
	}
	if err := s.audit.RecordTx(ctx, tx, shared.AuditLog{
		ActorID:  actorID,
		Action:   action,
		Entity:   auditEntity,
		EntityID: strconv.FormatInt(employeeID, 10),
		Meta:     meta,
	}); err != nil {
		return fmt.Errorf("audit %s: %w", action, err)
	}
	return nil
}

// validateRefs checks that department, position, and manager ids (when set)
// belong to the company.
func validateRefs(ctx context.Context, q DBTX, companyID int64, departmentID, positionID, managerID *int64) error {
	checks := []struct {
		id    *int64
		query string
		err   error
	}{
		{departmentID, `SELECT 1 FROM hr_departments WHERE id=$1 AND company_id=$2`, ErrDepartmentNotFound},
		{positionID, `SELECT 1 FROM hr_positions WHERE id=$1 AND company_id=$2`, ErrPositionNotFound},
		{managerID, `SELECT 1 FROM hr_employees WHERE id=$1 AND company_id=$2`, ErrManagerNotFound},
	}
	for _, c := range checks {
		if c.id == nil {
			continue
		}
		var one int
		err := q.QueryRow(ctx, c.query, *c.id, companyID).Scan(&one)
		if errors.Is(err, pgx.ErrNoRows) {
			return c.err
		}
		if err != nil {
			return fmt.Errorf("validate reference: %w", err)
		}
	}
	return nil
}

func validEmploymentType(v string) bool {
	switch v {
	case EmploymentPermanent, EmploymentContract, EmploymentProbation, EmploymentIntern:
		return true
	}
	return false
}

func validStatus(v string) bool { return v == StatusActive || v == StatusInactive }

func normalizeProfile(p *Profile) error {
	p.Gender = strings.ToUpper(strings.TrimSpace(p.Gender))
	p.MaritalStatus = strings.ToUpper(strings.TrimSpace(p.MaritalStatus))
	for _, f := range []*string{&p.NationalID, &p.TaxID, &p.Phone, &p.AddressLine1, &p.AddressLine2, &p.City, &p.Province, &p.PostalCode, &p.Country} {
		*f = strings.TrimSpace(*f)
	}
	switch p.Gender {
	case "", "MALE", "FEMALE", "OTHER":
	default:
		return ErrInvalidGender
	}
	switch p.MaritalStatus {
	case "", "SINGLE", "MARRIED", "DIVORCED", "WIDOWED":
	default:
		return ErrInvalidMaritalStatus
	}
	if p.DependentsCount < 0 {
		return ErrInvalidDependents
	}
	if p.ContractStartDate != nil && p.ContractEndDate != nil && p.ContractEndDate.Before(*p.ContractStartDate) {
		return ErrInvalidContractDates
	}
	return nil
}

// applyProfile copies in onto dst, leaving sensitive fields untouched unless
// includeSensitive is set.
func applyProfile(dst *Profile, in Profile, includeSensitive bool) {
	if !includeSensitive {
		in.NationalID, in.TaxID, in.DateOfBirth = dst.NationalID, dst.TaxID, dst.DateOfBirth
		in.AddressLine1, in.AddressLine2, in.City = dst.AddressLine1, dst.AddressLine2, dst.City
		in.Province, in.PostalCode, in.Country = dst.Province, dst.PostalCode, dst.Country
	}
	*dst = in
}

func sameID(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func sameDate(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Format("2006-01-02") == b.Format("2006-01-02")
}

// changedEmployeeFields returns the column names whose values differ.
func changedEmployeeFields(old, next Employee) []string {
	var out []string
	add := func(changed bool, name string) {
		if changed {
			out = append(out, name)
		}
	}
	add(!sameID(old.UserID, next.UserID), "user_id")
	add(old.EmployeeNumber != next.EmployeeNumber, "employee_number")
	add(old.Name != next.Name, "name")
	add(old.Email != next.Email, "email")
	add(!sameDate(&old.HireDate, &next.HireDate), "hire_date")
	o, n := old.Profile, next.Profile
	add(!sameDate(o.DateOfBirth, n.DateOfBirth), "date_of_birth")
	add(o.Gender != n.Gender, "gender")
	add(o.NationalID != n.NationalID, "national_id")
	add(o.TaxID != n.TaxID, "tax_id")
	add(o.MaritalStatus != n.MaritalStatus, "marital_status")
	add(o.DependentsCount != n.DependentsCount, "dependents_count")
	add(o.Phone != n.Phone, "phone")
	add(o.AddressLine1 != n.AddressLine1, "address_line1")
	add(o.AddressLine2 != n.AddressLine2, "address_line2")
	add(o.City != n.City, "city")
	add(o.Province != n.Province, "province")
	add(o.PostalCode != n.PostalCode, "postal_code")
	add(o.Country != n.Country, "country")
	add(!sameDate(o.ContractStartDate, n.ContractStartDate), "contract_start_date")
	add(!sameDate(o.ContractEndDate, n.ContractEndDate), "contract_end_date")
	add(!sameDate(o.ProbationEndDate, n.ProbationEndDate), "probation_end_date")
	return out
}

// createdFields lists the optional fields supplied on create (names only).
func createdFields(in CreateInput) []string {
	empty := Employee{HireDate: in.HireDate}
	filled := Employee{HireDate: in.HireDate, UserID: in.UserID, Profile: in.Profile}
	out := changedEmployeeFields(empty, filled)
	for _, f := range []struct {
		set  bool
		name string
	}{{in.DepartmentID != nil, "department_id"}, {in.PositionID != nil, "position_id"}, {in.ManagerID != nil, "manager_id"}, {in.Grade != "", "grade"}} {
		if f.set {
			out = append(out, f.name)
		}
	}
	return append(out, "employment_type")
}

func nullableID(id int64) *int64 {
	if id <= 0 {
		return nil
	}
	return &id
}

func (s *Service) ListDepartments(ctx context.Context, companyID int64) ([]Department, error) {
	if companyID <= 0 {
		return nil, errCompanyRequired
	}
	rows, err := s.pool.Query(ctx, `SELECT d.id, d.company_id, d.code, d.name, COUNT(e.id)::int, d.created_at FROM hr_departments d LEFT JOIN hr_employees e ON e.department_id=d.id WHERE d.company_id=$1 GROUP BY d.id, d.company_id, d.code, d.name, d.created_at ORDER BY d.name`, companyID)
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
	if companyID <= 0 {
		return nil, errCompanyRequired
	}
	rows, err := s.pool.Query(ctx, `SELECT p.id, p.company_id, p.code, p.name, COUNT(e.id)::int, p.created_at FROM hr_positions p LEFT JOIN hr_employees e ON e.position_id=p.id WHERE p.company_id=$1 GROUP BY p.id, p.company_id, p.code, p.name, p.created_at ORDER BY p.name`, companyID)
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
