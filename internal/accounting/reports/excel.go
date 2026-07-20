package reports

import (
	"io"
	"strconv"

	"github.com/xuri/excelize/v2"
)

// WriteProfitAndLossXLSX writes a native spreadsheet suitable for finance review.
func WriteProfitAndLossXLSX(w io.Writer, report ProfitAndLoss, period string) error {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	sheet := f.GetSheetName(0)
	for cell, value := range map[string]any{"A1": "Profit and Loss", "A2": period, "A4": "Account", "B4": "Amount"} {
		if err := f.SetCellValue(sheet, cell, value); err != nil {
			return err
		}
	}
	row := 5
	for _, section := range []ProfitAndLossSection{report.Revenue, report.Expense} {
		if err := f.SetCellValue(sheet, "A"+itoa(row), section.Label); err != nil {
			return err
		}
		row++
		for _, line := range section.Accounts {
			if err := f.SetCellValue(sheet, "A"+itoa(row), line.Code+" - "+line.Name); err != nil {
				return err
			}
			if err := f.SetCellValue(sheet, "B"+itoa(row), line.Amount); err != nil {
				return err
			}
			row++
		}
		if err := f.SetCellValue(sheet, "A"+itoa(row), "Total "+section.Label); err != nil {
			return err
		}
		if err := f.SetCellValue(sheet, "B"+itoa(row), section.Total); err != nil {
			return err
		}
		row += 2
	}
	if err := f.SetCellValue(sheet, "A"+itoa(row), "Net income"); err != nil {
		return err
	}
	if err := f.SetCellValue(sheet, "B"+itoa(row), report.NetIncome); err != nil {
		return err
	}
	return formatAndWrite(f, sheet, w)
}

func WriteBudgetVsActualXLSX(w io.Writer, report BudgetVsActual, period string) error {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	sheet := f.GetSheetName(0)
	if err := f.SetCellValue(sheet, "A1", "Budget vs Actual"); err != nil {
		return err
	}
	if err := f.SetCellValue(sheet, "A2", period); err != nil {
		return err
	}
	for col, heading := range []string{"Account", "Budget", "Actual", "Variance", "Variance %"} {
		cell, _ := excelize.CoordinatesToCellName(col+1, 4)
		if err := f.SetCellValue(sheet, cell, heading); err != nil {
			return err
		}
	}
	row := 5
	for _, lines := range [][]BudgetVsActualLine{report.Revenue, report.Expense} {
		for _, line := range lines {
			for column, value := range map[string]any{"A": line.AccountCode + " - " + line.AccountName, "B": line.Budget, "C": line.Actual, "D": line.Variance, "E": line.VariancePct} {
				if err := f.SetCellValue(sheet, column+itoa(row), value); err != nil {
					return err
				}
			}
			row++
		}
	}
	return formatAndWrite(f, sheet, w)
}

func formatAndWrite(f *excelize.File, sheet string, w io.Writer) error {
	style, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Color: "FFFFFF", Bold: true}, Fill: excelize.Fill{Type: "pattern", Color: []string{"1F4E78"}, Pattern: 1}})
	if err != nil {
		return err
	}
	_ = f.SetCellStyle(sheet, "A4", "E4", style)
	_ = f.SetColWidth(sheet, "A", "A", 42)
	_ = f.SetColWidth(sheet, "B", "E", 16)
	return f.Write(w)
}

func itoa(value int) string { return strconv.Itoa(value) }
