-- 000066_tax_validation_outage_log.up.sql
-- One row per observed registry failure. The clock-pause aggregator rolls this
-- up per-(country, store) within the active 14-day validation window; when
-- cumulative outage seconds > 72 * 3600 the orchestrator pauses the deadline
-- (§5.2). Also used for SEA queue-entry pauses (error_class='sea_queue').

CREATE TABLE IF NOT EXISTS tax_validation_outage_log (
    id               BIGSERIAL PRIMARY KEY,
    country          CHAR(2)      NOT NULL,
    registry         VARCHAR(30)  NOT NULL,
    store_id         UUID,
    tenant_id        UUID,
    error_class      VARCHAR(30)  NOT NULL,
    started_at       TIMESTAMPTZ  NOT NULL,
    ended_at         TIMESTAMPTZ,
    seconds_observed INTEGER,

    CHECK (ended_at IS NULL OR ended_at >= started_at)
);

CREATE INDEX IF NOT EXISTS tvol_open_idx     ON tax_validation_outage_log (registry, started_at) WHERE ended_at IS NULL;
CREATE INDEX IF NOT EXISTS tvol_store_idx    ON tax_validation_outage_log (store_id, started_at) WHERE store_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS tvol_registry_idx ON tax_validation_outage_log (registry, started_at);
