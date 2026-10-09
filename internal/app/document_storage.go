package app

import (
	"context"

	"github.com/odyssey-erp/odyssey-erp/internal/storage"
)

// DocumentStorageDir is a test stub that keeps the v0.10.0-rc.10 behaviour.
func DocumentStorageDir(cfg *Config) string {
	return "./data/storage"
}

// OpenDocumentStorage is a test stub that keeps the v0.10.0-rc.10 behaviour.
func OpenDocumentStorage(ctx context.Context, cfg *Config) (storage.Storage, string, error) {
	dir := DocumentStorageDir(cfg)
	s, err := storage.NewStorage(ctx, storage.StorageConfig{Driver: "local", LocalDir: dir})
	return s, dir, err
}
