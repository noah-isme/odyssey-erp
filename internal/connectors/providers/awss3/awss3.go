package awss3

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

type Adapter struct {
	logger *slog.Logger
	vault  *shared.Vault
}

func NewAdapter(logger *slog.Logger, vault *shared.Vault) *Adapter {
	return &Adapter{
		logger: logger,
		vault:  vault,
	}
}

type Credentials struct {
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	Region          string `json:"region"`
	Bucket          string `json:"bucket"`
}

func (a *Adapter) ValidateConnection(ctx context.Context, conn *connectors.Connection) error {
	credsStr, err := conn.GetCredentials(a.vault)
	if err != nil {
		return err
	}

	var creds Credentials
	if err := json.Unmarshal([]byte(credsStr), &creds); err != nil {
		return errors.New("invalid credentials format")
	}

	if creds.AccessKeyID == "" || creds.SecretAccessKey == "" || creds.Bucket == "" {
		return errors.New("missing required credentials")
	}

	return nil
}

func (a *Adapter) CheckHealth(ctx context.Context, conn *connectors.Connection) (connectors.ConnectionStatus, error) {
	return connectors.StatusHealthy, nil
}

func (a *Adapter) RefreshToken(ctx context.Context, conn *connectors.Connection) error {
	return nil
}

func (a *Adapter) VerifyCallbackSignature(ctx context.Context, conn *connectors.Connection, headers map[string]string, payload []byte) error {
	return nil
}

type ExportPayload struct {
	ObjectKey string `json:"object_key"`
	Content   string `json:"content"` // base64 encoded content or plain text depending on size
	MimeType  string `json:"mime_type"`
}

func (a *Adapter) ExecuteCommand(ctx context.Context, conn *connectors.Connection, cmd *connectors.OutboxCommand) error {
	switch cmd.CommandType {
	case "bi.export":
		return a.handleBIExport(ctx, conn, cmd)
	default:
		return errors.New("unsupported command type for awss3")
	}
}

func (a *Adapter) handleBIExport(ctx context.Context, conn *connectors.Connection, cmd *connectors.OutboxCommand) error {
	a.logger.Info("Executing bi.export for AWS S3", slog.Int64("company_id", conn.CompanyID))

	// credsStr, err := conn.GetCredentials(a.vault)
	// if err != nil {
	// 	return err
	// }
	// Simulated credentials
	credsStr := `{"access_key_id":"simulated_key","secret_access_key":"simulated_secret","region":"us-east-1","bucket":"bi-exports"}`

	var creds Credentials
	if err := json.Unmarshal([]byte(credsStr), &creds); err != nil {
		return err
	}

	var reqPayload ExportPayload
	if err := json.Unmarshal(cmd.Payload, &reqPayload); err != nil {
		return err
	}

	// TODO: implement actual S3 PutObject call using aws-sdk-go-v2
	// e.g., client.PutObject(ctx, &s3.PutObjectInput{
	// 	Bucket: &creds.Bucket,
	// 	Key:    &reqPayload.ObjectKey,
	// 	Body:   strings.NewReader(reqPayload.Content),
	// })

	return nil
}

func (a *Adapter) TranslateWebhook(ctx context.Context, conn *connectors.Connection, headers map[string]string, payload []byte) ([]*connectors.CanonicalEvent, error) {
	return nil, nil
}
