package assets

import (
	"context"
	"testing"
	"time"
)

type serviceRepo struct {
	assetInput    CreateAssetInput
	categoryInput CreateCategoryInput
	assetCalls    int
	categoryCalls int
	disposedID    int64
}

func (r *serviceRepo) ListAssets(context.Context, int64) ([]Asset, error) {
	return []Asset{{ID: 1}}, nil
}
func (r *serviceRepo) ListCategories(context.Context, int64) ([]Category, error) {
	return []Category{{ID: 2}}, nil
}
func (r *serviceRepo) CreateAsset(_ context.Context, in CreateAssetInput) error {
	r.assetCalls++
	r.assetInput = in
	return nil
}
func (r *serviceRepo) CreateCategory(_ context.Context, in CreateCategoryInput) error {
	r.categoryCalls++
	r.categoryInput = in
	return nil
}
func (r *serviceRepo) DisposeAsset(_ context.Context, id int64, _ time.Time, _ float64) error {
	r.disposedID = id
	return nil
}

func TestServiceValidatesInputsBeforeRepository(t *testing.T) {
	repo := &serviceRepo{}
	svc := NewService(repo)
	ctx := context.Background()

	if _, err := svc.ListAssets(ctx, 0); err == nil {
		t.Fatal("ListAssets() accepted a missing company")
	}
	if err := svc.CreateAsset(ctx, 1, 2, "", "Asset", time.Time{}, time.Time{}, 10, 12); err == nil {
		t.Fatal("CreateAsset() accepted a missing number")
	}
	if err := svc.CreateCategory(ctx, 1, "CAT", "Category", 0, 2, 3, 0, 0, 0, 12, 0); err == nil {
		t.Fatal("CreateCategory() accepted a missing asset account")
	}
	if err := svc.DisposeAsset(ctx, 0, time.Now(), 1); err == nil {
		t.Fatal("DisposeAsset() accepted a missing asset ID")
	}
	if repo.assetCalls != 0 || repo.categoryCalls != 0 {
		t.Fatal("invalid inputs reached the repository")
	}
}

func TestServiceForwardsValidAssetOperations(t *testing.T) {
	repo := &serviceRepo{}
	svc := NewService(repo)
	when := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	if err := svc.CreateAsset(context.Background(), 1, 2, "FA-1", "Laptop", when, when, 1200, 36); err != nil {
		t.Fatal(err)
	}
	if repo.assetInput.CompanyID != 1 || repo.assetInput.AcquisitionCost != 1200 || repo.assetInput.UsefulLifeMonths != 36 {
		t.Fatalf("CreateAsset() input = %#v", repo.assetInput)
	}
	if err := svc.CreateCategory(context.Background(), 1, "EQUIP", "Equipment", 10, 11, 12, 13, 14, 15, 36, 0.1); err != nil {
		t.Fatal(err)
	}
	if repo.categoryInput.DisposalLossAccountID != 15 || repo.categoryInput.ResidualRate != 0.1 {
		t.Fatalf("CreateCategory() input = %#v", repo.categoryInput)
	}
	if err := svc.DisposeAsset(context.Background(), 9, when, 100); err != nil || repo.disposedID != 9 {
		t.Fatalf("DisposeAsset() = %v, id %d", err, repo.disposedID)
	}
}
