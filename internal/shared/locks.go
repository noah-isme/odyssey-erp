package shared

import "fmt"

// FinanceLockKey builds redis keys for finance critical sections.
func FinanceLockKey(periodID int64) string {
	return fmt.Sprintf("finance:period:%d:lock", periodID)
}
