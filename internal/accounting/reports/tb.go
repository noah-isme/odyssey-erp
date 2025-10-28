package reports

import (
	"sort"
	"strings"
)

// AccountBalance models a general ledger account with aggregated balances.
type AccountBalance struct {
	Code    string
	Name    string
	Type    string
	Opening float64
	Debit   float64
	Credit  float64
}

// Closing computes the closing balance for the account.
func (a AccountBalance) Closing() float64 {
	return a.Opening + a.Debit - a.Credit
}

// GroupKey returns a key used for grouping trial balance rows.
func (a AccountBalance) GroupKey() string {
	if idx := strings.Index(a.Code, "."); idx > 0 {
		return a.Code[:idx]
	}
	if len(a.Code) >= 2 {
		return a.Code[:2]
	}
	return a.Code
}

// TrialBalanceAccount represents a row inside a trial balance group.
type TrialBalanceAccount struct {
	Code    string
	Name    string
	Opening float64
	Debit   float64
	Credit  float64
	Closing float64
}

// TrialBalanceGroup aggregates accounts for presentation.
type TrialBalanceGroup struct {
	Key      string
	Accounts []TrialBalanceAccount
	Opening  float64
	Debit    float64
	Credit   float64
	Closing  float64
}

// TrialBalance is the final structure rendered in UI/PDF.
type TrialBalance struct {
	Groups       []TrialBalanceGroup
	TotalDebit   float64
	TotalCredit  float64
	TotalOpening float64
	TotalClosing float64
}

// BuildTrialBalance converts account balances into grouped trial balance data.
func BuildTrialBalance(accounts []AccountBalance) TrialBalance {
	groups := make(map[string]*TrialBalanceGroup)
	keys := make([]string, 0)
	for _, acc := range accounts {
		key := acc.GroupKey()
		grp, ok := groups[key]
		if !ok {
			grp = &TrialBalanceGroup{Key: key}
			groups[key] = grp
			keys = append(keys, key)
		}
		row := TrialBalanceAccount{
			Code:    acc.Code,
			Name:    acc.Name,
			Opening: acc.Opening,
			Debit:   acc.Debit,
			Credit:  acc.Credit,
			Closing: acc.Closing(),
		}
		grp.Accounts = append(grp.Accounts, row)
		grp.Opening += row.Opening
		grp.Debit += row.Debit
		grp.Credit += row.Credit
		grp.Closing += row.Closing
	}

	sort.Strings(keys)
	result := TrialBalance{}
	for _, key := range keys {
		grp := groups[key]
		sort.Slice(grp.Accounts, func(i, j int) bool {
			return grp.Accounts[i].Code < grp.Accounts[j].Code
		})
		result.Groups = append(result.Groups, *grp)
		result.TotalOpening += grp.Opening
		result.TotalDebit += grp.Debit
		result.TotalCredit += grp.Credit
		result.TotalClosing += grp.Closing
	}
	return result
}
