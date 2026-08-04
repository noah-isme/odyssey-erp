package storage

import (
	"context"
	"io"
)

// Storage defines the interface for object storage operations.
// Implementations must support local filesystem (dev/test) and S3-compatible (production).
type Storage interface {
	// Put stores an object and returns an opaque storage key.
	// The key must not be derived from the original filename.
	Put(ctx context.Context, input PutInput) (string, error)

	// Get retrieves an object by storage key.
	Get(ctx context.Context, key string) (io.ReadCloser, error)

	// Stat returns object metadata without downloading content.
	Stat(ctx context.Context, key string) (ObjectInfo, error)

	// Delete removes an object. Only Document Management may call this for managed objects.
	Delete(ctx context.Context, key string) error

	// SignedURL generates a short-lived pre-signed URL for authorized download.
	// Returns empty string if not supported by the backend.
	SignedURL(ctx context.Context, key string, expirySeconds int) (string, error)
}

// PutInput contains the data and metadata for a storage put operation.
type PutInput struct {
	// Data is the object content. The implementation should stream, not buffer entirely.
	Data io.Reader

	// Size is the expected byte count. Used for validation and progress.
	Size int64

	// DeclaredContentType is the caller-provided MIME type.
	DeclaredContentType string

	// DetectedContentType is the server-detected MIME type (e.g., via libmagic).
	// If empty, the implementation may detect it.
	DetectedContentType string

	// ChecksumSHA256 is the expected SHA-256 hex digest. If provided, the implementation
	// must verify it matches the written data.
	ChecksumSHA256 string

	// Metadata is arbitrary key-value pairs stored with the object.
	Metadata map[string]string

	// CompanyID scopes the storage key for company isolation and deduplication.
	CompanyID int64

	// Classification hints at the sensitivity for storage handling (e.g., encryption).
	Classification string
}

// ObjectInfo contains metadata about a stored object.
type ObjectInfo struct {
	Key                 string
	Size                int64
	ContentType         string
	ChecksumSHA256      string
	EncryptionMetadata  map[string]string
	MalwareScanStatus   MalwareScanStatus
	CreatedAt           string // RFC3339
	Metadata            map[string]string
}

// MalwareScanStatus represents the result of malware scanning.
type MalwareScanStatus string

const (
	MalwareScanPending  MalwareScanStatus = "PENDING"
	MalwareScanClean    MalwareScanStatus = "CLEAN"
	MalwareScanInfected MalwareScanStatus = "INFECTED"
	MalwareScanError    MalwareScanStatus = "ERROR"
	MalwareScanSkipped  MalwareScanStatus = "SKIPPED"
)

// StorageConfig holds configuration for constructing a Storage implementation.
type StorageConfig struct {
	Driver          string // "local" or "s3"
	LocalDir        string
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	UsePathStyle    bool
	AutoCreate      bool
}

// NewStorage constructs the configured storage backend.
func NewStorage(ctx context.Context, cfg StorageConfig) (Storage, error) {
	switch cfg.Driver {
	case "", "local":
		return NewLocalStorage(cfg.LocalDir), nil
	case "s3":
		return NewS3Storage(ctx, cfg)
	default:
		return nil, ErrUnsupportedDriver(cfg.Driver)
	}
}

// ErrUnsupportedDriver is returned when an unknown storage driver is requested.
type ErrUnsupportedDriver string

func (e ErrUnsupportedDriver) Error() string {
	return "storage: unsupported driver " + string(e)
}

// ErrNotFound is returned when an object does not exist.
type ErrNotFound struct {
	Key string
}

func (e ErrNotFound) Error() string {
	return "storage: object not found: " + e.Key
}

// ErrChecksumMismatch is returned when the provided checksum doesn't match the data.
type ErrChecksumMismatch struct {
	Expected, Actual string
}

func (e ErrChecksumMismatch) Error() string {
	return "storage: checksum mismatch: expected " + e.Expected + ", got " + e.Actual
}

// ErrMalwareDetected is returned when malware scanning finds a threat.
type ErrMalwareDetected struct {
	Key string
}

func (e ErrMalwareDetected) Error() string {
	return "storage: malware detected in object: " + e.Key
}