package documents

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

// CreateCollaborationSession persists an active editing session for a
// tenant-owned document version. The token is returned to the caller so a
// collaboration transport can authenticate subsequent websocket requests.
func (s *Service) CreateCollaborationSession(ctx context.Context, req CollaborationSession) (CollaborationSession, error) {
	if req.CompanyID <= 0 || req.VersionID <= 0 || req.HostUserID <= 0 {
		return CollaborationSession{}, errors.New("documents: company, version, and host user ids are required")
	}
	if req.Status != "" && strings.ToUpper(strings.TrimSpace(req.Status)) != "ACTIVE" {
		return CollaborationSession{}, errors.New("documents: collaboration session must start active")
	}

	version, err := s.repo.GetDocumentVersion(ctx, req.VersionID)
	if err != nil {
		return CollaborationSession{}, err
	}
	if version.CompanyID != req.CompanyID {
		return CollaborationSession{}, errors.New("documents: document version does not belong to company")
	}

	token, err := newCollaborationToken()
	if err != nil {
		return CollaborationSession{}, err
	}
	expiresAt := s.now().Add(24 * time.Hour)
	id, err := s.repo.CreateCollaborationSession(ctx, DocumentCollaborationSession{
		CompanyID:         req.CompanyID,
		DocumentVersionID: req.VersionID,
		SessionToken:      token,
		HostUserID:        req.HostUserID,
		Active:            true,
		ExpiresAt:         expiresAt,
	})
	if err != nil {
		return CollaborationSession{}, fmt.Errorf("documents: create collaboration session: %w", err)
	}

	return CollaborationSession{
		ID:           id,
		CompanyID:    req.CompanyID,
		VersionID:    req.VersionID,
		HostUserID:   req.HostUserID,
		SessionToken: token,
		Status:       "ACTIVE",
		CreatedAt:    s.now(),
		ExpiresAt:    expiresAt,
	}, nil
}

// RecordCollaborationChange appends an immutable operation to an active,
// unexpired collaboration session. The session lookup is also the tenant
// boundary: callers cannot write to a session owned by another company.
func (s *Service) RecordCollaborationChange(ctx context.Context, req CollaborationChange) (CollaborationChange, error) {
	if req.CompanyID <= 0 || req.SessionID <= 0 || req.ActorID <= 0 {
		return CollaborationChange{}, errors.New("documents: company, session, and actor ids are required")
	}
	req.Operation = strings.ToUpper(strings.TrimSpace(req.Operation))
	switch req.Operation {
	case "INSERT", "DELETE", "REPLACE":
	default:
		return CollaborationChange{}, fmt.Errorf("documents: invalid collaboration operation %q", req.Operation)
	}
	if strings.TrimSpace(req.Payload) == "" {
		return CollaborationChange{}, errors.New("documents: collaboration payload required")
	}

	session, err := s.repo.GetCollaborationSession(ctx, req.SessionID)
	if err != nil {
		return CollaborationChange{}, err
	}
	if session.CompanyID != req.CompanyID {
		return CollaborationChange{}, errors.New("documents: collaboration session does not belong to company")
	}
	if !session.Active || !session.ExpiresAt.After(s.now()) {
		return CollaborationChange{}, errors.New("documents: collaboration session is inactive or expired")
	}
	if req.Timestamp.IsZero() {
		req.Timestamp = s.now()
	}

	created, err := s.repo.CreateCollaborationChange(ctx, req)
	if err != nil {
		return CollaborationChange{}, fmt.Errorf("documents: record collaboration change: %w", err)
	}
	created.CompanyID = req.CompanyID
	return created, nil
}

// SearchContent is the public content-search entry point. It keeps input
// validation here and delegates to the tenant-scoped PostgreSQL full-text
// query used by the library and OCR worker.
func (s *Service) SearchContent(ctx context.Context, companyID int64, query string) ([]Document, error) {
	if companyID <= 0 {
		return nil, errors.New("documents: company id required")
	}
	if strings.TrimSpace(query) == "" {
		return nil, errors.New("documents: search query required")
	}
	return s.SearchDocumentsFullText(ctx, companyID, strings.TrimSpace(query), 20)
}

func newCollaborationToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("documents: generate collaboration token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
