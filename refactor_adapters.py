import os
import glob

def refactor():
    for filepath in glob.glob("internal/connectors/providers/*/adapter.go"):
        with open(filepath, "r") as f:
            content = f.read()

        # Replace VerifyCallbackSignature
        content = content.replace(
            "VerifyCallbackSignature(ctx context.Context, payload []byte, signature string) error",
            "VerifyCallbackSignature(ctx context.Context, headers map[string]string, payload []byte) error"
        )
        
        # Replace TranslateWebhook
        content = content.replace(
            "TranslateWebhook(ctx context.Context, conn *connectors.Connection, providerEventID string, payload []byte) ([]*connectors.CanonicalEvent, error)",
            "TranslateWebhook(ctx context.Context, conn *connectors.Connection, headers map[string]string, payload []byte) ([]*connectors.CanonicalEvent, error)"
        )

        with open(filepath, "w") as f:
            f.write(content)
            print(f"Updated {filepath}")

refactor()
