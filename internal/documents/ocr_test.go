package documents

import (
	"context"
	"strings"
	"testing"
)

func TestPlainTextOCRExtractorExtractsText(t *testing.T) {
	text, err := (PlainTextOCRExtractor{}).Extract(context.Background(), "text/plain; charset=utf-8", strings.NewReader("  purchase order 42  "))
	if err != nil {
		t.Fatal(err)
	}
	if text != "purchase order 42" {
		t.Fatalf("text=%q", text)
	}
}

func TestPlainTextOCRExtractorRejectsBinaryDocuments(t *testing.T) {
	_, err := (PlainTextOCRExtractor{}).Extract(context.Background(), "application/pdf", strings.NewReader("%PDF-1.7\x00binary"))
	if err == nil || !strings.Contains(err.Error(), "configured OCR extractor") {
		t.Fatalf("err=%v", err)
	}
}

func TestNewCollaborationTokenIsOpaque(t *testing.T) {
	first, err := newCollaborationToken()
	if err != nil {
		t.Fatal(err)
	}
	second, err := newCollaborationToken()
	if err != nil {
		t.Fatal(err)
	}
	if first == second || len(first) < 40 || len(second) < 40 {
		t.Fatalf("tokens are not unique opaque values: %q %q", first, second)
	}
}
