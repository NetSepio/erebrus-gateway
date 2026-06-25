// Package identity derives gateway-local secrets from a BIP39 mnemonic.
package identity

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"strings"

	bip32 "github.com/tyler-smith/go-bip32"
	bip39 "github.com/tyler-smith/go-bip39"
)

// PasetoDerivationPath is the frozen HD path for gateway PASETO signing.
// Account 1 keeps this key separate from a node's wallet at m/44'/501'/0'/0'.
const PasetoDerivationPath = `m/44'/501'/1'/0'`

// PasetoKeyFromMnemonic derives a stable 64-byte Ed25519 private key (hex)
// for PASETO signing from the gateway mnemonic.
func PasetoKeyFromMnemonic(mnemonic string) (string, error) {
	mnemonic = strings.TrimSpace(mnemonic)
	if mnemonic == "" {
		return "", fmt.Errorf("mnemonic is empty")
	}
	if !bip39.IsMnemonicValid(mnemonic) {
		return "", fmt.Errorf("invalid mnemonic")
	}
	seed := bip39.NewSeed(mnemonic, "")
	master, err := bip32.NewMasterKey(seed)
	if err != nil {
		return "", fmt.Errorf("master key: %w", err)
	}
	child := master
	for _, idx := range []uint32{
		bip32.FirstHardenedChild + 44,
		bip32.FirstHardenedChild + 501,
		bip32.FirstHardenedChild + 1,
		0,
	} {
		child, err = child.NewChildKey(idx)
		if err != nil {
			return "", fmt.Errorf("derive %s: %w", PasetoDerivationPath, err)
		}
	}
	if len(child.Key) < ed25519.SeedSize {
		return "", fmt.Errorf("derived seed too short")
	}
	priv := ed25519.NewKeyFromSeed(child.Key[:ed25519.SeedSize])
	return hex.EncodeToString(priv), nil
}

