package masterdata

import (
	"context"
	"time"
)
 
// ListFilters represents standard list page filters
type ListFilters struct {
	Page       int
	Limit      int
	Search     string
	SortBy     string
	SortDir    string
	IsActive   *bool
	
	// Entity specific filters
	CompanyID  *int64
	BranchID   *int64
	CategoryID *int64
}
// Company represents a company entity
type Company struct {
	ID        int64     `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Address   string    `json:"address"`
	TaxID     string    `json:"tax_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Branch represents a branch entity
type Branch struct {
	ID        int64     `json:"id"`
	CompanyID int64     `json:"company_id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Address   string    `json:"address"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Warehouse represents a warehouse entity
type Warehouse struct {
	ID        int64     `json:"id"`
	BranchID  int64     `json:"branch_id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Address   string    `json:"address"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Unit represents a unit of measure
type Unit struct {
	ID   int64  `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// Tax represents a tax configuration
type Tax struct {
	ID   int64   `json:"id"`
	Code string  `json:"code"`
	Name string  `json:"name"`
	Rate float64 `json:"rate"`
}

// Category represents a product category
type Category struct {
	ID       int64  `json:"id"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	ParentID *int64 `json:"parent_id"`
}

// Supplier represents a supplier entity
type Supplier struct {
	ID        int64     `json:"id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Phone     string    `json:"phone"`
	Email     string    `json:"email"`
	Address   string    `json:"address"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Product represents a product entity
type Product struct {
	ID         int64      `json:"id"`
	SKU        string     `json:"sku"`
	Name       string     `json:"name"`
	CategoryID int64      `json:"category_id"`
	UnitID     int64      `json:"unit_id"`
	Price      float64    `json:"price"`
	TaxID      *int64     `json:"tax_id"`
	IsActive   bool       `json:"is_active"`
	DeletedAt  *time.Time `json:"deleted_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// Repository interface for master data operations
type Repository interface {
	// Company operations
	ListCompanies(ctx context.Context, filters ListFilters) ([]Company, int, error)
	GetCompany(ctx context.Context, id int64) (Company, error)
	CreateCompany(ctx context.Context, company Company) (Company, error)
	UpdateCompany(ctx context.Context, id int64, company Company) error
	DeleteCompany(ctx context.Context, id int64) error

	// Branch operations
	ListBranches(ctx context.Context, filters ListFilters) ([]Branch, int, error)
	GetBranch(ctx context.Context, id int64) (Branch, error)
	CreateBranch(ctx context.Context, branch Branch) (Branch, error)
	UpdateBranch(ctx context.Context, id int64, branch Branch) error
	DeleteBranch(ctx context.Context, id int64) error

	// Warehouse operations
	ListWarehouses(ctx context.Context, filters ListFilters) ([]Warehouse, int, error)
	GetWarehouse(ctx context.Context, id int64) (Warehouse, error)
	CreateWarehouse(ctx context.Context, warehouse Warehouse) (Warehouse, error)
	UpdateWarehouse(ctx context.Context, id int64, warehouse Warehouse) error
	DeleteWarehouse(ctx context.Context, id int64) error

	// Unit operations
	ListUnits(ctx context.Context, filters ListFilters) ([]Unit, int, error)
	GetUnit(ctx context.Context, id int64) (Unit, error)
	CreateUnit(ctx context.Context, unit Unit) (Unit, error)
	UpdateUnit(ctx context.Context, id int64, unit Unit) error
	DeleteUnit(ctx context.Context, id int64) error

	// Tax operations
	ListTaxes(ctx context.Context, filters ListFilters) ([]Tax, int, error)
	GetTax(ctx context.Context, id int64) (Tax, error)
	CreateTax(ctx context.Context, tax Tax) (Tax, error)
	UpdateTax(ctx context.Context, id int64, tax Tax) error
	DeleteTax(ctx context.Context, id int64) error

	// Category operations
	ListCategories(ctx context.Context, filters ListFilters) ([]Category, int, error)
	GetCategory(ctx context.Context, id int64) (Category, error)
	CreateCategory(ctx context.Context, category Category) (Category, error)
	UpdateCategory(ctx context.Context, id int64, category Category) error
	DeleteCategory(ctx context.Context, id int64) error

	// Supplier operations
	ListSuppliers(ctx context.Context, filters ListFilters) ([]Supplier, int, error)
	GetSupplier(ctx context.Context, id int64) (Supplier, error)
	CreateSupplier(ctx context.Context, supplier Supplier) (Supplier, error)
	UpdateSupplier(ctx context.Context, id int64, supplier Supplier) error
	DeleteSupplier(ctx context.Context, id int64) error

	// Product operations
	ListProducts(ctx context.Context, filters ListFilters) ([]Product, int, error)
	GetProduct(ctx context.Context, id int64) (Product, error)
	CreateProduct(ctx context.Context, product Product) (Product, error)
	UpdateProduct(ctx context.Context, id int64, product Product) error
	DeleteProduct(ctx context.Context, id int64) error
}

// Service interface for master data business logic
type Service interface {
	// Company operations
	ListCompanies(ctx context.Context, filters ListFilters) ([]Company, int, error)
	GetCompany(ctx context.Context, id int64) (Company, error)
	CreateCompany(ctx context.Context, company Company) (Company, error)
	UpdateCompany(ctx context.Context, id int64, company Company) error
	DeleteCompany(ctx context.Context, id int64) error

	// Branch operations
	ListBranches(ctx context.Context, filters ListFilters) ([]Branch, int, error)
	GetBranch(ctx context.Context, id int64) (Branch, error)
	CreateBranch(ctx context.Context, branch Branch) (Branch, error)
	UpdateBranch(ctx context.Context, id int64, branch Branch) error
	DeleteBranch(ctx context.Context, id int64) error

	// Warehouse operations
	ListWarehouses(ctx context.Context, filters ListFilters) ([]Warehouse, int, error)
	GetWarehouse(ctx context.Context, id int64) (Warehouse, error)
	CreateWarehouse(ctx context.Context, warehouse Warehouse) (Warehouse, error)
	UpdateWarehouse(ctx context.Context, id int64, warehouse Warehouse) error
	DeleteWarehouse(ctx context.Context, id int64) error

	// Unit operations
	ListUnits(ctx context.Context, filters ListFilters) ([]Unit, int, error)
	GetUnit(ctx context.Context, id int64) (Unit, error)
	CreateUnit(ctx context.Context, unit Unit) (Unit, error)
	UpdateUnit(ctx context.Context, id int64, unit Unit) error
	DeleteUnit(ctx context.Context, id int64) error

	// Tax operations
	ListTaxes(ctx context.Context, filters ListFilters) ([]Tax, int, error)
	GetTax(ctx context.Context, id int64) (Tax, error)
	CreateTax(ctx context.Context, tax Tax) (Tax, error)
	UpdateTax(ctx context.Context, id int64, tax Tax) error
	DeleteTax(ctx context.Context, id int64) error

	// Category operations
	ListCategories(ctx context.Context, filters ListFilters) ([]Category, int, error)
	GetCategory(ctx context.Context, id int64) (Category, error)
	CreateCategory(ctx context.Context, category Category) (Category, error)
	UpdateCategory(ctx context.Context, id int64, category Category) error
	DeleteCategory(ctx context.Context, id int64) error

	// Supplier operations
	ListSuppliers(ctx context.Context, filters ListFilters) ([]Supplier, int, error)
	GetSupplier(ctx context.Context, id int64) (Supplier, error)
	CreateSupplier(ctx context.Context, supplier Supplier) (Supplier, error)
	UpdateSupplier(ctx context.Context, id int64, supplier Supplier) error
	DeleteSupplier(ctx context.Context, id int64) error

	// Product operations
	ListProducts(ctx context.Context, filters ListFilters) ([]Product, int, error)
	GetProduct(ctx context.Context, id int64) (Product, error)
	CreateProduct(ctx context.Context, product Product) (Product, error)
	UpdateProduct(ctx context.Context, id int64, product Product) error
	DeleteProduct(ctx context.Context, id int64) error
}
