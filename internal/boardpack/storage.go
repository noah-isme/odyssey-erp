package boardpack

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Storage persists generated board-pack documents and opens them for download.
type Storage interface {
	Save(ctx context.Context, id int64, pdf []byte) (string, error)
	Open(ctx context.Context, key string) (io.ReadCloser, error)
}

// StorageConfig selects local filesystem or S3-compatible object storage.
type StorageConfig struct {
	Driver          string
	LocalDir        string
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	UsePathStyle    bool
	AutoCreate      bool
}

// NewStorage constructs the configured board-pack storage backend.
func NewStorage(ctx context.Context, cfg StorageConfig) (Storage, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Driver)) {
	case "", "local":
		return &LocalStorage{dir: cfg.LocalDir}, nil
	case "s3":
		return newS3Storage(ctx, cfg)
	default:
		return nil, fmt.Errorf("boardpack: unsupported storage driver %q", cfg.Driver)
	}
}

// LocalStorage stores files on a shared local filesystem.
type LocalStorage struct {
	dir string
}

func (s *LocalStorage) Save(_ context.Context, id int64, pdf []byte) (string, error) {
	dir := strings.TrimSpace(s.dir)
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "boardpacks")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("board-pack-%d.pdf", id))
	if err := os.WriteFile(path, pdf, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func (s *LocalStorage) Open(_ context.Context, key string) (io.ReadCloser, error) {
	return os.Open(key)
}

type s3API interface {
	PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	HeadBucket(context.Context, *s3.HeadBucketInput, ...func(*s3.Options)) (*s3.HeadBucketOutput, error)
	CreateBucket(context.Context, *s3.CreateBucketInput, ...func(*s3.Options)) (*s3.CreateBucketOutput, error)
}

type s3Storage struct {
	client s3API
	bucket string
}

func newS3Storage(ctx context.Context, cfg StorageConfig) (Storage, error) {
	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, errors.New("boardpack: S3 bucket is required")
	}
	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = "us-east-1"
	}
	loadOptions := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(region)}
	if cfg.AccessKeyID != "" || cfg.SecretAccessKey != "" {
		if cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
			return nil, errors.New("boardpack: both S3 access key and secret key are required")
		}
		loadOptions = append(loadOptions, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("boardpack: load S3 config: %w", err)
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
	storage := &s3Storage{client: client, bucket: cfg.Bucket}
	if cfg.AutoCreate {
		if err := storage.ensureBucket(ctx); err != nil {
			return nil, err
		}
	}
	return storage, nil
}

func (s *s3Storage) ensureBucket(ctx context.Context) error {
	if _, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)}); err == nil {
		return nil
	}
	if _, err := s.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(s.bucket)}); err != nil {
		return fmt.Errorf("boardpack: create S3 bucket: %w", err)
	}
	return nil
}

func (s *s3Storage) Save(ctx context.Context, id int64, pdf []byte) (string, error) {
	key := fmt.Sprintf("board-packs/board-pack-%d.pdf", id)
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(pdf),
		ContentType: aws.String("application/pdf"),
	})
	if err != nil {
		return "", fmt.Errorf("boardpack: upload PDF: %w", err)
	}
	return key, nil
}

func (s *s3Storage) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("boardpack: download PDF: %w", err)
	}
	return result.Body, nil
}
