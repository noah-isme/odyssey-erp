package http

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/odyssey-erp/odyssey-erp/internal/consol"
)

const (
	csvFlushEvery = 200
	csvBufferSize = 32 * 1024
)

type csvStreamer struct {
	buf          *bufio.Writer
	csv          *csv.Writer
	flushEvery   int
	pendingLines int
}

func newCSVStreamer(w io.Writer) *csvStreamer {
	buf := bufio.NewWriterSize(w, csvBufferSize)
	writer := csv.NewWriter(buf)
	writer.UseCRLF = true
	return &csvStreamer{buf: buf, csv: writer, flushEvery: csvFlushEvery}
}

func (s *csvStreamer) writeComment(line string) error {
	if s == nil || s.buf == nil {
		return fmt.Errorf("csv streamer not initialised")
	}
	if !strings.HasSuffix(line, "\r\n") {
		line = strings.TrimSuffix(line, "\n")
		line += "\r\n"
	}
	if _, err := s.buf.WriteString(line); err != nil {
		return err
	}
	return nil
}

func (s *csvStreamer) writeRow(row []string) error {
	if s == nil || s.csv == nil {
		return fmt.Errorf("csv streamer not initialised")
	}
	if err := s.csv.Write(row); err != nil {
		return err
	}
	s.pendingLines++
	if s.flushEvery > 0 && s.pendingLines >= s.flushEvery {
		return s.Flush()
	}
	return nil
}

func (s *csvStreamer) Flush() error {
	if s == nil || s.csv == nil || s.buf == nil {
		return fmt.Errorf("csv streamer not initialised")
	}
	s.csv.Flush()
	if err := s.csv.Error(); err != nil {
		return err
	}
	if err := s.buf.Flush(); err != nil {
		return err
	}
	s.pendingLines = 0
	return nil
}

func (s *csvStreamer) Close() error {
	if err := s.Flush(); err != nil {
		return err
	}
	return nil
}

func writePLCSV(w io.Writer, report consol.ProfitLossReport, warnings []string) error {
	streamer := newCSVStreamer(w)
	if err := writeMetadata(streamer, "Consolidated Profit & Loss", report.Filters.GroupID, report.Filters.Period, report.Filters.Entities, report.Filters.FxOn, warnings); err != nil {
		return err
	}
	if err := streamer.writeRow([]string{"Section", "Account Code", "Account Name", "Local Amount", "Group Amount"}); err != nil {
		return err
	}
	for _, line := range report.Lines {
		if err := streamer.writeRow([]string{
			line.Section,
			line.AccountCode,
			line.AccountName,
			formatDecimal(line.LocalAmount),
			formatDecimal(line.GroupAmount),
		}); err != nil {
			return err
		}
	}
	if err := streamer.writeRow([]string{"", "", "", "", ""}); err != nil {
		return err
	}
	totalsRows := [][]string{
		{"Totals", "", "Revenue", "", formatDecimal(report.Totals.Revenue)},
		{"Totals", "", "COGS", "", formatDecimal(report.Totals.COGS)},
		{"Totals", "", "Gross Profit", "", formatDecimal(report.Totals.GrossProfit)},
		{"Totals", "", "Opex", "", formatDecimal(report.Totals.Opex)},
		{"Totals", "", "Net Income", "", formatDecimal(report.Totals.NetIncome)},
		{"Totals", "", "Delta FX", "", formatDecimal(report.Totals.DeltaFX)},
	}
	for _, row := range totalsRows {
		if err := streamer.writeRow(row); err != nil {
			return err
		}
	}
	return streamer.Close()
}

func writeBSCsv(w io.Writer, report consol.BalanceSheetReport, warnings []string) error {
	streamer := newCSVStreamer(w)
	if err := writeMetadata(streamer, "Consolidated Balance Sheet", report.Filters.GroupID, report.Filters.Period, report.Filters.Entities, report.Filters.FxOn, warnings); err != nil {
		return err
	}
	if err := streamer.writeRow([]string{"Section", "Account Code", "Account Name", "Local Amount", "Group Amount"}); err != nil {
		return err
	}
	for _, line := range report.Assets {
		if err := streamer.writeRow([]string{
			"ASSET",
			line.AccountCode,
			line.AccountName,
			formatDecimal(line.LocalAmount),
			formatDecimal(line.GroupAmount),
		}); err != nil {
			return err
		}
	}
	if err := streamer.writeRow([]string{"", "", "", "", ""}); err != nil {
		return err
	}
	for _, line := range report.LiabilitiesEq {
		if err := streamer.writeRow([]string{
			line.Section,
			line.AccountCode,
			line.AccountName,
			formatDecimal(line.LocalAmount),
			formatDecimal(line.GroupAmount),
		}); err != nil {
			return err
		}
	}
	if err := streamer.writeRow([]string{"", "", "", "", ""}); err != nil {
		return err
	}
	totalsRows := [][]string{
		{"Totals", "", "Assets", "", formatDecimal(report.Totals.Assets)},
		{"Totals", "", "Liabilities + Equity", "", formatDecimal(report.Totals.LiabEquity)},
		{"Totals", "", "Balanced", "", strconv.FormatBool(report.Totals.Balanced)},
		{"Totals", "", "Delta FX", "", formatDecimal(report.Totals.DeltaFX)},
	}
	for _, row := range totalsRows {
		if err := streamer.writeRow(row); err != nil {
			return err
		}
	}
	return streamer.Close()
}

func writeTBCsv(w io.Writer, tb consol.TrialBalance, warnings []string) error {
	streamer := newCSVStreamer(w)
	if err := writeMetadata(streamer, "Consolidated Trial Balance", tb.Filters.GroupID, tb.Filters.Period, tb.Filters.Entities, false, warnings); err != nil {
		return err
	}
	if err := streamer.writeRow([]string{"Group Account", "Name", "Local Amount", "Group Amount"}); err != nil {
		return err
	}
	for _, line := range tb.Lines {
		if err := streamer.writeRow([]string{
			line.GroupAccountCode,
			line.GroupAccountName,
			formatDecimal(line.LocalAmount),
			formatDecimal(line.GroupAmount),
		}); err != nil {
			return err
		}
	}
	return streamer.Close()
}

func writeMetadata(streamer *csvStreamer, reportName string, groupID int64, period string, entities []int64, fxOn bool, warnings []string) error {
	if err := streamer.writeComment(fmt.Sprintf("# Report: %s", reportName)); err != nil {
		return err
	}
	fxState := "OFF"
	if fxOn {
		fxState = "ON"
	}
	entitiesLine := "All"
	if len(entities) > 0 {
		copyEntities := append([]int64(nil), entities...)
		sort.Slice(copyEntities, func(i, j int) bool { return copyEntities[i] < copyEntities[j] })
		parts := make([]string, len(copyEntities))
		for i, id := range copyEntities {
			parts[i] = strconv.FormatInt(id, 10)
		}
		entitiesLine = strings.Join(parts, ",")
	}
	if err := streamer.writeComment(fmt.Sprintf("# Group: %d | Period: %s | FX: %s | Entities: %s", groupID, period, fxState, entitiesLine)); err != nil {
		return err
	}
	if len(warnings) == 0 {
		return streamer.writeComment("# Warnings: none")
	}
	joined := make([]string, len(warnings))
	for i, w := range warnings {
		joined[i] = strings.TrimSpace(w)
	}
	return streamer.writeComment("# Warnings: " + strings.Join(joined, "; "))
}

func formatDecimal(v float64) string {
	return fmt.Sprintf("%.2f", v)
}
