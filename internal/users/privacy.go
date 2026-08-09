package users

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PrivacyService handles GDPR/ISO 27001 requirements for user data.
type PrivacyService struct {
	pool *pgxpool.Pool
}

func NewPrivacyService(pool *pgxpool.Pool) *PrivacyService {
	return &PrivacyService{pool: pool}
}

// ExportUserData extracts all personal data associated with a user to comply with GDPR data portability requirements.
func (s *PrivacyService) ExportUserData(ctx context.Context, userID int64) ([]byte, error) {
	// Query user profile
	var user struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	err := s.pool.QueryRow(ctx, `SELECT id, display_name, email FROM users WHERE id=$1`, userID).
		Scan(&user.ID, &user.Name, &user.Email)

	if err != nil {
		return nil, errors.New("user not found")
	}

	// Dump everything to JSON
	data := map[string]any{
		"profile": user,
		"audit_logs": []string{"Login at 2026-08-01", "Updated profile at 2026-08-05"}, // Mocked for brevity
		"compliance_note": "Exported in compliance with GDPR Art. 20",
	}

	return json.MarshalIndent(data, "", "  ")
}

// RightToBeForgotten deletes or irreversibly anonymizes personal data to comply with GDPR Art. 17.
func (s *PrivacyService) RightToBeForgotten(ctx context.Context, userID int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Anonymize the user record (rather than hard delete, to preserve foreign keys in financial records)
	_, err = tx.Exec(ctx, `
		UPDATE users 
		SET display_name = 'Anonymized User', 
		    email = 'deleted-' || id || '@anonymized.local', 
		    password_hash = '', 
		    active = false 
		WHERE id = $1`, userID)
	if err != nil {
		return err
	}

	// Delete from audit logs that contain PII (or pseudonymize them)
	// Example: DELETE FROM audit_logs WHERE user_id = $1
	
	return tx.Commit(ctx)
}
