package products

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/shared"
)

type Repository interface {
	List(ctx context.Context, filters shared.ListFilters) ([]Product, int, error)
	Get(ctx context.Context, id int64) (Product, error)
	Create(ctx context.Context, product Product) (Product, error)
	Update(ctx context.Context, id int64, product Product) error
	Delete(ctx context.Context, id int64) error
}

type repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &repository{db: db}
}

func (r *repository) List(ctx context.Context, filters shared.ListFilters) ([]Product, int, error) {
	query := `SELECT id, code, name, category_id, unit_id, price, cost, tax_id, is_active, created_at, updated_at FROM products WHERE 1=1`
	args := []interface{}{}
	argCount := 0

	if filters.CategoryID != nil {
		argCount++
		query += ` AND category_id = $` + strconv.Itoa(argCount)
		args = append(args, *filters.CategoryID)
	}

	if filters.Search != "" {
		argCount++
		query += ` AND (name ILIKE $` + strconv.Itoa(argCount) + ` OR code ILIKE $` + strconv.Itoa(argCount) + `)`
		args = append(args, "%"+filters.Search+"%")
	}

	if filters.IsActive != nil {
		argCount++
		query += ` AND is_active = $` + strconv.Itoa(argCount)
		args = append(args, *filters.IsActive)
	}

	// Count
	countQuery := `SELECT COUNT(*) FROM products WHERE 1=1`
	countArgs := []interface{}{}
	countArgCount := 0

	if filters.CategoryID != nil {
		countArgCount++
		countQuery += ` AND category_id = $` + strconv.Itoa(countArgCount)
		countArgs = append(countArgs, *filters.CategoryID)
	}
	if filters.Search != "" {
		countArgCount++
		countQuery += ` AND (name ILIKE $` + strconv.Itoa(countArgCount) + ` OR code ILIKE $` + strconv.Itoa(countArgCount) + `)`
		countArgs = append(countArgs, "%"+filters.Search+"%")
	}
	if filters.IsActive != nil {
		countArgCount++
		countQuery += ` AND is_active = $` + strconv.Itoa(countArgCount)
		countArgs = append(countArgs, *filters.IsActive)
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

	var products []Product
	for rows.Next() {
		var p Product
		err := rows.Scan(&p.ID, &p.Code, &p.Name, &p.CategoryID, &p.UnitID, &p.Price, &p.Cost, &p.TaxID, &p.IsActive, &p.CreatedAt, &p.UpdatedAt)
		if err != nil {
			return nil, 0, err
		}
		products = append(products, p)
	}
	return products, total, rows.Err()
}

func (r *repository) Get(ctx context.Context, id int64) (Product, error) {
	query := `SELECT id, code, name, category_id, unit_id, price, cost, tax_id, is_active, created_at, updated_at FROM products WHERE id = $1`
	var p Product
	err := r.db.QueryRow(ctx, query, id).Scan(&p.ID, &p.Code, &p.Name, &p.CategoryID, &p.UnitID, &p.Price, &p.Cost, &p.TaxID, &p.IsActive, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func (r *repository) Create(ctx context.Context, product Product) (Product, error) {
	query := `INSERT INTO products (code, name, category_id, unit_id, price, cost, tax_id, is_active, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING id`
	now := time.Now()
	err := r.db.QueryRow(ctx, query, product.Code, product.Name, product.CategoryID, product.UnitID, product.Price, product.Cost, product.TaxID, product.IsActive, now, now).Scan(&product.ID)
	if err != nil {
		return Product{}, err
	}
	product.CreatedAt = now
	product.UpdatedAt = now
	return product, nil
}

func (r *repository) Update(ctx context.Context, id int64, product Product) error {
	query := `UPDATE products SET code = $1, name = $2, category_id = $3, unit_id = $4, price = $5, cost = $6, tax_id = $7, is_active = $8, updated_at = $9 WHERE id = $10`
	_, err := r.db.Exec(ctx, query, product.Code, product.Name, product.CategoryID, product.UnitID, product.Price, product.Cost, product.TaxID, product.IsActive, time.Now(), id)
	return err
}

func (r *repository) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM products WHERE id = $1`
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
	case "price":
		return "price " + dir
	case "cost":
		return "cost " + dir
	case "created_at":
		return "created_at " + dir
	default:
		return "name " + dir
	}
}
