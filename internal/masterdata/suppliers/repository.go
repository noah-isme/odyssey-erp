package suppliers

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/shared"
)

type Repository interface {
	List(ctx context.Context, filters shared.ListFilters) ([]Supplier, int, error)
	Get(ctx context.Context, id int64) (Supplier, error)
	Create(ctx context.Context, supplier Supplier) (Supplier, error)
	Update(ctx context.Context, id int64, supplier Supplier) error
	Delete(ctx context.Context, id int64) error
}

type repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &repository{db: db}
}

func (r *repository) List(ctx context.Context, filters shared.ListFilters) ([]Supplier, int, error) {
	query := `SELECT id, code, name, address, email, phone, created_at, updated_at FROM suppliers WHERE 1=1`
	args := []interface{}{}
	argCount := 0

	if filters.Search != "" {
		argCount++
		query += ` AND (name ILIKE $` + strconv.Itoa(argCount) + ` OR code ILIKE $` + strconv.Itoa(argCount) + `)`
		args = append(args, "%"+filters.Search+"%")
	}

	// Count
	countQuery := `SELECT COUNT(*) FROM suppliers WHERE 1=1`
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

	var suppliers []Supplier
	for rows.Next() {
		var s Supplier
		err := rows.Scan(&s.ID, &s.Code, &s.Name, &s.Address, &s.Email, &s.Phone, &s.CreatedAt, &s.UpdatedAt)
		if err != nil {
			return nil, 0, err
		}
		suppliers = append(suppliers, s)
	}
	return suppliers, total, rows.Err()
}

func (r *repository) Get(ctx context.Context, id int64) (Supplier, error) {
	query := `SELECT id, code, name, address, email, phone, created_at, updated_at FROM suppliers WHERE id = $1`
	var s Supplier
	err := r.db.QueryRow(ctx, query, id).Scan(&s.ID, &s.Code, &s.Name, &s.Address, &s.Email, &s.Phone, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

func (r *repository) Create(ctx context.Context, supplier Supplier) (Supplier, error) {
	query := `INSERT INTO suppliers (code, name, address, email, phone, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`
	now := time.Now()
	err := r.db.QueryRow(ctx, query, supplier.Code, supplier.Name, supplier.Address, supplier.Email, supplier.Phone, now, now).Scan(&supplier.ID)
	if err != nil {
		return Supplier{}, err
	}
	supplier.CreatedAt = now
	supplier.UpdatedAt = now
	return supplier, nil
}

func (r *repository) Update(ctx context.Context, id int64, supplier Supplier) error {
	query := `UPDATE suppliers SET code = $1, name = $2, address = $3, email = $4, phone = $5, updated_at = $6 WHERE id = $7`
	_, err := r.db.Exec(ctx, query, supplier.Code, supplier.Name, supplier.Address, supplier.Email, supplier.Phone, time.Now(), id)
	return err
}

func (r *repository) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM suppliers WHERE id = $1`
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
