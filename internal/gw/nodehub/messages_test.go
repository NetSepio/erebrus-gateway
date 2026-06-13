package nodehub

import (
	"encoding/json"
	"testing"
)

// Canonical hello frame from docs/v2/ws-protocol.md. If this stops parsing
// field-for-field, the contract has drifted.
const canonicalHello = `{
  "type": "hello",
  "data": {
    "node_id": "9d3b0d5e-3a3c-4b9e-9a31-0c5a9f0e6c11",
    "version": "2.0.0",
    "identity": {
      "peer_id": "12D3KooWQYhTNQdmr3ArTeo5gCtJ8m1bbb73Bb4Q4xxK9zMrf1nK",
      "did": "did:erebrus:12D3KooWQYhTNQdmr3ArTeo5gCtJ8m1bbb73Bb4Q4xxK9zMrf1nK",
      "ip_hash": "f1820f54e0e51b8a1a47b0ec96265d6021b3a0b6c6c61563b1d62fa4a4b0d3c2"
    },
    "spec": { "cpu": "4 vCPU", "mem_mb": 8192, "region": "SG", "ip": "203.0.113.10" },
    "capabilities": { "app_hosting": false, "wildcard_domain": "" },
    "endpoints": {
      "wireguard":     { "port": 51820, "public_key": "wOLuwnTGzkkCC1WiV2t5HpJ56FftZyXTK0WnWxSDFkI=" },
      "vless_reality": { "port": 8443,  "public_key": "SRYxyiZ1Tr3w0aV3PXAhd1NSjpvm8wOCnnlLWWBd7Vc", "short_ids": ["6ba85179e30d4fc2"], "sni": "www.microsoft.com" },
      "hysteria2":     { "port": 4443,  "obfs": "" }
    }
  }
}`

func TestParseCanonicalHello(t *testing.T) {
	var env Envelope
	if err := json.Unmarshal([]byte(canonicalHello), &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env.Type != TypeHello {
		t.Fatalf("type = %q, want %q", env.Type, TypeHello)
	}
	var h Hello
	if err := json.Unmarshal(env.Data, &h); err != nil {
		t.Fatalf("unmarshal hello: %v", err)
	}
	if h.NodeID != "9d3b0d5e-3a3c-4b9e-9a31-0c5a9f0e6c11" {
		t.Errorf("node_id = %q", h.NodeID)
	}
	if h.Identity.DID != "did:erebrus:"+"12D3KooWQYhTNQdmr3ArTeo5gCtJ8m1bbb73Bb4Q4xxK9zMrf1nK" {
		t.Errorf("did = %q", h.Identity.DID)
	}
	if h.Spec.MemMB != 8192 || h.Spec.Region != "SG" {
		t.Errorf("spec = %+v", h.Spec)
	}
	if h.Endpoints.WireGuard.Port != 51820 || h.Endpoints.Hysteria2.Port != 4443 {
		t.Errorf("endpoints ports wrong: %+v", h.Endpoints)
	}
	if len(h.Endpoints.VLESSReality.ShortIDs) != 1 || h.Endpoints.VLESSReality.SNI != "www.microsoft.com" {
		t.Errorf("vless endpoint = %+v", h.Endpoints.VLESSReality)
	}
}

func TestHeartbeatAndUsageRoundTrip(t *testing.T) {
	hb := Heartbeat{
		TS: 1765584000, Status: "online",
		Load:      Load{WGPeers: 42, ProxySessions: 7, CPUPct: 23.5, MemPct: 41.2, RxBytes: 123456789, TxBytes: 987654321},
		Speedtest: Speedtest{DownloadMbps: 940.2, UploadMbps: 870.1, LatencyMs: 3.2, MeasuredAt: 1765580400},
		Versions:  map[string]string{"node": "2.0.0", "singbox": "1.11.4"},
	}
	frame, err := wrap(TypeHeartbeat, hb)
	if err != nil {
		t.Fatalf("wrap: %v", err)
	}
	var env Envelope
	if err := json.Unmarshal(frame, &env); err != nil || env.Type != TypeHeartbeat {
		t.Fatalf("envelope: %v type=%s", err, env.Type)
	}
	var got Heartbeat
	if err := json.Unmarshal(env.Data, &got); err != nil {
		t.Fatalf("unmarshal heartbeat: %v", err)
	}
	if got.Load.RxBytes != 123456789 || got.Load.TxBytes != 987654321 {
		t.Errorf("byte counters lost: %+v", got.Load)
	}

	ur := UsageReport{TS: 1765584000, Peers: []PeerUsage{{PeerID: "c0a4f1de", RxBytesDelta: 1048576, TxBytesDelta: 8388608, LastHandshake: 1765583970}}}
	frame, _ = wrap(TypeUsageReport, ur)
	_ = json.Unmarshal(frame, &env)
	var gotUR UsageReport
	if err := json.Unmarshal(env.Data, &gotUR); err != nil {
		t.Fatalf("unmarshal usage: %v", err)
	}
	if len(gotUR.Peers) != 1 || gotUR.Peers[0].TxBytesDelta != 8388608 {
		t.Errorf("usage peers wrong: %+v", gotUR.Peers)
	}
}
