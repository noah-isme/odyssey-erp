package whatsapp_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/whatsapp"
)

func TestExecuteCommand_SendWhatsApp(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	adapter := whatsapp.NewAdapter(logger, nil)

	payload := whatsapp.SendWhatsAppPayload{
		To:      "1234567890",
		Content: "Hello World",
	}
	payloadBytes, _ := json.Marshal(payload)

	cmd := &connectors.OutboxCommand{
		CommandType: "messaging.send",
		Payload:     payloadBytes,
	}

	conn := &connectors.Connection{
		CompanyID: 1,
	}

	err := adapter.ExecuteCommand(context.Background(), conn, cmd)
	require.NoError(t, err)
}
