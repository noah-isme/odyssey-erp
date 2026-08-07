package shared

import (
	"crypto/sha256"
	"errors"
	"os"

	"golang.org/x/crypto/pbkdf2"
)

// Vault provides application-wide Encryption-at-Rest for sensitive fields.
// It derives a secure 32-byte key from the APP_MASTER_KEY environment variable.
type Vault struct {
	key []byte
}

func NewVault() (*Vault, error) {
	masterKey := os.Getenv("APP_MASTER_KEY")
	if masterKey == "" {
		return nil, errors.New("APP_MASTER_KEY environment variable is required for Encryption-at-Rest")
	}

	// Derive a 32-byte key using PBKDF2 (using a static salt for application-level deterministic encryption)
	salt := []byte("odyssey-erp-encryption-salt-2026")
	key := pbkdf2.Key([]byte(masterKey), salt, 100000, 32, sha256.New)

	return &Vault{key: key}, nil
}

// EncryptSecure stores API keys, PII, and other sensitive data safely in the DB.
func (v *Vault) EncryptSecure(plaintext string) (string, error) {
	return Encrypt(v.key, plaintext)
}

// DecryptSecure retrieves the original string for active use in memory.
func (v *Vault) DecryptSecure(ciphertext string) (string, error) {
	return Decrypt(v.key, ciphertext)
}
