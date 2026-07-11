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
  (`POST /api/v2/nodes/register`, see `docs/gateway-api.openapi.yaml`): gated by
  a scoped org **registration token** (`registration_token` in JSON). On the node, set
  `EREBRUS_NODE_REGISTRATION_TOKEN` (replaces
  `EREBRUS_ORG_ENROLLMENT_SECRET`). Tokens are minted via
  `POST /api/v2/orgs/{org_id}/node-registration-tokens`.
  The gateway returns a machine challenge; the node signs it with its
  mnemonic-derived wallet key (not the human EULA auth flow). The gateway
  responds with `{ node_token (PASETO role=node), node_id (= peer_id), peer_id,
  node_key, gateway_public_key }`.
- Optional REST heartbeat: `POST /api/v2/nodes/{peer_id}/heartbeat` with the same
  node PASETO (updates runtime `nodes` row and `org_nodes.last_seen_at` when linked).
  WS heartbeats also touch `org_nodes.last_seen_at`.
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
    "node_id": "12D3KooWQYhTNQdmr3ArTeo5gCtJ8m1bbb73Bb4Q4xxK9zMrf1nK",
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
      "zone": "",
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
| `sync_firewall` | gateway policy payload (`org_id`, `node_id`, `service_kind`, `rules`, `upstreams`, `licensed`) | node applies rules to Sentinel (Unbound) or clears Shield (AdGuard) cache |
| `restart_firewall` | `{}` | reload Sentinel Unbound or restart Shield |
| `reset_firewall_credentials` | `{}` | Shield credential reset hook (operator re-opens AdGuard setup) |
| `set_firewall_credentials` | `{"admin_user","admin_password"}` | apply a new AdGuard admin password (Shield) |

Optional additive fields (v2.0+):

- `hello.deployment_profile` — `standard` | `shield` | `sentinel`
- `hello.services` / `heartbeat.services` — map of service health (`vpn`, `community_firewall`, `erebrus_firewall`, `drop`)

Unknown actions → `command_result` with `ok=false, error="unknown action"`.
The node must answer every `command` within 30s or the gateway logs a timeout.

## Drop (Kubo storage) — additive fields (v2.0+)

Drop is an **independent optional service**, not a deployment profile. A node
that runs a Kubo daemon advertises Drop through additive `hello`/`heartbeat`
fields. All fields are optional: nodes that do not run Drop omit them and the
gateway leaves the corresponding pointers `nil` (older nodes remain fully
compatible). Standard, Shield, and Sentinel profiles may all run Drop.

### `hello.capabilities.drop` (node → gateway)

```json
{
  "capabilities": {
    "app_hosting": false,
    "wildcard_domain": "",
    "drop": {
      "enabled": true,
      "accepts_public_uploads": true,
      "webui_available": true
    }
  }
}
```

- `enabled` — the node runs a Drop/Kubo service at all.
- `accepts_public_uploads` — the node participates in the **public** storage
  pool (public quota model). Private-org-only nodes set this `false`.
- `webui_available` — the node exposes a Kubo WebUI that the gateway may proxy
  through a short-lived same-origin session. The raw Kubo RPC/WebUI address is
  never published; it stays on the node's internal Docker network.

### `heartbeat.drop` + `heartbeat.versions.kubo` (node → gateway)

```json
{
  "versions": { "node": "2.0.0", "singbox": "1.11.4", "kubo": "0.29.0" },
  "services": { "vpn": "active", "drop": "active" },
  "drop": {
    "state": "active",
    "kubo_version": "0.29.0",
    "repo_size_bytes": 734003200,
    "storage_max_bytes": 53687091200,
    "num_objects": 1284
  }
}
```

- `drop.state` ∈ `disabled | starting | active | degraded | full | unreachable`.
  Runtime health (not the mere presence of a capability) determines whether the
  gateway treats the node as usable for reservations.
- `services.drop` mirrors the runtime service health used by the org-node
  service map; it is populated as a `drop` service record on the gateway.
- `repo_size_bytes` / `storage_max_bytes` / `num_objects` feed capacity
  accounting. The gateway reserves against `storage_max_bytes` when it is > 0
  (a 0 max means "unbounded / unknown"). Exact capacity stays in PostgreSQL and
  in gateway-internal responses; public discovery only exposes a **coarse**
  bucket (`available | limited | full | unknown`).

The gateway persists the latest Drop capability on `hello` and the latest Drop
status/Kubo version on each `heartbeat` (`node_drop_status` table).

## Versioning

The envelope is open for additive change only: new optional fields and new
message types/actions are allowed within v2; renames/removals require a new
WS path (`/api/v3/nodes/ws`). Both sides must ignore unknown fields
(`json.Unmarshal` default) and unknown message types (log + drop).
