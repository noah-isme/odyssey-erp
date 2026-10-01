package attendance

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Row struct {
	Line              int
	EmployeeNumber    string
	Date              time.Time
	CheckIn, CheckOut *time.Time
	Status            string
}

type ImportResult struct {
	ID                        int64
	Total, Accepted, Rejected int
	Errors                    []string
}

type AttendanceRecord struct {
	ID             int64
	EmployeeID     int64
	EmployeeNumber string
	EmployeeName   string
	Department     string
	AttendanceDate time.Time
	CheckIn        *time.Time
	CheckOut       *time.Time
	DurationHours  float64
	Status         string
	Source         string
	ImportID       *int64
}

type AttendanceStats struct {
	TotalActive int
	Present     int
	Absent      int
	OnLeave     int
}

type AttendanceFilter struct {
	Date         time.Time
	DepartmentID *int64
	Status       string
	Search       string
}

type ManualRecordInput struct {
	EmployeeID int64
	Date       time.Time
	CheckIn    *time.Time
	CheckOut   *time.Time
	Status     string
}

type EmployeeOption struct {
	ID             int64
	EmployeeNumber string
	Name           string
	Department     string
}

type DepartmentOption struct {
	ID   int64
	Code string
	Name string
}

type db interface {
	Begin(context.Context) (pgx.Tx, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

type Service struct{ pool db }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func ParseCSV(reader io.Reader) ([]Row, []string, error) {
	records, err := csv.NewReader(reader).ReadAll()
	if err != nil {
		return nil, nil, err
	}
	if len(records) == 0 {
		return nil, nil, errors.New("hr: empty attendance CSV")
	}
	header := map[string]int{}
	for i, v := range records[0] {
		header[strings.ToLower(strings.TrimSpace(v))] = i
	}
	for _, key := range []string{"employee_number", "date"} {
		if _, ok := header[key]; !ok {
			return nil, nil, fmt.Errorf("hr: missing %s column", key)
		}
	}
	var rows []Row
	var problems []string
	seen := make(map[string]struct{})
	for i, record := range records[1:] {
		get := func(k string) string {
			idx, ok := header[k]
			if !ok || idx >= len(record) {
				return ""
			}
			return strings.TrimSpace(record[idx])
		}
		row := Row{Line: i + 2, EmployeeNumber: get("employee_number"), Status: strings.ToUpper(get("status"))}
		if row.Status == "" {
			row.Status = "PRESENT"
		}
		row.Date, err = time.Parse("2006-01-02", get("date"))
		if err != nil || row.EmployeeNumber == "" {
			problems = append(problems, fmt.Sprintf("line %d: employee_number and YYYY-MM-DD date are required", row.Line))
			continue
		}
		key := row.EmployeeNumber + "\x00" + row.Date.Format("2006-01-02")
		if _, exists := seen[key]; exists {
			problems = append(problems, fmt.Sprintf("line %d: duplicate employee/date row", row.Line))
			continue
		}
		seen[key] = struct{}{}
		parseTime := func(v string) (*time.Time, error) {
			if v == "" {
				return nil, nil
			}
			for _, layout := range []string{time.RFC3339, "2006-01-02 15:04"} {
				if t, e := time.Parse(layout, v); e == nil {
					return &t, nil
				}
			}
			return nil, errors.New("invalid time")
		}
		row.CheckIn, err = parseTime(get("check_in"))
		if err == nil {
			row.CheckOut, err = parseTime(get("check_out"))
		}
		if err != nil || (row.CheckIn != nil && row.CheckOut != nil && row.CheckOut.Before(*row.CheckIn)) || (row.Status != "PRESENT" && row.Status != "ABSENT" && row.Status != "LEAVE") {
			problems = append(problems, fmt.Sprintf("line %d: invalid time or status", row.Line))
			continue
		}
		rows = append(rows, row)
	}
	return rows, problems, nil
}

func (s *Service) Import(ctx context.Context, companyID, userID int64, filename string, reader io.Reader) (ImportResult, error) {
	rows, problems, err := ParseCSV(reader)
	if err != nil {
		return ImportResult{}, err
	}
	result := ImportResult{Total: len(rows) + len(problems), Rejected: len(problems), Errors: problems}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	payload, _ := json.Marshal(problems)
	err = tx.QueryRow(ctx, `INSERT INTO hr_attendance_imports(company_id,filename,imported_by,total_rows,rejected_rows,errors) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, companyID, filename, userID, result.Total, result.Rejected, payload).Scan(&result.ID)
	if err != nil {
		return result, err
	}
	for _, row := range rows {
		var employeeID int64
		err = tx.QueryRow(ctx, `SELECT id FROM hr_employees WHERE company_id=$1 AND employee_number=$2 AND status='ACTIVE'`, companyID, row.EmployeeNumber).Scan(&employeeID)
		if err != nil {
			result.Rejected++
			result.Errors = append(result.Errors, fmt.Sprintf("line %d: employee not found", row.Line))
			continue
		}
		_, err = tx.Exec(ctx, `INSERT INTO hr_attendance(employee_id,attendance_date,check_in,check_out,status,source,import_id) VALUES($1,$2,$3,$4,$5,'CSV',$6) ON CONFLICT(employee_id,attendance_date) DO UPDATE SET check_in=EXCLUDED.check_in,check_out=EXCLUDED.check_out,status=EXCLUDED.status,source='CSV',import_id=EXCLUDED.import_id,updated_at=NOW()`, employeeID, row.Date, row.CheckIn, row.CheckOut, row.Status, result.ID)
		if err != nil {
			return result, err
		}
		result.Accepted++
	}
	result.Rejected = result.Total - result.Accepted
	payload, _ = json.Marshal(result.Errors)
	_, err = tx.Exec(ctx, `UPDATE hr_attendance_imports SET accepted_rows=$2,rejected_rows=$3,errors=$4 WHERE id=$1`, result.ID, result.Accepted, result.Rejected, payload)
	if err != nil {
		return result, err
	}
	if err = tx.Commit(ctx); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Service) Recent(ctx context.Context, companyID int64) ([]ImportResult, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,total_rows,accepted_rows,rejected_rows,errors FROM hr_attendance_imports WHERE company_id=$1 ORDER BY created_at DESC LIMIT 20`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ImportResult
	for rows.Next() {
		var x ImportResult
		var raw []byte
		if err := rows.Scan(&x.ID, &x.Total, &x.Accepted, &x.Rejected, &raw); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &x.Errors)
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Service) ListRecords(ctx context.Context, companyID int64, filter AttendanceFilter) ([]AttendanceRecord, AttendanceStats, error) {
	if filter.Date.IsZero() {
		filter.Date = time.Now()
	}
	targetDate := time.Date(filter.Date.Year(), filter.Date.Month(), filter.Date.Day(), 0, 0, 0, 0, time.UTC)

	query := `SELECT a.id, a.employee_id, e.employee_number, e.name, COALESCE(d.name,''), a.attendance_date, a.check_in, a.check_out, a.status, a.source, a.import_id FROM hr_attendance a JOIN hr_employees e ON e.id=a.employee_id LEFT JOIN hr_departments d ON d.id=e.department_id WHERE ($1=0 OR e.company_id=$1) AND a.attendance_date=$2`
	args := []any{companyID, targetDate}

	if filter.DepartmentID != nil && *filter.DepartmentID > 0 {
		args = append(args, *filter.DepartmentID)
		query += fmt.Sprintf(" AND e.department_id=$%d", len(args))
	}
	if filter.Status != "" && filter.Status != "ALL" {
		args = append(args, strings.ToUpper(strings.TrimSpace(filter.Status)))
		query += fmt.Sprintf(" AND a.status=$%d", len(args))
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		pattern := "%" + strings.ToLower(search) + "%"
		args = append(args, pattern)
		query += fmt.Sprintf(" AND (LOWER(e.name) LIKE $%d OR LOWER(e.employee_number) LIKE $%d)", len(args), len(args))
	}

	query += ` ORDER BY e.name`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, AttendanceStats{}, err
	}
	defer rows.Close()

	var records []AttendanceRecord
	for rows.Next() {
		var r AttendanceRecord
		if err := rows.Scan(&r.ID, &r.EmployeeID, &r.EmployeeNumber, &r.EmployeeName, &r.Department, &r.AttendanceDate, &r.CheckIn, &r.CheckOut, &r.Status, &r.Source, &r.ImportID); err != nil {
			return nil, AttendanceStats{}, err
		}
		if r.CheckIn != nil && r.CheckOut != nil && !r.CheckOut.Before(*r.CheckIn) {
			r.DurationHours = r.CheckOut.Sub(*r.CheckIn).Hours()
		}
		records = append(records, r)
	}

	var stats AttendanceStats
	_ = s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM hr_employees WHERE ($1=0 OR company_id=$1) AND status='ACTIVE'`, companyID).Scan(&stats.TotalActive)
	_ = s.pool.QueryRow(ctx, `SELECT 
		COUNT(*) FILTER (WHERE a.status='PRESENT'),
		COUNT(*) FILTER (WHERE a.status='ABSENT'),
		COUNT(*) FILTER (WHERE a.status='LEAVE')
		FROM hr_attendance a JOIN hr_employees e ON e.id=a.employee_id WHERE ($1=0 OR e.company_id=$1) AND a.attendance_date=$2`, companyID, targetDate).Scan(&stats.Present, &stats.Absent, &stats.OnLeave)

	return records, stats, rows.Err()
}

func (s *Service) RecordManual(ctx context.Context, companyID int64, in ManualRecordInput) error {
	if companyID <= 0 || in.EmployeeID <= 0 || in.Date.IsZero() {
		return errors.New("hr: invalid attendance record parameters")
	}
	in.Status = strings.ToUpper(strings.TrimSpace(in.Status))
	if in.Status == "" {
		in.Status = "PRESENT"
	}
	if in.Status != "PRESENT" && in.Status != "ABSENT" && in.Status != "LEAVE" {
		return errors.New("hr: invalid attendance status")
	}
	if in.CheckIn != nil && in.CheckOut != nil && in.CheckOut.Before(*in.CheckIn) {
		return errors.New("hr: check-out cannot be before check-in")
	}

	targetDate := time.Date(in.Date.Year(), in.Date.Month(), in.Date.Day(), 0, 0, 0, 0, time.UTC)

	tag, err := s.pool.Exec(ctx, `INSERT INTO hr_attendance(employee_id, attendance_date, check_in, check_out, status, source)
		SELECT e.id, $2, $3, $4, $5, 'MANUAL'
		FROM hr_employees e
		WHERE e.id=$1 AND e.company_id=$6
		ON CONFLICT(employee_id, attendance_date) DO UPDATE SET check_in=EXCLUDED.check_in, check_out=EXCLUDED.check_out, status=EXCLUDED.status, source='MANUAL', updated_at=NOW()`,
		in.EmployeeID, targetDate, in.CheckIn, in.CheckOut, in.Status, companyID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("hr: employee not found in current company")
	}
	return nil
}

func (s *Service) CSVTemplate() []byte {
	return []byte("employee_number,date,check_in,check_out,status\nEMP-001,2026-09-18,2026-09-18 08:30,2026-09-18 17:30,PRESENT\nEMP-002,2026-09-18,,,ABSENT\nEMP-003,2026-09-18,,,LEAVE\n")
}

func (s *Service) ListEmployees(ctx context.Context, companyID int64) ([]EmployeeOption, error) {
	rows, err := s.pool.Query(ctx, `SELECT e.id, e.employee_number, e.name, COALESCE(d.name,'') FROM hr_employees e LEFT JOIN hr_departments d ON d.id=e.department_id WHERE ($1=0 OR e.company_id=$1) AND e.status='ACTIVE' ORDER BY e.name`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EmployeeOption
	for rows.Next() {
		var o EmployeeOption
		if err := rows.Scan(&o.ID, &o.EmployeeNumber, &o.Name, &o.Department); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Service) ListDepartments(ctx context.Context, companyID int64) ([]DepartmentOption, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, code, name FROM hr_departments WHERE ($1=0 OR company_id=$1) ORDER BY name`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DepartmentOption
	for rows.Next() {
		var o DepartmentOption
		if err := rows.Scan(&o.ID, &o.Code, &o.Name); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func ParseCompany(raw string) int64 { id, _ := strconv.ParseInt(raw, 10, 64); return id }
