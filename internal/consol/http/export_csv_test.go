package http

import (
	"bytes"
	"strings"
	"testing"

	"github.com/odyssey-erp/odyssey-erp/internal/consol"
)

func TestCSVStreamerFlushInterval(t *testing.T) {
	var buf bytes.Buffer
	streamer := newCSVStreamer(&buf)
	for i := 0; i < csvFlushEvery; i++ {
		if err := streamer.writeRow([]string{"row"}); err != nil {
			t.Fatalf("write row: %v", err)
		}
	}
	if streamer.pendingLines != 0 {
		t.Fatalf("expected pending lines reset to 0, got %d", streamer.pendingLines)
	}
	if err := streamer.writeRow([]string{"next"}); err != nil {
		t.Fatalf("write row: %v", err)
	}
	if streamer.pendingLines != 1 {
		t.Fatalf("expected pending lines 1, got %d", streamer.pendingLines)
	}
	if err := streamer.Close(); err != nil {
		t.Fatalf("close streamer: %v", err)
	}
}

func TestWritePLCSVIncludesMetadataAndTotals(t *testing.T) {
	report := consol.ProfitLossReport{
		Filters: consol.ProfitLossFilters{
			GroupID:  42,
			Period:   "2024-01",
			Entities: []int64{5, 2},
			FxOn:     true,
		},
		Lines: []consol.ProfitLossLine{
			{Section: "Revenue", AccountCode: "4000", AccountName: "Sales", LocalAmount: 1500, GroupAmount: 1400},
		},
		Totals: consol.ProfitLossTotals{
			Revenue:     1500,
			COGS:        500,
			GrossProfit: 1000,
			Opex:        200,
			NetIncome:   800,
			DeltaFX:     10,
		},
	}
	var buf bytes.Buffer
	warnings := []string{"FX rate missing for IDR/USD"}
	if err := writePLCSV(&buf, report, warnings); err != nil {
		t.Fatalf("writePLCSV: %v", err)
	}
	content := buf.String()
	if !strings.Contains(content, "\r\n") {
		t.Fatalf("expected CRLF line endings")
	}
	lines := strings.Split(strings.TrimSuffix(content, "\r\n"), "\r\n")
	if len(lines) < 8 {
		t.Fatalf("expected at least 8 lines, got %d", len(lines))
	}
	if want := "# Report: Consolidated Profit & Loss"; lines[0] != want {
		t.Fatalf("unexpected metadata line 1: %q", lines[0])
	}
	if want := "# Group: 42 | Period: 2024-01 | FX: ON | Entities: 2,5"; lines[1] != want {
		t.Fatalf("unexpected metadata line 2: %q", lines[1])
	}
	if want := "# Warnings: FX rate missing for IDR/USD"; lines[2] != want {
		t.Fatalf("unexpected metadata line 3: %q", lines[2])
	}
	if want := "Section,Account Code,Account Name,Local Amount,Group Amount"; lines[3] != want {
		t.Fatalf("unexpected header: %q", lines[3])
	}
	if want := "Totals,,Delta FX,,10.00"; !strings.Contains(content, want) {
		t.Fatalf("expected totals row containing %q", want)
	}
}

func TestWriteBSCsvAddsBalanceWarning(t *testing.T) {
	report := consol.BalanceSheetReport{
		Filters:       consol.BalanceSheetFilters{GroupID: 7, Period: "2024-06"},
		Assets:        []consol.BalanceSheetLine{{AccountCode: "1000", AccountName: "Cash", LocalAmount: 500, GroupAmount: 500}},
		LiabilitiesEq: []consol.BalanceSheetLine{{Section: "LIAB", AccountCode: "2000", AccountName: "Payable", LocalAmount: 400, GroupAmount: 400}},
		Totals:        consol.BalanceSheetTotals{Assets: 500, LiabEquity: 400, Balanced: false, DeltaFX: -5},
	}
	var buf bytes.Buffer
	warnings := []string{"Consolidated BS not balanced"}
	if err := writeBSCsv(&buf, report, warnings); err != nil {
		t.Fatalf("writeBSCsv: %v", err)
	}
	content := buf.String()
	if !strings.Contains(content, "# Warnings: Consolidated BS not balanced") {
		t.Fatalf("expected balance warning in metadata, got %q", content)
	}
}
