package attendance

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseCSVValidatesRows(t *testing.T) {
	rows, problems, err := ParseCSV(strings.NewReader("employee_number,date,check_in,check_out,status\nE-1,2026-07-30,2026-07-30 08:00,2026-07-30 17:00,PRESENT\nE-2,bad,,,PRESENT\n"))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Len(t, problems, 1)
	require.Equal(t, "E-1", rows[0].EmployeeNumber)
}

func TestParseCSVRejectsDuplicatesAndInvalidStatusOrTimeRange(t *testing.T) {
	input := "employee_number,date,check_in,check_out,status\n" +
		"E-1,2026-07-30,2026-07-30 08:00,2026-07-30 17:00,PRESENT\n" +
		"E-1,2026-07-30,2026-07-30 09:00,2026-07-30 18:00,PRESENT\n" +
		"E-2,2026-07-31,2026-07-31 17:00,2026-07-31 08:00,PRESENT\n" +
		"E-3,2026-07-31,,,HOLIDAY\n"

	rows, problems, err := ParseCSV(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Len(t, problems, 3)
	require.Contains(t, problems[0], "duplicate")
}

func TestParseCSVEnforcesDateFormatAndRequiredColumns(t *testing.T) {
	_, _, err := ParseCSV(strings.NewReader("employee_number,check_in\nE-1,2026-07-30 08:00\n"))
	require.ErrorContains(t, err, "missing date column")

	rows, problems, err := ParseCSV(strings.NewReader("employee_number,date\nE-1,2026-02-30\nE-2,2026-12-31\n"))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Len(t, problems, 1)
	require.Equal(t, "E-2", rows[0].EmployeeNumber)
}
