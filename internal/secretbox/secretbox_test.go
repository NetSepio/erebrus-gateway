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
	got, err := b.Open(blob)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if got != secret {
		t.Fatalf("round trip = %q, want %q", got, secret)
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
