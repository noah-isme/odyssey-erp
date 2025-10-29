//go:build !production && !pdf

package http

import (
	"log/slog"
	"net/http"
)

type stubBSPDFExporter struct{}

func newBSPDFExporter(*slog.Logger, PDFRenderClient) (bsPDFExporter, error) {
	return &stubBSPDFExporter{}, nil
}

func (s *stubBSPDFExporter) Ready() bool {
	return false
}

func (s *stubBSPDFExporter) Serve(http.ResponseWriter, *http.Request, *BalanceSheetHandler) {}
