package fixedassets

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/journals"
	"time"
)

// Service owns monthly straight-line depreciation and disposal accounting.
type Service struct {
	db       *pgxpool.Pool
	journals *journals.Service
}

func NewService(db *pgxpool.Pool, journalService *journals.Service) *Service {
	return &Service{db: db, journals: journalService}
}

// RunMonthlyDepreciation posts one journal per eligible asset for the supplied month.
func (s *Service) RunMonthlyDepreciation(ctx context.Context, month time.Time) (int, error) {
	if s.db == nil || s.journals == nil {
		return 0, fmt.Errorf("fixed assets not configured")
	}
	month = time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
	rows, err := s.db.Query(ctx, `SELECT a.id,a.name,a.acquisition_cost,a.residual_value,a.useful_life_months,a.accumulated_depreciation,c.depreciation_expense_account_id,c.accumulated_depreciation_account_id,p.id
 FROM fixed_assets a JOIN fixed_asset_categories c ON c.id=a.category_id JOIN periods p ON $1 BETWEEN p.start_date AND p.end_date
 WHERE a.status='ACTIVE' AND a.in_service_date <= $1 AND (a.last_depreciated_on IS NULL OR a.last_depreciated_on < $1) AND p.status='OPEN'`, month)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id, expense, accum, periodID int64
		var name string
		var cost, residual, life, accumulated float64
		if err := rows.Scan(&id, &name, &cost, &residual, &life, &accumulated, &expense, &accum, &periodID); err != nil {
			return count, err
		}
		amount := monthlyDepreciation(cost, residual, life, accumulated)
		if amount <= 0 {
			continue
		}
		source := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("asset-depreciation:%d:%s", id, month.Format("2006-01"))))
		_, err = s.journals.PostJournal(ctx, journals.PostingInput{PeriodID: periodID, Date: month, SourceModule: "FIXED_ASSET_DEPRECIATION", SourceID: source, Memo: "Depreciation: " + name, Lines: []journals.PostingLineInput{{AccountID: expense, Debit: amount}, {AccountID: accum, Credit: amount}}})
		if err != nil {
			return count, err
		}
		_, err = s.db.Exec(ctx, `UPDATE fixed_assets SET accumulated_depreciation=accumulated_depreciation+$1,last_depreciated_on=$2,status=CASE WHEN accumulated_depreciation+$1>=acquisition_cost-residual_value THEN 'FULLY_DEPRECIATED' ELSE status END,updated_at=NOW() WHERE id=$3`, amount, month, id)
		if err != nil {
			return count, err
		}
		count++
	}
	return count, rows.Err()
}

func monthlyDepreciation(cost, residual, usefulLifeMonths, accumulated float64) float64 {
	if cost <= 0 || usefulLifeMonths <= 0 || residual < 0 || residual >= cost || accumulated >= cost-residual {
		return 0
	}
	amount := (cost - residual) / usefulLifeMonths
	remaining := cost - residual - accumulated
	if amount > remaining {
		return remaining
	}
	return amount
}

// Dispose posts the derecognition journal and marks an asset disposed.
func (s *Service) Dispose(ctx context.Context, assetID int64, date time.Time, proceeds float64) error {
	if assetID <= 0 || proceeds < 0 || date.IsZero() {
		return fmt.Errorf("fixed assets: invalid disposal input")
	}
	var periodID, assetAccount, accumAccount, cashAccount, gainAccount, lossAccount int64
	var name string
	var cost, accumulated float64
	err := s.db.QueryRow(ctx, `SELECT p.id,c.asset_account_id,c.accumulated_depreciation_account_id,COALESCE(c.cash_proceeds_account_id,0),COALESCE(c.disposal_gain_account_id,0),COALESCE(c.disposal_loss_account_id,0),a.name,a.acquisition_cost,a.accumulated_depreciation
		FROM fixed_assets a JOIN fixed_asset_categories c ON c.id=a.category_id JOIN periods p ON $2 BETWEEN p.start_date AND p.end_date WHERE a.id=$1 AND a.status!='DISPOSED' AND p.status='OPEN'`, assetID, date).Scan(&periodID, &assetAccount, &accumAccount, &cashAccount, &gainAccount, &lossAccount, &name, &cost, &accumulated)
	if err != nil {
		return err
	}
	book := cost - accumulated
	lines := []journals.PostingLineInput{{AccountID: accumAccount, Debit: accumulated}, {AccountID: assetAccount, Credit: cost}}
	if proceeds > 0 && cashAccount > 0 {
		lines = append(lines, journals.PostingLineInput{AccountID: cashAccount, Debit: proceeds})
	}
	difference := proceeds - book
	if difference > 0 && gainAccount > 0 {
		lines = append(lines, journals.PostingLineInput{AccountID: gainAccount, Credit: difference})
	} else if difference < 0 && lossAccount > 0 {
		lines = append(lines, journals.PostingLineInput{AccountID: lossAccount, Debit: -difference})
	} else if difference != 0 {
		return fmt.Errorf("fixed assets: disposal gain/loss account is required")
	}
	entry, err := s.journals.PostJournal(ctx, journals.PostingInput{PeriodID: periodID, Date: date, SourceModule: "FIXED_ASSET_DISPOSAL", SourceID: uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("asset-disposal:%d", assetID))), Memo: "Disposal: " + name, Lines: lines})
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO fixed_asset_disposals(asset_id,disposal_date,proceeds,journal_entry_id) VALUES($1,$2,$3,$4); UPDATE fixed_assets SET status='DISPOSED',updated_at=NOW() WHERE id=$1`, assetID, date, proceeds, entry.ID)
	return err
}
