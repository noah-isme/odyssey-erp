package products

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
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
	queries *sqlc.Queries
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &repository{
		pool:    pool,
		queries: sqlc.New(pool),
	}
}

// List uses dynamic query (not sqlc) due to filter complexity
func (r *repository) List(ctx context.Context, filters shared.ListFilters) ([]Product, int, error) {
	// Note: DB uses 'sku' column, but we map to 'code' for backward compatibility
	query := `SELECT id, sku, name, category_id, unit_id, price, tax_id, is_active, deleted_at,
		cost_method, min_stock, reorder_target, preferred_supplier_id, track_batch, track_serial
		FROM products WHERE 1=1`
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
		var minStock, reorderTarget pgtype.Numeric
		var supplierID pgtype.Int8
		err := rows.Scan(&p.ID, &p.Code, &p.Name, &p.CategoryID, &p.UnitID, &price, &taxID, &p.IsActive, &deletedAt,
			&p.CostMethod, &minStock, &reorderTarget, &supplierID, &p.TrackBatch, &p.TrackSerial)
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
		if minStock.Valid {
			v, _ := minStock.Float64Value()
			p.MinStock = v.Float64
		}
		if reorderTarget.Valid {
			v, _ := reorderTarget.Float64Value()
			p.ReorderTarget = v.Float64
		}
		if supplierID.Valid {
			p.PreferredSupplierID = supplierID.Int64
		}
		if deletedAt.Valid {
			t := deletedAt.Time
			p.DeletedAt = &t
		}
		products = append(products, p)
	}
	return products, total, rows.Err()
}

func (r *repository) Get(ctx context.Context, id int64) (Product, error) {
	items, _, err := r.listByQuery(ctx, `WHERE id = $1`, []interface{}{id})
	if err != nil {
		return Product{}, err
	}
	if len(items) == 0 {
		return Product{}, shared.ErrNotFound
	}
	return items[0], nil
}

func (r *repository) Create(ctx context.Context, product Product) (Product, error) {
	var supplierID *int64
	if product.PreferredSupplierID > 0 {
		supplierID = &product.PreferredSupplierID
	}
	err := r.pool.QueryRow(ctx, `INSERT INTO products
		(sku, name, category_id, unit_id, price, tax_id, is_active, cost_method, min_stock, reorder_target, preferred_supplier_id, track_batch, track_serial)
		VALUES ($1,$2,$3,$4,$5,NULLIF($6,0),$7,$8,$9,$10,$11,$12,$13) RETURNING id`,
		product.Code, product.Name, product.CategoryID, product.UnitID, product.Price, product.TaxID, product.IsActive,
		product.CostMethod, product.MinStock, product.ReorderTarget, supplierID, product.TrackBatch, product.TrackSerial).Scan(&product.ID)
	if err != nil {
		return Product{}, err
	}
	return product, nil
}

func (r *repository) Update(ctx context.Context, id int64, product Product) error {
	var supplierID *int64
	if product.PreferredSupplierID > 0 {
		supplierID = &product.PreferredSupplierID
	}
	_, err := r.pool.Exec(ctx, `UPDATE products SET sku=$1, name=$2, category_id=$3, unit_id=$4, price=$5,
		tax_id=NULLIF($6,0), is_active=$7, cost_method=$8, min_stock=$9, reorder_target=$10,
		preferred_supplier_id=$11, track_batch=$12, track_serial=$13 WHERE id=$14`,
		product.Code, product.Name, product.CategoryID, product.UnitID, product.Price, product.TaxID, product.IsActive,
		product.CostMethod, product.MinStock, product.ReorderTarget, supplierID, product.TrackBatch, product.TrackSerial, id)
	return err
}

func (r *repository) listByQuery(ctx context.Context, suffix string, args []interface{}) ([]Product, int, error) {
	query := `SELECT id, sku, name, category_id, unit_id, price, tax_id, is_active, deleted_at,
		cost_method, min_stock, reorder_target, preferred_supplier_id, track_batch, track_serial FROM products ` + suffix
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var result []Product
	for rows.Next() {
		var p Product
		var price, minStock, reorderTarget pgtype.Numeric
		var taxID, supplierID pgtype.Int8
		var deletedAt pgtype.Timestamptz
		if err := rows.Scan(&p.ID, &p.Code, &p.Name, &p.CategoryID, &p.UnitID, &price, &taxID, &p.IsActive, &deletedAt, &p.CostMethod, &minStock, &reorderTarget, &supplierID, &p.TrackBatch, &p.TrackSerial); err != nil {
			return nil, 0, err
		}
		if price.Valid {
			v, _ := price.Float64Value()
			p.Price = v.Float64
		}
		if minStock.Valid {
			v, _ := minStock.Float64Value()
			p.MinStock = v.Float64
		}
		if reorderTarget.Valid {
			v, _ := reorderTarget.Float64Value()
			p.ReorderTarget = v.Float64
		}
		if taxID.Valid {
			p.TaxID = taxID.Int64
		}
		if supplierID.Valid {
			p.PreferredSupplierID = supplierID.Int64
		}
		if deletedAt.Valid {
			t := deletedAt.Time
			p.DeletedAt = &t
		}
		result = append(result, p)
	}
	return result, len(result), rows.Err()
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
