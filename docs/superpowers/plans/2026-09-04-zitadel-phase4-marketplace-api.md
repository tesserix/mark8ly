# Zitadel Phase 4 — `marketplace-api`'s Mobile Admin Verifier, and Dropping the `tenant_id` Claim

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let `marketplace-api`'s mobile admin API accept Zitadel tokens, and stop depending on the `tenant_id` custom claim that Zitadel will not mint.

**Architecture:** `TokenVerifier` stays as it is — an interface returning verified claims. A Zitadel implementation joins the GIP one, selected by flag. The real change is where tenancy comes from: today it rides a custom claim on the GIP token; from here the client states which tenant it is acting as and the server validates that membership against FGA. `RequireTenantClaim`'s fail-closed 404 behaviour is preserved exactly.

**Spec:** `docs/superpowers/specs/2026-09-03-zitadel-migration-design.md` (decision D7)

## Why the tenant moves to the request

D7 drops the claim rather than porting it: Zitadel keeps user metadata out of the token, and putting it there needs an Actions v2 "complement token" script — a new runtime dependency on a shared instance. The spec's answer is to resolve tenancy from FGA, as `autologin` already does.

A claim carries **one** tenant; FGA membership is a **list**. A merchant who owns one store and is staff on another has two, and the non-store-scoped mobile admin groups have no store in the path — the code says so: *"Not store-scoped: it rides the admin's tenant from the bearer token."* Picking `tenants[0]` server-side would silently drop such a merchant into an arbitrary store with no way to switch.

So the client states the tenant and the server validates it. This mirrors `authz.RequireTenantRelation`, which already does exactly this on store-scoped routes, and it is one `Can()` check rather than a list query. The mobile admin app is still being built (#119–#122), so the contract change costs nothing now and grows more expensive later.

## Global Constraints

- **Fail closed.** A caller with no valid tenant must end up with an empty `tenant_id`, so `RequireTenantClaim` answers **404**, never 401. A 401 makes the mobile client sign the user out and bounce to `/login` — the bug that whole design note exists to prevent. Read `require_tenant_claim.go`'s comment before touching anything here.
- **Never trust the client's tenant without an FGA check.** The header states intent; membership is the authority. An unvalidated header would let any authenticated merchant act on any tenant.
- **The GIP path must keep working** until cutover. Flag-select the verifier; do not delete the GIP one.
- No token, secret, or bearer value in any log line.
- Test key literals stay low-entropy (`"thirtytwo-bytes-for-testing-only"`).
- `go build ./... && go vet ./... && go test -race ./...` must pass in `services/marketplace-api`.

---

### Task 1: Tenant from the request, validated against FGA

**Files:**
- Create: `services/marketplace-api/internal/auth/tenant_from_request.go` + test

A middleware mounted after bearer auth. It reads the caller's stated tenant from the request, checks membership with the FGA client (`go-shared/authz`'s `Can(ctx, userID, "member", "tenant", tenantID)` — confirm the exact relation and object type against `infra/openfga/model.fga` rather than assuming), and sets `tenant_id` on the gin context **only** when the check passes.

On a missing header, a failed check, or an FGA error it must leave `tenant_id` empty and call `c.Next()` — never abort. `RequireTenantClaim` supplies the 404. This split is deliberate: authentication and authorization answer separately.

- [ ] **Step 1: Write the failing tests** — member passes and `tenant_id` is set; non-member leaves it empty; absent header leaves it empty; an FGA error leaves it empty (fail closed, no panic, no 500); the middleware never aborts; `user_id` is untouched.
- [ ] **Step 2: Run, confirm they fail**
- [ ] **Step 3: Implement**
- [ ] **Step 4: build, vet, `go test -race ./...`**
- [ ] **Step 5: Commit** — `feat(marketplace-api): validate a client-stated tenant against FGA`

---

### Task 2: A Zitadel token verifier

**Files:**
- Create: `services/marketplace-api/internal/auth/zitadel_verifier.go` + test

Implement `TokenVerifier` for Zitadel access tokens. Return the subject as `UserID` and leave `TenantID` empty — Zitadel mints no tenant claim, and Task 1 now supplies tenancy.

Verify the token properly: signature against the issuer's JWKS, plus issuer and expiry. `services/auth-bff` and `go-shared/middleware` already verify Zitadel/GIP tokens — read them first and reuse whatever exists rather than hand-rolling JWT validation. If you find yourself writing signature verification from scratch, stop and look again.

- [ ] **Step 1: Write the failing tests** — a valid token yields the subject; a bad signature, wrong issuer, and expired token each fail; `TenantID` is always empty.
- [ ] **Step 2: Run, confirm they fail**
- [ ] **Step 3: Implement**
- [ ] **Step 4: build, vet, `go test -race ./...`**
- [ ] **Step 5: Commit** — `feat(marketplace-api): verify Zitadel bearer tokens`

---

### Task 3: Select the verifier and mount the middleware

**Files:**
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go` (verifier selection, around the existing Firebase block near line 1334)
- Modify: `services/marketplace-api/internal/handlers/admin/mobile_routes.go` (mount Task 1's middleware in `tenantMW`, between `bearerAuth` and `requireTenant`)
- Modify: their tests

Select the verifier by config, defaulting to GIP so nothing changes without opting in. `RegisterAdminMobile` returns early when `TokenVerifier == nil`, which currently disables the whole group when GIP is unconfigured — preserve that shape for whichever provider is selected.

The middleware order matters: `bearerAuth` → tenant-from-request → `requireTenant` → TenantGate → rate limiter. Getting it wrong either skips the FGA check or 404s before identity exists.

- [ ] **Step 1: Write the failing tests** — with the flag unset the GIP verifier is selected and behaviour is unchanged; with it set the Zitadel one is; the middleware sits in the right order; a member reaches a handler and a non-member gets 404 (never 401).
- [ ] **Step 2: Run, confirm they fail**
- [ ] **Step 3: Implement**
- [ ] **Step 4: build, vet, `go test -race ./...`**
- [ ] **Step 5: Commit** — `feat(marketplace-api): select the bearer verifier by provider`

---

### Task 4: Stop reading the claim

**Files:**
- Modify: `services/marketplace-api/internal/auth/gip_verifier.go`, `gip_bearer.go`, and tests
- Consider renaming `RequireTenantClaim` — it no longer guards a claim

With Task 1 supplying `tenant_id`, `GIPBearerAuth` must stop setting it from the token, or it would overwrite the FGA-validated value with an unvalidated one — the exact bug this phase exists to remove. Remove `TenantID` from the claim path and keep `UserID`.

`RequireTenantClaim`'s name is now misleading; rename it to describe what it does (a tenant is bound and validated) and update its doc comment, keeping the 404-not-401 reasoning intact — that comment records a real incident and must survive.

- [ ] **Step 1: Write the failing tests** — a token carrying a `tenant_id` claim no longer influences the context; the FGA-validated value is what handlers see; the 404-not-401 behaviour is unchanged.
- [ ] **Step 2: Run, confirm they fail**
- [ ] **Step 3: Implement**
- [ ] **Step 4: build, vet, `go test -race ./...`**
- [ ] **Step 5: Commit** — `refactor(marketplace-api): take tenancy from FGA, not the token claim`

---

### Task 5: Documentation

**Files:**
- Modify: the package README or doc comments that describe mobile admin auth

- [ ] **Step 1:** Record that the client now states its tenant and the server validates membership; why the claim was dropped (Zitadel keeps metadata out of the token; an Actions v2 script would be a new runtime dependency); why a list needs a client-stated choice rather than `tenants[0]`; and that the 404-not-401 rule is load-bearing because a 401 signs the mobile user out.

  Also record, as a caution for whoever wires it: `RegisterMobileStorefront` is defined but never mounted, and its authenticated chain omits `MobileCustomerAuth`, so wiring it as-is would 401 every authenticated mobile storefront request. Only the support routes are live, and those chain the real verifier.
- [ ] **Step 2: Commit** — `docs(marketplace-api): record mobile admin tenancy resolution`
