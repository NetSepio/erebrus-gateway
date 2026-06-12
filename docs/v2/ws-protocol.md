# Erebrus v2 — Node ↔ Gateway WebSocket Protocol

**Status: FROZEN (v2.0).** This document is the single source of truth for the
node↔gateway control plane. The Go structs in
`erebrus/internal/gatewayclient/messages.go` and
`erebrus-gateway/internal/nodehub/messages.go` are hand-mirrored copies of these
schemas; both repos carry contract tests that marshal the canonical examples
below and compare them field-for-field. Change this file first, then both repos.

## Transport

- Endpoint: `wss://<gateway>/api/v2/nodes/ws`
- Auth: node-scoped PASETO bearer token in the `Authorization` header of the
  upgrade request. Tokens are issued by the HTTPS registration flow
  (`POST /api/v2/nodes/register`, see `gateway-api.openapi.yaml`): the gateway
  returns a challenge, the node signs it with its wallet key (derived from its
  mnemonic), and the gateway responds with a PASETO carrying
  `{node_id, peer_id, role: "node"}`.
- Encoding: one JSON object per WebSocket text frame. Every frame has the
  envelope `{"type": "<message-type>", "data": {...}}`.
- Direction: the node dials the gateway. The node reconnects with exponential
  backoff (1s base, factor 2, max 60s, jitter ±20%). On reconnect the node
  re-sends `hello`.
- Liveness: heartbeat every **30s** (value handed down in `hello_ack`; the node
  must honor it). The gateway marks a node `offline` after **3 missed
  heartbeats (90s)** and `online` again on the next heartbeat or `hello`.

## Message types

### `hello` (node → gateway, on every (re)connect)

```json
{
  "type": "hello",
  "data": {
    "node_id": "9d3b0d5e-3a3c-4b9e-9a31-0c5a9f0e6c11",
    "version": "2.0.0",
    "identity": {
      "peer_id": "12D3KooWQYhTNQdmr3ArTeo5gCtJ8m1bbb73Bb4Q4xxK9zMrf1nK",
      "did": "did:erebrus:12D3KooWQYhTNQdmr3ArTeo5gCtJ8m1bbb73Bb4Q4xxK9zMrf1nK",
      "ip_hash": "f1820f54e0e51b8a1a47b0ec96265d6021b3a0b6c6c61563b1d62fa4a4b0d3c2"
    },
    "spec": {
      "cpu": "4 vCPU",
      "mem_mb": 8192,
      "region": "SG",
      "ip": "203.0.113.10"
    },
    "capabilities": {
      "app_hosting": false,
      "wildcard_domain": ""
    },
    "endpoints": {
      "wireguard":     { "port": 51820, "public_key": "wOLuwnTGzkkCC1WiV2t5HpJ56FftZyXTK0WnWxSDFkI=" },
      "vless_reality": { "port": 8443,  "public_key": "SRYxyiZ1Tr3w0aV3PXAhd1NSjpvm8wOCnnlLWWBd7Vc", "short_ids": ["6ba85179e30d4fc2"], "sni": "www.microsoft.com" },
      "hysteria2":     { "port": 4443,  "obfs": "" }
    }
  }
}
```

- `ip_hash` = lowercase hex SHA3-256 of the node's public IPv4 dotted-quad
  string (see `identity.md`). The raw `spec.ip` is sent over the authenticated
  channel for gateway operational use (client endpoint construction); only
  `ip_hash` may ever be published externally (future on-chain registration).
- `capabilities.wildcard_domain` is set only when `app_hosting` is true
  (Phase 5), e.g. `"*.node-sg-1.erebrus.network"`.
- `endpoints.hysteria2.obfs` is `""` (no obfuscation) or `"salamander"`.

### `hello_ack` (gateway → node)

```json
{ "type": "hello_ack", "data": { "heartbeat_interval_sec": 30 } }
```

### `heartbeat` (node → gateway, every `heartbeat_interval_sec`)

```json
{
  "type": "heartbeat",
  "data": {
    "ts": 1765584000,
    "status": "online",
    "load": {
      "wg_peers": 42,
      "proxy_sessions": 7,
      "cpu_pct": 23.5,
      "mem_pct": 41.2,
      "rx_bytes": 123456789,
      "tx_bytes": 987654321
    },
    "speedtest": {
      "download_mbps": 940.2,
      "upload_mbps": 870.1,
      "latency_ms": 3.2,
      "measured_at": 1765580400
    },
    "versions": { "node": "2.0.0", "singbox": "1.11.4" }
  }
}
```

- `status` ∈ `online | draining` (`draining` after a `drain` command:
  serving existing peers, rejecting new provisioning).
- `rx_bytes`/`tx_bytes` are cumulative interface counters since node start
  (gateway computes deltas; counters reset on restart, gateway must treat a
  decrease as a reset).
- `speedtest` is refreshed by the node at most every 6h; `measured_at` says
  how stale it is.

### `usage_report` (node → gateway, every 60s)

```json
{
  "type": "usage_report",
  "data": {
    "ts": 1765584000,
    "peers": [
      {
        "peer_id": "c0a4f1de-77a2-4b6e-8b13-1de52a7a4e10",
        "rx_bytes_delta": 1048576,
        "tx_bytes_delta": 8388608,
        "last_handshake": 1765583970
      }
    ]
  }
}
```

- `peer_id` is the gateway-issued VPN client UUID (the same `{id}` used in
  `PUT /api/v2/peers/{id}` on the node).
- Deltas are bytes since the previous `usage_report` (WireGuard transfer
  counters + sing-box per-user traffic, summed across protocols). Peers with
  zero traffic in the window are omitted. `last_handshake` is 0 if the peer
  has never completed a WireGuard handshake (e.g. stealth-only usage).
- Delivery is at-least-once from the node's perspective but the node does NOT
  buffer across restarts; metering is best-effort by design.

### `command` (gateway → node) / `command_result` (node → gateway)

```json
{ "type": "command", "data": { "action": "drain", "request_id": "req-7f3a", "args": {} } }
```

```json
{ "type": "command_result", "data": { "request_id": "req-7f3a", "ok": true, "error": "" } }
```

Actions (v2.0):

| action | args | effect |
|---|---|---|
| `drain` | `{}` | node rejects new peer provisioning (HTTP 409), keeps serving; status becomes `draining` |
| `undrain` | `{}` | leave draining state |
| `rotate_reality` | `{}` | regenerate REALITY short-ids (keypair kept), rebuild sing-box; node re-sends `hello` with new endpoint data |
| `resync_peers` | `{"peer_ids": ["...", "..."]}` | authoritative list of active peer ids from the gateway; node deletes local peers not in the list and reports peers it has that are missing (in `command_result.error` as a JSON detail, `ok=true`) |
| `sync_apps` | `{"apps": [...]}` (Phase 5) | reconcile hosted-apps table; schema frozen in Phase 5 addendum |

Unknown actions → `command_result` with `ok=false, error="unknown action"`.
The node must answer every `command` within 30s or the gateway logs a timeout.

## Versioning

The envelope is open for additive change only: new optional fields and new
message types/actions are allowed within v2; renames/removals require a new
WS path (`/api/v3/nodes/ws`). Both sides must ignore unknown fields
(`json.Unmarshal` default) and unknown message types (log + drop).
