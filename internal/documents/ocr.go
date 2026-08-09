package documents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const maxOCRInputBytes = 25 << 20

// OCRExtractor is the worker-side extraction boundary. Production can inject
// a real PDF/image OCR provider without coupling the document service to a
// vendor SDK.
type OCRExtractor interface {
	Extract(context.Context, string, io.Reader) (string, error)
}

// PlainTextOCRExtractor handles text-based document formats locally. Binary
// PDF/image content is rejected explicitly until a real OCR provider is
// configured through WithOCRExtractor.
type PlainTextOCRExtractor struct{}

func (PlainTextOCRExtractor) Extract(ctx context.Context, contentType string, reader io.Reader) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxOCRInputBytes+1))
	if err != nil {
		return "", fmt.Errorf("read OCR input: %w", err)
	}
	if len(data) > maxOCRInputBytes {
		return "", fmt.Errorf("OCR input exceeds %d bytes", maxOCRInputBytes)
	}
	if !isTextContentType(contentType) || !utf8.Valid(data) || strings.IndexByte(string(data), 0) >= 0 {
		return "", errors.New("binary OCR input requires a configured OCR extractor")
	}
	return strings.TrimSpace(string(data)), nil
}

func isTextContentType(contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if contentType == "" {
		return true
	}
	return strings.HasPrefix(contentType, "text/") || contentType == "application/json" ||
		contentType == "application/xml" || contentType == "application/rtf" ||
		contentType == "application/x-yaml" || contentType == "text/csv"
}

// ProcessOCR creates and processes a job synchronously for callers that need
// a direct service API. HTTP uses InitiateOCRJob plus the durable worker task.
func (s *Service) ProcessOCR(ctx context.Context, versionID int64) error {
	if versionID <= 0 {
		return errors.New("documents: version id required")
	}
	version, err := s.repo.GetDocumentVersion(ctx, versionID)
	if err != nil {
		return err
	}
	if version.BlobID == nil {
		return errors.New("documents: document version has no blob")
	}
	jobID, err := s.InitiateOCRJob(ctx, version.CompanyID, versionID, *version.BlobID)
	if err != nil {
		return err
	}
	return s.ProcessOCRJob(ctx, jobID)
}

// ProcessOCRJob executes one persisted OCR job and updates the search index
// only after extraction succeeds. IndexDocumentSearch replaces the version's
// previous entry, making retries safe.
func (s *Service) ProcessOCRJob(ctx context.Context, jobID int64) error {
	if jobID <= 0 {
		return errors.New("documents: OCR job id required")
	}
	if s.repo == nil || s.storage == nil {
		return errors.New("documents: OCR worker dependencies are not configured")
	}
	job, err := s.repo.GetOCRJob(ctx, jobID)
	if err != nil {
		return err
	}
	if job.Status == "COMPLETED" {
		return nil
	}
	if err := s.repo.UpdateOCRJob(ctx, job.ID, "PROCESSING", "", "", nil); err != nil {
		return fmt.Errorf("documents: mark OCR job processing: %w", err)
	}

	failed := func(cause error) error {
		message := cause.Error()
		if updateErr := s.repo.UpdateOCRJob(ctx, job.ID, "FAILED", "", message, nil); updateErr != nil {
			return fmt.Errorf("%w; mark OCR job failed: %v", cause, updateErr)
		}
		return cause
	}

	version, err := s.repo.GetDocumentVersion(ctx, job.DocumentVersionID)
	if err != nil {
		return failed(err)
	}
	if version.CompanyID != job.CompanyID || version.BlobID == nil || *version.BlobID != job.BlobID {
		return failed(errors.New("documents: OCR job does not match document version"))
	}
	blob, err := s.repo.GetBlob(ctx, job.BlobID)
	if err != nil {
		return failed(err)
	}
	reader, err := s.storage.Get(ctx, blob.StorageKey)
	if err != nil {
		return failed(fmt.Errorf("documents: load OCR blob: %w", err))
	}
	defer func() { _ = reader.Close() }()

	extractor := s.ocrExtractor
	if extractor == nil {
		extractor = PlainTextOCRExtractor{}
	}
	contentType := blob.DetectedContentType
	if contentType == "" {
		contentType = blob.DeclaredContentType
	}
	extracted, err := extractor.Extract(ctx, contentType, reader)
	if err != nil {
		return failed(fmt.Errorf("documents: extract OCR text: %w", err))
	}

	doc, err := s.repo.GetDocument(ctx, version.DocumentID)
	if err != nil {
		return failed(err)
	}
	if _, err := s.repo.IndexDocumentSearch(ctx, doc.ID, version.ID, doc.Title, extracted, ""); err != nil {
		return failed(fmt.Errorf("documents: update search index: %w", err))
	}
	completedAt := s.now()
	if err := s.repo.UpdateOCRJob(ctx, job.ID, "COMPLETED", extracted, "", &completedAt); err != nil {
		return fmt.Errorf("documents: mark OCR job completed: %w", err)
	}
	return nil
}

// OCRJobStatus returns a persisted job for status polling endpoints.
func (s *Service) OCRJobStatus(ctx context.Context, jobID int64) (DocumentOCRJob, error) {
	if jobID <= 0 {
		return DocumentOCRJob{}, errors.New("documents: OCR job id required")
	}
	return s.repo.GetOCRJob(ctx, jobID)
}
