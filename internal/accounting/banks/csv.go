package banks

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

type ParsedStatementLine struct {
	Date        time.Time
	Description string
	Amount      float64
	Reference   string
}

// ParseCSV parses a standard bank statement CSV format.
// Expected format: Date (YYYY-MM-DD), Description, Amount (positive/negative), Reference
func ParseCSV(r io.Reader) ([]ParsedStatementLine, error) {
	reader := csv.NewReader(r)
	
	// Read header
	if _, err := reader.Read(); err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("empty CSV file")
		}
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	var lines []ParsedStatementLine

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read line: %w", err)
		}

		if len(record) < 4 {
			continue // Skip malformed lines
		}

		dateStr := strings.TrimSpace(record[0])
		date, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			// Try another common format DD/MM/YYYY
			date, err = time.Parse("02/01/2006", dateStr)
			if err != nil {
				return nil, fmt.Errorf("invalid date format %q: %w", dateStr, err)
			}
		}

		desc := strings.TrimSpace(record[1])
		amountStr := strings.TrimSpace(record[2])
		amountStr = strings.ReplaceAll(amountStr, ",", "") // Remove commas
		
		amount, err := strconv.ParseFloat(amountStr, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid amount %q: %w", amountStr, err)
		}

		ref := strings.TrimSpace(record[3])

		lines = append(lines, ParsedStatementLine{
			Date:        date,
			Description: desc,
			Amount:      amount,
			Reference:   ref,
		})
	}

	return lines, nil
}
