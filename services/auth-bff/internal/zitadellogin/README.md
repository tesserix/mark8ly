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

## Testing and Validation

The three architecture tests (`TestFinalizeIsOnlyCalledFromSufficiency`, `TestSufficientWitnessIsOnlyConstructedInSufficiency`, `TestSufficiencyNeverUsesTheUnscopedDisplayPolicy`) are not unit tests in the traditional sense. They are code-inspection tests that verify file-level invariants. They catch the obvious mistakes but accept the blind spot described in section 2.

When writing new tests or modifying `sufficiency.go`:

1. Run `go test ./...` to ensure all tests pass, including the architecture tests.
2. Any new path to `finalize` is a design decision. Document it and consider whether an architecture test should be updated to pin the decision logic itself.
3. Do not treat a passing architecture test as proof that MFA enforcement is sound. Read the code.
