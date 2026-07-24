package assets

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *repository {
	return &repository{pool: pool}
}

func (r *repository) ListAssets(ctx context.Context, companyID int64) ([]Asset, error) {
	rows, err := r.pool.Query(ctx, `SELECT a.id, a.number, a.name, c.name, a.acquisition_cost, a.accumulated_depreciation, a.status FROM fixed_assets a JOIN fixed_asset_categories c ON c.id=a.category_id WHERE a.company_id=$1 ORDER BY a.number`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assets []Asset
	for rows.Next() {
		var a Asset
		if err := rows.Scan(&a.ID, &a.Number, &a.Name, &a.Category, &a.AcquisitionCost, &a.AccumulatedDep, &a.Status); err != nil {
			return nil, err
		}
		assets = append(assets, a)
	}
	return assets, nil
}

func (r *repository) ListCategories(ctx context.Context, companyID int64) ([]Category, error) {
	rows, err := r.pool.Query(ctx, `SELECT c.id, c.code, c.name, a.name, ad.name, e.name, c.useful_life_months, c.residual_rate FROM fixed_asset_categories c JOIN accounts a ON a.id=c.asset_account_id JOIN accounts ad ON ad.id=c.accumulated_depreciation_account_id JOIN accounts e ON e.id=c.depreciation_expense_account_id WHERE c.company_id=$1 ORDER BY c.code`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categories []Category
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Code, &c.Name, &c.AssetAccount, &c.AccumAccount, &c.ExpenseAccount, &c.UsefulLifeMonths, &c.ResidualRate); err != nil {
			return nil, err
		}
		categories = append(categories, c)
	}
	return categories, nil
}

func (r *repository) CreateAsset(ctx context.Context, input CreateAssetInput) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO fixed_assets (company_id, category_id, number, name, acquisition_date, in_service_date, acquisition_cost, useful_life_months) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, input.CompanyID, input.CategoryID, input.Number, input.Name, input.AcquisitionDate, input.InServiceDate, input.AcquisitionCost, input.UsefulLifeMonths)
	return err
}

func (r *repository) CreateCategory(ctx context.Context, input CreateCategoryInput) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO fixed_asset_categories (company_id, code, name, asset_account_id, accumulated_depreciation_account_id, depreciation_expense_account_id, cash_proceeds_account_id, disposal_gain_account_id, disposal_loss_account_id, useful_life_months, residual_rate) VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,0),NULLIF($8,0),NULLIF($9,0),$10,$11)`, input.CompanyID, input.Code, input.Name, input.AssetAccountID, input.AccumDepAccountID, input.DepExpenseAccountID, input.CashProceedsAccountID, input.DisposalGainAccountID, input.DisposalLossAccountID, input.UsefulLifeMonths, input.ResidualRate)
	return err
}

func (r *repository) DisposeAsset(ctx context.Context, id int64, date time.Time, proceeds float64) error {
	_, err := r.pool.Exec(ctx, `UPDATE fixed_assets SET status='DISPOSED', disposal_date=$2, disposal_proceeds=$3 WHERE id=$1`, id, date, proceeds)
	return err
}

type txRepo struct {
	tx pgx.Tx
}

func (r *txRepo) CreateAsset(ctx context.Context, input CreateAssetInput) error {
	_, err := r.tx.Exec(ctx, `INSERT INTO fixed_assets (company_id, category_id, number, name, acquisition_date, in_service_date, acquisition_cost, useful_life_months) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, input.CompanyID, input.CategoryID, input.Number, input.Name, input.AcquisitionDate, input.InServiceDate, input.AcquisitionCost, input.UsefulLifeMonths)
	return err
}

func (r *txRepo) CreateCategory(ctx context.Context, input CreateCategoryInput) error {
	_, err := r.tx.Exec(ctx, `INSERT INTO fixed_asset_categories (company_id, code, name, asset_account_id, accumulated_depreciation_account_id, depreciation_expense_account_id, cash_proceeds_account_id, disposal_gain_account_id, disposal_loss_account_id, useful_life_months, residual_rate) VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,0),NULLIF($8,0),NULLIF($9,0),$10,$11)`, input.CompanyID, input.Code, input.Name, input.AssetAccountID, input.AccumDepAccountID, input.DepExpenseAccountID, input.CashProceedsAccountID, input.DisposalGainAccountID, input.DisposalLossAccountID, input.UsefulLifeMonths, input.ResidualRate)
	return err
}

func (r *txRepo) DisposeAsset(ctx context.Context, id int64, date time.Time, proceeds float64) error {
	_, err := r.tx.Exec(ctx, `UPDATE fixed_assets SET status='DISPOSED', disposal_date=$2, disposal_proceeds=$3 WHERE id=$1`, id, date, proceeds)
	return err
}