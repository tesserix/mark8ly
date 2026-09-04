# Break-Glass Activation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `SessionIssuer` so the break-glass login and per-tenant SSO routes become reachable, and move break-glass credentials off the decommissioned GCP Secret Manager onto OpenBao.

**Architecture:** auth-bff gains one internal endpoint that mints a session cookie for an already-authenticated principal, on the existing `/internal` group (Bearer service key + rate limit). marketplace-api implements `SessionIssuer` as an HTTP client to it, swaps the break-glass `SecretClient` to an OpenBao adapter, and mounts the two route groups that were written but never registered. No downstream service changes — verified: marketplace-api authenticates on `X-User-Id`/`X-Tenant-Id`, and its Istio policy is mTLS workload-identity with no JWT requirement.

**Tech Stack:** Go 1.26, Gin, `openbao/openbao/api`, AES-256-GCM cookie sessions, bcrypt, TOTP (`pquerna/otp`).

**Spec:** [`docs/superpowers/specs/2026-09-04-break-glass-activation-design.md`](../specs/2026-09-04-break-glass-activation-design.md)

## Global Constraints

- **marketplace-api MUST NEVER sign or encrypt a session cookie.** Cookie cryptography stays in auth-bff. marketplace-api only passes a `Set-Cookie` string through.
- **`auth_context` is an explicit allow-list**: `staff`, `customer`, `break_glass`. Reject anything else with 400. It must never default to `staff` — a typo would silently mint a full staff session.
- **No secret value in any log line** — not the password, TOTP secret, session cookie, or service key. Log lengths and outcomes, never contents.
- **Break-glass login responses stay uniform.** `{"error":"invalid_credentials"}` regardless of which factor failed; forensics goes to the audit log.
- **bcrypt cost stays 12** (§12.4). **Secret-store write precedes the DB write** in `Bootstrapper`, so a store failure never leaves a `password_hash` referencing a non-existent blob.
- Repo conventions: single-line commit messages, no signatures, conventional-commit prefixes.

---

### Task 1: `CookieStore.Encode` in auth-bff

Extract the encrypt step from `Save` so a non-HTTP caller can produce a cookie value. `Save` must keep behaving identically.

**Files:**
- Modify: `auth-bff/internal/session/cookie.go:62-79`
- Test: `auth-bff/internal/session/cookie_test.go`

**Interfaces:**
- Produces: `func (cs *CookieStore) Encode(sess *Session) (string, error)` — sets `IssuedAt`, marshals, AES-GCM encrypts, returns the raw cookie value.

- [ ] **Step 1: Write the failing test**

```go
func TestCookieStore_Encode_RoundTrips(t *testing.T) {
	cs := NewCookieStore(testKeyHex, time.Hour, false)
	in := &Session{UserID: "u1", TenantID: "t1", AuthContext: "break_glass"}

	value, err := cs.Encode(in)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if value == "" {
		t.Fatal("Encode returned an empty cookie value")
	}

	out, err := cs.LoadFromValue(value)
	if err != nil {
		t.Fatalf("LoadFromValue: %v", err)
	}
	if out.UserID != "u1" || out.AuthContext != "break_glass" {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
	if in.IssuedAt == 0 {
		t.Fatal("Encode must stamp IssuedAt, as Save does")
	}
}
```

- [ ] **Step 2: Run it, confirm it fails**

Run: `cd auth-bff && go test ./internal/session/ -run TestCookieStore_Encode -v`
Expected: FAIL — `cs.Encode undefined`

- [ ] **Step 3: Implement, and make `Save` use it**

```go
// Encode stamps IssuedAt, marshals and encrypts the session, returning
// the raw cookie value. Save writes this into an HTTP cookie; internal
// endpoints that mint a session for another service use it directly.
func (cs *CookieStore) Encode(sess *Session) (string, error) {
	sess.IssuedAt = time.Now().Unix()

	data, err := json.Marshal(sess)
	if err != nil {
		return "", fmt.Errorf("session: marshal: %w", err)
	}
	encrypted, err := crypto.EncryptAESGCM(data, cs.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("session: encrypt: %w", err)
	}
	return encrypted, nil
}

func (cs *CookieStore) Save(c *gin.Context, cookieName, cookieDomain string, sess *Session) error {
	encrypted, err := cs.Encode(sess)
	if err != nil {
		return err
	}
	maxAgeSeconds := int(cs.maxAge.Seconds())
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(cookieName, encrypted, maxAgeSeconds, "/", cookieDomain, cs.secure, true)
	return nil
}
```

- [ ] **Step 4: Run the whole session package**

Run: `cd auth-bff && go test ./internal/session/ -v`
Expected: PASS, including the pre-existing `Save`/`Load` tests — this is a refactor, not a behaviour change.

- [ ] **Step 5: Commit**

```bash
git add internal/session/cookie.go internal/session/cookie_test.go
git commit -m "refactor(session): extract Encode from Save so internal callers can mint a cookie value"
```

---

### Task 2: `POST /internal/mint-session` in auth-bff

**Files:**
- Modify: `auth-bff/internal/handlers/internal.go` (register route + handler)
- Test: `auth-bff/internal/handlers/internal_mint_session_test.go`

**Interfaces:**
- Consumes: `CookieStore.Encode` from Task 1.
- Produces: `POST /internal/mint-session` — body `{tenant_id, tenant_slug, user_id, email, auth_context, app_name, cookie_name, ttl_seconds}`, response `{"set_cookie": "<header value>"}`.

**Note:** `cookie_name` is validated with the existing `cfg.IsKnownSessionCookie`, exactly as `SessionExchange` does — that guard already exists and prevents the endpoint being used to mint a cookie under an arbitrary name.

- [ ] **Step 1: Write the failing tests**

```go
func TestMintSession_RejectsUnknownAuthContext(t *testing.T) {
	h, r := newTestInternalHandler(t)
	_ = h
	body := `{"tenant_id":"t1","user_id":"u1","auth_context":"root","cookie_name":"mark8ly_session","ttl_seconds":7200}`
	w := postJSON(r, "/internal/mint-session", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for an unknown auth_context, got %d", w.Code)
	}
}

func TestMintSession_AcceptsBreakGlassAndSetsNoIdPTokens(t *testing.T) {
	h, r := newTestInternalHandler(t)
	body := `{"tenant_id":"t1","tenant_slug":"bondi","user_id":"u1","email":"bg@example.test","auth_context":"break_glass","app_name":"marketplace-admin","cookie_name":"mark8ly_session","ttl_seconds":7200}`
	w := postJSON(r, "/internal/mint-session", body)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		SetCookie string `json:"set_cookie"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if !strings.HasPrefix(resp.SetCookie, "mark8ly_session=") {
		t.Fatalf("set_cookie not a cookie header: %q", resp.SetCookie)
	}

	value := strings.TrimPrefix(strings.Split(resp.SetCookie, ";")[0], "mark8ly_session=")
	sess, err := h.sessions.LoadFromValue(value)
	if err != nil {
		t.Fatalf("minted cookie does not decrypt: %v", err)
	}
	if sess.AuthContext != "break_glass" {
		t.Fatalf("auth_context = %q, want break_glass", sess.AuthContext)
	}
	// The whole point: a break-glass principal has no IdP identity.
	if sess.AccessToken != "" || sess.IDToken != "" || sess.RefreshToken != "" {
		t.Fatalf("break-glass session must carry no IdP tokens: %+v", sess)
	}
	if sess.ExpiresAt <= time.Now().Unix() {
		t.Fatal("ExpiresAt must be in the future")
	}
}

func TestMintSession_RejectsUnknownCookieName(t *testing.T) {
	_, r := newTestInternalHandler(t)
	body := `{"tenant_id":"t1","user_id":"u1","auth_context":"break_glass","cookie_name":"evil_cookie","ttl_seconds":7200}`
	w := postJSON(r, "/internal/mint-session", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for an unknown cookie_name, got %d", w.Code)
	}
}
```

- [ ] **Step 2: Run, confirm failure**

Run: `cd auth-bff && go test ./internal/handlers/ -run TestMintSession -v`
Expected: FAIL — route returns 404.

- [ ] **Step 3: Implement**

Register alongside the existing internal routes:

```go
	internal.POST("/mint-session", h.MintSession)
```

```go
// allowedAuthContexts is an explicit allow-list. It must never fall
// back to "staff": a typo in a caller would otherwise silently mint a
// full staff session.
var allowedAuthContexts = map[string]bool{
	"staff":       true,
	"customer":    true,
	"break_glass": true,
}

// MintSession issues a Mark8ly session cookie for a principal that the
// CALLER has already authenticated. auth-bff performs no credential
// check here — the Bearer service key on this route group is what makes
// that safe.
//
// Used by marketplace-api's break-glass login and SSO callback, whose
// principals have no IdP identity: AccessToken/IDToken/RefreshToken are
// deliberately left empty, so nothing downstream can mistake this for a
// federated session.
func (h *InternalHandler) MintSession(c *gin.Context) {
	var req struct {
		TenantID    string `json:"tenant_id" binding:"required"`
		TenantSlug  string `json:"tenant_slug"`
		UserID      string `json:"user_id" binding:"required"`
		Email       string `json:"email"`
		AuthContext string `json:"auth_context" binding:"required"`
		AppName     string `json:"app_name"`
		CookieName  string `json:"cookie_name" binding:"required"`
		TTLSeconds  int    `json:"ttl_seconds" binding:"required,min=60,max=86400"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "INVALID_REQUEST"})
		return
	}
	if !allowedAuthContexts[req.AuthContext] {
		slog.Warn("mint-session: rejected auth_context", "auth_context", req.AuthContext)
		c.JSON(http.StatusBadRequest, gin.H{"error": "INVALID_AUTH_CONTEXT"})
		return
	}
	if !h.cfg.IsKnownSessionCookie(req.CookieName) {
		slog.Warn("mint-session: unknown cookie name", "cookie_name", req.CookieName)
		c.JSON(http.StatusBadRequest, gin.H{"error": "INVALID_COOKIE_NAME"})
		return
	}

	ttl := time.Duration(req.TTLSeconds) * time.Second
	csrf, err := crypto.RandomToken(32)
	if err != nil {
		slog.Error("mint-session: csrf generation failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "MINT_FAILED"})
		return
	}

	sess := &session.Session{
		UserID:      req.UserID,
		Email:       req.Email,
		TenantID:    req.TenantID,
		TenantSlug:  req.TenantSlug,
		AuthContext: req.AuthContext,
		AppName:     req.AppName,
		CSRFToken:   csrf,
		ExpiresAt:   time.Now().Add(ttl).Unix(),
		// AccessToken / IDToken / RefreshToken intentionally empty.
	}

	value, err := h.sessions.Encode(sess)
	if err != nil {
		slog.Error("mint-session: encode failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "MINT_FAILED"})
		return
	}

	ck := &http.Cookie{
		Name:     req.CookieName,
		Value:    value,
		Path:     "/",
		Domain:   h.cfg.SessionCookieDomain,
		MaxAge:   int(ttl.Seconds()),
		Secure:   h.cfg.SecureCookies(),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	slog.Info("mint-session: issued",
		"auth_context", req.AuthContext, "tenant_id", req.TenantID,
		"user_id", req.UserID, "ttl_seconds", req.TTLSeconds)
	c.JSON(http.StatusOK, gin.H{"set_cookie": ck.String()})
}
```

> If `crypto.RandomToken` / `cfg.SecureCookies()` / `cfg.SessionCookieDomain` differ in name, use the equivalents already used by `DirectAuthHandler` when it builds a session — match that call site rather than inventing helpers.

- [ ] **Step 4: Run**

Run: `cd auth-bff && go test ./internal/handlers/ -run TestMintSession -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/handlers/internal.go internal/handlers/internal_mint_session_test.go
git commit -m "feat(internal): mint a session cookie for an already-authenticated principal"
```

---

### Task 3: `HTTPIssuer` in marketplace-api

**Files:**
- Create: `services/marketplace-api/internal/authbffclient/http_issuer.go`
- Test: `services/marketplace-api/internal/authbffclient/http_issuer_test.go`

**Interfaces:**
- Consumes: `POST /internal/mint-session` from Task 2.
- Produces: `authbffclient.NewHTTPIssuer(baseURL, serviceKey, cookieName string, slug TenantSlugFunc) *HTTPIssuer`, satisfying `SessionIssuer`.

- [ ] **Step 1: Write the failing test**

```go
func TestHTTPIssuer_Issue_SendsBreakGlassContextAndReturnsCookie(t *testing.T) {
	var gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"set_cookie":"mark8ly_session=abc; Path=/; HttpOnly"}`))
	}))
	defer srv.Close()

	iss := NewHTTPIssuer(srv.URL, "svc-key", "mark8ly_session",
		func(context.Context, uuid.UUID) (string, error) { return "bondi", nil })

	tid, uid := uuid.New(), uuid.New()
	cookie, err := iss.Issue(context.Background(), tid, uid, 2*time.Hour)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if cookie != "mark8ly_session=abc; Path=/; HttpOnly" {
		t.Fatalf("cookie = %q", cookie)
	}
	if gotAuth != "Bearer svc-key" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if !strings.Contains(gotBody, `"auth_context":"break_glass"`) {
		t.Fatalf("body missing break_glass context: %s", gotBody)
	}
	if !strings.Contains(gotBody, `"ttl_seconds":7200`) {
		t.Fatalf("body missing ttl: %s", gotBody)
	}
}

func TestHTTPIssuer_Issue_NonOKIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	iss := NewHTTPIssuer(srv.URL, "wrong", "mark8ly_session",
		func(context.Context, uuid.UUID) (string, error) { return "bondi", nil })

	if _, err := iss.Issue(context.Background(), uuid.New(), uuid.New(), time.Hour); err == nil {
		t.Fatal("want an error on 401, got nil")
	}
}

func TestHTTPIssuer_SatisfiesSessionIssuer(t *testing.T) {
	var _ SessionIssuer = (*HTTPIssuer)(nil)
}
```

- [ ] **Step 2: Run, confirm failure**

Run: `cd services/marketplace-api && go test ./internal/authbffclient/ -run TestHTTPIssuer -v`
Expected: FAIL — `NewHTTPIssuer` undefined.

- [ ] **Step 3: Implement**

```go
package authbffclient

// TenantSlugFunc resolves a tenant's slug. The minted session carries it
// so the admin app can render tenant context without a second lookup.
type TenantSlugFunc func(context.Context, uuid.UUID) (string, error)

// HTTPIssuer mints sessions by calling auth-bff's internal endpoint.
// It authenticates with the same Bearer service key the rest of the
// /internal group uses.
type HTTPIssuer struct {
	baseURL    string
	serviceKey string
	cookieName string
	slug       TenantSlugFunc
	client     *http.Client
}

func NewHTTPIssuer(baseURL, serviceKey, cookieName string, slug TenantSlugFunc) *HTTPIssuer {
	return &HTTPIssuer{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		serviceKey: serviceKey,
		cookieName: cookieName,
		slug:       slug,
		client:     &http.Client{Timeout: 5 * time.Second},
	}
}

func (h *HTTPIssuer) Issue(ctx context.Context, tenantID, userID uuid.UUID, ttl time.Duration) (string, error) {
	slug := ""
	if h.slug != nil {
		if s, err := h.slug(ctx, tenantID); err == nil {
			slug = s
		}
	}

	payload := map[string]any{
		"tenant_id":    tenantID.String(),
		"tenant_slug":  slug,
		"user_id":      userID.String(),
		"auth_context": "break_glass",
		"app_name":     "marketplace-admin",
		"cookie_name":  h.cookieName,
		"ttl_seconds":  int(ttl.Seconds()),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("authbffclient: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.baseURL+"/internal/mint-session", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("authbffclient: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+h.serviceKey)

	resp, err := h.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("authbffclient: mint-session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Never echo the response body — it is auth-bff's and may name
		// internals. The status is enough to act on.
		return "", fmt.Errorf("authbffclient: mint-session: status %d", resp.StatusCode)
	}

	var out struct {
		SetCookie string `json:"set_cookie"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("authbffclient: decode: %w", err)
	}
	if out.SetCookie == "" {
		return "", errors.New("authbffclient: mint-session returned an empty cookie")
	}
	return out.SetCookie, nil
}
```

- [ ] **Step 4: Run**

Run: `cd services/marketplace-api && go test ./internal/authbffclient/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/authbffclient/http_issuer.go internal/authbffclient/http_issuer_test.go
git commit -m "feat(authbffclient): implement SessionIssuer against auth-bff mint-session"
```

---

### Task 4: OpenBao adapter for break-glass secrets

`breakglass.SecretClient` needs two methods; `carriersecrets.BaoClient` already provides both under different names.

**Files:**
- Create: `services/marketplace-api/internal/breakglass/bao_secret_client.go`
- Test: `services/marketplace-api/internal/breakglass/bao_secret_client_test.go`

**Interfaces:**
- Consumes: `carriersecrets.BaoClient.CreateOrAddVersion(ctx, name, payload)` and `.AccessLatest(ctx, name)`.
- Produces: `breakglass.NewBaoSecretClient(*carriersecrets.BaoClient) SecretClient`, and `BreakGlassSecretName(tenantID uuid.UUID) string` returning `break-glass/<tenant_id>`.

- [ ] **Step 1: Write the failing test**

```go
type fakeBao struct {
	saved map[string][]byte
	err   error
}

func (f *fakeBao) CreateOrAddVersion(_ context.Context, name string, payload []byte) error {
	if f.err != nil {
		return f.err
	}
	if f.saved == nil {
		f.saved = map[string][]byte{}
	}
	f.saved[name] = payload
	return nil
}
func (f *fakeBao) AccessLatest(_ context.Context, name string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	b, ok := f.saved[name]
	if !ok {
		return nil, fmt.Errorf("not found: %s", name)
	}
	return b, nil
}

func TestBaoSecretClient_RoundTripsABlob(t *testing.T) {
	f := &fakeBao{}
	c := NewBaoSecretClientFrom(f)
	sm := NewSecretManager(c)

	tenant := uuid.New()
	path := BreakGlassSecretName(tenant)
	if !strings.HasPrefix(path, "break-glass/") {
		t.Fatalf("path = %q, want break-glass/<tenant>", path)
	}

	in := Blob{Password: "pw", TOTPSecret: "totp", GeneratedAt: time.Now().UTC()}
	if err := sm.Write(context.Background(), path, in); err != nil {
		t.Fatalf("Write: %v", err)
	}
	out, err := sm.Read(context.Background(), path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if out.Password != "pw" || out.TOTPSecret != "totp" {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
}

func TestBaoSecretClient_PropagatesWriteFailure(t *testing.T) {
	c := NewBaoSecretClientFrom(&fakeBao{err: errors.New("bao down")})
	if err := NewSecretManager(c).Write(context.Background(), "break-glass/x", Blob{}); err == nil {
		t.Fatal("a store failure must propagate — Bootstrapper relies on it to skip the DB write")
	}
}
```

> Match `SecretManager`'s real method names when writing this test — read `secret_manager.go` first and use whatever it calls read/write.

- [ ] **Step 2: Run, confirm failure**

Run: `cd services/marketplace-api && go test ./internal/breakglass/ -run TestBaoSecretClient -v`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement**

```go
package breakglass

// baoWriter is the slice of carriersecrets.BaoClient this package needs.
// Declared as an interface so tests need no live OpenBao.
type baoWriter interface {
	CreateOrAddVersion(ctx context.Context, name string, payload []byte) error
	AccessLatest(ctx context.Context, name string) ([]byte, error)
}

// BaoSecretClient adapts an OpenBao client to SecretClient.
//
// Break-glass credentials used to live in GCP Secret Manager. That
// backend was decommissioned (milestone 10: secrets deleted, IAM
// revoked), so this is now the only store — not an alternative to one.
type BaoSecretClient struct{ bao baoWriter }

func NewBaoSecretClientFrom(b baoWriter) *BaoSecretClient { return &BaoSecretClient{bao: b} }

func (c *BaoSecretClient) AddVersion(ctx context.Context, path string, payload []byte) error {
	return c.bao.CreateOrAddVersion(ctx, path, payload)
}

func (c *BaoSecretClient) AccessLatest(ctx context.Context, path string) ([]byte, error) {
	return c.bao.AccessLatest(ctx, path)
}

// BreakGlassSecretName is the OpenBao path for a tenant's break-glass
// blob, mirroring the carrier-secrets convention.
func BreakGlassSecretName(tenantID uuid.UUID) string {
	return "break-glass/" + tenantID.String()
}
```

- [ ] **Step 4: Run**

Run: `cd services/marketplace-api && go test ./internal/breakglass/ -v`
Expected: PASS, existing break-glass tests included.

- [ ] **Step 5: Commit**

```bash
git add internal/breakglass/bao_secret_client.go internal/breakglass/bao_secret_client_test.go
git commit -m "feat(breakglass): store credentials in OpenBao instead of the retired GCP Secret Manager"
```

---

### Task 5: Delete the GCP Secret Manager client

**Files:**
- Delete: `services/marketplace-api/internal/breakglass/gcp_secret_manager.go`
- Delete: `services/marketplace-api/internal/breakglass/secret_manager_test.go` cases that construct `GCPSecretClient` (keep the `FakeSecretClient` ones)
- Modify: `services/marketplace-api/go.mod` if `cloud.google.com/go/secretmanager` becomes unused

- [ ] **Step 1: Confirm nothing else imports it**

Run: `cd services/marketplace-api && grep -rn "GCPSecretClient\|secretmanager" --include=*.go . | grep -v _test.go`
Expected: only `gcp_secret_manager.go` itself. **If anything else appears, stop and report** — another caller means this is not a clean delete.

- [ ] **Step 2: Delete and tidy**

```bash
git rm internal/breakglass/gcp_secret_manager.go
go mod tidy
```

- [ ] **Step 3: Build and test**

Run: `cd services/marketplace-api && go build ./... && go test ./internal/breakglass/ -v`
Expected: builds clean, tests pass.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "chore(breakglass): delete the GCP Secret Manager client for a decommissioned backend"
```

---

### Task 6: Wire and mount in `main.go`

**Files:**
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go`
- Modify: `services/marketplace-api/internal/handlers/admin/routes.go` (if the login route registers there)

- [ ] **Step 1: Build the issuer and the Bao-backed secret manager**

Construct `HTTPIssuer` from config, falling back to `NoopIssuer` when unset so a misconfigured deploy fails loudly at first use rather than serving an unauthenticated route:

```go
	var bgIssuer authbffclient.SessionIssuer = authbffclient.NoopIssuer{}
	if cfg.AuthBFFInternalURL != "" && cfg.AuthBFFInternalServiceKey != "" {
		bgIssuer = authbffclient.NewHTTPIssuer(
			cfg.AuthBFFInternalURL,
			cfg.AuthBFFInternalServiceKey,
			cfg.SessionCookieName,
			tenantSlugLookup,
		)
	} else {
		log.Warn("break-glass: no auth-bff internal config; login will return 500 until set")
	}
```

- [ ] **Step 2: Mount the login route OUTSIDE the store-scoped group**

The handler's own comment is explicit: it must survive `read_only` / `store_closed` subscription states (§12.4). Gate it with `plangate.RequireFeature(FeatureSSO)` — break-glass exists for SSO tenants.

```go
	bgHandler := admin.NewBreakGlassLoginHandler(admin.BreakGlassDeps{
		Repo: bgRepo, Secrets: bgSecrets, Audit: bgAudit, Slack: bgSlack,
		RateLimiter: bgLimiter, IPHMACKey: bgHMACKey,
		Sessions: bgIssuer, Logger: appLogger,
	})
	adminRoot.POST("/break-glass/login",
		plangate.RequireFeature(plangate.FeatureSSO),
		bgHandler.Login)
```

- [ ] **Step 3: Mount the SSO routes**

Same issuer. Register `SSOLoginHandler`'s `Login`, `Callback`, `Logout` on the public group.

- [ ] **Step 4: Prove the routes exist**

Add a route-registration test — an unmounted route is this codebase's recurring silent failure, so assert presence directly:

```go
func TestBreakGlassLoginRouteIsMounted(t *testing.T) {
	r := buildTestRouter(t)
	found := false
	for _, ri := range r.Routes() {
		if ri.Method == http.MethodPost && strings.HasSuffix(ri.Path, "/break-glass/login") {
			found = true
		}
	}
	if !found {
		t.Fatal("POST /break-glass/login is not registered — the handler exists but is unreachable")
	}
}
```

- [ ] **Step 5: Build, test, commit**

```bash
cd services/marketplace-api && go build ./... && go test ./... 
git add -A
git commit -m "feat(marketplace-api): mount the break-glass login and SSO routes"
```

---

### Task 7: Config and deployment

**Ordering matters and is the opposite of intuition** for the two directions:

- `AUTH_BFF_INTERNAL_URL` / `AUTH_BFF_INTERNAL_SERVICE_KEY` are **new required-ish config**, so the **cluster change goes first**: add the env vars and the ExternalSecret, then ship the code that reads them. Shipping code first would crashloop on validate.
- The OpenBao grant needs a `break-glass/*` path policy for the marketplace-api roles. CronJobs inherit the deployment ServiceAccount, so the existing grant machinery applies — what a new path needs is the policy line, not new IAM.

- [ ] **Step 1: tesserix-k8s — add the env vars + secret, chart version bump, merge, confirm synced**
- [ ] **Step 2: Extend the OpenBao policy to `kv/data/mark8ly/break-glass/*`**
- [ ] **Step 3: Only then merge the marketplace-api change**

---

### Task 8: Provision accounts and verify end to end

Zero break-glass accounts exist today, so the feature is not real until this runs.

- [ ] **Step 1: Run `Bootstrapper` for each Pro+SSO tenant** — verify the OpenBao blob exists and the DB row references it, and that a store failure leaves **no** DB row.
- [ ] **Step 2: Confirm the credentials are delivered to their owner out-of-band.** Never paste a password or TOTP secret into a PR, issue, log, or chat.
- [ ] **Step 3: End-to-end** — a correct password+TOTP returns 200 with `Set-Cookie`; the cookie is accepted by marketplace-api-admin (this is the §4 claim, proven against the live Istio policy rather than the manifest).
- [ ] **Step 4: Negative** — wrong factor returns the uniform `invalid_credentials`; the lockout path still trips; the Slack alert and `severity=critical` audit event both fire.
- [ ] **Step 5: Audit `session-exchange` callers** for any that assume a non-empty `access_token`, since break-glass sessions return empty strings there.
