package companies

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	masterdatadb "github.com/odyssey-erp/odyssey-erp/internal/masterdata/db"
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
	pool    *pgxpool.Pool
	queries *masterdatadb.Queries
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &repository{
		pool:    pool,
		queries: masterdatadb.New(pool),
	}
}

// List uses dynamic query (not sqlc) due to filter complexity
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
	err := r.pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total)
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
		if offset < 0 {
			offset = 0
		}
		args = append(args, offset)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var companies []Company
	for rows.Next() {
		var c Company
		var createdAt, updatedAt pgtype.Timestamptz
		err := rows.Scan(&c.ID, &c.Code, &c.Name, &c.Address, &c.TaxID, &createdAt, &updatedAt)
		if err != nil {
			return nil, 0, err
		}
		if createdAt.Valid {
			c.CreatedAt = createdAt.Time
		}
		if updatedAt.Valid {
			c.UpdatedAt = updatedAt.Time
		}
		companies = append(companies, c)
	}
	return companies, total, rows.Err()
}

// Get uses sqlc generated query
func (r *repository) Get(ctx context.Context, id int64) (Company, error) {
	row, err := r.queries.GetCompany(ctx, id)
	if err != nil {
		return Company{}, err
	}
	c := Company{
		ID:      row.ID,
		Code:    row.Code,
		Name:    row.Name,
		Address: row.Address,
		TaxID:   row.TaxID,
	}
	if row.CreatedAt.Valid {
		c.CreatedAt = row.CreatedAt.Time
	}
	if row.UpdatedAt.Valid {
		c.UpdatedAt = row.UpdatedAt.Time
	}
	return c, nil
}

// Create uses sqlc generated query
func (r *repository) Create(ctx context.Context, company Company) (Company, error) {
	now := time.Now()
	row, err := r.queries.CreateCompany(ctx, masterdatadb.CreateCompanyParams{
		Code:      company.Code,
		Name:      company.Name,
		Address:   company.Address,
		TaxID:     company.TaxID,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		UpdatedAt: pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return Company{}, err
	}
	return Company{
		ID:        row.ID,
		Code:      row.Code,
		Name:      row.Name,
		Address:   row.Address,
		TaxID:     row.TaxID,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// Update uses sqlc generated query
func (r *repository) Update(ctx context.Context, id int64, company Company) error {
	return r.queries.UpdateCompany(ctx, masterdatadb.UpdateCompanyParams{
		Code:      company.Code,
		Name:      company.Name,
		Address:   company.Address,
		TaxID:     company.TaxID,
		UpdatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		ID:        id,
	})
}

// Delete uses sqlc generated query
func (r *repository) Delete(ctx context.Context, id int64) error {
	return r.queries.DeleteCompany(ctx, id)
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
