package api

import (
	"strings"
	"testing"

	"github.com/NetSepio/gateway/internal/store"
)

func TestDropGatewayLinks(t *testing.T) {
	cid := "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG"
	sources := []store.DropObjectSource{
		{NodeID: "origin", PublicGatewayURL: "https://a.gw.erebrus.io/"}, // trailing slash trimmed
		{NodeID: "b", PublicGatewayURL: "https://b.gw.erebrus.io"},
		{NodeID: "c", PublicGatewayURL: ""},                          // skipped (no base)
		{NodeID: "dup", PublicGatewayURL: "https://a.gw.erebrus.io"}, // dedup vs first
	}
	primary, all := dropGatewayLinks(sources, cid)
	want := "https://a.gw.erebrus.io/ipfs/" + cid
	if primary != want {
		t.Errorf("primary = %q want %q", primary, want)
	}
	if len(all) != 2 {
		t.Fatalf("all = %v (want 2 deduped, non-empty)", all)
	}
	if all[1] != "https://b.gw.erebrus.io/ipfs/"+cid {
		t.Errorf("all[1] = %q", all[1])
	}
	// No sources → empty.
	if p, a := dropGatewayLinks(nil, cid); p != "" || len(a) != 0 {
		t.Errorf("empty sources should yield no links, got %q %v", p, a)
	}
}

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
