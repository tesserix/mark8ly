-- Enforces that an admin email owns at most one tenant.
--
-- Before this, `idx_tenants_owner_email` (migration 0003) was a plain,
-- non-unique index and the only cross-tenant admin-email guard lived in
-- the onboarding frontend (apps/onboarding/app/onboarding/actions.ts) —
-- which keyed on GIP UID rather than email and was bypassed entirely by
-- calling POST /onboarding/sessions/:id/complete directly. Two tenants
-- could therefore share an owner_email.
--
-- The application-level check in onboarding.Service (Create + Complete)
-- produces the friendly "use a different email" error. This index is the
-- backstop that closes the TOCTOU window between that check and the
-- INSERT: two concurrent onboardings for the same email now make one
-- fail with 23505 instead of both committing.
--
-- Case-insensitive on purpose: Founder@example.com and
-- founder@example.com are the same admin. onboarding.Service writes
-- owner_email already lower-cased, so new rows match the index directly.

-- Normalize existing rows first, otherwise the index below would treat
-- mixed-case duplicates as distinct and let them survive.
UPDATE tenants
   SET owner_email = lower(trim(owner_email))
 WHERE owner_email <> lower(trim(owner_email));

-- Verified against prod (mark8ly_platform_api) before writing this
-- migration: zero duplicate lower(owner_email) groups across all
-- tenants, so this index builds without a data cleanup step. If it ever
-- fails on another environment, that environment has genuine duplicates
-- that need a product decision (which tenant keeps the email) rather
-- than an automatic fix — hence no DELETE/merge here.
CREATE UNIQUE INDEX IF NOT EXISTS tenants_owner_email_unique
  ON tenants (lower(owner_email));

-- The old non-unique index is now redundant: the unique index above
-- serves the same lookup. Dropping it saves a write on every tenant
-- insert/update.
DROP INDEX IF EXISTS idx_tenants_owner_email;
