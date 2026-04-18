-- 000054_signup_anomaly_log.up.sql
-- Idempotency marker for the daily signup-anomaly cron. Unique (alert_date, signup_date)
-- means a second same-day run is a no-op (ON CONFLICT DO NOTHING).
CREATE TABLE signup_anomaly_log (
    alert_date   DATE         NOT NULL,
    signup_date  DATE         NOT NULL,
    count        INT          NOT NULL,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    PRIMARY KEY (alert_date, signup_date)
);
