package banking

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// StatementEntry is a normalized statement row ready for persistence.
type StatementEntry struct {
	Date        time.Time
	Amount      float64
	Description string
	Reference   string
}

func parseStatement(filename string, content []byte) ([]StatementEntry, error) {
	ext := strings.ToLower(filename)
	if strings.HasSuffix(ext, ".ofx") || strings.HasSuffix(ext, ".qfx") {
		return parseOFX(content)
	}
	return parseCSV(content)
}

func parseCSV(content []byte) ([]StatementEntry, error) {
	reader := csv.NewReader(bytes.NewReader(content))
	records, err := reader.ReadAll()
	if err != nil || len(records) < 2 {
		return nil, fmt.Errorf("file CSV tidak memiliki transaksi")
	}
	columns := make(map[string]int)
	for index, header := range records[0] {
		columns[strings.ToLower(strings.TrimSpace(header))] = index
	}
	dateColumn, ok := findColumn(columns, "date", "tanggal", "transaction date")
	if !ok {
		return nil, fmt.Errorf("kolom tanggal tidak ditemukan")
	}
	amountColumn, ok := findColumn(columns, "amount", "jumlah", "nominal")
	if !ok {
		return nil, fmt.Errorf("kolom jumlah tidak ditemukan")
	}
	descriptionColumn, _ := findColumn(columns, "description", "deskripsi", "memo", "narration")
	referenceColumn, _ := findColumn(columns, "reference", "referensi", "fitid", "id")
	entries := make([]StatementEntry, 0, len(records)-1)
	for line, record := range records[1:] {
		if len(record) <= maxIndex(dateColumn, amountColumn, descriptionColumn, referenceColumn) {
			return nil, fmt.Errorf("baris %d tidak lengkap", line+2)
		}
		date, err := parseStatementDate(record[dateColumn])
		if err != nil {
			return nil, fmt.Errorf("baris %d: %w", line+2, err)
		}
		amount, err := parseStatementAmount(record[amountColumn])
		if err != nil || amount == 0 {
			return nil, fmt.Errorf("baris %d: jumlah tidak valid", line+2)
		}
		entry := StatementEntry{Date: date, Amount: amount}
		if descriptionColumn >= 0 {
			entry.Description = strings.TrimSpace(record[descriptionColumn])
		}
		if referenceColumn >= 0 {
			entry.Reference = strings.TrimSpace(record[referenceColumn])
		}
		if entry.Description == "" {
			entry.Description = "Imported bank statement"
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

var ofxField = regexp.MustCompile(`(?is)<([A-Z0-9]+)>([^<\r\n]+)`)

func parseOFX(content []byte) ([]StatementEntry, error) {
	parts := strings.Split(string(content), "<STMTTRN>")
	if len(parts) < 2 {
		return nil, fmt.Errorf("file OFX tidak memiliki transaksi")
	}
	entries := make([]StatementEntry, 0, len(parts)-1)
	for _, part := range parts[1:] {
		body := []byte(part)
		if end := bytes.Index(body, []byte("</STMTTRN>")); end >= 0 {
			body = body[:end]
		} else if end := bytes.Index(body, []byte("</BANKTRANLIST>")); end >= 0 {
			body = body[:end]
		}
		fields := make(map[string]string)
		for _, match := range ofxField.FindAllSubmatch(body, -1) {
			fields[strings.ToUpper(string(match[1]))] = strings.TrimSpace(string(match[2]))
		}
		date, err := parseStatementDate(fields["DTPOSTED"])
		if err != nil {
			return nil, fmt.Errorf("tanggal OFX tidak valid: %w", err)
		}
		amount, err := parseStatementAmount(fields["TRNAMT"])
		if err != nil || amount == 0 {
			return nil, fmt.Errorf("jumlah OFX tidak valid")
		}
		description := fields["NAME"]
		if description == "" {
			description = fields["MEMO"]
		}
		if description == "" {
			description = "Imported bank statement"
		}
		entries = append(entries, StatementEntry{Date: date, Amount: amount, Description: description, Reference: fields["FITID"]})
	}
	return entries, nil
}

func findColumn(columns map[string]int, names ...string) (int, bool) {
	for _, name := range names {
		if index, ok := columns[name]; ok {
			return index, true
		}
	}
	return -1, false
}

func maxIndex(indexes ...int) int {
	max := -1
	for _, index := range indexes {
		if index > max {
			max = index
		}
	}
	return max
}

func parseStatementDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if len(value) >= 8 && value[0] >= '0' && value[0] <= '9' {
		if date, err := time.Parse("20060102", value[:8]); err == nil {
			return date, nil
		}
	}
	for _, layout := range []string{"2006-01-02", "02/01/2006", "01/02/2006"} {
		if date, err := time.Parse(layout, value); err == nil {
			return date, nil
		}
	}
	return time.Time{}, fmt.Errorf("tanggal %q tidak dikenali", value)
}

func parseStatementAmount(value string) (float64, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, " ", ""))
	if strings.Count(value, ",") == 1 && strings.Count(value, ".") >= 1 {
		value = strings.ReplaceAll(value, ".", "")
		value = strings.ReplaceAll(value, ",", ".")
	} else {
		value = strings.ReplaceAll(value, ",", "")
	}
	return strconv.ParseFloat(value, 64)
}
