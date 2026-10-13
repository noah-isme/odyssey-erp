package shared

import (
	"bytes"
	"encoding/csv"
	"testing"
)

func TestCSVSafeCell(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"formula", "=SUM(A1)", "'=SUM(A1)"},
		{"plus", "+1", "'+1"},
		{"minus", "-1", "'-1"},
		{"at", "@cmd", "'@cmd"},
		{"tab prefixed", "\t=1+1", "'\t=1+1"},
		{"carriage return prefixed", "\r=1+1", "'\r=1+1"},
		{"leading spaces before formula", "  =x", "'  =x"},
		{"hyperlink", `=HYPERLINK("http://x","y")`, `'=HYPERLINK("http://x","y")`},
		{"plain text unchanged", "Human Resources", "Human Resources"},
		{"plain number unchanged", "1250.50", "1250.50"},
		{"inner operator unchanged", "R&D = Engineering", "R&D = Engineering"},
		{"leading spaces plain unchanged", "  text", "  text"},
		{"empty unchanged", "", ""},
		{"spaces only unchanged", "   ", "   "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CSVSafeCell(tc.in); got != tc.want {
				t.Fatalf("CSVSafeCell(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestWriteCSVRowEscapesEveryCell(t *testing.T) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := WriteCSVRow(w, "plain", "=1+1", "@x", "-5"); err != nil {
		t.Fatal(err)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), "plain,'=1+1,'@x,'-5\n"; got != want {
		t.Fatalf("row = %q, want %q", got, want)
	}
}
