-- Migration 124: journal_subscribers — mark8ly.com marketing-site email
-- capture (#153), starting with the Journal "coming soon" page's
-- "Notify me when the first piece goes up" field.
--
-- This table intentionally carries NO tenant_id. Every other table in
-- this service is scoped to a merchant tenant (X-Tenant-ID header,
-- TenantMiddleware, TenantRepository[T]) — a Journal subscriber belongs
-- to mark8ly.com itself, not to any merchant's store. Bolting on a fake
-- tenant id here would misrepresent a platform-level marketing record as
-- tenant data. See internal/handlers/public/journal_subscribe.go for the
-- handler, which is mounted outside TenantMiddleware for the same reason.
--
-- gen_random_uuid() needs no extension: it is core in PostgreSQL 13+
-- (see migration 000058's comment on gen_random_bytes, which is the
-- pgcrypto function that DOES need one — this table uses neither).
CREATE TABLE IF NOT EXISTS journal_subscribers (
    id         UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    -- Stored normalised (trimmed + lowercased) by the repository so the
    -- unique index below actually catches "Foo@Bar.com" vs "foo@bar.com".
    email      VARCHAR(254) NOT NULL,
    -- Capture point, e.g. 'journal'. Lets this same table serve other
    -- marketing capture points later (a footer newsletter field, a
    -- waitlist, etc.) without another migration.
    source     VARCHAR(40)  NOT NULL DEFAULT 'journal',
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- One row per email, full stop. A double submit from the same capture
-- point — or a resubmission from a second one later — must not create a
-- duplicate. The repository inserts with ON CONFLICT DO NOTHING against
-- this index, which is also what makes the endpoint idempotent.
CREATE UNIQUE INDEX IF NOT EXISTS idx_journal_subscribers_email
    ON journal_subscribers (email);
