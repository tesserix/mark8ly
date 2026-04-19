-- 000076_white_label_app_state.up.sql
-- P15 §13.5 — mutable current-state table for the white-label app teardown
-- lifecycle. Complements the existing append-only `white_label_app_lifecycle`
-- transition log (migration 000048), which stays untouched: advancer writes
-- one log row per transition there, one state row here per store.
--
-- The pair of tables is by design:
--   - white_label_app_state       → one row per store, mutable, advancer rewrites on transition
--   - white_label_app_lifecycle   → append-only log, one row per transition, audit trail
--
-- At day 30/60/60/90 the advancer reads a state row due for action, calls
-- Apple/Google/Firebase + appcreds.PurgeAll, then updates the state row's
-- status + next_action_at AND inserts a matching log row into the existing
-- table.

CREATE TABLE white_label_app_state (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID        NOT NULL,
    store_id            UUID        NOT NULL UNIQUE,        -- one state row per store
    status              VARCHAR(30) NOT NULL
        CHECK (status IN (
            'active',
            'sunset_scheduled',
            'downloads_blocked',
            'pulled',
            'firebase_archived',
            'credentials_purged'
        )),
    -- When the advancer should next examine this row. NULL means "no
    -- pending action" (e.g. the row reached a terminal status or is live).
    next_action_at      TIMESTAMPTZ,
    -- Timestamp the sunset was scheduled for — the anchor the advancer
    -- uses to decide "is it day 30 yet / day 60 / day 90?".
    scheduled_at        TIMESTAMPTZ,
    -- External IDs stored here (not in the log table) because the log
    -- records transitions, not the mutable metadata needed to call
    -- Apple/Google/Firebase APIs.
    apple_app_id        VARCHAR(100),
    google_package      VARCHAR(255),
    firebase_project_id VARCHAR(100),
    -- merchant_initiated=true compresses the schedule to 7 days (§15.5).
    -- The consumer seeds scheduled_at 53 days in the past so the advancer
    -- treats the row as "day 53 already elapsed".
    merchant_initiated  BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- next_action_at dominates the advancer query; index it with the WHERE
-- clause so the partial index stays small (most rows are terminal).
CREATE INDEX wlas_next_action_idx ON white_label_app_state (next_action_at)
    WHERE next_action_at IS NOT NULL;
CREATE INDEX wlas_tenant_idx ON white_label_app_state (tenant_id);

COMMENT ON TABLE white_label_app_state IS
    '§13.5 — mutable current-state row per store for the white-label app
     teardown. Append-only audit trail lives in white_label_app_lifecycle.';
