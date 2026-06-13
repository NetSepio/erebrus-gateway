// Package wallet verifies wallet-signature logins across the chains Erebrus
// supports (EVM, Solana, Aptos, Sui). Each Verify* returns the wallet address
// recovered/derived from the signature; the caller compares it (case-insensitive)
// to the address that requested the flow id. No database access here.
package wallet

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	blake2b "github.com/minio/blake2b-simd"
	"github.com/mr-tron/base58"
	"golang.org/x/crypto/sha3"
)

// Chains.
const (
	ChainEVM = "evm"
	ChainSOL = "sol"
	ChainAPT = "apt"
	ChainSUI = "sui"
)

// ErrUnsupportedChain is returned for unknown chain identifiers.
var ErrUnsupportedChain = errors.New("unsupported chain")

// Verify dispatches to the per-chain verifier and returns the recovered address.
// publicKey is required for sol/apt/sui (the EVM address is recovered from the
// signature itself).
func Verify(chain, message, signature, publicKey string) (string, error) {
	switch chain {
	case ChainEVM:
		return VerifyEVM(message, signature)
	case ChainSOL:
		return VerifySolana(message, signature, publicKey)
	case ChainAPT:
		return VerifyAptos(message, signature, publicKey)
	case ChainSUI:
		return VerifySui(message, signature)
	default:
		return "", ErrUnsupportedChain
	}
}

// VerifyEVM recovers the signer address from an EIP-191 personal_sign signature.
func VerifyEVM(message, signature string) (string, error) {
	sig, err := hex.DecodeString(strings.TrimPrefix(signature, "0x"))
	if err != nil {
		return "", fmt.Errorf("invalid signature hex: %w", err)
	}
	if len(sig) != 65 {
		return "", fmt.Errorf("invalid signature length: expected 65, got %d", len(sig))
	}
	if sig[64] == 27 || sig[64] == 28 {
		sig[64] -= 27
	}
	prefixed := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(message), message)
	hash := ethcrypto.Keccak256Hash([]byte(prefixed))
	pub, err := ethcrypto.SigToPub(hash.Bytes(), sig)
	if err != nil {
		return "", fmt.Errorf("recover pubkey: %w", err)
	}
	return ethcrypto.PubkeyToAddress(*pub).Hex(), nil
}

// VerifySolana checks an ed25519 signature over the message and returns the
// base58 public key (which is the Solana address).
func VerifySolana(message, signature, publicKey string) (string, error) {
	pub, err := base58.Decode(publicKey)
	if err != nil {
		return "", fmt.Errorf("invalid solana public key: %w", err)
	}
	if len(pub) != ed25519.PublicKeySize {
		return "", fmt.Errorf("invalid solana public key length")
	}
	sig, err := decodeFlexible(signature)
	if err != nil {
		return "", fmt.Errorf("invalid solana signature: %w", err)
	}
	if !ed25519.Verify(pub, []byte(message), sig) {
		return "", errors.New("solana signature verification failed")
	}
	return publicKey, nil
}

// VerifyAptos checks an ed25519 signature and derives the Aptos address as
// sha3-256(pubkey || 0x00).
func VerifyAptos(message, signature, publicKey string) (string, error) {
	pub, err := decodeFlexible(publicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return "", fmt.Errorf("invalid aptos public key")
	}
	sig, err := decodeFlexible(signature)
	if err != nil {
		return "", fmt.Errorf("invalid aptos signature: %w", err)
	}
	if !ed25519.Verify(pub, []byte(message), sig) {
		return "", errors.New("aptos signature verification failed")
	}
	h := sha3.New256()
	h.Write(pub)
	h.Write([]byte{0x00})
	return "0x" + hex.EncodeToString(h.Sum(nil)), nil
}

// VerifySui checks an ed25519 Sui signature (base64: scheme||sig||pubkey) over
// the message and derives the Sui address as blake2b-256(0x00 || pubkey).
func VerifySui(message, signature string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return "", fmt.Errorf("invalid sui signature base64: %w", err)
	}
	// serialized = flag(1) || signature(64) || public_key(32)
	if len(raw) != 1+64+ed25519.PublicKeySize {
		return "", fmt.Errorf("invalid sui signature length: %d", len(raw))
	}
	sig := raw[1:65]
	pub := raw[65:]
	if !ed25519.Verify(pub, []byte(message), sig) {
		return "", errors.New("sui signature verification failed")
	}
	h := blake2b.New256()
	h.Write([]byte{0x00}) // ed25519 scheme flag
	h.Write(pub)
	return "0x" + hex.EncodeToString(h.Sum(nil)), nil
}

// decodeFlexible tries hex (with optional 0x), then base58, then base64.
func decodeFlexible(s string) ([]byte, error) {
	if b, err := hex.DecodeString(strings.TrimPrefix(s, "0x")); err == nil {
		return b, nil
	}
	if b, err := base58.Decode(s); err == nil {
		return b, nil
	}
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return nil, errors.New("unrecognized encoding")
}
