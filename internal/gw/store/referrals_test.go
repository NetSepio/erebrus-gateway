package store

import (
	"strings"
	"testing"
)

func TestGenReferralCode(t *testing.T) {
	seen := map[string]int{}
	for i := 0; i < 500; i++ {
		code, err := genReferralCode(8)
		if err != nil {
			t.Fatalf("genReferralCode: %v", err)
		}
		if len(code) != 8 {
			t.Fatalf("len(%q) = %d, want 8", code, len(code))
		}
		for _, r := range code {
			if !strings.ContainsRune(referralAlphabet, r) {
				t.Fatalf("char %q not in alphabet (code %q)", r, code)
			}
		}
		seen[code]++
	}
	// 500 draws from 32^8 should essentially never collide.
	if len(seen) != 500 {
		t.Fatalf("expected 500 distinct codes, got %d", len(seen))
	}
	// No ambiguous characters in the alphabet.
	for _, bad := range []string{"I", "L", "O", "U"} {
		if strings.Contains(referralAlphabet, bad) {
			t.Fatalf("alphabet should not contain ambiguous %q", bad)
		}
	}
}
