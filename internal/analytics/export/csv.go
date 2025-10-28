package export

import (
	"encoding/csv"
	"io"

	"github.com/odyssey-erp/odyssey-erp/internal/analytics"
)

// WriteKPICSV serialises KPI summary metrics to a CSV representation.
func WriteKPICSV(w io.Writer, summary analytics.KPISummary, period string) error {
	writer := csv.NewWriter(w)
	defer writer.Flush()

	if err := writer.Write([]string{"Metric", "Value"}); err != nil {
		return err
	}
	records := [][]string{
		{"Period", period},
		{"Net Profit", formatFloat(summary.NetProfit)},
		{"Revenue", formatFloat(summary.Revenue)},
		{"Operating Expense", formatFloat(summary.Opex)},
		{"Cost of Goods Sold", formatFloat(summary.COGS)},
		{"Cash In", formatFloat(summary.CashIn)},
		{"Cash Out", formatFloat(summary.CashOut)},
		{"AR Outstanding", formatFloat(summary.AROutstanding)},
		{"AP Outstanding", formatFloat(summary.APOutstanding)},
	}
	for _, record := range records {
		if err := writer.Write(record); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

// WritePLTrendCSV emits the monthly P&L movement as CSV.
func WritePLTrendCSV(w io.Writer, points []analytics.PLTrendPoint) error {
	writer := csv.NewWriter(w)
	defer writer.Flush()
	if err := writer.Write([]string{"Period", "Revenue", "COGS", "Opex", "Net"}); err != nil {
		return err
	}
	for _, point := range points {
		if err := writer.Write([]string{
			point.Period,
			formatFloat(point.Revenue),
			formatFloat(point.COGS),
			formatFloat(point.Opex),
			formatFloat(point.Net),
		}); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

// WriteCashflowTrendCSV emits monthly cash movement as CSV.
func WriteCashflowTrendCSV(w io.Writer, points []analytics.CashflowTrendPoint) error {
	writer := csv.NewWriter(w)
	defer writer.Flush()
	if err := writer.Write([]string{"Period", "Cash In", "Cash Out"}); err != nil {
		return err
	}
	for _, point := range points {
		if err := writer.Write([]string{
			point.Period,
			formatFloat(point.In),
			formatFloat(point.Out),
		}); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

// WriteAgingCSV prints aging buckets to CSV.
func WriteAgingCSV(w io.Writer, buckets []analytics.AgingBucket) error {
	writer := csv.NewWriter(w)
	defer writer.Flush()
	if err := writer.Write([]string{"Bucket", "Amount"}); err != nil {
		return err
	}
	for _, bucket := range buckets {
		if err := writer.Write([]string{bucket.Bucket, formatFloat(bucket.Amount)}); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}
