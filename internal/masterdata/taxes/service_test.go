package taxes

import (
	"context"
	"testing"

	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/shared"
)

type crudRepo struct {
	items  map[int64]Tax
	nextID int64
}

func newCRUDRepo() *crudRepo { return &crudRepo{items: make(map[int64]Tax), nextID: 1} }
func (r *crudRepo) List(context.Context, shared.ListFilters) ([]Tax, int, error) {
	out := make([]Tax, 0, len(r.items))
	for _, v := range r.items {
		out = append(out, v)
	}
	return out, len(out), nil
}
func (r *crudRepo) Get(_ context.Context, id int64) (Tax, error) {
	v, ok := r.items[id]
	if !ok {
		return Tax{}, shared.ErrNotFound
	}
	return v, nil
}
func (r *crudRepo) Create(_ context.Context, v Tax) (Tax, error) {
	v.ID = r.nextID
	r.nextID++
	r.items[v.ID] = v
	return v, nil
}
func (r *crudRepo) Update(_ context.Context, id int64, v Tax) error {
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
		in   Tax
		want string
	}{
		{name: "missing code", in: Tax{Name: "VAT", Rate: 10}, want: "tax code is required"},
		{name: "missing name", in: Tax{Code: "VAT", Rate: 10}, want: "tax name is required"},
		{name: "rate below zero", in: Tax{Code: "VAT", Name: "VAT", Rate: -1}, want: "tax rate must be between 0 and 100"},
		{name: "rate above one hundred", in: Tax{Code: "VAT", Name: "VAT", Rate: 101}, want: "tax rate must be between 0 and 100"},
		{name: "valid", in: Tax{Code: "VAT", Name: "VAT", Rate: 11}},
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
	if err := svc.Update(ctx, 0, Tax{}); err == nil {
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
	created, err := svc.Create(ctx, Tax{Code: "VAT", Name: "VAT", Rate: 11})
	if err != nil {
		t.Fatal(err)
	}
	if _, total, err := svc.List(ctx, shared.ListFilters{}); err != nil || total != 1 {
		t.Fatalf("List() = %d, %v", total, err)
	}
	created.Rate = 12
	if err := svc.Update(ctx, created.ID, created); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get(ctx, created.ID)
	if err != nil || got.Rate != 12 {
		t.Fatalf("Get() = %#v, %v", got, err)
	}
	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
}
