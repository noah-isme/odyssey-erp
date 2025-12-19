package units

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/db"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/shared"
)

type Repository interface {
	List(ctx context.Context, filters shared.ListFilters) ([]Unit, int, error)
	Get(ctx context.Context, id int64) (Unit, error)
	Create(ctx context.Context, unit Unit) (Unit, error)
	Update(ctx context.Context, id int64, unit Unit) error
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
func (r *repository) List(ctx context.Context, filters shared.ListFilters) ([]Unit, int, error) {
	query := `SELECT id, code, name, created_at, updated_at FROM units WHERE 1=1`
	args := []interface{}{}
	argCount := 0

	if filters.Search != "" {
		argCount++
		query += ` AND (name ILIKE $` + strconv.Itoa(argCount) + ` OR code ILIKE $` + strconv.Itoa(argCount) + `)`
		args = append(args, "%"+filters.Search+"%")
	}

	// Count
	countQuery := `SELECT COUNT(*) FROM units WHERE 1=1`
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

	var units []Unit
	for rows.Next() {
		var u Unit
		var createdAt, updatedAt pgtype.Timestamptz
		err := rows.Scan(&u.ID, &u.Code, &u.Name, &createdAt, &updatedAt)
		if err != nil {
			return nil, 0, err
		}
		units = append(units, u)
	}
	return units, total, rows.Err()
}

// Get uses sqlc generated query
func (r *repository) Get(ctx context.Context, id int64) (Unit, error) {
	row, err := r.queries.GetUnit(ctx, id)
	if err != nil {
		return Unit{}, err
	}
	return Unit{
		ID:   row.ID,
		Code: row.Code,
		Name: row.Name,
	}, nil
}

// Create uses sqlc generated query
func (r *repository) Create(ctx context.Context, unit Unit) (Unit, error) {
	now := time.Now()
	row, err := r.queries.CreateUnit(ctx, masterdatadb.CreateUnitParams{
		Code:      unit.Code,
		Name:      unit.Name,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		UpdatedAt: pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return Unit{}, err
	}
	return Unit{
		ID:   row.ID,
		Code: row.Code,
		Name: row.Name,
	}, nil
}

// Update uses sqlc generated query
func (r *repository) Update(ctx context.Context, id int64, unit Unit) error {
	return r.queries.UpdateUnit(ctx, masterdatadb.UpdateUnitParams{
		Code:      unit.Code,
		Name:      unit.Name,
		UpdatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		ID:        id,
	})
}

// Delete uses sqlc generated query
func (r *repository) Delete(ctx context.Context, id int64) error {
	return r.queries.DeleteUnit(ctx, id)
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
