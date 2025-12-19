package companies

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/shared"
)

type Repository interface {
	List(ctx context.Context, filters shared.ListFilters) ([]Company, int, error)
	Get(ctx context.Context, id int64) (Company, error)
	Create(ctx context.Context, company Company) (Company, error)
	Update(ctx context.Context, id int64, company Company) error
	Delete(ctx context.Context, id int64) error
}

type repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &repository{db: db}
}

func (r *repository) List(ctx context.Context, filters shared.ListFilters) ([]Company, int, error) {
	query := `SELECT id, code, name, address, tax_id, created_at, updated_at FROM companies WHERE 1=1`
	args := []interface{}{}
	argCount := 0

	if filters.Search != "" {
		argCount++
		query += ` AND (name ILIKE $` + strconv.Itoa(argCount) + ` OR code ILIKE $` + strconv.Itoa(argCount) + `)`
		args = append(args, "%"+filters.Search+"%")
	}

	// Count query
	countQuery := `SELECT COUNT(*) FROM companies WHERE 1=1`
	countArgs := []interface{}{}
	if filters.Search != "" {
		countArgs = append(countArgs, "%"+filters.Search+"%")
		countQuery += ` AND (name ILIKE $1 OR code ILIKE $1)`
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

	var companies []Company
	for rows.Next() {
		var c Company
		err := rows.Scan(&c.ID, &c.Code, &c.Name, &c.Address, &c.TaxID, &c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			return nil, 0, err
		}
		companies = append(companies, c)
	}
	return companies, total, rows.Err()
}

func (r *repository) Get(ctx context.Context, id int64) (Company, error) {
	query := `SELECT id, code, name, address, tax_id, created_at, updated_at FROM companies WHERE id = $1`
	var c Company
	err := r.db.QueryRow(ctx, query, id).Scan(&c.ID, &c.Code, &c.Name, &c.Address, &c.TaxID, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func (r *repository) Create(ctx context.Context, company Company) (Company, error) {
	query := `INSERT INTO companies (code, name, address, tax_id, created_at, updated_at)
	          VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`
	now := time.Now()
	err := r.db.QueryRow(ctx, query, company.Code, company.Name, company.Address, company.TaxID, now, now).Scan(&company.ID)
	if err != nil {
		return Company{}, err
	}
	company.CreatedAt = now
	company.UpdatedAt = now
	return company, nil
}

func (r *repository) Update(ctx context.Context, id int64, company Company) error {
	query := `UPDATE companies SET code = $1, name = $2, address = $3, tax_id = $4, updated_at = $5 WHERE id = $6`
	_, err := r.db.Exec(ctx, query, company.Code, company.Name, company.Address, company.TaxID, time.Now(), id)
	return err
}

func (r *repository) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM companies WHERE id = $1`
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
	case "created_at":
		return "created_at " + dir
	default:
		return "name " + dir
	}
}
