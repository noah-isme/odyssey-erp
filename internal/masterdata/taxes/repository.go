package taxes

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
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
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &repository{db: db}
}

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

func (r *repository) Get(ctx context.Context, id int64) (Tax, error) {
	query := `SELECT id, code, name, rate FROM taxes WHERE id = $1`
	var t Tax
	err := r.db.QueryRow(ctx, query, id).Scan(&t.ID, &t.Code, &t.Name, &t.Rate)
	return t, err
}

func (r *repository) Create(ctx context.Context, tax Tax) (Tax, error) {
	query := `INSERT INTO taxes (code, name, rate) VALUES ($1, $2, $3) RETURNING id`
	err := r.db.QueryRow(ctx, query, tax.Code, tax.Name, tax.Rate).Scan(&tax.ID)
	return tax, err
}

func (r *repository) Update(ctx context.Context, id int64, tax Tax) error {
	query := `UPDATE taxes SET code = $1, name = $2, rate = $3 WHERE id = $4`
	_, err := r.db.Exec(ctx, query, tax.Code, tax.Name, tax.Rate, id)
	return err
}

func (r *repository) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM taxes WHERE id = $1`
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
	case "rate":
		return "rate " + dir
	default:
		return "name " + dir
	}
}
