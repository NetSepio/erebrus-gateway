package metrics

import "testing"

func TestNormalizeRegion(t *testing.T) {
	cases := []struct{ in, want string }{
		{"sg", "sg"},
		{"EU", "eu"},
		{"", "unknown"},
		{"ap-southeast", "unknown"},
	}
	for _, tc := range cases {
		if got := NormalizeRegion(tc.in); got != tc.want {
			t.Errorf("NormalizeRegion(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizePlatform(t *testing.T) {
	cases := []struct{ in, want string }{
		{"android", "android"},
		{"IOS", "ios"},
		{"webapp", "webapp"},
		{"desktop", "unknown"},
	}
	for _, tc := range cases {
		if got := NormalizePlatform(tc.in); got != tc.want {
			t.Errorf("NormalizePlatform(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeStatus(t *testing.T) {
	cases := []struct{ in, want string }{
		{"success", "success"},
		{"FAILED", "failed"},
		{"attempted", "attempted"},
		{"pending", "unknown"},
	}
	for _, tc := range cases {
		if got := NormalizeStatus(tc.in); got != tc.want {
			t.Errorf("NormalizeStatus(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeEvent(t *testing.T) {
	cases := []struct{ in, want string }{
		{"vpn_connect_success", "vpn_connect_success"},
		{"VPN_DISCONNECT", "vpn_disconnect"},
		{"custom_event", "unknown"},
	}
	for _, tc := range cases {
		if got := NormalizeEvent(tc.in); got != tc.want {
			t.Errorf("NormalizeEvent(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}