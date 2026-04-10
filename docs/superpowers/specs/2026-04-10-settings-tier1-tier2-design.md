# Settings Tier 1 & 2 — Unified Design Spec

**Date:** 2026-04-10
**Status:** Approved

## 1. Overview

Five settings features completing the admin settings surface: Account & Security, Custom Domains (Cloudflare DNS management), Subscription/Billing (Stripe Billing), Audit Logs (read-only viewer), and Notifications (preferences + bell dropdown). All inside the existing `marketplace-api` binary except audit logs which proxies to the existing `audit-service`.

### Build order

1. **S1 — Account & Security** (profile, MFA via auth-bff, sessions, delete tenant)
2. **S2 — Custom Domains** (Cloudflare API DNS management, verification worker, auto-SSL)
3. **S3 — Subscription/Billing** (Stripe Billing checkout + portal, webhook)
4. **S4 — Audit Logs** (read-only viewer proxying to audit-service)
5. **S5 — Notifications** (preferences, notification table, bell dropdown, 30s poll)

### Constraints

- Same binary (`marketplace-api`), same repo (`mark8ly`)
- Auth mutations proxy through `auth-bff` — marketplace-api does NOT manage GIP directly
- Custom domains use merchant's Cloudflare API token — we manage DNS records on their behalf
- Subscription uses Stripe Billing — no custom billing UI, leverage Stripe Checkout + Customer Portal
- Audit logs are read-only — existing `audit-service` is the source of truth
- Notifications use polling (30s), not WebSocket — simple and sufficient

## 2. Data Model

### 2.1 Migration 000015 — Custom Domains

```sql
CREATE TABLE custom_domains (
    id                      UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id               UUID          NOT NULL,
    store_id                UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    domain                  VARCHAR(253)  NOT NULL,
    status                  VARCHAR(20)   NOT NULL DEFAULT 'pending',
    cloudflare_zone_id      VARCHAR(100),
    cloudflare_dns_record_id VARCHAR(100),
    cf_api_token_encrypted  TEXT          NOT NULL,
    ssl_status              VARCHAR(20)   NOT NULL DEFAULT 'pending',
    verified_at             TIMESTAMPTZ,
    error_message           TEXT,
    created_at              TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (domain)
);
CREATE INDEX cd_store_idx ON custom_domains (store_id);
```

Status values: `pending` → `verifying` → `active` | `error` | `removing`
SSL status: `pending` → `active` | `error`

### 2.2 Migration 000016 — Subscriptions

```sql
CREATE TABLE store_subscriptions (
    id                      UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id               UUID          NOT NULL,
    store_id                UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    stripe_customer_id      VARCHAR(100)  NOT NULL,
    stripe_subscription_id  VARCHAR(100),
    plan                    VARCHAR(30)   NOT NULL DEFAULT 'free',
    status                  VARCHAR(30)   NOT NULL DEFAULT 'active',
    current_period_start    TIMESTAMPTZ,
    current_period_end      TIMESTAMPTZ,
    cancel_at_period_end    BOOLEAN       NOT NULL DEFAULT false,
    created_at              TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id)
);
CREATE INDEX ss_stripe_cust_idx ON store_subscriptions (stripe_customer_id);
```

Plan values: `free`, `starter`, `pro`, `enterprise`
Status values: `active`, `trialing`, `past_due`, `cancelled`, `incomplete`

### 2.3 Migration 000017 — Notifications

```sql
CREATE TABLE notification_preferences (
    id              UUID    PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID    NOT NULL,
    store_id        UUID    NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    preferences     JSONB   NOT NULL DEFAULT '{
        "new_order": true,
        "low_stock": true,
        "return_requested": true,
        "payment_received": true,
        "review_submitted": true
    }'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (store_id)
);

CREATE TABLE notifications (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL,
    store_id        UUID          NOT NULL,
    type            VARCHAR(40)   NOT NULL,
    title           VARCHAR(200)  NOT NULL,
    message         TEXT,
    resource_type   VARCHAR(40),
    resource_id     UUID,
    is_read         BOOLEAN       NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX notif_store_unread_idx ON notifications (store_id, is_read, created_at DESC);
CREATE INDEX notif_store_recent_idx ON notifications (store_id, created_at DESC);
```

Notification types: `new_order`, `low_stock`, `return_requested`, `payment_received`, `review_submitted`, `domain_verified`, `domain_error`, `subscription_expiring`, `subscription_cancelled`

## 3. Architecture

### 3.1 Account & Security (S1)

```
Admin UI → marketplace-api /admin/account/* → proxy to auth-bff for MFA/sessions
                                            → direct DB for profile updates
                                            → tenant-service for delete tenant
```

MFA flow: admin UI calls marketplace-api, which proxies to auth-bff's internal API. Auth-bff manages GIP MFA enrollment (TOTP + passkey). Marketplace-api never touches GIP directly.

### 3.2 Custom Domains (S2)

```
Admin UI → marketplace-api /admin/stores/:storeId/domains
  → POST: store CF token + domain → call Cloudflare API to create CNAME record
  → Background worker: poll Cloudflare for DNS propagation + SSL status
  → On active: update tenant-router-service to route the domain
```

Cloudflare API calls:
- `POST /zones/:zoneId/dns_records` — create CNAME pointing to `stores.mark8ly.com`
- `GET /zones/:zoneId/dns_records/:recordId` — check propagation status
- `DELETE /zones/:zoneId/dns_records/:recordId` — remove on domain deletion
- Zone ID derived from merchant's Cloudflare token via `GET /zones?name=<domain>`

### 3.3 Subscription/Billing (S3)

```
Admin UI → marketplace-api → Stripe API
  → Create Checkout Session (plan selection) → redirect to Stripe Checkout
  → Create Portal Session (manage billing) → redirect to Stripe Portal
  → Webhook: subscription lifecycle events → update store_subscriptions
```

Stripe events handled:
- `checkout.session.completed` — create subscription record
- `customer.subscription.updated` — plan/status changes
- `customer.subscription.deleted` — mark cancelled
- `invoice.payment_failed` — mark past_due

### 3.4 Audit Logs (S4)

```
Admin UI → marketplace-api /admin/stores/:storeId/audit-logs
  → proxy to audit-service GET /api/v1/logs?tenant_id=&store_id=&...
  → return formatted response
```

No new tables in marketplace-api. Read-only proxy with search/filter/pagination passthrough.

### 3.5 Notifications (S5)

```
Outbox events (order.created, return.requested, etc.)
  → notification.Listener checks preferences
  → if enabled: INSERT INTO notifications
  
Admin UI bell icon → GET /notifications (poll every 30s)
  → show dropdown with unread badge
  → click notification → mark read + navigate to resource
```

The notification listener runs as a goroutine in marketplace-api (same pattern as outbox publisher), subscribing to outbox events.

## 4. API Endpoints

### 4.1 Account & Security

- `GET /admin/account` — profile (name, email, mfa_enabled, created_at)
- `PATCH /admin/account` — update name, email
- `POST /admin/account/mfa/enable` — proxy to auth-bff, returns QR code / passkey challenge
- `POST /admin/account/mfa/verify` — verify TOTP code to complete enrollment
- `POST /admin/account/mfa/disable` — proxy to auth-bff
- `GET /admin/account/sessions` — active sessions from auth-bff
- `DELETE /admin/account/sessions/:id` — revoke a session
- `DELETE /admin/account` — delete tenant (owner only, requires confirmation token)

### 4.2 Custom Domains

- `GET /admin/stores/:storeId/domains` — list custom domains with status
- `POST /admin/stores/:storeId/domains` — add domain (body: `{ domain, cf_api_token }`)
- `DELETE /admin/stores/:storeId/domains/:id` — remove domain + DNS record
- `POST /admin/stores/:storeId/domains/:id/verify` — manual re-verify trigger

### 4.3 Subscription/Billing

- `GET /admin/stores/:storeId/subscription` — current plan, status, period dates
- `POST /admin/stores/:storeId/subscription/checkout` — create Stripe Checkout session (body: `{ plan }`)
- `POST /admin/stores/:storeId/subscription/portal` — create Stripe Portal session
- `POST /webhooks/stripe-billing` — Stripe subscription webhooks (separate from payment webhooks)

### 4.4 Audit Logs

- `GET /admin/stores/:storeId/audit-logs` — list with filters: user, action, resource_type, severity, date_from, date_to, search. Paginated.
- `GET /admin/stores/:storeId/audit-logs/export` — CSV download with same filters

### 4.5 Notifications

- `GET /admin/stores/:storeId/notifications` — recent 50, newest first. Query param: `?unread_only=true`
- `GET /admin/stores/:storeId/notifications/unread-count` — just the count (for badge polling)
- `PATCH /admin/stores/:storeId/notifications/:id/read` — mark single read
- `PATCH /admin/stores/:storeId/notifications/read-all` — mark all read
- `GET /admin/stores/:storeId/notification-preferences` — get toggles
- `PATCH /admin/stores/:storeId/notification-preferences` — update toggles

## 5. Admin UI

### 5.1 Pages

```
/settings/account           — Profile form (name, email)
                              MFA section: status badge, enable/disable button, QR code modal for TOTP
                              Active sessions table (device, IP, last active, revoke button)
                              Danger zone: "Delete account" with typed-confirmation dialog

/settings/domains           — Domain list table (domain, status badge, SSL badge, verified date)
                              "Add domain" form: domain input + Cloudflare API token input
                              Per-domain: verify button, remove button with confirmation
                              Status: pending (spinner), verifying (progress), active (green), error (red + message)

/settings/subscription      — Current plan card (plan name, status badge, period dates)
                              Plan comparison grid (free/starter/pro/enterprise features)
                              "Change plan" → Stripe Checkout redirect
                              "Manage billing" → Stripe Portal redirect
                              Cancel warning if past_due status

/settings/audit-logs        — Search bar + filters (user, action, resource type, severity, date range)
                              Table: timestamp, user, action, resource, status, severity
                              Row expand for full detail (IP, user agent, metadata)
                              "Export CSV" button

/settings/notifications     — Toggle grid: each notification type with on/off switch
                              Preview of what each notification looks like
```

### 5.2 Bell dropdown (topbar)

```
Bell icon in AdminShell topbar (already exists as static button)
  → Red dot badge when unread_count > 0
  → Click opens dropdown (max-h-96, scrollable)
  → Each notification: icon by type, title, message excerpt, time ago, unread dot
  → Click notification: mark read + router.push to resource
  → "Mark all read" link at bottom
  → "Notification settings" link → /settings/notifications
  → Poll GET /notifications/unread-count every 30s
```

### 5.3 Sidebar update

```
Settings ▸
  Store Settings    → /settings/general
  Storefront        → /settings/storefront
  Stores            → /settings/stores
  Team              → /settings/team
  Payments          → /settings/payments
  Shipping          → /settings/shipping
  Tax               → /settings/tax
  Domains           → /settings/domains
  Subscription      → /settings/subscription
  Account           → /settings/account
  Audit Logs        → /settings/audit-logs
  Notifications     → /settings/notifications
```

### 5.4 Design

All pages follow Paper/Ink/Moss editorial pattern:
- `AdminShell` wrapper
- Source Serif 4 headings
- Hairline rules between sections
- Danger zone: border-[color:var(--danger)] with ink-900 delete button
- Status badges: moss-700 for active/success, signal for error, ink-900/40 for pending
- Bell dropdown: white elevated card with shadow-2, same component patterns as UserMenu

## 6. Security

### 6.1 Account
- Delete tenant requires owner role + typed confirmation ("delete my store")
- MFA enable/disable proxied through auth-bff — marketplace-api never sees TOTP secrets
- Session revocation proxied through auth-bff

### 6.2 Custom Domains
- Cloudflare API token stored encrypted (same column pattern as payment keys — `cf_api_token_encrypted`)
- Domain ownership verified via DNS record creation (merchant must own the Cloudflare zone)
- Domain uniqueness: `UNIQUE (domain)` prevents cross-tenant domain claiming

### 6.3 Subscription
- Stripe webhook signature verification (reuse existing Stripe webhook pattern)
- Separate webhook endpoint `/webhooks/stripe-billing` to avoid confusion with payment webhooks
- Checkout session created server-side — client never sees Stripe secret key

### 6.4 Notifications
- Notifications scoped by store_id + tenant_id — no cross-tenant leak
- Bell dropdown polls count endpoint (lightweight) not full list
- No sensitive data in notification messages (order numbers, not amounts)

## 7. Background Workers

### 7.1 Domain verification worker
- Runs in admin + both modes (same as csvjob worker pattern)
- Polls every 60s for domains with status `verifying`
- Calls Cloudflare API to check DNS propagation
- On success: set status=active, ssl_status=active, verified_at=now()
- On failure after 24h: set status=error with message
- On domain removal: call Cloudflare DELETE, set status=removing

### 7.2 Notification listener
- Runs in admin + both modes
- Subscribes to outbox events (reuses the outbox publisher's event stream)
- On each event: check notification_preferences for the store
- If enabled: INSERT notification row
- Lightweight — just a goroutine reading from a channel

## 8. Testing

- **S1:** Profile update, MFA enable/disable proxy, session list, delete tenant (owner vs non-owner), confirmation validation
- **S2:** Domain add (Cloudflare API mock), verification worker cycle, DNS propagation success/failure, domain removal + DNS cleanup, duplicate domain rejection
- **S3:** Stripe Checkout session creation, Stripe Portal session, webhook signature verification, subscription lifecycle (created → updated → cancelled → past_due)
- **S4:** Audit log list with filters (proxy passthrough), CSV export, date range filter, pagination
- **S5:** Notification creation from outbox event, preference check (disabled type not created), unread count, mark read, mark all read, bell poll integration

## 9. Out of Scope

- Backup codes for MFA — follow-up
- Forced MFA policy for team members — follow-up
- Password change (GIP handles this via auth-bff login flow)
- Real-time WebSocket for notifications — polling is sufficient
- In-app notification center page (just bell dropdown + settings)
- Custom email templates for notifications — uses notification-service defaults
- Stripe metered billing / usage-based pricing — fixed plans only
- Multi-domain per store — one custom domain per store for now
- Wildcard domains — explicit domain only
