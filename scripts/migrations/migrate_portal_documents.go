//go:build ignore

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/storage"
)

func main() {
	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		dsn = "postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable"
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()

	// Initialize storage
	storageDir := os.Getenv("STORAGE_DIR")
	if storageDir == "" {
		storageDir = "./data/storage"
	}
	store, err := storage.NewStorage(ctx, storage.StorageConfig{
		Driver:   "local",
		LocalDir: storageDir,
	})
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}

	// Fetch all portal documents
	rows, err := pool.Query(ctx, `SELECT id, company_id, user_id, portal_type, filename, content_type, content, uploaded_at FROM portal_documents`)
	if err != nil {
		log.Fatalf("Failed to query portal documents: %v", err)
	}
	defer rows.Close()

	type portalDoc struct {
		ID          int64
		CompanyID   int64
		UserID      int64
		PortalType  string
		Filename    string
		ContentType string
		Content     []byte
		UploadedAt  string
	}

	var docs []portalDoc
	for rows.Next() {
		var doc portalDoc
		if err := rows.Scan(&doc.ID, &doc.CompanyID, &doc.UserID, &doc.PortalType, &doc.Filename, &doc.ContentType, &doc.Content, &doc.UploadedAt); err != nil {
			log.Fatalf("Error scanning row: %v", err)
		}
		docs = append(docs, doc)
	}
	rows.Close()

	for _, doc := range docs {
		// Calculate SHA256
		hash := sha256.Sum256(doc.Content)
		checksum := hex.EncodeToString(hash[:])
		size := int64(len(doc.Content))

		// Put object into storage
		reader := bytes.NewReader(doc.Content)
		storageKey, err := store.Put(ctx, storage.PutInput{
			Data:                reader,
			Size:                size,
			DeclaredContentType: doc.ContentType,
			ChecksumSHA256:      checksum,
			CompanyID:           doc.CompanyID,
			Classification:      "INTERNAL",
		})
		if err != nil {
			log.Printf("Failed to upload doc ID %d: %v", doc.ID, err)
			continue
		}

		// DB Transaction
		tx, err := pool.Begin(ctx)
		if err != nil {
			log.Printf("Failed to begin tx: %v", err)
			continue
		}

		// Get INTERNAL classification ID
		var classID int64
		err = tx.QueryRow(ctx, `SELECT id FROM document_classifications WHERE company_id = $1 AND code = 'INTERNAL'`, doc.CompanyID).Scan(&classID)
		if err != nil {
			log.Printf("Failed to get classification for company %d: %v", doc.CompanyID, err)
			tx.Rollback(ctx)
			continue
		}

		// Insert Blob
		var blobID int64
		err = tx.QueryRow(ctx, `
			INSERT INTO storage_blobs (company_id, storage_key, size_bytes, checksum_sha256, mime_type, malware_scan_status, created_by)
			VALUES ($1, $2, $3, $4, $5, 'CLEAN', $6)
			RETURNING id
		`, doc.CompanyID, storageKey, size, checksum, doc.ContentType, doc.UserID).Scan(&blobID)
		if err != nil {
			log.Printf("Failed to insert blob: %v", err)
			tx.Rollback(ctx)
			continue
		}

		// Insert Document
		var docID int64
		docNumber := fmt.Sprintf("PORTAL-MIG-%d", doc.ID)
		err = tx.QueryRow(ctx, `
			INSERT INTO documents (company_id, classification_id, document_number, title, owner_id, status, migration_source, migration_source_id, created_by, updated_by)
			VALUES ($1, $2, $3, $4, $5, 'PUBLISHED', 'portal_documents', $6, $5, $5)
			RETURNING id
		`, doc.CompanyID, classID, docNumber, doc.Filename, doc.UserID, fmt.Sprintf("%d", doc.ID)).Scan(&docID)
		if err != nil {
			log.Printf("Failed to insert document: %v", err)
			tx.Rollback(ctx)
			continue
		}

		// Insert Document Version
		_, err = tx.Exec(ctx, `
			INSERT INTO document_versions (company_id, document_id, version_number, version_label, blob_id, status, classification_id, change_summary, created_by)
			VALUES ($1, $2, 1, '1.0', $3, 'APPROVED', $4, 'Migrated from portal', $5)
		`, doc.CompanyID, docID, blobID, classID, doc.UserID)
		if err != nil {
			log.Printf("Failed to insert document version: %v", err)
			tx.Rollback(ctx)
			continue
		}

		if err := tx.Commit(ctx); err != nil {
			log.Printf("Failed to commit tx: %v", err)
		} else {
			log.Printf("Successfully migrated portal_document %d to document %d", doc.ID, docID)
		}
	}
	log.Println("Portal Documents Migration completed successfully!")
}
