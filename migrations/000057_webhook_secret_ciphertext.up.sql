ALTER TABLE webhook_subscriptions ADD COLUMN IF NOT EXISTS secret_ciphertext TEXT NOT NULL DEFAULT '';
