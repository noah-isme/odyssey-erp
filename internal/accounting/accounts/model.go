package accounts

import "time"

// AccountType enumerates CoA categories.
type AccountType string

const (
	AccountTypeAsset     AccountType = "ASSET"
	AccountTypeLiability AccountType = "LIABILITY"
	AccountTypeEquity    AccountType = "EQUITY"
	AccountTypeRevenue   AccountType = "REVENUE"
	AccountTypeExpense   AccountType = "EXPENSE"
)

// Account models a chart of accounts node.
type Account struct {
	ID        int64
	Code      string
	Name      string
	Type      AccountType
	ParentID  *int64
	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}
