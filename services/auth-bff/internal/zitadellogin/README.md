# Zitadel Login Client

This package implements Zitadel's v2 login-client HTTP API so auth-bff can host its own login page instead of redirecting to Zitadel's managed UI. It is a narrow, purpose-built client that speaks only the shapes observed against a live Zitadel v4.15.3 instance.

## Known Gaps and Unusual Design Choices

### 1. Zitadel Does Not Enforce MFA for Login Clients

**The issue:** Under a `forceMfa: true` login policy, Zitadel's v2 API still issues an OIDC authorization code for a password-only session. It does not refuse the request, it does not signal "more factors required"—it succeeds silently. This has been verified against live v4.15.3 instances by two independent teams.

**Why this matters:** A password-only login would normally violate an MFA policy. Zitadel's server makes no attempt to enforce this at the API boundary, which means this package **must** enforce it on the client side—or MFA gets silently bypassed.

**How we prevent it:** The `sufficiency.go` file contains all MFA enforcement logic. Every path to finalize a session goes through a sufficiency decision in `CompleteIfSufficient` or `CompleteAfterFactor`, which checks the policy, compares it against what factors the user has verified, and either blocks the login (returning `OutcomeFactorRequired` or `OutcomeHandoff`) or permits it. The `finalize` function itself is unexported and requires a `sufficient` witness type that can only be constructed in `sufficiency.go`.

This design makes "call finalize without a decision" a compile error. It does not stop *all* mistakes—see the blind spot below—but it makes the most obvious bypass (a direct finalize call) impossible.

### 2. The Architecture Tests Have a Known Blind Spot

Three architecture tests police the MFA enforcement boundary:

- `TestFinalizeIsOnlyCalledFromSufficiency` — checks that no file other than `sufficiency.go` calls `finalize`
- `TestSufficientWitnessIsOnlyConstructedInSufficiency` — checks that `sufficient{}` is constructed only in `sufficiency.go`
- `TestSufficiencyNeverUsesTheUnscopedDisplayPolicy` — checks that `sufficiency.go` never calls `InstanceLoginPolicyForDisplay`

**The blind spot:** These tests check *which file* performs an action, not *how the decision was reached*. They verify file-level facts via string search; they do not perform semantic analysis.

A future contributor could add a helper function inside `sufficiency.go` itself that constructs a `sufficient{}` witness and calls `finalize` while skipping the core sufficiency checks (`mfaRequired`, `classifyEnrolledMethods`, `SessionFactors`). Such a function might be named `bypassMFA`, `trustDeviceComplete`, or `adminFinalizeLogin`. It would pass all three tests because it lives in the right file and constructs the witness there—yet it would be exactly the MFA bypass the tests are meant to prevent.

**Why we accepted this gap:** Closing it properly requires abstract-syntax-tree (AST) or call-graph analysis that can validate the *decision path*, not just the *file*. That was judged disproportionate to the risk. The package is small and changes rarely; the tests catch the obvious mistakes.

**Mitigation—required action for future editors:** Any new code path to `finalize` is a decision about MFA enforcement, not a refactor. Before adding a new helper that leads to `finalize`, treat it as a policy decision: review it carefully, document why the shortcut is sound, and consider whether a new architecture-level test should pin the decision logic itself (not just the file). Treat any suggestion to add a "trusted device" mode, "admin bypass", or "skip MFA" feature as requiring design review and test changes, never as a code-only change.

### 3. Password Change Requirement Is Not Signalled

**The issue:** When a user logs in via a password that Zitadel has marked as requiring a change, the session create, session read, and finalize endpoints return byte-identical responses to a normal login. No flag, no error, no alert—just a successful login that actually requires a password change before the user can proceed.

This is an upstream gap in Zitadel; it is not a bug in this package. However, it is load-bearing: if Zitadel later adds a `passwordChangeRequired` flag to the session or factors object, this package will silently ignore it until the handler is updated to check it and respond with `OutcomeHandoff` instead of `OutcomeComplete`.

**Current behaviour:** A user whose password is expired or flagged for change will complete the login and receive an authorization code. The browser will exchange that code for an auth token and be logged in—with a password that should not have allowed it. This is a known gap in the hosted Zitadel UI as well and should be filed as an upstream issue if it hasn't been already.

### 4. Handoff Targets the Aurora-Branded Hosted Login

**The design:** When this login page cannot (or should not) complete a login on its own, it hands off to Zitadel's managed login UI. The factors this package can collect are:

- PASSWORD (via plaintext submission to `/zitadel/login`)
- TOTP (via 6-digit code submission to `/zitadel/totp`)

Everything else is uncollectible: passkeys, U2F security keys, SMS OTP, recovery codes, SAML, OIDC federation (unless Zitadel adds those to this API). This login page is deliberately narrow.

**Include-list design:** The `classifyEnrolledMethods` function maintains an include-list of known, collectible method types. If Zitadel's API returns a method type we have never seen before—`AUTHENTICATION_METHOD_TYPE_WEBAUTHN` or `AUTHENTICATION_METHOD_TYPE_SMS`, for example—the function treats it as uncollectible and triggers a handoff. An unknown method type fails closed to `OutcomeHandoff` rather than being silently skipped.

**Hosted login URL:** The handoff target is built by `Handler.handoffURL` using `hostedLoginBaseURL`. It must point to the Aurora-branded v4.15.3+ Zitadel instance (the URL scheme is `/ui/v2/login/login?authRequestID=...`). If `hostedLoginBaseURL` is not configured, the handoff still succeeds but returns an empty `handoff_url`; the auth request itself is preserved and logged.

### 5. The Login-Client PAT Is Instance-Level

**The credential:** Every request in this package is authenticated with a Personal Access Token (PAT) that is instance-level in Zitadel. It can:

- Check any user's password (via `CreatePasswordSession`)
- Verify any user's TOTP code (via `VerifyTOTP`)
- Read any user's enrolled authentication methods
- Mint a session for any user of any product on the shared instance
- Read login policies of any organization

Zitadel offers no narrower role that still permits these operations. This is the most powerful credential auth-bff holds.

**Handling the PAT:** The token is passed to `New` and stored in the `Client` struct. It is automatically added to every request's `Authorization: Bearer` header by the `do` method. **Never log the token.** When debugging, never print the client or the struct that holds it. When tests need to validate auth, assert on the header key, not the token value.

### 6. `forceMfaLocalOnly` Is Not Folded Into `forceMfa`

**The separation:** The `LoginPolicy` struct carries two boolean fields: `ForceMFA` and `ForceMFALocalOnly`. They are not combined with a logical OR; they are kept separate.

- `ForceMFA`: Require MFA for all login methods (password, federated, etc.).
- `ForceMFALocalOnly`: Require MFA for password/local users only. Do not require it for federated logins (Google, Apple, etc.).

**Why this matters for mark8ly:** The mark8ly instance has federated Google and Apple identity providers. Some organizations want password users to use MFA but do not want to force it on Google/Apple federated users (who often have their own MFA via Google Account or Apple ID).

**Comparison to hms:** The hms service (which uses Zitadel) folded these two fields into a single policy decision. That was safe for hms because hms has no federated IdPs—all users are local/password. For mark8ly, folding them would silently force MFA on federated logins, which violates the organization's intent.

The `mfaRequired` function applies both policies correctly:

```go
if p.ForceMFA {
    return true // MFA required for everyone
}
return p.ForceMFALocalOnly && !federated // MFA required for local users only
```

If `federated` is true and `ForceMFALocalOnly` is true, MFA is not required.

### 7. LoginContext Carries Request-Scoped Metadata for a Reason

**The purpose:** `LoginContext` is the handoff type between this package (Zitadel-specific) and the shared post-identity gauntlet (provider-agnostic). It carries:

- `UID`, `Email`, `TenantID`: Identity facts
- `UserAgent`, `IPAddress`, `Device`, `Country`: Client metadata

The request-scoped fields exist because of a narrow near-miss during development.

**The near-miss:** An earlier iteration passed only `UID`, `Email`, and `TenantID` to the gauntlet, leaving the client metadata empty. The shared code includes `deviceguard.Fingerprint("")`, which builds a device fingerprint from the user agent. Passing an empty string produces a constant—every Zitadel user would have the same fingerprint. This would have silently broken device-change detection: the first Zitadel login would be flagged as "new device", but every subsequent login from anywhere would look like the same familiar device.

This bug would have shipped silently—no failing test, no error log—because `Fingerprint("")` is a valid input that returns a constant output.

**The fix:** `LoginContext.UserAgent` must always be populated from `r.UserAgent()`. `IPAddress`, `Device`, and `Country` follow the same pattern that the existing `autologin` handler uses for GIP logins, ensuring the two providers see the same metadata.

Anything added to this boundary must carry the request-scoped fields. Fields built from credential checks (user ID, email, tenant ID) can be populated after authentication; fields built from the HTTP request must be captured at the request boundary.

### 8. Two Open Follow-Ups

#### 8a. Missing End-to-End Coverage for Device Metadata

Only `UserAgent` has a handler-level pinning test (checking that it flows into the response when certain conditions are met). The fields `IPAddress`, `Device`, and `Country` are not proven end-to-end from a real HTTP request. They are extracted and passed correctly by inspection, but a test that exercises a full login flow (or a fixture that mocks a real request) and verifies these fields reach the gauntlet would be prudent. This is low-risk because the code is small and the pattern is clear, but it closes a gap.

#### 8b. Device Label Heuristic Is Duplicated

The `deviceFromUA` function in this package is byte-identical to a function of the same name in `internal/autologin/handler.go`. Both extract a short device label (e.g. "iPhone", "Windows", "Browser") from a User-Agent header. The label is used only for display and audit log context; it never flows into security decisions. 

Because the duplication is presentation-only and unlikely to evolve differently, it is low-severity. However, before either copy diverges—or before a third consumer adds a third copy—the function should be extracted to a shared package (e.g. `internal/useragent/`) with appropriate placement in the codebase hierarchy. When you refactor this, treat it as a pure consolidation: no behavior change, copy the comments, run all handler tests from both packages.

### 9. `login_name` on `/zitadel/totp` Was Client-Supplied and Unverified (FIXED)

**The issue:** `handler.go`'s `totp` read `login_name` from the request body and passed it straight through to `finishComplete`, which used it as `LoginContext.Email`. On `/zitadel/login` that string is verified, because `CreatePasswordSession` checks it against the password in the same call. On `/zitadel/totp` nothing checks it against anything — the password check already happened on the prior `/zitadel/login` call, against a session the caller submitting `/zitadel/totp` may not even own.

**Consequence:** anyone with valid credentials of their own could POST an arbitrary `login_name` to `/zitadel/totp` and receive a session cookie carrying that address, an audit event attributing the login to it, and — when the login looked like a new device — a mark8ly-branded sign-in code mailed to an address of their choosing. UID and FGA membership were unaffected (this is spoofing and unsolicited mail, not privilege escalation), but it was still a real defect.

**The fix:** `Client.UserEmail` (in `client.go`) reads a user's email directly from Zitadel via `GET /v2/users/{id}`, keyed off the user id `SessionFactors` already returns. `finishComplete` now resolves the email this way on BOTH `/zitadel/login` and `/zitadel/totp`, so there is one source of truth regardless of which step is finishing the login. `login_name` from the request body is used for exactly one thing anywhere in this package: the credential check inside `CreatePasswordSession`.

### 10. `CompleteForProvider` Discarded the Step-Up State (FIXED)

**The issue:** `autologin.Service.CompleteForProvider` called `s.completeLogin` and threw away its `*Result`, keeping only the error. When auth-bff's own MFA gate or the new-device email-OTP gate fired inside the gauntlet, `completeLogin` minted a *pending* cookie (not a real session) and returned a nil error — so `finishComplete` would answer `200 {"callback_url": ...}`, telling the browser the login had finished while a step-up was still outstanding. The GIP path (`autologin/handler.go`) surfaces `MFARequired`/`EmailOTPRequired` correctly; the Zitadel path had no way to.

**The fix:** `CompleteFunc` now returns `(CompleteResult, error)` instead of just `error`. `CompleteResult{MFARequired, EmailOTPRequired}` propagates out of `completeLogin` through `CompleteForProvider` to the handler. `finishComplete` checks these flags before answering: if either is set, it responds with the same `{"data": {"uid", "email", "tenant_id", "mfa_required", "email_otp_required"}}` shape the GIP handler uses for the same two cases — never `callback_url` — so the two providers cannot silently diverge in what they tell the browser.

---

## Deliberately Deferred Items

These are known gaps, tracked here so a future reader recognizes them as accepted rather than missed. None block this branch; all are candidates for the phase-3 rewrite of this package's client contract.

1. **The Zitadel `session_token` rides in the `/zitadel/login` JSON response body** when a factor is still required (`OutcomeFactorRequired`). This is bounded today — using it still requires the instance-level login-client PAT, which only this service holds — but the response contract for this endpoint is being rewritten in phase 3, and the token should move into the encrypted pending cookie at that point instead of the JSON body.
2. **All gauntlet errors collapse to `500 internal_error` on the Zitadel path.** `autologin.ErrNotMember` (should be 403), `ErrFGAUnreachable` (503), and email-OTP rate-limiting (429) are all indistinguishable from a generic internal error until phase 3 gives this package a client shaped to carry that distinction to the handler.
3. **`Identity.TenantID` is written by every caller and read by none.** Both `AutoLogin` and `CompleteForProvider` populate it, but nothing downstream consumes the field from `Identity` — `completeLogin` uses `req.WorkspaceTenant` instead. Not a bug (WorkspaceTenant is the correct value to use), but a bit of dead weight worth resolving when this type is next touched.
4. **`arch_test.go` looks up `sufficiency.go` by filename**, not by content or symbol. Renaming that file — for any reason, including an unrelated refactor — would make `TestSufficientWitnessIsOnlyConstructedInSufficiency` and `TestSufficiencyNeverUsesTheUnscopedDisplayPolicy` pass vacuously (nothing named `sufficiency.go` would exist, so the `for` loop / lookup finds nothing to check). Treat any rename of that file as requiring a matching update to `arch_test.go` in the same change.

---

## The Storefront Customer Path

This package also contains `customer_handler.go`, which handles login for the **storefront** (end-customer purchases). It is deliberately incomplete compared to `handler.go` (the merchant admin path) and operates under a different contract.

### Why the Customer Endpoint Mints No Session and Touches No OpenFGA

**The design:** The customer endpoint verifies a credential against Zitadel and returns `{uid, email}`. It stops there. It mints no cookie, calls nothing in `internal/autologin`, and touches no OpenFGA. The storefront's existing sign-in action (see `apps/storefront/app/sign-in/actions.ts`) handles everything else: resolving the store, minting the `mp_customer_session` cookie, and driving profile side effects.

**Why it cannot be "completed" by centralizing session minting here:** The storefront's `mp_customer_session` cookie is minted in a different format from `m8_session` and is scoped to the exact request host — a customer signed in on one store's subdomain (e.g. `store1.mark8ly.com`) must never be handed a session usable on another store's subdomain (e.g. `store2.mark8ly.com`). Minting the session here would require either:
- Adding a third session format to this package (beside `m8_session` and `mp_customer_session`), or
- Having this auth-bff package know about individual store subdomains, violating the separation of concerns between a centralized auth service and store-specific frontend logic.

Both destroy the per-store isolation the storefront's own minting code provides for free. The current design is simpler and safer: auth-bff verifies the identity; the storefront applies the store-scoped isolation.

### The Customer Path Never Calls Finalize

**The design:** The customer path's `login` and `totp` handlers call `DecideSufficiency` and `DecideAfterFactor` from `sufficiency.go`. These methods evaluate whether a session meets the MFA policy and return a decision — `OutcomeComplete`, `OutcomeFactorRequired`, or `OutcomeHandoff` — but they never call the unexported `finalize` function. On `OutcomeComplete`, the customer handler returns `{uid, email}`. No `finalize` call. No authorization code.

**Why no authorization code:** An authorization code is a handle to complete an OIDC flow that requires an `auth_request_id` from an earlier OIDC authorize round trip. The storefront never performs that round trip — it already has an identity from the password/TOTP verification and jumps straight to minting its own session. Asking for an authorization code would be asking for something the storefront has no way to use.

**Why the decision logic is not duplicated:** The merchant path (`Handler`) and customer path (`CustomerHandler`) share the same decision functions in `sufficiency.go`. `CompleteIfSufficient` and `CompleteAfterFactor` in that file call `DecideSufficiency` and `DecideAfterFactor` respectively and then call `finalize` — so they apply the same MFA enforcement with one decision path. Having two separate decision paths, even if they started identical, is how one drifts into a bypass without anyone noticing. By keeping the decision in one place and having one caller path through `finalize` and another skip it, the enforcement stays single-source-of-truth.

### KNOWN LIMITATION (TOTP HALF CLOSED) — Customers with a Second Factor Cannot Complete Sign-In on This Path

**The issue:** A customer whose Zitadel account has enrolled a second factor (TOTP, passkey, U2F, SMS OTP, recovery code) cannot complete the sign-in flow on the storefront.

**TOTP case — CLOSED (`f7078148`, `0797bc3f`):** If the customer has TOTP enrolled, the endpoint returns `{"totp_required": true, "session_id": "...", "session_token": "..."}`. The storefront now has a code-entry screen for this: `confirmCustomerTotp` in `apps/storefront/app/sign-in/actions.ts` submits the code to `verifyCustomerTotp` (mirroring `apps/admin/app/login/actions.ts`'s `confirmZitadelTotp`), and `CustomerSignInForm` renders the entry step and carries `sessionId`/`sessionToken` through — including the repeat-challenge case, where Zitadel hands back a fresh pair on a second wrong-looking attempt rather than a rejection, and the UI must carry the new pair forward or every retry after the first would submit stale credentials. The browser message that used to say "TOTP entry not supported here" is gone for this case.

**Other second factors (passkey, U2F, SMS OTP, recovery code) — STILL OPEN:** The `classifyEnrolledMethods` function maintains an include-list of known, collectible method types. PASSWORD and TOTP are in it; everything else is treated as uncollectible. When a customer has enrolled a method outside this list, the endpoint returns `{"handoff_url": "..."}` pointing to Zitadel's managed login UI. If no hosted login base URL is configured, it returns `{"error": "signin_unavailable"}` (a 503) rather than leaving the customer hung with a dead link. Nothing in this branch collects these methods; the storefront still cannot complete sign-in for them and still routes to the handoff URL, which is deliberately never surfaced as a clickable link (see the `handoff` case in both `customerSignIn` and `confirmCustomerTotp`).

**Why the remaining half is an honest dead end, not an oversight:** As of this writing, **zero customers have a second factor enrolled**. Passkey/U2F/SMS OTP/recovery-code entry is real UI work that belongs with the Google trampoline phase (3c-2) when that work is planned. Before the original fix, both conditions rendered as the identical error message — "Email or password is incorrect" — which incorrectly told a customer with a correct password that it was wrong. Now the messages are honest: TOTP is collected inline, and "Complete your sign-in in our full login UI" for the remaining methods.

### KNOWN LIMITATION — Network Timing May Still Distinguish a Wrong Password from an Unknown User

**The limitation:** The customer endpoint returns byte-identical responses for a user-not-found error and a bad-credentials error: both produce `401 {"error": "invalid_credentials"}`. This is asserted by tests. However, Zitadel's internal password hashing happens on different code paths — roughly 0.7 seconds was observed in a sibling project for the correct-username-wrong-password case, while the user-not-found path is faster.

**Why this is not ours to fix:** This storefront is a public website that anyone can probe. The endpoint's response symmetry is correct. Zitadel's latency asymmetry is Zitadel's problem, not this package's. If the consumer of this data wants to mitigate timing attacks from the browser side, they can use a client-side artificial delay before rendering the error, but this endpoint alone cannot close that gap without making legitimate requests artificially slow. Do not interpret the identical response bodies as proof of protection against timing attacks; they are necessary but not sufficient.

### The Google Trampoline Is Phase 3c

**The design:** The onboarding wizard (where merchants start) and the storefront (where customers purchase) both use Google Sign-In. When a user signs in with Google, the sign-in token comes from Google's Identity Services JavaScript SDK (`accounts.google.com/gsi/client`). To bounce that token from the multi-tenant mark8ly.com trampoline back to a tenant's own subdomain (e.g. `mystore.mark8ly.com`), the token is wrapped in an HMAC-signed exchange code and sent to the target store's `/auth/google/finish` handler.

**Constraint that makes it its own phase:** The onboarding app's Content-Security-Policy allowlists `accounts.google.com/gsi/client` by host with no `strict-dynamic`. Any replacement or supplementary script origin needs an explicit CSP change, which requires coordination across multiple deployments. Additionally, the exchange-code protocol carries its own HMAC-based authentication and host-matching defenses. This phase (3b) touches only the Zitadel customer login path and leaves the Google trampoline untouched. Replacing or securing that integration is phase 3c.

### Storefront Google Controls Are Hidden on the Zitadel Path (`98894b55`, `463d56cc`)

**The problem this closes:** The Google Sign-In/Sign-Up/link-account controls on the storefront all still authenticate through the trampoline described above, which is GIP end-to-end. Once a store runs `NEXT_PUBLIC_AUTH_PROVIDER=zitadel`, offering those controls would let a customer "sign in with Google" through an identity store mark8ly is migrating off, alongside — and disconnected from — the Zitadel identity the rest of the sign-in flow now uses.

**The fix:** `apps/storefront/lib/auth/provider.ts` exports `getAuthProvider()` and `isGoogleSignInOffered()` as the single source of truth for this decision, matching `apps/storefront/app/sign-in/actions.ts`'s `AUTH_PROVIDER` rule exactly (only the literal string `"zitadel"`, case-sensitive, switches off GIP — pinned by `provider.test.ts`'s wrong-case and non-boolean cases). `CustomerSignInForm.tsx`, `CreateAccountForm.tsx`, and `app/account/security/SecurityClient.tsx` all gate their Google control on `isGoogleSignInOffered()`. The function lives under `lib/auth/` specifically so it is reachable by `apps/storefront`'s own vitest config (see the note on `components/**` coverage below) rather than only being exercisable by inspection inside a component.

**What this does not do:** It only hides the controls; it does not make Google-through-Zitadel work. That is phase 3c-2, described next.

### What Phase 3c-2 Still Owes

Phase 3c-1 closed the TOTP half of the second-factor gap, closed the exchange-code `kind` ambiguity, and hid the storefront's Google controls so they stop pointing at GIP once a store is on Zitadel. It did not make Google sign-in work under Zitadel, and — this is the gap most likely to bite — it did not touch customer **sign-up** at all. Concretely, phase 3c-2 owes:

- **Customer sign-up is still GIP-only, full stop.** `apps/storefront/app/create-account/actions.ts` has no `AUTH_PROVIDER` branching whatsoever, and `CreateAccountForm.tsx`'s `signUpWithPassword` calls `identitytoolkit.googleapis.com/v1/accounts:signUp` unconditionally — there is no Zitadel-aware path here at all. Hiding that page's Google button via `isGoogleSignInOffered()` (see above) is cosmetic: it removes one button, but the email/password sign-up underneath still creates a GIP user regardless of `NEXT_PUBLIC_AUTH_PROVIDER`. The concrete failure this produces: a store running `NEXT_PUBLIC_AUTH_PROVIDER=zitadel` lets a shopper create an account, which mints a GIP identity; the shopper then goes to sign in, `customerSignIn` asks Zitadel about them, Zitadel has never heard of this user, the outcome is `rejected`, and `apps/storefront/app/sign-in/actions.ts` returns "Email or password is incorrect." That is exactly the false-password message the "Customers with a Second Factor Cannot Complete Sign-In" limitation above describes fixing for legitimate Zitadel customers hitting an uncollectible factor — reached here instead through the create-account door, for a customer who was never in Zitadel to begin with. Completing the customer path for phase 3c-2 means porting sign-up to Zitadel, not just sign-in.
- **Google sign-in through Zitadel**, and re-enabling the three storefront controls hidden above once that path is real. Until then, `isGoogleSignInOffered()` returning `false` under `NEXT_PUBLIC_AUTH_PROVIDER=zitadel` is the correct, permanent-until-3c-2 answer — not a bug to work around.
- **An Apple IDP.** Zitadel has no Apple identity provider configured yet; mark8ly's Apple sign-in story on Zitadel does not exist.
- **Using the Zitadel org's existing Google IDP.** Verified 2026-09-03 against the live instance: a Google IDP already exists on the Zitadel side — id `386381087862948767`, owned by the TESSERIX org, active, `isAutoCreation: true`, `autoLinking: AUTO_LINKING_OPTION_EMAIL`. It is declared in git at `tesserix-k8s/charts/apps/zitadel-bootstrap/values.yaml` and asserted — never created — by `reconcile_org_idps` in that chart's `bootstrap.py`. This is deliberate: the reconciler is never handed a client secret, so the IDP itself must be created once by hand through the Zitadel console; automation only keeps its configuration in sync afterward.
- **A caveat to carry into that work:** the Google IDP's OAuth client id is not the same client as the apps' `NEXT_PUBLIC_GOOGLE_CLIENT_ID`. Both live in Google project `849928263410`, and because Google's `sub` claim is stable per project rather than per OAuth client, IDP links still resolve correctly despite the mismatch; email auto-linking (`AUTO_LINKING_OPTION_EMAIL` above) is a second backstop on top of that. The Zitadel migration spec warns that IDP links only work when the client id matches, so this discrepancy is worth keeping visible rather than rediscovering it under pressure.
- **A Zitadel API gotcha that cost real debugging time verifying the above:** the v1 IDP endpoints (`/admin/v1/idps/_search`, `/management/v1/idps/_search`, `GET /management/v1/idps/{id}`) return empty results or "doesn't exist" even for an IDP that is active and in use. Only `/management/v1/idps/templates/_search` and `/management/v1/policies/login/idps/_search` show providers created through the v2 console flow. Anyone re-verifying IDP state should start with those two endpoints, not the v1 ones.

### First-Time Google Sign-In: Registration and Account Linking

**The design:** `idp/finish`'s unlinked branch (a Google sign-in whose intent resolves to no existing Zitadel user) checks whether a Zitadel user already exists with that exact, verified email (`Client.FindUserByVerifiedEmail`). If one does, the identity is attached to THAT existing account (`Client.LinkIDPToUser`) rather than creating a second, disconnected one for the same person; if not, a new user is registered pre-linked (`Client.CreateHumanUserWithIDPLink`). Either way, the very next sign-in for that provider identity resolves `ZitadelUserID` directly and takes neither path again.

**The absolute gate on both paths:** an unlinked federated identity may be attached to an account — new or existing — ONLY when the provider asserts that email is verified. `identity.EmailVerified` is read soft from the provider's raw claims (see `IDPIdentity`'s doc) and defaults to false when the claim is absent, so an absent claim refuses exactly like an explicit false. Skipping this check is how a federated login becomes an account-takeover primitive: anyone able to register a victim's address at any federated provider would otherwise inherit that victim's existing account.

**Verified 2026-09-04 against the live TESSERIX Zitadel instance** (a throwaway user was created, linked, checked, and deleted): `POST /v2/users/human` with an `idpLinks` array attaches the link at creation exactly as modelled; `POST /v2/users` as a search (not a create — easy to misread from the path alone) with an `emailQuery`/`TEXT_QUERY_METHOD_EQUALS` body returns the matching user; `POST /v2/users/{userId}/links` with an `idpLink` body attaches a link to an already-existing user and returns 200. All three calls in `client.go` (`FindUserByVerifiedEmail`, `CreateHumanUserWithIDPLink`, `LinkIDPToUser`) match the confirmed shapes. One asymmetry worth knowing if this is extended: `POST /v2/users/{userId}/links/_search` (not currently called by this package) reports the external id back under `userId`, not `providedUserId`.

**Placeholder profile names, deliberately:** `CreateHumanUserWithIDPLink` sets `givenName` to the email's local part and `familyName` to the literal `"Member"`. This is not an oversight — Zitadel's `AddHumanUser` requires both non-empty, and a federated identity carries no reliable first/last name split (only `email`/`email_verified` are read from the provider's raw claims elsewhere in this package; see `readRawEmail`). The placeholder makes the account immediately usable for sign-in; a merchant can correct their own display name afterward like any other profile field.

### Carried from Phase 3a: `exchange-code.ts` Has No `kind` Field (FIXED, `3af10ba2`)

**The issue:** The file `packages/ui/src/auth/exchange-code.ts` mints and verifies HMAC-signed exchange codes used by the Google trampoline. It has two sibling files that also mint and verify codes for different purposes — `admin-handoff-code.ts` and `zitadel-totp-code.ts`. All three use `SESSION_ENCRYPT_KEY` as their signing key. However, unlike its two siblings, `exchange-code.ts` had no `kind` field to distinguish its intended purpose — a code minted for one purpose would pass the HMAC signature check when validated by another purpose's verifier, because they share the same key and the payload structure is compatible.

**Example of the risk:** If a code minted for "Google sign-in on the storefront" and a code minted for "password-reset confirmation" both use the same key and are both JSON payloads, an attacker could swap them and potentially complete the wrong action (if the receiving handler doesn't validate the intent strictly enough).

**The fix:** `exchange-code.ts` now exports `EXCHANGE_CODE_KIND = "google_exchange_v1"`, stamps it into the payload when minting, and its verifier rejects a mismatch with error code `"wrong_kind"` before doing anything else with the claims. This came to light in phase 3a and closed in phase 3c-1; the Google trampoline itself is still untouched — see "The Google Trampoline Is Phase 3c" above — this closes only the cross-purpose code-swap risk, not the trampoline's broader deferral.

### Note: `components/**` Auth Logic Is Untested Under `apps/storefront`'s Own Vitest Config

**The gap:** `apps/storefront/vitest.config.ts` includes only `lib/**/*.{test,spec}.{ts,tsx}` and `app/**/*.{test,spec}.{ts,tsx}`. Storefront components under `components/**` — including `CustomerSignInForm.tsx` and `CreateAccountForm.tsx` — are not covered by that config at all. This is why `getAuthProvider`/`isGoogleSignInOffered` (see above) were deliberately placed under `lib/auth/provider.ts` rather than inlined in a component: it's the only place in the storefront app where a unit test can reach the decision.

**The bigger exposure:** the three HMAC code modules under `packages/ui/src/auth/` (`exchange-code.ts`, `admin-handoff-code.ts`, `zitadel-totp-code.ts`) live outside every app's own package and have no `vitest.config.ts` of their own. They are executed today only because `apps/admin/vitest.config.ts` explicitly adds `../../packages/ui/src/**/*.test.{ts,tsx}` to its `include` list. If that one line is ever dropped — during an admin vitest config cleanup, for instance — all three modules' tests, `exchange-code.test.ts` included, go silently untested; `npm test` in `apps/admin` would still pass, just against a smaller surface, with nothing to say so.

**This is pre-existing, not introduced by this branch.** The `include` line predates phase 3c-1 and none of this branch's tasks touched `apps/admin/vitest.config.ts`. It is recorded here because this branch added tests to one of the three modules (`exchange-code.test.ts`) and that made the fragility visible.

---

## Testing and Validation

The three architecture tests (`TestFinalizeIsOnlyCalledFromSufficiency`, `TestSufficientWitnessIsOnlyConstructedInSufficiency`, `TestSufficiencyNeverUsesTheUnscopedDisplayPolicy`) are not unit tests in the traditional sense. They are code-inspection tests that verify file-level invariants. They catch the obvious mistakes but accept the blind spot described in section 2.

When writing new tests or modifying `sufficiency.go`:

1. Run `go test ./...` to ensure all tests pass, including the architecture tests.
2. Any new path to `finalize` is a design decision. Document it and consider whether an architecture test should be updated to pin the decision logic itself.
3. Do not treat a passing architecture test as proof that MFA enforcement is sound. Read the code.
