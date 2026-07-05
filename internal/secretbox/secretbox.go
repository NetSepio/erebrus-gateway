// Package secretbox provides authenticated symmetric encryption (AES-256-GCM)
// for small secrets stored at rest (e.g. Shield / AdGuard admin passwords).
//
// This is reversible encryption, not one-way password hashing: Seal stores
// ciphertext in Postgres; authorized callers Open to reveal the original
// plaintext for operators who need to copy credentials into AdGuard.
//
// HKDF-SHA256 derives the AES key from the gateway MNEMONIC (domain-separated,
// not bcrypt/argon2-style login hashing). Ciphertext sealed before the HKDF
// upgrade is still readable via the legacy SHA-256 KDF until re-sealed.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"

	"golang.org/x/crypto/hkdf"
)

const (
	kdfSalt       = "erebrus-firewall-cred-v1"
	kdfInfo       = "aes-256-gcm-key"
	legacyKDFJoin = "\x00" + kdfSalt
)

// Box encrypts and decrypts secrets at rest. Seal writes ciphertext; Open
// returns the original plaintext (e.g. AdGuard admin_password on reveal).
type Box struct {
	gcm       cipher.AEAD
	legacyGCM cipher.AEAD
}

// New derives a Box from a passphrase (the gateway MNEMONIC). An empty
// passphrase yields a nil Box (Enabled()=false) so callers can 503 gracefully.
func New(passphrase string) *Box {
	if passphrase == "" {
		return nil
	}
	key, err := deriveKeyHKDF(passphrase)
	if err != nil {
		return nil
	}
	gcm, err := newGCM(key[:])
	if err != nil {
		return nil
	}
	legacyKey := deriveKeyLegacy(passphrase)
	legacyGCM, err := newGCM(legacyKey[:])
	if err != nil {
		return nil
	}
	return &Box{gcm: gcm, legacyGCM: legacyGCM}
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// deriveKeyHKDF builds the AES-256-GCM key from the MNEMONIC (KDF, not password hash).
func deriveKeyHKDF(passphrase string) ([32]byte, error) {
	hk := hkdf.New(sha256.New, []byte(passphrase), []byte(kdfSalt), []byte(kdfInfo))
	var key [32]byte
	if _, err := io.ReadFull(hk, key[:]); err != nil {
		return key, err
	}
	return key, nil
}

// deriveKeyLegacy is the pre-HKDF KDF kept for decrypting existing rows.
func deriveKeyLegacy(passphrase string) [32]byte {
	return sha256.Sum256([]byte(passphrase + legacyKDFJoin))
}

// Enabled reports whether encryption is available (passphrase was set).
func (b *Box) Enabled() bool { return b != nil && b.gcm != nil }

// Seal encrypts plaintext with the HKDF-derived key, returning nonce||ciphertext.
func (b *Box) Seal(plaintext string) ([]byte, error) {
	return b.sealWith(b.gcm, plaintext)
}

func (b *Box) sealWith(gcm cipher.AEAD, plaintext string) ([]byte, error) {
	if !b.Enabled() {
		return nil, errors.New("secretbox not configured")
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

// Open decrypts a nonce||ciphertext blob produced by Seal (or the legacy KDF).
func (b *Box) Open(blob []byte) (string, error) {
	plaintext, _, err := b.OpenBlob(blob)
	return plaintext, err
}

// OpenBlob decrypts blob. legacyKDF is true when the row used the pre-HKDF key;
// callers should re-Seal and persist to upgrade storage.
func (b *Box) OpenBlob(blob []byte) (plaintext string, legacyKDF bool, err error) {
	if !b.Enabled() {
		return "", false, errors.New("secretbox not configured")
	}
	if pt, err := b.openWith(b.gcm, blob); err == nil {
		return pt, false, nil
	}
	pt, err := b.openWith(b.legacyGCM, blob)
	if err != nil {
		return "", false, errors.New("decryption failed")
	}
	return pt, true, nil
}

func (b *Box) openWith(gcm cipher.AEAD, blob []byte) (string, error) {
	ns := gcm.NonceSize()
	if len(blob) < ns {
		return "", errors.New("ciphertext too short")
	}
	nonce, ct := blob[:ns], blob[ns:]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

// sealLegacy encrypts with the legacy KDF (tests only).
func (b *Box) sealLegacy(plaintext string) ([]byte, error) {
	return b.sealWith(b.legacyGCM, plaintext)
}