-- migration 063: fx_rates table for MRR USD rollup (P17 observability)
--
-- Stores the latest mid-market FX rates used by the MRRRollup Prometheus
-- collector (internal/metrics/mrr_rollup.go) to convert per-currency plan
-- prices into a unified USD MRR gauge. Rows are upserted by the
-- cmd/fx-rate-refresh binary on a daily schedule.
CREATE TABLE IF NOT EXISTS fx_rates (
    currency    CHAR(3)          PRIMARY KEY,
    usd_mid_rate NUMERIC(18, 10) NOT NULL,   -- 1 unit of currency = X USD, mid-market
    fetched_at  TIMESTAMPTZ      NOT NULL DEFAULT now(),
    source      TEXT             NOT NULL
);
