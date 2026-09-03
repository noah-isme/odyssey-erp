package banking

import (
	"time"

	"github.com/google/uuid"
)

// BankAccount is the storage-neutral bank account used by banking workflows.
type BankAccount struct {
	ID             int64     `json:"id"`
	CompanyID      int64     `json:"company_id"`
	Name           string    `json:"name"`
	AccountNumber  string    `json:"account_number"`
	Currency       string    `json:"currency"`
	GLAccountID    int64     `json:"gl_account_id"`
	InitialBalance float64   `json:"initial_balance"`
	IsActive       bool      `json:"is_active"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// BankTransaction is the storage-neutral transaction record used by reports and handlers.
type BankTransaction struct {
	ID                uuid.UUID `json:"id"`
	BankAccountID     int64     `json:"bank_account_id"`
	Date              time.Time `json:"date"`
	Amount            float64   `json:"amount"`
	Description       string    `json:"description"`
	Reference         string    `json:"reference"`
	Status            string    `json:"status"`
	GLJournalID       *int64    `json:"gl_journal_id,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	ImportRunID       *int64    `json:"import_run_id,omitempty"`
	ExternalReference string    `json:"external_reference,omitempty"`
	Fingerprint       string    `json:"fingerprint,omitempty"`
	SkipReason        string    `json:"skip_reason,omitempty"`
}

type BankAccountCreate struct {
	CompanyID      int64
	Name           string
	AccountNumber  string
	Currency       string
	GLAccountID    int64
	InitialBalance float64
	IsActive       bool
}

type BankTransactionCreate struct {
	ID                uuid.UUID
	BankAccountID     int64
	Date              time.Time
	Amount            float64
	Description       string
	Reference         string
	Status            string
	GLJournalID       *int64
	ImportRunID       *int64
	ExternalReference string
	Fingerprint       string
	SkipReason        string
}

type BankTransactionStatusUpdate struct {
	ID          uuid.UUID
	Status      string
	GLJournalID *int64
}

type StatementImportRunCreate struct {
	CompanyID     int64
	BankAccountID int64
	Filename      string
	ContentHash   string
	ImportedBy    *int64
}

type StatementImportRun struct {
	ID            int64
	CompanyID     int64
	BankAccountID int64
}

type BankStatementCreate struct {
	BankAccountID   int64
	StatementDate   time.Time
	StartingBalance float64
	EndingBalance   float64
	Status          string
}

type BankStatement struct {
	ID            int64
	BankAccountID int64
	StatementDate time.Time
	Status        string
}

type BankStatementLineCreate struct {
	StatementID     int64
	Date            time.Time
	Description     string
	Amount          float64
	ReferenceNumber string
	Status          string
	MatchedDocType  string
	MatchedDocID    *int64
}
