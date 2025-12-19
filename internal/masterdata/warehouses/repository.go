package warehouses

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/shared"
)

type Repository interface {
	List(ctx context.Context, filters shared.ListFilters) ([]Warehouse, int, error)
	Get(ctx context.Context, id int64) (Warehouse, error)
	Create(ctx context.Context, warehouse Warehouse) (Warehouse, error)
	Update(ctx context.Context, id int64, warehouse Warehouse) error
	Delete(ctx context.Context, id int64) error
}

type repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &repository{db: db}
}

func (r *repository) List(ctx context.Context, filters shared.ListFilters) ([]Warehouse, int, error) {
	query := `SELECT id, branch_id, code, name, address FROM warehouses WHERE 1=1`
	args := []interface{}{}
	argCount := 0

	if filters.BranchID != nil {
		argCount++
		query += ` AND branch_id = $` + strconv.Itoa(argCount)
		args = append(args, *filters.BranchID)
	}

	if filters.Search != "" {
		argCount++
		query += ` AND (name ILIKE $` + strconv.Itoa(argCount) + ` OR code ILIKE $` + strconv.Itoa(argCount) + `)`
		args = append(args, "%"+filters.Search+"%")
	}

	// Count
	countQuery := `SELECT COUNT(*) FROM warehouses WHERE 1=1`
	countArgs := []interface{}{}
	countArgCount := 0
	
	if filters.BranchID != nil {
		countArgCount++
		countQuery += ` AND branch_id = $` + strconv.Itoa(countArgCount)
		countArgs = append(countArgs, *filters.BranchID)
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

	var warehouses []Warehouse
	for rows.Next() {
		var w Warehouse
		err := rows.Scan(&w.ID, &w.BranchID, &w.Code, &w.Name, &w.Address)
		if err != nil {
			return nil, 0, err
		}
		warehouses = append(warehouses, w)
	}
	return warehouses, total, rows.Err()
}

func (r *repository) Get(ctx context.Context, id int64) (Warehouse, error) {
	query := `SELECT id, branch_id, code, name, address FROM warehouses WHERE id = $1`
	var w Warehouse
	err := r.db.QueryRow(ctx, query, id).Scan(&w.ID, &w.BranchID, &w.Code, &w.Name, &w.Address)
	return w, err
}

func (r *repository) Create(ctx context.Context, warehouse Warehouse) (Warehouse, error) {
	query := `INSERT INTO warehouses (branch_id, code, name, address) VALUES ($1, $2, $3, $4) RETURNING id`
	err := r.db.QueryRow(ctx, query, warehouse.BranchID, warehouse.Code, warehouse.Name, warehouse.Address).Scan(&warehouse.ID)
	return warehouse, err
}

func (r *repository) Update(ctx context.Context, id int64, warehouse Warehouse) error {
	query := `UPDATE warehouses SET branch_id = $1, code = $2, name = $3, address = $4 WHERE id = $5`
	_, err := r.db.Exec(ctx, query, warehouse.BranchID, warehouse.Code, warehouse.Name, warehouse.Address, id)
	return err
}

func (r *repository) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM warehouses WHERE id = $1`
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
