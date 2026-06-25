package store

import "testing"

func TestTierForXP(t *testing.T) {
	th := defaultTierThresholds // 0, 100, 500, 2000, 10000
	cases := []struct {
		xp   int64
		want int
	}{
		{0, 0}, {1, 0}, {99, 0},
		{100, 1}, {499, 1},
		{500, 2}, {1999, 2},
		{2000, 3}, {9999, 3},
		{10000, 4}, {50000, 4},
	}
	for _, tc := range cases {
		if got := tierForXP(tc.xp, th); got != tc.want {
			t.Errorf("tierForXP(%d) = %d, want %d", tc.xp, got, tc.want)
		}
	}
}

func TestTierThresholdsDefault(t *testing.T) {
	var s Store // no override set
	got := s.TierThresholds()
	if len(got) != 5 || got[0] != 0 || got[4] != 10000 {
		t.Fatalf("default thresholds = %v", got)
	}
	s.SetTierThresholds([]int64{0, 50})
	if g := s.TierThresholds(); len(g) != 2 || g[1] != 50 {
		t.Fatalf("override thresholds = %v", g)
	}
}
