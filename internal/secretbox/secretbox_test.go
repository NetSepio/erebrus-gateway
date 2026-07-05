package secretbox

import "testing"

func TestSealOpenRoundTrip(t *testing.T) {
	b := New("legal winner thank year wave sausage worth useful legal winner thank yellow")
	if !b.Enabled() {
		t.Fatal("box should be enabled with a passphrase")
	}
	const secret = "s3cr3t-adguard-pass"
	blob, err := b.Seal(secret)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if string(blob) == secret {
		t.Fatal("ciphertext must not equal plaintext")
	}
	got, legacy, err := b.OpenBlob(blob)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if legacy {
		t.Fatal("fresh seal must use HKDF key")
	}
	if got != secret {
		t.Fatalf("round trip = %q, want %q", got, secret)
	}
}

func TestOpenLegacyCiphertext(t *testing.T) {
	b := New("passphrase")
	const secret = "legacy-adguard-pass"
	blob, err := b.sealLegacy(secret)
	if err != nil {
		t.Fatalf("legacy seal: %v", err)
	}
	got, legacy, err := b.OpenBlob(blob)
	if err != nil {
		t.Fatalf("open legacy: %v", err)
	}
	if !legacy {
		t.Fatal("expected legacy KDF flag")
	}
	if got != secret {
		t.Fatalf("legacy round trip = %q, want %q", got, secret)
	}
	resealed, err := b.Seal(got)
	if err != nil {
		t.Fatalf("reseal: %v", err)
	}
	got2, legacy2, err := b.OpenBlob(resealed)
	if err != nil || legacy2 || got2 != secret {
		t.Fatalf("resealed blob should decrypt with HKDF: got=%q legacy=%v err=%v", got2, legacy2, err)
	}
}

func TestHKDFDiffersFromLegacy(t *testing.T) {
	pass := "mnemonic-phrase"
	hkdfKey, err := deriveKeyHKDF(pass)
	if err != nil {
		t.Fatal(err)
	}
	legacyKey := deriveKeyLegacy(pass)
	if hkdfKey == legacyKey {
		t.Fatal("HKDF and legacy keys must differ")
	}
}

func TestOpenRejectsTamper(t *testing.T) {
	b := New("passphrase")
	blob, _ := b.Seal("hello")
	blob[len(blob)-1] ^= 0xff // flip a byte
	if _, err := b.Open(blob); err == nil {
		t.Fatal("tampered ciphertext should not decrypt")
	}
}

func TestDifferentPassphraseDifferentKey(t *testing.T) {
	blob, _ := New("one").Seal("x")
	if _, err := New("two").Open(blob); err == nil {
		t.Fatal("a different passphrase must not decrypt")
	}
}

func TestDisabledWhenEmpty(t *testing.T) {
	if New("").Enabled() {
		t.Fatal("empty passphrase => disabled")
	}
}