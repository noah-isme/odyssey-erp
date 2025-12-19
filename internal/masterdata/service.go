package masterdata

import (
	"context"
	"errors"
	"strings"
)

// service implements Service interface
type service struct {
	repo Repository
}

// NewService creates a new master data service
func NewService(repo Repository) Service {
	return &service{repo: repo}
}

// Company operations
func (s *service) ListCompanies(ctx context.Context, filters ListFilters) ([]Company, int, error) {
	return s.repo.ListCompanies(ctx, filters)
}

func (s *service) GetCompany(ctx context.Context, id int64) (Company, error) {
	if id <= 0 {
		return Company{}, errors.New("invalid company ID")
	}
	return s.repo.GetCompany(ctx, id)
}

func (s *service) CreateCompany(ctx context.Context, company Company) (Company, error) {
	if err := s.validateCompany(company); err != nil {
		return Company{}, err
	}
	return s.repo.CreateCompany(ctx, company)
}

func (s *service) UpdateCompany(ctx context.Context, id int64, company Company) error {
	if id <= 0 {
		return errors.New("invalid company ID")
	}
	if err := s.validateCompany(company); err != nil {
		return err
	}
	return s.repo.UpdateCompany(ctx, id, company)
}

func (s *service) DeleteCompany(ctx context.Context, id int64) error {
	if id <= 0 {
		return errors.New("invalid company ID")
	}
	return s.repo.DeleteCompany(ctx, id)
}

// Branch operations
func (s *service) ListBranches(ctx context.Context, filters ListFilters) ([]Branch, int, error) {
	return s.repo.ListBranches(ctx, filters)
}

func (s *service) GetBranch(ctx context.Context, id int64) (Branch, error) {
	if id <= 0 {
		return Branch{}, errors.New("invalid branch ID")
	}
	return s.repo.GetBranch(ctx, id)
}

func (s *service) CreateBranch(ctx context.Context, branch Branch) (Branch, error) {
	if err := s.validateBranch(branch); err != nil {
		return Branch{}, err
	}
	return s.repo.CreateBranch(ctx, branch)
}

func (s *service) UpdateBranch(ctx context.Context, id int64, branch Branch) error {
	if id <= 0 {
		return errors.New("invalid branch ID")
	}
	if err := s.validateBranch(branch); err != nil {
		return err
	}
	return s.repo.UpdateBranch(ctx, id, branch)
}

func (s *service) DeleteBranch(ctx context.Context, id int64) error {
	if id <= 0 {
		return errors.New("invalid branch ID")
	}
	return s.repo.DeleteBranch(ctx, id)
}

// Warehouse operations
func (s *service) ListWarehouses(ctx context.Context, filters ListFilters) ([]Warehouse, int, error) {
	return s.repo.ListWarehouses(ctx, filters)
}

func (s *service) GetWarehouse(ctx context.Context, id int64) (Warehouse, error) {
	if id <= 0 {
		return Warehouse{}, errors.New("invalid warehouse ID")
	}
	return s.repo.GetWarehouse(ctx, id)
}

func (s *service) CreateWarehouse(ctx context.Context, warehouse Warehouse) (Warehouse, error) {
	if err := s.validateWarehouse(warehouse); err != nil {
		return Warehouse{}, err
	}
	return s.repo.CreateWarehouse(ctx, warehouse)
}

func (s *service) UpdateWarehouse(ctx context.Context, id int64, warehouse Warehouse) error {
	if id <= 0 {
		return errors.New("invalid warehouse ID")
	}
	if err := s.validateWarehouse(warehouse); err != nil {
		return err
	}
	return s.repo.UpdateWarehouse(ctx, id, warehouse)
}

func (s *service) DeleteWarehouse(ctx context.Context, id int64) error {
	if id <= 0 {
		return errors.New("invalid warehouse ID")
	}
	return s.repo.DeleteWarehouse(ctx, id)
}

// Unit operations
func (s *service) ListUnits(ctx context.Context, filters ListFilters) ([]Unit, int, error) {
	return s.repo.ListUnits(ctx, filters)
}

func (s *service) GetUnit(ctx context.Context, id int64) (Unit, error) {
	if id <= 0 {
		return Unit{}, errors.New("invalid unit ID")
	}
	return s.repo.GetUnit(ctx, id)
}

func (s *service) CreateUnit(ctx context.Context, unit Unit) (Unit, error) {
	if err := s.validateUnit(unit); err != nil {
		return Unit{}, err
	}
	return s.repo.CreateUnit(ctx, unit)
}

func (s *service) UpdateUnit(ctx context.Context, id int64, unit Unit) error {
	if id <= 0 {
		return errors.New("invalid unit ID")
	}
	if err := s.validateUnit(unit); err != nil {
		return err
	}
	return s.repo.UpdateUnit(ctx, id, unit)
}

func (s *service) DeleteUnit(ctx context.Context, id int64) error {
	if id <= 0 {
		return errors.New("invalid unit ID")
	}
	return s.repo.DeleteUnit(ctx, id)
}

// Tax operations
func (s *service) ListTaxes(ctx context.Context, filters ListFilters) ([]Tax, int, error) {
	return s.repo.ListTaxes(ctx, filters)
}

func (s *service) GetTax(ctx context.Context, id int64) (Tax, error) {
	if id <= 0 {
		return Tax{}, errors.New("invalid tax ID")
	}
	return s.repo.GetTax(ctx, id)
}

func (s *service) CreateTax(ctx context.Context, tax Tax) (Tax, error) {
	if err := s.validateTax(tax); err != nil {
		return Tax{}, err
	}
	return s.repo.CreateTax(ctx, tax)
}

func (s *service) UpdateTax(ctx context.Context, id int64, tax Tax) error {
	if id <= 0 {
		return errors.New("invalid tax ID")
	}
	if err := s.validateTax(tax); err != nil {
		return err
	}
	return s.repo.UpdateTax(ctx, id, tax)
}

func (s *service) DeleteTax(ctx context.Context, id int64) error {
	if id <= 0 {
		return errors.New("invalid tax ID")
	}
	return s.repo.DeleteTax(ctx, id)
}

// Category operations
func (s *service) ListCategories(ctx context.Context, filters ListFilters) ([]Category, int, error) {
	return s.repo.ListCategories(ctx, filters)
}

func (s *service) GetCategory(ctx context.Context, id int64) (Category, error) {
	if id <= 0 {
		return Category{}, errors.New("invalid category ID")
	}
	return s.repo.GetCategory(ctx, id)
}

func (s *service) CreateCategory(ctx context.Context, category Category) (Category, error) {
	if err := s.validateCategory(category); err != nil {
		return Category{}, err
	}
	return s.repo.CreateCategory(ctx, category)
}

func (s *service) UpdateCategory(ctx context.Context, id int64, category Category) error {
	if id <= 0 {
		return errors.New("invalid category ID")
	}
	if err := s.validateCategory(category); err != nil {
		return err
	}
	return s.repo.UpdateCategory(ctx, id, category)
}

func (s *service) DeleteCategory(ctx context.Context, id int64) error {
	if id <= 0 {
		return errors.New("invalid category ID")
	}
	return s.repo.DeleteCategory(ctx, id)
}

// Supplier operations
func (s *service) ListSuppliers(ctx context.Context, filters ListFilters) ([]Supplier, int, error) {
	return s.repo.ListSuppliers(ctx, filters)
}

func (s *service) GetSupplier(ctx context.Context, id int64) (Supplier, error) {
	if id <= 0 {
		return Supplier{}, errors.New("invalid supplier ID")
	}
	return s.repo.GetSupplier(ctx, id)
}

func (s *service) CreateSupplier(ctx context.Context, supplier Supplier) (Supplier, error) {
	if err := s.validateSupplier(supplier); err != nil {
		return Supplier{}, err
	}
	return s.repo.CreateSupplier(ctx, supplier)
}

func (s *service) UpdateSupplier(ctx context.Context, id int64, supplier Supplier) error {
	if id <= 0 {
		return errors.New("invalid supplier ID")
	}
	if err := s.validateSupplier(supplier); err != nil {
		return err
	}
	return s.repo.UpdateSupplier(ctx, id, supplier)
}

func (s *service) DeleteSupplier(ctx context.Context, id int64) error {
	if id <= 0 {
		return errors.New("invalid supplier ID")
	}
	return s.repo.DeleteSupplier(ctx, id)
}

// Product operations
func (s *service) ListProducts(ctx context.Context, filters ListFilters) ([]Product, int, error) {
	return s.repo.ListProducts(ctx, filters)
}

func (s *service) GetProduct(ctx context.Context, id int64) (Product, error) {
	if id <= 0 {
		return Product{}, errors.New("invalid product ID")
	}
	return s.repo.GetProduct(ctx, id)
}

func (s *service) CreateProduct(ctx context.Context, product Product) (Product, error) {
	if err := s.validateProduct(product); err != nil {
		return Product{}, err
	}
	return s.repo.CreateProduct(ctx, product)
}

func (s *service) UpdateProduct(ctx context.Context, id int64, product Product) error {
	if id <= 0 {
		return errors.New("invalid product ID")
	}
	if err := s.validateProduct(product); err != nil {
		return err
	}
	return s.repo.UpdateProduct(ctx, id, product)
}

func (s *service) DeleteProduct(ctx context.Context, id int64) error {
	if id <= 0 {
		return errors.New("invalid product ID")
	}
	return s.repo.DeleteProduct(ctx, id)
}

// Validation methods
func (s *service) validateCompany(company Company) error {
	if strings.TrimSpace(company.Code) == "" {
		return errors.New("company code is required")
	}
	if strings.TrimSpace(company.Name) == "" {
		return errors.New("company name is required")
	}
	return nil
}

func (s *service) validateBranch(branch Branch) error {
	if branch.CompanyID <= 0 {
		return errors.New("company ID is required")
	}
	if strings.TrimSpace(branch.Code) == "" {
		return errors.New("branch code is required")
	}
	if strings.TrimSpace(branch.Name) == "" {
		return errors.New("branch name is required")
	}
	return nil
}

func (s *service) validateWarehouse(warehouse Warehouse) error {
	if warehouse.BranchID <= 0 {
		return errors.New("branch ID is required")
	}
	if strings.TrimSpace(warehouse.Code) == "" {
		return errors.New("warehouse code is required")
	}
	if strings.TrimSpace(warehouse.Name) == "" {
		return errors.New("warehouse name is required")
	}
	return nil
}

func (s *service) validateUnit(unit Unit) error {
	if strings.TrimSpace(unit.Code) == "" {
		return errors.New("unit code is required")
	}
	if strings.TrimSpace(unit.Name) == "" {
		return errors.New("unit name is required")
	}
	return nil
}

func (s *service) validateTax(tax Tax) error {
	if strings.TrimSpace(tax.Code) == "" {
		return errors.New("tax code is required")
	}
	if strings.TrimSpace(tax.Name) == "" {
		return errors.New("tax name is required")
	}
	if tax.Rate < 0 || tax.Rate > 100 {
		return errors.New("tax rate must be between 0 and 100")
	}
	return nil
}

func (s *service) validateCategory(category Category) error {
	if strings.TrimSpace(category.Code) == "" {
		return errors.New("category code is required")
	}
	if strings.TrimSpace(category.Name) == "" {
		return errors.New("category name is required")
	}
	return nil
}

func (s *service) validateSupplier(supplier Supplier) error {
	if strings.TrimSpace(supplier.Code) == "" {
		return errors.New("supplier code is required")
	}
	if strings.TrimSpace(supplier.Name) == "" {
		return errors.New("supplier name is required")
	}
	return nil
}

func (s *service) validateProduct(product Product) error {
	if strings.TrimSpace(product.SKU) == "" {
		return errors.New("product SKU is required")
	}
	if strings.TrimSpace(product.Name) == "" {
		return errors.New("product name is required")
	}
	if product.CategoryID <= 0 {
		return errors.New("category ID is required")
	}
	if product.UnitID <= 0 {
		return errors.New("unit ID is required")
	}
	if product.Price < 0 {
		return errors.New("product price cannot be negative")
	}
	return nil
}
