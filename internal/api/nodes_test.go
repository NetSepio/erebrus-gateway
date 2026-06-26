package api

import (
	"encoding/json"
	"testing"
)

func TestEnrichEndpointsForDiscovery(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  json.RawMessage
		ip   string
		want map[string]any
	}{
		{
			name: "injects host when wireguard host missing",
			raw:  json.RawMessage(`{"wireguard":{"port":51820,"public_key":"abc"}}`),
			ip:   "203.0.113.10",
			want: map[string]any{
				"wireguard": map[string]any{"port": float64(51820), "public_key": "abc", "host": "203.0.113.10"},
			},
		},
		{
			name: "preserves existing wireguard host",
			raw:  json.RawMessage(`{"wireguard":{"host":"10.0.0.1","port":51820}}`),
			ip:   "203.0.113.10",
			want: map[string]any{
				"wireguard": map[string]any{"host": "10.0.0.1", "port": float64(51820)},
			},
		},
		{
			name: "creates wireguard host when endpoints empty",
			raw:  nil,
			ip:   "203.0.113.10",
			want: map[string]any{
				"wireguard": map[string]any{"host": "203.0.113.10"},
			},
		},
		{
			name: "no ip leaves payload unchanged",
			raw:  json.RawMessage(`{"wireguard":{"port":51820}}`),
			ip:   "",
			want: map[string]any{
				"wireguard": map[string]any{"port": float64(51820)},
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := enrichEndpointsForDiscovery(tc.raw, tc.ip)
			var m map[string]any
			if err := json.Unmarshal(got, &m); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if string(mustJSON(m)) != string(mustJSON(tc.want)) {
				t.Fatalf("got %s, want %s", got, mustJSON(tc.want))
			}
		})
	}
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
