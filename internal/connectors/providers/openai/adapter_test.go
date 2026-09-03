package openai_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/openai"
)

func TestAdapterValidatesAndExecutesGenerationCommands(t *testing.T) {
	adapter := openai.NewAdapter(slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()
	conn := &connectors.Connection{ID: 6, CompanyID: 8, Name: "OpenAI"}
	if err := adapter.ValidateConnection(ctx, conn); err == nil {
		t.Fatal("ValidateConnection() accepted missing API key")
	}
	conn.SecretRef = "openai-key-ref"
	if err := adapter.ValidateConnection(ctx, conn); err != nil {
		t.Fatal(err)
	}
	if status, err := adapter.CheckHealth(ctx, conn); err != nil || status != connectors.StatusHealthy {
		t.Fatalf("CheckHealth() = %v, %v", status, err)
	}
	if err := adapter.RefreshToken(ctx, conn); err != nil {
		t.Fatal(err)
	}
	if err := adapter.VerifyCallbackSignature(ctx, conn, nil, nil); err != nil {
		t.Fatal(err)
	}

	payload, _ := json.Marshal(openai.PromptPayload{Prompt: "Summarize", Model: "gpt-test"})
	if err := adapter.ExecuteCommand(ctx, conn, &connectors.OutboxCommand{CommandType: "ai.generate", Payload: payload}); err != nil {
		t.Fatal(err)
	}
	if err := adapter.ExecuteCommand(ctx, conn, &connectors.OutboxCommand{CommandType: "ai.generate", Payload: []byte("bad")}); err == nil {
		t.Fatal("ExecuteCommand() accepted malformed payload")
	}
	if err := adapter.ExecuteCommand(ctx, conn, &connectors.OutboxCommand{CommandType: "ai.embed"}); err == nil {
		t.Fatal("ExecuteCommand() accepted unsupported command")
	}
	events, err := adapter.TranslateWebhook(ctx, conn, nil, []byte("payload"))
	if err != nil || events != nil {
		t.Fatalf("TranslateWebhook() = %#v, %v", events, err)
	}
}
