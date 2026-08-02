package shared

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSourcingAndLogisticsScopesAreUniqueAndComplete(t *testing.T) {
	scopes := SourcingAndLogisticsScopes()
	require.Len(t, scopes, 16)
	seen := map[string]struct{}{}
	for _, scope := range scopes {
		_, exists := seen[scope]
		require.False(t, exists, "duplicate permission %q", scope)
		seen[scope] = struct{}{}
	}
	require.Contains(t, scopes, PermProcurementRFQAward)
	require.Contains(t, scopes, PermLogisticsFreightManage)
}
