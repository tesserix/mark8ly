-- Onboarding acquisition attribution and session classification (mark8ly#992).
--
-- acquisition is the campaign context the merchant arrived with: an
-- allowlisted record of UTM parameters, a query-stripped landing path, a
-- query-stripped referrer, and capture times for the first and most recent
-- tagged arrival. It is captured by the onboarding app in the browser and
-- sent with the session create request, sanitised server-side in
-- internal/onboarding/acquisition.go before it is stored, and copied to the
-- tenant row on completion so the merchant carries it after the session is
-- gone. Never free-form: the sanitiser drops anything outside the allowlist,
-- so no full query string, token or email can land here.
--
-- classification is server-owned and set at session creation from the
-- email address (test domains, configured internal domains, configured demo
-- emails), re-checked at completion against configured demo slugs. The
-- funnel keeps its existing all-sessions counts unchanged and adds an
-- "external" view filtered on this column, so operational totals and
-- genuine-merchant conversions are reported side by side rather than one
-- silently replacing the other.
ALTER TABLE onboarding_sessions
    ADD COLUMN acquisition JSONB,
    ADD COLUMN classification VARCHAR(16) NOT NULL DEFAULT 'external';

ALTER TABLE onboarding_sessions
    ADD CONSTRAINT onboarding_sessions_classification_check
    CHECK (classification IN ('external', 'internal', 'test', 'demo'));

-- The external funnel view filters on classification within a created_at
-- window; this keeps that aggregate off a full scan as the table grows.
CREATE INDEX onboarding_sessions_classification_created_idx
    ON onboarding_sessions (classification, created_at);

ALTER TABLE tenants
    ADD COLUMN acquisition JSONB;
