package api

import "testing"

func TestClampInt(t *testing.T) {
	cases := []struct {
		in          string
		def, lo, hi int
		want        int
	}{
		{"", 50, 1, 100, 50},     // empty -> default
		{"abc", 50, 1, 100, 50},  // invalid -> default
		{"10", 50, 1, 100, 10},   // in range
		{"0", 50, 1, 100, 1},     // below lo -> lo
		{"999", 50, 1, 100, 100}, // above hi -> hi
		{"-5", 0, 0, 100, 0},     // negative -> lo
	}
	for _, tc := range cases {
		if got := clampInt(tc.in, tc.def, tc.lo, tc.hi); got != tc.want {
			t.Errorf("clampInt(%q, %d, %d, %d) = %d, want %d", tc.in, tc.def, tc.lo, tc.hi, got, tc.want)
		}
	}
}
