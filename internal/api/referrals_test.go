package api

import "testing"

func TestTruncWallet(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"short", "short"},
		{"0x1234567890", "0x1234567890"},  // 12 chars: unchanged
		{"0x1234567890ab", "0x1234…90ab"}, // 14 chars: truncated
		{"8bDuPx3kSo1anaWa11etAddrjv5d", "8bDuPx…jv5d"},
	}
	for _, tc := range cases {
		if got := truncWallet(tc.in); got != tc.want {
			t.Errorf("truncWallet(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
