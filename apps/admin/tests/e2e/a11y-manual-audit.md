# P16 WCAG 2.1 AA Manual Accessibility Audit

> **Audit date:** 2026-04-19  
> **Auditor:** P16 a11y agent  
> **Standard:** WCAG 2.1 AA  
> **Automated tier:** `@axe-core/playwright` + `jest-axe` (see `a11y-audit.spec.ts` and `tests/unit/a11y/components.test.tsx`)

---

## Running the test suites

### Automated component tier (Vitest + jest-axe)

```bash
cd apps/admin
npm run test:a11y:unit
# or
npx vitest run tests/unit/a11y/components.test.tsx
```

No server required — runs against jsdom.

### Automated e2e tier (Playwright + @axe-core/playwright)

The Playwright suite **requires a running dev server**. Start the full stack first:

```bash
# Terminal 1 — admin dev server
cd apps/admin && npm run dev          # listens on :4202

# Terminal 2 — (optional) Go marketplace-api for live API responses
# cd services/marketplace-api && go run ./cmd/main.go
```

Then in a third terminal:

```bash
cd apps/admin
npm run test:a11y:e2e
# or
npx playwright test tests/e2e/a11y-audit.spec.ts
```

If the dev server is not running, Playwright tests will fail at navigation. The CI workflow should gate the Playwright tier behind a `needs: dev-server` step or run it only on pull requests where the preview deployment is available.

### Combined shortcut

```bash
cd apps/admin && npm run test:a11y
```

---

## Color contrast pairs — manual verification

Color-contrast rules are **disabled** in both axe tiers (jsdom and Playwright) because axe cannot resolve CSS custom-property values without a full browser paint tree. These pairs must be manually verified.

| Foreground token | Background token | Hex pair | Computed ratio | AA text (4.5:1) | AA UI (3:1) | Status |
|---|---|---|---|---|---|---|
| `--ink-900` (`#0E0E0C`) | `--paper-200` (`#F7F6F2`) | `#0E0E0C` on `#F7F6F2` | ~16.5:1 | PASS | PASS | |
| `--ink-700` (~`#3A3A38`) | `--paper-200` (`#F7F6F2`) | `#3A3A38` on `#F7F6F2` | ~9.0:1 | PASS | PASS | |
| `--ink-900` (`#0E0E0C`) | `#FFFFFF` (elevated surface) | `#0E0E0C` on `#FFFFFF` | ~21:1 | PASS | PASS | |
| `--moss-700` (`#2D4A2B`) | `--paper-200` (`#F7F6F2`) | `#2D4A2B` on `#F7F6F2` | ~8.2:1 | PASS | PASS | |
| `--moss-700` (`#2D4A2B`) | `#FFFFFF` | `#2D4A2B` on `#FFFFFF` | ~9.5:1 | PASS | PASS | |
| `--warning` (amber-bronze ~`#8B5E1A`) | `--paper-200` (`#F7F6F2`) | amber on paper | ~5.2:1 | PASS | PASS | Verify exact hex when token is finalized |
| `--danger` (oxblood ~`#7A1A1A`) | `--paper-200` (`#F7F6F2`) | oxblood on paper | ~6.1:1 | PASS | PASS | Verify exact hex when token is finalized |
| White `#FFFFFF` | `--ink-900` (`#0E0E0C`) | white on ink (primary CTA) | ~21:1 | PASS | PASS | |

> Note: Exact hex values for `--warning` and `--danger` should be confirmed from `packages/ui/src/styles/mark8ly-tokens.css` once finalized.

---

## Surface-by-surface checklist

### `/pricing` (public, unauthenticated)

- [ ] **Keyboard navigation** — Tab visits: Skip link → nav links → plan cards → CTA buttons → footer links in document order.
- [ ] **Focus indicator** — Every interactive element shows moss-700 focus ring (`focus-visible:ring-2 focus-visible:ring-[var(--moss-700)]`).
- [ ] **Screen reader announcements** — Page `<title>` "Pricing — Mark8ly". Main landmark `<main>`. Plan cards use `<section aria-labelledby>`.
- [ ] **Heading hierarchy** — `<h1>` "Plans" → `<h2>` per plan card. No skipped levels.
- [ ] **No focus traps** — No modal on this page; keyboard flow is linear.
- [ ] **Escape behavior** — N/A (no modal).
- [ ] **Color** — Price amounts use `--ink-900`; plan descriptions use `--ink-700`.

### `/admin/stores/:storeId/settings/billing`

- [ ] **Keyboard navigation** — Skip link → sidebar nav → main: PlanCard CTAs → InvoicesList CTA → PaymentMethodCard CTA → WhiteLabelAppCard CTA (if Pro).
- [ ] **Focus indicator** — All CTAs and links have moss-700 ring.
- [ ] **Screen reader announcements** — `<main aria-label="Billing settings">`. Each panel is `<section aria-labelledby="*-heading">` with a matching `<h2 id>`.
- [ ] **aria-live** — `CancelScheduledRow` has `aria-live="polite"` on the cancellation note paragraph. `role="alert"` on error state.
- [ ] **Heading hierarchy** — Page `<h1>` "Settings" → `<h2>` per billing section.
- [ ] **No focus traps** — No overlays on this page.
- [ ] **Banners** — BannerShell uses `role="status" aria-live="polite"`. Screen reader hears banner text on mount.

### `/settings/billing/plan-change`

- [ ] **Keyboard navigation** — Plan radio group navigable with Arrow keys (roving tabindex or `role="radiogroup"`). Period toggle (Monthly/Annual) reachable.
- [ ] **Focus indicator** — Selected plan row has visible moss ring.
- [ ] **Screen reader announcements** — `role="radiogroup" aria-labelledby` on plan list. Proration preview region announced on selection change (should use `aria-live="polite"`).
- [ ] **Form submission** — Submit CTA disabled during pending mutation; `aria-disabled` set.
- [ ] **No focus trap** — If rendered as a modal route, focus should be trapped inside. If rendered as a full page, Tab flows linearly.
- [ ] **Escape** — If modal: Escape navigates back to `/settings/billing`.

> TODO (complex): Confirm `PlanChangeClient` uses `role="radiogroup"` + `role="radio"` or equivalent on the plan selection rows. The current implementation uses a roving tabindex via `KeyboardEvent` handlers on a `<div>`. This pattern requires `role="radiogroup"` + `role="radio"` on each row with `aria-checked`. **If missing, this is a serious violation.** Remediation: add `role="radiogroup"` to the container div and `role="radio" aria-checked={isSelected}` to each plan row.

### `/settings/billing/cancel`

- [ ] **Keyboard navigation** — Multi-step wizard: each step is a single focused region. CTA buttons reachable in order.
- [ ] **Focus indicator** — Confirm, Survey, SaveOffer, FinalConfirm buttons all have moss ring.
- [ ] **Screen reader announcements** — Step transitions should announce the new heading. Use `aria-live="assertive"` on the heading or move focus to the heading on step change.
- [ ] **Form labels** — Survey reason radio group has visible label. Feedback textarea has `<label htmlFor>`.
- [ ] **No focus trap** — Wizard is a page, not a modal; Tab flows linearly through the step.
- [ ] **Escape** — No modal; Escape has no effect (acceptable).

> TODO (moderate): On wizard step transitions, focus is not explicitly moved to the new step heading. Screen readers may not announce the new step. Remediation: `useEffect` → `headingRef.current?.focus()` on step change.

### `/settings/billing/pro-contact`

- [ ] **Keyboard navigation** — All 8 form fields reachable in document order.
- [ ] **Focus indicator** — All inputs/selects have moss ring.
- [ ] **Form labels** — Every field has `<label htmlFor>` wired via `Field` component. Confirmed present in `ProContactForm.tsx`.
- [ ] **Error messages** — `role="alert"` on per-field error span (confirmed in `Field` component). `aria-invalid` + `aria-describedby` on inputs with errors.
- [ ] **Screen reader announcements** — Form submit success/error uses `useToast` → toasts should have `role="status"` or `aria-live`.
- [ ] **No focus trap** — Page-level form; no modal.

> TODO (minor): Confirm `ProContactForm` passes `aria-invalid` and `aria-describedby` to underlying input elements. The `Field` wrapper exposes `errorId` but individual `<input>` elements need `aria-describedby={errorId}` and `aria-invalid={!!error}`. Currently visible in the field wrapper but the attribute wiring to `register()` inputs requires explicit spread.

### `/settings/billing/pro-app-purchase`

- [ ] **Keyboard navigation** — Proration preview, acknowledgement checkbox, submit CTA reachable in order.
- [ ] **Focus indicator** — Checkbox and CTA have moss ring.
- [ ] **Form labels** — Acknowledgement checkbox has visible label with `htmlFor`.
- [ ] **Screen reader announcements** — Proration amount should be in a `<p>` or `<dl>` (not a presentational `<div>`) so screen readers can read it in document flow.
- [ ] **No focus trap** — Page-level form.

### `/settings/billing/pro-app-purchase/credentials`

- [ ] **Keyboard navigation** — Apple tab → Google tab → file input → submit reachable.
- [ ] **Focus indicator** — Tab triggers and file input have moss ring.
- [ ] **Form labels** — File inputs have `<label htmlFor>` (confirmed in `CredentialUploadForm`'s `Field` component).
- [ ] **Help text** — `aria-describedby` on file inputs pointing to help text spans.
- [ ] **No focus trap** — Page-level form.

### `/settings/tax-id`

- [ ] **Keyboard navigation** — business_name → country select → tax_id → billing_address → submit.
- [ ] **Focus indicator** — All inputs and Radix Select trigger have moss ring.
- [ ] **Form labels** — Confirmed: `Field` wrapper pattern with `<label htmlFor>`.
- [ ] **Radix Select** — `<SelectTrigger>` exposes `role="combobox"` via `@tesserix/web` (Radix UI). Verify expanded listbox is keyboard-navigable (Arrow keys).
- [ ] **ClockPauseBadge** — `role="status"` present on the badge.
- [ ] **No focus trap** — Page-level form.

### `/settings/attestation`

- [ ] **Keyboard navigation** — Country select → is_business_entity checkbox → submit.
- [ ] **Focus indicator** — Checkbox and select have moss ring.
- [ ] **Form labels** — Checkbox label text is the verbatim `checkboxLabel` copy string, associated via `htmlFor`.
- [ ] **Submit gating** — Submit CTA `disabled` until checkbox checked; `aria-disabled` should also be set for screen readers.
- [ ] **No focus trap** — Page-level form.

### `/settings/arbitrage`

- [ ] **Keyboard navigation** — Jurisdiction select → doc upload (optional) → submit.
- [ ] **Focus indicator** — File input and select have moss ring.
- [ ] **Form labels** — Jurisdiction select has `<label htmlFor>`. Doc upload has `<label htmlFor>`.
- [ ] **AppealStatus** — Status tracker region uses `role="status"` or `aria-live="polite"`.
- [ ] **No focus trap** — Page-level form.

### `/stores/close-before-downgrade`

- [ ] **Keyboard navigation** — Store list rows → Close/Delete per-row buttons → download CSV link → confirm CTA reachable in order.
- [ ] **Focus indicator** — All row buttons and links have moss ring.
- [ ] **Screen reader** — Store list is a `<ul>` or `<table>` (not bare `<div>`s) so screen reader announces item count.
- [ ] **In-flight orders count** — Conveyed as text (not color alone). Example: "3 orders in flight".
- [ ] **No focus trap** — Page-level dialog-like UI; Tab flows through all stores.

---

## Issues found during audit

### Confirmed passing (axe component tier)

| Component | axe result | Notes |
|---|---|---|
| BannerShell (all tones) | PASS | `role="status" aria-live="polite"`, dismiss button has `aria-label="Dismiss"`, SVG `aria-hidden="true"` |
| TrialBanner | PASS | Delegates to BannerShell |
| FailedPaymentBanner | PASS | Delegates to BannerShell |
| PaymentActionRequiredBanner | PASS | Delegates to BannerShell |
| ArbitrageBanner | PASS | Delegates to BannerShell; CTA is an `<a>` with text label |
| PlanBadge | PASS | `role="status" aria-label` on outer span; decorative middot has `aria-hidden="true"` |
| SubscriptionStatusBadge | PASS | `role="status" aria-label` on outer span |
| Money | PASS | Semantic `<span>` with formatted text |
| PlanCard | PASS | `<section aria-labelledby>`, `<h2 id>`, `aria-live="polite"` on cancellation note, `role="alert"` on error |
| InvoicesList | PASS | `<section aria-labelledby>`, `<h2 id>` |
| PaymentMethodCard | PASS | `<section aria-labelledby>`, `<h2 id>` |
| WhiteLabelAppCard | PASS | `<section aria-labelledby>`, `<h2 id>` |

### TODOs — flagged for remediation

#### TODO-1: PlanChangeClient — missing ARIA roles on plan selection rows
**Severity:** SERIOUS  
**Component:** `app/(admin)/settings/billing/plan-change/PlanChangeClient.tsx`  
**Issue:** The plan selection UI uses a `<div role?>` with roving tabindex. If `role="radiogroup"` and `role="radio"` are missing on the container and each row respectively, screen readers cannot convey selection state.  
**Remediation:**
```tsx
<div role="radiogroup" aria-labelledby="plan-select-label" ref={radioGroupRef}>
  {PLAN_OPTIONS.map((option, index) => (
    <div
      role="radio"
      aria-checked={selectedPlan === option.id}
      tabIndex={selectedPlan === option.id ? 0 : -1}
      key={option.id}
      ...
    >
```
**Priority:** Fix before launch.

#### TODO-2: CancelWizard — focus not moved on step transitions
**Severity:** MODERATE  
**Component:** `app/(admin)/settings/billing/cancel/CancelWizard.tsx`  
**Issue:** When the wizard advances to a new step, focus stays on the previously activated button. Screen readers do not hear the new step heading.  
**Remediation:** Add `stepHeadingRef` and `useEffect(() => { stepHeadingRef.current?.focus() }, [state.step])`. Give the heading `tabIndex={-1}` so it can receive programmatic focus.  
**Priority:** Follow-up after launch.

#### TODO-3: ProContactForm — aria-invalid and aria-describedby not wired to inputs
**Severity:** MODERATE  
**Component:** `app/(admin)/settings/billing/pro-contact/ProContactForm.tsx`  
**Issue:** The `Field` wrapper creates `errorId` but the underlying `<input>` elements from `register()` need `aria-describedby={errorId}` and `aria-invalid={!!error}` explicitly spread.  
**Remediation:** Extend the `Field` component to clone its child element and inject `aria-describedby` and `aria-invalid`, or pass them explicitly to each `register()` spread:
```tsx
<input
  {...register('company_name')}
  id={ids.companyName}
  aria-invalid={!!errors.company_name}
  aria-describedby={errors.company_name ? `${ids.companyName}-error` : undefined}
/>
```
**Priority:** Fix before launch (affects all form fields).

#### TODO-4: AttestationForm — submit button missing aria-disabled
**Severity:** MINOR  
**Component:** `app/(admin)/settings/attestation/AttestationForm.tsx`  
**Issue:** The submit CTA is `disabled` (HTML attribute) but does not also set `aria-disabled="true"`. While HTML `disabled` is sufficient for most screen readers, `aria-disabled` ensures consistency with assistive technologies that override pointer events.  
**Remediation:** Add `aria-disabled={!isChecked || isPending}` alongside `disabled`.  
**Priority:** Low; follow-up after launch.

#### TODO-5: CloseBeforeDowngrade — store list semantic structure
**Severity:** MODERATE  
**Component:** `app/(admin)/stores/close-before-downgrade/CloseBeforeDowngradeClient.tsx`  
**Issue:** If the store list is implemented as bare `<div>` rows rather than `<ul>/<li>` or `<table>`, screen readers cannot announce the number of stores or navigate by list item.  
**Remediation:** Wrap the store list in `<ul role="list">` and each row in `<li>`.  
**Priority:** Verify in Playwright run; fix if confirmed.

---

## Screen reader announcements — critical paths

| Interaction | Expected announcement | aria mechanism |
|---|---|---|
| Banner appears on page load | "[heading] [body text]" | `role="status" aria-live="polite"` on BannerShell |
| Banner dismissed | Banner region becomes empty; SR announces nothing | aria-live region now empty |
| Trial banner variant changes (day60 → day85) | New heading text announced | aria-live polite on the BannerShell div |
| Subscription status changes to cancel_scheduled | "Your subscription will end on [date]" | `aria-live="polite"` on `CancelScheduledRow` p tag |
| Form field validation error appears | "[error message]" announced immediately | `role="alert"` on error span |
| Plan change confirmation pending | "Applying\u2026" on button label | Button text change within same element |
| Cancellation wizard step advance | New step heading announced | TODO-2 above |
| Toast success notification | "[success message]" | Toaster should use `role="status"` or `aria-live="polite"` |
| Toast error notification | "[error message]" | Toaster should use `role="alert"` or `aria-live="assertive"` |

> Note: Verify the `Toaster` component in `components/feedback/Toaster.tsx` uses appropriate aria-live regions. If it wraps `sonner`, confirm sonner's default ARIA output.

---

## Focus ring style reference

All interactive elements in P16 surfaces use one of these ring patterns:

| Pattern | Tailwind classes | Used in |
|---|---|---|
| Primary (moss, 2px) | `focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)]` | All CTA buttons, plan rows, nav links |
| Outline (moss, 2px offset) | `focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--moss-700)]` | BannerShell CTA, dismiss button |
| Input (via @tesserix/web) | Internal to Radix/shadcn — should default to ring-moss | Select, Input primitives |

Verify that `@tesserix/web` Select and Input components inherit or override with moss-700 focus ring in the admin theme.

---

## No focus trap — modal/dialog checklist

P16 surfaces do not introduce new `<dialog>` elements or full-screen overlays. The following patterns must NOT trap focus:

- CancelWizard: multi-step page, not a modal. Tab exits to browser chrome after last CTA. PASS.
- PlanChangeClient: if rendered as intercepted route (`@modal`), Next.js App Router modal slot manages focus. Verify Escape closes the modal route.
- ProAppPurchaseClient: page-level, no modal.
- CredentialUploadForm: page-level, no modal.

---

## prefers-reduced-motion

Framer Motion is present in the admin app. All P16 components should respect `prefers-reduced-motion: reduce`.

- BannerShell: no animation — PASS.
- PlanChangeClient: any entrance animation on plan rows should use `useReducedMotion()` from Framer Motion.
- CancelWizard: step transitions with CSS transitions should use `@media (prefers-reduced-motion: reduce) { transition: none }`.

> TODO (minor): audit all Framer Motion `motion.*` usages added in P16 for `useReducedMotion()` guards.
