-- Restore the plain non-unique index from migration 0003.
-- The lower-casing of existing owner_email values is intentionally NOT
-- reverted: the original casing is not recoverable, and lower-cased
-- emails remain valid and deliverable.

CREATE INDEX IF NOT EXISTS idx_tenants_owner_email ON tenants (owner_email);

DROP INDEX IF EXISTS tenants_owner_email_unique;
