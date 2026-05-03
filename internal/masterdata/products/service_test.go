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
	return nil, 0, nil
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
