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
	f.SetCellValue(sheet, "A1", "Profit and Loss")
	f.SetCellValue(sheet, "A2", period)
	f.SetCellValue(sheet, "A4", "Account")
	f.SetCellValue(sheet, "B4", "Amount")
	row := 5
	for _, section := range []ProfitAndLossSection{report.Revenue, report.Expense} {
		f.SetCellValue(sheet, "A"+itoa(row), section.Label)
		row++
		for _, line := range section.Accounts {
			f.SetCellValue(sheet, "A"+itoa(row), line.Code+" - "+line.Name)
			f.SetCellValue(sheet, "B"+itoa(row), line.Amount)
			row++
		}
		f.SetCellValue(sheet, "A"+itoa(row), "Total "+section.Label)
		f.SetCellValue(sheet, "B"+itoa(row), section.Total)
		row += 2
	}
	f.SetCellValue(sheet, "A"+itoa(row), "Net income")
	f.SetCellValue(sheet, "B"+itoa(row), report.NetIncome)
	return formatAndWrite(f, sheet, w)
}

func WriteBudgetVsActualXLSX(w io.Writer, report BudgetVsActual, period string) error {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	sheet := f.GetSheetName(0)
	f.SetCellValue(sheet, "A1", "Budget vs Actual")
	f.SetCellValue(sheet, "A2", period)
	for col, heading := range []string{"Account", "Budget", "Actual", "Variance", "Variance %"} {
		cell, _ := excelize.CoordinatesToCellName(col+1, 4)
		f.SetCellValue(sheet, cell, heading)
	}
	row := 5
	for _, lines := range [][]BudgetVsActualLine{report.Revenue, report.Expense} {
		for _, line := range lines {
			f.SetCellValue(sheet, "A"+itoa(row), line.AccountCode+" - "+line.AccountName)
			f.SetCellValue(sheet, "B"+itoa(row), line.Budget)
			f.SetCellValue(sheet, "C"+itoa(row), line.Actual)
			f.SetCellValue(sheet, "D"+itoa(row), line.Variance)
			f.SetCellValue(sheet, "E"+itoa(row), line.VariancePct)
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
