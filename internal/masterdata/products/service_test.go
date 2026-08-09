package products

import (
	"context"
	"testing"

	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/shared"
	"github.com/stretchr/testify/require"
)

type memoryRepo struct {
	products map[int64]*Product
	nextID   int64
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{
		products: make(map[int64]*Product),
		nextID:   1,
	}
}

func (m *memoryRepo) List(ctx context.Context, filters shared.ListFilters) ([]Product, int, error) {
	items := make([]Product, 0, len(m.products))
	for _, product := range m.products {
		items = append(items, *product)
	}
	return items, len(items), nil
}

func (m *memoryRepo) Get(ctx context.Context, id int64) (Product, error) {
	if p, ok := m.products[id]; ok {
		return *p, nil
	}
	return Product{}, shared.ErrNotFound
}

func (m *memoryRepo) Create(ctx context.Context, p Product) (Product, error) {
	id := m.nextID
	m.nextID++
	p.ID = id
	m.products[id] = &p
	return p, nil
}

func (m *memoryRepo) Update(ctx context.Context, id int64, p Product) error {
	if _, ok := m.products[id]; !ok {
		return shared.ErrNotFound
	}
	p.ID = id
	m.products[id] = &p
	return nil
}

func (m *memoryRepo) Delete(ctx context.Context, id int64) error {
	delete(m.products, id)
	return nil
}

func TestCreateProduct(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(repo)

	p := Product{
		Code:       "PRD-001",
		Name:       "Test Product",
		UnitID:     1,
		CategoryID: 1,
	}

	created, err := svc.Create(context.Background(), p)
	require.NoError(t, err)
	require.Equal(t, "PRD-001", created.Code)
	require.Equal(t, int64(1), created.ID)
}

func TestUpdateProduct(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(repo)

	p := Product{
		Code:       "PRD-001",
		Name:       "Test Product",
		UnitID:     1,
		CategoryID: 1,
	}

	created, _ := svc.Create(context.Background(), p)
	created.Name = "Updated Product"

	err := svc.Update(context.Background(), created.ID, created)
	require.NoError(t, err)

	updated, _ := svc.Get(context.Background(), created.ID)
	require.Equal(t, "Updated Product", updated.Name)
}

func TestProductValidationRejectsUnsafeInventorySettings(t *testing.T) {
	svc := NewService(nil)
	tests := []struct {
		name string
		in   Product
		want string
	}{
		{name: "missing code", in: Product{Name: "Product"}, want: "product code is required"},
		{name: "missing name", in: Product{Code: "P-1"}, want: "product name is required"},
		{name: "unsupported cost method", in: Product{Code: "P-1", Name: "Product", CostMethod: "LIFO"}, want: "cost method must be AVG or FIFO"},
		{name: "negative threshold", in: Product{Code: "P-1", Name: "Product", MinStock: -1}, want: "stock thresholds cannot be negative"},
		{name: "batch and serial together", in: Product{Code: "P-1", Name: "Product", TrackBatch: true, TrackSerial: true}, want: "a product cannot track both batches and serial numbers"},
		{name: "valid", in: Product{Code: "P-1", Name: "Product", CostMethod: "FIFO"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.validate(tt.in)
			if tt.want == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tt.want)
		})
	}
}

func TestCreateProductDefaultsCostMethod(t *testing.T) {
	repo := newMemoryRepo()
	created, err := NewService(repo).Create(context.Background(), Product{Code: "P-AVG", Name: "Average cost"})
	require.NoError(t, err)
	require.Equal(t, "AVG", created.CostMethod)
}

func TestProductListAndDelete(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(repo)
	created, err := svc.Create(context.Background(), Product{Code: "P-LIST", Name: "Listed"})
	require.NoError(t, err)
	items, total, err := svc.List(context.Background(), shared.ListFilters{})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, created.ID, items[0].ID)
	require.NoError(t, svc.Delete(context.Background(), created.ID))
	_, err = svc.Get(context.Background(), created.ID)
	require.ErrorIs(t, err, shared.ErrNotFound)
}
