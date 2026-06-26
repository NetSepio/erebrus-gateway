-- Wallet chain used at node enrollment (SOLANA | ETHEREUM).
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS chain text;