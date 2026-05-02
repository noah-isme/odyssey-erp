package customers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type memoryRepo struct {
	customers map[int64]*Customer
	nextID    int64
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{
		customers: make(map[int64]*Customer),
		nextID:    1,
	}
}

func (m *memoryRepo) WithTx(ctx context.Context, fn func(context.Context, Repository) error) error {
	return fn(ctx, m)
}

func (m *memoryRepo) Create(ctx context.Context, c Customer) (int64, error) {
	id := m.nextID
	m.nextID++
	c.ID = id
	m.customers[id] = &c
	return id, nil
}

func (m *memoryRepo) Update(ctx context.Context, id int64, updates map[string]interface{}) error {
	c, ok := m.customers[id]
	if !ok {
		return ErrNotFound
	}
	if v, ok := updates["name"].(string); ok {
		c.Name = v
	}
	if v, ok := updates["email"].(string); ok {
		c.Email = &v
	}
	return nil
}

func (m *memoryRepo) Get(ctx context.Context, id int64) (*Customer, error) {
	c, ok := m.customers[id]
	if !ok {
		return nil, ErrNotFound
	}
	return c, nil
}

func (m *memoryRepo) GetByCode(ctx context.Context, companyID int64, code string) (*Customer, error) {
	for _, c := range m.customers {
		if c.CompanyID == companyID && c.Code == code {
			return c, nil
		}
	}
	return nil, ErrNotFound
}

func (m *memoryRepo) List(ctx context.Context, req ListCustomersRequest) ([]Customer, int, error) {
	return nil, 0, nil
}

func (m *memoryRepo) GenerateCode(ctx context.Context, companyID int64) (string, error) {
	return "CUST-001", nil
}

func TestCreateCustomer(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(repo)

	req := CreateCustomerRequest{
		CompanyID: 1,
		Code:      "C001",
		Name:      "Test Customer",
	}

	cust, err := svc.Create(context.Background(), req, 1)
	require.NoError(t, err)
	require.Equal(t, "C001", cust.Code)
	require.Equal(t, int64(1), cust.ID)

	// Test duplicate code
	_, err = svc.Create(context.Background(), req, 1)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrAlreadyExists)
}

func TestUpdateCustomer(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(repo)

	cust, _ := svc.Create(context.Background(), CreateCustomerRequest{
		CompanyID: 1,
		Code:      "C001",
		Name:      "Original",
	}, 1)

	newName := "Updated"
	updated, err := svc.Update(context.Background(), cust.ID, UpdateCustomerRequest{
		Name: &newName,
	})
	require.NoError(t, err)
	require.Equal(t, "Updated", updated.Name)
}
