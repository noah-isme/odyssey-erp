package banks

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseCSV(t *testing.T) {
	csvData := `date,description,amount,reference
2023-10-01,Deposit from Client A,150.50,REF-001
2023-10-02,Payment to Vendor B,-50.00,REF-002
`
	reader := strings.NewReader(csvData)

	lines, err := ParseCSV(reader)
	require.NoError(t, err)
	require.Len(t, lines, 2)

	// Verify first line
	require.Equal(t, 2023, lines[0].Date.Year())
	require.Equal(t, time.Month(10), lines[0].Date.Month())
	require.Equal(t, 1, lines[0].Date.Day())
	require.Equal(t, 150.50, lines[0].Amount)
	require.Equal(t, "Deposit from Client A", lines[0].Description)
	require.Equal(t, "REF-001", lines[0].Reference)

	// Verify second line
	require.Equal(t, -50.00, lines[1].Amount)
	require.Equal(t, "Payment to Vendor B", lines[1].Description)
	require.Equal(t, "REF-002", lines[1].Reference)
}
