package api

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
)

func (h *Handler) decryptSecret(encoded string) (string, error) {
	if len(h.encryptionKey) == 0 {
		return "", fmt.Errorf("api: webhook encryption key is not configured")
	}
	raw, err := hex.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	key := sha256Key(h.encryptionKey)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	size := gcm.NonceSize()
	if len(raw) < size {
		return "", fmt.Errorf("api: invalid webhook secret")
	}
	plain, err := gcm.Open(nil, raw[:size], raw[size:], nil)
	return string(plain), err
}

func sha256Key(value []byte) [32]byte { return sha256.Sum256(value) }

// Publish enqueues deliveries only after the caller's source transaction has committed.
func (h *Handler) Publish(ctx context.Context, companyID int64, eventType string, payload any) error {
	eventID := uuid.New()
	body, err := json.Marshal(map[string]any{"event_id": eventID.String(), "event_type": eventType, "occurred_at": time.Now().UTC(), "company_id": companyID, "schema_version": "1.0", "aggregate": payload})
	if err != nil {
		return err
	}
	_, err = h.pool.Exec(ctx, `INSERT INTO webhook_deliveries(subscription_id,event_id,payload,next_attempt_at) SELECT id,$1,$2,NOW() FROM webhook_subscriptions WHERE company_id=$3 AND event_type=$4 AND active ON CONFLICT(subscription_id,event_id) DO NOTHING`, eventID, body, companyID, eventType)
	return err
}

// DeliverDue sends queued webhook deliveries and records retries/dead letters.
func (h *Handler) DeliverDue(ctx context.Context, client *http.Client, limit int) (int, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := h.pool.Query(ctx, `WITH claimed AS (
		SELECT d.id FROM webhook_deliveries d
		WHERE d.delivered_at IS NULL AND d.dead_lettered_at IS NULL
		  AND COALESCE(d.next_attempt_at,NOW()) <= NOW()
		ORDER BY d.id LIMIT $1 FOR UPDATE SKIP LOCKED
	)
	UPDATE webhook_deliveries d SET next_attempt_at=NOW()+INTERVAL '5 minutes'
	FROM claimed c WHERE d.id=c.id
	RETURNING d.id,d.event_id,d.payload,(SELECT s.endpoint FROM webhook_subscriptions s WHERE s.id=d.subscription_id),(SELECT s.secret_ciphertext FROM webhook_subscriptions s WHERE s.id=d.subscription_id),d.attempt_count`, limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	type delivery struct {
		id               int64
		eventID          uuid.UUID
		payload          []byte
		endpoint, secret string
		attempts         int
	}
	items := []delivery{}
	for rows.Next() {
		var d delivery
		if err := rows.Scan(&d.id, &d.eventID, &d.payload, &d.endpoint, &d.secret, &d.attempts); err != nil {
			return 0, err
		}
		items = append(items, d)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	count := 0
	for _, d := range items {
		secret, err := h.decryptSecret(d.secret)
		if err != nil {
			return count, err
		}
		timestamp := fmt.Sprintf("%d", time.Now().Unix())
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.endpoint, bytes.NewReader(d.payload))
		if err != nil {
			return count, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Odyssey-Event-ID", d.eventID.String())
		req.Header.Set("X-Odyssey-Timestamp", timestamp)
		req.Header.Set("X-Odyssey-Signature", SignWebhook(secret, d.payload, timestamp))
		resp, err := client.Do(req)
		status := 0
		if err == nil {
			status = resp.StatusCode
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
		if err == nil && status >= 200 && status < 300 {
			_, err = h.pool.Exec(ctx, `UPDATE webhook_deliveries SET attempt_count=attempt_count+1,response_status=$1,delivered_at=NOW() WHERE id=$2`, status, d.id)
		} else {
			attempt := d.attempts + 1
			if attempt >= 8 {
				_, err = h.pool.Exec(ctx, `UPDATE webhook_deliveries SET attempt_count=$1,response_status=$2,dead_lettered_at=NOW() WHERE id=$3`, attempt, status, d.id)
			} else {
				delay := time.Duration(1<<minInt(attempt, 6)) * time.Minute
				_, err = h.pool.Exec(ctx, `UPDATE webhook_deliveries SET attempt_count=$1,response_status=$2,next_attempt_at=$3 WHERE id=$4`, attempt, status, time.Now().Add(delay), d.id)
			}
		}
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
