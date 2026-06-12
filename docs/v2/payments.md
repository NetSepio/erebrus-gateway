# Erebrus v2 — USDC Payments (Solana + Base)

**Status: FROZEN (v2.0).** Crypto-only. No Stripe, no fiat processors.
Reown AppKit drives the wallet UX in both webapp and Flutter app.

## Flow

```
client                    gateway                          chain
  │ POST /api/v2/payments   │                                │
  │─────────────────────────►  create crypto_payments row    │
  │ ◄─ {payment_id, amount_usdc, chain, treasury_address,    │
  │     reference, expires_at}                               │
  │                                                          │
  │ pay USDC via Reown AppKit (user signs in their wallet) ──►
  │                                                          │
  │ POST /payments/{id}/confirm {tx_hash}                    │
  │─────────────────────────►  verify on-chain via RPC ──────►
  │ ◄─ 200 confirmed → subscription activated                │
  │    (or 202 pending finality → background re-verifier)    │
```

## Payment request

- `amount_usdc`: decimal string from the plan price. To disambiguate
  concurrent payers on Base, the gateway may add a unique sub-cent dust
  amount (e.g. 9.990000 + 0.00NNNN) — the **expected exact amount is stored
  on the payment row** and must match the transfer.
- `reference`:
  - **Solana**: a fresh ephemeral public key, Solana-Pay style. The client
    includes it as a read-only account in the transfer instruction. The
    verifier can then find the tx by reference even without a submitted hash.
  - **Base**: opaque id for bookkeeping (match is by treasury + exact amount
    + sender + time window).
- `expires_at`: request is void after 30 min (row → `expired`).

## Verification rules

Common: `tx_hash` is UNIQUE across `crypto_payments` (replay-safe);
amount must equal the stored expected amount; recipient must be the
configured treasury for that chain; token must be canonical USDC.

| | Solana | Base |
|---|---|---|
| RPC | `SOLANA_RPC_URL` (Helius or standard) | `BASE_RPC_URL` |
| Token | USDC mint `EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v` | USDC contract `0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913` |
| Check | `getTransaction` (finalized); SPL token transfer to treasury ATA; reference key present in account list | `eth_getTransactionReceipt`; ≥ `BASE_MIN_CONFIRMATIONS` (default 6); `Transfer(address,address,uint256)` log on USDC contract with `to == treasury` |
| Finality | commitment=finalized | confirmation count |

Failure modes: wrong amount/mint/recipient → `409` + row `mismatched`
(support can inspect); not yet final → `202` + row stays `awaiting_finality`;
background re-verifier polls every 30s for up to 1h, also sweeps `pending`
rows by Solana reference in case the user never called confirm.

## Data

```
crypto_payments(
  id uuid PK, user_id, org_id NULL, plan_id,
  chain  text CHECK (chain IN ('solana','base')),
  expected_amount numeric(18,6), token text,
  treasury_address text, reference text,
  tx_hash text UNIQUE NULL, payer_address text NULL,
  status text CHECK (status IN
    ('pending','awaiting_finality','confirmed','mismatched','expired')),
  created_at, confirmed_at NULL
)
```

`status=confirmed` atomically (same DB tx) creates/extends the
`subscriptions` row (`source='crypto'`, `payment_id` linked) — for orgs it
settles the open invoice instead. Webhooks/notifications can hang off the
status transition later; nothing else may activate a crypto subscription.

## Env

```
SOLANA_RPC_URL=            BASE_RPC_URL=
SOLANA_TREASURY_ADDRESS=   BASE_TREASURY_ADDRESS=
BASE_MIN_CONFIRMATIONS=6   PAYMENT_EXPIRY_MINUTES=30
```
