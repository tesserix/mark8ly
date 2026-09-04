# Zitadel Phase 6a — Customer Sign-Up, With a Verified Email

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the last GIP-only user-facing path. A storefront shopper can create an account against Zitadel, with their email verified, behind `NEXT_PUBLIC_AUTH_PROVIDER=zitadel`.

**Architecture:** Today the *browser* calls GIP's `accounts:signUp` directly and hands the resulting `idToken` to a server action. That cannot work for Zitadel — creating a user needs the instance PAT, which must never reach a browser. So `auth-bff` gains two endpoints: one that creates the user and emails a verification code, one that verifies it. The storefront calls them and then mints `mp_customer_session` through the existing shared helper.

**Spec:** `docs/superpowers/specs/2026-09-03-zitadel-migration-design.md`

## Why the email must be verified — this is the reason the phase is not a straight port

Our customer Google path **deliberately refuses** to link a Google identity to an account whose email is unverified. Read the `email_taken` branch in `internal/zitadellogin/customer_handler.go`: an unverified account may have been created by an attacker who typed the victim's address and set their own password, so linking would hand the real owner an attacker-controlled account.

GIP's `accounts:signUp` creates an account with an **unverified** email. Porting that shape as-is would manufacture that exact state at scale: every shopper who signs up with a password and later clicks "Continue with Google" would hit a permanent `email_taken` refusal. So sign-up verifies the address, and the lockout never arises.

## Verified API facts — probed live 2026-09-04, use verbatim

- **Create:** `POST /v2/users/human` with `{"profile":…, "email":{"email":…,"returnCode":{}}, "password":{"password":…}}` → 200 `{"userId":…, "details":…, "emailCode":"<6 chars>"}`. The code is returned to us; Zitadel sends nothing.
- **Verify:** `POST /v2/users/{id}/email/verify` with `{"verificationCode":…}` → 200. A wrong code → 400, `details[0].id` = **`COMMAND-eis9R`** ("Code is invalid"). A bogus user id → `COMMAND-ieJ2e`.
- Note the path is `email/verify`, **not** `email/_verify` — the underscore form is a routing 404 (plain `{"code":5,"message":"Not Found"}` with no error id, which is how you tell a wrong path from a domain error).
- After verifying, the account is returned by the verified-email search with `isVerified: true`, so Google auto-linking works.
- **Zitadel v2 is protojson and flattens oneofs** — `returnCode` sits directly under `email`, with no wrapper named after the oneof. Phase 5 shipped a critical defect by wrapping one; a wrapped oneof returns **200** and silently does the wrong thing.

## Global Constraints

- **The verification code never reaches the browser.** It is a credential. `auth-bff` holds it, emails it via `internal/notify`, and returns only a non-secret handle to the storefront. If any response or log carries the code, that is a defect.
- **With `NEXT_PUBLIC_AUTH_PROVIDER` unset, the GIP signup path must be byte-identical.** Verify by diffing, not by a green suite.
- `internalauth` is the first statement of every new endpoint, before the body is read.
- Never tell a shopper their credentials are wrong when they are not. Every outcome gets truthful, distinct copy; an internal error string never reaches them.
- No password, verification code, token, or secret in any log line.
- **`npm run build` must pass** for the storefront — a `"use server"` module may export only async functions, and neither vitest nor tsc catches a violation.
- Test key literals stay low-entropy (`"thirtytwo-bytes-for-testing-only"`).

---

### Task 1: `auth-bff` creates the account and emails the code

**Files:**
- Modify: `services/auth-bff/internal/zitadellogin/client.go` (a `CreateHumanUserWithPassword` sibling to the existing `CreateHumanUserWithIDPLink`), `customer_handler.go`, `cmd/server/main.go`, and tests

Add `POST /auth/customer/register`. It must:

1. Refuse if an account already holds this email. Reuse `FindUserByVerifiedEmail` — it is org-scoped and refuses ambiguity rather than picking. Return the existing `email_taken` outcome so the storefront's copy stays consistent.
2. Create the user with the password and `email.returnCode`, capturing `emailCode`.
3. Send the verification email through `services/auth-bff/internal/notify` — read it first and reuse whatever it already does for transactional mail rather than inventing a path.
4. Return the identity and a handle the storefront can later use to verify — **never the code**.

- [ ] **Step 1: Write the failing tests** — happy path creates and emails; an existing verified email refuses with `email_taken` and creates nothing; a weak password surfaces distinctly (Zitadel returns `DOMAIN-HuJf6` "Password is too short" — phase 5 verified this id); an unauthenticated caller never reaches Zitadel (use the `unreachableZitadel` fixture); the code appears in no response body and no log.
- [ ] **Step 2: Run, confirm failure**
- [ ] **Step 3: Implement**
- [ ] **Step 4: `go build ./... && go vet ./... && go test -race ./...`**
- [ ] **Step 5: Commit** — `feat(auth-bff): register a storefront customer against Zitadel`

---

### Task 2: `auth-bff` verifies the emailed code

**Files:**
- Modify: `client.go`, `customer_handler.go`, `cmd/server/main.go`, and tests

Add `POST /auth/customer/verify-email`, calling `POST /v2/users/{id}/email/verify`.

Map `COMMAND-eis9R` to a distinct, truthful "that code is wrong or expired" outcome — not a generic failure, and not anything implying the password was wrong. Key off `details[0].id`, not message text: phase 5 found two different failures whose messages differed by one word.

- [ ] **Step 1: Write the failing tests** — a correct code verifies and the account is then verified; a wrong code returns the distinct outcome and does not verify; an unauthenticated caller never reaches Zitadel; no code in any log.
- [ ] **Step 2: Run, confirm failure**
- [ ] **Step 3: Implement**
- [ ] **Step 4: build, vet, `go test -race ./...`**
- [ ] **Step 5: Commit** — `feat(auth-bff): verify a customer's email address`

---

### Task 3: The storefront sign-up flow

**Files:**
- Modify: `apps/storefront/app/create-account/actions.ts`, `apps/storefront/components/auth/CreateAccountForm.tsx`, `apps/storefront/lib/auth/auth-bff-customer.ts`
- Modify/create: their tests

Under the flag: call register → show a "check your email" step → call verify → then mint the session through the **existing** `completeCustomerSignIn` in `lib/auth/customer-session.ts`. Do not write a second cookie-minting path.

With the flag unset, the GIP path must be untouched — the browser still calls `accounts:signUp` and `customerSignUp` still delegates to `customerSignIn`.

Note `actions.ts` is a `"use server"` module: every export must be an async function. Put helpers in `lib/`.

- [ ] **Step 1: Write the failing tests** — flag set: register → verify → session minted, with no cookie on any failure path; flag unset: the GIP flow is unchanged; an already-taken email shows the `email_taken` copy, which must be actionable and must not suggest retrying; a wrong verification code keeps the shopper on the verify step rather than losing their progress.
- [ ] **Step 2: Run, confirm failure**
- [ ] **Step 3: Implement**
- [ ] **Step 4: `npx vitest run`, `npx tsc --noEmit`, and `npm run build` — report the build's exit status**
- [ ] **Step 5: Commit** — `feat(storefront): create customer accounts against Zitadel`

---

### Task 4: Documentation

**Files:**
- Modify: `services/auth-bff/internal/zitadellogin/README.md`

- [ ] **Step 1:** Record that customer sign-up now verifies the email, and **why** — that our Google path refuses to link to unverified accounts on purpose, so an unverified sign-up would manufacture the `email_taken` lockout for every shopper who later uses Google. Record the verified endpoint shapes and ids, that `email/verify` has no underscore, and that the code stays inside `auth-bff` because it is a credential. Note what remains for cutover.
- [ ] **Step 2: Commit** — `docs(auth-bff): record customer sign-up and email verification`
