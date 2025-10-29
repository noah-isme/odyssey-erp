//go:build !production && !pdf

package http

import (
	"log/slog"
	"net/http"
)

type stubPDFExporter struct{}

func newPDFExporter(*slog.Logger, PDFRenderClient) (pdfExporter, error) {
	return &stubPDFExporter{}, nil
}

func (s *stubPDFExporter) Ready() bool {
	return false
}

func (s *stubPDFExporter) Serve(http.ResponseWriter, *http.Request, *Handler) {}
