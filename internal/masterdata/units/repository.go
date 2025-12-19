package units

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
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
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &repository{db: db}
}

func (r *repository) List(ctx context.Context, filters shared.ListFilters) ([]Unit, int, error) {
	query := `SELECT id, code, name FROM units WHERE 1=1`
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
	err := r.db.QueryRow(ctx, countQuery, countArgs...).Scan(&total)
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
		if offset < 0 { offset = 0 }
		args = append(args, offset)
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var units []Unit
	for rows.Next() {
		var u Unit
		err := rows.Scan(&u.ID, &u.Code, &u.Name)
		if err != nil {
			return nil, 0, err
		}
		units = append(units, u)
	}
	return units, total, rows.Err()
}

func (r *repository) Get(ctx context.Context, id int64) (Unit, error) {
	query := `SELECT id, code, name FROM units WHERE id = $1`
	var u Unit
	err := r.db.QueryRow(ctx, query, id).Scan(&u.ID, &u.Code, &u.Name)
	return u, err
}

func (r *repository) Create(ctx context.Context, unit Unit) (Unit, error) {
	query := `INSERT INTO units (code, name) VALUES ($1, $2) RETURNING id`
	err := r.db.QueryRow(ctx, query, unit.Code, unit.Name).Scan(&unit.ID)
	return unit, err
}

func (r *repository) Update(ctx context.Context, id int64, unit Unit) error {
	query := `UPDATE units SET code = $1, name = $2 WHERE id = $3`
	_, err := r.db.Exec(ctx, query, unit.Code, unit.Name, id)
	return err
}

func (r *repository) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM units WHERE id = $1`
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
