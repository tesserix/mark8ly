-- Record the carrier-side cancel/return action taken for a shipment when its
-- order is refunded or cancelled. Lets the admin see per-shipment outcome and
-- lets a future sweep re-drive failures. All nullable-or-defaulted so existing
-- rows stay valid with no backfill.
--
--   cancel_action       none | cancel_forward | rto | reverse_pickup
--   cancel_status       none | requested | succeeded | failed | unsupported
--   cancel_reason       carrier's short reason on failure (never the raw body)
--   cancel_requested_at when the action was attempted
ALTER TABLE shipments
    ADD COLUMN IF NOT EXISTS cancel_action       varchar(20) NOT NULL DEFAULT 'none',
    ADD COLUMN IF NOT EXISTS cancel_status       varchar(20) NOT NULL DEFAULT 'none',
    ADD COLUMN IF NOT EXISTS cancel_reason       text,
    ADD COLUMN IF NOT EXISTS cancel_requested_at timestamptz;
