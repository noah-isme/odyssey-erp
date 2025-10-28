package analytics

import (
	"context"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/analytics/db"
)

// AgingFilter scopes AR/AP aging computations.
type AgingFilter struct {
	AsOf      time.Time
	CompanyID int64
	BranchID  *int64
}

// AgingBucket summarises an amount inside a time bucket.
type AgingBucket struct {
	Bucket string
	Amount float64
}

// GetARAging fetches receivable aging buckets.
func (s *Service) GetARAging(ctx context.Context, filter AgingFilter) ([]AgingBucket, error) {
	return s.fetchAging(ctx, filter, true)
}

// GetAPAging fetches payable aging buckets.
func (s *Service) GetAPAging(ctx context.Context, filter AgingFilter) ([]AgingBucket, error) {
	return s.fetchAging(ctx, filter, false)
}

func (s *Service) fetchAging(ctx context.Context, filter AgingFilter, ar bool) ([]AgingBucket, error) {
	if filter.AsOf.IsZero() {
		filter.AsOf = time.Now().UTC().Truncate(24 * time.Hour)
	}
	loader := func(ctx context.Context) (interface{}, error) {
		if ar {
			rows, err := s.repo.AgingAR(ctx, analyticsdb.AgingARParams{
				AsOf:      dateParam(filter.AsOf),
				CompanyID: filter.CompanyID,
				BranchID:  optionalBranch(filter.BranchID),
			})
			if err != nil {
				return nil, err
			}
			buckets := make([]AgingBucket, 0, len(rows))
			for _, row := range rows {
				buckets = append(buckets, AgingBucket{Bucket: row.Bucket, Amount: row.Amount})
			}
			return buckets, nil
		}
		rows, err := s.repo.AgingAP(ctx, analyticsdb.AgingAPParams{
			AsOf:      dateParam(filter.AsOf),
			CompanyID: filter.CompanyID,
			BranchID:  optionalBranch(filter.BranchID),
		})
		if err != nil {
			return nil, err
		}
		buckets := make([]AgingBucket, 0, len(rows))
		for _, row := range rows {
			buckets = append(buckets, AgingBucket{Bucket: row.Bucket, Amount: row.Amount})
		}
		return buckets, nil
	}

	if s.cache == nil {
		value, err := loader(ctx)
		if err != nil {
			return nil, err
		}
		return value.([]AgingBucket), nil
	}

	prefix := "aging_ap"
	if ar {
		prefix = "aging_ar"
	}
	keyBase := keyAging(prefix, filter.CompanyID, filter.BranchID, filter.AsOf)
	key, err := s.cache.BuildKey(ctx, keyBase)
	if err != nil {
		return nil, err
	}
	var buckets []AgingBucket
	if err := s.cache.FetchJSON(ctx, key, &buckets, loader); err != nil {
		return nil, err
	}
	return buckets, nil
}
