# Erebrus v2 — Node Identity, PeerID & DID

**Status: FROZEN (v2.0).**

## Identity derivation

Every node has exactly one secret: a BIP39 **mnemonic** (generated at install
time, stored in `/etc/erebrus/config.env`, never transmitted). From it the
node deterministically derives:

1. **Wallet keypair** — BIP39 seed → BIP32 path (same derivation already used
   in v1 `erebrus/p2p/host.go` / `util/pkg`); used to sign the gateway
   registration challenge and, later, Solana registration transactions.
2. **libp2p Ed25519 keypair → PeerID** — seeded from the same BIP39 seed
   (v1 `p2p/host.go` logic carried over verbatim so existing node mnemonics
   keep their PeerIDs). PeerID is the node's stable identity anchor.
3. **DID** — `did:erebrus:<PeerID>`, e.g.
   `did:erebrus:12D3KooWQYhTNQdmr3ArTeo5gCtJ8m1bbb73Bb4Q4xxK9zMrf1nK`.

A golden test in `erebrus/internal/p2p` pins a fixed test mnemonic to its
expected PeerID/DID so the derivation can never drift silently.

## What libp2p is (and is not) used for in v2

- **IS:** identity (PeerID), DID derivation, DHT advertisement (nodes
  advertise under rendezvous tag `erebrus`, gateway runs the bootstrap peer).
  This keeps future pathways open: IPFS/decentralized storage, DHT-based
  discovery, peer-to-peer DID resolution.
- **IS NOT:** status reporting, heartbeats, provisioning, or any operational
  control flow. All of that is HTTPS + WebSocket (`ws-protocol.md`).
  Gossipsub is gone.

## IP obfuscation

Anywhere node info leaves the operational trust boundary (public APIs beyond
connection endpoints, future on-chain registration), the node's IP appears
only as:

```
ip_hash = lowercase hex SHA3-256 of the dotted-quad public IPv4 string
sha3_256("203.0.113.10") = "f1820f54…"
```

The raw IP travels only over the authenticated node↔gateway channel and is
used to build client connection endpoints.

## Future Solana registration (mocked in v2.0)

`erebrus/internal/registrar` defines:

```go
type NodeIdentity struct {
    PeerID  string // 12D3KooW…
    DID     string // did:erebrus:12D3KooW…
    IPHash  string // sha3-256 hex
    Region  string
    Spec    string // coarse hw description
    Wallet  string // solana address derived from mnemonic
    Version string
}

type Registrar interface {
    Register(ctx context.Context, id NodeIdentity) error
    UpdateStatus(ctx context.Context, peerID string, status string) error
}
```

v2.0 ships only `noop.Registrar` (logs and returns nil). The Solana
implementation (program TBD) will register `NodeIdentity` on-chain; the
payload above is the frozen shape it must serialize. No EVM/Peaq code remains.
