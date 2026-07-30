package payroll

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegularRunUUIDIsDeterministicByCompanyAndPeriod(t *testing.T) {
	require.Equal(t, regularRunUUID(1, 2), regularRunUUID(1, 2))
	require.NotEqual(t, regularRunUUID(1, 2), regularRunUUID(1, 3))
	require.NotEqual(t, regularRunUUID(1, 2), regularRunUUID(2, 2))
}
