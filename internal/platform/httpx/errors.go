// Package httpx provides HTTP response utilities.
package httpx

import (
	"errors"
	"net/http"
)

// Sentinel errors for domain layer.
var (
	ErrNotFound    = errors.New("resource not found")
	ErrDuplicate   = errors.New("duplicate entry")
	ErrValidation  = errors.New("validation failed")
	ErrForbidden   = errors.New("forbidden")
	ErrUnauthorized = errors.New("unauthorized")
)

// RespondError maps domain errors to HTTP responses using RFC7807.
func RespondError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		Problem(w, http.StatusNotFound, "Not Found", err.Error())
	case errors.Is(err, ErrDuplicate):
		Problem(w, http.StatusConflict, "Duplicate", err.Error())
	case errors.Is(err, ErrValidation):
		Problem(w, http.StatusBadRequest, "Validation Failed", err.Error())
	case errors.Is(err, ErrForbidden):
		Problem(w, http.StatusForbidden, "Forbidden", err.Error())
	case errors.Is(err, ErrUnauthorized):
		Problem(w, http.StatusUnauthorized, "Unauthorized", err.Error())
	default:
		Problem(w, http.StatusInternalServerError, "Internal Error", "")
	}
}
