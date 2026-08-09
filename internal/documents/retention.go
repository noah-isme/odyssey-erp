package documents

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// CreateDisposition creates a pending request for a tenant-owned version.
// Approval remains a separate operation so automatic retention expiry cannot
// silently delete content.
func (s *Service) CreateDisposition(ctx context.Context, req CreateDispositionRequest) (DispositionRequest, error) {
	if req.CompanyID <= 0 || req.DocumentVersionID <= 0 || req.PolicyID <= 0 || req.RequestedBy <= 0 {
		return DispositionRequest{}, errors.New("documents: company, version, policy, and requester ids are required")
	}
	version, err := s.repo.GetDocumentVersion(ctx, req.DocumentVersionID)
	if err != nil {
		return DispositionRequest{}, err
	}
	if version.CompanyID != req.CompanyID {
		return DispositionRequest{}, errors.New("documents: document version does not belong to company")
	}
	if err := s.repo.RetentionPolicyExists(ctx, req.PolicyID, req.CompanyID); err != nil {
		return DispositionRequest{}, err
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Reason == "" {
		req.Reason = fmt.Sprintf("retention policy %d disposition", req.PolicyID)
	}
	created, err := s.repo.CreateDispositionRequest(ctx, req)
	if err != nil {
		return DispositionRequest{}, fmt.Errorf("documents: create disposition request: %w", err)
	}
	return created, nil
}

// UpdateDispositionRequest records an explicit approval or rejection. Only a
// pending request can transition, which keeps worker execution idempotent.
func (s *Service) UpdateDispositionRequest(ctx context.Context, id int64, status string, actorID int64) (DispositionRequest, error) {
	if id <= 0 || actorID <= 0 {
		return DispositionRequest{}, errors.New("documents: disposition id and actor id are required")
	}
	status = strings.ToUpper(strings.TrimSpace(status))
	if status != "APPROVED" && status != "REJECTED" {
		return DispositionRequest{}, errors.New("documents: disposition status must be APPROVED or REJECTED")
	}
	current, err := s.repo.GetDispositionRequest(ctx, id)
	if err != nil {
		return DispositionRequest{}, err
	}
	if current.Status != "PENDING" {
		return DispositionRequest{}, errors.New("documents: disposition request is not pending")
	}
	return s.repo.UpdateDispositionRequest(ctx, id, status, actorID)
}

// ProcessExpiredRetention marks expired schedules and creates pending
// disposition requests. It intentionally stops at approval; the separate
// approved-disposition phase performs legal-hold checks and storage deletion.
func (s *Service) ProcessExpiredRetention(ctx context.Context) error {
	expired, err := s.repo.ListExpiredRetention(ctx)
	if err != nil {
		return fmt.Errorf("documents: list expired retention: %w", err)
	}
	var failures []error
	for _, item := range expired {
		if _, err := s.repo.GetOpenDispositionRequestForVersion(ctx, item.DocumentVersionID); err == nil {
			if err := s.repo.MarkRetentionExpired(ctx, item.ID); err != nil {
				failures = append(failures, fmt.Errorf("retention %d: %w", item.ID, err))
			}
			continue
		} else if !errors.Is(err, ErrNoOpenDisposition) {
			failures = append(failures, fmt.Errorf("retention %d: check disposition: %w", item.ID, err))
			continue
		}

		_, createErr := s.CreateDisposition(ctx, CreateDispositionRequest{
			CompanyID:         item.CompanyID,
			DocumentVersionID: item.DocumentVersionID,
			PolicyID:          item.PolicyID,
			RequestedBy:       1,
			Reason:            fmt.Sprintf("retention policy %d expired", item.PolicyID),
		})
		if createErr != nil {
			failures = append(failures, fmt.Errorf("retention %d: create disposition: %w", item.ID, createErr))
			continue
		}
		if err := s.repo.MarkRetentionExpired(ctx, item.ID); err != nil {
			failures = append(failures, fmt.Errorf("retention %d: mark expired: %w", item.ID, err))
		}
	}
	return errors.Join(failures...)
}

// ProcessRetentionAndDispositions is the scheduled worker entry point.
func (s *Service) ProcessRetentionAndDispositions(ctx context.Context) error {
	return errors.Join(s.ProcessExpiredRetention(ctx), s.ExecuteApprovedDispositions(ctx))
}
