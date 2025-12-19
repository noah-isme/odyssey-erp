package products

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
	List(ctx context.Context, filters shared.ListFilters) ([]Product, int, error)
	Get(ctx context.Context, id int64) (Product, error)
	Create(ctx context.Context, product Product) (Product, error)
	Update(ctx context.Context, id int64, product Product) error
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
func (r *repository) List(ctx context.Context, filters shared.ListFilters) ([]Product, int, error) {
	// Note: DB uses 'sku' column, but we map to 'code' for backward compatibility
	query := `SELECT id, sku, name, category_id, unit_id, price, tax_id, is_active, deleted_at FROM products WHERE 1=1`
	args := []interface{}{}
	argCount := 0

	if filters.CategoryID != nil {
		argCount++
		query += ` AND category_id = $` + strconv.Itoa(argCount)
		args = append(args, *filters.CategoryID)
	}

	if filters.Search != "" {
		argCount++
		query += ` AND (name ILIKE $` + strconv.Itoa(argCount) + ` OR sku ILIKE $` + strconv.Itoa(argCount) + `)`
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
		countQuery += ` AND (name ILIKE $` + strconv.Itoa(countArgCount) + ` OR sku ILIKE $` + strconv.Itoa(countArgCount) + `)`
		countArgs = append(countArgs, "%"+filters.Search+"%")
	}
	if filters.IsActive != nil {
		countArgCount++
		countQuery += ` AND is_active = $` + strconv.Itoa(countArgCount)
		countArgs = append(countArgs, *filters.IsActive)
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

	var products []Product
	for rows.Next() {
		var p Product
		var price pgtype.Numeric
		var taxID pgtype.Int8
		var deletedAt pgtype.Timestamptz
		err := rows.Scan(&p.ID, &p.Code, &p.Name, &p.CategoryID, &p.UnitID, &price, &taxID, &p.IsActive, &deletedAt)
		if err != nil {
			return nil, 0, err
		}
		if price.Valid {
			f8, _ := price.Float64Value()
			p.Price = f8.Float64
		}
		if taxID.Valid {
			p.TaxID = taxID.Int64
		}
		if deletedAt.Valid {
			t := deletedAt.Time
			p.DeletedAt = &t
		}
		products = append(products, p)
	}
	return products, total, rows.Err()
}

// Get uses sqlc generated query
func (r *repository) Get(ctx context.Context, id int64) (Product, error) {
	row, err := r.queries.GetProduct(ctx, id)
	if err != nil {
		return Product{}, err
	}
	p := Product{
		ID:         row.ID,
		Code:       row.Sku, // map sku -> code
		Name:       row.Name,
		CategoryID: row.CategoryID,
		UnitID:     row.UnitID,
		IsActive:   row.IsActive,
	}
	if row.Price.Valid {
		f8, _ := row.Price.Float64Value()
		p.Price = f8.Float64
	}
	if row.TaxID.Valid {
		p.TaxID = row.TaxID.Int64
	}
	if row.DeletedAt.Valid {
		t := row.DeletedAt.Time
		p.DeletedAt = &t
	}
	return p, nil
}

// Create uses sqlc generated query
func (r *repository) Create(ctx context.Context, product Product) (Product, error) {
	// Convert price to pgtype.Numeric
	priceStr := strconv.FormatFloat(product.Price, 'f', 2, 64)
	var price pgtype.Numeric
	_ = price.Scan(priceStr)

	var taxID pgtype.Int8
	if product.TaxID > 0 {
		taxID = pgtype.Int8{Int64: product.TaxID, Valid: true}
	}

	row, err := r.queries.CreateProduct(ctx, masterdatadb.CreateProductParams{
		Sku:        product.Code, // map code -> sku
		Name:       product.Name,
		CategoryID: product.CategoryID,
		UnitID:     product.UnitID,
		Price:      price,
		TaxID:      taxID,
		IsActive:   product.IsActive,
	})
	if err != nil {
		return Product{}, err
	}

	p := Product{
		ID:         row.ID,
		Code:       row.Sku,
		Name:       row.Name,
		CategoryID: row.CategoryID,
		UnitID:     row.UnitID,
		IsActive:   row.IsActive,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if row.Price.Valid {
		f8, _ := row.Price.Float64Value()
		p.Price = f8.Float64
	}
	if row.TaxID.Valid {
		p.TaxID = row.TaxID.Int64
	}
	return p, nil
}

// Update uses sqlc generated query
func (r *repository) Update(ctx context.Context, id int64, product Product) error {
	priceStr := strconv.FormatFloat(product.Price, 'f', 2, 64)
	var price pgtype.Numeric
	_ = price.Scan(priceStr)

	var taxID pgtype.Int8
	if product.TaxID > 0 {
		taxID = pgtype.Int8{Int64: product.TaxID, Valid: true}
	}

	return r.queries.UpdateProduct(ctx, masterdatadb.UpdateProductParams{
		Sku:        product.Code, // map code -> sku
		Name:       product.Name,
		CategoryID: product.CategoryID,
		UnitID:     product.UnitID,
		Price:      price,
		TaxID:      taxID,
		IsActive:   product.IsActive,
		ID:         id,
	})
}

// Delete uses sqlc generated query
func (r *repository) Delete(ctx context.Context, id int64) error {
	return r.queries.DeleteProduct(ctx, id)
}

func sortOrder(sortBy, sortDir string) string {
	dir := "ASC"
	if sortDir == "desc" {
		dir = "DESC"
	}
	switch sortBy {
	case "code":
		return "sku " + dir // map code -> sku for sorting
	case "name":
		return "name " + dir
	case "price":
		return "price " + dir
	default:
		return "name " + dir
	}
}
