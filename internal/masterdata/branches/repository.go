package branches

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/shared"
)

type Repository interface {
	List(ctx context.Context, filters shared.ListFilters) ([]Branch, int, error)
	Get(ctx context.Context, id int64) (Branch, error)
	Create(ctx context.Context, branch Branch) (Branch, error)
	Update(ctx context.Context, id int64, branch Branch) error
	Delete(ctx context.Context, id int64) error
}

type repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &repository{db: db}
}

func (r *repository) List(ctx context.Context, filters shared.ListFilters) ([]Branch, int, error) {
	query := `SELECT id, company_id, code, name, address FROM branches WHERE 1=1`
	args := []interface{}{}
	argCount := 0

	if filters.CompanyID != nil {
		argCount++
		query += ` AND company_id = $` + strconv.Itoa(argCount)
		args = append(args, *filters.CompanyID)
	}

	if filters.Search != "" {
		argCount++
		query += ` AND (name ILIKE $` + strconv.Itoa(argCount) + ` OR code ILIKE $` + strconv.Itoa(argCount) + `)`
		args = append(args, "%"+filters.Search+"%")
	}

	// Count
	countQuery := `SELECT COUNT(*) FROM branches WHERE 1=1`
	countArgs := []interface{}{}
	countArgCount := 0
	
	if filters.CompanyID != nil {
		countArgCount++
		countQuery += ` AND company_id = $` + strconv.Itoa(countArgCount)
		countArgs = append(countArgs, *filters.CompanyID)
	}
	if filters.Search != "" {
		countArgCount++
		countQuery += ` AND (name ILIKE $` + strconv.Itoa(countArgCount) + ` OR code ILIKE $` + strconv.Itoa(countArgCount) + `)`
		countArgs = append(countArgs, "%"+filters.Search+"%")
	}

	var total int
	err := r.db.QueryRow(ctx, countQuery, countArgs...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Sorting
	query += " ORDER BY " + sortOrder(filters.SortBy, filters.SortDir)

	// Pagination
	if filters.Limit > 0 {
		argCount++
		query += ` LIMIT $` + strconv.Itoa(argCount)
		args = append(args, filters.Limit)
		
		argCount++
		query += ` OFFSET $` + strconv.Itoa(argCount)
		offset := (filters.Page - 1) * filters.Limit
		if offset < 0 { offset = 0 }
		args = append(args, offset)
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var branches []Branch
	for rows.Next() {
		var b Branch
		err := rows.Scan(&b.ID, &b.CompanyID, &b.Code, &b.Name, &b.Address)
		if err != nil {
			return nil, 0, err
		}
		branches = append(branches, b)
	}
	return branches, total, rows.Err()
}

func (r *repository) Get(ctx context.Context, id int64) (Branch, error) {
	query := `SELECT id, company_id, code, name, address FROM branches WHERE id = $1`
	var b Branch
	err := r.db.QueryRow(ctx, query, id).Scan(&b.ID, &b.CompanyID, &b.Code, &b.Name, &b.Address)
	return b, err
}

func (r *repository) Create(ctx context.Context, branch Branch) (Branch, error) {
	query := `INSERT INTO branches (company_id, code, name, address) VALUES ($1, $2, $3, $4) RETURNING id`
	err := r.db.QueryRow(ctx, query, branch.CompanyID, branch.Code, branch.Name, branch.Address).Scan(&branch.ID)
	return branch, err
}

func (r *repository) Update(ctx context.Context, id int64, branch Branch) error {
	query := `UPDATE branches SET company_id = $1, code = $2, name = $3, address = $4 WHERE id = $5`
	_, err := r.db.Exec(ctx, query, branch.CompanyID, branch.Code, branch.Name, branch.Address, id)
	return err
}

func (r *repository) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM branches WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id)
	return err
}

func sortOrder(sortBy, sortDir string) string {
	dir := "ASC"
	if sortDir == "desc" {
		dir = "DESC"
	}
	switch sortBy {
	case "code":
		return "code " + dir
	case "name":
		return "name " + dir
	default:
		return "name " + dir
	}
}
