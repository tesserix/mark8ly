# GIP removal audit (#708)

Audited 2026-09-07 against `main` @ 716c60a5, by reading all 91 code files that
reference GIP/Firebase/Identity Platform. This is the "audit commit" #708 asks
for, in document form.

## Read this first: #708 is not a cleanup issue

The issue is framed as removing dead scaffolding left behind by a finished
migration. That framing is wrong in one decisive way, and planning against it
will delete working product.

**Five live, merchant- or customer-facing capabilities are implemented entirely
on GIP and have no Zitadel replacement anywhere in the codebase.** Removing GIP
removes the feature:

| Capability | Where | Consequence of a naive removal |
|---|---|---|
| Storefront **customer** token verification | `marketplace-api/internal/handlers/storefront/gip_customer_verifier.go` + `main.go:1755-1765` | Mobile storefront customer support chat stops authenticating. The Zitadel migration covered the ADMIN path only. |
| Custom-domain browser-key allowlist | `marketplace-api/internal/gipkey` + `main.go:545-579` | A merchant's storefront sign-in stops working from their verified custom domain. Self-service feature. |
| Tenant SAML/OIDC SSO | `marketplace-api/internal/sso/gip_client.go` | Enterprise tenant SSO provisioning disappears. May already be dark — nothing in `main.go` constructs it — confirm before deciding. |
| ~~Merchant display-name seeding~~ **RESOLVED (#790)** | was `marketplace-api/internal/gipuser`; now `marketplace-api/internal/displayname` (`SetDisplayNames`) reading auth-bff's `GET /internal/users/:id/display-name`, backed by `zitadellogin.Client.UserDisplayName` | None. `internal/gipuser` is deleted. The seam is provider-neutral and wired unconditionally, so Zitadel-mode merchants now get the name too — closing the pre-existing gap rather than merely preserving it. |
| ~~Invite tenant-claim write~~ **RESOLVED (#791)** | was `platform-api/internal/gipadmin` + `cmd/server/provider_wiring.go` | None. No replacement was needed: #786/#800 removed the last reader of the `tenant_id` claim from marketplace-api, and the `gip.set_tenant_claim` outbox was verified drained in production (5 rows, all `completed`, newest 2026-07-31) before the write was retired. `internal/gipadmin` is deleted along with `requireGIPForTenantClaim`, `newTenantClaimSetter` and `cmd/backfill-gip-claims`; its sentinel errors moved to `internal/idperr`. |

None of these are in #708's inventory. Each needs a **replacement decision**,
not a deletion.

## Two whole apps were never migrated

`apps/mobile-storefront/` and `apps/storefront-mobile/` contain **zero**
references to Zitadel and are 100% GIP/Firebase, with `gipTenantId` a required
build field. Verified:

```
apps/mobile-storefront : zitadel refs = 0
apps/storefront-mobile : zitadel refs = 0
apps/mobile-admin      : zitadel refs = 21
```

They need their own migration phase. #708 cannot touch them.

## The environment is not ready either

| Service | State (verified in prod, 2026-09-07) |
|---|---|
| `marketplace-api-admin` | `ZITADEL_ENABLED=true`; `ZITADEL_DUAL_ISSUER` **was `true`, set to `false` 2026-09-07** (tesserix-k8s#1041) — GIP tokens are no longer accepted |
| `marketplace-api-storefront` | **no `ZITADEL_ENABLED`** — customer path still GIP |
| `platform-api` | `ZITADEL_ENABLED=true`. `GIP_PROJECT_ID`/`GIP_TENANT_ID`/`GIP_WEB_API_KEY`/`GIP_SERVER_API_KEY` became unread in code with #791; the chart values follow in a separate PR, per the code-first ordering rule below |

Dual-issuer has since been turned OFF (#785), which is what unblocked the
admin-path collapse. The paragraph below describes the state that blocked it,
kept because the reasoning still explains the ordering.

While dual-issuer was ON, GIP-issued tokens were still accepted, so the
`tenant_id` custom claim invite-accept writes through GIP is still load-bearing.
`provider_wiring.go` states the unblock condition in its own error text:
GIP config may only leave platform-api once Zitadel is enabled on
marketplace-api — which is true for the admin engine and **false for the
storefront engine**.

## Corrected scope

#708 says 255 files (a later note says 373). Excluding build artifacts,
lockfiles and `go.sum`, and using word-boundary matching: **139 files, of which
91 are code** and 48 are docs/planning prose. Docs include legal pages
(cookies, DPA, security, sub-processors) that name "Google Identity Platform"
and need a **content edit timed with the real cutover**, not a deletion.

## The grep is a trap in both directions

#708's suggested approach is a greppable marker sweep. A `grep -r gip` gets
this wrong both ways, and that is the single most useful output of this audit.

### It would DELETE things that must stay

- **`marketplace-api/internal/auth/gip_bearer.go`** — despite the name, this is
  the shared, **provider-agnostic** bearer middleware that **Zitadel already
  uses today**. Deleting it breaks 100% of mobile-admin auth. Rename it; do not
  remove it.
- **`auth-bff/internal/autologin/service.go`** — saturated with "gip" strings,
  but `completeLogin` is shared and Zitadel's `CompleteForProvider` calls it.
  Only `AutoLogin` + the `gip` field go.
- ~~**`platform-api/internal/gipadmin`'s sentinel errors**~~ **DONE (#791)**:
  relocated to the leaf package `platform-api/internal/idperr` (messages
  re-prefixed `idp:`) before the package was deleted, exactly as this warned.
  `zitadeladmin` and `internal/auth` now import `idperr`.
- **All of `marketplace-api/internal/whitelabel/**`** plus
  `subscription/app_lifecycle.go` and migrations `000048`/`000076`. This is a
  *third* meaning of "firebase": GCP **project lifecycle** teardown for a
  merchant's own white-label app. Not auth, not push, not ours to remove.
- **`GoogleService-Info.plist` / `google-services.json`** — consumed by
  `app.config.js` at **prebuild**. PR #781 already proved that losing them
  breaks the iOS build outright.
- **`handlers/admin/validation.go`'s `isValidUUID` guard** — the comment
  mentions Firebase UIDs, but a Zitadel `sub` is also non-UUID. Fix the
  comment, keep the code.

### It would MISS things that must go

- **`auth-bff` `OAUTH_CLIENT_ID` / `OAUTH_CLIENT_SECRET`** — GIP-era OIDC
  redirect flow, **zero readers**, still `required:"true"`. No "gip" in the
  name, so no grep finds them. Verified by direct search.
- **`auth-bff` `GIP_PROJECT_NUMBER`** — `required:"true"`, **zero readers**.
- **`packages/mobile-shared`'s `LinkAccountPrompt.tsx` and the 7 GIP-only
  `AuthBackend` methods** (`completeLinkWith*`, `existingSignInMethods`,
  `linkedProviderIds`, `linkGoogleToCurrentUser`, `linkAppleToCurrentUser`,
  `unlinkProvider`) contain **no string "gip" at all** — the largest GIP-only
  surface in the mobile app is invisible to the sweep.
- **`ZitadelEnabled` / `ZitadelDualIssuer`** in `mobile_routes.go` and
  `config.go` — no "gip" in the identifier, but entirely GIP-removal work.

## Three hazards that fail silently

1. ~~**The outbox rots without erroring.**~~ **CLOSED (#791)**: the hazard was
   real, and was closed by measurement rather than by code — production held 5
   `gip.set_tenant_claim` rows, all `completed`, newest 2026-07-31, and zero
   not-completed. `gip_claim_handler.go` and the enqueue in
   `onboarding/service.go` are both gone. The integration tests now assert by
   the LITERAL kind string that no such row is ever enqueued again, since the
   constant no longer exists.
2. **`gip.ts` is `require()`d lazily** inside `provider.tsx`'s
   `createFirebaseBackend` (to keep Firebase out of Expo Go). Deleting it
   without removing that call site fails **only at runtime on a device** —
   `tsc` and `jest` stay green. This is the same class of failure as #781 and
   as the sign-out bug in #780, both found only on hardware.
3. **`NEXT_PUBLIC_GIP_*` are build-time inlined**, including into server
   actions. Removing one is a rebuild and redeploy, and rollback is another
   rebuild — never a config flip.

## Ordering rules

- **Removing a required config is code-first**: delete the reader, deploy, then
  drop the chart value. (#774 did the opposite — *adding* one is k8s-first.
  Same milestone, opposite directions.) Applies to auth-bff's
  `GIP_PROJECT_ID`, `GIP_PROJECT_NUMBER`, `GIP_WEB_API_KEY`,
  `GIP_INTERNAL_TENANT_ID`, `OAUTH_CLIENT_ID`, `OAUTH_CLIENT_SECRET`. Backwards
  = CrashLoopBackOff on the auth gateway.
- **Grep every reader**, not just `Validate()`. `GIPProjectID` in
  marketplace-api alone gates three independent features.
- `gip_bearer.go`'s collapse must land in the **same PR** as
  `mobile_routes.go`'s flag removal and `main.go`'s call site — they share one
  boolean contract. A partial edit leaves either zero tenant-id writers (mobile
  404s) or two racing, which is exactly the bug #524 phase 4 fixed.
- `ZitadelDualIssuer` collapse goes **last**, after every consumer has
  collapsed.
- **Do not delete the three GCP Identity Platform tenant pools.** Irreversible,
  and explicitly gated on human approval. Deleting the *config that names them*
  is not the same act as deleting the pools.

## Recommended split of #708

As written, #708 is not executable — it mixes dead-code deletion with feature
migration. Suggested split:

1. **Free wins** (no product decision, no replacement needed): auth-bff's three
   unread required env vars; the unread `GIPCustomerTenantID` /
   `GIPPlatformTenantID`; stale comments. Code-first ordering, small and safe.
2. **Admin-path collapse**: `selectMobileTokenVerifier`, `gip_bearer.go`
   rename + parameter drop, `mobile_routes.go` flags, mobile app's
   `isZitadelProvider` branches, `gip.ts` and the Firebase backend. Gated on
   turning **dual-issuer off**.
3. **Feature replacements** (one issue each, each needs a decision): storefront
   customer verification; custom-domain key allowlist; tenant SSO;
   ~~display-name seeding~~ (#790); ~~invite tenant-claim~~ (#791 — no
   replacement needed, see the table above).
4. **Storefront/customer cutover**: `marketplace-api-storefront`'s
   `ZITADEL_ENABLED`, plus `apps/mobile-storefront` and `apps/storefront-mobile`
   migrations.
5. **Schema and pools, last**: `customer_profiles.gip_uid` (+ migration 000084),
   `tenant_sso_configs.gip_provider_id`, and finally the GCP tenant pools —
   human-approved, irreversible.

Steps 1 and 2 are the only ones that are cleanup. Steps 3 and 4 are migration
work that has not been scoped anywhere yet.
