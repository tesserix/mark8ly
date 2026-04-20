---
title: Plan-gate enforcement gaps — remediation plan
date: 2026-04-20
status: pending
owner: unassigned
---

# Plan-gate enforcement gaps

Audit of the v2.3 feature-matrix (`services/marketplace-api/internal/plangate/matrix.go`) vs. actual runtime enforcement in handler code. Matrix is spec-aligned; enforcement has gaps.

## Summary of gaps

| # | Feature | Spec tier | Status | Impact |
|---|---------|-----------|--------|--------|
| P0.1 | `FeatureFullAPI` | Pro only | Not enforced — comment claims Pro-only gating but every API-key route uses `RequireFeature(FeatureReadAPI)`. No callsite found for `FeatureFullAPI` in the codebase. | Studio merchants can mint API keys with write scopes. Revenue leak + compliance risk. |
| P0.2 | `FeatureCustomCSS` | Studio+ | Not enforced — no plangate check in storefront branding/theme write handlers. | Trial/Starter merchants can POST custom CSS. Revenue leak. |
| P0.3 | `FeatureCustomCodeInjection` | Pro only | Not enforced — same as above. | Trial/Starter/Studio merchants can inject custom `<script>` tags. Revenue + security risk (XSS surface). |
| P1.1 | `FeatureImagesPerProduct` | per-plan, grandfathered | Helper `plangate.ImagesAllowed()` exists and is unit-tested, but has zero production callers. | Any merchant can attach arbitrary image counts per product, ignoring plan caps. |
| P1.2 | `FeatureWhiteLabelApp` | Pro + add-on | `WhiteLabelAppEnabled()` helper is consulted by the add-on purchase flow, but the storefront **render path** is not verified to gate on it. | Needs spot-check — if the storefront serves branded-app endpoints without checking the add-on flag, a Pro merchant who cancelled the add-on could still serve branded surfaces. |
| P2 | `TransactionalEmails` | Unlimited/Negotiated | No runtime meter. Spec says "fair-use" not hard cap. | Low risk; deliberate. Flag for future metering work. |
| P2 | `UptimeSLA` + support tiers | per-plan | Display-only, no runtime surface. | Expected. No action. |
| P2 | Trivial-open features | all plans = 1 | Matrix entries are `1` for every plan — gate is a no-op by design. | Low risk; document as forward-compat hedges. |

## What IS enforced today (for reference)

| Feature | Where | Notes |
|---------|-------|-------|
| `FeatureReadAPI` | `internal/handlers/admin/apikeys_handler.go:261-273` | Middleware on API-key CRUD |
| `FeatureSSO` | `internal/handlers/admin/routes.go:117` | `RequireFeatureByTenant` on `/admin/tenants/:id/sso/*` |
| `FeatureStores` (cap) | `internal/subscription/planchange/{preflight,downgrade,cron}.go` | Blocks downgrade when over cap |
| `FeatureCampaignEmailsPerMonth` (quota) | `internal/campaignbudget/{ramp,recompute}.go` | Feeds campaign budget caps |

## Remediation — P0 fixes (~25 LOC total)

### P0.1 — Gate API-key writes on `FeatureFullAPI`

**File:** `services/marketplace-api/internal/handlers/admin/apikeys_handler.go`

Current state (every route under `/api-keys` gates on `FeatureReadAPI`):
```go
g.POST("", plangate.RequireFeature(h.resolver, plangate.FeatureReadAPI, logger), h.Create)
g.POST("/:id/rotate", plangate.RequireFeature(h.resolver, plangate.FeatureReadAPI, logger), h.Rotate)
g.POST("/:id/revoke", plangate.RequireFeature(h.resolver, plangate.FeatureReadAPI, logger), h.Revoke)
```

Desired: split read vs. write gates.
```go
g.GET("", plangate.RequireFeature(h.resolver, plangate.FeatureReadAPI, logger), h.List)
g.GET("/:id", plangate.RequireFeature(h.resolver, plangate.FeatureReadAPI, logger), h.Get)
g.POST("", plangate.RequireFeature(h.resolver, plangate.FeatureFullAPI, logger), h.Create)
g.POST("/:id/rotate", plangate.RequireFeature(h.resolver, plangate.FeatureFullAPI, logger), h.Rotate)
g.POST("/:id/revoke", plangate.RequireFeature(h.resolver, plangate.FeatureFullAPI, logger), h.Revoke)
```

Also verify: any **minted API key** with write scopes should be rejected at key-creation time if the plan doesn't permit `FeatureFullAPI`. The comment on `routes.go:676` hints at "service layer enforces for write scopes (Pro+)" — confirm the scope-allowlist step actually consults `plangate.IsAllowed(plan, FeatureFullAPI)` and rejects `write:*` scopes when false. If it doesn't, add that check in `apikeys/service.go` at the `Create()`/`Rotate()` entrypoints.

**Test:** Studio merchant → POST `/api-keys` → expect 403 with `upgrade_to_pro` hint.

### P0.2 — Gate storefront theme writes on `FeatureCustomCSS`

**Find the handler first:** `grep -rn "custom_css\|CustomCSS\|theme.*css" internal/handlers/` — expected in the branding or theme handler (`branding.go` or similar).

Add middleware:
```go
branding.POST("/theme/css",
    plangate.RequireFeature(h.resolver, plangate.FeatureCustomCSS, logger),
    deps.AuthzMiddleware.RequireTenantRelation(authz.BrandingEditRole),
    deps.BrandingHandler.UpsertCustomCSS)
```

**Test:** Starter merchant → POST theme CSS → 403 with `upgrade_to_studio` hint.

### P0.3 — Gate custom-code-injection writes on `FeatureCustomCodeInjection`

**Find the handler:** same area as P0.2. Apply the same pattern, gating on `FeatureCustomCodeInjection` (Pro only).

Given this is an XSS surface, also audit:
- Is the injected code HTML-escaped at render time, or served raw?
- Is there a CSP header that would block inline scripts regardless?

If the answer to either is "no", the gate is not just a revenue concern — it's a security one.

**Test:** Studio merchant → POST custom-code-injection → 403 with `upgrade_to_pro` hint.

## Remediation — P1 fixes

### P1.1 — Wire `ImagesAllowed()` into product-media upload

**File:** likely `services/marketplace-api/internal/handlers/admin/products.go` (or a media sub-handler).

At the media-upload handler, before persisting, count existing images for the product and compare:
```go
currentCount, _ := h.productRepo.CountImages(ctx, productID)
lastPlanChangeAt := sub.LastPlanChangeAt
limit := plangate.ImagesAllowed(sub.Plan, product.CreatedAt, lastPlanChangeAt)
if limit != plangate.Unlimited && currentCount >= limit {
    RespondErr(c, apperrors.PlanLimit("images_per_product", limit), h.logger)
    return
}
```

Grandfathering is already baked into `ImagesAllowed()` — no extra logic needed at the callsite.

**Test:** Starter merchant with a 25-image product → upload 26th image → 403 with `plan_limit_exceeded`.

### P1.2 — Audit white-label app gating

**Read:** `services/marketplace-api/internal/handlers/storefront/` for any `app/build` or `app/config` routes. Confirm they call `plangate.WhiteLabelAppEnabled(sub.Plan, sub.HasWhiteLabelAppAddOn)` before serving. If not, the add-on can be cancelled and the app surfaces continue working.

Also confirm: the downgrade/cancel-addon flow actually flips `has_white_label_app_add_on` to `false` in the subscription row.

## Smaller issues

- **`FeatureFullAPI` dead code confusion** — the header comment in `apikeys_handler.go:3-4` and `routes.go:676` both reference `FeatureFullAPI` enforcement that doesn't exist. Either wire it (preferred) or delete the misleading comments.
- **Matrix parity test tightening** — `matrix_test.go` asserts feature-list cardinality. Extend it to assert that every matrix entry passes `plangate.IsAllowed()` round-trip so missing cells fail the test.
- **`Negotiated` (-2) vs. `Unlimited` (-1) in deltas** — `planchange/preflight.go:210-213` conflates both to `-1` in `limitDelta`. Frontend needs to know the difference to render "contact sales" vs. "unlimited". Consider returning a tagged delta type.

## Acceptance criteria for the follow-up PR

- [ ] `FeatureFullAPI` gates write routes on API-keys handler
- [ ] API-key write-scope grants check `plangate.IsAllowed(plan, FeatureFullAPI)` in service layer
- [ ] `FeatureCustomCSS` gates storefront CSS write endpoints
- [ ] `FeatureCustomCodeInjection` gates custom-code-injection write endpoints
- [ ] Misleading header comments in `apikeys_handler.go` + `routes.go:676` removed or corrected
- [ ] Integration tests added for the three P0 403 paths
- [ ] P1.1 + P1.2 either fixed or explicitly deferred with a `// TODO(plangate-gap): …` comment referencing this doc

## References

- Matrix source: `services/marketplace-api/internal/plangate/matrix.go`
- Spec §9 (feature matrix)
- Current enforcement: `grep -rn "plangate\." services/marketplace-api/internal/handlers/`
- Helper functions: `IsAllowed`, `Limit`, `WhiteLabelAppEnabled`, `ImagesAllowed`, `MinPlanForFeature`
