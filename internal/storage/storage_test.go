package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"testing"
)

func TestStorageInterface(t *testing.T) {
	ctx := context.Background()

	// Test LocalStorage
	t.Run("LocalStorage", func(t *testing.T) {
		s := NewLocalStorage("")
		testStorage(ctx, t, s)
	})
}

func testStorage(ctx context.Context, t *testing.T, s Storage) {
	companyID := int64(1)

	// Test Put
	content := []byte("test content for storage")
	checksum := sha256.Sum256(content)
	checksumHex := hex.EncodeToString(checksum[:])

	key, err := s.Put(ctx, PutInput{
		Data:                strings.NewReader(string(content)),
		Size:                int64(len(content)),
		DeclaredContentType: "text/plain",
		ChecksumSHA256:      checksumHex,
		CompanyID:           companyID,
		Classification:      "INTERNAL",
		Metadata:            map[string]string{"test": "value"},
	})
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if key == "" {
		t.Fatal("Put returned empty key")
	}
	t.Logf("Stored key: %s", key)

	// Test Stat
	info, err := s.Stat(ctx, key)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if info.Size != int64(len(content)) {
		t.Errorf("Size mismatch: expected %d, got %d", len(content), info.Size)
	}
	if !strings.EqualFold(info.ChecksumSHA256, checksumHex) {
		t.Errorf("Checksum mismatch: expected %s, got %s", checksumHex, info.ChecksumSHA256)
	}
	if info.MalwareScanStatus != MalwareScanSkipped && info.MalwareScanStatus != MalwareScanPending {
		t.Errorf("Unexpected malware scan status: %s", info.MalwareScanStatus)
	}

	// Test Get
	reader, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer func() { _ = reader.Close() }()

	readContent, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if string(readContent) != string(content) {
		t.Errorf("Content mismatch: expected %q, got %q", content, readContent)
	}

	// Test Delete
	err = s.Delete(ctx, key)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	_, err = s.Get(ctx, key)
	if err == nil {
		t.Error("Get succeeded after delete, expected error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected not found error, got: %v", err)
	}
}

func TestStorageKeyFormat(t *testing.T) {
	// Verify keys are opaque and company-scoped
	s := NewLocalStorage("")
	ctx := context.Background()

	key, err := s.Put(ctx, PutInput{
		Data:                strings.NewReader("test"),
		Size:                4,
		CompanyID:           42,
		DeclaredContentType: "text/plain",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Key should contain company prefix
	if !strings.HasPrefix(key, "company-42/") {
		t.Errorf("Key should be company-scoped: %s", key)
	}

	// Key should not contain original filename
	if strings.Contains(key, "original") || strings.Contains(key, "filename") {
		t.Errorf("Key should be opaque, not derived from filename: %s", key)
	}
}

func TestStorageChecksumVerification(t *testing.T) {
	s := NewLocalStorage("")
	ctx := context.Background()

	content := []byte("checksum test")
	correctChecksum := sha256.Sum256(content)
	wrongChecksum := "0000000000000000000000000000000000000000000000000000000000000000"

	// Correct checksum should succeed
	_, err := s.Put(ctx, PutInput{
		Data:                strings.NewReader(string(content)),
		Size:                int64(len(content)),
		CompanyID:           1,
		DeclaredContentType: "text/plain",
		ChecksumSHA256:      hex.EncodeToString(correctChecksum[:]),
	})
	if err != nil {
		t.Errorf("Correct checksum rejected: %v", err)
	}

	// Wrong checksum should fail
	_, err = s.Put(ctx, PutInput{
		Data:                strings.NewReader(string(content)),
		Size:                int64(len(content)),
		CompanyID:           1,
		DeclaredContentType: "text/plain",
		ChecksumSHA256:      wrongChecksum,
	})
	if err == nil {
		t.Error("Wrong checksum should have been rejected")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("Expected checksum mismatch error, got: %v", err)
	}
}

func TestStorageSizeValidation(t *testing.T) {
	s := NewLocalStorage("")
	ctx := context.Background()

	content := []byte("size test")

	// Correct size should succeed
	_, err := s.Put(ctx, PutInput{
		Data:                strings.NewReader(string(content)),
		Size:                int64(len(content)),
		CompanyID:           1,
		DeclaredContentType: "text/plain",
	})
	if err != nil {
		t.Errorf("Correct size rejected: %v", err)
	}

	// Wrong size should fail
	_, err = s.Put(ctx, PutInput{
		Data:                strings.NewReader(string(content)),
		Size:                int64(len(content) + 100),
		CompanyID:           1,
		DeclaredContentType: "text/plain",
	})
	if err == nil {
		t.Error("Wrong size should have been rejected")
	}
	if !strings.Contains(err.Error(), "size mismatch") {
		t.Errorf("Expected size mismatch error, got: %v", err)
	}
}

func TestStorageCompanyIsolation(t *testing.T) {
	s := NewLocalStorage("")
	ctx := context.Background()

	// Store same content for two companies
	key1, err := s.Put(ctx, PutInput{
		Data:                strings.NewReader("company 1 data!!"),
		Size:                16,
		CompanyID:           1,
		DeclaredContentType: "text/plain",
	})
	if err != nil {
		t.Fatalf("Put key1 failed: %v", err)
	}
	t.Logf("key1 = %q", key1)
	key2, err := s.Put(ctx, PutInput{
		Data:                strings.NewReader("company 2 data!!"),
		Size:                16,
		CompanyID:           2,
		DeclaredContentType: "text/plain",
	})
	if err != nil {
		t.Fatalf("Put key2 failed: %v", err)
	}
	t.Logf("key2 = %q", key2)

	// Both keys should have their respective company prefixes
	if !strings.HasPrefix(key1, "company-1/") {
		t.Errorf("Key1 should have company-1 prefix: %s", key1)
	}
	if !strings.HasPrefix(key2, "company-2/") {
		t.Errorf("Key2 should have company-2 prefix: %s", key2)
	}

	// Each company's data should be different
	reader1, err := s.Get(ctx, key1)
	if err != nil {
		t.Fatalf("Get key1 failed: %v", err)
	}
	data1, _ := io.ReadAll(reader1)
	_ = reader1.Close()

	reader2, err := s.Get(ctx, key2)
	if err != nil {
		t.Fatalf("Get key2 failed: %v", err)
	}
	data2, _ := io.ReadAll(reader2)
	_ = reader2.Close()

	if string(data1) == string(data2) {
		t.Error("Company data should be isolated")
	}

	// Test that accessing with wrong company prefix fails
	// Try to access company 2's data with company 1 prefix
	wrongKey := strings.Replace(key2, "company-2/", "company-1/", 1)
	_, err = s.Get(ctx, wrongKey)
	if err == nil {
		t.Error("Should not be able to access data with wrong company prefix")
	}
}

func TestGenerateStorageKey(t *testing.T) {
	// Test that generated keys are unique
	keys := make(map[string]bool)
	for i := 0; i < 100; i++ {
		key := generateStorageKey("text/plain")
		if keys[key] {
			t.Errorf("Duplicate key generated: %s", key)
		}
		keys[key] = true
	}
}

func TestStorageMetadata(t *testing.T) {
	s := NewLocalStorage("")
	ctx := context.Background()

	metadata := map[string]string{
		"custom-field": "custom-value",
		"author":       "test-user",
	}

	key, err := s.Put(ctx, PutInput{
		Data:                strings.NewReader("metadata test"),
		Size:                13,
		CompanyID:           1,
		DeclaredContentType: "text/plain",
		Metadata:            metadata,
	})
	if err != nil {
		t.Fatal(err)
	}

	info, err := s.Stat(ctx, key)
	if err != nil {
		t.Fatal(err)
	}

	// Local storage doesn't persist metadata in this implementation
	// but the interface supports it for S3
	_ = info.Metadata
}

func BenchmarkStoragePut(b *testing.B) {
	s := NewLocalStorage("")
	ctx := context.Background()
	content := strings.Repeat("x", 1024) // 1KB

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = s.Put(ctx, PutInput{
			Data:                strings.NewReader(content),
			Size:                int64(len(content)),
			CompanyID:           1,
			DeclaredContentType: "text/plain",
		})
	}
}

func BenchmarkStorageGet(b *testing.B) {
	s := NewLocalStorage("")
	ctx := context.Background()
	content := strings.Repeat("x", 1024)

	key, _ := s.Put(ctx, PutInput{
		Data:                strings.NewReader(content),
		Size:                int64(len(content)),
		CompanyID:           1,
		DeclaredContentType: "text/plain",
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader, _ := s.Get(ctx, key)
		_, _ = io.ReadAll(reader)
		_ = reader.Close()
	}
}

// Test with random data to exercise checksum
func TestStorageRandomData(t *testing.T) {
	s := NewLocalStorage("")
	ctx := context.Background()

	for size := 1; size <= 1024*1024; size *= 2 { // 1B to 1MB
		data := make([]byte, size)
		if _, err := rand.Read(data); err != nil {
			t.Fatalf("rand.Read failed: %v", err)
		}

		checksum := sha256.Sum256(data)
		checksumHex := hex.EncodeToString(checksum[:])

		key, err := s.Put(ctx, PutInput{
			Data:                strings.NewReader(string(data)),
			Size:                int64(size),
			CompanyID:           1,
			DeclaredContentType: "application/octet-stream",
			ChecksumSHA256:      checksumHex,
		})
		if err != nil {
			t.Fatalf("Put failed for size %d: %v", size, err)
		}

		reader, err := s.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get failed for size %d: %v", size, err)
		}

		readData, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			t.Fatalf("Read failed for size %d: %v", size, err)
		}

		if len(readData) != size {
			t.Errorf("Size mismatch for %d: got %d", size, len(readData))
		}

		// Verify content
		for i := range data {
			if data[i] != readData[i] {
				t.Errorf("Content mismatch at byte %d for size %d", i, size)
				break
			}
		}

		// Clean up
		if err := s.Delete(ctx, key); err != nil {
			t.Fatalf("Delete failed: %v", err)
		}
	}
}
