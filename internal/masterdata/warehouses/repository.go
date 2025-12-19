package warehouses

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
	List(ctx context.Context, filters shared.ListFilters) ([]Warehouse, int, error)
	Get(ctx context.Context, id int64) (Warehouse, error)
	Create(ctx context.Context, warehouse Warehouse) (Warehouse, error)
	Update(ctx context.Context, id int64, warehouse Warehouse) error
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

// Get uses sqlc generated query
func (r *repository) Get(ctx context.Context, id int64) (Warehouse, error) {
	row, err := r.queries.GetWarehouse(ctx, id)
	if err != nil {
		return Warehouse{}, err
	}
	return Warehouse{
		ID:       row.ID,
		BranchID: row.BranchID,
		Code:     row.Code,
		Name:     row.Name,
		Address:  row.Address,
	}, nil
}

// Create uses sqlc generated query
func (r *repository) Create(ctx context.Context, warehouse Warehouse) (Warehouse, error) {
	now := time.Now()
	row, err := r.queries.CreateWarehouse(ctx, masterdatadb.CreateWarehouseParams{
		BranchID:  warehouse.BranchID,
		Code:      warehouse.Code,
		Name:      warehouse.Name,
		Address:   warehouse.Address,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		UpdatedAt: pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return Warehouse{}, err
	}
	return Warehouse{
		ID:       row.ID,
		BranchID: row.BranchID,
		Code:     row.Code,
		Name:     row.Name,
		Address:  row.Address,
	}, nil
}

// Update uses sqlc generated query
func (r *repository) Update(ctx context.Context, id int64, warehouse Warehouse) error {
	return r.queries.UpdateWarehouse(ctx, masterdatadb.UpdateWarehouseParams{
		BranchID:  warehouse.BranchID,
		Code:      warehouse.Code,
		Name:      warehouse.Name,
		Address:   warehouse.Address,
		UpdatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		ID:        id,
	})
}

// Delete uses sqlc generated query
func (r *repository) Delete(ctx context.Context, id int64) error {
	return r.queries.DeleteWarehouse(ctx, id)
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
