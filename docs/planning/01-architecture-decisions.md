# Architecture Decisions — Service Topology

## The starting point

Mark8ly inherited ~30 Go microservices from a "microservices by default" era.
All of them share a single Cloud SQL db-f1-micro instance, all serve one
product, and most are coupled at the database level. They are a **distributed
monolith pretending to be microservices**: paying every distributed-systems
cost (network hops, eventual consistency, 30 deployments, 30 CI pipelines,
sidecar proxies, cross-service auth) and getting **none** of the benefits
(independent scaling, independent deploys, fault isolation — they all fail
together when Cloud SQL hiccups anyway).

The intermittent auth/authz bugs are not caused by "too many services." They're
caused by **distributed state and contracts that aren't enforced anywhere**.
But the service sprawl makes the bugs harder to debug, harder to fix, and
harder to deploy fixes for.

## The decision: collapse to ~6 services

Don't merge to one Go binary. Do **two domain monoliths** plus the genuine
externals:

```
services/
├── platform-api/        ← Control plane (slower-changing, runs platform-wide)
│                          tenant, tenant-router, onboarding, verification,
│                          location, audit, document, settings, content,
│                          analytics-views, feature-flags-table
│
├── marketplace-api/     ← Product runtime (the marketplace itself)
│                          products, orders, inventory, cart, checkout, coupons,
│                          categories, customers, staff, vendors, tax, shipping,
│                          reviews   (+ deferred: gift-cards, marketing, approvals)
│
├── auth-bff/            ← Security boundary (kept separate intentionally)
├── notification/        ← Worker (long-lived Pub/Sub consumer)
├── payment/             ← PCI-isolated (keeps PCI scope tiny)
+ openfga                ← External / managed image (not built in-house)
```

**5 services owned + 1 managed.** Down from ~30.

## Why these specific boundaries

### `platform-api` — the control plane
One product surface that the admin and storefront frontends both consume for
tenant/setup-related operations. Slower-changing than the marketplace runtime.
Separate blast radius.

**Absorbs:** `tenant-service`, `tenant-router-service`, the onboarding logic
that lived in `tenant-service`, `verification-service`, `location-service`,
`audit-service`, `document-service`, `settings-service`, `analytics-service`,
the content/CMS DB used by onboarding marketing pages.

### `marketplace-api` — the product runtime
The actual marketplace. Admin and storefront both talk to it. Single
transaction boundary for cross-domain writes (creating an order updates
inventory, reservations, coupons in one DB transaction — not 4 Pub/Sub events
with retry storms).

**Absorbs:** all the `marketplace-*-service` repos and the `mp-*` services that
serve a single product surface.

### `auth-bff` — kept separate, intentionally
Close call. The deciding argument: **it's a security boundary**. Session cookie
minting, GIP token validation, OIDC flows, MFA, passkey ceremonies — you want
a small auditable surface area for this. Mixing it into platform-api means
every coupons-code change touches the same binary that mints session cookies.
Bad blast radius.

It also has a different deployment shape (`minScale: 1` for fast login),
different secrets (OIDC client secret, cookie encryption key, MFA pepper), and
serves a different domain (`auth.mark8ly.com`) for cookie isolation.

The cost of "one more service" is near-zero once it's in the monorepo.

### `notification` — kept separate, it's a worker
Long-lived Pub/Sub subscriber, not a request/response API. Different deployment
shape (no scale-to-zero, `minScale: 1`, no concurrency limit). Different
fan-out (SendGrid, Firebase, AWS SES/SNS, webpush). If notification fan-out
hangs (e.g., SendGrid latency spike), you don't want it taking down platform-api
request handlers.

**Note:** During the onboarding-only first slice, notification is **inlined
into platform-api** (just call SendGrid SDK directly). Extract when there's a
second consumer or when the worker shape becomes necessary.

### `payment` — kept separate for PCI scope isolation
Even with Stripe Elements / Razorpay handling raw card data, keeping the
webhook receiver and payment intent creation in a tiny separate service
drastically reduces what's in PCI scope when an auditor asks. Don't pollute
marketplace-api with PCI scope. Webhooks also need a stable public URL and
signature verification — different ingress concerns.

### `openfga` — external, managed
You don't build it. You deploy a managed image. It's infrastructure like
Postgres, not a "service you maintain."

## Services merged into others

| Old service | Merged into | Reason |
|---|---|---|
| `tenant-service` | `platform-api` | Core control-plane domain |
| `tenant-router-service` | `platform-api` | ~300 LOC managing Cloudflare DNS |
| `verification-service` | `platform-api` | Tightly coupled to onboarding |
| `location-service` | `platform-api` | Read-mostly reference data |
| `audit-service` | `platform-api` | Writes to a table + Pub/Sub fan-out; cron+circuit-breaker overkill |
| `document-service` | `platform-api` | Thin GCS wrapper, ~200 LOC of real logic; signs URLs, doesn't stream |
| `subscription-service` | `payment` (or `platform-api`) | Stripe Billing already does 95%; service is a thin DB layer |
| All `mp-*-service` repos | `marketplace-api` | One product surface, one DB instance, one tenant_id |

## Services deleted entirely

Pre-launch is the only cheap time to delete features. Be ruthless.

| Service | Action | Replacement |
|---|---|---|
| `qr-service` | **Delete** | Generate QR codes inline (`github.com/skip2/go-qrcode`, ~5 lines) |
| `status-service` | **Delete** | SaaS (Better Stack / Statuspage / Cronitor) or a Cloudflare Worker |
| `feature-flags-service` | **Delete** | A `flags` table in platform-api, or LaunchDarkly free tier, or Vercel Flags |
| `analytics-service` | **Delete** | It's "FDW only, no owned tables" — that's a SQL view, not a service. Expose from platform-api or query Cloud SQL via read replica. |
| `tickets-service` | **Delete** | SaaS (Plain, Help Scout, Crisp). **Confirmed: Mark8ly is not in the support-software business and tesserix-home tickets integration is a liability, not an asset.** |
| `mp-connector` | **Already deferred** | Stay deferred; don't even stub |
| `mp-marketing` | **Question scope** | If just email campaigns → merge into notification; if speculative → defer |
| `mp-gift-cards` / `mp-loyalty` / `mp-approvals` | **Defer** | Port stubs only unless validated as launch features |

**Total: ~6 services deleted with zero capability loss** (when SaaS replaces
the few real needs).

## Why not collapse further (one big monolith)?

Tempting but wrong. The case for keeping the 5/6 split:

- **`auth-bff` is a security boundary.** Bug-for-bug review of cookie/session
  code is much easier in a 5k-line binary than buried in 200k lines of
  marketplace logic.
- **`notification` is a worker, not an API.** Different runtime shape.
  Conflating the two means you can't tune them independently.
- **`payment` is PCI scope.** Auditor scope is determined by what process
  touches card data flows, even indirectly. Keep that surface tiny.
- **`platform-api` vs `marketplace-api`** is a deploy-frequency boundary.
  Marketplace code changes daily; platform/control-plane code changes weekly.
  Splitting them lets marketplace deploys not require regression testing
  the entire onboarding/tenant lifecycle.

These are **shape-based** boundaries (different runtime, different blast
radius, different change frequency), not **domain-based** boundaries. Good
service boundaries follow shape, not domain.

## What this collapse does not fix

Be honest about the limits. Merging won't fix:

- GIP tenant pool routing bugs (auth-bff bug, separate concern)
- Cookie domain / session propagation across `*.mark8ly.com` subdomains
- Cold-start latency on Knative scale-to-zero (Knative config — set
  `minScale: 1` for hot paths)
- OpenFGA tuple write races (need outbox pattern; see
  [04-auth-and-authz.md](04-auth-and-authz.md))

These get fixed in their own right, regardless of service topology.

## What this collapse does fix

- **Coordination bugs** between services that didn't need to be services in
  the first place: cross-service transactions become local DB transactions.
- **Eventual-consistency races** for tenant creation → admin login: single DB
  read instead of waiting for Pub/Sub propagation.
- **Cold-start cascades:** 30 Knative services scaling from zero on first
  request → 5. ~80% reduction in cold-start surface area.
- **Deploy surface:** ~80% fewer CI pipelines, secret bundles, ArgoCD apps,
  Terraform modules.
- **Debuggability:** one log stream per logical boundary instead of stitching
  trace IDs across 6 services.
- **Local dev:** docker-compose with 6 services is feasible. With 30 it's not.

## Tradeoffs accepted

- **Single-service deploys are gone for marketplace-api.** A bug in coupons
  code blocks shipping fixes from going out. Mitigation: trunk-based dev,
  fast CI, feature flags.
- **One process = one OOM.** If product import balloons memory, the whole
  marketplace-api OOMs. Mitigation: Knative resource limits; move heavy batch
  jobs to a separate worker binary later if/when it bites.
- **Refactoring back to services later is harder than going forward to
  monolith now.** But the current code is already coupled at the DB level —
  you're not getting microservice independence anyway. The cost is theoretical.
- **Team scaling friction** if Mark8ly grows to multiple teams. Non-issue
  today (solo / small team). Revisit when it matters.
