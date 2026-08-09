package awss3

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// Adapter implements the connector for S3-backed exports.
type Adapter struct {
	logger  *slog.Logger
	vault   *shared.Vault
	options connectors.ProviderOptions
}

func NewAdapter(logger *slog.Logger, vault *shared.Vault, options ...connectors.ProviderOptions) *Adapter {
	var opts connectors.ProviderOptions
	if len(options) > 0 {
		opts = options[0]
	}
	if opts.Vault == nil {
		opts.Vault = vault
	}
	return &Adapter{logger: logger, vault: vault, options: opts}
}

type Credentials struct {
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	SessionToken    string `json:"session_token"`
	Region          string `json:"region"`
	Bucket          string `json:"bucket"`
	Endpoint        string `json:"endpoint"`
	UsePathStyle    bool   `json:"use_path_style"`
}

func (a *Adapter) credentials(conn *connectors.Connection) (Credentials, error) {
	options := a.options
	if options.Vault == nil {
		options.Vault = a.vault
	}
	secret, err := options.ResolveSecret(conn)
	if err != nil {
		return Credentials{}, err
	}
	var creds Credentials
	if err := json.Unmarshal([]byte(secret), &creds); err != nil {
		return Credentials{}, fmt.Errorf("awss3: invalid credential format: %w", err)
	}
	if strings.TrimSpace(creds.AccessKeyID) == "" || strings.TrimSpace(creds.SecretAccessKey) == "" {
		return Credentials{}, errors.New("awss3: access_key_id and secret_access_key are required")
	}
	if strings.TrimSpace(creds.Region) == "" {
		return Credentials{}, errors.New("awss3: region is required")
	}
	if strings.TrimSpace(creds.Bucket) == "" {
		return Credentials{}, errors.New("awss3: bucket is required")
	}
	return creds, nil
}

func (a *Adapter) ValidateConnection(ctx context.Context, conn *connectors.Connection) error {
	if _, err := a.credentials(conn); err != nil {
		return fmt.Errorf("awss3: validate credentials: %w", err)
	}
	return nil
}

func (a *Adapter) CheckHealth(ctx context.Context, conn *connectors.Connection) (connectors.ConnectionStatus, error) {
	creds, err := a.credentials(conn)
	if err != nil {
		return connectors.StatusActionRequired, err
	}
	if a.options.DevelopmentMode {
		return connectors.StatusHealthy, nil
	}
	client, err := a.client(ctx, creds)
	if err != nil {
		return connectors.StatusActionRequired, err
	}
	if _, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(creds.Bucket)}); err != nil {
		return connectors.StatusActionRequired, fmt.Errorf("awss3: health check failed: %w", err)
	}
	return connectors.StatusHealthy, nil
}

func (a *Adapter) RefreshToken(ctx context.Context, conn *connectors.Connection) error {
	return connectors.ErrUnsupportedOperation
}

func (a *Adapter) VerifyCallbackSignature(ctx context.Context, conn *connectors.Connection, headers map[string]string, payload []byte) error {
	return errors.New("awss3: callbacks are not supported")
}

type ExportPayload struct {
	ObjectKey string `json:"object_key"`
	Content   string `json:"content"`
	MimeType  string `json:"mime_type"`
}

func (a *Adapter) ExecuteCommand(ctx context.Context, conn *connectors.Connection, cmd *connectors.OutboxCommand) error {
	if cmd == nil {
		return errors.New("awss3: command is required")
	}
	if cmd.CommandType != "bi.export" {
		return errors.New("unsupported command type for awss3")
	}
	if a.options.DevelopmentMode && strings.TrimSpace(conn.SecretRef) == "" {
		a.logger.Info("S3 export simulated in explicit development mode", slog.Int64("company_id", conn.CompanyID))
		return nil
	}
	return a.handleBIExport(ctx, conn, cmd)
}

func (a *Adapter) handleBIExport(ctx context.Context, conn *connectors.Connection, cmd *connectors.OutboxCommand) error {
	creds, err := a.credentials(conn)
	if err != nil {
		return err
	}
	var reqPayload ExportPayload
	if err := json.Unmarshal(cmd.Payload, &reqPayload); err != nil {
		return fmt.Errorf("awss3: failed to parse export payload: %w", err)
	}
	if strings.TrimSpace(reqPayload.ObjectKey) == "" {
		if cmd.ID > 0 {
			reqPayload.ObjectKey = fmt.Sprintf("exports/commands/%d", cmd.ID)
		} else if strings.TrimSpace(cmd.CorrelationID) != "" {
			reqPayload.ObjectKey = "exports/correlation/" + cmd.CorrelationID
		} else {
			return errors.New("awss3: object_key or correlation_id is required")
		}
	}
	client, err := a.client(ctx, creds)
	if err != nil {
		return err
	}
	content := []byte(reqPayload.Content)
	digest := sha256.Sum256(content)
	contentType := reqPayload.MimeType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:            aws.String(creds.Bucket),
		Key:               aws.String(reqPayload.ObjectKey),
		Body:              io.NopCloser(strings.NewReader(string(content))),
		ContentType:       aws.String(contentType),
		ChecksumAlgorithm: "SHA256",
		ChecksumSHA256:    aws.String(base64.StdEncoding.EncodeToString(digest[:])),
		Metadata: map[string]string{
			"odyssey-company-id":     fmt.Sprintf("%d", conn.CompanyID),
			"odyssey-correlation-id": cmd.CorrelationID,
		},
	})
	if err != nil {
		return fmt.Errorf("awss3: put object failed: %w", err)
	}
	return nil
}

func (a *Adapter) client(ctx context.Context, creds Credentials) (*s3.Client, error) {
	loadOptions := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(creds.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(creds.AccessKeyID, creds.SecretAccessKey, creds.SessionToken)),
		awsconfig.WithHTTPClient(a.options.HTTPClientOrDefault()),
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("awss3: load AWS config: %w", err)
	}
	return s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		options.UsePathStyle = creds.UsePathStyle
		if strings.TrimSpace(creds.Endpoint) != "" {
			options.BaseEndpoint = aws.String(strings.TrimRight(creds.Endpoint, "/"))
		}
	}), nil
}

func (a *Adapter) TranslateWebhook(ctx context.Context, conn *connectors.Connection, headers map[string]string, payload []byte) ([]*connectors.CanonicalEvent, error) {
	return nil, errors.New("awss3: callbacks are not supported")
}
