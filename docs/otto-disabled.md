# Otto support chat — currently hidden from the UI

Otto is the real-time customer↔staff support chat service. As of this
doc the backend is fully implemented + deployed, but both the
customer-facing widget (storefront) and the moderator inbox (admin)
are **hidden from their respective apps** until we're ready to launch
publicly. Everything is left in-tree so re-enabling is a small,
surgical change.

## What's hidden

| Surface | What changed | Where |
|---|---|---|
| Storefront widget | `OttoSupportChat` import + render commented out | `apps/storefront/app/layout.tsx` |
| Admin nav — Live chat | Nav entry commented out | `apps/admin/components/shell/AdminShell.tsx` (the `support` group) |
| Admin nav — Audit log | Nav entry commented out | same file |
| Admin route `/support/live-chat` | Returns `notFound()` so direct URL hits also 404 | `apps/admin/app/(admin)/support/live-chat/page.tsx` |
| Admin route `/support/audit-log` | Returns `notFound()` | `apps/admin/app/(admin)/support/audit-log/page.tsx` |

## What's still running

- **otto service** (`services/otto/`) — the Go microservice is
  deployed and healthy. Customer-side and admin-side REST + WS
  endpoints are all live. You can verify with
  `curl https://<storefront>/api/otto/resume` (returns
  `{"conversation":null}` when unauthenticated).
- **Mongo collections** — `conversations`, `messages`, `otp_codes`,
  `staff_availability`, `otto_audit`, `sessions`. Any existing case
  data is preserved.
- **Inactivity sweeper** — still ticking every 60s inside the otto
  pod. No visible customer impact since no one can open new cases.
- **Shared widget package** (`packages/otto-widget`) — still built
  and published as `@repo/otto-widget`. No downstream consumers.

## Re-enabling checklist

1. **Storefront widget** —
   `apps/storefront/app/layout.tsx`: uncomment the `OttoSupportChat`
   import line and the `<OttoSupportChat ... />` element inside
   `<CustomerAuthProvider>`.
2. **Admin nav** —
   `apps/admin/components/shell/AdminShell.tsx`: inside the
   `support` group, uncomment the two child entries
   (`Live chat`, `Audit log`).
3. **Admin pages** — in each of:
   - `apps/admin/app/(admin)/support/live-chat/page.tsx`
   - `apps/admin/app/(admin)/support/audit-log/page.tsx`

   Remove the `notFound();` call, uncomment the `getServerSessionContext`
   import, uncomment the real `return (...)` JSX.
4. Deploy as normal — nothing in Helm, ArgoCD, or tesserix-k8s needs
   to change. The backend pods and Istio routes are unchanged.

## Why this shape

- **No feature-flag plumbing** — a prod flag check on every request
  is overkill for a single-hide-for-one-release decision. Four
  comment blocks + two `notFound()` calls are easy to review,
  easier to revert, and don't add any runtime cost.
- **Backend stays up** — shutting it down would save ~one pod's
  worth of resources but requires an ArgoCD change and loses the
  sweeper, index state, and any data created during internal
  dogfooding. The saved compute isn't worth the operational
  friction.
- **Routes 404 instead of redirecting** — `notFound()` hits the
  admin's generic not-found page which respects tenant + auth
  scoping. A redirect would leak "this URL used to be live" info
  to anyone who previously bookmarked it.

## Follow-ups before we actually launch

- Wire a feature flag (`features.otto.enabled`) through the tenant
  settings service so Otto can be rolled out per-store rather than
  globally.
- Audit whether any tenants actually *want* a self-serve support
  channel — some merchants prefer email-only.
- Playwright spec `playwright-tests/mark8ly/admin/13-otto-admin.spec.ts`
  will start failing once re-enabled with the new flag gate — update
  the spec to seed the flag before running.
- Decide what happens to in-flight cases stored in Mongo from the
  internal-test period. Purge? Migrate? Leave? (Currently left.)
