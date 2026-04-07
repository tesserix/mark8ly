# Auth & Authz — Diagnosis of Old Code

> **Phase B output.** Read before porting auth-bff or the onboarding completion handler.
> Findings are file:line referenced from `mark8ly_backup/`.

## Confirmed bugs

### Bug #1: Negative cache of "tenant not found" blocks new tenants for 5+ minutes

**File:line** `apps/admin/middleware.ts:54, 320–322`

**What's wrong**
When a tenant doesn't exist, the middleware caches the negative result with a 5-minute TTL:
```typescript
// Line 54
const CACHE_TTL = 5 * 60 * 1000; // 5 minutes
// Line 320-322
if (response.status === 404) {
  validatedTenants.set(slug, { exists: false, timestamp: now, validatedAt: now });
  return { exists: false };
}
```

On first request to a new tenant subdomain (during/after onboarding), if the tenant hasn't been committed to the database yet, the middleware gets a 404. It caches `exists: false` for 5 minutes. Even after the tenant is created, the middleware still serves the cached negative result until the TTL expires.

**Why it produces the symptom**
- User completes onboarding on `onboarding.mark8ly.com`, then clicks "Go to Admin"
- Redirect goes to `{tenant}-admin.mark8ly.com`
- Admin middleware checks tenant existence and gets 404 (either onboarding hasn't committed tenant yet, or there's a transient lookup delay)
- Negative result is cached for 5 minutes
- User sees "Tenant not found" until cache TTL expires, even though tenant exists
- Meanwhile, user is logged in and has a valid session, making the error appear inconsistent

**Fix in the new code**
- Do NOT cache negative tenant lookups (existence checks)
- Only cache positive lookups (we know the tenant exists)
- If a lookup fails, return "fail open" (allow the request through if there's stale positive data) but don't create new negative cache entries

---

### Bug #2: Race condition between onboarding DB commit and OpenFGA tuple write

**File:line** `services/tenant-service/internal/services/onboarding_completion.go:477–494`

**What's wrong**
Membership tuple is written AFTER the DB transaction commits:
```go
// Line 477-479: DB transaction commits
if err := userTx.Commit().Error; err != nil {
  return nil, fmt.Errorf("failed to commit transaction: %w", err)
}

// Line 482-493: OpenFGA tuple write happens AFTER
if s.membershipSvc == nil {
  slog.Error("membership service not configured", ...)
  return nil, fmt.Errorf("membership service not configured...")
}
if _, membershipErr := s.membershipSvc.CreateOwnerMembership(ctx, tenantID, userID); membershipErr != nil {
  slog.Error("failed to create owner membership", ...)
  return nil, fmt.Errorf("failed to create user access to tenant: %w...")
}
```

**Why it produces the symptom**
1. Onboarding completes, user is redirected to admin
2. Admin middleware validates tenant exists — PASSES (tenant was committed to DB)
3. User is routed to admin app and attempts an action that requires FGA Check (e.g., view dashboard)
4. Meanwhile, onboarding handler is still writing the OpenFGA tuple in the background
5. FGA Check fails because the tuple doesn't exist yet
6. User sees "permission denied" or 403 even though they're the owner
7. Refreshing page a few seconds later works (tuple has been written)

**Fix in the new code**
- Write the FGA tuple INSIDE the transaction (or in a nested transaction that commits before returning)
- If FGA write fails, roll back the entire tenant+user creation
- Use an outbox table with a background processor if tuple write must be asynchronous for performance

---

### Bug #3: No retry/recovery mechanism if OpenFGA tuple write fails after DB commit

**File:line** `services/tenant-service/internal/services/onboarding_completion.go:486–493`

**What's wrong**
If the OpenFGA tuple write fails (lines 486–493), the code logs an error and returns an error response. But the tenant and user records have already been committed to the database. The tuple write is lost — there is no outbox table, retry queue, or mechanism to retry the write.

```go
if _, membershipErr := s.membershipSvc.CreateOwnerMembership(ctx, tenantID, userID); membershipErr != nil {
  slog.Error("failed to create owner membership",
    "user_id", userID, "tenant_id", tenantID, "error", membershipErr)
  // ... sets tenant status to inactive, but tuple is never retried
  return nil, fmt.Errorf("failed to create user access to tenant: %w — please contact support", membershipErr)
}
```

**Why it produces the symptom**
- If `membershipSvc.CreateOwnerMembership` times out or the FGA service is temporarily unavailable
- The tenant and user are created in the DB (no rollback)
- The tuple write is lost
- User can log in but has no permissions because the tuple was never created
- Admin UI appears broken; all FGA Checks return "no permission"

**Fix in the new code**
- Use an outbox/inbox pattern: write the tuple intent to the DB as part of the same transaction
- Background processor retries failed tuples indefinitely
- Monitor outbox backlog and alert on stalled entries

---

### Bug #4: Cookie SameSite=Lax may block OIDC callback on some browsers/proxies

**File:line** `internal/session/cookie.go:76, 134`

**What's wrong**
All session cookies are set with `SameSite=Lax`:
```go
// Line 76
c.SetSameSite(http.SameSiteLaxMode)
c.SetCookie(cookieName, encrypted, maxAgeSeconds, "/", cookieDomain, cs.secure, true)
```

During the OIDC authorization code flow, the GIP authorization server redirects back to `{host}/auth/callback` with the `code` and `state` parameters. The callback handler immediately tries to set a session cookie. With `SameSite=Lax`, if the redirect is from a different site (even from the same auth provider), the browser may not send or set the cookie. Some middleware proxies (Cloudflare, Authorizer) can also strip cookies on cross-origin redirects.

**Why it produces the symptom**
- User clicks login, gets redirected to Google OAuth, signs in
- Google redirects back to auth-bff callback with `code` and `state`
- Callback sets the session cookie with `SameSite=Lax`
- On some browser/proxy combinations, the cookie is not set
- Session is null, user sees "not authenticated" immediately after login
- Refreshing the page doesn't help (no cookie was ever persisted)
- Manual login via email/password works fine (no OIDC redirect)

**Fix in the new code**
- For the OIDC callback response, use `SameSite=None` temporarily (this is the OAuth spec-compliant behavior)
- Require `Secure=true` when using `SameSite=None`
- For all other endpoints, keep `SameSite=Lax`

---

### Bug #5: GIP issuer URL construction lacks per-tenant isolation

**File:line** `internal/gip/client.go:89–90`

**What's wrong**
The GIP OIDC issuer URL is hardcoded to the project level:
```go
// Line 89-90
gipProv, err := gooidc.NewProvider(ctx, fmt.Sprintf("https://securetoken.google.com/%s", cfg.GCPProjectID))
```

All GIP tenants within a project share the same issuer URL (`https://securetoken.google.com/{projectID}`). The verifier is created per OAuth client ID:
```go
// Line 104-106
verifier: gipProv.Verifier(&gooidc.Config{
  ClientID: app.OAuthClientID,
}),
```

However, GIP issuer URLs include the tenant ID in the `iss` claim, but the verifier's issuer validation is only checking that it matches `https://securetoken.google.com/{projectID}`. The tenant ID in the token claims is NOT validated against the expected tenant.

**Why it produces the symptom**
- An attacker or misconfiguration could send a valid ID token from GIP tenant A to an app expecting tenant B
- As long as the OAuth client ID matches, the token is accepted
- User from `MP-Customer-zoe11` (storefront) logs in with a token minted in that tenant
- Callback handler extracts `tenantID` from the token claims and trusts it
- Session is created with the wrong tenant context
- User can now access another tenant's admin dashboard if the slug matches

This is partially mitigated by the OAuth client ID being separate per app, but the defense is weak.

**Fix in the new code**
- Store the expected GIP tenant ID with each app config
- After token verification, explicitly check that `token.Firebase.Tenant == app.GIPTenantID`
- Reject tokens from mismatched tenants even if the OAuth client ID is correct

---

### Bug #6: Middleware ordering — CSRFProtection applied AFTER SessionExtractor

**File:line** `cmd/auth-bff/main.go:113–116`

**What's wrong**
CSRF middleware is applied after the session is extracted:
```go
// Line 105: Session extracted globally
router.Use(middleware.SessionExtractor(cookieStore))

// Line 113-114: Auth group with CSRF protection
authGroup := router.Group("")
authGroup.Use(middleware.CSRFProtection())
```

This means CSRF tokens are validated on auth routes but the session cookie may not be bound to the CSRF token in all paths. Specifically, on GET requests that don't have a CSRF token in the query string, the token validation might pass trivially while allowing some state-changing operations via GET parameters.

**Why it produces the symptom**
- CSRF tokens are required for POST but not always for GET
- An attacker crafts a malicious link like `/auth/logout?force=true` (hypothetical)
- The user clicks the link from another site
- CSRF check doesn't trigger for GET requests
- User is logged out without their consent

This is low-severity because the app likely doesn't expose state-changing operations on GET, but it's a defense-in-depth issue.

**Fix in the new code**
- Ensure CSRFProtection middleware is applied globally, not just to auth routes
- Make CSRF token binding explicit in the session cookie (include csrf token hash)
- For all state-changing operations, require CSRF token in the body, not as a query param

---

## Likely bugs (suspicious code, not 100% confirmed)

### Suspicious #1: Cookie domain fallback might expose cross-subdomain access

**File:line** `internal/middleware/session.go:155–195`

The `GetCookieDomain` function tries multiple domain patterns:
```go
// Line 163-165: Product domain
if app.ProductDomain != "" {
  d := app.ProductDomain
  if strings.HasSuffix(host, "."+d) || host == d {
    return "." + d
  }
}
// Line 169-172: Platform domain
if platformDomain != "" {
  if strings.HasSuffix(host, "."+platformDomain) || host == platformDomain {
    return "." + platformDomain
  }
}
```

If the product domain is not set in config, it falls back to the platform domain. This could allow cookies for one product to be sent to another product's subdomain if they're on the same platform domain.

**Confidence: MEDIUM** — This is only a problem if config is incomplete, but it's worth auditing.

**Fix in new code**
- Require explicit product domain in config; don't fall back
- Validate that product domain is an exact match with the host suffix

---

### Suspicious #2: No rate limiting on token refresh endpoint

**File:line** `internal/handlers/auth.go:322–371`

The `/auth/refresh` endpoint validates the session exists but has no per-user rate limiting. An attacker could rapidly refresh tokens to hammer the GIP API.

```go
func (h *AuthHandler) Refresh(c *gin.Context) {
  app := middleware.GetApp(c)
  sess := middleware.GetSession(c)
  tokens, err := h.gip.Refresh(c.Request.Context(), app, sess.RefreshToken)
  // ... no rate limit per user/session
}
```

**Confidence: LOW** — The server-level rate limiter may catch it, but per-session limits would be better.

---

## Ruled out

### NOT a bug: Cookie size under 4KB
Session cookie payload is reasonable:
- `uid`, `email`, `tid`, `ts`, `at`, `idt`, `rt`, `exp`, `csrf`, `app` — all small strings
- Encrypted, base64-encoded is ~2–3KB for typical payloads
- Well below the 4KB HTTP cookie limit

### NOT a bug: Secure flag in local dev
The code correctly respects the environment:
```go
// config/config.go:89
CookieSecure: getEnv("ENVIRONMENT", "development") == "production",
// session/cookie.go:77
c.SetCookie(cookieName, encrypted, maxAgeSeconds, "/", cookieDomain, cs.secure, true)
```
`Secure` is only set in production. Development over HTTP works.

### NOT a bug: Stale-while-revalidate semantics in admin middleware
The middleware returns stale positive data (we know tenant exists) while fetching fresh data in background. This is safe — worst case the tenant existed 5 minutes ago and still does now.

---

## Open questions

1. **Multi-store OpenFGA setup**: Are there separate FGA stores for platform and marketplace? How does `CreateOwnerMembership` know which store to write to? (Code doesn't show the resolver.)

2. **GIP custom claims propagation latency**: How long does GIP take to propagate custom claims (staff_id, tenant_id, vendor_id) after `UpdateCustomUserClaims`? The code retries 3 times with 1s delays (lines 725–743), but if claims take >3 seconds to propagate, the token will still lack RBAC claims.

3. **Onboarding session visibility race**: If onboarding handler crashes between DB commit and FGA tuple write, is there cleanup logic that marks the tenant as failed? (Code sets status to "inactive" on membership creation failure, but not for other crashes.)

4. **Redis fail-open for admin cookie domain cache**: The admin middleware caches tenant validation but doesn't show a Redis client. Is the cache in-memory only, or backed by Redis? If Redis, what happens if Redis is down?

---

## Fix priorities for the rewrite

**Priority 1: Critical (causes data loss / auth bypass)**
1. Move OpenFGA tuple write INSIDE the DB transaction, or use an outbox pattern with retries
2. Do NOT cache negative tenant lookups; only cache positives

**Priority 2: High (causes user-blocking bugs)**
3. Add per-tenant GIP tenant ID validation after token verification
4. Fix OIDC cookie blocking by using `SameSite=None; Secure` for callback response
5. Ensure CSRF middleware applies globally and validates all mutations

**Priority 3: Medium (data consistency)**
6. Require explicit product domain config; don't fall back to platform domain
7. Add per-session rate limits on token refresh
8. Document and audit multi-store OpenFGA setup

**Priority 4: Low (observability)**
9. Add metrics/alerts for OpenFGA tuple write latency
10. Monitor GIP custom claims propagation lag

---

## Testing recommendations for the rewrite

- **Test 1**: Complete onboarding, immediately navigate to admin before middleware cache expires. Verify no "tenant not found" even if the admin middleware check returns 404 initially.
- **Test 2**: Simulate FGA tuple write failure after DB commit. Verify tenant is marked as failed and cleanup is triggered.
- **Test 3**: Send OIDC callback from an incognito browser (no existing cookies). Verify session cookie is set and persisted.
- **Test 4**: Send a valid token from the wrong GIP tenant pool. Verify it's rejected.
- **Test 5**: Load test token refresh endpoint with one session. Verify per-session rate limit is enforced.
