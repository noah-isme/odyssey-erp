package assets

import (
	"context"
	"errors"
	"time"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) ListAssets(ctx context.Context, companyID int64) ([]Asset, error) {
	if companyID == 0 {
		return nil, errors.New("company required")
	}
	return s.repo.ListAssets(ctx, companyID)
}

func (s *Service) ListCategories(ctx context.Context, companyID int64) ([]Category, error) {
	if companyID == 0 {
		return nil, errors.New("company required")
	}
	return s.repo.ListCategories(ctx, companyID)
}

func (s *Service) CreateAsset(ctx context.Context, companyID int64, categoryID int64, number, name string, acquisitionDate, inServiceDate time.Time, acquisitionCost float64, usefulLifeMonths int) error {
	if companyID == 0 || categoryID == 0 || number == "" || name == "" || acquisitionCost <= 0 || usefulLifeMonths <= 0 {
		return errors.New("invalid asset data")
	}
	return s.repo.CreateAsset(ctx, CreateAssetInput{
		CompanyID:       companyID,
		CategoryID:      categoryID,
		Number:          number,
		Name:            name,
		AcquisitionDate: acquisitionDate,
		InServiceDate:   inServiceDate,
		AcquisitionCost: acquisitionCost,
		UsefulLifeMonths: usefulLifeMonths,
	})
}

func (s *Service) CreateCategory(ctx context.Context, companyID int64, code, name string, assetAccountID, accumDepAccountID, depExpenseAccountID, cashProceedsAccountID, disposalGainAccountID, disposalLossAccountID int64, usefulLifeMonths int, residualRate float64) error {
	if companyID == 0 || code == "" || name == "" || usefulLifeMonths <= 0 || assetAccountID == 0 || accumDepAccountID == 0 || depExpenseAccountID == 0 {
		return errors.New("invalid category data")
	}
	return s.repo.CreateCategory(ctx, CreateCategoryInput{
		CompanyID:             companyID,
		Code:                  code,
		Name:                  name,
		AssetAccountID:        assetAccountID,
		AccumDepAccountID:     accumDepAccountID,
		DepExpenseAccountID:   depExpenseAccountID,
		CashProceedsAccountID: cashProceedsAccountID,
		DisposalGainAccountID: disposalGainAccountID,
		DisposalLossAccountID: disposalLossAccountID,
		UsefulLifeMonths:      usefulLifeMonths,
		ResidualRate:          residualRate,
	})
}

func (s *Service) DisposeAsset(ctx context.Context, id int64, date time.Time, proceeds float64) error {
	if id <= 0 {
		return errors.New("invalid asset ID")
	}
	return s.repo.DisposeAsset(ctx, id, date, proceeds)
}