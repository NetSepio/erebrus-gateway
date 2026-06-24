package api

import "testing"

func TestNormalizeEmail(t *testing.T) {
	cases := []struct {
		in    string
		want  string
		valid bool
	}{
		{"User@Example.COM", "user@example.com", true},
		{"  a@b.co  ", "a@b.co", true},
		{"plainaddress", "", false},
		{"@no-local.com", "", false},
		{"no-domain@", "", false},
		{"no-dot@localhost", "", false},
		{"has space@x.com", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got, valid := normalizeEmail(tc.in)
		if valid != tc.valid {
			t.Errorf("normalizeEmail(%q) valid=%v, want %v", tc.in, valid, tc.valid)
			continue
		}
		if valid && got != tc.want {
			t.Errorf("normalizeEmail(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestGenerateOTPShape(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		code, err := generateOTP()
		if err != nil {
			t.Fatalf("generateOTP: %v", err)
		}
		if len(code) != 6 {
			t.Fatalf("len(%q) = %d, want 6", code, len(code))
		}
		for _, r := range code {
			if r < '0' || r > '9' {
				t.Fatalf("non-digit in %q", code)
			}
		}
		seen[code] = true
	}
	// Not a strict randomness test, but 200 draws collapsing to a handful of
	// values would indicate a broken generator.
	if len(seen) < 50 {
		t.Fatalf("only %d distinct codes in 200 draws — suspicious", len(seen))
	}
}

func TestHashOTPDeterministicAndHex(t *testing.T) {
	a := hashOTP("123456")
	b := hashOTP("123456")
	if a != b {
		t.Fatal("hashOTP not deterministic")
	}
	if a == hashOTP("123457") {
		t.Fatal("different codes hashed to same value")
	}
	if len(a) != 64 { // sha256 hex
		t.Fatalf("hash len = %d, want 64", len(a))
	}
}
