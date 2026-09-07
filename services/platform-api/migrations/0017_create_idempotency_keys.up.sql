-- idempotency_keys — replay protection for platform-api's HMAC-signed
-- console writes (mark8ly#720 Task 5).
--
-- marketplace-api's platformadmin surface already has this table
-- (its migration 000001) and internal/idempotency.Reserve/Lookup/Complete/
-- Release built on it; platform-api never needed the pattern until now,
-- because its first platformadmin write route is the email-template PUT
-- this migration exists for. Schema and index are copied verbatim from
-- marketplace-api's so internal/idempotency here (services/platform-api/
-- internal/idempotency) is the same code against the same shape, not a
-- reinterpretation.
--
-- tenant_id is NOT NULL for the same reason marketplace-api's copy is: it
-- lets one table serve both tenant-scoped writes (a real tenant/store id)
-- and estate-wide ones. The email-template PUT is estate-wide, so its
-- reservations record uuid.Nil — see emailtemplates.Store's
-- estateWideTenant for why that is the honest value rather than a
-- borrowed one.
CREATE TABLE idempotency_keys (
    key        varchar(255) PRIMARY KEY,
    tenant_id  uuid         NOT NULL,
    response   jsonb,
    created_at timestamptz  NOT NULL DEFAULT now(),
    expires_at timestamptz  NOT NULL
);

CREATE INDEX idempotency_expires_idx ON idempotency_keys (expires_at);
