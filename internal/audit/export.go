package audit

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// ErrPDFUnavailable is returned when PDF export is not configured.
var ErrPDFUnavailable = errors.New("audit export: pdf unavailable")

// Exporter mengelola ekspor CSV/PDF untuk timeline audit.
type Exporter struct {
	templates *view.Engine
}

// NewExporter membangun exporter dengan template engine opsional.
func NewExporter(templates *view.Engine) *Exporter {
	return &Exporter{templates: templates}
}

// WriteCSV menuliskan data timeline ke CSV.
func (e *Exporter) WriteCSV(rows []TimelineRow) ([]byte, error) {
	buf := &bytes.Buffer{}
	writer := csv.NewWriter(buf)
	header := []string{"Timestamp", "Actor", "Action", "Entity", "Entity ID", "Period", "Journal No"}
	if err := writer.Write(header); err != nil {
		return nil, err
	}
	for _, row := range rows {
		record := []string{
			row.At.Format(time.RFC3339),
			row.Actor,
			row.Action,
			row.Entity,
			row.EntityID,
			row.Period,
			row.JournalNo,
		}
		if err := writer.Write(record); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// RenderPDF saat ini belum tersedia dan mengembalikan ErrPDFUnavailable.
func (e *Exporter) RenderPDF(ctx context.Context, vm ViewModel) ([]byte, error) {
	return nil, ErrPDFUnavailable
}
