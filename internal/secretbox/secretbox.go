// Package secretbox provides authenticated symmetric encryption (AES-256-GCM)
// for small secrets stored at rest (e.g. node firewall admin passwords). The key
// is derived from the gateway MNEMONIC, so no extra config is required and the
// key is stable across restarts. Dependency-free (stdlib crypto only).
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
)

// Box seals/opens secrets with a key derived from a passphrase.
type Box struct {
	gcm cipher.AEAD
}

// New derives a Box from a passphrase (the gateway MNEMONIC). An empty
// passphrase yields a nil Box (Enabled()=false) so callers can 503 gracefully.
func New(passphrase string) *Box {
	if passphrase == "" {
		return nil
	}
	// Domain-separated 256-bit key: SHA-256(passphrase || label).
	key := sha256.Sum256([]byte(passphrase + "\x00erebrus-firewall-cred-v1"))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil
	}
	return &Box{gcm: gcm}
}

// Enabled reports whether encryption is available (passphrase was set).
func (b *Box) Enabled() bool { return b != nil && b.gcm != nil }

// Seal encrypts plaintext, returning nonce||ciphertext.
func (b *Box) Seal(plaintext string) ([]byte, error) {
	if !b.Enabled() {
		return nil, errors.New("secretbox not configured")
	}
	nonce := make([]byte, b.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return b.gcm.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

// Open decrypts a nonce||ciphertext blob produced by Seal.
func (b *Box) Open(blob []byte) (string, error) {
	if !b.Enabled() {
		return "", errors.New("secretbox not configured")
	}
	ns := b.gcm.NonceSize()
	if len(blob) < ns {
		return "", errors.New("ciphertext too short")
	}
	nonce, ct := blob[:ns], blob[ns:]
	pt, err := b.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", errors.New("decryption failed")
	}
	return string(pt), nil
}
