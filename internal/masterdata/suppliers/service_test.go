package suppliers

import (
	"context"
	"testing"

	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/shared"
)

type crudRepo struct {
	items  map[int64]Supplier
	nextID int64
}

func newCRUDRepo() *crudRepo { return &crudRepo{items: make(map[int64]Supplier), nextID: 1} }
func (r *crudRepo) List(context.Context, shared.ListFilters) ([]Supplier, int, error) {
	out := make([]Supplier, 0, len(r.items))
	for _, v := range r.items {
		out = append(out, v)
	}
	return out, len(out), nil
}
func (r *crudRepo) Get(_ context.Context, id int64) (Supplier, error) {
	v, ok := r.items[id]
	if !ok {
		return Supplier{}, shared.ErrNotFound
	}
	return v, nil
}
func (r *crudRepo) Create(_ context.Context, v Supplier) (Supplier, error) {
	v.ID = r.nextID
	r.nextID++
	r.items[v.ID] = v
	return v, nil
}
func (r *crudRepo) Update(_ context.Context, id int64, v Supplier) error {
	if _, ok := r.items[id]; !ok {
		return shared.ErrNotFound
	}
	v.ID = id
	r.items[id] = v
	return nil
}
func (r *crudRepo) Delete(_ context.Context, id int64) error {
	if _, ok := r.items[id]; !ok {
		return shared.ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func TestServiceValidate(t *testing.T) {
	svc := NewService(nil)
	cases := []struct {
		name string
		in   Supplier
		want string
	}{
		{name: "missing code", in: Supplier{Name: "Vendor"}, want: "supplier code is required"},
		{name: "missing name", in: Supplier{Code: "SUP-1"}, want: "supplier name is required"},
		{name: "valid", in: Supplier{Code: "SUP-1", Name: "Vendor"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.validate(tc.in)
			if tc.want == "" && err != nil {
				t.Fatalf("validate() error = %v", err)
			}
			if tc.want != "" && (err == nil || err.Error() != tc.want) {
				t.Fatalf("validate() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestServiceRejectsInvalidIDsBeforeRepository(t *testing.T) {
	svc := NewService(nil)
	ctx := context.Background()
	if _, err := svc.Get(ctx, 0); err == nil {
		t.Fatal("Get() accepted an invalid ID")
	}
	if err := svc.Update(ctx, 0, Supplier{}); err == nil {
		t.Fatal("Update() accepted an invalid ID")
	}
	if err := svc.Delete(ctx, 0); err == nil {
		t.Fatal("Delete() accepted an invalid ID")
	}
}

func TestServiceCRUD(t *testing.T) {
	repo := newCRUDRepo()
	svc := NewService(repo)
	ctx := context.Background()
	created, err := svc.Create(ctx, Supplier{Code: "SUP-1", Name: "Vendor"})
	if err != nil {
		t.Fatal(err)
	}
	if _, total, err := svc.List(ctx, shared.ListFilters{}); err != nil || total != 1 {
		t.Fatalf("List() = %d, %v", total, err)
	}
	created.Name = "Updated"
	if err := svc.Update(ctx, created.ID, created); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get(ctx, created.ID)
	if err != nil || got.Name != "Updated" {
		t.Fatalf("Get() = %#v, %v", got, err)
	}
	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
}
