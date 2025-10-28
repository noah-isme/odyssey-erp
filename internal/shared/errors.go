package shared

import "errors"

var (
	// ErrNotFound indicates resource not found.
	ErrNotFound = errors.New("not found")
	// ErrInvalidCredentials indicates login failure.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrCSRFTokenMissing occurs when CSRF token missing.
	ErrCSRFTokenMissing = errors.New("csrf token missing")
	// ErrCSRFTokenMismatch occurs when CSRF tokens do not match.
	ErrCSRFTokenMismatch = errors.New("csrf token mismatch")
)
