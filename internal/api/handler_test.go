package api

import "testing"

func TestWebhookSignature(t *testing.T) {
	payload := []byte(`{"event":"project.created"}`)
	sig := SignWebhook("secret", payload, "1700000000")
	if !VerifyWebhookSignature("secret", payload, "1700000000", sig) {
		t.Fatal("signature did not verify")
	}
	if VerifyWebhookSignature("wrong", payload, "1700000000", sig) {
		t.Fatal("wrong secret verified")
	}
}
func TestHashKeyIsDeterministic(t *testing.T) {
	if HashKey("key") != HashKey("key") || HashKey("key") == HashKey("other") {
		t.Fatal("invalid key hash")
	}
}

func TestWebhookSecretEncryptionRoundTrip(t *testing.T) {
	h := NewHandler(nil, []byte("session-secret"))
	ciphertext, err := h.encryptSecret("webhook-secret")
	if err != nil {
		t.Fatal(err)
	}
	plain, err := h.decryptSecret(ciphertext)
	if err != nil || plain != "webhook-secret" {
		t.Fatalf("round trip plain=%q err=%v", plain, err)
	}
}
