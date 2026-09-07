-- 000136_drop_tenant_sso_tables.up.sql
-- Retires per-tenant SSO (mark8ly#820).
--
-- The whole implementation (internal/sso, the admin config handler, the
-- public login/callback/logout handler) was GIP-based and never wired
-- into main.go: NewSSOConfigHandler / NewSSOLoginHandler were never
-- called, TenantResolver had no implementation, and loginSAML was a
-- stub. This estate migrated auth to Zitadel (#524), so finishing SSO
-- would mean rewriting it from scratch, not wiring up what's here.
--
-- Both tables held ZERO rows in production, so nothing is lost.
-- 000070/000071 (which created them) and 000133 (which dropped a
-- column off tenant_sso_configs) are NOT deleted — applied migrations
-- are history.

DROP TABLE IF EXISTS tenant_sso_user_mappings;
DROP TABLE IF EXISTS tenant_sso_configs;
DROP TYPE IF EXISTS sso_provider_kind;
