package mrp

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

// verifyUserReauthentication checks a live credential against the account that
// is about to sign. It deliberately returns the authentication method rather
// than the credential or a client-provided evidence string.
func verifyUserReauthentication(ctx context.Context, tx pgx.Tx, actorID int64, token string) (string, bool, error) {
	if actorID <= 0 || strings.TrimSpace(token) == "" {
		return "", false, nil
	}

	var (
		passwordHash string
		mfaEnabled   bool
		totpSecret   *string
	)
	if err := tx.QueryRow(ctx, `
		SELECT password_hash, mfa_enabled, NULLIF(totp_secret, '')
		FROM users
		WHERE id = $1 AND is_active`, actorID).Scan(&passwordHash, &mfaEnabled, &totpSecret); err != nil {
		if err == pgx.ErrNoRows {
			return "", false, nil
		}
		return "", false, err
	}

	method, valid := verifyReauthenticationCredential(passwordHash, mfaEnabled, stringValue(totpSecret), token)
	return method, valid, nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func verifyReauthenticationCredential(passwordHash string, mfaEnabled bool, totpSecret, token string) (string, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", false
	}
	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(token)) == nil {
		return "PASSWORD", true
	}
	if mfaEnabled && totpSecret != "" && totp.Validate(token, totpSecret) {
		return "TOTP", true
	}
	return "", false
}
