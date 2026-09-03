# Migrating mark8ly authentication from GIP to Zitadel

Design for #524. Written 2026-09-03.

Every claim marked **VERIFIED** below was observed against the live instance
(`auth.tesserix.app`, zitadel v4.15.3) or against production data on
2026-09-03, not read from documentation. Throwaway users and projects created
during verification were deleted and confirmed gone.

## Why this is smaller than #524 assumes, and where the risk actually is

#524 lists `autologin`, `deviceguard`, `emailotp`/`loginotp`, `adminhandoff`,
`session` and MFA/TOTP as needing "a Zitadel equivalent or a deliberate
decision to drop". **None of them call GIP.** They are our own code sitting
downstream of a GIP-issued `sub`.

The real GIP surface is:

1. Two independent token verifiers — `auth-bff`'s hand-rolled JWKS verifier
   (`keyfunc` + `golang-jwt`) and `marketplace-api`'s Firebase Admin SDK
   verifier (`internal/auth/gip_verifier.go`) for mobile bearer tokens. Only
   the second reads the `tenant_id` custom claim.
2. `gipadmin` — five Identity Toolkit REST endpoints.
3. `gipkey` — deleted outright, see below.
4. `/auth/me/providers`.

The sibling checkout `tesserix-new/auth-bff` is a different, stale service
(module `github.com/tesserix/auth-bff`, last commit 2026-04-04) and is **not**
in scope. All live auth code is `mark8ly/services/auth-bff`.

**There are no users to migrate.** VERIFIED: production GIP holds 13 accounts
across all mark8ly pools (`MP-Internal-e986p` 5, `MP-Customer-39opy` 4,
`Platform-9bu14` 3, UAT pools 1). Four carry a password hash. **Zero** have MFA
enrolled. `mark8ly_marketplace_api` agrees: 5 customer profiles, 11 user
profiles, 3 orders, 3 stores. `user_mfa` in `mark8ly_platform_api` has **0
rows**.

So the risk is entirely in the code, not the data — which is what #524 says:
five defects (#493, #499, #502, #503, #504) shipped with passing tests, all in
the seams. This design's job is to not add more seams.

**Timing.** This migration is far cheaper now than after the CRM push converts
leads. Every converted merchant brings passwords, staff accounts and storefront
customers. The data risk grows; the code risk does not shrink by waiting.

## Decisions

| # | Decision |
|---|---|
| D1 | One Zitadel org: `TESSERIX`. No new orgs, no org-per-tenant. |
| D2 | Two projects: `mark8ly-admin` (`restricted`) and `mark8ly-storefront` (`public`). |
| D3 | Full login-client model over the OIDC authorization request. Our existing login page is kept. |
| D4 | GIP UIDs are preserved as Zitadel `userId`. |
| D5 | No user migration. Recreate the four password accounts by hand. Atomic cutover, no dual-run. |
| D6 | Delete `usermfa`; use Zitadel TOTP. Keep `deviceguard` and `emailotp`. |
| D7 | Drop the `tenant_id` custom claim; resolve tenancy from FGA. |
| D8 | Delete `gipkey` and the server/browser API key split. |
| D9 | `Platform-9bu14` is out of scope — it is tesserix-home's pool and no mark8ly Go code reads it. |

### D1 — one org, one human, one account

The org is Zitadel's user namespace; projects and apps are not. One org
therefore means **one human, one user account**. A merchant who also shops is
the same user with the same `sub`, holding different authority in different
places. There is no duplicate-email ambiguity because no second account is ever
created.

This matters because two users sharing an email make login non-deterministic —
a trap tesserix-home hit in production on 2026-08-19.

`customer_profiles` is unaffected: it is keyed `(store_id, email)` and
`(store_id, gip_uid)`, so one uid across several stores is already the normal
case.

### D2 — two projects, because app-level isolation does not exist

VERIFIED: an OIDC application's entire configuration is `accessTokenType,
allowedOrigins, appType, authMethodType, clientId, clockSkew, grantTypes,
idTokenUserinfoAssertion, redirectUris, responseTypes`. There is **no field
restricting which users may authenticate to an application**. The isolation
boundary is the project.

Today `mp-internal` and `mp-customer` are separate GIP pools, so a storefront
customer cannot authenticate against the internal surface at all. That property
is preserved structurally by splitting the projects and setting
`mark8ly-admin` to `restricted` (`projectRoleCheck: true`), rather than
behaviourally by relying on our own checks.

| Project | Access | Applications |
|---|---|---|
| `mark8ly-admin` | `restricted` | Mark8ly Admin Web (User Agent, `admin.mark8ly.com`), Mark8ly Admin Mobile (Native) |
| `mark8ly-storefront` | `public` | Mark8ly Storefront Web (User Agent, trampoline origin), Mark8ly Storefront Mobile (Native) |

No onboarding application: onboarding has signup, not login, and signup is a
server-side management-API call (see D7).

`auth-bff` additionally needs one **confidential** client. The
`zitadel-operator` issues public PKCE clients only — a confidential client is
explicitly out of its contract — so that one client is hand-created with a GCP
Secret Manager handoff, the pattern `zitadel-bootstrap`'s `platformProjects`
already uses for AgentGateway, AgentRegistry and Atlantis.

Accepted risks, inherited rather than created:

- The login-client PAT is **instance-level**. hms's runbook: it "can check
  anyone's password and mint a session for anyone... the most powerful
  credential this application holds." Zitadel offers no narrower role.
- `mark8ly-storefront` being `public` means any `TESSERIX` user can
  authenticate to it, including staff of other products. This matches today's
  open customer pool, so it is not a regression — but it is a decision.
- A single shared instance means one Zitadel outage signs out every Tesserix
  product at once.

### D3 — full login-client, because Session-API-only does not enforce the role check

This was the decisive experiment. With a `restricted` project and a user
holding **no role**:

| Path | Result |
|---|---|
| `POST /v2/sessions` (Session API only) | **201, session created.** Password genuinely verified (`password.verifiedAt` set; wrong password → 400 `COMMAND-3M0fs`). The project role check never runs. |
| `POST /v2/oidc/auth_requests/{id}` (finalize) | **403 `Errors.User.GrantRequired (OIDC-foSyH49RvL)`** |
| Same, after granting a role (positive control) | **200 + `callbackUrl?code=…`** |

A Session-API-only design would therefore produce a topology that looks
isolated and is not, with nothing in the API to reveal it. D2's guarantee only
exists if login goes through the OIDC authorization request.

Flow:

```
browser --> /oauth/v2/authorize                    [creates V2 auth request]
        <-- 302 to OUR login page ?authRequest=V2_…
browser --> existing login UI --POST creds--> auth-bff
            auth-bff: POST /v2/sessions            [Zitadel verifies password]
            auth-bff: decideSufficiency(...)       [fail-closed, ours]
            auth-bff: addTotpCheck if required     [Zitadel TOTP]
            auth-bff: POST /v2/oidc/auth_requests/{id}   <-- role check enforced HERE
        <-- callbackUrl?code=…
auth-bff: exchange code
          -> FGA CheckMembership (8 retries)  ] unchanged
          -> deviceguard                      ] unchanged
          -> email OTP                        ] unchanged
          -> mint m8_session                  ] unchanged
```

Everything after the code exchange is today's code. A user can complete Zitadel
authentication and still not receive an `m8_session` — precisely today's
semantics, where a valid GIP token must still pass our gates.

**Cost to acknowledge:** this adds a redirect round-trip to storefront customer
login, where there is none today. Unremarkable for merchant admin; a real
change for checkout conversion.

**Storefront origin.** VERIFIED: `loginVersion.loginV2.baseUri` is a single
origin and must carry no path. Per-tenant subdomains cannot each be a login
origin. This is the problem `apps/onboarding/app/auth/google` already solves for
Google, whose OAuth client has the same limitation — fixed registered origin at
`mark8ly.com`, short-lived HMAC exchange code, bounce back to the store's
`/auth/google/finish`. Storefront login reuses that shape. Custom admin domains
reuse `adminhandoff` for the same reason.

Note the boundary confusion worth writing down: the **Storefront Web Zitadel
application's redirect URI physically lives in the onboarding Next.js app**,
because that is what serves `mark8ly.com`. The Zitadel app boundary is not the
Next.js app boundary.

### D4 — preserve the subject

VERIFIED: `POST /v2/users/human` accepts a caller-supplied `userId` and stores
it **verbatim** — a 28-character alphanumeric GIP-shaped id was accepted and
read back `USER_STATE_ACTIVE`, despite Zitadel's own ids being numeric
snowflakes. `userId` cannot be changed after creation. VERIFIED: `sub == userId`
in a real minted token (`mark8ly-catalog-reader`: sub `388414281508455697`).

This matters because OpenFGA's subject is the raw GIP uid concatenated as
`"user:" + userID`, and the same uid is denormalised into
`tenants.owner_user_id`, `invitations.invited_by_user_id`,
`invitations.accepted_by_user_id`, `user_sessions.user_id`, `user_mfa.user_id`
(PK), live `m8_session` cookies, and the `X-User-Id` header contract between
`marketplace-api` and `auth-bff`.

The FGA **model** needs no changes at all: `infra/openfga/model.fga` declares a
bare `type user` with no attributes, and objects are `tenant:<our UUID>` /
`store:<our UUID>`. Nothing touches a GIP claim shape.

**Required post-condition of the import: assert the returned `userId` equals
the GIP uid.** If the import ever silently falls back to a generated id the
failure is not loud — `user_mfa` and `user_sessions` are keyed by that uid, so
the symptom is un-enrolled MFA and every device re-challenged, not an error.

VERIFIED caveat: caller-supplied ids are **human-users-only** on v4.15.3.
`POST /v2/users/machine` returns 405, and the management-v1 fallback has no
`userId` field.

### D5 — no user migration, atomic cutover

Password hashes cannot move: Zitadel's own Firebase guide states it has no
verifier for Firebase's proprietary modified scrypt, and `zitadel/passwap`
ships no firebase package (`argon2, bcrypt, drupal7, md5, md5plain, md5salted,
pbkdf2, phpass, scrypt, sha2`; its `scrypt` is standard scrypt with no salt
separator or signer key). A custom verifier would mean forking and
self-building Zitadel.

Just-in-time migration against GIP `accounts:signInWithPassword` is the
documented way to avoid resets, and was the plan until the row counts came in.
With four password accounts it is unjustifiable machinery. Recreate them by
hand.

That removes the only reason for dual-run, so hms's conclusion applies
unchanged: "Tasks 2-8 are ONE cutover and must merge together ... Two providers
at once is the worst state."

Federated users have no password and import clean, but the IDP link only works
if the OAuth client id matches between systems. Passkeys do not transfer unless
the domain is identical. TOTP is moot — nobody is enrolled.

### D6 — delete `usermfa`, keep `deviceguard` and `emailotp`

VERIFIED: `user_mfa` has 0 rows and 0 enabled; GIP MFA enrolment is 0 across
all pools. So this decision affects nobody.

Under the login-client model the session API expects TOTP via `addTotpCheck`.
Keeping our own TOTP would mean two TOTP systems against one session — worse
than either. Deleting ours also drops AES-GCM secret-at-rest handling and
server-side PNG QR generation we have no reason to own.

`deviceguard` and `emailotp` stay unchanged: Zitadel has no
new-device-email-OTP concept, neither calls GIP, and because D4 preserves the
subject, `user_sessions.user_id` stays valid — known devices stay known and
nobody is re-challenged at cutover.

**The sufficiency check is the load-bearing new code.** VERIFIED independently
by hms and tesserix-home against the live instance: under `forceMfa: true`,
Zitadel **still issues an authorization code for a password-only session**. It
does not refuse and does not signal a missing factor. MFA enforcement is
therefore entirely ours.

Required shape, copied from hms rather than reinvented: a pure
`decideSufficiency(policy, factors, checks)`, fail-closed on every uncertain
input, with `finalize` unexported and taking an unexported branded type so
omitting the check is a compile error, plus an archtest pinning the single call
site.

Four failure modes it must handle, all found in production by others:

- **protojson elides zero-value fields.** A healthy login policy never sends
  `forceMfa` at all when false. Decode into `map[string]any` with an anchor
  field, not a struct. (Same elision: `projectRoleCheck` is absent from the
  project JSON when false — VERIFIED on the HomeChef project — and
  `POST /v2/sessions` omits `factors` in its create response, so the session
  must be re-read with `GET` to see what was actually verified.)
- **`forceMfaLocalOnly`** forces MFA while `forceMfa` reads absent.
- **Read the policy scoped to the user's org** (`x-zitadel-orgid`), refusing an
  empty org id rather than falling back. hms #913 was a cross-org MFA bypass
  from exactly this.
- **The session token rotates on every TOTP check.** Returning the input token
  makes finalize fail *after* a correct code, which the user reads as "my code
  was wrong". The method is `PATCH`; `POST` returns 405.

### D7 — `gipadmin` replacement, and dropping the custom claim

| Today (GIP) | Zitadel |
|---|---|
| `sendOobCode` (PASSWORD_RESET, `returnOobLink`) | `POST /v2/users/{id}/password_reset`, returning the code so we send the mail |
| `accounts:resetPassword` | `POST /v2/users/{id}/password` with `verificationCode` |
| `accounts:delete` | `DELETE /v2/users/{id}` (VERIFIED) |
| `accounts:lookup` → `/auth/me/providers` | `GET /v2/users/{id}/authentication_methods` |
| `customAttributes` read/write | **dropped — see below** |

Returning the code rather than letting Zitadel send the mail preserves today's
behaviour (`returnOobLink=true`) and keeps branding and delivery on the
existing `notify` → platform-api path rather than the shared instance's SMTP.

**The `tenant_id` custom claim is dropped, not ported.** Zitadel has user
metadata, but metadata is not in the token; putting it there needs an Actions
v2 "complement token" script — a new runtime dependency on a shared instance.
hms faced the same choice and deliberately minted its own claim instead of
asking Zitadel for one. Its only consumer is `marketplace-api`'s mobile bearer
verifier, which will resolve tenancy from FGA exactly as `autologin` already
does. This deletes the read-modify-write dance that exists only because GIP has
no partial update for `customAttributes`.

**Signup** is a server-side `POST /v2/users/human` from `platform-api`. No
browser client, no app registration.

VERIFIED: creating a user via `/v2/users/human` with
`{"password":{"password":…}}` and then authenticating with that password works.
This sidesteps hms's management-v1 trap, where sending `password` instead of
`initialPassword` returns 201 with a `userId` and leaves no usable credential.
`email.isVerified` must be set deliberately; omitting it leaves the user
`USER_STATE_INITIAL` and unable to log in.

### D8 — delete `gipkey`

`gipkey` exists only because GIP browser API keys are HTTP-referrer restricted,
so a verified custom domain must be patched onto the key's `allowedReferrers`
or the storefront's `signInWithPassword` fails with
`API_KEY_HTTP_REFERRER_BLOCKED`. Zitadel has no such concept.

Deleting it also retires the `GIP_SERVER_API_KEY` / `GIP_WEB_API_KEY` split
from #499 and tesserix-k8s#780, and the latent hazard that `auth-bff`'s
`/auth/me/providers` uses the browser key for a server-side call.

## What is reusable

- `tesserix-home/platform-auth/` — standalone Go module, own `go.mod`, single
  dependency `go-oidc/v3`, JWKS verification with no introspection. Near
  drop-in: swap the capability vocabulary, set issuer and project id. Its
  `middleware.go` is `net/http` and needs re-shaping for Gin.
- `packages/platform-auth/src/zitadel.ts` — the TS twin, `jose` only.
- hms's `sufficiency.go` and its login-client HTTP client shape.
- hms's `docs/runbooks/` and tesserix-home's `RUNBOOK-ZITADEL-IDENTITY.md` —
  the documented traps are worth more than the code.

Neither repo uses a Zitadel SDK; both hand-roll against generic OIDC libraries
so they control the wire shape. We do the same. (The Go SDK does not expose
`UserId` — zitadel/zitadel#8146, closed as not planned — which would otherwise
block D4.)

## Infrastructure

Existing shared instance, no new infrastructure: `auth.tesserix.app`,
`zitadel:v4.15.3`, 3 replicas + HPA, CloudNativePG (not Cloud SQL) with mTLS,
behind a dedicated Istio gateway because Zitadel PATs are opaque rather than
JWTs and Envoy 401'd every authenticated call.

Two `ZitadelProject` / `ZitadelApplication` claim files under
`tesserix-k8s/k8s/operators/zitadel/claims/`, plus the hand-created confidential
client for `auth-bff` and its login-client machine user.

VERIFIED trap: `PUT .../oidc_config` is a **full replace, not a patch**. A
`loginVersion`-only PUT silently resets `authMethodType` and breaks token
exchange — the bug that broke every hms login for the life of a PR. Any write to
an app's OIDC config must send every field.

VERIFIED: per-app `loginVersion.loginV2.baseUri` works on this instance, so we
are not blocked by the `LOGINV2_REQUIRED` upstream bug (zitadel/zitadel#10722).
Without `loginVersion` set to V2, `/authorize` redirects to `/ui/login/login`
(V1) and the v2 API cannot finalize that request.

Chart changes need a `Chart.yaml` version bump or `ct lint` fails.

## Testing

Unit tests are explicitly not the mitigation — every one of the five defects
this issue cites had them. The mitigation is exercising the real flows:

- `decideSufficiency` — table-driven over absent/false/true `forceMfa`,
  `forceMfaLocalOnly`, unknown policy, unknown factors. Fail-closed asserted for
  every uncertain input.
- An archtest pinning the single `finalize` call site.
- An end-to-end test that a user with **no role** on `mark8ly-admin` is refused
  at finalize, and admitted once granted — the D2 guarantee, asserted rather
  than assumed.
- A cross-site cookie probe. hms ran app and IdP on different ports of
  `localhost`, which is same-site, so no test could catch a `SameSite` failure.
  Dev must use distinct registrable domains.
- An import post-condition test asserting the returned `userId` equals the GIP
  uid (D4).
- Manual walk of the real merchant login path before cutover, including new
  device → email OTP.

## Open questions

1. **Storefront redirect round-trip.** D3 adds one to customer login. Acceptable
   for conversion, or worth a storefront-specific mitigation?
2. **Mobile — likely resolved.** Of the three candidates, `apps/mobile-admin`
   (317 TS files, last commit 2026-08-15) and `apps/storefront-mobile` (80
   files, 2026-08-15) are live; `apps/mobile-storefront` (35 files, last commit
   2026-05-10) appears abandoned and superseded by `storefront-mobile`. So
   **two** Native registrations, not three. Confirm before writing the claim
   files — a wrong guess here registers an app nobody uses, or omits one people
   do.
3. **`mark8ly-storefront` is `public`**, so any `TESSERIX` user can
   authenticate to it. Matches today, but should be an explicit acceptance.

## Non-goals

Not an implementation plan and not an estimate. Zitadel Actions are not used.
No new Zitadel instance. `Platform-9bu14` and tesserix-home's identity model are
out of scope.
