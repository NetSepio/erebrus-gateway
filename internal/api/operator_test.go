package api

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	def := 24 * time.Hour
	max := 90 * 24 * time.Hour
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"", def},        // empty -> default
		{"garbage", def}, // invalid -> default
		{"0s", def},      // non-positive -> default
		{"-5h", def},     // negative -> default
		{"6h", 6 * time.Hour},
		{"720h", 720 * time.Hour},
		{"100000h", max}, // over max -> clamped
	}
	for _, tc := range cases {
		if got := parseDuration(tc.in, def, max); got != tc.want {
			t.Errorf("parseDuration(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
}
