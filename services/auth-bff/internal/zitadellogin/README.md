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

### Storefront Google Controls Were Hidden on the Zitadel Path, Then Re-Enabled (`98894b55`, `463d56cc`; re-enabled in phase 3c-2)

**The problem this originally closed:** The Google Sign-In/Sign-Up/link-account controls on the storefront all authenticated through the trampoline described above, which is GIP end-to-end. Before phase 3c-2, once a store ran `NEXT_PUBLIC_AUTH_PROVIDER=zitadel`, offering those controls would have let a customer "sign in with Google" through an identity store mark8ly is migrating off, alongside — and disconnected from — the Zitadel identity the rest of the sign-in flow used.

**The mechanism:** `apps/storefront/lib/auth/provider.ts` exports `getAuthProvider()` and `isGoogleSignInOffered()` as the single source of truth for this decision, matching `apps/storefront/app/sign-in/actions.ts`'s `AUTH_PROVIDER` rule exactly (only the literal string `"zitadel"`, case-sensitive, switches off GIP — pinned by `provider.test.ts`'s wrong-case and non-boolean cases). `CustomerSignInForm.tsx`, `CreateAccountForm.tsx`, and `app/account/security/SecurityClient.tsx` all gate their Google control on `isGoogleSignInOffered()`. The function lives under `lib/auth/` specifically so it is reachable by `apps/storefront`'s own vitest config (see the note on `components/**` coverage below) rather than only being exercisable by inspection inside a component.

**Now that Google-through-Zitadel exists (phase 3c-2, below), `isGoogleSignInOffered()` unconditionally returns `true`.** The function is kept, rather than being deleted and its call sites replaced with a literal `true`, so a future gap in Google coverage (a third auth provider, for instance) has one place to express "is Google offered at all" again — see the function's own comment.

### Phase 3c-2: Google Sign-In Through Zitadel, on Both Web Surfaces

Phase 3c-1 closed the TOTP half of the second-factor gap, closed the exchange-code `kind` ambiguity, and hid the storefront's Google controls so they stop pointing at GIP once a store is on Zitadel. Phase 3c-2 is what makes Google-through-Zitadel real: `POST /auth/zitadel/idp/{start,finish}` for the merchant admin, `POST /auth/customer/idp/{start,finish}` for the storefront customer, plus the Next.js routes on both apps that Zitadel's browser redirect actually lands on (`apps/admin/app/auth/idp/finish/route.ts`, `apps/storefront/app/auth/idp/finish/route.ts`). `isGoogleSignInOffered()` (see above) now unconditionally returns `true` — the controls it used to hide are live on both providers.

**The mark8ly.com trampoline described above is NOT part of this path.** That trampoline exists because Google's OAuth client has exactly one fixed registered origin, so a token minted on the shared `mark8ly.com` domain has to be bounced back to a tenant's own subdomain by hand. Zitadel's IDP-intent API takes a `successUrl`/`failureUrl` per request instead — Zitadel does the bouncing — so the browser goes straight back to the store's own host. Both `route.ts` files say this explicitly in their header comments. The trampoline remains exactly as documented above, but only for the GIP path; it does not apply here and nothing in this phase touches it.

**Zitadel does not validate `successUrl` at all — verified live.** An intent pointing `successUrl` at `https://evil.example.com/x` was accepted and returned a working Google `authUrl` with no complaint. `returnurl.go`'s `ReturnURLAllowlist` is therefore not defence in depth; it is the entire control against handing a finished Google sign-in to an attacker's domain, and it MUST run before `StartIDPIntent`, which it does in both `idpStart` implementations. `internal_auth.go`'s `X-Internal-Auth` check (also required, and checked even earlier, before the request body is read) closes off unauthenticated callers of this endpoint entirely — it doesn't add anything on top of the allowlist for a caller who *is* authorized to call it.

**The allowlist is split admin/storefront on purpose, not out of caution.** `cmd/server/main.go`'s `newZitadelHandlers` builds two separate `ReturnURLAllowlist` values — `ZitadelReturnURLAllowedHosts/SuffixesAdmin` for the merchant `Handler`, `ZitadelReturnURLAllowedHosts/SuffixesStorefront` for the `CustomerHandler` — and never merges them. A single flat list containing a `.mark8ly.com` suffix would admit every tenant storefront subdomain, and merchants self-provision those at signup — which would make a malicious merchant's own `evil-shop.mark8ly.com` a valid `successUrl` destination for a completed *admin* sign-in. The failure mode is one swapped argument at the call site, which is why `main_test.go`'s `TestZitadelHandlersUseTheCorrectReturnURLAllowlist` exists: it drives both handlers' real `/idp/start` routes with an admin-only host and a storefront-only host and fails if `main.go` ever wires the wrong pair to the wrong handler — nothing in `internal/zitadellogin`'s own tests would ever notice that swap, since that package only ever sees whatever allowlist it's handed.

**A dangerous allowlist entry is rejected at boot, not at request time.** `NewReturnURLAllowlist` refuses a bare TLD, a stray leading/trailing/doubled dot, or an entry carrying a scheme, path, credential, or wildcard character — see `validateDomainEntry` in `returnurl.go`. Given that Zitadel performs no `successUrl` validation of its own (above), a Helm-values typo that slipped past this check would silently turn the only open-redirect control into a no-op while everything still looked configured.

**The `user` param on Zitadel's success redirect is never an identity, on either app.** Success carries `id` and `token` (plus `user`, only when Zitadel believes the identity is already linked); failure carries `id`, `error`, and `error_description` — see `StartIDPIntent`'s doc in `idpintent.go`. `user` rides in a URL the browser followed and is attacker-controlled. Both `idpFinishRequest.User` (merchant) and `customerIDPFinishRequest.User` (customer) are decoded and never read again; the authoritative identity comes only from `RetrieveIDPIntent(intentID, intentToken)`. Both `route.ts` files repeat this in their own header comments so the rule survives being read from either side of the HTTP boundary.

**The IDP is pinned on both paths.** Both `idpFinish` implementations check `identity.IDPID == h.googleIDPID` before any lookup, link, or create — see the check immediately after `RetrieveIDPIntent` in `handler.go` and the identical one in `customer_handler.go`. Without it, an intent from any other provider configured on the instance would be trusted exactly like Google's, including its `email_verified` claim — and, as of 2026-09-04, an Apple IDP exists on the same Zitadel org, so this is not a hypothetical weaker-provider scenario.

**Linking or creating an account from an unlinked identity requires a provider-verified email, on both paths, absolutely.** `identity.EmailVerified` is read soft from the provider's raw claims and defaults to `false` when the claim is absent — an absent claim refuses exactly like an explicit `false`. Skipping this is how a federated login becomes an account-takeover primitive: whoever can register a victim's address at Google (or whatever IDP is pinned) would otherwise inherit the victim's existing account by simply signing in with it.

**The merchant path is link-only; the customer path self-registers — see the "LINK-ONLY, Never Create" section above for the merchant reasoning.** The customer path in `customer_handler.go`'s `idpFinish` takes the opposite decision at the exact same fork: when no existing account matches the verified email, it calls `Client.CreateHumanUserWithIDPLink` and registers a new user. The reason is not stylistic symmetry-breaking — merchant authorization is FGA tenant membership keyed by user id, so a merchant user created on the spot is guaranteed to fail the post-identity gauntlet a few lines later (garbage rows, not accounts); a shopper carries no such membership requirement, so a freshly created customer account is simply a normal new customer.

**`email_taken` is a distinct, usually-permanent outcome — not a re-tryable race.** `FindUserByVerifiedEmail` only matches *verified* emails, so when it returns no match and the subsequent `CreateHumanUserWithIDPLink` call still 400s with `ErrEmailAlreadyExists`, that means some UNVERIFIED account already holds the exact email — an abandoned signup, an unverified invite, or an attacker who typed the victim's address and set their own password. Refusing is correct: proving the person owns the address at Google does not make it safe to link them to an account someone else may control, for the same reason linking an unverified email to an *existing* account is refused above. `customer_handler.go`'s comment on this branch is explicit that it is usually permanent (retrying changes nothing while that unverified account exists) but can rarely be a genuine race against a concurrent request — and that it must stay a distinct error code from `email_ambiguous` (which is `FindUserByVerifiedEmail` itself finding more than one *verified* match) so logs and customer-facing copy don't conflate two different failures.

**Admin Google sign-in only works on a per-tenant admin subdomain (`{slug}-admin.mark8ly.com`).** Unlike the password path, where the tenant is known before the identity is (typed into a form), the admin `idp/finish` route needs `workspace_tenant` *before* it can call auth-bff, because the Google identity isn't known yet at that point. `apps/admin/app/auth/idp/finish/route.ts` resolves it via `tenantIdForHostSlug(forwardedHost)`, matching the same `{slug}-admin.mark8ly.com` pattern the GIP path's `resolveWorkspaceTenant` already uses. A request that doesn't land on a per-tenant admin host has no tenant to guess and fails closed with `store_not_found`.

**A step-up cannot complete through this redirect-only flow.** If `finishComplete`/`finishComplete`'s customer-side equivalent reports `totpRequired`/`mfaRequired`/`emailOtpRequired` after a Google sign-in, `apps/admin/app/auth/idp/finish/route.ts` maps it to `step_up_unsupported` and sends the merchant back to `/login` rather than stranding them on a broken continuation — there's no interactive form on this GET-redirect route to collect a TOTP code, and Zitadel's own step-up session id/token must not travel in a URL. The merchant completes the same account's second factor through the ordinary password + TOTP path instead.

**A pre-existing defect surfaced and fixed while wiring the admin route (`a2ada8f4`):** `finishComplete` in `handler.go` answers a genuinely completed Zitadel login with `{"callback_url": ...}` alone — no `data`, no `uid`, no `tenantId` — but `apps/admin/lib/auth/login-response.ts`'s `parseLoginResponse` required `uid`/`tenantId` to recognize the `"complete"` outcome and threw otherwise. Every real completed Zitadel sign-in (password included, not just this phase's Google path) therefore rendered a generic "Something went wrong" error. It shipped unnoticed because the module's own test fixtures supplied `uid`/`tenantId` fields auth-bff never actually sends — a fixture is only as trustworthy as its fidelity to the real wire format. The fix makes `uid`/`email`/`tenantId` optional on `"complete"`, requiring only that at least one of an identity or a `callback_url` be present; no caller in the admin app reads those fields off this outcome anyway (each caller already carries its own server-resolved tenant id separately).

**An Apple IDP now exists on the Zitadel org**, alongside Google — the reasoning above about IDP pinning is no longer a hypothetical. It has never been exercised end to end: Zitadel accepts and stores its `.p8` private key without validating it, and Apple only rejects a bad client secret when a real Apple sign-in is attempted. Verifying it is real remains open work.

**Using the Zitadel org's existing Google IDP.** Verified 2026-09-03 against the live instance: a Google IDP already exists on the Zitadel side — id `386381087862948767`, owned by the TESSERIX org, active, `isAutoCreation: true`, `autoLinking: AUTO_LINKING_OPTION_EMAIL`. It is declared in git at `tesserix-k8s/charts/apps/zitadel-bootstrap/values.yaml` and asserted — never created — by `reconcile_org_idps` in that chart's `bootstrap.py`. This is deliberate: the reconciler is never handed a client secret, so the IDP itself must be created once by hand through the Zitadel console; automation only keeps its configuration in sync afterward.
- **A caveat carried into this work:** the Google IDP's OAuth client id is not the same client as the apps' `NEXT_PUBLIC_GOOGLE_CLIENT_ID`. Both live in Google project `849928263410`, and because Google's `sub` claim is stable per project rather than per OAuth client, IDP links still resolve correctly despite the mismatch; email auto-linking (`AUTO_LINKING_OPTION_EMAIL` above) is a second backstop on top of that. The Zitadel migration spec warns that IDP links only work when the client id matches, so this discrepancy is worth keeping visible rather than rediscovering it under pressure.
- **A Zitadel API gotcha that cost real debugging time verifying the above:** the v1 IDP endpoints (`/admin/v1/idps/_search`, `/management/v1/idps/_search`, `GET /management/v1/idps/{id}`) return empty results or "doesn't exist" even for an IDP that is active and in use. Only `/management/v1/idps/templates/_search` and `/management/v1/policies/login/idps/_search` show providers created through the v2 console flow. Anyone re-verifying IDP state should start with those two endpoints, not the v1 ones.

#### What Still Isn't Done

- **CUTOVER BLOCKER: tenant custom domains.** `ZitadelReturnURLAllowedHosts/SuffixesStorefront` is a static, env-configured list (`config.go`). A merchant's custom domain is resolved dynamically by `tenant-router-service` at request time and can never appear in a list fixed at deploy time — so a storefront login on a custom domain will be rejected by the allowlist the moment the Zitadel flag flips on for that store. This needs a dynamic lookup against `tenant-router-service` (or equivalent) before it can go out; it is not built, and nothing in this phase touches it.
- **Customer sign-up is still GIP-only.** `apps/storefront/app/create-account/actions.ts` still delegates unconditionally to the GIP-backed `customerSignIn` after a client-side `identitytoolkit.googleapis.com` sign-up — there is no `AUTH_PROVIDER` branching there. Google sign-in now self-registers (`CreateHumanUserWithIDPLink`, above), so the Google route into a Zitadel-backed store is covered; email/password sign-up is not, and still mints a GIP identity a Zitadel-backed store's `customerSignIn` won't recognize.
- **The storefront CSP still allowlists `accounts.google.com/gsi/client`** (`apps/storefront/lib/security/csp.ts`) for the GIP trampoline path. The Zitadel redirect flow this phase built needs no client-side script at all — it's a server-driven redirect round trip — so that CSP entry can be dropped once GIP is retired, not before.

### First-Time Google Sign-In: The Merchant Path Is LINK-ONLY, Never Create

**The design:** `idp/finish`'s unlinked branch (a Google sign-in whose intent resolves to no existing Zitadel user) checks whether a Zitadel user already exists with that exact, verified email, scoped to the merchant org (`Client.FindUserByVerifiedEmail`). If one does, the identity is attached to THAT existing account (`Client.LinkIDPToUser`). If not, this endpoint REFUSES (`403 {"error":"no_admin_account"}`) — it never registers a new user.

**Why link-only, by ruling (2026-09-04):** an earlier version of this endpoint also registered a brand-new user (`Client.CreateHumanUserWithIDPLink`) when no match existed. Two independent reasons killed that: (1) merchant authorization is FGA tenant membership keyed by user id — a user that did not exist a moment ago cannot be a member of anything, so a freshly created merchant user is GUARANTEED to fail the post-identity gauntlet a few lines later; creating one is pure garbage generation, unbounded user-table growth by any unauthenticated visitor with a Google account, and (2) every such row becomes a future ambiguous-match target — see the next point. Merchants get an account through onboarding, not through the login page. `CreateHumanUserWithIDPLink` still exists as a primitive in `client.go` for a future customer path, where self-registration IS the desired behaviour, but `idp/finish` never calls it.

**Three checks stand between an intent and either outcome, all found by the same adversarial review and all load-bearing:**

1. **The intent must have come from the configured Google IDP.** The instance can (and does, as of 2026-09-04) carry more than one IDP — an Apple IDP was added the same day. Without pinning `identity.IDPID == googleIDPID`, an intent started against a WEAKER or attacker-influenced provider would be trusted exactly like Google's: register `victim@merchant.com` there, POST the resulting id/token here, and this endpoint would link it straight onto the victim's account. This check runs before any lookup, link, or (formerly) creation call.
2. **`FindUserByVerifiedEmail` is scoped to the merchant org and refuses ambiguity.** The login-client PAT is instance-level and Zitadel's email uniqueness is per-org, so two different orgs on the shared instance can each hold a verified copy of the same email — an unscoped, first-match search could bind a merchant's Google identity to an account in a completely unrelated org. More than one match, even within one scoped search, is refused (`409 {"error":"email_ambiguous"}`) rather than picked from — see `ErrAmbiguousEmailMatch`.
3. **The search is case-insensitive.** `TEXT_QUERY_METHOD_EQUALS_IGNORE_CASE` plus Go-side `strings.EqualFold`. Zitadel's own account uniqueness is case-insensitive; a case-sensitive search here would simply never find `Person@x.com` for Google's `person@x.com`, reading as "no match" (and, when this endpoint still created users, would fall through to a create Zitadel itself rejects with 400 for the same reason — see `ErrEmailAlreadyExists` on `CreateHumanUserWithIDPLink`, kept for whoever calls that primitive next).

**The absolute gate underneath all of this:** an unlinked federated identity may be attached to an existing account ONLY when the provider asserts that email is verified. `identity.EmailVerified` is read soft from the provider's raw claims (see `IDPIdentity`'s doc) and defaults to false when the claim is absent, so an absent claim refuses exactly like an explicit false. Skipping this check is how a federated login becomes an account-takeover primitive.

**Verified 2026-09-04 against the live TESSERIX Zitadel instance** (a throwaway user was created, linked, checked, and deleted): `POST /v2/users/human` with an `idpLinks` array attaches the link at creation exactly as modelled; `POST /v2/users` as a search (not a create — easy to misread from the path alone) with an `emailQuery` body returns the matching user; `POST /v2/users/{userId}/links` with an `idpLink` body attaches a link to an already-existing user and returns 200. All three calls in `client.go` (`FindUserByVerifiedEmail`, `CreateHumanUserWithIDPLink`, `LinkIDPToUser`) match the confirmed shapes; the org-scoping header and `IGNORE_CASE` query method added afterward follow the same documented request shape but were not independently re-verified live. One asymmetry worth knowing if this is extended: `POST /v2/users/{userId}/links/_search` (not currently called by this package) reports the external id back under `userId`, not `providedUserId`.

**Profile names prefer Google's claims, and never read as merchant-flavoured.** `CreateHumanUserWithIDPLink` (called by the customer path's `idp/finish` for a first-time verified identity) uses `identity.GivenName`/`FamilyName` — read best-effort from the same raw userinfo payload as `email`/`email_verified` (see `readRawName`) — when Google sent them. Only when a claim is absent does it fall back: the email's local part for `givenName`, and the neutral `"User"` for `familyName`. The literal `"Member"` this used to fall back to unconditionally is gone — that word reads as an organization-membership term, which is wrong on what is usually a shopper account. Zitadel's `AddHumanUser` requires both fields non-empty, which is the only reason a fallback exists at all; the identity's own account holder can correct their display name afterward like any other profile field. Both values are bounded (`boundedProfileName`) before being sent, since a provider payload is untrusted input even for a field this package makes no trust decision on.

**Intent ids are truncated out of logged error strings.** `Client.do`'s error strings normally embed the request path for debugging; `RetrieveIDPIntent` overrides that (`withLogPath("/v2/idp_intents/{id}")`) so a caller intent id — not a secret the way its token is, but still request-scoped input — does not ride along into every error line an operator might log. The token itself was never logged at all.

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
