package boardpack

import (
	"context"
	"io"

	"github.com/odyssey-erp/odyssey-erp/internal/storage"
)

// StorageAdapter adapts the shared storage.Storage interface to the boardpack.Storage interface.
type StorageAdapter struct {
	storage storage.Storage
	companyID int64
}

// NewStorageAdapter creates a new adapter for boardpack using shared storage.
func NewStorageAdapter(storage storage.Storage, companyID int64) *StorageAdapter {
	return &StorageAdapter{storage: storage, companyID: companyID}
}

// Save stores a board pack PDF using the shared storage.
func (a *StorageAdapter) Save(ctx context.Context, id int64, pdf []byte) (string, error) {
	key, err := a.storage.Put(ctx, storage.PutInput{
		Data:                newBytesReader(pdf),
		Size:                int64(len(pdf)),
		CompanyID:           a.companyID,
		DeclaredContentType: "application/pdf",
		Classification:      "INTERNAL",
		Metadata: map[string]string{
			"board-pack-id": string(rune(id)),
			"module":        "boardpack",
		},
	})
	return key, err
}

// Open retrieves a board pack PDF using the shared storage.
func (a *StorageAdapter) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	return a.storage.Get(ctx, key)
}

// bytesReader is a simple io.Reader implementation for []byte.
type bytesReader struct {
	data []byte
	pos  int
}

func newBytesReader(data []byte) *bytesReader {
	return &bytesReader{data: data}
}

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}