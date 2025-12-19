package shared

import "errors"

var (
	// ErrUnbalanced indicates debit != credit.
	ErrUnbalanced = errors.New("accounting: journal lines must balance")
	// ErrTooFewLines indicates less than two lines.
	ErrTooFewLines = errors.New("accounting: journal requires at least two lines")
	// ErrInvalidPeriod indicates missing or locked period.
	ErrInvalidPeriod = errors.New("accounting: period is not open")
	// ErrSourceAlreadyLinked indicates idempotency conflict.
	ErrSourceAlreadyLinked = errors.New("accounting: source already linked")
	// ErrJournalNotFound indicates missing entry.
	ErrJournalNotFound = errors.New("accounting: journal entry not found")
	// ErrPeriodLocked indicates locked period.
	ErrPeriodLocked = errors.New("accounting: period locked")
	// ErrInvalidStatus indicates action can't proceed.
	ErrInvalidStatus = errors.New("accounting: invalid status transition")
	// ErrDateOutOfRange indicates journal date mismatch.
	ErrDateOutOfRange = errors.New("accounting: date outside period")
	// ErrMappingNotFound indicates account mapping missing.
	ErrMappingNotFound = errors.New("accounting: account mapping not found")
	// ErrSourceConflict indicates the source link already exists.
	ErrSourceConflict = errors.New("accounting: source link conflict")
)
