# Zitadel Phase 5 — `platform-api`'s Account Operations

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let `platform-api` perform password reset and account deletion against Zitadel instead of GIP's Identity Toolkit, flag-selected.

**Architecture:** `gipadmin` today is a concrete `*AdminClient` doing five things over GIP's REST API. Three of them — send a password-reset code, reset a password, delete an account — get a Zitadel implementation behind an interface, selected by the same `ZITADEL_ENABLED` flag shape used elsewhere. The other two stay on GIP deliberately.

**Spec:** `docs/superpowers/specs/2026-09-03-zitadel-migration-design.md` (decision D7)

## What is deliberately NOT in scope

**`EnsureTenantClaim` and the `backfill-gip-claims` CLI stay on GIP and are not ported.** D7 says the custom claim is dropped rather than ported, and phase 4 removed its consumer — but **only under the Zitadel flag**. With the flag off, which is today's production, `marketplace-api`'s `gip_bearer.go` still reads `tenant_id` from that claim. `invitation/service.go:429` writes it on invite-accept, and its own comment records the consequence of it not landing: *"mobile shows its 'No store yet' empty state until the claim lands."*

Deleting it now would give every newly-invited merchant a permanent no-store state on mobile. It gets deleted at cutover, once `ZITADEL_ENABLED` is true on marketplace-api — not before. `lookupCustomAttributes` serves only `EnsureTenantClaim`, so it stays too.

## Global Constraints

- **The nil-interface trap is the single biggest hazard here — read `cmd/server/account_wiring.go`'s comment in full before writing any interface.** Assigning a nil `*AdminClient` into an interface yields a **non-nil interface holding a nil pointer**, so a `!= nil` guard passes and the method panics on a nil receiver. That panic lands *after* the teardown transaction commits, gin's Recovery answers 500, and the operator is told `503 upstream_unavailable` for work that already happened. Every new interface in this phase must avoid reintroducing it, and the existing guard pattern must survive.
- **With the flag unset, behaviour must be byte-identical to `main`.** Phase 4 shipped a revision that violated this and would have bricked a live app; verify by diffing the GIP path, not by reasoning.
- Password reset and account deletion are destructive, user-visible paths. A failure must not silently succeed, and an unavailable provider must surface as unavailable rather than as "wrong code".
- No token, secret, password, reset code, or bearer value in any log line.
- `ZITADEL_ISSUER` and any project id must be `TrimSpace`d **on assignment**, like the other secret-sourced fields — a trailing newline from a mounted secret caused a ~25-hour outage in this codebase before.
- `go build ./... && go vet ./... && go test -race ./...` must pass in `services/platform-api`.

---

### Task 1: An interface for the account operations

**Files:**
- Modify: `services/platform-api/internal/auth/service.go` (takes `Admin *gipadmin.AdminClient` concretely today), and its tests
- Check: `internal/account/service.go:45` already defines `gipDeleter` — reuse that shape rather than inventing a second one

Introduce interfaces covering the three operations to be swapped: send reset code, reset password, delete account. `gipadmin.AdminClient` must satisfy them unchanged — this task adds no behaviour.

**Handle the nil trap explicitly.** Construct the interface value only when a real client exists; never assign a possibly-nil concrete pointer into it. Follow whatever `account_wiring.go` already does, and say in your report how you avoided it.

- [ ] **Step 1: Write the failing tests** — a nil provider is genuinely nil at the interface (a `!= nil` guard sees it as absent, and calling through it never panics); `gipadmin.AdminClient` satisfies the interfaces; existing auth/account behaviour is unchanged.
- [ ] **Step 2: Run, confirm failure**
- [ ] **Step 3: Implement**
- [ ] **Step 4: build, vet, `go test -race ./...`**
- [ ] **Step 5: Commit** — `refactor(platform-api): put account operations behind an interface`

---

### Task 2: The Zitadel implementation

**Files:**
- Create: `services/platform-api/internal/zitadeladmin/` (client + tests), or follow whatever package convention this service already uses

Implement the three operations against Zitadel's v2 API. The spec's D7 table gives the mapping:

| GIP today | Zitadel |
|---|---|
| `sendOobCode` (PASSWORD_RESET, `returnOobLink`) | `POST /v2/users/{id}/password_reset`, returning the code so we send the mail |
| `accounts:resetPassword` | `POST /v2/users/{id}/password` with `verificationCode` |
| `accounts:delete` | `DELETE /v2/users/{id}` |

**Returning the code rather than letting Zitadel send the mail is deliberate** — it preserves today's `returnOobLink=true` behaviour and keeps branding and delivery on the existing `notify` → platform-api path rather than the shared instance's SMTP.

Note the reset flow is **id-based**, not email-based: GIP's `sendOobCode` takes an email, Zitadel's takes a user id. Resolving email → id is part of this task. `POST /v2/users` is a **search** on this instance (verified), which is how auth-bff finds users by email.

Read `services/auth-bff/internal/zitadellogin/client.go` first for the established request/error conventions in this codebase, and follow them.

- [ ] **Step 1: Write the failing tests** against an `httptest` fake: each operation's happy path; a user-not-found; an unavailable provider surfacing as unavailable rather than as a wrong-code/weak-password error; the existing sentinel errors (`ErrUserNotFound`, `ErrWeakPassword`, `ErrTooManyAttempts`, `ErrUnavailable`, `ErrUnauthenticated`) mapping correctly, since `internal/auth/handler.go` already branches on them.
- [ ] **Step 2: Run, confirm failure**
- [ ] **Step 3: Implement**
- [ ] **Step 4: build, vet, `go test -race ./...`**
- [ ] **Step 5: Commit** — `feat(platform-api): Zitadel account operations`

---

### Task 3: Selection and wiring

**Files:**
- Modify: `services/platform-api/cmd/server/main.go` (around the `gipAdmin` construction at ~line 243), `cmd/server/account_wiring.go`, `pkg/config`, and tests

Select by flag, defaulting to GIP. Mirror `services/auth-bff/pkg/config`'s `ZITADEL_ENABLED` + `ValidateZitadel()` shape: dependent values required only when enabled, inert when not.

**`EnsureTenantClaim` still needs the GIP client**, so a Zitadel deployment keeps `gipAdmin` alive for that one call. Two concerns with two lifetimes — say clearly in the code which is which, so a later reader does not "tidy up" the GIP client and break invite-accept.

- [ ] **Step 1: Write the failing tests** — flag unset selects GIP and behaviour is unchanged; flag set selects Zitadel for the three operations while `EnsureTenantClaim` still runs against GIP; a misconfigured Zitadel fails clearly rather than silently falling back; the nil-interface guard still behaves when a provider is absent.
- [ ] **Step 2: Run, confirm failure**
- [ ] **Step 3: Implement**
- [ ] **Step 4: build, vet, `go test -race ./...`**
- [ ] **Step 5: Commit** — `feat(platform-api): select account operations by provider`

---

### Task 4: Documentation

**Files:**
- Modify: the doc comments or package docs where this service documents auth

- [ ] **Step 1:** Record which operations moved and which did not, and **why `EnsureTenantClaim` is still here** — that it is dead only once `ZITADEL_ENABLED` is true on marketplace-api, and that deleting it sooner gives newly-invited merchants a permanent "No store yet" on mobile. Record that the reset code is returned to us on purpose so mail stays on our own delivery path. Note the nil-interface trap and how the wiring avoids it.
- [ ] **Step 2: Commit** — `docs(platform-api): record the Zitadel account-operation split`
