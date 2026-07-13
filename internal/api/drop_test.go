package api

import (
	"strings"
	"testing"
)

func TestValidCID(t *testing.T) {
	cases := map[string]bool{
		"QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG":            true,
		"bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbz": true,
		"":                     false,
		"short":                false,
		"has/slash/in/it/xxxx": false,
		"../../etc/passwd":     false,
		"has space in it here": false,
	}
	for in, want := range cases {
		if got := validCID(in); got != want {
			t.Errorf("validCID(%q)=%v want %v", in, got, want)
		}
	}
}

func TestContentDispositionSanitizes(t *testing.T) {
	// Control chars must not survive into the header (no header injection).
	got := contentDisposition("re\"port\r\n.pdf")
	if strings.ContainsAny(got, "\r\n") {
		t.Errorf("disposition leaked control char: %q", got)
	}
	if !strings.Contains(got, `filename="`) || !strings.Contains(got, "filename*=UTF-8''") {
		t.Errorf("disposition missing ascii/utf-8 forms: %q", got)
	}
	// Empty names fall back to a safe default.
	if d := contentDisposition(""); !strings.Contains(d, "download") {
		t.Errorf("expected fallback filename, got %q", d)
	}
}

func TestNormalizeDropScope(t *testing.T) {
	for input, want := range map[string]string{
		"":            "public",
		"public":      "public",
		"private":     "private_org",
		"private_org": "private_org",
	} {
		got, ok := normalizeDropScope(input)
		if !ok || got != want {
			t.Fatalf("normalizeDropScope(%q) = %q, %v", input, got, ok)
		}
	}
	if _, ok := normalizeDropScope("invalid"); ok {
		t.Fatal("invalid scope accepted")
	}
}
