package mappings

import "time"

// AccountMapping links integration keys to ledger accounts.
type AccountMapping struct {
	Module    string
	Key       string
	AccountID int64
	CreatedAt time.Time
	UpdatedAt time.Time
}
