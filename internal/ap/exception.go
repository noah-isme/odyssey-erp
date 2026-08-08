package ap

import (
	"context"
	"errors"
	"time"
)

type ExceptionService struct {
	repo Repository
}

func NewExceptionService(repo Repository) *ExceptionService {
	return &ExceptionService{repo: repo}
}

func (s *ExceptionService) CreateException(ctx context.Context, exc APException) (int64, error) {
	if exc.Status == "" {
		exc.Status = "OPEN"
	}
	if exc.SLADueAt == nil {
		sla := time.Now().Add(24 * time.Hour)
		exc.SLADueAt = &sla
	}
	var id int64
	err := s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		var err error
		id, err = tx.CreateAPException(ctx, exc)
		return err
	})
	return id, err
}

func (s *ExceptionService) ListExceptions(ctx context.Context, status string, ownerID, invoiceID int64, limit, offset int) ([]APException, error) {
	if limit == 0 {
		limit = 100
	}
	return s.repo.ListAPExceptions(ctx, status, ownerID, invoiceID, limit, offset)
}

func (s *ExceptionService) ResolveException(ctx context.Context, id int64, resolvedBy int64, resolution string) error {
	exc, err := s.repo.GetAPException(ctx, id)
	if err != nil {
		return err
	}
	if exc.Status == "RESOLVED" || exc.Status == "REJECTED" {
		return errors.New("exception is already closed")
	}

	return s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		return tx.UpdateAPExceptionStatus(ctx, id, resolution, &resolvedBy)
	})
}
