-- 000007_csv_import_jobs: CSV import job tracking table (M7e).
CREATE TABLE csv_import_jobs (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    store_id            UUID        NOT NULL REFERENCES stores(id),
    user_id             TEXT        NOT NULL,
    gcs_path            TEXT        NOT NULL,
    content_hash        TEXT        NOT NULL,
    error_csv_gcs_path  TEXT,
    status              TEXT        NOT NULL DEFAULT 'queued',
    total_rows          INT,
    last_processed_row  INT         NOT NULL DEFAULT 0,
    success_count       INT         NOT NULL DEFAULT 0,
    error_count         INT         NOT NULL DEFAULT 0,
    heartbeat_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT csv_import_jobs_status_check
        CHECK (status IN ('queued', 'running', 'paused', 'completed', 'failed', 'cancelled')),
    CONSTRAINT csv_import_jobs_last_processed_row_check
        CHECK (last_processed_row >= 0)
);

-- Composite index for listing jobs by store, status, and recency.
CREATE INDEX idx_csv_import_jobs_store_status_created
    ON csv_import_jobs (store_id, status, created_at DESC);

-- Prevent duplicate active imports of the same file content per store.
CREATE UNIQUE INDEX idx_csv_import_jobs_store_content_hash_active
    ON csv_import_jobs (store_id, content_hash)
    WHERE status IN ('queued', 'running', 'paused');
