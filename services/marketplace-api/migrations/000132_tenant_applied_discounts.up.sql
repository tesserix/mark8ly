-- 000132_tenant_applied_discounts.up.sql
-- mark8ly#660 T6 — what THIS service applied for a tenant, so a store created
-- later can be covered by an override granted before it existed.
--
-- ══ THE GAP THIS CLOSES ══
--
-- internal/billing/tenantdiscount fans an operator's apply out over every
-- store the tenant owns today. A store whose subscription has no
-- stripe_subscription_id yet — the card-less trialing tenant, which is exactly
-- the population an operator discounts — is reported `pending`, and until this
-- migration NOTHING picked that store up afterwards: the service held no
-- record that the tenant had an override at all, so when the subscription was
-- finally created there was nothing here to consult. The discount silently
-- stopped covering the tenant as they grew, which surfaces in a renewal
-- negotiation rather than in a log.
--
-- One row per (tenant, coupon) makes the override a durable, readable fact
-- inside this service. The two places this service ITSELF creates a Stripe
-- subscription — internal/billing/trial.Subscriber.subscribeInTx and
-- internal/subscription/planchange.Orchestrator.executeInitialSubscription,
-- the only two callers of billingstripe.CreateSubscription — read it and
-- attach the coupon to the subscription they just created.
--
-- A THIRD ROUTE IS NOT COVERED, and saying so here is cheaper than finding it
-- in a renewal. internal/billing/stripe.CreateCheckoutSession opens a hosted
-- Checkout with mode=subscription: Stripe creates the subscription, and this
-- service only learns of it from a webhook. Nothing on that path consults this
-- table, so a store subscribed through hosted Checkout does not get the
-- tenant's override applied. Closing that would mean hooking the webhook that
-- records the new stripe_subscription_id, which is a separate change against a
-- path that is currently unreachable anyway — its PriceID is a documented
-- placeholder (cmd/marketplace-api/main.go, stripeClientAdapter).
--
-- ══ WHAT THIS TABLE IS NOT ══
--
-- IT IS NOT THE GRANT, AND IT IS NOT AUTHORITATIVE ABOUT ONE.
--
-- The console's `tenant_pricing_override_coupons` (tesserix-home 0047) remains
-- the record of WHO was granted WHAT and WHY. It is minted there, in the same
-- act that creates the Stripe Coupon, and this service cannot read it. What is
-- recorded here is narrower and is the only thing this service can honestly
-- claim: THIS SERVICE APPLIED THIS COUPON FOR THIS TENANT, and has not since
-- been told to stop.
--
-- The difference is not academic. A console-side retirement that never reaches
-- mark8ly — the DELETE call fails, the transport is down, the operator retires
-- the grant in a window where these two products disagree — leaves the row
-- below LIVE, and a store created afterwards is given a discount the console
-- believes it withdrew. THAT IS A KNOWN, ACCEPTED LIMITATION, and it is the
-- unavoidable cost of duplicating a fact another service owns: the alternative
-- is a synchronous read across a product boundary on the subscription-creation
-- hot path, which would make a console outage stop merchants subscribing.
-- Reconciliation is a human's, and both sides carry the coupon id.
--
-- IT IS ALSO NOT "AT MOST ONE ACTIVE OVERRIDE PER TENANT" AS #660 STATES IT.
-- #660 asserts that guarantee; no local table can deliver it, because the
-- discounts a Stripe customer actually carries are held in Stripe, and a
-- coupon attached out of band — by an operator in the Stripe dashboard, or by
-- this service before this table existed — is invisible here. The partial
-- unique index below is a CEILING on what THIS SERVICE will record and act on,
-- in the same sense 0047's own index is a ceiling on what the console will
-- mint. Neither is a floor, and neither can answer "is this tenant really
-- being charged less".
--
-- ══ SHAPE ══
--
-- Deliberately 0047's shape, because it is the same fact viewed from the other
-- side of the boundary and two spellings of one fact is two things to
-- reconcile: a surrogate key, a `granted_by`/`granted_at` pair, a
-- `removed_by`/`removed_at` retirement pair kept whole by a biconditional, and
-- the at-most-one rule as a PARTIAL UNIQUE INDEX over the live rows rather
-- than a bare unique constraint on tenant_id.
--
-- The partial index is what makes re-granting possible. A tenant whose
-- override was removed in March must be able to receive a different one in
-- June; under a plain UNIQUE (tenant_id) that second grant is refused forever
-- by a row describing a coupon nobody is using, and the only ways out are
-- deleting the row (erasing the record of a discount that was really applied
-- to real invoices) or a later migration rewriting the uniqueness rule under
-- live data.
--
-- Two differences from 0047, both deliberate:
--
--   * `tenant_id` is a bare uuid, not the console's namespaced
--     `<source>:<id>` text. Every tenant id in this database is a uuid and
--     every other table spells it that way; platform-api splits the namespace
--     before a request reaches this service.
--
--   * there is no `mode` column. 0047 needs one because the console mints into
--     a Stripe account it chooses per request. This service talks to exactly
--     one Stripe account — the one STRIPE_BILLING_SECRET_KEY names — so every
--     row here is already reachable from one mode and a column recording it
--     would record the same value forever.
--
-- `granted_by` is NULLABLE, where 0047's is NOT NULL, and that is not an
-- oversight. 0047 is only ever written by an operator pressing a button. This
-- table is also written by the subscription-creation hook, which runs with no
-- request and no operator behind it; NULL there says "this service, on its own
-- behalf" rather than inventing a fake identity. There is no FK either way —
-- operator identity lives in Zitadel, not in this database.

CREATE TABLE IF NOT EXISTS tenant_applied_discounts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    tenant_id uuid NOT NULL,

    -- `co_…`. The blank CHECK is not decoration: '' is NOT NULL, so without it
    -- an empty string records an application that named no coupon, in the one
    -- spelling NOT NULL cannot see. tenantdiscount.Service trims and refuses a
    -- blank coupon id before it gets here; this is the second line.
    stripe_coupon_id text NOT NULL
                     CONSTRAINT tenant_applied_discounts_coupon_id_is_not_blank
                     CHECK (btrim(stripe_coupon_id) <> ''),

    -- The operator who applied it, as an opaque identity. NULL means no
    -- operator was behind the write — see the header.
    granted_by text
               CONSTRAINT tenant_applied_discounts_granted_by_is_not_blank
               CHECK (granted_by IS NULL OR btrim(granted_by) <> ''),
    granted_at timestamptz NOT NULL DEFAULT now(),

    -- The retirement half. Both NULL is a live override; both set is a
    -- retired one.
    removed_by text,
    removed_at timestamptz,

    -- One biconditional rather than two NULL checks, 0047's rule for the same
    -- reason: the two halves are the same rule, and splitting them lets a
    -- future edit satisfy one and drop the other. A `removed_at` with no
    -- `removed_by` is a removal nobody is accountable for; a `removed_by` with
    -- no `removed_at` is a row the partial index below still counts as live,
    -- which would keep handing the coupon to new stores after it was removed.
    --
    -- Both sides are IS NULL tests, so the expression is never UNKNOWN and the
    -- constraint cannot pass by accident.
    CONSTRAINT tenant_applied_discounts_removal_is_whole
    CHECK ((removed_by IS NULL) = (removed_at IS NULL)),

    -- A removal cannot precede the application it removes. `>=` and not `>`:
    -- an override applied and removed in the same transaction is odd but
    -- coherent, whereas a removal timestamped before its grant can only be a
    -- clock or an input error, and it reads as a live row on any surface
    -- ordering by date.
    CONSTRAINT tenant_applied_discounts_removal_follows_grant
    CHECK (removed_at IS NULL OR removed_at >= granted_at)
);

-- AT MOST ONE LIVE ROW PER TENANT, with the retired ones not counted.
--
-- This is what makes the read on the subscription-creation path unambiguous:
-- "which coupon does this tenant hold" has one answer or none, and the code
-- that reads it never has to choose between two.
--
-- A partial unique index rather than a table constraint, because a table
-- constraint cannot carry a WHERE — and CREATE UNIQUE INDEX IF NOT EXISTS is
-- idempotent on its own, so this file needs no DROP/ADD dance.
CREATE UNIQUE INDEX IF NOT EXISTS tenant_applied_discounts_one_live_per_tenant
    ON tenant_applied_discounts (tenant_id)
    WHERE removed_at IS NULL;

COMMENT ON TABLE tenant_applied_discounts IS
    'mark8ly#660 — the platform override THIS SERVICE applied for a tenant, read by the subscription-creation paths so a store created later is covered too. Not the grant: tesserix-home 0047 records who granted what and why, and a console-side retirement that never reaches mark8ly leaves a row here stale.';
