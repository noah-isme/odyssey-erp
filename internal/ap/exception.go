package ap

import (
	"context"
	"errors"
	"strings"
	"time"
)

var ErrInvalidExceptionResolution = errors.New("exception resolution must be RESOLVED or REJECTED")
var ErrExceptionCompanyScopeRequired = errors.New("company scope is required")
var ErrExceptionAlreadyClosed = errors.New("exception is already closed")
var ErrExceptionResolverRequired = errors.New("exception resolver is required")

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

// ListExceptionsForCompany applies the tenant boundary in the repository
// query before pagination and workbench filters are evaluated.
func (s *ExceptionService) ListExceptionsForCompany(ctx context.Context, companyID int64, status string, ownerID, invoiceID int64, limit, offset int) ([]APException, error) {
	if companyID <= 0 {
		return nil, ErrExceptionCompanyScopeRequired
	}
	if limit == 0 {
		limit = 100
	}
	return s.repo.ListAPExceptionsForCompany(ctx, companyID, status, ownerID, invoiceID, limit, offset)
}

// GetException returns one exception for request-level scope checks and
// detail views. Keeping this wrapper on the service prevents handlers from
// reaching through to the repository directly.
func (s *ExceptionService) GetException(ctx context.Context, id int64) (APException, error) {
	return s.repo.GetAPException(ctx, id)
}

// GetExceptionForCompany keeps single-record reads inside the same SQL scope
// as list and update operations. A cross-company ID is therefore indistinguishable
// from a missing exception to callers.
func (s *ExceptionService) GetExceptionForCompany(ctx context.Context, companyID, id int64) (APException, error) {
	if companyID <= 0 {
		return APException{}, ErrExceptionCompanyScopeRequired
	}
	return s.repo.GetAPExceptionForCompany(ctx, companyID, id)
}

func (s *ExceptionService) ResolveException(ctx context.Context, id int64, resolvedBy int64, resolution string) error {
	return s.resolveException(ctx, 0, id, resolvedBy, resolution, "")
}

// ResolveExceptionWithComment preserves the legacy resolution API while
// allowing callers that collect operator context to append it immutably.
func (s *ExceptionService) ResolveExceptionWithComment(ctx context.Context, id int64, resolvedBy int64, resolution, comment string) error {
	return s.resolveException(ctx, 0, id, resolvedBy, resolution, comment)
}

func (s *ExceptionService) resolveException(ctx context.Context, companyID, id, resolvedBy int64, resolution, comment string) error {
	if resolvedBy <= 0 {
		return ErrExceptionResolverRequired
	}
	resolution = strings.ToUpper(strings.TrimSpace(resolution))
	if resolution != "RESOLVED" && resolution != "REJECTED" {
		return ErrInvalidExceptionResolution
	}
	comment = strings.TrimSpace(comment)
	var exc APException
	var err error
	if companyID > 0 {
		exc, err = s.repo.GetAPExceptionForCompany(ctx, companyID, id)
	} else {
		exc, err = s.repo.GetAPException(ctx, id)
	}
	if err != nil {
		return err
	}
	if exc.Status == "RESOLVED" || exc.Status == "REJECTED" {
		return ErrExceptionAlreadyClosed
	}

	return s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		if companyID > 0 {
			return tx.ResolveAPExceptionForCompany(ctx, companyID, id, resolution, resolvedBy, comment)
		}
		return tx.ResolveAPException(ctx, id, resolution, resolvedBy, comment)
	})
}

// ResolveExceptionForCompany repeats the scope check on both the read and the
// terminal update. The update's WHERE clause is company-qualified as well, so
// a stale or cross-company ID cannot be changed by a workbench request.
func (s *ExceptionService) ResolveExceptionForCompany(ctx context.Context, companyID, id, resolvedBy int64, resolution string) error {
	return s.ResolveExceptionForCompanyWithComment(ctx, companyID, id, resolvedBy, resolution, "")
}

// ResolveExceptionForCompanyWithComment performs a tenant-scoped terminal
// transition and appends its comment in the same database transaction.
func (s *ExceptionService) ResolveExceptionForCompanyWithComment(ctx context.Context, companyID, id, resolvedBy int64, resolution, comment string) error {
	if companyID <= 0 {
		return ErrExceptionCompanyScopeRequired
	}
	return s.resolveException(ctx, companyID, id, resolvedBy, resolution, comment)
}

// ListResolutionEventsForCompany returns append-only terminal-resolution
// evidence under the same tenant key used by the workbench.
func (s *ExceptionService) ListResolutionEventsForCompany(ctx context.Context, companyID, exceptionID int64) ([]APExceptionResolutionEvent, error) {
	if companyID <= 0 {
		return nil, ErrExceptionCompanyScopeRequired
	}
	return s.repo.ListAPExceptionResolutionEventsForCompany(ctx, companyID, exceptionID)
}
