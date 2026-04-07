# Frontend Strategy

## The MFE question — answered: no

The instinct to split admin into a Micro-Frontend (MFE) architecture was
considered and rejected. Recording the reasoning so the question doesn't
come back without new information.

### When MFE makes sense

- Multiple independent teams own different parts of the same UI and need
  independent deploy cadences.
- The app is so large that build times or bundle sizes are hurting
  development velocity *and* code-splitting + lazy loading isn't enough.
- Different parts of the UI have genuinely different tech stacks you can't
  unify.

### Why none of those apply to Mark8ly

- **Solo / small team.** No team-autonomy benefit. MFE's #1 value disappears.
- **763 TS/TSX files in admin** is big but not too big. Linear, the Vercel
  dashboard, and GitHub itself are all bigger and not MFE. Next.js with
  proper route-based code splitting handles this comfortably.
- **All one stack** (Next.js 16 + React 19 + Tailwind + Radix + `@tesserix/web`).
  No stack-divergence problem to solve.
- **Currently broken.** MFE adds *more* moving parts (module federation, shared
  dependency versioning, runtime composition, cross-MFE routing, cross-MFE
  auth state, cross-MFE error boundaries). You'd be adding distributed-systems
  problems to your frontend at the exact moment you're trying to remove them
  from your backend.

### The real costs of MFE

- **Webpack Module Federation** (or similar) — Next.js App Router support is
  still rough. You'd be fighting the framework.
- **Shared state** (auth, tenant, query cache) becomes hard. TanStack Query
  cache doesn't cross MFE boundaries cleanly.
- **Type safety across MFE boundaries** → either ship types as a package
  (back to monorepo packages, which is what the plan already does) or lose
  type safety.
- **Versioning hell.** MFE A wants `@tesserix/web@1.7`, MFE B wants `@1.4`.
- **Bundle size often gets *worse***, not better, because each MFE ships its
  own React/Radix copies unless externalized — and externalizing is fragile.
- **Local dev tax**: spinning up 5 MFEs + their host shell on different ports
  vs `next dev`. Productivity tax every single day.

### What people actually want from MFE — and how to get it without

| Goal | Non-MFE solution |
|---|---|
| Faster builds | Turborepo remote cache + Next.js incremental builds |
| Smaller initial bundle | Route-based code splitting (free in Next.js App Router) + dynamic imports for heavy modules (TipTap, Recharts, dnd-kit) |
| Independent deploys of admin sections | Solo. Don't need this. |
| Logical separation of admin domains | Route groups (`(tenant)/products`, `(tenant)/orders`) — what admin already does |
| Reusable UI across admin/storefront/onboarding | `packages/ui`, `packages/domain-types` |
| Lazy-loading expensive features | `next/dynamic` + Suspense |

### The escape hatch (if admin still hurts after cleanup)

If after the turborepo restructure admin is *still* painful, the next step is
**splitting it into multiple Next.js apps in the same monorepo**:
`apps/admin-core`, `apps/admin-marketing`, `apps/admin-reports`. Each is a
normal Next.js app on its own subdomain or path prefix. That gives ~90% of
MFE benefits with ~10% of the complexity.

**Only do this if real pain shows up after the cleanup**, not preemptively.

## Design system: keep `@tesserix/web`

**Decision: continue using the Tesserix design system (`@tesserix/web`,
`@tesserix/hooks`, `@tesserix/tokens`, `@tesserix/utils`) as the UI
foundation for all three apps.** It's working, it's installed, all three apps
already consume it, and it's built on shadcn/ui + Radix which is exactly the
stack we'd pick if starting fresh.

What this means in practice:

- **Don't redesign anything.** Pages render with the same components they
  render with today. Visual parity is part of "behavioral parity."
- **Don't fork or rewrap `@tesserix/web` components.** If a button needs a
  variant that doesn't exist, add it upstream in the design system, not in
  app code.
- **`packages/ui` in this monorepo is for *composition*, not primitives.**
  Primitives stay in `@tesserix/web`. `packages/ui` holds *application-level*
  shared components — things like `<TenantSwitcher>`, `<OnboardingStepHeader>`,
  `<EmptyState>`, `<DataTable>` — that compose multiple `@tesserix/web`
  primitives into reusable app patterns.
- **Version pinning matters.** The three apps currently use
  `@tesserix/web@1.4.0`, `1.7.0`, and `1.7.1`. **Pin all three to the same
  version** in the new monorepo (root `package.json` or per-app, but
  consistent). Drift causes subtle visual bugs.
- **Public npm registry.** `@tesserix/web` is now published to the public
  npm registry — no auth token, no `.npmrc` config, no GHCR setup. Plain
  `npm install` works in CI and on fresh clones.

## Duplicate code cleanup → shared packages

The frontend duplication observed is the cleanup target. The principle is
the same as the rest of the plan: **wait for the second consumer, then
extract.** But during the onboarding-only slice, write code in a way that
makes future extraction cheap.

**During the onboarding slice (no shared packages yet):**
- App-level shared components live in `apps/onboarding/components/shared/`.
- Domain types live in `apps/onboarding/lib/types/`.
- API client lives in `apps/onboarding/lib/api/`.
- Auth-bff client lives in `apps/onboarding/lib/auth/`.
- Logger / analytics wiring lives in `apps/onboarding/lib/observability/`.

**When `apps/storefront` is added (the second consumer trigger):**
- Promote duplicated components from `apps/onboarding/components/shared/`
  → `packages/ui/`
- Promote duplicated types → `packages/domain-types/`
- Promote API client patterns (typed fetch wrapper, error handling, retry)
  → `packages/api-client/`. Per-service client modules import from here.
- Promote auth-bff client → `packages/auth/`
- Promote logger / analytics → `packages/observability/`

**When `apps/admin` is added:**
- Tenant resolution middleware (Redis-cached, stale-while-revalidate
  pattern) → `packages/tenant/`
- GCP secrets / storage helpers → `packages/gcp/`
- Anything *else* that turns out to be shared by ≥2 apps gets extracted at
  this point.

**Hard rule: nothing gets extracted at consumer #1.** A thing has shared
shape only after you've written it twice and seen the variation. Extracting
at consumer #1 is guesswork; extracting at consumer #2 is informed.

**Soft rule: write consumer #1 code with eventual extraction in mind.** Keep
files cleanly separated by concern. Don't reach into Next.js-specific APIs
from anything that could be reused. Use dependency injection for things like
fetch, logger, config. The cleaner the boundaries during the slice, the
cheaper the eventual extraction.

## The three apps

### `apps/onboarding` (port 4201)

- Smallest. Most isolated. Migrate first.
- ~213 TS/TSX files.
- Stack extras: Drizzle ORM (own CMS DB), Stripe + Razorpay SDKs, OpenPanel +
  PostHog analytics, Zustand, SWR.
- Includes marketing/CMS pages (blog, guides, help center, footer) sharing
  the same Next.js app. **Decision: port these as-is** in the same phase as
  the wizard. They're working today, low-risk, removing them adds friction
  without value.

### `apps/storefront` (port 3200)

- Second-smallest. Public-facing customer storefront.
- ~381 TS/TSX files.
- Stack extras: TanStack Query, Zustand, framer-motion v11, DOMPurify,
  SimpleWebAuthn v9, Headless UI.
- Tenant model: subdomain `{tenant}.mark8ly.com`.

### `apps/admin` (port 3001)

- Biggest, hardest port. ~763 TS/TSX files.
- Stack extras: TanStack Query, TipTap (10+ extensions), dnd-kit, Recharts,
  ioredis, SimpleWebAuthn (passkeys).
- Tenant model: subdomain `{tenant}-admin.mark8ly.com`. Middleware does tenant
  resolution + Redis-cached validation.
- Admin's tickets-related pages and API proxies are **deleted** along with
  `tickets-service`.

## Massive duplication observed (the cleanup target)

| Concern | Admin | Storefront | Onboarding |
|---|---|---|---|
| Shared Radix/UI primitives | huge overlap | huge overlap | partial |
| Shared API client/proxy pattern | yes (`lib/api-handler`) | yes | yes |
| Shared auth-bff client | yes | yes | yes |
| Shared GCP secrets loader | yes | yes | yes |
| Shared tenant middleware | yes (Redis) | yes | n/a |
| Shared logger | yes | yes | yes |
| Shared domain types (products/orders/customers) | yes | yes (heavy overlap) | minimal |
| Shared zod validation schemas | scattered | scattered | scattered |

**~50% of admin's `app/api/*` routes are thin proxies** that forward to
backend services with auth header injection. These collapse to a generated
handler factory once `packages/api-client` exists.

**~80% of storefront's product/order/cart types are duplicated** from admin.
Extracting `domain-types` first prevents re-doing it twice.

## Migration order

1. **Onboarding** — smallest, isolated, only depends on tenant/location/
   auth-bff/tenant-router. Use it to **establish the package conventions**
   (`api-client`, `auth`, `gcp`, `observability`).
2. **Storefront** — second, simpler than admin, validates `domain-types` and
   `tenant` packages.
3. **Admin** — last, biggest, benefits from everything extracted in steps 1–2.

The first slice of work is **only onboarding**. Admin and storefront re-evaluation
happens after onboarding ports cleanly.

## Reuse strategy: copy-then-clean

Copy the old code into the new structure first. Make it compile in the new
layout. Then clean it up in place. Don't try to rewrite from memory or from
looking at the old code in another window — that's how you introduce bugs that
didn't exist before.

For Next.js apps:
1. Copy route groups into `apps/<app>/app/`
2. Copy components into `apps/<app>/components/` initially
3. **Promote shared components to `packages/ui` only when a second app needs
   them**. Don't speculatively extract.
