package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// S3Storage implements Storage using S3-compatible object storage.
type S3Storage struct {
	client *s3.Client
	bucket string
}

// NewS3Storage creates a new S3 storage backend.
func NewS3Storage(ctx context.Context, cfg StorageConfig) (*S3Storage, error) {
	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, errors.New("storage: S3 bucket is required")
	}

	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = "us-east-1"
	}

	loadOptions := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(region)}
	if cfg.AccessKeyID != "" || cfg.SecretAccessKey != "" {
		if cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
			return nil, errors.New("storage: both S3 access key and secret key are required")
		}
		loadOptions = append(loadOptions, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("storage: load S3 config: %w", err)
	}

	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint != "" && !strings.Contains(endpoint, "://") {
		endpoint = "http://" + endpoint
	}

	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		options.UsePathStyle = cfg.UsePathStyle
		if endpoint != "" {
			options.BaseEndpoint = aws.String(strings.TrimRight(endpoint, "/"))
		}
	})

	storage := &S3Storage{client: client, bucket: cfg.Bucket}

	if cfg.AutoCreate {
		if err := storage.ensureBucket(ctx); err != nil {
			return nil, err
		}
	}

	return storage, nil
}

func (s *S3Storage) ensureBucket(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(s.bucket),
	})
	if err == nil {
		return nil // Bucket exists and accessible
	}

	// Check if it's a 404 (not found)
	var apiErr *smithy.OperationError
	if errors.As(err, &apiErr) {
		var respErr *types.NotFound
		if errors.As(apiErr.Err, &respErr) {
			// Try to create bucket
			_, createErr := s.client.CreateBucket(ctx, &s3.CreateBucketInput{
				Bucket: aws.String(s.bucket),
			})
			if createErr != nil {
				return fmt.Errorf("storage: create S3 bucket: %w", createErr)
			}
			return nil
		}
	}
	return fmt.Errorf("storage: verify S3 bucket: %w", err)
}

func (s *S3Storage) Put(ctx context.Context, input PutInput) (string, error) {
	if input.Data == nil {
		return "", errors.New("storage: nil data reader")
	}
	if input.CompanyID <= 0 {
		return "", errors.New("storage: company_id required")
	}

	// Generate opaque storage key with company prefix
	key := fmt.Sprintf("company-%d/%s", input.CompanyID, generateStorageKey(input.DeclaredContentType))

	// Stream with checksum verification
	hasher := sha256.New()
	multiReader := io.TeeReader(input.Data, hasher)

	// Prepare metadata
	metadata := map[string]string{}
	for k, v := range input.Metadata {
		metadata[k] = v
	}
	metadata["company-id"] = fmt.Sprintf("%d", input.CompanyID)
	metadata["classification"] = input.Classification
	if input.DetectedContentType != "" {
		metadata["detected-content-type"] = input.DetectedContentType
	}

	contentType := input.DeclaredContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        multiReader,
		ContentType: aws.String(contentType),
		Metadata:    metadata,
		ChecksumAlgorithm: types.ChecksumAlgorithmSha256,
	})
	if err != nil {
		return "", fmt.Errorf("storage: upload object: %w", err)
	}

	// Verify checksum if provided
	if input.ChecksumSHA256 != "" {
		actual := hex.EncodeToString(hasher.Sum(nil))
		if !strings.EqualFold(actual, input.ChecksumSHA256) {
			// Attempt to delete the uploaded object
			_ = s.Delete(ctx, key)
			return "", ErrChecksumMismatch{Expected: input.ChecksumSHA256, Actual: actual}
		}
	}

	return key, nil
}

func (s *S3Storage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var apiErr *smithy.OperationError
		if errors.As(err, &apiErr) {
			var respErr *types.NotFound
			if errors.As(apiErr.Err, &respErr) {
				return nil, ErrNotFound{Key: key}
			}
		}
		return nil, fmt.Errorf("storage: download object: %w", err)
	}
	return result.Body, nil
}

func (s *S3Storage) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	result, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var apiErr *smithy.OperationError
		if errors.As(err, &apiErr) {
			var respErr *types.NotFound
			if errors.As(apiErr.Err, &respErr) {
				return ObjectInfo{}, ErrNotFound{Key: key}
			}
		}
		return ObjectInfo{}, fmt.Errorf("storage: head object: %w", err)
	}

	info := ObjectInfo{
		Key:           key,
		Size:          aws.ToInt64(result.ContentLength),
		ContentType:   aws.ToString(result.ContentType),
		CreatedAt:     aws.ToTime(result.LastModified).Format(time.RFC3339),
		Metadata:      result.Metadata,
	}

	// Extract checksum from metadata
	if result.Metadata != nil {
		if cs, ok := result.Metadata["checksum-sha256"]; ok {
			info.ChecksumSHA256 = cs
		}
	}
	// ETag might have the checksum
	if info.ChecksumSHA256 == "" && result.ETag != nil {
		info.ChecksumSHA256 = strings.Trim(*result.ETag, "\"")
	}

	// Parse encryption metadata
	info.EncryptionMetadata = map[string]string{}
	for k, v := range result.Metadata {
		if strings.HasPrefix(k, "encryption-") {
			info.EncryptionMetadata[k] = v
		}
	}

	// Parse malware scan status
	if result.Metadata != nil {
		if ms, ok := result.Metadata["malware-scan-status"]; ok {
			info.MalwareScanStatus = MalwareScanStatus(ms)
		}
	}

	return info, nil
}

func (s *S3Storage) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var apiErr *smithy.OperationError
		if errors.As(err, &apiErr) {
			var respErr *types.NotFound
			if errors.As(apiErr.Err, &respErr) {
				return ErrNotFound{Key: key}
			}
		}
		return fmt.Errorf("storage: delete object: %w", err)
	}
	return nil
}

func (s *S3Storage) SignedURL(ctx context.Context, key string, expirySeconds int) (string, error) {
	presigner := s3.NewPresignClient(s.client)
	result, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = time.Duration(expirySeconds) * time.Second
	})
	if err != nil {
		return "", fmt.Errorf("storage: generate signed URL: %w", err)
	}
	return result.URL, nil
}