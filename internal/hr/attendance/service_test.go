package attendance

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestParseCSVValidatesRows(t *testing.T) {
	rows, problems, err := ParseCSV(strings.NewReader("employee_number,date,check_in,check_out,status\nE-1,2026-07-30,2026-07-30 08:00,2026-07-30 17:00,PRESENT\nE-2,bad,,,PRESENT\n"))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Len(t, problems, 1)
	require.Equal(t, "E-1", rows[0].EmployeeNumber)
}
