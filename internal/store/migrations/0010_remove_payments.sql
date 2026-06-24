-- Remove payment scaffolding (USDC flow deferred). Pricing returns later.

DROP TABLE IF EXISTS crypto_payments;

ALTER TABLE subscriptions DROP COLUMN IF EXISTS payment_id;
ALTER TABLE plans DROP COLUMN IF EXISTS price_usdc;

-- Align product copy with erebrus.io (webapp) / gateway.erebrus.io (API).
UPDATE platform_settings
SET value = 'I accept the Erebrus Terms of Service https://erebrus.io/terms.'
WHERE key = 'auth_eula';