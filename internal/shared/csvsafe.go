package shared

import (
	"encoding/csv"
	"strings"
)

// CSVSafeCell neutralises spreadsheet formula injection (OWASP "CSV
// Injection"). When the cell, after trimming leading spaces, starts with one
// of = + - @ TAB or CR, the original value is prefixed with a single quote so
// spreadsheet applications treat it as text. Other values are returned
// unchanged. Server-formatted negative numbers are prefixed too; consumers
// that need the numeric value strip the leading quote.
func CSVSafeCell(s string) string {
	trimmed := strings.TrimLeft(s, " ")
	if trimmed == "" {
		return s
	}
	switch trimmed[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}

// WriteCSVRow writes one record with every cell passed through CSVSafeCell.
func WriteCSVRow(w *csv.Writer, cells ...string) error {
	safe := make([]string, len(cells))
	for i, c := range cells {
		safe[i] = CSVSafeCell(c)
	}
	return w.Write(safe)
}
