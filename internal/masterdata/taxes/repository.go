package taxes

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	masterdatadb "github.com/odyssey-erp/odyssey-erp/internal/masterdata/db"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/shared"
)

type Repository interface {
	List(ctx context.Context, filters shared.ListFilters) ([]Tax, int, error)
	Get(ctx context.Context, id int64) (Tax, error)
	Create(ctx context.Context, tax Tax) (Tax, error)
	Update(ctx context.Context, id int64, tax Tax) error
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
func (r *repository) List(ctx context.Context, filters shared.ListFilters) ([]Tax, int, error) {
	query := `SELECT id, code, name, rate FROM taxes WHERE 1=1`
	args := []interface{}{}
	argCount := 0

	if filters.Search != "" {
		argCount++
		query += ` AND (name ILIKE $` + strconv.Itoa(argCount) + ` OR code ILIKE $` + strconv.Itoa(argCount) + `)`
		args = append(args, "%"+filters.Search+"%")
	}

	// Count
	countQuery := `SELECT COUNT(*) FROM taxes WHERE 1=1`
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

	query += " ORDER BY " + sortOrder(filters.SortBy, filters.SortDir)

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

	var taxes []Tax
	for rows.Next() {
		var t Tax
		err := rows.Scan(&t.ID, &t.Code, &t.Name, &t.Rate)
		if err != nil {
			return nil, 0, err
		}
		taxes = append(taxes, t)
	}
	return taxes, total, rows.Err()
}

// Get uses sqlc generated query
func (r *repository) Get(ctx context.Context, id int64) (Tax, error) {
	row, err := r.queries.GetTax(ctx, id)
	if err != nil {
		return Tax{}, err
	}
	var rate float64
	if row.Rate.Valid {
		f8, _ := row.Rate.Float64Value()
		rate = f8.Float64
	}
	return Tax{
		ID:   row.ID,
		Code: row.Code,
		Name: row.Name,
		Rate: rate,
	}, nil
}

// Create uses sqlc generated query
func (r *repository) Create(ctx context.Context, tax Tax) (Tax, error) {
	row, err := r.queries.CreateTax(ctx, masterdatadb.CreateTaxParams{
		Code: tax.Code,
		Name: tax.Name,
		Rate: pgtype.Numeric{Valid: true},
	})
	if err != nil {
		return Tax{}, err
	}
	var rate float64
	if row.Rate.Valid {
		f8, _ := row.Rate.Float64Value()
		rate = f8.Float64
	}
	return Tax{
		ID:   row.ID,
		Code: row.Code,
		Name: row.Name,
		Rate: rate,
	}, nil
}

// Update uses sqlc generated query
func (r *repository) Update(ctx context.Context, id int64, tax Tax) error {
	return r.queries.UpdateTax(ctx, masterdatadb.UpdateTaxParams{
		Code: tax.Code,
		Name: tax.Name,
		Rate: pgtype.Numeric{Valid: true},
		ID:   id,
	})
}

// Delete uses sqlc generated query
func (r *repository) Delete(ctx context.Context, id int64) error {
	return r.queries.DeleteTax(ctx, id)
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
	case "rate":
		return "rate " + dir
	default:
		return "name " + dir
	}
}
