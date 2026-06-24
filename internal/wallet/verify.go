// Package wallet verifies wallet-signature logins across the chains Erebrus
// supports (EVM and Solana). Each Verify* returns the wallet address
// recovered/derived from the signature; the caller compares it (case-insensitive)
// to the address that requested the flow id. No database access here.
//
// v2 deliberately supports EVM + Solana only (Reown AppKit mediates both, plus
// social/email logins that resolve to an embedded wallet). Aptos and Sui were
// dropped in S2.
package wallet

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/mr-tron/base58"
)

// Chains.
const (
	ChainEVM = "evm"
	ChainSOL = "sol"
)

// ErrUnsupportedChain is returned for unknown chain identifiers.
var ErrUnsupportedChain = errors.New("unsupported chain")

// Verify dispatches to the per-chain verifier and returns the recovered address.
// publicKey is required for sol (the EVM address is recovered from the signature
// itself).
func Verify(chain, message, signature, publicKey string) (string, error) {
	switch chain {
	case ChainEVM:
		return VerifyEVM(message, signature)
	case ChainSOL:
		return VerifySolana(message, signature, publicKey)
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
