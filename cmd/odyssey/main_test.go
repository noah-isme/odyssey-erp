package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/odyssey-erp/odyssey-erp/internal/ap"
	"github.com/odyssey-erp/odyssey-erp/internal/app"
	"github.com/odyssey-erp/odyssey-erp/internal/ar"
	"github.com/odyssey-erp/odyssey-erp/internal/cmms"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/storage"
)

func TestWireAuditLoggers(t *testing.T) {
	arService := ar.NewService(ar.NewRepository(nil))
	apService := ap.NewService(ap.NewRepository(nil), nil)
	cmmsService := cmms.NewService(cmms.NewRepository(nil))

	wireAuditLoggers(shared.NewAuditLogger(nil), arService, apService, cmmsService)

	tests := []struct {
		name    string
		service interface{ AuditLoggerConfigured() bool }
	}{
		{name: "ar", service: arService},
		{name: "ap", service: apService},
		{name: "cmms", service: cmmsService},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.service.AuditLoggerConfigured() {
				t.Fatalf("%s service has no audit logger after wiring", tc.name)
			}
		})
	}
}

// TestDocumentStorageUsesSharedResolver pins the documents storage wiring of
// this binary to app.OpenDocumentStorage, the function the other binary
// (cmd/odyssey and cmd/worker) uses too, so web-written blobs are readable by
// the worker's OCR and disposition jobs. It also checks the behaviour: with
// DOCUMENT_STORAGE_DIR set, a blob lands in that directory and not under the
// working directory.
func TestDocumentStorageUsesSharedResolver(t *testing.T) {
	if reflect.ValueOf(openDocumentStorage).Pointer() != reflect.ValueOf(app.OpenDocumentStorage).Pointer() {
		t.Errorf("openDocumentStorage is not app.OpenDocumentStorage")
	}

	workDir := t.TempDir()
	t.Chdir(workDir)
	sharedDir := filepath.Join(t.TempDir(), "shared-storage")
	t.Setenv("APP_ENV", "staging")
	t.Setenv("RELEASE_PROFILE", string(app.ReleaseProfileV010Core))
	t.Setenv("SESSION_SECRET", "test-session-secret")
	t.Setenv("CSRF_SECRET", "test-csrf-secret")
	t.Setenv("DOCUMENT_STORAGE_DRIVER", "local")
	t.Setenv("DOCUMENT_STORAGE_DIR", sharedDir)
	cfg, err := app.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	ctx := context.Background()
	store, dir, err := openDocumentStorage(ctx, cfg)
	if err != nil {
		t.Fatalf("openDocumentStorage() error = %v", err)
	}
	if dir != sharedDir {
		t.Errorf("openDocumentStorage() dir = %q, want DOCUMENT_STORAGE_DIR %q", dir, sharedDir)
	}
	key, err := store.Put(ctx, storage.PutInput{CompanyID: 1, Data: strings.NewReader("blob"), Size: 4})
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(sharedDir, filepath.FromSlash(key))); err != nil {
		t.Fatalf("blob %q is not under DOCUMENT_STORAGE_DIR %q: %v", key, sharedDir, err)
	}
	entries, err := os.ReadDir(workDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("documents storage wrote %d entries under the working directory", len(entries))
	}
}
