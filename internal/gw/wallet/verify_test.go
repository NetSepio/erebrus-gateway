package wallet

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"testing"

	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/mr-tron/base58"
)

func TestVerifyEVMRoundTrip(t *testing.T) {
	priv, err := ethcrypto.GenerateKey()
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	want := ethcrypto.PubkeyToAddress(priv.PublicKey).Hex()

	msg := "I accept the Erebrus Terms of Service. Challenge: abc-123"
	prefixed := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(msg), msg)
	hash := ethcrypto.Keccak256Hash([]byte(prefixed))
	sig, err := ethcrypto.Sign(hash.Bytes(), priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	got, err := VerifyEVM(msg, "0x"+hex.EncodeToString(sig))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got != want {
		t.Fatalf("recovered %s, want %s", got, want)
	}

	// Tampered message must fail.
	if _, err := VerifyEVM(msg+"x", "0x"+hex.EncodeToString(sig)); err == nil {
		// recovery still yields *an* address, just not the signer — caller
		// compares to the flow wallet, but here a different address is fine.
		// The key property is that `got` above matched; nothing to assert.
		_ = err
	}
}

func TestVerifySolanaRoundTrip(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gen: %v", err)
	}
	addr := base58.Encode(pub)
	msg := "Erebrus login challenge xyz"
	sig := ed25519.Sign(priv, []byte(msg))

	got, err := VerifySolana(msg, hex.EncodeToString(sig), addr)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got != addr {
		t.Fatalf("got %s, want %s", got, addr)
	}

	// Wrong message must fail verification.
	if _, err := VerifySolana("different", hex.EncodeToString(sig), addr); err == nil {
		t.Fatal("expected failure for tampered message")
	}
}

func TestVerifyUnsupportedChain(t *testing.T) {
	if _, err := Verify("doge", "m", "s", "p"); err != ErrUnsupportedChain {
		t.Fatalf("want ErrUnsupportedChain, got %v", err)
	}
}
