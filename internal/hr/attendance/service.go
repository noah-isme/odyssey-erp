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
type db interface {
	Begin(context.Context) (pgx.Tx, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
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
func ParseCompany(raw string) int64 { id, _ := strconv.ParseInt(raw, 10, 64); return id }
