-- 000046_white_label_app_lifecycle.up.sql
-- §13.5 — app lifecycle runs independently of subscription state during Pro+App teardown.

CREATE TABLE white_label_app_lifecycle (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    store_id        UUID        NOT NULL,
    tenant_id       UUID        NOT NULL,
    status          VARCHAR(30) NOT NULL
        CHECK (status IN (
            'active',
            'sunset_scheduled',
            'downloads_blocked',
            'pulled',
            'firebase_archived',
            'credentials_purged'
        )),
    scheduled_at    TIMESTAMPTZ,
    actor           VARCHAR(100) NOT NULL,
    reason          TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX wlal_store_idx ON white_label_app_lifecycle (store_id, created_at DESC);
CREATE INDEX wlal_scheduled_idx ON white_label_app_lifecycle (scheduled_at)
    WHERE scheduled_at IS NOT NULL;

COMMENT ON TABLE white_label_app_lifecycle IS '§13.5 — append-only transition log for the white-label app teardown sequence.';
