package shared

import (
	"errors"
	"log/slog"
	"strings"
)

// ============================================================================
// Domain Sentinel Errors
// ============================================================================

var (
	// ErrNotFound indicates resource not found.
	ErrNotFound = errors.New("not found")
	// ErrDuplicate indicates a duplicate entry conflict.
	ErrDuplicate = errors.New("duplicate entry")
	// ErrValidation indicates validation failure.
	ErrValidation = errors.New("validation failed")
	// ErrForbidden indicates access denied.
	ErrForbidden = errors.New("forbidden")
	// ErrUnauthorized indicates authentication required.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrInvalidCredentials indicates login failure.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrCSRFTokenMissing occurs when CSRF token missing.
	ErrCSRFTokenMissing = errors.New("csrf token missing")
	// ErrCSRFTokenMismatch occurs when CSRF tokens do not match.
	ErrCSRFTokenMismatch = errors.New("csrf token mismatch")
	// ErrInvalidInput indicates malformed user input.
	ErrInvalidInput = errors.New("invalid input")
	// ErrConflict indicates a state conflict (e.g., already processed).
	ErrConflict = errors.New("conflict")
)

// ============================================================================
// FormErrors - Shared type for SSR form validation errors
// ============================================================================

// FormErrors holds field-level validation errors for template rendering.
// Use "_form" key for general form-level errors.
type FormErrors map[string]string

// Add adds an error for a specific field.
func (fe FormErrors) Add(field, message string) {
	fe[field] = message
}

// HasErrors returns true if there are any errors.
func (fe FormErrors) HasErrors() bool {
	return len(fe) > 0
}

// General sets or gets the general form error (non-field-specific).
func (fe FormErrors) General() string {
	return fe["_form"]
}

// SetGeneral sets a general form error.
func (fe FormErrors) SetGeneral(message string) {
	fe["_form"] = message
}

// ============================================================================
// User-Safe Error Messages
// ============================================================================

// userSafeMessages maps known error types to user-friendly messages.
// These are safe to display to end users.
var userSafeMessages = map[error]string{
	ErrNotFound:           "The requested item was not found.",
	ErrDuplicate:          "This item already exists.",
	ErrValidation:         "Please check your input and try again.",
	ErrForbidden:          "You don't have permission to perform this action.",
	ErrUnauthorized:       "Please log in to continue.",
	ErrInvalidCredentials: "Invalid username or password.",
	ErrCSRFTokenMissing:   "Security token missing. Please refresh and try again.",
	ErrCSRFTokenMismatch:  "Security token expired. Please refresh and try again.",
	ErrInvalidInput:       "The provided input is invalid.",
	ErrConflict:           "This action cannot be completed due to a conflict.",
}

// UserSafeMessage returns a user-friendly error message.
// It maps known sentinel errors to safe messages and returns a generic
// message for unknown errors to prevent leaking internal details.
//
// Usage:
//
//	h.redirectWithFlash(w, r, "/path", "error", shared.UserSafeMessage(err))
func UserSafeMessage(err error) string {
	if err == nil {
		return ""
	}

	// Check for known sentinel errors
	for sentinel, message := range userSafeMessages {
		if errors.Is(err, sentinel) {
			return message
		}
	}

	// For unknown errors, return generic message (don't leak internals)
	return "An unexpected error occurred. Please try again."
}

// UserSafeMessageWithLog returns a user-friendly message and logs the actual error.
// Use this in handlers to both log the real error and return a safe message.
//
// Usage:
//
//	msg := shared.UserSafeMessageWithLog(err, h.logger, "failed to create order", slog.Int64("order_id", id))
//	h.redirectWithFlash(w, r, "/path", "error", msg)
func UserSafeMessageWithLog(err error, logger *slog.Logger, context string, attrs ...any) string {
	if err == nil {
		return ""
	}

	// Log the actual error with context
	allAttrs := append([]any{slog.Any("error", err)}, attrs...)
	logger.Error(context, allAttrs...)

	return UserSafeMessage(err)
}

// ============================================================================
// Error Classification Helpers
// ============================================================================

// IsUserError returns true if the error is caused by user input
// (validation, not found, etc.) rather than a system failure.
func IsUserError(err error) bool {
	return errors.Is(err, ErrNotFound) ||
		errors.Is(err, ErrDuplicate) ||
		errors.Is(err, ErrValidation) ||
		errors.Is(err, ErrInvalidInput) ||
		errors.Is(err, ErrConflict) ||
		errors.Is(err, ErrInvalidCredentials)
}

// IsAuthError returns true if the error is authentication/authorization related.
func IsAuthError(err error) bool {
	return errors.Is(err, ErrForbidden) ||
		errors.Is(err, ErrUnauthorized) ||
		errors.Is(err, ErrCSRFTokenMissing) ||
		errors.Is(err, ErrCSRFTokenMismatch)
}

// ============================================================================
// Validation Helpers
// ============================================================================

// ValidationError wraps ErrValidation with field-specific details.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Field != "" {
		return e.Field + ": " + e.Message
	}
	return e.Message
}

func (e *ValidationError) Unwrap() error {
	return ErrValidation
}

// NewValidationError creates a new validation error for a specific field.
func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{Field: field, Message: message}
}

// ============================================================================
// Error Message Sanitization
// ============================================================================

// sensitivePatterns contains substrings that indicate an error message
// might contain sensitive information.
var sensitivePatterns = []string{
	"password",
	"token",
	"secret",
	"key",
	"credential",
	"auth",
	"sql",
	"database",
	"connection",
	"timeout",
	"panic",
	"runtime",
	"goroutine",
	"stack",
	"pq:",        // PostgreSQL driver errors
	"redis:",     // Redis errors
	"dial tcp",   // Network errors
	"EOF",        // IO errors
	"i/o",        // IO errors
	"no such",    // File system errors
	"permission", // Permission errors that might leak paths
}

// SanitizeErrorMessage checks if an error message contains sensitive patterns
// and returns a safe version. This is a last-resort sanitizer for cases where
// err.Error() might be displayed.
//
// Prefer using UserSafeMessage() instead when possible.
func SanitizeErrorMessage(msg string) string {
	lower := strings.ToLower(msg)
	for _, pattern := range sensitivePatterns {
		if strings.Contains(lower, pattern) {
			return "An error occurred. Please try again or contact support."
		}
	}
	// If no sensitive patterns found, the message is probably safe
	// but truncate if too long
	if len(msg) > 200 {
		return msg[:197] + "..."
	}
	return msg
}
