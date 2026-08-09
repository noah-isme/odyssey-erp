package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LocalStorage implements Storage using the local filesystem.
// Suitable for development and testing only.
type LocalStorage struct {
	rootDir string
}

// NewLocalStorage creates a new local storage backend.
func NewLocalStorage(rootDir string) *LocalStorage {
	dir := strings.TrimSpace(rootDir)
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "odyssey-storage")
	}
	return &LocalStorage{rootDir: dir}
}

// companyPath returns the company-scoped directory path.
func (s *LocalStorage) companyPath(companyID int64) string {
	return filepath.Join(s.rootDir, fmt.Sprintf("company-%d", companyID))
}

// objectPath returns the full filesystem path for a storage key.
func (s *LocalStorage) objectPath(companyID int64, key string) string {
	return filepath.Join(s.companyPath(companyID), key)
}

func removeTempFile(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *LocalStorage) Put(ctx context.Context, input PutInput) (string, error) {
	if input.Data == nil {
		return "", errors.New("storage: nil data reader")
	}
	if input.CompanyID <= 0 {
		return "", errors.New("storage: company_id required")
	}

	// Generate opaque storage key
	key := generateStorageKey(input.DeclaredContentType)

	// Ensure key parent directory exists
	keyDir := filepath.Dir(s.objectPath(input.CompanyID, key))
	if err := os.MkdirAll(keyDir, 0o750); err != nil {
		return "", fmt.Errorf("storage: create key dir: %w", err)
	}

	// Write to temporary file first (atomic rename)
	tmpPath := s.objectPath(input.CompanyID, key+".tmp")
	finalPath := s.objectPath(input.CompanyID, key)

	file, err := os.Create(tmpPath)
	if err != nil {
		return "", fmt.Errorf("storage: create temp file: %w", err)
	}

	// Stream with checksum verification
	hasher := sha256.New()
	multiWriter := io.MultiWriter(file, hasher)

	written, err := io.Copy(multiWriter, input.Data)
	if err != nil {
		_ = file.Close()
		if cleanupErr := removeTempFile(tmpPath); cleanupErr != nil {
			return "", fmt.Errorf("storage: write data: %w (cleanup: %v)", err, cleanupErr)
		}
		return "", fmt.Errorf("storage: write data: %w", err)
	}
	if err := file.Close(); err != nil {
		if cleanupErr := removeTempFile(tmpPath); cleanupErr != nil {
			return "", fmt.Errorf("storage: close temp file: %w (cleanup: %v)", err, cleanupErr)
		}
		return "", fmt.Errorf("storage: close temp file: %w", err)
	}

	// Verify size
	if input.Size > 0 && written != input.Size {
		if cleanupErr := removeTempFile(tmpPath); cleanupErr != nil {
			return "", fmt.Errorf("storage: size mismatch: expected %d, got %d (cleanup: %v)", input.Size, written, cleanupErr)
		}
		return "", fmt.Errorf("storage: size mismatch: expected %d, got %d", input.Size, written)
	}

	// Verify checksum if provided
	if input.ChecksumSHA256 != "" {
		actual := hex.EncodeToString(hasher.Sum(nil))
		if !strings.EqualFold(actual, input.ChecksumSHA256) {
			if cleanupErr := removeTempFile(tmpPath); cleanupErr != nil {
				return "", fmt.Errorf("storage: checksum mismatch: %w (cleanup: %v)", ErrChecksumMismatch{Expected: input.ChecksumSHA256, Actual: actual}, cleanupErr)
			}
			return "", ErrChecksumMismatch{Expected: input.ChecksumSHA256, Actual: actual}
		}
	}

	// Atomically move to final location
	if err := os.Rename(tmpPath, finalPath); err != nil {
		if cleanupErr := removeTempFile(tmpPath); cleanupErr != nil {
			return "", fmt.Errorf("storage: finalize: %w (cleanup: %v)", err, cleanupErr)
		}
		return "", fmt.Errorf("storage: finalize: %w", err)
	}

	// Return key with company prefix
	return fmt.Sprintf("company-%d/%s", input.CompanyID, key), nil
}

func (s *LocalStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	// Key format: company-{id}/{opaque-key}
	parts := strings.SplitN(key, "/", 2)
	if len(parts) != 2 {
		return nil, ErrNotFound{Key: key}
	}

	var companyID int64
	_, err := fmt.Sscanf(parts[0], "company-%d", &companyID)
	if err != nil || companyID <= 0 {
		return nil, ErrNotFound{Key: key}
	}

	path := s.objectPath(companyID, parts[1])
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound{Key: key}
		}
		return nil, fmt.Errorf("storage: open: %w", err)
	}
	return file, nil
}

func (s *LocalStorage) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	parts := strings.SplitN(key, "/", 2)
	if len(parts) != 2 {
		return ObjectInfo{}, ErrNotFound{Key: key}
	}

	var companyID int64
	_, err := fmt.Sscanf(parts[0], "company-%d", &companyID)
	if err != nil || companyID <= 0 {
		return ObjectInfo{}, ErrNotFound{Key: key}
	}

	path := s.objectPath(companyID, parts[1])
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ObjectInfo{}, ErrNotFound{Key: key}
		}
		return ObjectInfo{}, fmt.Errorf("storage: stat: %w", err)
	}

	// Compute checksum for stat
	file, err := os.Open(path)
	if err != nil {
		return ObjectInfo{}, fmt.Errorf("storage: open for checksum: %w", err)
	}
	defer func() { _ = file.Close() }()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return ObjectInfo{}, fmt.Errorf("storage: checksum: %w", err)
	}

	return ObjectInfo{
		Key:               key,
		Size:              info.Size(),
		ContentType:       "", // Would need magic detection
		ChecksumSHA256:    hex.EncodeToString(hasher.Sum(nil)),
		CreatedAt:         info.ModTime().Format(time.RFC3339),
		MalwareScanStatus: MalwareScanSkipped,
	}, nil
}

func (s *LocalStorage) Delete(ctx context.Context, key string) error {
	parts := strings.SplitN(key, "/", 2)
	if len(parts) != 2 {
		return ErrNotFound{Key: key}
	}

	var companyID int64
	_, err := fmt.Sscanf(parts[0], "company-%d", &companyID)
	if err != nil || companyID <= 0 {
		return ErrNotFound{Key: key}
	}

	path := s.objectPath(companyID, parts[1])
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound{Key: key}
		}
		return fmt.Errorf("storage: delete: %w", err)
	}
	return nil
}

func (s *LocalStorage) SignedURL(ctx context.Context, key string, expirySeconds int) (string, error) {
	// Local storage doesn't support signed URLs
	return "", nil
}

// generateStorageKey creates an opaque storage key.
// Format: {type-prefix}/{uuid}.{ext-hint}
func generateStorageKey(contentType string) string {
	// In production, use UUID v7 or similar for time-ordered keys
	// For now, use timestamp + random
	return fmt.Sprintf("obj/%d-%s", time.Now().UnixNano(), randomString(16))
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
	}
	return string(b)
}
