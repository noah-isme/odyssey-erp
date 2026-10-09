package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/odyssey-erp/odyssey-erp/internal/storage"
)

const (
	// DefaultDocumentStorageDir is the documents directory for staging and
	// production when DOCUMENT_STORAGE_DIR is empty. It is absolute so it
	// never resolves inside an immutable release directory.
	DefaultDocumentStorageDir = "/var/lib/odyssey/storage"
)

// devDocumentStorageDir is the relative development and test default.
var devDocumentStorageDir = filepath.Join("data", "storage")

// DocumentStorageRequired reports whether a documents storage that cannot be
// opened or written must stop the process: true for staging and production.
func DocumentStorageRequired(cfg *Config) bool {
	return cfg != nil && (cfg.AppEnv == "staging" || cfg.IsProduction())
}

// DocumentStorageDir resolves the documents directory shared by the web
// process and the worker: an explicit DOCUMENT_STORAGE_DIR wins; otherwise
// staging and production use DefaultDocumentStorageDir and every other
// APP_ENV the relative data/storage.
func DocumentStorageDir(cfg *Config) string {
	if cfg != nil {
		if dir := strings.TrimSpace(cfg.DocumentStorageDir); dir != "" {
			return dir
		}
	}
	if DocumentStorageRequired(cfg) {
		return DefaultDocumentStorageDir
	}
	return devDocumentStorageDir
}

// OpenDocumentStorage opens the documents storage both binaries use. It
// returns the resolved directory. When the directory is not writable it
// still returns the storage together with the error, so callers outside
// staging and production can log a warning and keep running; with an
// unsupported driver the storage is nil.
func OpenDocumentStorage(ctx context.Context, cfg *Config) (storage.Storage, string, error) {
	dir := DocumentStorageDir(cfg)
	driver := ""
	if cfg != nil {
		driver = strings.TrimSpace(cfg.DocumentStorageDriver)
	}
	if driver != "" && driver != "local" {
		return nil, dir, fmt.Errorf("documents storage: %w", storage.ErrUnsupportedDriver(driver))
	}
	s, err := storage.NewStorage(ctx, storage.StorageConfig{Driver: "local", LocalDir: dir})
	if err != nil {
		return nil, dir, fmt.Errorf("documents storage: %w", err)
	}
	if err := storage.EnsureWritableDir(dir); err != nil {
		return s, dir, fmt.Errorf("documents storage %s: %w", dir, err)
	}
	return s, dir, nil
}
