package masterdata

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// repo implements Repository interface
type repo struct {
	db *pgxpool.Pool
}

// NewRepository creates a new master data repository
func NewRepository(db *pgxpool.Pool) Repository {
	return &repo{db: db}
}

// Company operations
func (r *repo) ListCompanies(ctx context.Context) ([]Company, error) {
	query := `SELECT id, code, name, address, tax_id, created_at, updated_at FROM companies ORDER BY name`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var companies []Company
	for rows.Next() {
		var c Company
		err := rows.Scan(&c.ID, &c.Code, &c.Name, &c.Address, &c.TaxID, &c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			return nil, err
		}
		companies = append(companies, c)
	}
	return companies, rows.Err()
}

func (r *repo) GetCompany(ctx context.Context, id int64) (Company, error) {
	query := `SELECT id, code, name, address, tax_id, created_at, updated_at FROM companies WHERE id = $1`
	var c Company
	err := r.db.QueryRow(ctx, query, id).Scan(&c.ID, &c.Code, &c.Name, &c.Address, &c.TaxID, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func (r *repo) CreateCompany(ctx context.Context, company Company) (Company, error) {
	query := `INSERT INTO companies (code, name, address, tax_id, created_at, updated_at)
	          VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`
	now := time.Now()
	err := r.db.QueryRow(ctx, query, company.Code, company.Name, company.Address, company.TaxID, now, now).Scan(&company.ID)
	if err != nil {
		return Company{}, err
	}
	company.CreatedAt = now
	company.UpdatedAt = now
	return company, nil
}

func (r *repo) UpdateCompany(ctx context.Context, id int64, company Company) error {
	query := `UPDATE companies SET code = $1, name = $2, address = $3, tax_id = $4, updated_at = $5 WHERE id = $6`
	_, err := r.db.Exec(ctx, query, company.Code, company.Name, company.Address, company.TaxID, time.Now(), id)
	return err
}

func (r *repo) DeleteCompany(ctx context.Context, id int64) error {
	query := `DELETE FROM companies WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id)
	return err
}

// Branch operations
func (r *repo) ListBranches(ctx context.Context, companyID *int64) ([]Branch, error) {
	query := `SELECT id, company_id, code, name, address FROM branches`
	args := []interface{}{}
	if companyID != nil {
		query += ` WHERE company_id = $1`
		args = append(args, *companyID)
	}
	query += ` ORDER BY name`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var branches []Branch
	for rows.Next() {
		var b Branch
		err := rows.Scan(&b.ID, &b.CompanyID, &b.Code, &b.Name, &b.Address)
		if err != nil {
			return nil, err
		}
		branches = append(branches, b)
	}
	return branches, rows.Err()
}

func (r *repo) GetBranch(ctx context.Context, id int64) (Branch, error) {
	query := `SELECT id, company_id, code, name, address FROM branches WHERE id = $1`
	var b Branch
	err := r.db.QueryRow(ctx, query, id).Scan(&b.ID, &b.CompanyID, &b.Code, &b.Name, &b.Address)
	return b, err
}

func (r *repo) CreateBranch(ctx context.Context, branch Branch) (Branch, error) {
	query := `INSERT INTO branches (company_id, code, name, address) VALUES ($1, $2, $3, $4) RETURNING id`
	err := r.db.QueryRow(ctx, query, branch.CompanyID, branch.Code, branch.Name, branch.Address).Scan(&branch.ID)
	return branch, err
}

func (r *repo) UpdateBranch(ctx context.Context, id int64, branch Branch) error {
	query := `UPDATE branches SET company_id = $1, code = $2, name = $3, address = $4 WHERE id = $5`
	_, err := r.db.Exec(ctx, query, branch.CompanyID, branch.Code, branch.Name, branch.Address, id)
	return err
}

func (r *repo) DeleteBranch(ctx context.Context, id int64) error {
	query := `DELETE FROM branches WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id)
	return err
}

// Warehouse operations
func (r *repo) ListWarehouses(ctx context.Context, branchID *int64) ([]Warehouse, error) {
	query := `SELECT id, branch_id, code, name, address FROM warehouses`
	args := []interface{}{}
	if branchID != nil {
		query += ` WHERE branch_id = $1`
		args = append(args, *branchID)
	}
	query += ` ORDER BY name`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var warehouses []Warehouse
	for rows.Next() {
		var w Warehouse
		err := rows.Scan(&w.ID, &w.BranchID, &w.Code, &w.Name, &w.Address)
		if err != nil {
			return nil, err
		}
		warehouses = append(warehouses, w)
	}
	return warehouses, rows.Err()
}

func (r *repo) GetWarehouse(ctx context.Context, id int64) (Warehouse, error) {
	query := `SELECT id, branch_id, code, name, address FROM warehouses WHERE id = $1`
	var w Warehouse
	err := r.db.QueryRow(ctx, query, id).Scan(&w.ID, &w.BranchID, &w.Code, &w.Name, &w.Address)
	return w, err
}

func (r *repo) CreateWarehouse(ctx context.Context, warehouse Warehouse) (Warehouse, error) {
	query := `INSERT INTO warehouses (branch_id, code, name, address) VALUES ($1, $2, $3, $4) RETURNING id`
	err := r.db.QueryRow(ctx, query, warehouse.BranchID, warehouse.Code, warehouse.Name, warehouse.Address).Scan(&warehouse.ID)
	return warehouse, err
}

func (r *repo) UpdateWarehouse(ctx context.Context, id int64, warehouse Warehouse) error {
	query := `UPDATE warehouses SET branch_id = $1, code = $2, name = $3, address = $4 WHERE id = $5`
	_, err := r.db.Exec(ctx, query, warehouse.BranchID, warehouse.Code, warehouse.Name, warehouse.Address, id)
	return err
}

func (r *repo) DeleteWarehouse(ctx context.Context, id int64) error {
	query := `DELETE FROM warehouses WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id)
	return err
}

// Unit operations
func (r *repo) ListUnits(ctx context.Context) ([]Unit, error) {
	query := `SELECT id, code, name FROM units ORDER BY name`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var units []Unit
	for rows.Next() {
		var u Unit
		err := rows.Scan(&u.ID, &u.Code, &u.Name)
		if err != nil {
			return nil, err
		}
		units = append(units, u)
	}
	return units, rows.Err()
}

func (r *repo) GetUnit(ctx context.Context, id int64) (Unit, error) {
	query := `SELECT id, code, name FROM units WHERE id = $1`
	var u Unit
	err := r.db.QueryRow(ctx, query, id).Scan(&u.ID, &u.Code, &u.Name)
	return u, err
}

func (r *repo) CreateUnit(ctx context.Context, unit Unit) (Unit, error) {
	query := `INSERT INTO units (code, name) VALUES ($1, $2) RETURNING id`
	err := r.db.QueryRow(ctx, query, unit.Code, unit.Name).Scan(&unit.ID)
	return unit, err
}

func (r *repo) UpdateUnit(ctx context.Context, id int64, unit Unit) error {
	query := `UPDATE units SET code = $1, name = $2 WHERE id = $3`
	_, err := r.db.Exec(ctx, query, unit.Code, unit.Name, id)
	return err
}

func (r *repo) DeleteUnit(ctx context.Context, id int64) error {
	query := `DELETE FROM units WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id)
	return err
}

// Tax operations
func (r *repo) ListTaxes(ctx context.Context) ([]Tax, error) {
	query := `SELECT id, code, name, rate FROM taxes ORDER BY name`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var taxes []Tax
	for rows.Next() {
		var t Tax
		err := rows.Scan(&t.ID, &t.Code, &t.Name, &t.Rate)
		if err != nil {
			return nil, err
		}
		taxes = append(taxes, t)
	}
	return taxes, rows.Err()
}

func (r *repo) GetTax(ctx context.Context, id int64) (Tax, error) {
	query := `SELECT id, code, name, rate FROM taxes WHERE id = $1`
	var t Tax
	err := r.db.QueryRow(ctx, query, id).Scan(&t.ID, &t.Code, &t.Name, &t.Rate)
	return t, err
}

func (r *repo) CreateTax(ctx context.Context, tax Tax) (Tax, error) {
	query := `INSERT INTO taxes (code, name, rate) VALUES ($1, $2, $3) RETURNING id`
	err := r.db.QueryRow(ctx, query, tax.Code, tax.Name, tax.Rate).Scan(&tax.ID)
	return tax, err
}

func (r *repo) UpdateTax(ctx context.Context, id int64, tax Tax) error {
	query := `UPDATE taxes SET code = $1, name = $2, rate = $3 WHERE id = $4`
	_, err := r.db.Exec(ctx, query, tax.Code, tax.Name, tax.Rate, id)
	return err
}

func (r *repo) DeleteTax(ctx context.Context, id int64) error {
	query := `DELETE FROM taxes WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id)
	return err
}

// Category operations
func (r *repo) ListCategories(ctx context.Context) ([]Category, error) {
	query := `SELECT id, code, name, parent_id FROM categories ORDER BY name`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categories []Category
	for rows.Next() {
		var c Category
		err := rows.Scan(&c.ID, &c.Code, &c.Name, &c.ParentID)
		if err != nil {
			return nil, err
		}
		categories = append(categories, c)
	}
	return categories, rows.Err()
}

func (r *repo) GetCategory(ctx context.Context, id int64) (Category, error) {
	query := `SELECT id, code, name, parent_id FROM categories WHERE id = $1`
	var c Category
	err := r.db.QueryRow(ctx, query, id).Scan(&c.ID, &c.Code, &c.Name, &c.ParentID)
	return c, err
}

func (r *repo) CreateCategory(ctx context.Context, category Category) (Category, error) {
	query := `INSERT INTO categories (code, name, parent_id) VALUES ($1, $2, $3) RETURNING id`
	err := r.db.QueryRow(ctx, query, category.Code, category.Name, category.ParentID).Scan(&category.ID)
	return category, err
}

func (r *repo) UpdateCategory(ctx context.Context, id int64, category Category) error {
	query := `UPDATE categories SET code = $1, name = $2, parent_id = $3 WHERE id = $4`
	_, err := r.db.Exec(ctx, query, category.Code, category.Name, category.ParentID, id)
	return err
}

func (r *repo) DeleteCategory(ctx context.Context, id int64) error {
	query := `DELETE FROM categories WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id)
	return err
}

// Supplier operations
func (r *repo) ListSuppliers(ctx context.Context, isActive *bool) ([]Supplier, error) {
	query := `SELECT id, code, name, phone, email, address, is_active FROM suppliers`
	args := []interface{}{}
	if isActive != nil {
		query += ` WHERE is_active = $1`
		args = append(args, *isActive)
	}
	query += ` ORDER BY name`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var suppliers []Supplier
	for rows.Next() {
		var s Supplier
		err := rows.Scan(&s.ID, &s.Code, &s.Name, &s.Phone, &s.Email, &s.Address, &s.IsActive)
		if err != nil {
			return nil, err
		}
		suppliers = append(suppliers, s)
	}
	return suppliers, rows.Err()
}

func (r *repo) GetSupplier(ctx context.Context, id int64) (Supplier, error) {
	query := `SELECT id, code, name, phone, email, address, is_active FROM suppliers WHERE id = $1`
	var s Supplier
	err := r.db.QueryRow(ctx, query, id).Scan(&s.ID, &s.Code, &s.Name, &s.Phone, &s.Email, &s.Address, &s.IsActive)
	return s, err
}

func (r *repo) CreateSupplier(ctx context.Context, supplier Supplier) (Supplier, error) {
	query := `INSERT INTO suppliers (code, name, phone, email, address, is_active) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`
	err := r.db.QueryRow(ctx, query, supplier.Code, supplier.Name, supplier.Phone, supplier.Email, supplier.Address, supplier.IsActive).Scan(&supplier.ID)
	return supplier, err
}

func (r *repo) UpdateSupplier(ctx context.Context, id int64, supplier Supplier) error {
	query := `UPDATE suppliers SET code = $1, name = $2, phone = $3, email = $4, address = $5, is_active = $6 WHERE id = $7`
	_, err := r.db.Exec(ctx, query, supplier.Code, supplier.Name, supplier.Phone, supplier.Email, supplier.Address, supplier.IsActive, id)
	return err
}

func (r *repo) DeleteSupplier(ctx context.Context, id int64) error {
	query := `DELETE FROM suppliers WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id)
	return err
}

// Product operations
func (r *repo) ListProducts(ctx context.Context, categoryID *int64, isActive *bool, sortBy, sortDir string) ([]Product, error) {
	query := `SELECT id, sku, name, category_id, unit_id, price, tax_id, is_active, deleted_at FROM products WHERE deleted_at IS NULL`
	args := []interface{}{}
	argCount := 0

	if categoryID != nil {
		argCount++
		query += ` AND category_id = $` + strconv.Itoa(argCount)
		args = append(args, *categoryID)
	}

	if isActive != nil {
		argCount++
		query += ` AND is_active = $` + strconv.Itoa(argCount)
		args = append(args, *isActive)
	}

	query += " ORDER BY " + sortOrderProduct(sortBy, sortDir)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []Product
	for rows.Next() {
		var p Product
		err := rows.Scan(&p.ID, &p.SKU, &p.Name, &p.CategoryID, &p.UnitID, &p.Price, &p.TaxID, &p.IsActive, &p.DeletedAt)
		if err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	return products, rows.Err()
}

func (r *repo) GetProduct(ctx context.Context, id int64) (Product, error) {
	query := `SELECT id, sku, name, category_id, unit_id, price, tax_id, is_active, deleted_at FROM products WHERE id = $1 AND deleted_at IS NULL`
	var p Product
	err := r.db.QueryRow(ctx, query, id).Scan(&p.ID, &p.SKU, &p.Name, &p.CategoryID, &p.UnitID, &p.Price, &p.TaxID, &p.IsActive, &p.DeletedAt)
	return p, err
}

func (r *repo) CreateProduct(ctx context.Context, product Product) (Product, error) {
	query := `INSERT INTO products (sku, name, category_id, unit_id, price, tax_id, is_active) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`
	err := r.db.QueryRow(ctx, query, product.SKU, product.Name, product.CategoryID, product.UnitID, product.Price, product.TaxID, product.IsActive).Scan(&product.ID)
	return product, err
}

func (r *repo) UpdateProduct(ctx context.Context, id int64, product Product) error {
	query := `UPDATE products SET sku = $1, name = $2, category_id = $3, unit_id = $4, price = $5, tax_id = $6, is_active = $7 WHERE id = $8 AND deleted_at IS NULL`
	_, err := r.db.Exec(ctx, query, product.SKU, product.Name, product.CategoryID, product.UnitID, product.Price, product.TaxID, product.IsActive, id)
	return err
}

func (r *repo) DeleteProduct(ctx context.Context, id int64) error {
	query := `UPDATE products SET deleted_at = $1 WHERE id = $2 AND deleted_at IS NULL`
	_, err := r.db.Exec(ctx, query, time.Now(), id)
	return err
}

func sortOrderProduct(sortBy, sortDir string) string {
dir := "ASC"
if sortDir == "desc" {
dir = "DESC"
}

switch sortBy {
case "sku":
return "sku " + dir
case "name":
return "name " + dir
case "price":
return "price " + dir
default:
return "name " + dir
}
}
