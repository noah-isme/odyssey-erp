package assets

import (
	"context"
	"time"
)

type Asset struct {
	ID              int64
	Number          string
	Name            string
	Category        string
	AcquisitionCost float64
	AccumulatedDep  float64
	Status          string
}

type Category struct {
	ID              int64
	Code            string
	Name            string
	AssetAccount    string
	AccumAccount    string
	ExpenseAccount  string
	UsefulLifeMonths int
	ResidualRate    float64
}

type CreateAssetInput struct {
	CompanyID       int64
	CategoryID      int64
	Number          string
	Name            string
	AcquisitionDate time.Time
	InServiceDate   time.Time
	AcquisitionCost float64
	UsefulLifeMonths int
}

type CreateCategoryInput struct {
	CompanyID             int64
	Code                  string
	Name                  string
	AssetAccountID        int64
	AccumDepAccountID     int64
	DepExpenseAccountID   int64
	CashProceedsAccountID int64
	DisposalGainAccountID int64
	DisposalLossAccountID int64
	UsefulLifeMonths      int
	ResidualRate          float64
}

type Repository interface {
	ListAssets(ctx context.Context, companyID int64) ([]Asset, error)
	ListCategories(ctx context.Context, companyID int64) ([]Category, error)
	CreateAsset(ctx context.Context, input CreateAssetInput) error
	CreateCategory(ctx context.Context, input CreateCategoryInput) error
	DisposeAsset(ctx context.Context, id int64, date time.Time, proceeds float64) error
}