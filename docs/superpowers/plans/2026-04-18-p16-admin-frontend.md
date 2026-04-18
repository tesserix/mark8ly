# P16 — Mark8ly Admin Frontend for Subscription v2.3 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver the entire merchant-facing admin surface that consumes the v2.3 subscription backend — `/pricing` (public, geo-localized), `/admin/settings/billing` (plan + invoices + payment method), plan-change modal (upgrade/downgrade proration), cancellation flow with save-offer, trial + tax-ID onboarding, failed-payment and payment_action_required banners, arbitrage appeal, downgrade store-block UX, Pro contact-sales + Pro+App purchase + credential upload, and a set of shared subscription UI primitives (`PlanBadge`, `SubscriptionStatusBadge`, currency formatter, 402 error-envelope handler). Everything lives in the Mark8ly paper/ink/moss design system: Source Serif 4 headlines, hairline rules (never bordered cards), left-aligned asymmetric grids, editorial/calm copy. No emoji, no urgency language.

**Architecture:** Next.js 16 App Router under `apps/admin/`. Public `/pricing` route served with Cloudflare `CF-IPCountry` header read by a Next.js middleware to pick the billing currency before RSC render. Everything protected lives under the existing `app/(admin)/` route group, which already runs through `middleware.ts` tenant/auth extraction. All subscription data is fetched through a new `lib/api/subscription/` client (one thin axios/fetch wrapper per backend plan: P3 state, P4 upgrade/downgrade, P5 trial, P6 dunning, P7 cancellation, P8 arbitrage, P9 tax-ID, P10 refunds/proration, P11 webhooks, P12 storefront closed, P13 Pro+App, P14 Apple/Google creds, P15 admin API surface). TanStack React Query owns every fetch with `staleTime: 30_000` for billing data, invalidated explicitly after any mutation. React Hook Form + Zod for every form. `@tesserix/web` for primitives (Input, Label, Select, Dialog, Button, Tabs, Popover). `@repo/ui` gains three new brand components: `PlanBadge`, `SubscriptionStatusBadge`, `Money` (currency formatter reading `billing_currency`). A new `lib/api/client.ts` interceptor handles the `{"error":"subscription_inactive","status":"..."}` 402 envelope (from P3) and redirects to `/admin/settings/billing`. MSW (Mock Service Worker) seeds every admin backend endpoint for Vitest component tests and Playwright runs against a fixture server. Playwright config extends the existing `playwright.config.ts` with six new spec files (pricing, plan-change, cancellation, tax-id, arbitrage, pro-app). Mark8ly tokens are imported once in `apps/admin/app/globals.css` via `@import "@repo/ui/styles/mark8ly-tokens.css"` (already done) — every new component uses `--paper-200`, `--ink-900`, `--moss-700` exclusively.

**Tech Stack:** Next.js 16 App Router, React 19, TypeScript 5.x, TanStack React Query v5, React Hook Form v7 + Zod v3, Tailwind v4, `@tesserix/web` form primitives, `@repo/ui` brand components (Paper/Ink/Moss tokens, Source Serif 4 + Source Sans 3), Playwright v1.57, Vitest, MSW v2, `@hookform/resolvers/zod`.

**Spec:** [`docs/superpowers/specs/2026-04-17-subscription-model-design.md`](../specs/2026-04-17-subscription-model-design.md) — §2 (plans), §3 (pricing), §4.5.1 (downgrade store-block), §4.7 (payment_action_required), §5 (trial), §5.1.1 (migration fast-path), §5.3 (email ramp), §9 (feature matrix), §15.1 (save-offer copy), §17 (state machine), §18.8.1 (arbitrage appeal), §19.3.1 (US/CA attestation), §28 (success criteria #38/#42/#47).

**Depends on:**
- **P1–P15** — every backend plan for the endpoints consumed. Where a backend plan did not explicitly expose an admin endpoint, this plan adds the missing `/admin/...` route to `services/marketplace-api/internal/handlers/admin/` as a passthrough to the underlying service module.
- **Design system:** `mark8ly/packages/ui/src/styles/mark8ly-tokens.css` (Paper/Ink/Moss tokens).
- **`@tesserix/web`** v1.7.1+ for form primitives.

**Related plans:** This is the terminal UI layer. Nothing depends on it.

---

## Scope Check

In scope:

### Group A — Pricing page + plan management

1. Public `/pricing` route with geo-localized currency (Cloudflare `CF-IPCountry` middleware).
2. 4-plan grid (Starter, Growth, Scale, Pro) + Pro+App add-on card.
3. Pro card copy: "From $1,188/yr ($99/mo equivalent), billed annually. Monthly available at $119/mo." + Contact-sales CTA + Download-brief CTA.
4. `/admin/settings/billing` — current plan, billing period, next renewal, payment method card last-4, invoices list with PDF download links.
5. Plan-change modal with upgrade-immediate proration preview + downgrade-at-period-end disclosure + Pro monthly +20% premium disclosure.
6. Cancellation flow: entry → confirm dialog → exit survey → save-offer step (§15.1 verbatim, prospective-only) → final confirmation.
7. Cancellation reversal (save-offer accept or `cancel_scheduled → active`) — single-click.

### Group B — Trial + tax ID onboarding

8. Signup wizard: business-name + country + tax-ID field + migration fast-path evidence upload (WHOIS OR screenshot per §5.1.1).
9. Trial banner (days 60 / 75 / 85) + email usage ramp visualizer (§5.3 timeline).
10. Tax-ID clock-pause indicator with human-readable reason (registry down OR SEA review queue).
11. US/CA business-entity attestation checkbox (§19.3.1) + accept timestamp capture.

### Group C — Failed payment + payment_action_required

12. Failed-payment banner (past_due): retry date + `Add new card` CTA → Stripe Customer Portal.
13. Payment-action-required banner (§4.7, Council finding #3): keeps full admin access, opens hosted invoice URL, shows T-14/T-7/T-1 countdown.
14. Dunning email preview component (internal CSM use; gated behind `isPlatformOwner`).

### Group D — Arbitrage appeal

15. Arbitrage flag banner when `arbitrage_flag = true`: "We've noted a discrepancy" + Resolve CTA.
16. Self-service appeal form (§18.8.1): jurisdiction picker + optional doc upload (reuses existing GCS/media upload path from `lib/brandingUpload.ts`).
17. Status tracker for pending appeal (5-business-day SLA).

### Group E — Store management (downgrade-block UX)

18. Store-close-before-downgrade dialog (§4.5.1): lists every store with Close/Delete choice + in-flight-order counts + download-orders-CSV link.
19. Copy distinguishing Close (keeps slot) vs Delete (frees slot, 60d soft-delete grace).
20. Image-limit grandfathering badge on affected products post-downgrade.

### Group F — Pro + White-label App

21. Pro contact-sales form → marketing webhook (Notion + Slack from a single backend endpoint).
22. Pro+App purchase flow with co-termination preview (exact proration amount + renewal date).
23. Apple + Google credential upload form (API key / service account JSON); stored via backend (P14) — never persisted in browser.
24. White-label app lifecycle status widget on settings page.

### Group G — Shared UI primitives

25. `PlanBadge` — renders plan + add-on (e.g. "Growth · +White-label App").
26. `SubscriptionStatusBadge` — every state (signup / trialing / active / past_due / payment_action_required / cancel_scheduled / expired / store_closed / pending_hard_delete).
27. `Money` — currency-formatted amount reading `billing_currency`.
28. `apiClient` 402 interceptor: `subscription_inactive` → route to `/admin/settings/billing` with toast.

Out of scope:

- Storefront closed page (P12, served by Cloudflare Worker).
- CSM internal tooling (separate project).
- Marketing site outside `/pricing`.
- Auth flow changes (login/logout already exist).
- Subscription webhook ingestion UI (backend-only).
- Admin API surface extensions beyond what each Group requires.

---

## File Structure

### Create — Routes (Next.js App Router)

- `apps/admin/app/pricing/page.tsx` — public pricing page (RSC; reads `CF-IPCountry` from request headers)
- `apps/admin/app/pricing/layout.tsx` — public marketing layout (no admin shell)
- `apps/admin/app/pricing/PricingClient.tsx` — interactive plan selector (client component)
- `apps/admin/app/(admin)/settings/billing/page.tsx` — billing dashboard entry
- `apps/admin/app/(admin)/settings/billing/PlanCard.tsx`
- `apps/admin/app/(admin)/settings/billing/InvoicesList.tsx`
- `apps/admin/app/(admin)/settings/billing/PaymentMethodCard.tsx`
- `apps/admin/app/(admin)/settings/billing/WhiteLabelAppCard.tsx`
- `apps/admin/app/(admin)/settings/billing/plan-change/page.tsx` — modal-compatible route
- `apps/admin/app/(admin)/settings/billing/cancel/page.tsx` — cancellation flow entry (multi-step wizard)
- `apps/admin/app/(admin)/settings/billing/cancel/ConfirmStep.tsx`
- `apps/admin/app/(admin)/settings/billing/cancel/SurveyStep.tsx`
- `apps/admin/app/(admin)/settings/billing/cancel/SaveOfferStep.tsx`
- `apps/admin/app/(admin)/settings/billing/cancel/FinalConfirmStep.tsx`
- `apps/admin/app/(admin)/settings/billing/pro-contact/page.tsx` — Pro contact-sales
- `apps/admin/app/(admin)/settings/billing/pro-app-purchase/page.tsx` — Pro+App co-terminated purchase
- `apps/admin/app/(admin)/settings/billing/pro-app-purchase/CredentialUploadForm.tsx`
- `apps/admin/app/(admin)/settings/arbitrage/page.tsx` — appeal form entry
- `apps/admin/app/(admin)/settings/arbitrage/AppealForm.tsx`
- `apps/admin/app/(admin)/settings/arbitrage/AppealStatus.tsx`
- `apps/admin/app/(admin)/stores/close-before-downgrade/page.tsx` — downgrade block flow

### Create — Signup wizard extensions (onboarding app touches admin via API, wizard UI lives in marketplace-onboarding — BUT the tax-ID re-entry surface lives here because tax IDs are editable post-signup)

- `apps/admin/app/(admin)/settings/tax-id/page.tsx` — tax-ID management
- `apps/admin/app/(admin)/settings/tax-id/TaxIdForm.tsx`
- `apps/admin/app/(admin)/settings/tax-id/ClockPauseBadge.tsx`
- `apps/admin/app/(admin)/settings/attestation/page.tsx` — US/CA business-entity attestation
- `apps/admin/app/(admin)/settings/attestation/AttestationForm.tsx`

### Create — Trial banner shell injection

- `apps/admin/components/shell/banners/TrialBanner.tsx` — days 60 / 75 / 85 variants
- `apps/admin/components/shell/banners/EmailRampVisualizer.tsx` — §5.3 timeline
- `apps/admin/components/shell/banners/FailedPaymentBanner.tsx` — past_due
- `apps/admin/components/shell/banners/PaymentActionRequiredBanner.tsx` — §4.7 T-14/T-7/T-1
- `apps/admin/components/shell/banners/ArbitrageBanner.tsx` — arbitrage_flag = true
- `apps/admin/components/shell/banners/BannerStack.tsx` — priority-ordered stack (one banner at a time max)

### Create — Shared UI primitives (live in `@repo/ui` — promoted)

- `mark8ly/packages/ui/src/subscription/PlanBadge.tsx`
- `mark8ly/packages/ui/src/subscription/PlanBadge.test.tsx`
- `mark8ly/packages/ui/src/subscription/SubscriptionStatusBadge.tsx`
- `mark8ly/packages/ui/src/subscription/SubscriptionStatusBadge.test.tsx`
- `mark8ly/packages/ui/src/subscription/Money.tsx`
- `mark8ly/packages/ui/src/subscription/Money.test.tsx`
- `mark8ly/packages/ui/src/subscription/index.ts` — barrel export

### Create — Dunning preview (CSM-gated)

- `apps/admin/components/settings/DunningEmailPreview.tsx`
- `apps/admin/components/settings/DunningEmailPreview.test.tsx`

### Create — API clients (one file per backend plan surface)

- `apps/admin/lib/api/client.ts` — shared fetch wrapper + 402 interceptor
- `apps/admin/lib/api/subscription/index.ts` — barrel
- `apps/admin/lib/api/subscription/plans.ts` — P3/P4 plan catalogue + feature matrix
- `apps/admin/lib/api/subscription/billing.ts` — P3/P4/P10 current plan + proration preview + apply
- `apps/admin/lib/api/subscription/cancellation.ts` — P7 cancel/revert/save-offer
- `apps/admin/lib/api/subscription/trial.ts` — P5 trial state + email ramp
- `apps/admin/lib/api/subscription/dunning.ts` — P6 payment_action_required + past_due
- `apps/admin/lib/api/subscription/arbitrage.ts` — P8 appeal CRUD
- `apps/admin/lib/api/subscription/taxId.ts` — P9 tax-ID submit + clock-pause status
- `apps/admin/lib/api/subscription/attestation.ts` — P9 US/CA attestation
- `apps/admin/lib/api/subscription/stores.ts` — P4 store-block + close/delete
- `apps/admin/lib/api/subscription/proApp.ts` — P13/P14 Pro+App purchase + credentials
- `apps/admin/lib/api/subscription/invoices.ts` — P10 invoice list + PDF download
- `apps/admin/lib/api/subscription/pricing.ts` — public pricing endpoint (unauthenticated)

### Create — React Query hooks (one per domain, wrapping the API clients)

- `apps/admin/lib/api/subscription/hooks/useBilling.ts` — `useCurrentPlan`, `useInvoices`, `usePaymentMethod`
- `apps/admin/lib/api/subscription/hooks/usePlanChange.ts` — `useProrationPreview`, `useApplyPlanChange`
- `apps/admin/lib/api/subscription/hooks/useCancellation.ts` — `useStartCancellation`, `useSubmitSurvey`, `useAcceptSaveOffer`, `useConfirmCancel`, `useRevertCancel`
- `apps/admin/lib/api/subscription/hooks/useTrial.ts` — `useTrialStatus`, `useEmailRamp`
- `apps/admin/lib/api/subscription/hooks/useDunning.ts` — `usePaymentActionRequired`, `usePastDue`
- `apps/admin/lib/api/subscription/hooks/useArbitrage.ts` — `useArbitrageFlag`, `useSubmitAppeal`, `useAppealStatus`
- `apps/admin/lib/api/subscription/hooks/useTaxId.ts` — `useTaxIdStatus`, `useSubmitTaxId`
- `apps/admin/lib/api/subscription/hooks/useAttestation.ts` — `useAttestation`, `useAcceptAttestation`
- `apps/admin/lib/api/subscription/hooks/useStoreDowngrade.ts` — `useDowngradeBlockList`, `useCloseStore`, `useDeleteStore`
- `apps/admin/lib/api/subscription/hooks/useProApp.ts` — `useProContactSubmit`, `useProAppQuote`, `useProAppPurchase`, `useUploadCredentials`

### Create — Zod schemas

- `apps/admin/lib/api/subscription/schemas/plan.ts`
- `apps/admin/lib/api/subscription/schemas/billing.ts`
- `apps/admin/lib/api/subscription/schemas/cancellation.ts`
- `apps/admin/lib/api/subscription/schemas/trial.ts`
- `apps/admin/lib/api/subscription/schemas/dunning.ts`
- `apps/admin/lib/api/subscription/schemas/arbitrage.ts`
- `apps/admin/lib/api/subscription/schemas/taxId.ts`
- `apps/admin/lib/api/subscription/schemas/attestation.ts`
- `apps/admin/lib/api/subscription/schemas/stores.ts`
- `apps/admin/lib/api/subscription/schemas/proApp.ts`

### Create — Copy (editorial voice centralized)

- `apps/admin/lib/copy/subscription.ts` — all subscription-facing copy strings (single source; enables future i18n)
- `apps/admin/lib/copy/pricing.ts` — pricing-page copy
- `apps/admin/lib/copy/cancellation.ts` — §15.1 save-offer verbatim
- `apps/admin/lib/copy/arbitrage.ts` — §18.8.1 calm/editorial tone

### Create — Utilities

- `apps/admin/lib/geo/countryToCurrency.ts` — map `CF-IPCountry` → billing currency (18 supported countries from spec §3.2)
- `apps/admin/lib/geo/geoMiddleware.ts` — Next.js middleware helper
- `apps/admin/lib/format/money.ts` — `Intl.NumberFormat` wrapper
- `apps/admin/lib/format/date.ts` — `Intl.DateTimeFormat` wrapper
- `apps/admin/lib/subscription/routeToBillingOnInactive.ts` — 402 redirect helper

### Create — Tests

- `apps/admin/tests/unit/api/subscription/*.test.ts` — one per API client (MSW-backed)
- `apps/admin/tests/unit/components/banners/*.test.tsx` — one per banner
- `apps/admin/tests/unit/components/settings/billing/*.test.tsx` — one per panel
- `apps/admin/tests/e2e/pricing.spec.ts`
- `apps/admin/tests/e2e/plan-change.spec.ts`
- `apps/admin/tests/e2e/cancellation.spec.ts`
- `apps/admin/tests/e2e/tax-id.spec.ts`
- `apps/admin/tests/e2e/arbitrage.spec.ts`
- `apps/admin/tests/e2e/pro-app.spec.ts`
- `apps/admin/tests/e2e/fixtures/subscription.ts` — MSW handlers reused across specs
- `apps/admin/tests/msw/handlers/subscription.ts` — Vitest MSW handlers

### Modify

- `apps/admin/middleware.ts` — add `CF-IPCountry` → currency cookie on the public `/pricing` route (no other changes; existing tenant/auth logic untouched)
- `apps/admin/app/(admin)/layout.tsx` — wrap shell with `BannerStack` so all merchant pages see the right banner
- `apps/admin/components/settings/SettingsNav.tsx` — add Billing / Tax ID / Arbitrage entries (respect plan gating — don't show Arbitrage unless `arbitrage_flag === true`)
- `apps/admin/components/shell/PageShell.tsx` — inject `BannerStack` into the top of every page
- `apps/admin/app/globals.css` — confirm Mark8ly tokens import is present (no-op if already imported)
- `apps/admin/playwright.config.ts` — add the six new spec paths to the `testMatch`
- `apps/admin/vitest.config.ts` — extend include patterns for `tests/unit/**`
- `mark8ly/packages/ui/src/index.ts` — barrel-export the three new subscription primitives
- `services/marketplace-api/internal/handlers/admin/routes.go` — register any admin endpoints exposed for this plan (only where a backend plan didn't)

### Delete

- None. Every existing file stays; new surfaces are additive.

---

## Task Overview

Grouped by A–G per spec. Each task follows TDD: component test → implementation → Playwright smoke (for user-facing flows) → commit.

| # | Group | Task | Depends on |
|---|-------|------|-----------|
| 1 | G | `Money` currency formatter primitive | — |
| 2 | G | `PlanBadge` primitive | 1 |
| 3 | G | `SubscriptionStatusBadge` primitive | — |
| 4 | G | `apiClient` with 402 `subscription_inactive` interceptor | — |
| 5 | A | Geo middleware: `CF-IPCountry` → currency cookie | — |
| 6 | A | `/pricing` page (RSC) + 4-plan grid + Pro card copy | 1, 2, 5 |
| 7 | A | Pricing API client + hooks + schemas | — |
| 8 | A | `/admin/settings/billing` page + plan/invoices/payment-method panels | 1, 2, 3, 4 |
| 9 | A | Plan-change modal with proration preview | 8 |
| 10 | A | Cancellation flow: confirm → survey → save-offer → final | 4, 8 |
| 11 | A | Cancellation reversal (single-click) | 10 |
| 12 | B | Signup wizard tax-ID field + migration evidence upload | 4 |
| 13 | B | `TrialBanner` (day 60/75/85 variants) + `EmailRampVisualizer` | 4 |
| 14 | B | Tax-ID page + `ClockPauseBadge` | 4, 12 |
| 15 | B | US/CA attestation page + timestamp capture | 4 |
| 16 | C | `FailedPaymentBanner` (past_due) | 4 |
| 17 | C | `PaymentActionRequiredBanner` (§4.7 full-access) | 4, 16 |
| 18 | C | Dunning email preview (CSM-gated) | 4 |
| 19 | D | `ArbitrageBanner` | 4 |
| 20 | D | Self-service appeal form + doc upload | 4, 19 |
| 21 | D | Appeal status tracker (5-day SLA) | 20 |
| 22 | E | Store-close-before-downgrade dialog + orders CSV link | 4, 9 |
| 23 | E | Close vs Delete copy + image-limit grandfathering badge | 22 |
| 24 | F | Pro contact-sales form + webhook client | 4 |
| 25 | F | Pro+App purchase flow with co-termination preview | 9, 24 |
| 26 | F | Apple + Google credential upload form | 25 |
| 27 | F | White-label app lifecycle status widget | 25, 26 |
| 28 | G | `BannerStack` priority stack + shell wiring | 13, 16, 17, 19 |
| 29 | — | Full-journey Playwright suite (six specs) | all above |
| 30 | — | Accessibility audit (WCAG 2.1 AA) + keyboard + SR sweep | 29 |
| 31 | — | Final code-reviewer + build + commit | all |

---

## Reusable Patterns

### A. Design system discipline (Mark8ly Paper · Ink · Moss)

Every component:
- Uses only `--paper-200` (page bg), `--paper-100` / `--background-elevated` (cards/popovers), `--ink-900` (primary text + CTA), `--ink-700` (secondary text), `--moss-700` (one accent per view: link / focus / primary hover / success).
- Functional tokens allowed: `--signal` (rare info), `--danger` (errors only), `--warning` (amber-bronze; cap at one per page).
- **Hairline rules, never bordered cards.** Section dividers are `border-b border-[var(--hairline)]` — never boxed.
- **Left-aligned, asymmetric.** No centered heroes. Pricing grid uses a 4-col asymmetric split (Starter wide-narrow-narrow-wide is wrong — actual layout: left 2-col plan description + right 2-col plan chooser, mirrored on desktop, stacked on mobile).
- **Source Serif 4** for headlines, display prices, editorial moments — never body.
- **Source Sans 3** for body, UI, form labels.
- Radius 6px default (`rounded-md` with Tailwind token).
- Shadow scale `--shadow-1` (hover) / `--shadow-2` (popover) / `--shadow-3` (modal).
- No emoji. No urgency copy ("Last chance!", "Hurry!", "Limited offer!"). No "Hey there!" greetings. Editorial tone — "We've noted a discrepancy" (not "Uh-oh!"); "Your trial ends in 5 days" (not "Trial expires soon!").

### B. Editorial copy voice

Examples that belong vs don't:

| Situation | Correct (editorial/calm) | Wrong (SaaS-loud) |
|---|---|---|
| Trial expiry | "Your trial ends 18 April. Add a payment method to continue." | "Trial ending soon! Don't lose access!" |
| Failed payment | "Your most recent payment did not go through. We'll retry on 21 April." | "Payment failed! Update your card now!" |
| Payment action | "Your bank needs to confirm this payment. Review the invoice to complete." | "Action required! Click here immediately!" |
| Cancellation | "Your subscription will end on 18 May. You'll keep full access until then." | "Sorry to see you go! 😔" |
| Arbitrage | "We've noted a discrepancy between your registered tax jurisdiction and your customer addresses." | "Fraud detected! Verify now!" |
| Pro CTA | "Built for teams scaling past £10k orders a month. Start a conversation." | "Unlock unlimited power! Book a demo!" |

Every copy string lives in `lib/copy/*.ts` — never inline. Reviewers scan one file to catch voice drift.

### C. React Query conventions

```ts
// Pseudocode — actual types in `lib/api/subscription/hooks/*`
useCurrentPlan() → useQuery({
  queryKey: ['subscription', 'currentPlan', tenantId],
  queryFn: () => subscriptionAPI.getCurrentPlan(),
  staleTime: 30_000,
})

useApplyPlanChange() → useMutation({
  mutationFn: subscriptionAPI.applyPlanChange,
  onSuccess: () => {
    queryClient.invalidateQueries({ queryKey: ['subscription'] })
  },
})
```

Every mutation invalidates the `['subscription']` prefix. Every query keys off `tenantId` (from auth context) — cross-tenant cache leaks are a security bug.

### D. Auth + tenant context

Reuse the existing admin middleware (`apps/admin/middleware.ts`) + `lib/auth/*` + `lib/gip/*`. This plan does **not** change how auth/tenant is extracted; it only consumes the already-populated `tenant_id` / `store_id` / `user_id` on every API call.

### E. Error envelope handler

Every API call goes through `lib/api/client.ts`. On response:

```ts
// Pseudocode
if (response.status === 402) {
  const body = await response.json()
  if (body.error === 'subscription_inactive') {
    toast.error(copy.subscription.inactiveRedirect)
    router.push('/admin/settings/billing')
    throw new SubscriptionInactiveError(body.status)
  }
}
```

Components never handle 402 themselves — the interceptor owns it. If a component specifically needs to surface an inactive-state message (e.g. billing page itself), it queries `useCurrentPlan()` and renders from `data.status`.

### F. Form pattern (React Hook Form + Zod)

```ts
// Pseudocode
const form = useForm<TaxIdInput>({
  resolver: zodResolver(taxIdSchema),
  defaultValues: { country: '', taxId: '', businessName: '' },
})
const submit = useSubmitTaxId()
const onSubmit = (values) => submit.mutate(values, {
  onSuccess: () => toast.success(copy.taxId.submitted),
  onError: (err) => form.setError('taxId', { message: err.message }),
})
```

Schema-first. Server errors map onto form fields by error code. Never `alert()`; always `sonner` toast + inline field error.

### G. Testing conventions

- **Unit (Vitest):** Component renders + key interactions (button click, form submit, banner show/hide based on prop). One file per component. MSW handles network.
- **Integration (Vitest + MSW):** One file per API client; covers happy path + 4xx + 5xx + 402 redirect.
- **E2E (Playwright):** Six specs covering the six canonical flows. Each uses `apps/admin/tests/e2e/fixtures/subscription.ts` for consistent server mock.
- Run commands:
  ```bash
  npm run test:unit        # Vitest
  npm run test:e2e         # Playwright (headless)
  npm run test:e2e -- --ui # Playwright (debug)
  ```

### H. Import hygiene

- Form primitives: `import { Input, Label, Select, Dialog, Button } from '@tesserix/web'`.
- Brand components: `import { PlanBadge, SubscriptionStatusBadge, Money } from '@repo/ui/subscription'`.
- Copy: `import { copy } from '@/lib/copy/subscription'`.
- API hooks: `import { useCurrentPlan } from '@/lib/api/subscription/hooks/useBilling'`.

Never import from `@/components/*` into `@repo/ui` — the dependency direction is app → repo/ui, never the reverse.

---

## Task 1: `Money` currency formatter primitive (Group G)

**Files:**
- Create: `mark8ly/packages/ui/src/subscription/Money.tsx`
- Create: `mark8ly/packages/ui/src/subscription/Money.test.tsx`

**Spec references:** §3.2 (18 supported currencies), §28 success #42 (CAD display).

- [ ] **Step 1: Failing test**

```tsx
import { render, screen } from '@testing-library/react'
import { Money } from './Money'

describe('Money', () => {
  it('renders USD with cents as two decimals', () => {
    render(<Money amount={1188_00} currency="USD" />)
    expect(screen.getByText('$1,188.00')).toBeInTheDocument()
  })

  it('renders CAD explicitly with the CA$ symbol (success #42)', () => {
    render(<Money amount={1498_00} currency="CAD" />)
    expect(screen.getByText('CA$1,498.00')).toBeInTheDocument()
  })

  it('renders INR with ₹ symbol and no thousands separator in Indian grouping', () => {
    render(<Money amount={9900_00} currency="INR" />)
    expect(screen.getByText('₹9,900.00')).toBeInTheDocument()
  })

  it('accepts a locale override', () => {
    render(<Money amount={1188_00} currency="EUR" locale="de-DE" />)
    expect(screen.getByText(/1\.188,00/)).toBeInTheDocument()
  })

  it('handles zero', () => {
    render(<Money amount={0} currency="USD" />)
    expect(screen.getByText('$0.00')).toBeInTheDocument()
  })

  it('strips cents when showCents=false', () => {
    render(<Money amount={1188_00} currency="USD" showCents={false} />)
    expect(screen.getByText('$1,188')).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run — expect FAIL (file doesn't exist)**

```bash
cd mark8ly/packages/ui
pnpm test -- Money
```

- [ ] **Step 3: Implement `Money.tsx`**

```tsx
// Amount is in minor units (cents/paise) — matches backend money representation.
export interface MoneyProps {
  amount: number
  currency: string
  locale?: string
  showCents?: boolean
  className?: string
}

export function Money({ amount, currency, locale, showCents = true, className }: MoneyProps) {
  const formatter = new Intl.NumberFormat(locale ?? 'en-US', {
    style: 'currency',
    currency,
    minimumFractionDigits: showCents ? 2 : 0,
    maximumFractionDigits: showCents ? 2 : 0,
  })
  return <span className={className}>{formatter.format(amount / 100)}</span>
}
```

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Add to barrel export**

```ts
// mark8ly/packages/ui/src/subscription/index.ts
export { Money } from './Money'
export type { MoneyProps } from './Money'
```

- [ ] **Step 6: Commit**

```bash
git add mark8ly/packages/ui/src/subscription/Money.tsx mark8ly/packages/ui/src/subscription/Money.test.tsx mark8ly/packages/ui/src/subscription/index.ts
git commit -m "feat(ui): Money currency formatter with Intl.NumberFormat"
```

---

## Task 2: `PlanBadge` primitive (Group G)

**Files:**
- Create: `mark8ly/packages/ui/src/subscription/PlanBadge.tsx`
- Create: `mark8ly/packages/ui/src/subscription/PlanBadge.test.tsx`

**Spec references:** §2 (plans), §13 (Pro+App add-on).

- [ ] **Step 1: Failing test**

```tsx
describe('PlanBadge', () => {
  it.each([
    ['starter', 'Starter'],
    ['growth', 'Growth'],
    ['scale', 'Scale'],
    ['pro', 'Pro'],
  ])('renders %s plan as %s', (plan, label) => {
    render(<PlanBadge plan={plan} />)
    expect(screen.getByText(label)).toBeInTheDocument()
  })

  it('appends +White-label App when addon present', () => {
    render(<PlanBadge plan="growth" addon="white_label_app" />)
    expect(screen.getByText('Growth')).toBeInTheDocument()
    expect(screen.getByText(/White-label App/)).toBeInTheDocument()
  })

  it('uses moss accent only for Pro', () => {
    const { container } = render(<PlanBadge plan="pro" />)
    // visual: Pro gets the single accent; others are ink-only
    expect(container.firstChild).toHaveClass(/moss/)
  })

  it('is a semantic element with role="status" for screen readers', () => {
    render(<PlanBadge plan="growth" />)
    expect(screen.getByRole('status')).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement `PlanBadge.tsx`**

Editorial direction: hairline outline, Source Sans uppercase tracking-wide. Pro is the only plan with a moss accent; add-on renders as a middot-separated suffix.

```tsx
const PLAN_LABELS = {
  starter: 'Starter',
  growth: 'Growth',
  scale: 'Scale',
  pro: 'Pro',
} as const

const ADDON_LABELS = {
  white_label_app: '+White-label App',
} as const

export function PlanBadge({ plan, addon, className }: PlanBadgeProps) {
  const label = PLAN_LABELS[plan]
  const suffix = addon ? ADDON_LABELS[addon] : null
  return (
    <span
      role="status"
      className={cn(
        'inline-flex items-center gap-1.5 text-xs font-medium tracking-wide uppercase',
        'border border-[var(--hairline)] rounded-md px-2 py-0.5',
        plan === 'pro' && 'border-[var(--moss-700)] text-[var(--moss-700)]',
        className,
      )}
    >
      <span>{label}</span>
      {suffix && <span className="text-[var(--ink-600)]">{suffix}</span>}
    </span>
  )
}
```

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add mark8ly/packages/ui/src/subscription/PlanBadge.tsx mark8ly/packages/ui/src/subscription/PlanBadge.test.tsx
git commit -m "feat(ui): PlanBadge — plan + addon pill with Pro moss accent"
```

---

## Task 3: `SubscriptionStatusBadge` primitive (Group G)

**Files:**
- Create: `mark8ly/packages/ui/src/subscription/SubscriptionStatusBadge.tsx`
- Create: `mark8ly/packages/ui/src/subscription/SubscriptionStatusBadge.test.tsx`

**Spec references:** §17 (state machine), §4.7 (payment_action_required).

- [ ] **Step 1: Failing test — every state renders with correct label and correct semantic variant**

```tsx
describe('SubscriptionStatusBadge', () => {
  it.each([
    ['signup', 'Signup', 'neutral'],
    ['trialing', 'Trialing', 'neutral'],
    ['active', 'Active', 'success'],
    ['past_due', 'Past due', 'warning'],
    ['payment_action_required', 'Action required', 'warning'],
    ['cancel_scheduled', 'Ending', 'neutral'],
    ['expired', 'Expired', 'danger'],
    ['store_closed', 'Store closed', 'danger'],
    ['pending_hard_delete', 'Pending deletion', 'danger'],
  ])('renders %s as label %s with variant %s', (status, label, variant) => {
    render(<SubscriptionStatusBadge status={status} />)
    expect(screen.getByText(label)).toBeInTheDocument()
    expect(screen.getByTestId('status-badge')).toHaveAttribute('data-variant', variant)
  })

  it('uses moss only for active (editorial rule: one accent per view)', () => {
    render(<SubscriptionStatusBadge status="active" />)
    expect(screen.getByTestId('status-badge')).toHaveClass(/moss/)
  })

  it('never uses moss for warning/danger states', () => {
    render(<SubscriptionStatusBadge status="expired" />)
    expect(screen.getByTestId('status-badge')).not.toHaveClass(/moss/)
  })
})
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement `SubscriptionStatusBadge.tsx`** — status → label + variant map. `active` uses moss; `past_due`/`payment_action_required` use warning (amber-bronze); `expired`/`store_closed`/`pending_hard_delete` use danger (oxblood). Everything else is neutral ink.

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git commit -m "feat(ui): SubscriptionStatusBadge covering all 9 states"
```

---

## Task 4: `apiClient` with 402 interceptor (Group G)

**Files:**
- Create: `apps/admin/lib/api/client.ts`
- Create: `apps/admin/tests/unit/api/client.test.ts`
- Create: `apps/admin/lib/subscription/routeToBillingOnInactive.ts`

**Spec references:** P3 §17.3 — HTTP 402 `{"error":"subscription_inactive","status":"expired"}`.

- [ ] **Step 1: Failing test — 402 triggers redirect**

```ts
describe('apiClient', () => {
  it('redirects to /admin/settings/billing on 402 subscription_inactive', async () => {
    server.use(http.get('/api/admin/stores/:id/products', () =>
      HttpResponse.json({ error: 'subscription_inactive', status: 'expired' }, { status: 402 })
    ))
    const routerPush = vi.fn()
    vi.mocked(useRouter).mockReturnValue({ push: routerPush } as never)

    await expect(apiClient.get('/api/admin/stores/s1/products')).rejects.toThrow(SubscriptionInactiveError)
    expect(routerPush).toHaveBeenCalledWith('/admin/settings/billing')
  })

  it('passes through 200 responses unchanged', async () => {
    server.use(http.get('/api/admin/stores/:id', () => HttpResponse.json({ name: 'Example' })))
    const result = await apiClient.get('/api/admin/stores/s1')
    expect(result).toEqual({ name: 'Example' })
  })

  it('throws ApiError with status + code on other 4xx/5xx', async () => {
    server.use(http.get('/api/admin/stores/:id', () => HttpResponse.json({ error: 'not_found' }, { status: 404 })))
    await expect(apiClient.get('/api/admin/stores/s1')).rejects.toMatchObject({ status: 404, code: 'not_found' })
  })
})
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement `apiClient` with interceptor** — thin wrapper around `fetch` with JSON body parsing, tenant-ID header injection (read from cookie), and the 402 short-circuit.

```ts
export class SubscriptionInactiveError extends Error {
  constructor(public readonly subscriptionStatus: string) {
    super(`subscription inactive: ${subscriptionStatus}`)
  }
}

export class ApiError extends Error {
  constructor(public readonly status: number, public readonly code: string, message: string) {
    super(message)
  }
}

async function handleResponse<T>(res: Response): Promise<T> {
  if (res.status === 402) {
    const body = await res.json().catch(() => ({}))
    if (body.error === 'subscription_inactive') {
      routeToBillingOnInactive(body.status)
      throw new SubscriptionInactiveError(body.status)
    }
  }
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: 'unknown' }))
    throw new ApiError(res.status, body.error ?? 'unknown', body.message ?? res.statusText)
  }
  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}

export const apiClient = {
  get: <T>(url: string, init?: RequestInit) => fetch(url, { method: 'GET', ...init, headers: defaultHeaders(init) }).then(handleResponse<T>),
  post: <T>(url: string, body: unknown, init?: RequestInit) => fetch(url, { method: 'POST', body: JSON.stringify(body), ...init, headers: defaultHeaders(init, true) }).then(handleResponse<T>),
  patch: <T>(url: string, body: unknown, init?: RequestInit) => fetch(url, { method: 'PATCH', body: JSON.stringify(body), ...init, headers: defaultHeaders(init, true) }).then(handleResponse<T>),
  delete: <T>(url: string, init?: RequestInit) => fetch(url, { method: 'DELETE', ...init, headers: defaultHeaders(init) }).then(handleResponse<T>),
}
```

- [ ] **Step 4: Implement `routeToBillingOnInactive.ts`** — uses `window.location` if outside React context; otherwise `useRouter().push`.

- [ ] **Step 5: Run — expect PASS**

- [ ] **Step 6: Commit**

```bash
git commit -m "feat(admin): apiClient with 402 subscription_inactive interceptor"
```

---

## Task 5: Geo middleware — `CF-IPCountry` → currency cookie (Group A)

**Files:**
- Modify: `apps/admin/middleware.ts` (extend to read `CF-IPCountry` for `/pricing`)
- Create: `apps/admin/lib/geo/countryToCurrency.ts`
- Create: `apps/admin/lib/geo/countryToCurrency.test.ts`
- Create: `apps/admin/lib/geo/geoMiddleware.ts`

**Spec references:** §3.2 (18 countries with local currency).

- [ ] **Step 1: Failing test — country → currency mapping covers all 18**

```ts
describe('countryToCurrency', () => {
  it.each([
    ['US', 'USD'],
    ['CA', 'CAD'],
    ['GB', 'GBP'],
    ['AU', 'AUD'],
    ['EU' /* falls through to DE/FR/etc */, null],
    ['DE', 'EUR'],
    ['FR', 'EUR'],
    ['IE', 'EUR'],
    ['IN', 'INR'],
    ['BR', 'BRL'],
    ['MX', 'MXN'],
    ['ZA', 'ZAR'],
    ['SG', 'SGD'],
    ['MY', 'MYR'],
    ['ID', 'IDR'],
    ['PH', 'PHP'],
    ['TH', 'THB'],
    ['VN', 'VND'],
    ['NG', 'NGN'],
    ['KE', 'KES'],
  ])('maps %s → %s', (cc, currency) => {
    if (currency) expect(countryToCurrency(cc)).toBe(currency)
  })

  it('defaults to USD for unlisted country (e.g. RU, CN, NK)', () => {
    expect(countryToCurrency('RU')).toBe('USD')
    expect(countryToCurrency('XX')).toBe('USD')
  })

  it('is case-insensitive', () => {
    expect(countryToCurrency('gb')).toBe('GBP')
  })
})
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement `countryToCurrency.ts`** — static map, default USD.

- [ ] **Step 4: Extend `middleware.ts`**

Only modifies the `/pricing` matcher to read `CF-IPCountry` and set a `mk_currency` cookie for the RSC render. Every other path untouched.

```ts
// Inside middleware.ts
if (pathname === '/pricing' || pathname.startsWith('/pricing/')) {
  const cf = request.headers.get('CF-IPCountry') ?? 'US'
  const currency = countryToCurrency(cf)
  const res = NextResponse.next()
  res.cookies.set('mk_currency', currency, { maxAge: 60 * 60 * 24 })
  return res
}
```

- [ ] **Step 5: Commit**

```bash
git commit -m "feat(admin): CF-IPCountry middleware sets mk_currency cookie on /pricing"
```

---

## Task 6: `/pricing` page (RSC) — 4-plan grid + Pro card (Group A)

**Files:**
- Create: `apps/admin/app/pricing/layout.tsx`
- Create: `apps/admin/app/pricing/page.tsx`
- Create: `apps/admin/app/pricing/PricingClient.tsx`
- Create: `apps/admin/app/pricing/PlanTile.tsx`
- Create: `apps/admin/app/pricing/ProAddOnCard.tsx`
- Create: `apps/admin/lib/copy/pricing.ts`

**Spec references:** §2 (plans), §3 (pricing grid), §13 (Pro+App).

- [ ] **Step 1: Spec the copy in `lib/copy/pricing.ts`**

```ts
export const pricingCopy = {
  title: 'Pricing built for how you sell.',
  subtitle: 'Four plans. One clear path from your first order to your millionth.',
  plans: {
    starter: { name: 'Starter', tagline: 'For your first store.' },
    growth:  { name: 'Growth',  tagline: 'For brands finding their footing.' },
    scale:   { name: 'Scale',   tagline: 'For teams running multi-channel.' },
    pro:     {
      name: 'Pro',
      tagline: 'For established retailers.',
      headline: 'From $1,188/yr',
      subhead: '($99/mo equivalent), billed annually.',
      monthly: 'Monthly available at $119/mo.',
    },
  },
  ctas: {
    starter: 'Start free for 90 days',
    growth:  'Start free for 90 days',
    scale:   'Start free for 90 days',
    proContact: 'Start a conversation',
    proBrief:   'Download the brief',
  },
  addOn: {
    name: 'White-label App',
    tagline: 'Your store, in the App Store.',
    description: 'Ship a native Apple and Google app with your brand. Co-terminated with your plan.',
  },
} as const
```

- [ ] **Step 2: Failing Playwright test**

```ts
// tests/e2e/pricing.spec.ts
test('pricing page shows 4 plans + Pro+App add-on with correct copy', async ({ page }) => {
  await page.goto('/pricing')
  await expect(page.getByRole('heading', { level: 1 })).toContainText('Pricing built for how you sell.')
  await expect(page.getByText('Starter')).toBeVisible()
  await expect(page.getByText('Growth')).toBeVisible()
  await expect(page.getByText('Scale')).toBeVisible()
  await expect(page.getByText('Pro')).toBeVisible()
  await expect(page.getByText('From $1,188/yr')).toBeVisible()
  await expect(page.getByText('($99/mo equivalent), billed annually.')).toBeVisible()
  await expect(page.getByText('Monthly available at $119/mo.')).toBeVisible()
  await expect(page.getByRole('link', { name: 'Start a conversation' })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Download the brief' })).toBeVisible()
  await expect(page.getByText('White-label App')).toBeVisible()
})

test('pricing shows CAD when CF-IPCountry=CA', async ({ page, context }) => {
  await context.addCookies([{ name: 'mk_currency', value: 'CAD', domain: 'localhost', path: '/' }])
  await page.goto('/pricing')
  await expect(page.getByText(/CA\$/)).toBeVisible()
})
```

- [ ] **Step 3: Run — expect FAIL**

- [ ] **Step 4: Implement `layout.tsx`** — no admin shell; just the base editorial layout with logo + footer.

- [ ] **Step 5: Implement `page.tsx`** — RSC that reads `cookies().get('mk_currency')` and passes to the client component.

```tsx
import { cookies } from 'next/headers'
import { PricingClient } from './PricingClient'
import { getPricing } from '@/lib/api/subscription/pricing'
import { pricingCopy } from '@/lib/copy/pricing'

export default async function PricingPage() {
  const currency = (await cookies()).get('mk_currency')?.value ?? 'USD'
  const pricing = await getPricing({ currency })
  return (
    <main className="mx-auto max-w-6xl px-6 py-24">
      <header className="mb-16 max-w-2xl">
        <h1 className="font-serif text-5xl tracking-tight text-[var(--ink-900)]">
          {pricingCopy.title}
        </h1>
        <p className="mt-4 text-lg text-[var(--ink-700)]">{pricingCopy.subtitle}</p>
      </header>
      <PricingClient pricing={pricing} currency={currency} />
    </main>
  )
}
```

- [ ] **Step 6: Implement `PricingClient.tsx`** — 4-col grid on `lg:`, stacked below. Asymmetric emphasis: Growth is visually "recommended" via a hairline left-border accent (never a filled card).

- [ ] **Step 7: Implement `PlanTile.tsx`** — Source Serif 4 plan name + tagline + Money price + feature list (6 bullets max, compiled from spec §9 truncated to the 6 most-differentiating per plan) + CTA button. Hairline-rule separators between sections within the tile, no border around the tile itself except a left-aligned hairline.

- [ ] **Step 8: Implement `ProAddOnCard.tsx`** — sits below the 4-plan grid as a single full-width card with a hairline top border; left half editorial copy, right half CTA + price.

- [ ] **Step 9: Run Playwright — expect PASS**

- [ ] **Step 10: Commit**

```bash
git commit -m "feat(admin): /pricing page with 4-plan grid and Pro+App add-on"
```

---

## Task 7: Pricing API client + hooks + schemas (Group A)

**Files:**
- Create: `apps/admin/lib/api/subscription/pricing.ts`
- Create: `apps/admin/lib/api/subscription/schemas/plan.ts`
- Create: `apps/admin/lib/api/subscription/hooks/usePricing.ts`
- Create: `apps/admin/tests/unit/api/subscription/pricing.test.ts`

**Spec references:** §3 (pricing grid), P3 (feature matrix endpoint).

- [ ] **Step 1: Zod schema**

```ts
export const PricingSchema = z.object({
  currency: z.string().length(3),
  plans: z.array(z.object({
    code: z.enum(['starter', 'growth', 'scale', 'pro']),
    name: z.string(),
    priceAnnual: z.number().int().nullable(),
    priceMonthly: z.number().int().nullable(),
    features: z.array(z.string()),
    recommended: z.boolean().optional(),
  })),
  addOns: z.array(z.object({
    code: z.enum(['white_label_app']),
    name: z.string(),
    priceAnnual: z.number().int(),
  })),
})
```

- [ ] **Step 2: Failing test — client parses server response, rejects malformed**

- [ ] **Step 3: Implement `pricing.ts`** — unauthenticated fetch to `/api/public/pricing?currency=USD`.

- [ ] **Step 4: Implement `usePricing` hook** — client-side variant (for pricing changes inside admin, e.g. showing current plan cost to merchant).

- [ ] **Step 5: Run — expect PASS**

- [ ] **Step 6: Commit**

```bash
git commit -m "feat(admin): pricing API client + Zod schema + usePricing hook"
```

---

## Task 8: `/admin/settings/billing` page + panels (Group A)

**Files:**
- Create: `apps/admin/app/(admin)/settings/billing/page.tsx`
- Create: `apps/admin/app/(admin)/settings/billing/PlanCard.tsx`
- Create: `apps/admin/app/(admin)/settings/billing/InvoicesList.tsx`
- Create: `apps/admin/app/(admin)/settings/billing/PaymentMethodCard.tsx`
- Create: `apps/admin/lib/api/subscription/billing.ts`
- Create: `apps/admin/lib/api/subscription/invoices.ts`
- Create: `apps/admin/lib/api/subscription/hooks/useBilling.ts`
- Create: `apps/admin/lib/api/subscription/schemas/billing.ts`
- Create: Unit tests for every panel + API client

**Spec references:** §2 (plans), §10 (invoices), §14 (payment method).

- [ ] **Step 1: Failing tests for panels**

```tsx
describe('PlanCard', () => {
  it('renders current plan, billing period, next renewal', () => {
    render(<PlanCard plan={{ code: 'growth', billingPeriod: 'annual', nextRenewal: '2027-04-18', currency: 'USD', amount: 59900 }} />)
    expect(screen.getByText('Growth')).toBeInTheDocument()
    expect(screen.getByText('Annual')).toBeInTheDocument()
    expect(screen.getByText(/18 April 2027/)).toBeInTheDocument()
    expect(screen.getByText('$599.00')).toBeInTheDocument()
  })

  it('renders SubscriptionStatusBadge for current status', () => {
    render(<PlanCard plan={{ ..., status: 'active' }} />)
    expect(screen.getByRole('status')).toHaveTextContent('Active')
  })

  it('shows "Change plan" action linking to /plan-change', () => {
    render(<PlanCard plan={{ ... }} />)
    expect(screen.getByRole('link', { name: 'Change plan' })).toHaveAttribute('href', '/admin/settings/billing/plan-change')
  })
})

describe('InvoicesList', () => {
  it('lists invoices with date, number, amount, status, PDF link', () => { /* ... */ })
  it('shows empty state when no invoices', () => { /* ... */ })
})

describe('PaymentMethodCard', () => {
  it('shows masked card last-4 + expiry', () => { /* ... */ })
  it('CTA opens Stripe Customer Portal in a new tab', () => { /* ... */ })
})
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement API clients + schemas + hooks**

- [ ] **Step 4: Implement `PlanCard.tsx`** — left-aligned, hairline rule at top. Source Serif 4 plan name, Source Sans 3 everything else. Money component for the amount. Next-renewal formatted in `Intl.DateTimeFormat('en-GB', { day: 'numeric', month: 'long', year: 'numeric' })`.

- [ ] **Step 5: Implement `InvoicesList.tsx`** — table with hairline rules between rows (no borders), download-PDF icon button last column.

- [ ] **Step 6: Implement `PaymentMethodCard.tsx`** — card brand + last-4 + expiry. "Update payment method" CTA opens Stripe Customer Portal URL returned by the backend.

- [ ] **Step 7: Implement `page.tsx`** — suspense boundaries around each panel; errors bubble to `error.tsx`.

```tsx
export default async function BillingPage() {
  return (
    <div className="space-y-12">
      <header>
        <h1 className="font-serif text-3xl text-[var(--ink-900)]">Billing</h1>
        <p className="mt-2 text-[var(--ink-700)]">
          Your plan, payment method, and invoices.
        </p>
      </header>
      <Suspense fallback={<PanelSkeleton />}>
        <PlanCard />
      </Suspense>
      <Suspense fallback={<PanelSkeleton />}>
        <PaymentMethodCard />
      </Suspense>
      <Suspense fallback={<PanelSkeleton />}>
        <InvoicesList />
      </Suspense>
    </div>
  )
}
```

- [ ] **Step 8: Run all unit tests — expect PASS**

- [ ] **Step 9: Commit**

```bash
git commit -m "feat(admin): /settings/billing page with plan, payment method, invoices panels"
```

---

## Task 9: Plan-change modal with proration preview (Group A)

**Files:**
- Create: `apps/admin/app/(admin)/settings/billing/plan-change/page.tsx`
- Create: `apps/admin/app/(admin)/settings/billing/plan-change/PlanChangeFlow.tsx`
- Create: `apps/admin/app/(admin)/settings/billing/plan-change/ProrationPreview.tsx`
- Create: `apps/admin/app/(admin)/settings/billing/plan-change/DowngradeDisclosure.tsx`
- Create: `apps/admin/app/(admin)/settings/billing/plan-change/ProMonthlyDisclosure.tsx`
- Create: `apps/admin/lib/api/subscription/hooks/usePlanChange.ts`
- Create: Unit tests

**Spec references:** §4.1 (upgrade immediate), §4.2 (downgrade at period end), §4.3 (Pro monthly +20% premium), §4.5.1 (store-block on downgrade).

- [ ] **Step 1: Failing tests**

```tsx
describe('ProrationPreview', () => {
  it('shows upgrade immediate charge with prorated credit for unused period', () => {
    render(<ProrationPreview currentPlan="growth" targetPlan="scale" period="annual" />)
    expect(screen.getByText(/You'll be charged/)).toBeInTheDocument()
    expect(screen.getByText(/prorated/)).toBeInTheDocument()
  })

  it('shows downgrade-at-period-end disclosure for lower-tier target', () => {
    render(<ProrationPreview currentPlan="scale" targetPlan="growth" period="annual" />)
    expect(screen.getByText(/Your plan will change on your next renewal/)).toBeInTheDocument()
  })

  it('shows Pro +20% monthly premium when switching from annual Pro to monthly Pro', () => {
    render(<ProrationPreview currentPlan="pro" targetPlan="pro" currentPeriod="annual" targetPeriod="monthly" />)
    expect(screen.getByText(/Monthly billing is 20% higher/)).toBeInTheDocument()
  })

  it('blocks downgrade path when active store count exceeds target limit (§4.5.1)', () => {
    render(<PlanChangeFlow activeStoreCount={3} targetPlan="starter" />)
    expect(screen.getByText(/Close a store before downgrading/)).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement `usePlanChange` hook** — `useProrationPreview(target)` query + `useApplyPlanChange()` mutation.

- [ ] **Step 4: Implement `ProrationPreview.tsx`** — shows exact dollar amount (Money component) + effective date. Editorial copy, no "Save 20%!" banners.

- [ ] **Step 5: Implement `DowngradeDisclosure.tsx`** — "Your plan will change on 18 May 2027. Features outside the new plan will stop working then."

- [ ] **Step 6: Implement `ProMonthlyDisclosure.tsx`** — "Monthly billing is 20% higher than annual. You can switch back at any time."

- [ ] **Step 7: Implement `PlanChangeFlow.tsx`** — plan picker → period picker → proration preview → Confirm. On confirm, mutate and route back to `/admin/settings/billing` with a success toast.

- [ ] **Step 8: Implement `page.tsx`** — renders `PlanChangeFlow` in a `@tesserix/web` Dialog.

- [ ] **Step 9: Run tests — expect PASS**

- [ ] **Step 10: Commit**

```bash
git commit -m "feat(admin): plan-change modal with proration preview + disclosures"
```

---

## Task 10: Cancellation flow (confirm → survey → save-offer → final) (Group A)

**Files:**
- Create: `apps/admin/app/(admin)/settings/billing/cancel/page.tsx`
- Create: `apps/admin/app/(admin)/settings/billing/cancel/CancelFlow.tsx`
- Create: `apps/admin/app/(admin)/settings/billing/cancel/ConfirmStep.tsx`
- Create: `apps/admin/app/(admin)/settings/billing/cancel/SurveyStep.tsx`
- Create: `apps/admin/app/(admin)/settings/billing/cancel/SaveOfferStep.tsx`
- Create: `apps/admin/app/(admin)/settings/billing/cancel/FinalConfirmStep.tsx`
- Create: `apps/admin/lib/api/subscription/cancellation.ts`
- Create: `apps/admin/lib/api/subscription/hooks/useCancellation.ts`
- Create: `apps/admin/lib/copy/cancellation.ts`

**Spec references:** §15 (cancellation flow), §15.1 (save-offer copy verbatim — prospective merchants only), §17 (cancel_scheduled state).

- [ ] **Step 1: Paste §15.1 save-offer copy verbatim into `lib/copy/cancellation.ts`**

```ts
export const cancellationCopy = {
  confirmStep: {
    title: 'Cancel your subscription',
    body: "Your store will stay live until the end of your current billing period. You'll keep full access until then.",
    continueCta: 'Continue',
    keepCta: 'Keep my subscription',
  },
  surveyStep: {
    title: 'Before you go',
    body: "Tell us what didn't work. Two questions, then we'll get you on your way.",
    reasons: [
      { code: 'too_expensive', label: "It's too expensive for me right now" },
      { code: 'missing_features', label: "It's missing features I need" },
      { code: 'switching_to', label: "I'm switching to another platform" },
      { code: 'going_out_of_business', label: "I'm closing my business" },
      { code: 'technical_issues', label: "I had technical problems" },
      { code: 'other', label: 'Something else' },
    ],
    feedbackPlaceholder: 'Anything you want us to hear? (optional)',
    submitCta: 'Submit',
  },
  saveOfferStep: {
    // §15.1 VERBATIM — do not edit (prospective merchants only)
    title: 'Before you cancel',
    body: `We get it — Mark8ly isn't for everyone. Before you go, would one of these make a difference?`,
    offers: [
      /* populated by server based on §15.1 matrix */
    ],
    acceptCta: 'Take this offer',
    declineCta: 'No thanks, cancel my plan',
  },
  finalStep: {
    title: 'Your subscription is scheduled to end',
    body: (date: string) =>
      `Your plan ends on ${date}. You'll keep full access until then. If you change your mind, come back any time.`,
    revertCta: 'Keep my subscription after all',
    doneCta: 'Back to billing',
  },
} as const
```

- [ ] **Step 2: Failing Playwright test** — full four-step journey ending with `cancel_scheduled` state; separate test for save-offer accept path.

```ts
test('cancellation: confirm → survey → save-offer → final (decline offer)', async ({ page }) => {
  await page.goto('/admin/settings/billing/cancel')
  await expect(page.getByRole('heading', { name: 'Cancel your subscription' })).toBeVisible()
  await page.getByRole('button', { name: 'Continue' }).click()

  await expect(page.getByRole('heading', { name: 'Before you go' })).toBeVisible()
  await page.getByLabel("It's too expensive for me right now").check()
  await page.getByRole('button', { name: 'Submit' }).click()

  await expect(page.getByRole('heading', { name: 'Before you cancel' })).toBeVisible()
  await page.getByRole('button', { name: 'No thanks, cancel my plan' }).click()

  await expect(page.getByRole('heading', { name: 'Your subscription is scheduled to end' })).toBeVisible()
})

test('cancellation: accepting save-offer reverts cancel', async ({ page }) => { /* ... */ })
```

- [ ] **Step 3: Run — expect FAIL**

- [ ] **Step 4: Implement `useCancellation` hook** — `useStartCancellation`, `useSubmitSurvey`, `useOfferedSaveOffer`, `useAcceptSaveOffer`, `useConfirmCancel`, `useRevertCancel`.

- [ ] **Step 5: Implement `ConfirmStep.tsx`** — two buttons, left-aligned, Source Serif title.

- [ ] **Step 6: Implement `SurveyStep.tsx`** — radio group (one per reason) + optional textarea. React Hook Form + Zod.

- [ ] **Step 7: Implement `SaveOfferStep.tsx`** — server returns the offer; renders verbatim §15.1 copy. Decline routes to `FinalConfirmStep`; Accept routes back to billing with a success toast.

- [ ] **Step 8: Implement `FinalConfirmStep.tsx`** — confirms `cancel_scheduled` state + shows end date + revert CTA.

- [ ] **Step 9: Implement `CancelFlow.tsx`** — XState or a plain reducer tracking `{ step, surveyAnswers, offer }`.

- [ ] **Step 10: Run Playwright — expect PASS**

- [ ] **Step 11: Commit**

```bash
git commit -m "feat(admin): cancellation flow with save-offer per §15.1"
```

---

## Task 11: Cancellation reversal (single-click) (Group A)

**Files:**
- Modify: `apps/admin/app/(admin)/settings/billing/page.tsx` (show revert banner when `status === 'cancel_scheduled'`)
- Create: `apps/admin/app/(admin)/settings/billing/CancelScheduledBanner.tsx`
- Create: Unit test

**Spec references:** §17.2 (`cancel_scheduled → active`), §15 (reversal).

- [ ] **Step 1: Failing test**

```tsx
describe('CancelScheduledBanner', () => {
  it('shows end date and a single revert CTA', () => {
    render(<CancelScheduledBanner endDate="2027-05-18" />)
    expect(screen.getByText(/18 May 2027/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Keep my subscription/i })).toBeInTheDocument()
  })

  it('calls revert mutation on click and invalidates billing query', async () => {
    const mutate = vi.fn()
    vi.mocked(useRevertCancel).mockReturnValue({ mutate } as never)
    render(<CancelScheduledBanner endDate="2027-05-18" />)
    await userEvent.click(screen.getByRole('button'))
    expect(mutate).toHaveBeenCalled()
  })
})
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement banner** — hairline top border, moss accent text on the date, ink CTA.

- [ ] **Step 4: Wire into billing page** — show only when `currentPlan.status === 'cancel_scheduled'`.

- [ ] **Step 5: Run — expect PASS**

- [ ] **Step 6: Commit**

```bash
git commit -m "feat(admin): cancel_scheduled banner with single-click revert"
```

---

## Task 12: Signup wizard tax-ID field + migration evidence (Group B)

**Files:**
- Create: `apps/admin/lib/api/subscription/taxId.ts`
- Create: `apps/admin/lib/api/subscription/hooks/useTaxId.ts`
- Create: `apps/admin/lib/api/subscription/schemas/taxId.ts`

Note: the signup wizard itself lives in `marketplace-onboarding/`, a sibling Next.js app. This plan delivers the API contracts + a tax-ID management page inside admin (task 14). The wizard shim that submits to this endpoint is scheduled for a small follow-up change in onboarding.

**Spec references:** §5.1 (tax-ID during trial), §5.1.1 (migration fast-path).

- [ ] **Step 1: Zod schema**

```ts
export const TaxIdSubmissionSchema = z.object({
  businessName: z.string().min(2),
  country: z.string().length(2),
  taxId: z.string().min(3),
  migrationEvidence: z.union([
    z.object({ type: z.literal('whois'), domain: z.string() }),
    z.object({ type: z.literal('screenshot'), uploadId: z.string() }),
  ]).optional(),
})
```

- [ ] **Step 2: Failing test — validation rules, API client round-trip via MSW**

- [ ] **Step 3: Implement client + hook**

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git commit -m "feat(admin): tax-ID submission API client + Zod schema"
```

---

## Task 13: `TrialBanner` + `EmailRampVisualizer` (Group B)

**Files:**
- Create: `apps/admin/components/shell/banners/TrialBanner.tsx`
- Create: `apps/admin/components/shell/banners/EmailRampVisualizer.tsx`
- Create: `apps/admin/lib/api/subscription/trial.ts`
- Create: `apps/admin/lib/api/subscription/hooks/useTrial.ts`
- Create: Unit tests

**Spec references:** §5 (trial states), §5.3 (email usage ramp timeline).

- [ ] **Step 1: Failing test — three variants**

```tsx
describe('TrialBanner', () => {
  it('day 60: soft nudge — "You have 30 days left in your trial"', () => { /* ... */ })
  it('day 75: middle urgency — "Your trial ends in 15 days"', () => { /* ... */ })
  it('day 85: final — "Your trial ends in 5 days. Add a payment method to keep your store live."', () => { /* ... */ })
  it('renders EmailRampVisualizer with the current day marker', () => { /* ... */ })
})

describe('EmailRampVisualizer', () => {
  it('renders a timeline with 3 milestones: warm-up / full / post-trial', () => { /* ... */ })
  it('places the current-day marker correctly', () => { /* ... */ })
})
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement `TrialBanner.tsx`** — hairline top border. Source Serif headline for the day count ("30 days"), Source Sans supporting copy. Primary CTA "Add payment method" (ink bg). Editorial, never "Hurry!"

- [ ] **Step 4: Implement `EmailRampVisualizer.tsx`** — horizontal timeline with three milestones (warm-up, full, post-trial). Hairline connector line. Current-day marker uses moss. Single moss dot only.

- [ ] **Step 5: Run — expect PASS**

- [ ] **Step 6: Commit**

```bash
git commit -m "feat(admin): TrialBanner + EmailRampVisualizer (§5.3 timeline)"
```

---

## Task 14: Tax-ID page + `ClockPauseBadge` (Group B)

**Files:**
- Create: `apps/admin/app/(admin)/settings/tax-id/page.tsx`
- Create: `apps/admin/app/(admin)/settings/tax-id/TaxIdForm.tsx`
- Create: `apps/admin/app/(admin)/settings/tax-id/ClockPauseBadge.tsx`
- Create: Unit + e2e tests

**Spec references:** §9 (tax-ID workflow), §5.1 (clock-pause causes: registry down, SEA review).

- [ ] **Step 1: Failing tests**

```tsx
describe('ClockPauseBadge', () => {
  it('renders when clockPaused=true with human-readable reason', () => {
    render(<ClockPauseBadge paused={true} reason="registry_down" />)
    expect(screen.getByText(/Registry verification is temporarily unavailable/)).toBeInTheDocument()
  })

  it('renders SEA review reason with distinct copy', () => {
    render(<ClockPauseBadge paused={true} reason="sea_review" />)
    expect(screen.getByText(/Your submission is in regional review/)).toBeInTheDocument()
  })

  it('renders nothing when paused=false', () => {
    const { container } = render(<ClockPauseBadge paused={false} />)
    expect(container).toBeEmptyDOMElement()
  })
})

test('tax-id: submit, show pending, see clock-pause when backend returns pause reason', async ({ page }) => { /* ... */ })
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement `ClockPauseBadge.tsx`** — warning-level (amber-bronze) pill with icon-less inline dot + reason mapped from codes.

- [ ] **Step 4: Implement `TaxIdForm.tsx`** — country select + tax-ID input + migration evidence upload (WHOIS OR screenshot). Validated with Zod. Submit disabled while pending or mutation in flight.

- [ ] **Step 5: Implement `page.tsx`** — current tax-ID status + form. Reuses `useTaxIdStatus` + `useSubmitTaxId`.

- [ ] **Step 6: Run — expect PASS**

- [ ] **Step 7: Commit**

```bash
git commit -m "feat(admin): /settings/tax-id with clock-pause reason badge"
```

---

## Task 15: US/CA attestation page (Group B)

**Files:**
- Create: `apps/admin/app/(admin)/settings/attestation/page.tsx`
- Create: `apps/admin/app/(admin)/settings/attestation/AttestationForm.tsx`
- Create: `apps/admin/lib/api/subscription/attestation.ts`
- Create: `apps/admin/lib/api/subscription/hooks/useAttestation.ts`

**Spec references:** §19.3.1 (US/CA business-entity attestation + timestamp capture).

- [ ] **Step 1: Failing test**

```tsx
test('attestation: merchant in US sees attestation checkbox + timestamp recorded on accept', async ({ page }) => {
  await page.goto('/admin/settings/attestation')
  await expect(page.getByText(/I confirm that this business is registered/)).toBeVisible()
  await page.getByLabel(/I confirm/).check()
  await page.getByRole('button', { name: 'Confirm' }).click()
  await expect(page.getByText(/Confirmed on \d+/)).toBeVisible()
})
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement `AttestationForm.tsx`** — single checkbox + Confirm button. Server stores the accepted-at timestamp.

- [ ] **Step 4: Implement `page.tsx`** — renders form if `attestation.required && !attestation.accepted`; shows confirmation + timestamp otherwise.

- [ ] **Step 5: Run — expect PASS**

- [ ] **Step 6: Commit**

```bash
git commit -m "feat(admin): US/CA business-entity attestation with timestamp capture"
```

---

## Task 16: `FailedPaymentBanner` (past_due) (Group C)

**Files:**
- Create: `apps/admin/components/shell/banners/FailedPaymentBanner.tsx`
- Create: `apps/admin/lib/api/subscription/dunning.ts`
- Create: `apps/admin/lib/api/subscription/hooks/useDunning.ts`

**Spec references:** §6 (dunning retry schedule), §17 (past_due state).

- [ ] **Step 1: Failing test**

```tsx
describe('FailedPaymentBanner', () => {
  it('renders with retry date and "Add new card" CTA linking to Stripe Customer Portal', () => {
    render(<FailedPaymentBanner retryDate="2027-04-21" portalUrl="https://billing.stripe.com/..." />)
    expect(screen.getByText(/We'll retry on 21 April 2027/)).toBeInTheDocument()
    const cta = screen.getByRole('link', { name: /Add new card/ })
    expect(cta).toHaveAttribute('href', 'https://billing.stripe.com/...')
    expect(cta).toHaveAttribute('target', '_blank')
  })

  it('uses editorial/calm copy (no urgency words)', () => {
    render(<FailedPaymentBanner retryDate="2027-04-21" portalUrl="..." />)
    expect(screen.queryByText(/urgent|immediately|hurry|now/i)).not.toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement** — hairline top border, warning variant (amber-bronze inline dot), Source Serif date, Source Sans body.

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git commit -m "feat(admin): FailedPaymentBanner for past_due state"
```

---

## Task 17: `PaymentActionRequiredBanner` (§4.7) (Group C)

**Files:**
- Create: `apps/admin/components/shell/banners/PaymentActionRequiredBanner.tsx`
- Create: `apps/admin/components/shell/banners/CountdownDisclosure.tsx`

**Spec references:** §4.7, §17.3 (Council finding #3: merchants with `payment_action_required` retain FULL admin access, no read-only).

- [ ] **Step 1: Failing test — critically, verifies the admin does NOT enter read-only mode**

```tsx
describe('PaymentActionRequiredBanner', () => {
  it('renders the banner with hosted invoice URL and countdown (success #38)', () => {
    render(<PaymentActionRequiredBanner hostedInvoiceUrl="https://invoice.stripe.com/..." daysRemaining={14} />)
    expect(screen.getByRole('link', { name: /Review the invoice/ })).toHaveAttribute('href', 'https://invoice.stripe.com/...')
    expect(screen.getByText(/14 days to confirm/)).toBeInTheDocument()
  })

  it.each([
    [14, '14 days'],
    [7,  '7 days'],
    [1,  '1 day'],
  ])('shows countdown %i correctly', (days, expected) => {
    render(<PaymentActionRequiredBanner hostedInvoiceUrl="..." daysRemaining={days} />)
    expect(screen.getByText(new RegExp(expected))).toBeInTheDocument()
  })

  it('uses editorial copy, not urgency', () => {
    render(<PaymentActionRequiredBanner hostedInvoiceUrl="..." daysRemaining={1} />)
    expect(screen.queryByText(/urgent|act now|!/i)).not.toBeInTheDocument()
  })
})

test('success #38: admin with payment_action_required status has full access (not read-only)', async ({ page }) => {
  await page.goto('/admin/stores/s1/products')  // a product admin route
  await expect(page.getByRole('button', { name: /Create product/ })).toBeVisible()  // full access
  await expect(page.getByText(/Your bank needs to confirm/)).toBeVisible()  // but banner still shows
})
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement banner** — warning variant. Editorial copy: "Your bank needs to confirm this payment. Review the invoice to complete." Countdown: "14 days to confirm / 7 days / 1 day remaining."

- [ ] **Step 4: Implement `CountdownDisclosure.tsx`** — shared between banner and settings widget. Reads `remainingDays` prop.

- [ ] **Step 5: Run — expect PASS** (includes the full-access regression)

- [ ] **Step 6: Commit**

```bash
git commit -m "feat(admin): PaymentActionRequiredBanner with countdown + full-access (§4.7)"
```

---

## Task 18: Dunning email preview (CSM-gated) (Group C)

**Files:**
- Create: `apps/admin/components/settings/DunningEmailPreview.tsx`
- Create: Unit test

**Spec references:** §6.2 (dunning email copy); internal tooling.

- [ ] **Step 1: Failing test — gating enforced**

```tsx
describe('DunningEmailPreview', () => {
  it('renders nothing when user is not a platform owner', () => {
    const { container } = render(<DunningEmailPreview />, { wrapper: wrapWithNonPlatformOwner })
    expect(container).toBeEmptyDOMElement()
  })

  it('renders three email templates when platform owner', () => {
    render(<DunningEmailPreview />, { wrapper: wrapWithPlatformOwner })
    expect(screen.getByText(/Day 3 retry/)).toBeInTheDocument()
    expect(screen.getByText(/Day 7 retry/)).toBeInTheDocument()
    expect(screen.getByText(/Day 14 final/)).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement** — renders three email-template previews from `lib/copy/dunning.ts`. Hidden for non-platform-owners via auth context check.

- [ ] **Step 4: Commit**

```bash
git commit -m "feat(admin): dunning email preview (CSM-gated)"
```

---

## Task 19: `ArbitrageBanner` (Group D)

**Files:**
- Create: `apps/admin/components/shell/banners/ArbitrageBanner.tsx`
- Create: `apps/admin/lib/api/subscription/arbitrage.ts`
- Create: `apps/admin/lib/api/subscription/hooks/useArbitrage.ts`
- Create: `apps/admin/lib/copy/arbitrage.ts`

**Spec references:** §18.8 (arbitrage detection), §18.8.1 (self-service appeal).

- [ ] **Step 1: Arbitrage copy in `lib/copy/arbitrage.ts`**

```ts
export const arbitrageCopy = {
  banner: {
    title: "We've noted a discrepancy",
    body: "The tax jurisdiction on your account doesn't match where most of your customers are billed from. Review to keep your account in good standing.",
    cta: 'Resolve',
  },
  appeal: {
    title: "Share your side",
    intro: 'If you believe this is a mistake, tell us a bit about your setup. Our team reviews every appeal within five business days.',
    jurisdictionLabel: 'Your primary operating jurisdiction',
    explainLabel: 'What should we know?',
    uploadLabel: 'Supporting documents (optional)',
    submitCta: 'Submit appeal',
  },
  status: {
    pending: 'Under review',
    approved: 'Resolved',
    rejected: 'Not approved',
  },
} as const
```

- [ ] **Step 2: Failing tests**

```tsx
describe('ArbitrageBanner', () => {
  it('shows only when arbitrageFlag=true', () => { /* ... */ })
  it('uses editorial/calm copy: "We\'ve noted a discrepancy"', () => { /* ... */ })
  it('Resolve CTA links to /admin/settings/arbitrage', () => { /* ... */ })
})
```

- [ ] **Step 3: Run — expect FAIL**

- [ ] **Step 4: Implement banner + API client + hook**

- [ ] **Step 5: Run — expect PASS**

- [ ] **Step 6: Commit**

```bash
git commit -m "feat(admin): ArbitrageBanner with editorial calm copy"
```

---

## Task 20: Self-service appeal form + doc upload (Group D)

**Files:**
- Create: `apps/admin/app/(admin)/settings/arbitrage/page.tsx`
- Create: `apps/admin/app/(admin)/settings/arbitrage/AppealForm.tsx`
- Modify: `apps/admin/lib/brandingUpload.ts` → extract reusable `mediaUpload.ts` if not already generic

**Spec references:** §18.8.1.

- [ ] **Step 1: Failing Playwright test**

```ts
test('arbitrage appeal: pick jurisdiction, upload doc, submit, see pending status', async ({ page }) => {
  await page.goto('/admin/settings/arbitrage')
  await page.getByLabel(/primary operating jurisdiction/i).selectOption('GB')
  await page.getByLabel(/What should we know/i).fill('I operate primarily in the UK.')
  await page.setInputFiles('input[type=file]', 'fixtures/supporting-doc.pdf')
  await page.getByRole('button', { name: 'Submit appeal' }).click()
  await expect(page.getByText(/Under review/)).toBeVisible()
})
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement `AppealForm.tsx`** — jurisdiction select (ISO 3166 alpha-2), textarea, file upload (reuse GCS path), submit mutation.

- [ ] **Step 4: Implement `page.tsx`** — shows `AppealStatus` if appeal already pending; otherwise shows `AppealForm`.

- [ ] **Step 5: Run — expect PASS**

- [ ] **Step 6: Commit**

```bash
git commit -m "feat(admin): self-service arbitrage appeal form with doc upload"
```

---

## Task 21: Appeal status tracker (5-business-day SLA) (Group D)

**Files:**
- Create: `apps/admin/app/(admin)/settings/arbitrage/AppealStatus.tsx`
- Create: Unit test

**Spec references:** §18.8.1 (five business days).

- [ ] **Step 1: Failing test**

```tsx
describe('AppealStatus', () => {
  it('shows pending status with "Decision within 5 business days" body', () => {
    render(<AppealStatus status="pending" submittedAt="2027-04-15" />)
    expect(screen.getByText('Under review')).toBeInTheDocument()
    expect(screen.getByText(/Decision within 5 business days/)).toBeInTheDocument()
  })

  it('shows approved with resolved date', () => { /* ... */ })
  it('shows rejected with reason text and re-appeal CTA', () => { /* ... */ })
})
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement** — hairline-ruled section; status pill (SubscriptionStatusBadge-style but for appeals); timeline note.

- [ ] **Step 4: Commit**

```bash
git commit -m "feat(admin): arbitrage appeal status tracker (5-day SLA)"
```

---

## Task 22: Store-close-before-downgrade dialog (Group E)

**Files:**
- Create: `apps/admin/app/(admin)/stores/close-before-downgrade/page.tsx`
- Create: `apps/admin/app/(admin)/stores/close-before-downgrade/StoreList.tsx`
- Create: `apps/admin/app/(admin)/stores/close-before-downgrade/CloseVsDeleteExplainer.tsx`
- Create: `apps/admin/lib/api/subscription/stores.ts`
- Create: `apps/admin/lib/api/subscription/hooks/useStoreDowngrade.ts`

**Spec references:** §4.5.1 (downgrade-block: stores > target limit must be closed/deleted).

- [ ] **Step 1: Failing Playwright test**

```ts
test('downgrade block: 3 stores, target Starter (1 store) → lists 2 choices', async ({ page }) => {
  // seed: 3 active stores, user attempts Growth → Starter
  await page.goto('/admin/stores/close-before-downgrade?target=starter')
  await expect(page.getByText(/You have 3 stores/)).toBeVisible()
  await expect(page.getByText(/Starter allows 1 store/)).toBeVisible()

  const rows = page.getByRole('row')
  await expect(rows).toHaveCount(4)  // header + 3

  // row 1: in-flight orders count + download CSV link
  await expect(page.getByText(/12 in-flight orders/)).toBeVisible()
  await expect(page.getByRole('link', { name: /Download orders CSV/ })).toBeVisible()

  // each row has Close + Delete choice
  await expect(rows.nth(1).getByRole('button', { name: 'Close' })).toBeVisible()
  await expect(rows.nth(1).getByRole('button', { name: 'Delete' })).toBeVisible()
})
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement API clients + hooks** — `useDowngradeBlockList(target)`, `useCloseStore`, `useDeleteStore`.

- [ ] **Step 4: Implement `StoreList.tsx`** — table: store name | in-flight orders | action buttons (Close, Delete) | CSV download link.

- [ ] **Step 5: Implement `CloseVsDeleteExplainer.tsx`** — hairline-boxed editorial section:
  - **Close**: "Your store stays in your account. You can reopen it later. Slot is retained."
  - **Delete**: "Your store is permanently removed after a 60-day grace period. Slot is freed immediately."

- [ ] **Step 6: Implement `page.tsx`** — coordinates list + explainer. On success, routes back to plan-change modal.

- [ ] **Step 7: Run — expect PASS**

- [ ] **Step 8: Commit**

```bash
git commit -m "feat(admin): close-before-downgrade dialog (§4.5.1)"
```

---

## Task 23: Image-limit grandfathering badge (Group E)

**Files:**
- Create: `apps/admin/components/products/ImageLimitGrandfatheredBadge.tsx`
- Modify: `apps/admin/components/products/ProductImageManager.tsx` (integrate badge)

**Spec references:** §4.6 (image-limit grandfathering post-downgrade).

- [ ] **Step 1: Failing test**

```tsx
describe('ImageLimitGrandfatheredBadge', () => {
  it('renders on products with more images than current plan limit', () => { /* ... */ })
  it('tooltip explains: "This product has more images than your plan normally allows. Existing images are kept."', () => { /* ... */ })
})
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement** — hairline pill, neutral variant, hover tooltip.

- [ ] **Step 4: Wire into `ProductImageManager.tsx`** — shown when `product.imageCount > plan.imageLimit`.

- [ ] **Step 5: Commit**

```bash
git commit -m "feat(admin): image-limit grandfathering badge"
```

---

## Task 24: Pro contact-sales form (Group F)

**Files:**
- Create: `apps/admin/app/(admin)/settings/billing/pro-contact/page.tsx`
- Create: `apps/admin/app/(admin)/settings/billing/pro-contact/ProContactForm.tsx`
- Create: `apps/admin/lib/api/subscription/proApp.ts` (start — Pro contact first)
- Create: `apps/admin/lib/api/subscription/hooks/useProApp.ts` (start)

**Spec references:** §2.4 (Pro plan), §16 (sales qualification).

- [ ] **Step 1: Failing test**

```tsx
test('pro contact: fill form, submit, see confirmation (success #47: NO $49 app-only option)', async ({ page }) => {
  await page.goto('/admin/settings/billing/pro-contact')
  await page.getByLabel(/business name/i).fill('Example Ltd')
  await page.getByLabel(/monthly orders/i).fill('12000')
  await page.getByLabel(/tell us about your setup/i).fill('Migrating from Shopify.')
  await page.getByRole('button', { name: /Start a conversation/ }).click()
  await expect(page.getByText(/We'll be in touch/)).toBeVisible()

  // Hard assertion of success #47: NO $49 app-only option visible anywhere
  await expect(page.getByText(/\$49.*app-only|app-only.*\$49/)).toHaveCount(0)
})
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement form** — business name, monthly order volume, current platform (select), textarea, submit. Posts to marketing webhook endpoint (backend P13 exposes `/api/admin/pro/contact` which fans out to Notion + Slack).

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git commit -m "feat(admin): Pro contact-sales form + marketing webhook"
```

---

## Task 25: Pro+App purchase flow with co-termination preview (Group F)

**Files:**
- Create: `apps/admin/app/(admin)/settings/billing/pro-app-purchase/page.tsx`
- Create: `apps/admin/app/(admin)/settings/billing/pro-app-purchase/PurchaseFlow.tsx`
- Create: `apps/admin/app/(admin)/settings/billing/pro-app-purchase/CoTerminationPreview.tsx`
- Extend: `apps/admin/lib/api/subscription/proApp.ts` + `useProApp.ts`

**Spec references:** §13 (Pro+App co-termination), success #47 (no standalone $49 app-only).

- [ ] **Step 1: Failing test**

```tsx
test('pro+app purchase: preview shows exact proration + co-terminated renewal date', async ({ page }) => {
  await page.goto('/admin/settings/billing/pro-app-purchase')
  // assumes merchant is on Pro annual; remaining months = 8
  await expect(page.getByText(/Co-terminated with your Pro plan/)).toBeVisible()
  await expect(page.getByText(/prorated amount/i)).toBeVisible()
  await expect(page.getByText(/\$\d+/)).toBeVisible()  // exact number
  await expect(page.getByText(/renews on/i)).toBeVisible()

  // success #47: no standalone $49 app-only option
  await expect(page.getByText(/\$49.*app-only|app-only.*\$49/)).toHaveCount(0)
})
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement `CoTerminationPreview.tsx`** — Money for the prorated charge + formatted renewal date matching parent plan. Editorial explanation: "Your White-label App is billed on the same cadence as your Pro plan."

- [ ] **Step 4: Implement `PurchaseFlow.tsx`** — preview → confirm → routes to credential upload step (task 26).

- [ ] **Step 5: Run — expect PASS**

- [ ] **Step 6: Commit**

```bash
git commit -m "feat(admin): Pro+App purchase with co-termination preview"
```

---

## Task 26: Apple + Google credential upload form (Group F)

**Files:**
- Create: `apps/admin/app/(admin)/settings/billing/pro-app-purchase/CredentialUploadForm.tsx`
- Extend: `apps/admin/lib/api/subscription/proApp.ts` → `useUploadCredentials`

**Spec references:** P14 (credential storage — browser never persists raw credentials; upload via signed URL).

- [ ] **Step 1: Failing test — credentials never touch localStorage**

```tsx
describe('CredentialUploadForm', () => {
  it('uploads Apple API key + Google service-account JSON to backend', async () => { /* ... */ })

  it('never writes raw credentials to localStorage or sessionStorage', async () => {
    const form = render(<CredentialUploadForm />)
    // fill + submit
    expect(localStorage.getItem('apple_api_key')).toBeNull()
    expect(sessionStorage.getItem('apple_api_key')).toBeNull()
  })

  it('masks credential preview after entry (shows only final 4 chars)', () => { /* ... */ })
})
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement form** — two upload fields (Apple API key file, Google service account JSON file). Uses backend-issued signed URL (no direct body parse server-side) per P14.

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git commit -m "feat(admin): Apple + Google credential upload (signed URL, never stored client-side)"
```

---

## Task 27: White-label app lifecycle status widget (Group F)

**Files:**
- Create: `apps/admin/app/(admin)/settings/billing/WhiteLabelAppCard.tsx`
- Modify: `apps/admin/app/(admin)/settings/billing/page.tsx` (inject card when add-on active)

**Spec references:** P13 (white-label app lifecycle states: provisioning / built / submitted / live / rejected).

- [ ] **Step 1: Failing test — each lifecycle state renders**

```tsx
describe('WhiteLabelAppCard', () => {
  it.each([
    ['provisioning', 'Provisioning'],
    ['built',        'Ready for submission'],
    ['submitted',    'Under review'],
    ['live',         'Live'],
    ['rejected',     'Needs attention'],
  ])('renders %s as %s', (state, label) => { /* ... */ })

  it('does not render when add-on not purchased', () => { /* ... */ })
})
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement** — hairline-ruled section. SubscriptionStatusBadge-style chip but for app state. Moss only for `live`; warning for `rejected`.

- [ ] **Step 4: Commit**

```bash
git commit -m "feat(admin): white-label app lifecycle status widget"
```

---

## Task 28: `BannerStack` priority stack + shell wiring (Group G)

**Files:**
- Create: `apps/admin/components/shell/banners/BannerStack.tsx`
- Create: `apps/admin/components/shell/banners/bannerPriority.ts`
- Modify: `apps/admin/app/(admin)/layout.tsx` or `apps/admin/components/shell/PageShell.tsx`

**Spec references:** Implicit — only one banner at a time per the editorial "one accent per view" rule.

- [ ] **Step 1: Failing test — priority**

```tsx
describe('BannerStack', () => {
  it('shows only one banner at a time in priority order', () => {
    // order: arbitrage > payment_action_required > past_due > trial > cancel_scheduled
    render(<BannerStack arbitrageFlag={true} status="past_due" />)
    expect(screen.getByText("We've noted a discrepancy")).toBeInTheDocument()
    expect(screen.queryByText(/retry/)).not.toBeInTheDocument()
  })

  it('shows payment_action_required over past_due when both would apply', () => { /* ... */ })
  it('shows trial banner when trial days 60/75/85 and no other condition', () => { /* ... */ })
  it('renders nothing when status=active and no flags', () => { /* ... */ })
})
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement `bannerPriority.ts`** — ordered list of `(condition, banner)` pairs. First match wins.

- [ ] **Step 4: Implement `BannerStack.tsx`** — reads `useCurrentPlan()`, `useArbitrageFlag()`, `useTrialStatus()`, picks the winning banner.

- [ ] **Step 5: Inject into `PageShell.tsx`** so every page shows it at the top.

- [ ] **Step 6: Run — expect PASS**

- [ ] **Step 7: Commit**

```bash
git commit -m "feat(admin): BannerStack priority-ordered shell injection"
```

---

## Task 29: Full-journey Playwright suite

**Files:**
- `apps/admin/tests/e2e/pricing.spec.ts`
- `apps/admin/tests/e2e/plan-change.spec.ts`
- `apps/admin/tests/e2e/cancellation.spec.ts`
- `apps/admin/tests/e2e/tax-id.spec.ts`
- `apps/admin/tests/e2e/arbitrage.spec.ts`
- `apps/admin/tests/e2e/pro-app.spec.ts`
- `apps/admin/tests/e2e/fixtures/subscription.ts`

Most individual tests were already written in earlier tasks. This task ensures:
- Every spec file runs green in isolation.
- Every spec file can run together in one `npx playwright test` invocation without state bleed.
- Fixtures are parameterized so any backend plan can wire in changes without rewriting E2E.

- [ ] **Step 1: Consolidate fixtures into `fixtures/subscription.ts`** — every test uses the same seed function for deterministic merchant state.

- [ ] **Step 2: Run the full suite** — fix flakes (likely: timing on toast disappearance, focus order after modal close, CSS transition timing).

```bash
cd apps/admin && npx playwright test
```

- [ ] **Step 3: Commit**

```bash
git commit -m "test(admin): consolidate subscription E2E fixtures + fix flakes"
```

---

## Task 30: Accessibility audit (WCAG 2.1 AA)

**Files:**
- Create: `apps/admin/tests/e2e/a11y.spec.ts` (if missing) — `@axe-core/playwright` sweep across the new pages

- [ ] **Step 1: Failing test — axe violations on each new page**

```ts
const pages = [
  '/pricing',
  '/admin/settings/billing',
  '/admin/settings/billing/plan-change',
  '/admin/settings/billing/cancel',
  '/admin/settings/tax-id',
  '/admin/settings/attestation',
  '/admin/settings/arbitrage',
  '/admin/stores/close-before-downgrade',
  '/admin/settings/billing/pro-contact',
  '/admin/settings/billing/pro-app-purchase',
]

for (const url of pages) {
  test(`${url} has no a11y violations`, async ({ page }) => {
    await page.goto(url)
    const results = await new AxeBuilder({ page }).analyze()
    expect(results.violations).toEqual([])
  })
}
```

- [ ] **Step 2: Run — expect FAIL on any** and fix:
  - Missing `aria-label` on icon buttons
  - Insufficient contrast (re-check moss on paper — should be 4.5:1 minimum)
  - Missing `role` on status elements
  - Tab order on multi-step flows
  - `prefers-reduced-motion` honored on transitions

- [ ] **Step 3: Keyboard-only smoke through the 3 most critical flows** (cancellation, plan change, pro-app purchase).

- [ ] **Step 4: Screen-reader smoke** (VoiceOver) on pricing + cancellation. Confirm every banner is announced once; confirm form errors are in an `aria-live="polite"` region.

- [ ] **Step 5: Commit**

```bash
git commit -m "test(admin): WCAG 2.1 AA sweep + fixes on all new subscription pages"
```

---

## Task 31: Final code-reviewer + build + commit

- [ ] **Step 1: Lint + typecheck**

```bash
cd apps/admin && npm run lint && npm run typecheck
```

- [ ] **Step 2: Full test suite**

```bash
cd apps/admin && npm run test:unit && npx playwright test
```

- [ ] **Step 3: Production build**

```bash
cd apps/admin && npm run build
```

- [ ] **Step 4: Bundle-size check** — every new page must add ≤ 30 KB gzipped to the route chunk. Large offenders (likely `arbitrage` or `pro-app-purchase` forms) should dynamic-import.

- [ ] **Step 5: Invoke code-reviewer agent** — cross-check all new code against CLAUDE.md design rules:
  - No emojis
  - No urgency copy
  - No new colors outside paper/ink/moss/functional
  - Hairline rules, no bordered cards
  - Left-aligned
  - Source Serif 4 headlines only
  - Copy centralized in `lib/copy/*`

- [ ] **Step 6: Final commit**

```bash
git commit -m "feat(admin): P16 complete — subscription v2.3 UI surfaces"
```

---

## Final Verification

Before marking P16 done, confirm every item below:

### Design system compliance
- [ ] Every new component uses only `--paper-*`, `--ink-*`, `--moss-*` tokens (+ functional `--signal`/`--danger`/`--warning`)
- [ ] No bordered cards — only hairline rules between sections
- [ ] No centered heroes — every layout is left-aligned + asymmetric
- [ ] Source Serif 4 used only for headlines / display prices / editorial moments
- [ ] Source Sans 3 used for all body + UI + labels
- [ ] Zero emoji in code or copy
- [ ] Zero urgency language ("hurry", "act now", "limited time", "!")

### Spec coverage
- [ ] **§3.2 currencies** — 18 supported countries render native currency
- [ ] **§4.5.1 downgrade-block** — dialog blocks + lists stores with orders CSV link
- [ ] **§4.7 payment_action_required** — merchant retains full admin access (success #38)
- [ ] **§5.1.1 migration fast-path** — WHOIS + screenshot evidence upload work
- [ ] **§5.3 email ramp** — timeline visualizer renders current-day marker
- [ ] **§9 feature matrix** — pricing page features align with backend catalogue
- [ ] **§15.1 save-offer copy** — rendered verbatim, prospective-only
- [ ] **§17 state machine** — every status has a badge + correct variant
- [ ] **§18.8.1 arbitrage appeal** — self-service form + 5-day SLA tracker
- [ ] **§19.3.1 US/CA attestation** — checkbox + timestamp capture
- [ ] **§28 #38** — payment_action_required banner + full admin access (E2E asserts)
- [ ] **§28 #42** — CAD currency renders correctly (Money test asserts)
- [ ] **§28 #47** — no "$49 app-only" option exists anywhere (pro-contact + pro-app-purchase E2E assert count 0)

### Testing
- [ ] Unit test coverage ≥ 80% on every new component
- [ ] MSW handlers for every API client
- [ ] Six E2E specs green in isolation AND together
- [ ] Axe sweep zero violations on every new page
- [ ] Keyboard-only smoke passes on 3 critical flows
- [ ] VoiceOver smoke passes on pricing + cancellation

### API contracts
- [ ] Every backend plan endpoint (P1–P15) has a typed client + hook
- [ ] Every mutation invalidates `['subscription']` query key prefix
- [ ] Every request is tenant-scoped via the auth middleware cookie — no cross-tenant cache bleed
- [ ] 402 `subscription_inactive` interceptor routes to `/admin/settings/billing` + toast

### Copy
- [ ] Every copy string lives in `lib/copy/*.ts` — no inline strings in components
- [ ] §15.1 save-offer copy is VERBATIM from spec
- [ ] `lib/copy/cancellation.ts` has a comment marking the verbatim block
- [ ] Editorial tone reviewer (self-check): read every copy file out loud, flag any "hype" phrases

### Build
- [ ] Production build green
- [ ] No new bundle-size regressions (>30 KB gzipped per route)
- [ ] TypeScript strict mode clean
- [ ] No ESLint warnings

---

## What's Unlocked

P16 is the terminal UI layer. After P16 ships:

- The subscription model is complete: backend (P1–P15) + admin UI (P16) all online.
- Merchants can sign up → trial → onboard → pay → upgrade → downgrade → cancel → resurrect entirely self-service.
- CSM team has a visible dunning preview (P18 future work can wire real send capability).
- Pro sales can shepherd high-touch deals through Contact form → Notion → Slack → Stripe Invoice → manual activation.
- Arbitrage appeals are self-service with a visible SLA.
- The product is ready for public pricing announcement.

No downstream plan depends on P16. Future work:
- **Analytics dashboards** (separate project) would read billing events for churn analysis.
- **Multi-language pricing** would extend `lib/copy/pricing.ts` into an i18n bundle.
- **Mobile-native admin** (distant future) would reuse the `@repo/ui/subscription` primitives verbatim.

---

## Execution Handoff

When resuming:

1. **Read this plan header + the most recent completed task's commit message** to find your place.
2. Every task is independent where the graph allows (A/B/C/D/E/F can interleave once Group G primitives are built). Groups G 1–4 must be done first; after that any task can be parallelized.
3. **Always TDD.** If a component has no test, write the test first. If a test is missing for an existing component, fill the gap before adding new behavior.
4. **Design-system sanity check before every commit:** open the rendered page in a browser at `localhost:3002` (admin dev port) and confirm:
   - No bordered cards visible
   - Only one moss accent per view
   - No centered headlines
   - Headlines use Source Serif 4
5. **Copy sanity check:** re-read every string you wrote. If it sounds like a SaaS dashboard, rewrite it until it sounds like an editorial magazine.
6. If backend contracts change mid-execution (any P1–P15 refactor), update the corresponding `lib/api/subscription/*.ts` client + schema first, then any consuming hook + component. MSW handlers and Playwright fixtures follow.
7. If you hit a 402 flow you didn't expect, that's the interceptor working — check the logic but don't paper over it at the component layer.

**Commit cadence:** one commit per task step. Never batch unrelated changes. Each commit message uses the `feat(admin): <short>` or `test(admin): <short>` form.

**Done when:** Final Verification checklist is 100% green AND the full E2E suite runs three times consecutively without flake.
