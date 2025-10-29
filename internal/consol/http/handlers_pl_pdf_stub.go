//go:build !production && !pdf

package http

import (
	"log/slog"
	"net/http"
)

type stubPLPDFExporter struct{}

func newPLPDFExporter(*slog.Logger, PDFRenderClient) (plPDFExporter, error) {
	return &stubPLPDFExporter{}, nil
}

func (s *stubPLPDFExporter) Ready() bool {
	return false
}

func (s *stubPLPDFExporter) Serve(http.ResponseWriter, *http.Request, *ProfitLossHandler) {}
