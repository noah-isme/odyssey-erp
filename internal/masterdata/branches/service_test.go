package branches

import (
	"context"
	"testing"

	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/shared"
)

type crudRepo struct {
	items  map[int64]Branch
	nextID int64
}

func newCRUDRepo() *crudRepo { return &crudRepo{items: make(map[int64]Branch), nextID: 1} }
func (r *crudRepo) List(context.Context, shared.ListFilters) ([]Branch, int, error) {
	items := make([]Branch, 0, len(r.items))
	for _, item := range r.items {
		items = append(items, item)
	}
	return items, len(items), nil
}
func (r *crudRepo) Get(_ context.Context, id int64) (Branch, error) {
	item, ok := r.items[id]
	if !ok {
		return Branch{}, shared.ErrNotFound
	}
	return item, nil
}
func (r *crudRepo) Create(_ context.Context, item Branch) (Branch, error) {
	item.ID = r.nextID
	r.nextID++
	r.items[item.ID] = item
	return item, nil
}
func (r *crudRepo) Update(_ context.Context, id int64, item Branch) error {
	if _, ok := r.items[id]; !ok {
		return shared.ErrNotFound
	}
	item.ID = id
	r.items[id] = item
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
		in   Branch
		want string
	}{
		{name: "missing company", in: Branch{Code: "BR-1", Name: "Main"}, want: "company is required"},
		{name: "missing code", in: Branch{CompanyID: 1, Name: "Main"}, want: "branch code is required"},
		{name: "missing name", in: Branch{CompanyID: 1, Code: "BR-1"}, want: "branch name is required"},
		{name: "valid", in: Branch{CompanyID: 1, Code: "BR-1", Name: "Main"}},
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
	if err := svc.Update(ctx, 0, Branch{}); err == nil {
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
	created, err := svc.Create(ctx, Branch{CompanyID: 1, Code: "BR-1", Name: "Main"})
	if err != nil {
		t.Fatal(err)
	}
	if _, total, err := svc.List(ctx, shared.ListFilters{}); err != nil || total != 1 {
		t.Fatalf("List() = %d, %v", total, err)
	}
	updated := created
	updated.Name = "Updated"
	if err := svc.Update(ctx, created.ID, updated); err != nil {
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
