package treasury

import "time"

// SupplierBankAccount is the storage-neutral beneficiary account model.
type SupplierBankAccount struct {
	ID                 int64      `json:"id"`
	CompanyID          int64      `json:"company_id"`
	SupplierID         int64      `json:"supplier_id"`
	BankName           string     `json:"bank_name"`
	AccountNumber      string     `json:"account_number"`
	RoutingNumber      string     `json:"routing_number"`
	Currency           string     `json:"currency"`
	EffectiveFrom      time.Time  `json:"effective_from"`
	EffectiveTo        *time.Time `json:"effective_to,omitempty"`
	VerificationStatus string     `json:"verification_status"`
	EvidenceRef        string     `json:"evidence_ref"`
	HoldPayments       bool       `json:"hold_payments"`
	CreatedBy          int64      `json:"created_by"`
	ApprovedBy         *int64     `json:"approved_by,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type PaymentPolicy struct {
	RequiresMakerChecker bool `json:"requires_maker_checker"`
}

type PaymentBatch struct {
	ID               int64      `json:"id"`
	CompanyID        int64      `json:"company_id"`
	ReferenceCode    string     `json:"reference_code"`
	Status           string     `json:"status"`
	Currency         string     `json:"currency"`
	TotalAmount      float64    `json:"total_amount"`
	RevisionNumber   int32      `json:"revision_number"`
	ProposedBy       int64      `json:"proposed_by"`
	ApprovedBy       *int64     `json:"approved_by,omitempty"`
	ApprovedAt       *time.Time `json:"approved_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	ExportedFileHash string     `json:"exported_file_hash"`
	ExportedAt       *time.Time `json:"exported_at,omitempty"`
	ExportedBy       *int64     `json:"exported_by,omitempty"`
	SettledAt        *time.Time `json:"settled_at,omitempty"`
	SettledBy        *int64     `json:"settled_by,omitempty"`
}

type PaymentBatchItem struct {
	ID            int64     `json:"id"`
	BatchID       int64     `json:"batch_id"`
	SupplierID    int64     `json:"supplier_id"`
	BankAccountID int64     `json:"bank_account_id"`
	Amount        float64   `json:"amount"`
	APInvoiceID   *int64    `json:"ap_invoice_id,omitempty"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}

type SupplierBankAccountCreate struct {
	CompanyID     int64
	SupplierID    int64
	BankName      string
	AccountNumber string
	RoutingNumber string
	Currency      string
	EffectiveFrom time.Time
	EffectiveTo   *time.Time
	EvidenceRef   string
	CreatedBy     int64
}

type SupplierBankAccountVerificationUpdate struct {
	ID                 int64
	VerificationStatus string
	HoldPayments       bool
	ApprovedBy         *int64
}

type SupplierBankAccountFilter struct {
	SupplierID int64
	CompanyID  int64
}

type PaymentBatchCreate struct {
	CompanyID     int64
	ReferenceCode string
	Currency      string
	ProposedBy    int64
}

type PaymentBatchStatusUpdate struct {
	ID         int64
	Status     string
	ApprovedBy *int64
	ApprovedAt *time.Time
}

type PaymentBatchRevisionUpdate struct {
	ID          int64
	TotalAmount float64
}

type PaymentBatchExportUpdate struct {
	ID               int64
	ExportedFileHash string
	ExportedBy       *int64
}

type PaymentBatchSettlementUpdate struct {
	ID        int64
	SettledBy *int64
}

type PaymentBatchItemCreate struct {
	BatchID       int64
	SupplierID    int64
	BankAccountID int64
	Amount        float64
	APInvoiceID   *int64
}
