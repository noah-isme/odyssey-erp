package suppliers

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

type Repository interface {
	List(ctx context.Context, filters shared.ListFilters) ([]Supplier, int, error)
	Get(ctx context.Context, id int64) (Supplier, error)
	Create(ctx context.Context, supplier Supplier) (Supplier, error)
	Update(ctx context.Context, id int64, supplier Supplier) error
	Delete(ctx context.Context, id int64) error
}

type repository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &repository{
		pool:    pool,
		queries: sqlc.New(pool),
	}
}

// List uses dynamic query (not sqlc) due to filter complexity
func (r *repository) List(ctx context.Context, filters shared.ListFilters) ([]Supplier, int, error) {
	query := `SELECT id, code, name, address, email, phone, is_active FROM suppliers WHERE 1=1`
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

	var suppliers []Supplier
	for rows.Next() {
		var s Supplier
		err := rows.Scan(&s.ID, &s.Code, &s.Name, &s.Address, &s.Email, &s.Phone, &s.IsActive)
		if err != nil {
			return nil, 0, err
		}
		suppliers = append(suppliers, s)
	}
	return suppliers, total, rows.Err()
}

// Get uses sqlc generated query
func (r *repository) Get(ctx context.Context, id int64) (Supplier, error) {
	row, err := r.queries.GetSupplier(ctx, id)
	if err != nil {
		return Supplier{}, err
	}
	return Supplier{
		ID:       row.ID,
		Code:     row.Code,
		Name:     row.Name,
		Phone:    row.Phone,
		Email:    row.Email,
		Address:  row.Address,
		IsActive: row.IsActive,
	}, nil
}

// Create uses sqlc generated query
func (r *repository) Create(ctx context.Context, supplier Supplier) (Supplier, error) {
	row, err := r.queries.CreateSupplier(ctx, sqlc.CreateSupplierParams{
		Code:     supplier.Code,
		Name:     supplier.Name,
		Phone:    supplier.Phone,
		Email:    supplier.Email,
		Address:  supplier.Address,
		IsActive: supplier.IsActive,
	})
	if err != nil {
		return Supplier{}, err
	}
	supplier.ID = row.ID
	// timestamps not available
	return supplier, nil
}

// Update uses sqlc generated query
func (r *repository) Update(ctx context.Context, id int64, supplier Supplier) error {
	return r.queries.UpdateSupplier(ctx, sqlc.UpdateSupplierParams{
		Code:     supplier.Code,
		Name:     supplier.Name,
		Phone:    supplier.Phone,
		Email:    supplier.Email,
		Address:  supplier.Address,
		IsActive: supplier.IsActive,
		ID:       id,
	})
}

// Delete uses sqlc generated query
func (r *repository) Delete(ctx context.Context, id int64) error {
	return r.queries.DeleteSupplier(ctx, id)
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
