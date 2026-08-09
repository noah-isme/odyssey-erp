package documents

import (
	"context"
	"strings"
	"testing"
)

func TestServiceRejectsInvalidRequestsWithoutRepository(t *testing.T) {
	svc := NewService(nil, nil)
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{"document", func() error { _, err := svc.Create(ctx, CreateDocumentRequest{}); return err }},
		{"update", func() error { _, err := svc.Update(ctx, 1, UpdateDocumentRequest{}); return err }},
		{"version", func() error { _, err := svc.CreateVersion(ctx, CreateVersionRequest{}); return err }},
		{"disposition", func() error { _, err := svc.CreateDisposition(ctx, CreateDispositionRequest{}); return err }},
		{"OCR", func() error { return svc.ProcessOCR(ctx, 0) }},
		{"collaboration session", func() error { _, err := svc.CreateCollaborationSession(ctx, CollaborationSession{}); return err }},
		{"collaboration change", func() error { _, err := svc.RecordCollaborationChange(ctx, CollaborationChange{}); return err }},
		{"content search", func() error { _, err := svc.SearchContent(ctx, 0, ""); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.HasPrefix(err.Error(), "documents:") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestNormaliseStatus(t *testing.T) {
	if got := NormaliseStatus(" published "); got != StatusPublished {
		t.Fatalf("status=%q want %q", got, StatusPublished)
	}
	if got := NormaliseStatus("unknown"); got != StatusDraft {
		t.Fatalf("invalid status=%q want %q", got, StatusDraft)
	}
}
