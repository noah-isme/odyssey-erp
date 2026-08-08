package awss3_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/awss3"
)

func TestExecuteCommand_BIExport(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	adapter := awss3.NewAdapter(logger, nil)

	payload := awss3.ExportPayload{
		ObjectKey: "exports/2026-08/kpi_summary.csv",
		Content:   "Metric,Value\nRevenue,1000\n",
		MimeType:  "text/csv",
	}
	payloadBytes, _ := json.Marshal(payload)

	cmd := &connectors.OutboxCommand{
		CommandType: "bi.export",
		Payload:     payloadBytes,
	}

	conn := &connectors.Connection{
		CompanyID: 1,
	}

	err := adapter.ExecuteCommand(context.Background(), conn, cmd)
	require.NoError(t, err)
}
