package identity

import (
	"testing"

	"github.com/NetSepio/gateway/internal/gw/token"
)

func TestPasetoKeyFromMnemonicGolden(t *testing.T) {
	const mnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	got, err := PasetoKeyFromMnemonic(mnemonic)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	// Frozen — changing PasetoDerivationPath or algorithm must update this value deliberately.
	const want = "d7fbb803d1572d0e9f56fb79f105c529c9efb858cb25d62933f56cac221be4407d803c6d30908e63fe99e16ac6b1f6fe68d690ee08a489eb6a44f066fb264707"
	if got != want {
		t.Fatalf("golden mismatch:\ngot  %s\nwant %s", got, want)
	}
	if len(got) != 128 {
		t.Fatalf("expected 128 hex chars (64-byte ed25519 key), got %d", len(got))
	}
	if _, err := token.New(got, "Erebrus", 0); err != nil {
		t.Fatalf("derived key invalid for PASETO: %v", err)
	}
	again, err := PasetoKeyFromMnemonic(mnemonic)
	if err != nil {
		t.Fatalf("derive again: %v", err)
	}
	if got != again {
		t.Fatalf("derivation not deterministic")
	}
}

func TestResolvePasetoKeyPrefersExplicit(t *testing.T) {
	const explicit = "7f25acf638276a6e5de68e21a9f75f4d912b5cd5d2096d55547476ccca93129182245fe30373eaff574dd28560fb87855b29412472ea664990e128be9141759a"
	key, fromMnemonic, err := ResolvePasetoKey(explicit, "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if fromMnemonic {
		t.Fatal("expected explicit key, not mnemonic derivation")
	}
	if key != explicit {
		t.Fatalf("got %q want %q", key, explicit)
	}
}