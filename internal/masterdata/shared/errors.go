package shared

import "errors"

var (
	ErrNotFound      = errors.New("resource not found")
	ErrDuplicate     = errors.New("duplicate entry")
	ErrValidation    = errors.New("validation failed")
	ErrInvalidID     = errors.New("invalid ID")
	ErrRequiredField = errors.New("field is required")
)
