# P13 — Per-Tenant SSO (SAML 2.0 + OIDC via GIP) + Break-Glass Admin with Mandatory TOTP

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver two tightly-coupled capabilities for Pro-tier tenants:

1. **Per-tenant SSO** — merchants on the `Pro` plan attach their own SAML 2.0 IdP or OIDC provider to Mark8ly via `tenant_sso_configs`. Admins sign in through their corporate IdP; GIP receives the federated assertion; `auth-bff` issues the standard Mark8ly session cookie. Feature gated behind `plangate.RequireFeature(FeatureSSO)` from P3 so Starter/Studio tenants get **403** on any SSO endpoint.
2. **Break-glass admin** — each Pro+SSO tenant gets exactly one emergency local account whose password (CSPRNG 20 chars) and TOTP secret (32 bytes) live in GCP Secret Manager at `/projects/tesserix-prod/secrets/break-glass-{tenant_id}`. Login requires **both** password AND TOTP (never SMS). Every login triggers an immediate 24-hour rotation, a Slack alert to `#security-alerts`, and an audit event with `severity=critical, actor=break-glass`. Passwords also rotate on a 90-day cron.

**Architecture:** New package `internal/sso` owns SAML + OIDC flows; the existing `coreos/go-oidc/v3` client from `auth-bff` is reused for OIDC, and `github.com/crewjam/saml` is introduced for SAML 2.0 (battle-tested SP / ACS implementation). GIP's Admin SDK (via `firebase.google.com/go/v4`) uploads each tenant's SAML metadata / OIDC client config into GIP per-product tenant pools — we do **not** invent a second token-verification layer. Session minting stays inside `auth-bff/internal/session/cookie.go` (the same encrypted cookie store used by every other login path); SSO simply takes a different route to reach it. A separate package `internal/breakglass` owns password + TOTP generation, Secret Manager I/O, the rate-limited login handler, the 24-hour post-use rotation job, and the 90-day cron. Audit events go through `internal/audit` from P1. Both packages write through small, well-typed repositories so integration tests drive real SQL while GIP + Secret Manager are faked via interfaces.

**Tech Stack:** Go 1.26, Gin, GORM, PostgreSQL, `github.com/crewjam/saml` v0.4.x, `github.com/coreos/go-oidc/v3` (already pulled in via auth-bff), `github.com/pquerna/otp` v1.4.x (TOTP RFC 6238), `firebase.google.com/go/v4` (already a direct dep), `cloud.google.com/go/secretmanager` (already a direct dep), `golang.org/x/crypto/bcrypt`, standard `crypto/rand` for CSPRNG.

**Spec:** [`docs/superpowers/specs/2026-04-17-subscription-model-design.md`](../specs/2026-04-17-subscription-model-design.md) — §9 feature matrix row "SSO — Pro only", §12 SSO overview, §12.4 Break-glass admin.

**Depends on:**
- **P1** — data model machinery (migrations, `internal/audit`). This plan adds orthogonal tables under the same `golang-migrate` setup.
- **P3** — `plangate.RequireFeature(FeatureSSO)` is the exact gate used here. If P3 hasn't landed, every admin SSO endpoint returns 403 (intentional).
- **P8** (soft) — supplies `HMACIPHash` helper used in break-glass audit. Local fallback documented in Task 13 if P8 lands later.

**Related plans:**
- **P16** (admin frontend) consumes these APIs + renders the TOTP QR code.
- **P17** (observability) dashboards on `sso.login.*` and `break_glass.*` audit events.
- **tesserix-infra terraform** (ops, out of this plan) creates `break-glass-responders` Google Group + IAM policy on `/projects/tesserix-prod/secrets/break-glass-*`.

---

## Scope Check

**In scope — Part A (SSO):**
1. Migrations: `tenant_sso_configs` (per-tenant IdP config) + `tenant_sso_user_mappings` (JIT + audit).
2. `internal/sso` package — SAML SP via `crewjam/saml`, OIDC RP via `coreos/go-oidc/v3`.
3. GIP metadata upload helper via `firebase.google.com/go/v4` Admin SDK.
4. Admin endpoints (Pro-gated): `POST|GET|DELETE /admin/tenants/:tenantId/sso/config` + `POST /admin/tenants/:tenantId/sso/test`.
5. Login flow: `GET /sso/:tenantSlug/login` → IdP → `POST /sso/:tenantSlug/callback` → GIP exchange → Mark8ly session cookie.
6. JIT provisioning + minimal JSON-path attribute mapping DSL.
7. `POST /sso/:tenantSlug/logout` with optional IdP SLO.

**In scope — Part B (Break-glass):**
8. Migration: `break_glass_accounts` + `break_glass_lockouts`.
9. `internal/breakglass` package — CSPRNG password, TOTP, Secret Manager wrapper, bcrypt hashing.
10. Bootstrap job — one account per Pro+SSO tenant on signup.
11. `POST /admin/break-glass/login` — rate-limited (3/hour/IP, 24h lockout), dual-factor, 2-hour session, post-use 24h rotation, Slack + critical audit.
12. Rotation: post-use hook + 90-day cron (daily 04:00 UTC).

**Out of scope:**
- Admin UI for SSO config + break-glass screens (P16).
- TOTP QR-code image rendering (backend returns `otpauth://` URI; P16 renders).
- GIP tenant-pool provisioning Terraform (ops task).
- `break-glass-responders` Google Group + IAM policy (ops task).
- Backup codes for TOTP loss (deferred; spec §12.4 doesn't require).
- SCIM provisioning, WebAuthn SSO (deferred beyond v2.3).

---

## File Structure

### Create — Part A (SSO)

- `services/marketplace-api/internal/sso/models.go` — `Config`, `Provider`, `UserMapping` GORM models.
- `services/marketplace-api/internal/sso/repository.go` — tenant-scoped CRUD.
- `services/marketplace-api/internal/sso/repository_test.go`
- `services/marketplace-api/internal/sso/saml.go` — SP builder via `crewjam/saml`.
- `services/marketplace-api/internal/sso/saml_test.go`
- `services/marketplace-api/internal/sso/oidc.go` — RP builder via `go-oidc/v3`.
- `services/marketplace-api/internal/sso/oidc_test.go`
- `services/marketplace-api/internal/sso/gip_client.go` — wraps `firebase.google.com/go/v4` tenant admin.
- `services/marketplace-api/internal/sso/gip_client_test.go`
- `services/marketplace-api/internal/sso/attrmap.go` — JSON-path attribute mapping DSL.
- `services/marketplace-api/internal/sso/attrmap_test.go`
- `services/marketplace-api/internal/sso/jit.go` — JIT provisioning + mapping upsert.
- `services/marketplace-api/internal/sso/jit_test.go`
- `services/marketplace-api/internal/handlers/admin/sso_config.go` + `_test.go`
- `services/marketplace-api/internal/handlers/public/sso_login.go` + `_test.go`
- `services/marketplace-api/db/migrations/00NN_tenant_sso_configs.{up,down}.sql`
- `services/marketplace-api/db/migrations/00NN_tenant_sso_user_mappings.{up,down}.sql`

### Create — Part B (Break-glass)

- `services/marketplace-api/internal/breakglass/models.go` — `Account`, `Lockout`.
- `services/marketplace-api/internal/breakglass/repository.go` + `_test.go`
- `services/marketplace-api/internal/breakglass/credentials.go` — CSPRNG + TOTP.
- `services/marketplace-api/internal/breakglass/credentials_test.go`
- `services/marketplace-api/internal/breakglass/secret_manager.go` + `_test.go`
- `services/marketplace-api/internal/breakglass/rotation.go` + `_test.go`
- `services/marketplace-api/internal/breakglass/bootstrap.go` + `_test.go`
- `services/marketplace-api/internal/breakglass/audit.go` + `_test.go`
- `services/marketplace-api/internal/breakglass/slack.go` + `_test.go`
- `services/marketplace-api/internal/handlers/admin/break_glass_login.go` + `_test.go`
- `services/marketplace-api/cmd/break-glass-rotation/main.go` — daily cron entry point.
- `services/marketplace-api/db/migrations/00NN_break_glass_accounts.{up,down}.sql`
- `services/marketplace-api/db/migrations/00NN_break_glass_lockouts.{up,down}.sql`

### Modify

- `services/marketplace-api/internal/handlers/admin/routes.go` — mount SSO admin group (Pro-gated) + break-glass login (NOT behind `RequireActive`).
- `services/marketplace-api/internal/handlers/public/routes.go` — mount `/sso/:tenantSlug/*`.
- `services/marketplace-api/cmd/marketplace-api/main.go` — wire deps.
- `services/marketplace-api/go.mod` / `go.sum` — add `crewjam/saml` + `pquerna/otp`.
- `services/marketplace-api/internal/authbffclient/` — add `SessionIssuer` interface (thin wrapper over auth-bff's `session.Issue`).

### Delete

- Nothing.

---

## Task Sequence Overview

| # | Task | Depends on |
|---|---|---|
| 1 | Migrations: `tenant_sso_configs` + `tenant_sso_user_mappings` | — |
| 2 | `internal/sso` models + repository + tenant-isolation tests | 1 |
| 3 | SAML SP wiring (metadata, ACS) | 2 |
| 4 | OIDC RP wiring (discovery, callback) | 2 |
| 5 | GIP Admin SDK client (deterministic providerId) | 2 |
| 6 | Attribute-mapping DSL (JSON-path subset) | 2 |
| 7 | JIT provisioning + auth-bff session issuer surface | 3, 4, 6 |
| 8 | Admin SSO config endpoints (Pro-gated + tenant-isolated) | 2, 5, 6 |
| 9 | Public SSO login/callback/logout | 3, 4, 7 |
| 10 | Migrations: `break_glass_accounts` + `break_glass_lockouts` | — |
| 11 | `internal/breakglass` models + repository | 10 |
| 12 | Credentials (CSPRNG + TOTP) + Secret Manager client | 11 |
| 13 | Audit emitter (critical severity) + Slack client | 11 |
| 14 | Bootstrap — one break-glass per Pro+SSO tenant | 11, 12 |
| 15 | Break-glass login endpoint (rate-limited, dual-factor) | 12, 13 |
| 16 | Rotation: post-use 24h hook + 90-day cron | 12, 13 |
| 17 | Security regression tests (mandatory) | 8, 15 |
| 18 | Final scrub — grep for secrets, plaintext, missing audit | all |

---

## Reusable patterns

**A. Per-tenant isolation.** Every `internal/sso/repository.go` method takes `tenantID uuid.UUID` as the first parameter. Inline queries ALWAYS carry `WHERE tenant_id = ?`. Repository tests for every CRUD seed two tenants and assert the second cannot read/modify the first's row. This is the single most tested invariant in this plan (spec §12: "SSO config is per-tenant").

**B. GIP tenant-pool reuse.** Rather than provisioning a fresh GIP tenant pool per Mark8ly tenant (quota-blowup), reuse the existing `mp-internal` GIP tenant pool (per CLAUDE.md) and attach per-tenant SAML/OIDC providers under distinct `providerId` values derived deterministically from `tenant_id`: `saml.mark8ly-{sha256(tenant_id)[:8]}`. Idempotent re-uploads land on the same provider row.

**C. Secret Manager path convention.**
```
/projects/tesserix-prod/secrets/break-glass-{tenant_id}
  versions.latest → {"password":"<20-char CSPRNG>", "totp_secret":"<base32 32-byte>", "generated_at":"<rfc3339>"}
  IAM: roles/secretmanager.secretAccessor → group:break-glass-responders@mark8ly.com (≤2 members)
```
Plaintext password lives in Secret Manager only. DB stores **only** `bcrypt(password)` + a Secret Manager path reference + TOTP enrolment metadata (never the TOTP secret itself). Rotation writes Secret Manager FIRST, then DB (if DB write fails, a replay-rotation picks it up).

**D. Rate-limit + lockout.** 3 failed attempts per IP per hour → 24-hour lockout. Sliding window via Redis INCR/EXPIRE; row in `break_glass_lockouts` persists restarts. Successful login resets the counter for that IP.

**E. Dual-factor as a single step.** `/admin/break-glass/login` accepts `tenant_id + password + totp_code` in one POST. No two-step flow that could leak which factor failed. On any failure the response is uniform `{"error":"invalid_credentials"}` with 401; the audit event distinguishes the failure mode for forensics.

**F. Session minting via auth-bff.** SSO callbacks and break-glass login call `authbffclient.SessionIssuer.Issue(tenantID, userID, ttl)` which returns the `Set-Cookie` header value. The marketplace-api handler writes that header onto its response. This preserves auth-bff as the single owner of session-cookie cryptography. DO NOT invent a new session system.

**G. Audit event shape.** Every SSO config change + every break-glass login goes through `internal/audit`. Break-glass events carry `severity="critical"`, `actor="break-glass:<tenant_id>"`, and metadata `{tenant_id, success, ip_hash, user_agent, session_id}`. `ip_hash` is `HMAC-SHA256(P8_secret, client_ip)` — never raw IP.

---

## Task 1: Migrations — `tenant_sso_configs` + `tenant_sso_user_mappings`

**Files:**
- Create: `services/marketplace-api/db/migrations/00NN_tenant_sso_configs.{up,down}.sql`
- Create: `services/marketplace-api/db/migrations/00NN_tenant_sso_user_mappings.{up,down}.sql`

**Spec references:** §12.

- [ ] **Step 1: Pick next migration number**

```bash
cd services/marketplace-api
ls db/migrations/ | awk -F'_' '{print $1}' | sort -n | tail -1
```

- [ ] **Step 2: Write `tenant_sso_configs.up.sql`**

```sql
CREATE TYPE sso_provider_kind AS ENUM ('saml', 'oidc');

CREATE TABLE tenant_sso_configs (
    tenant_id       UUID PRIMARY KEY,
    provider        sso_provider_kind NOT NULL,
    -- SAML { idp_entity_id, idp_acs_url, idp_cert_pem, sp_entity_id, sp_acs_url }
    -- OIDC { issuer, client_id, client_secret_ref, discovery_url, redirect_uri, scopes }
    -- client_secret_ref is a Secret Manager path, NEVER a raw secret.
    metadata        JSONB NOT NULL,
    -- { "email":"claims.email", "firstName":"claims.given_name", "groups":"claims.groups" }
    attr_mapping    JSONB NOT NULL DEFAULT '{}'::jsonb,
    gip_provider_id TEXT,
    enabled         BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT tenant_sso_configs_metadata_required CHECK (jsonb_typeof(metadata) = 'object')
);
CREATE INDEX idx_tenant_sso_configs_enabled ON tenant_sso_configs(enabled) WHERE enabled = true;
COMMENT ON TABLE tenant_sso_configs IS 'Per-tenant SSO provider config (SAML 2.0 / OIDC via GIP). §12.';
```

- [ ] **Step 3: Write `tenant_sso_user_mappings.up.sql`**

```sql
CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE tenant_sso_user_mappings (
    tenant_id         UUID NOT NULL,
    external_user_id  TEXT NOT NULL,          -- SAML NameID / OIDC sub
    internal_user_id  UUID NOT NULL,
    email             CITEXT NOT NULL,
    last_login_at     TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, external_user_id),
    CONSTRAINT tenant_sso_user_mappings_internal_unique UNIQUE (tenant_id, internal_user_id)
);
CREATE INDEX idx_tenant_sso_user_mappings_email ON tenant_sso_user_mappings(tenant_id, email);
COMMENT ON TABLE tenant_sso_user_mappings IS 'JIT SSO user bindings + last-login audit trail. §12.';
```

- [ ] **Step 4: Write down migrations** — `DROP TABLE` + `DROP TYPE sso_provider_kind`.

- [ ] **Step 5: Run + verify**

```bash
make migrate-up && psql $DATABASE_URL -c "\d tenant_sso_configs"
make migrate-down-1 && make migrate-down-1 && make migrate-up
```

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/db/migrations/
git commit -m "feat(sso): tenant_sso_configs + tenant_sso_user_mappings migrations"
```

---

## Task 2: `internal/sso` models + repository + tenant-isolation tests

**Files:** `internal/sso/{models,repository,repository_test}.go`

**Spec references:** §12.

- [ ] **Step 1: Failing test — tenant isolation on GetConfig**

```go
//go:build integration
func TestRepository_GetConfig_TenantIsolation(t *testing.T) {
    db := testdb.NewDB(t, "tenant_sso_configs")
    repo := sso.NewRepository(db)
    a, b := uuid.New(), uuid.New()
    require.NoError(t, repo.Upsert(ctx, &sso.Config{TenantID: a, Provider: sso.ProviderSAML,
        Metadata: map[string]any{"idp_entity_id":"https://idp.a/"}, Enabled: true}))
    _, err := repo.GetByTenant(ctx, b)
    require.ErrorIs(t, err, sso.ErrNotFound)
    // Cross-tenant update affects 0 rows → ErrNotFound.
    require.ErrorIs(t, repo.UpdateEnabled(ctx, b, false), sso.ErrNotFound)
    cfg, _ := repo.GetByTenant(ctx, a)
    require.True(t, cfg.Enabled) // A's row unchanged
}
```

- [ ] **Step 2: Failing test — Upsert is idempotent by tenant, Delete is tenant-scoped, purges mappings.** (Two further tests along the same shape.)

- [ ] **Step 3: Run — expect FAIL**

- [ ] **Step 4: Write `models.go`** — `Config`, `UserMapping` GORM structs; `Provider` string enum (`ProviderSAML`, `ProviderOIDC`); `JSONMap` scan/value for jsonb columns; `ErrNotFound`, `ErrInvalidMetadata`, `ErrInvalidAttrMap` sentinel errors.

- [ ] **Step 5: Write `repository.go`** — methods scoped by tenant_id:
  - `GetByTenant(ctx, tenantID) (*Config, error)` — `Where("tenant_id = ?", ...)` then ErrNotFound on not-found.
  - `Upsert(ctx, *Config) error` — call `Validate` first, then GORM `Save`.
  - `UpdateEnabled(ctx, tenantID, enabled) error` — `Updates(...)`; check `RowsAffected == 0` → ErrNotFound. This is how cross-tenant calls fail loudly.
  - `Delete(ctx, tenantID) error` — TX: delete mappings, delete config.
  - `UpsertMapping(ctx, *UserMapping) error` — sets `last_login_at = now()`.
  - `Validate(*Config) error` — SAML requires `idp_entity_id, idp_acs_url, idp_cert_pem`; OIDC requires `issuer, client_id, discovery_url`.

- [ ] **Step 6: Run tests — expect PASS**

- [ ] **Step 7: Commit**

```bash
git commit -m "feat(sso): tenant-scoped repository for tenant_sso_configs"
```

---

## Task 3: SAML SP wiring

**Files:** `internal/sso/{saml,saml_test}.go`

**Spec references:** §12 (SAML 2.0).

- [ ] **Step 1: Add dep**

```bash
go get github.com/crewjam/saml@v0.4.14 && go mod tidy
```

- [ ] **Step 2: Failing test — SP builds from config + rejects unsigned assertions**

```go
func TestSAML_BuildSP_FromConfig(t *testing.T) {
    cfg := &sso.Config{Provider: sso.ProviderSAML, Metadata: map[string]any{
        "idp_entity_id":"https://idp.example.com/entity",
        "idp_acs_url":"https://idp.example.com/sso",
        "idp_cert_pem":samlTestCertPEM,
        "sp_entity_id":"https://api.mark8ly.com/sso/acme/metadata",
        "sp_acs_url":"https://api.mark8ly.com/sso/acme/callback",
    }}
    sp, err := sso.BuildSAMLServiceProvider(cfg, loadTestKeypair(t))
    require.NoError(t, err)
    require.Equal(t, "https://api.mark8ly.com/sso/acme/callback", sp.AcsURL.String())
}

func TestSAML_ACS_RejectsUnsignedAssertion(t *testing.T) {
    // crewjam/saml's ParseResponse already rejects unsigned; we assert our wrapper
    // returns 401 + emits sso.login.failed with reason "saml_signature_invalid".
    h, rec := newSAMLHandlerForTest(t)
    req := httptest.NewRequest("POST", "/sso/acme/callback", strings.NewReader(unsignedSAMLResponseBody))
    w := httptest.NewRecorder()
    h.ServeHTTP(w, req)
    require.Equal(t, 401, w.Code)
    require.Len(t, rec.EventsByType("sso.login.failed"), 1)
}
```

- [ ] **Step 3: Write `saml.go`** — `BuildSAMLServiceProvider(cfg *Config, kp tls.Certificate) (*samlsp.Middleware, error)` extracts `sp_acs_url`, `sp_entity_id`, `idp_entity_id`, `idp_acs_url` from metadata; parses `idp_cert_pem`; calls `samlsp.New` with an `IDPMetadata` `EntityDescriptor`. The shared Mark8ly SP keypair is loaded at startup from Secret Manager path `/projects/tesserix-prod/secrets/saml-sp-keypair` — the per-tenant customization lives entirely in IdP metadata.

- [ ] **Step 4: Run tests — expect PASS**

- [ ] **Step 5: Commit**

```bash
git commit -m "feat(sso): SAML 2.0 SP wiring via crewjam/saml"
```

---

## Task 4: OIDC RP wiring

**Files:** `internal/sso/{oidc,oidc_test}.go`

**Spec references:** §12 (OIDC via GIP).

- [ ] **Step 1: Failing tests — RP constructs from discovery, verifies ID token, rejects bad nonce**

```go
func TestOIDC_BuildRP_FromConfig(t *testing.T) {
    srv := stubOIDCDiscoveryServer(t)
    defer srv.Close()
    cfg := &sso.Config{Provider: sso.ProviderOIDC, Metadata: map[string]any{
        "issuer": srv.URL, "client_id":"client-abc", "discovery_url": srv.URL+"/.well-known/openid-configuration",
    }}
    rp, err := sso.BuildOIDCRelyingParty(ctx, cfg, "test-secret")
    require.NoError(t, err)
    require.Equal(t, "client-abc", rp.ClientID)
}

func TestOIDC_Exchange_VerifiesNonce(t *testing.T) {
    rp := newTestRPWithStubOIDCServer(t)
    _, err := rp.Exchange(ctx, "code-abc", "wrong-nonce")
    require.ErrorContains(t, err, "nonce mismatch")
}
```

- [ ] **Step 2: Write `oidc.go`** — `BuildOIDCRelyingParty(ctx, *Config, clientSecret string) (*OIDCRelyingParty, error)` wraps `oidc.NewProvider(ctx, issuer)` + `provider.Verifier(&oidc.Config{ClientID: ...})` + `oauth2.Config` with `openid email profile` scopes. `AuthURL(state, nonce)` returns the authorization endpoint URL. `Exchange(ctx, code, expectedNonce)` swaps the code, verifies ID token signature + nonce, returns claims map (with `sub` populated). `clientSecret` is resolved from a Secret Manager ref **outside** this function so plaintext secrets never sit in DB rows.

- [ ] **Step 3: Run tests — expect PASS**

- [ ] **Step 4: Commit**

```bash
git commit -m "feat(sso): OIDC RP wiring reusing go-oidc/v3"
```

---

## Task 5: GIP Admin SDK client (deterministic providerId)

**Files:** `internal/sso/{gip_client,gip_client_test}.go`

**Spec references:** §12 (via GIP), CLAUDE.md (`mp-internal` GIP tenant pool).

- [ ] **Step 1: Failing tests**

```go
func TestGIP_DeriveProviderID_Deterministic(t *testing.T) {
    tenantID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
    id1 := sso.GIPProviderID(tenantID, sso.ProviderSAML)
    id2 := sso.GIPProviderID(tenantID, sso.ProviderSAML)
    require.Equal(t, id1, id2, "must be deterministic for idempotent re-upload")
    require.True(t, strings.HasPrefix(id1, "saml.mark8ly-"))
    require.NotEqual(t, id1, sso.GIPProviderID(tenantID, sso.ProviderOIDC))
}

func TestGIP_UploadSAML_CallsFirebaseAdminSDK(t *testing.T) {
    fake := &fakeGIPAdminClient{}
    client := sso.NewGIPClientFromFake(fake, "mp-internal")
    require.NoError(t, client.UploadSAMLProvider(ctx, uuid.New(), sso.SAMLProviderPayload{
        IDPEntityID:"https://idp.example.com/entity",
    }))
    require.Equal(t, 1, fake.UpsertSAMLCallCount)
    require.Equal(t, "mp-internal", fake.LastPoolID)
    require.True(t, strings.HasPrefix(fake.LastProviderID, "saml.mark8ly-"))
}
```

- [ ] **Step 2: Write `gip_client.go`**
  - `GIPProviderID(tenantID, provider) string` — `fmt.Sprintf("%s.mark8ly-%s", provider, hex.EncodeToString(sha256(tenantID+providerByte)[:4]))`. Idempotent by construction.
  - `GIPClient` wraps `gipAdminClient` interface (`UpsertSAMLProvider`, `UpsertOIDCProvider`, `DeleteProvider` — all taking `poolID, providerID, payload`). Production impl uses `auth.TenantClient` from `firebase.google.com/go/v4`.
  - `UploadSAMLProvider(ctx, tenantID, SAMLProviderPayload) error` / `UploadOIDCProvider(...) error` / `Delete(ctx, tenantID, Provider) error`.

- [ ] **Step 3: Run tests — expect PASS**

- [ ] **Step 4: Commit**

```bash
git commit -m "feat(sso): GIP tenant-pool admin client with deterministic providerId"
```

---

## Task 6: Attribute-mapping DSL

**Files:** `internal/sso/{attrmap,attrmap_test}.go`

**Spec references:** §12 (minimal mapping per plan brief).

- [ ] **Step 1: Failing tests**

```go
func TestAttrMap_Resolve_SimpleKeys(t *testing.T) {
    claims := map[string]any{"email":"u@x", "given_name":"Jane", "groups":[]any{"admins","staff"}}
    m := sso.AttrMapping{"email":"claims.email", "firstName":"claims.given_name", "role":"claims.groups[0]"}
    out, _ := m.Resolve(map[string]any{"claims": claims})
    require.Equal(t, "u@x", out["email"])
    require.Equal(t, "Jane", out["firstName"])
    require.Equal(t, "admins", out["role"])
}

func TestAttrMap_Validate_RejectsNonClaimsRoot(t *testing.T) {
    require.Error(t, sso.AttrMapping{"role":"profile.role"}.Validate())
}

func TestAttrMap_Resolve_MissingPath_Skipped(t *testing.T) {
    out, _ := sso.AttrMapping{"firstName":"claims.given_name"}.Resolve(map[string]any{"claims": map[string]any{"email":"x"}})
    _, ok := out["firstName"]
    require.False(t, ok)
}
```

- [ ] **Step 2: Write `attrmap.go`** — `AttrMapping map[string]string` where RHS must start with `claims.` and use dot segments with optional `[N]` indexing. `Validate()` rejects non-`claims.` roots and invalid segments via regex `^([a-zA-Z_][a-zA-Z0-9_]*)(?:\[(\d+)\])?$`. `Resolve(root map[string]any)` walks the map, skipping missing paths silently. More elaborate transformations deferred to a support-only escape hatch.

- [ ] **Step 3: Run tests — expect PASS**

- [ ] **Step 4: Commit**

```bash
git commit -m "feat(sso): minimal JSON-path attribute mapping DSL"
```

---

## Task 7: JIT provisioning + auth-bff session issuer

**Files:** `internal/sso/{jit,jit_test}.go` + `internal/authbffclient/session_issuer.go`

**Spec references:** §12 (JIT provisioning).

- [ ] **Step 1: Failing tests**

```go
//go:build integration
func TestJIT_FirstLogin_CreatesUserAndMapping(t *testing.T) {
    out, err := mapper.Provision(ctx, sso.JITInput{
        TenantID: tenantID, Provider: sso.ProviderOIDC,
        ExternalUserID: "oidc-sub-123", Email: "new@example.com",
        Attributes: map[string]any{"firstName":"Jane"}, DefaultRole: "admin",
    })
    require.NoError(t, err)
    require.True(t, out.Created)
    m, _ := mappings.GetByTenant(ctx, tenantID, "oidc-sub-123")
    require.Equal(t, out.InternalUserID, m.InternalUserID)
}

func TestJIT_ExistingEmail_BindsInsteadOfCreating(t *testing.T) {
    existing := suite.SeedUser(tenantID, "existing@example.com")
    out, err := mapper.Provision(ctx, sso.JITInput{TenantID: tenantID, Provider: sso.ProviderSAML,
        ExternalUserID: "saml-sub-999", Email: "existing@example.com"})
    require.NoError(t, err)
    require.Equal(t, existing.ID, out.InternalUserID, "must bind to existing user")
    require.False(t, out.Created)
}
```

- [ ] **Step 2: Write `jit.go`** — `JITInput{TenantID, Provider, ExternalUserID, Email, Attributes, DefaultRole}` → `JITOutput{InternalUserID, Created}`. `UsersRepo` interface with `FindByTenantAndEmail` + `CreateForTenant` — minimal surface. `Provision` runs a GORM transaction: lookup by email (bind if found), else create, always upsert mapping (refreshes `last_login_at` on repeat logins).

- [ ] **Step 3: Write `authbffclient/session_issuer.go`**

```go
package authbffclient

type SessionIssuer interface {
    // Issue returns the Set-Cookie header value to write on the response.
    Issue(ctx context.Context, tenantID, userID uuid.UUID, ttl time.Duration) (string, error)
}
```

Production implementation calls auth-bff's HTTP endpoint (or a direct embedded package if auth-bff exposes `session.Issue`). **Do not duplicate cookie cryptography in marketplace-api.**

- [ ] **Step 4: Run tests — expect PASS**

- [ ] **Step 5: Commit**

```bash
git commit -m "feat(sso): JIT provisioner + auth-bff session issuer surface"
```

---

## Task 8: Admin SSO config endpoints (Pro-gated + tenant-isolated)

**Files:** `internal/handlers/admin/{sso_config,sso_config_test}.go` + modify `routes.go`

**Spec references:** §9 (SSO Pro-only), §12.

- [ ] **Step 1: Failing test — non-Pro returns 403**

```go
func TestAdminSSO_RejectsNonProPlan(t *testing.T) {
    tenantID := suite.SeedTenantWithPlan(subscription.PlanStarter)
    w := suite.DoJSON("POST", "/admin/tenants/"+tenantID.String()+"/sso/config",
        map[string]any{"provider":"saml","metadata":map[string]any{"idp_entity_id":"x"}})
    require.Equal(t, 403, w.Code)
}

func TestAdminSSO_AllowsProPlan(t *testing.T) {
    tenantID := suite.SeedTenantWithPlan(subscription.PlanPro)
    w := suite.DoJSON("POST", "/admin/tenants/"+tenantID.String()+"/sso/config", validSAMLBody())
    require.Equal(t, 201, w.Code)
}
```

- [ ] **Step 2: Failing test — cross-tenant blocked**

```go
func TestAdminSSO_CrossTenantRead_Rejected(t *testing.T) {
    a := suite.SeedTenantWithPlan(subscription.PlanPro)
    b := suite.SeedTenantWithPlan(subscription.PlanPro)
    suite.SeedSSOConfig(a, sso.ProviderSAML)
    // Authenticated as B; asking for A's config.
    w := suite.AsTenant(b).Do("GET", "/admin/tenants/"+a.String()+"/sso/config", nil)
    require.Equal(t, 403, w.Code)
}
```

- [ ] **Step 3: Failing test — DELETE purges mappings**

```go
func TestAdminSSO_Delete_DisablesAndPurgesMappings(t *testing.T) {
    tenantID := suite.SeedTenantWithPlan(subscription.PlanPro)
    suite.SeedSSOConfig(tenantID, sso.ProviderOIDC)
    suite.SeedSSOMapping(tenantID, "ext-1", "u@x")
    w := suite.AsTenant(tenantID).Do("DELETE", "/admin/tenants/"+tenantID.String()+"/sso/config", nil)
    require.Equal(t, 204, w.Code)
    _, err := suite.SSORepo.GetByTenant(ctx, tenantID)
    require.ErrorIs(t, err, sso.ErrNotFound)
}
```

- [ ] **Step 4: Write `sso_config.go`** — handler struct with four methods:
  - `Upsert(c)` — path-tenant check (must equal authenticated tenant; else 403), bind JSON, `sso.Validate(cfg)`, `AttrMapping.Validate()`, `repo.Upsert`, then best-effort `gipClient.UploadSAMLProvider` / `UploadOIDCProvider` (failure → `sso.gip_upload.failed` audit warning, DOES NOT roll back DB — retry endpoint recovers), emit `sso.config.upserted` audit, return 201.
  - `Get(c)` — tenant check; read config; **redact** `client_secret` / `client_secret_ref` from metadata before returning.
  - `Test(c)` — dry-run via `sso.TestConfig(...)`, returns per-provider diagnostic.
  - `Delete(c)` — tenant check; TX delete mappings + config; `gipClient.Delete` best-effort for both provider kinds; emit `sso.config.deleted` warning audit; return 204.

  The path-tenant check is a 5-line helper: `authed := c.Get("tenant_id"); if authed.(string) != wantedTenantID.String() { return 403 }`.

- [ ] **Step 5: Modify `routes.go`**

```go
adminTenant := admin.Group("/tenants/:tenantId",
    plangate.RequireFeature(plangate.FeatureSSO),
)
adminTenant.POST("/sso/config",   deps.SSO.Upsert)
adminTenant.GET("/sso/config",    deps.SSO.Get)
adminTenant.DELETE("/sso/config", deps.SSO.Delete)
adminTenant.POST("/sso/test",     deps.SSO.Test)
```

- [ ] **Step 6: Run tests — expect PASS**

- [ ] **Step 7: Commit**

```bash
git commit -m "feat(admin): Pro-gated SSO config endpoints with cross-tenant isolation"
```

---

## Task 9: Public SSO login/callback/logout

**Files:** `internal/handlers/public/{sso_login,sso_login_test}.go` + modify `routes.go`

**Spec references:** §12.

- [ ] **Step 1: Failing test — GET /sso/:slug/login 302-redirects**

```go
func TestSSO_Login_SAML_RedirectsToIdP(t *testing.T) {
    tenantID := suite.SeedTenantBySlug("acme", subscription.PlanPro)
    suite.SeedSSOConfig(tenantID, sso.ProviderSAML)
    w := suite.Do("GET", "/sso/acme/login", nil)
    require.Equal(t, 302, w.Code)
    require.Contains(t, w.Header().Get("Location"), "SAMLRequest=")
}
```

- [ ] **Step 2: Failing test — callback JIT-provisions + mints session cookie**

```go
func TestSSO_Callback_OIDC_ExchangesAndMintsSession(t *testing.T) {
    tenantID := suite.SeedTenantBySlug("acme", subscription.PlanPro)
    suite.StubOIDCProvider(tenantID, "sub-xyz", "jit-user@example.com")

    state := parseState(suite.Do("GET", "/sso/acme/login", nil))
    w := suite.DoForm("POST", "/sso/acme/callback", url.Values{"code":{"code-abc"}, "state":{state}})
    require.Equal(t, 302, w.Code)
    require.NotEmpty(t, w.Header().Values("Set-Cookie"), "must set session cookie")

    m, err := suite.Mappings.GetByTenant(ctx, tenantID, "sub-xyz")
    require.NoError(t, err)
    require.Equal(t, "jit-user@example.com", m.Email)
}
```

- [ ] **Step 3: Write `sso_login.go`** — resolve tenant from `:tenantSlug`; for SAML delegate to `samlSPs.Get(tenantID).HandleStartAuthFlow`; for OIDC, generate `state` + `nonce`, store in state-cache, redirect to `rp.AuthURL(state, nonce)`. Callback: for SAML read ACS-parsed assertion from crewjam, extract NameID + attributes; for OIDC `state` lookup → `rp.Exchange(ctx, code, expectedNonce)` → claims. Resolve attributes through `cfg.AttrMapping.Resolve(map[string]any{"claims": claims})`. Call `jit.Provision(ctx, JITInput{...})`. Call `session.Issue(ctx, tenantID, internalUserID, 12*time.Hour)` → write `Set-Cookie` → 302 to admin home. Emit `sso.login.success` audit. Logout: clear Mark8ly cookie + call IdP SLO if configured.

- [ ] **Step 4: Mount routes**

```go
public.GET("/sso/:tenantSlug/login",     deps.SSOLogin.Login)
public.POST("/sso/:tenantSlug/callback", deps.SSOLogin.Callback)
public.POST("/sso/:tenantSlug/logout",   deps.SSOLogin.Logout)
```

No `RequireFeature` here — gate is "does a config row exist for this slug?" If none → 404. Admin config is where the Pro gate enforces.

- [ ] **Step 5: Run tests — expect PASS**

- [ ] **Step 6: Commit**

```bash
git commit -m "feat(sso): public login/callback/logout endpoints with JIT + session mint"
```

---

## Task 10: Migrations — `break_glass_accounts` + `break_glass_lockouts`

**Files:** `db/migrations/00NN_break_glass_accounts.{up,down}.sql` + `00NN_break_glass_lockouts.{up,down}.sql`

**Spec references:** §12.4.

- [ ] **Step 1: Write `break_glass_accounts.up.sql`**

```sql
CREATE TABLE break_glass_accounts (
    tenant_id             UUID PRIMARY KEY,
    secret_path           TEXT NOT NULL,        -- /projects/tesserix-prod/secrets/break-glass-{uuid}
    password_hash         TEXT NOT NULL,        -- bcrypt cost 12
    totp_secret_ref       TEXT NOT NULL,        -- JSON pointer into secret blob (e.g. "$.totp_secret")
    totp_enrolled         BOOLEAN NOT NULL DEFAULT false,
    last_rotated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at          TIMESTAMPTZ,
    rotation_scheduled_at TIMESTAMPTZ,          -- set to now()+24h on successful login
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_break_glass_rotation_scheduled ON break_glass_accounts(rotation_scheduled_at)
    WHERE rotation_scheduled_at IS NOT NULL;
COMMENT ON TABLE break_glass_accounts IS 'One emergency account per Pro+SSO tenant. §12.4.';
```

- [ ] **Step 2: Write `break_glass_lockouts.up.sql`**

```sql
CREATE TABLE break_glass_lockouts (
    ip_hash      BYTEA NOT NULL,         -- HMAC of client IP (see §P8)
    tenant_id    UUID,
    locked_until TIMESTAMPTZ NOT NULL,
    reason       TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (ip_hash, locked_until)
);
CREATE INDEX idx_break_glass_lockouts_active ON break_glass_lockouts(locked_until) WHERE locked_until > now();
```

- [ ] **Step 3: Down migrations** — `DROP TABLE` each.

- [ ] **Step 4: Run up/down smoke**

- [ ] **Step 5: Commit**

```bash
git commit -m "feat(break-glass): break_glass_accounts + break_glass_lockouts migrations"
```

---

## Task 11: `internal/breakglass` models + repository

**Files:** `internal/breakglass/{models,repository,repository_test}.go`

- [ ] **Step 1: Failing tests — create + fetch, tenant isolation, lockout lifecycle**

```go
//go:build integration
func TestBreakGlassRepo_TenantIsolation(t *testing.T) {
    a, b := uuid.New(), uuid.New()
    require.NoError(t, repo.Create(ctx, &breakglass.Account{TenantID: a, SecretPath:"x", PasswordHash:"x", TOTPSecretRef:"x"}))
    require.NoError(t, repo.Create(ctx, &breakglass.Account{TenantID: b, SecretPath:"y", PasswordHash:"y", TOTPSecretRef:"y"}))
    require.NoError(t, repo.UpdateAfterUse(ctx, a))
    ax, _ := repo.GetByTenant(ctx, a); bx, _ := repo.GetByTenant(ctx, b)
    require.NotNil(t, ax.LastUsedAt)
    require.Nil(t, bx.LastUsedAt, "B's row must be untouched")
}

func TestBreakGlassRepo_LockoutLifecycle(t *testing.T) {
    ipHash := []byte{1,2,3,4}
    locked, _ := repo.IsIPLocked(ctx, ipHash); require.False(t, locked)
    require.NoError(t, repo.LockIP(ctx, ipHash, nil, "too_many_failed", 24*time.Hour))
    locked, _ = repo.IsIPLocked(ctx, ipHash); require.True(t, locked)
}
```

- [ ] **Step 2: Write `models.go`** — `Account{TenantID (PK), SecretPath, PasswordHash, TOTPSecretRef, TOTPEnrolled, LastRotatedAt, LastUsedAt, RotationScheduledAt, CreatedAt, UpdatedAt}`; `Lockout{IPHash, TenantID, LockedUntil, Reason, CreatedAt}`. GORM table names: `break_glass_accounts` / `break_glass_lockouts`.

- [ ] **Step 3: Write `repository.go`** — methods:
  - `Create(ctx, *Account) error`
  - `GetByTenant(ctx, tenantID) (*Account, error)` — ErrNotFound on not-found.
  - `UpdateAfterUse(ctx, tenantID)` — sets `last_used_at=now()`, `rotation_scheduled_at=now()+24h`; 0 rows → ErrNotFound.
  - `ReplaceAfterRotation(ctx, tenantID, newHash)` — updates `password_hash`, `last_rotated_at=now()`, clears `rotation_scheduled_at`. Caller MUST write Secret Manager first.
  - `FindDueForRotation(ctx, ninetyDaysAgo)` — `WHERE (rotation_scheduled_at IS NOT NULL AND rotation_scheduled_at <= now()) OR last_rotated_at <= ninetyDaysAgo`.
  - `LockIP(ctx, ipHash, tenantID, reason, dur)` / `IsIPLocked(ctx, ipHash) bool` — 0 `>now()` rows.

- [ ] **Step 4: Run tests — expect PASS**

- [ ] **Step 5: Commit**

```bash
git commit -m "feat(break-glass): account + lockout repository with tenant isolation"
```

---

## Task 12: Credentials (CSPRNG + TOTP) + Secret Manager client

**Files:** `internal/breakglass/{credentials,credentials_test,secret_manager,secret_manager_test}.go`

**Spec references:** §12.4 (CSPRNG 20 chars, TOTP mandatory not SMS).

- [ ] **Step 1: Add dep**

```bash
go get github.com/pquerna/otp@v1.4.0 && go mod tidy
```

- [ ] **Step 2: Failing tests**

```go
func TestGeneratePassword_20CharsWithAllClasses(t *testing.T) {
    for i := 0; i < 100; i++ {
        p, err := breakglass.GeneratePassword()
        require.NoError(t, err)
        require.Len(t, p, 20)
        require.Regexp(t, `[A-Z]`, p)
        require.Regexp(t, `[a-z]`, p)
        require.Regexp(t, `[0-9]`, p)
        require.Regexp(t, `[\W_]`, p)
    }
}

func TestTOTP_GenerateVerifyRoundTrip(t *testing.T) {
    s, _ := breakglass.GenerateTOTPSecret()
    code, _ := breakglass.TOTPCode(s, time.Now())
    require.True(t, breakglass.VerifyTOTP(s, code, time.Now()))
}

func TestTOTP_RejectsOldCode(t *testing.T) {
    s, _ := breakglass.GenerateTOTPSecret()
    old, _ := breakglass.TOTPCode(s, time.Now().Add(-5*time.Minute))
    require.False(t, breakglass.VerifyTOTP(s, old, time.Now()))
}
```

- [ ] **Step 3: Write `credentials.go`**
  - `GeneratePassword()` — `crypto/rand` loop picking from 4 pools (upper excluding `IO`, lower excluding `l/o`, digits excluding `0/1`, curated symbols `!#$%&*+-=?@^_~`). Rejection-sample until the output contains at least one of each class. 20 chars total.
  - `GenerateTOTPSecret()` — call `totp.Generate(totp.GenerateOpts{Issuer:"Mark8ly", AccountName:"break-glass", SecretSize:32, Algorithm: otp.AlgorithmSHA1})`; return `key.Secret()` (base32, ready for Secret Manager).
  - `TOTPCode(secret, t)` — `totp.GenerateCode(secret, t)` (RFC 6238, 6 digits, SHA-1, 30s period).
  - `VerifyTOTP(secret, code, at)` — `totp.ValidateCustom` with `Period: 30, Skew: 1` (±30s window). Wider windows are a well-known TOTP risk; spec §12.4 wants mandatory, not lenient.
  - `OTPAuthURI(secret, tenantID)` — returns `otpauth://totp/Mark8ly:break-glass-{id}?secret=...&issuer=Mark8ly&algorithm=SHA1&digits=6&period=30` for P16 QR rendering.

- [ ] **Step 4: Failing test + implementation — Secret Manager round-trip**

```go
func TestSecretManager_RoundTrip(t *testing.T) {
    fake := breakglass.NewFakeSecretClient()
    sm := breakglass.NewSecretManagerFromFake(fake)
    require.NoError(t, sm.Upsert(ctx, "/path", breakglass.Blob{Password:"hunter2", TOTPSecret:"T", GeneratedAt: time.Now()}))
    got, _ := sm.Fetch(ctx, "/path")
    require.Equal(t, "hunter2", got.Password)
}
```

`secret_manager.go` — `Blob{Password, TOTPSecret, GeneratedAt}`; `SecretClient` interface (`AddVersion(ctx, path, []byte) error`, `AccessLatest(ctx, path) ([]byte, error)`); `SecretManager{c SecretClient}` with `Upsert(ctx, path, Blob) error` (JSON-marshal blob, AddVersion) + `Fetch(ctx, path) (*Blob, error)`. Production impl wraps `cloud.google.com/go/secretmanager`.

- [ ] **Step 5: Run tests — expect PASS**

- [ ] **Step 6: Commit**

```bash
git commit -m "feat(break-glass): CSPRNG password + TOTP credentials + Secret Manager client"
```

---

## Task 13: Audit emitter (critical severity) + Slack client

**Files:** `internal/breakglass/{audit,audit_test,slack,slack_test}.go`

**Spec references:** §12.4 (Slack alert + audit log, severity=critical).

- [ ] **Step 1: Failing test — audit event carries ip_hash (not raw IP) and severity=critical**

```go
func TestAudit_EmitLoginEvent_Critical(t *testing.T) {
    rec := audit.NewRecorderForTesting()
    e := audit.NewEmitter(rec)
    a := breakglass.NewAuditEmitter(e, breakglass.HMACIPHash([]byte("test-key")))

    ctx := &gin.Context{Request: httptest.NewRequest("POST", "/", nil)}
    ctx.Request.RemoteAddr = "203.0.113.7:443"
    ctx.Request.Header.Set("User-Agent", "cli/1.0")
    a.EmitLogin(ctx, uuid.New(), true, "session-xyz")

    events := rec.EventsByType("break_glass.login.success")
    require.Len(t, events, 1)
    require.Equal(t, "critical", events[0].Severity)
    require.NotEmpty(t, events[0].Metadata["ip_hash"])
    require.NotContains(t, events[0].Metadata, "ip") // raw IP must NEVER appear
    require.Equal(t, "cli/1.0", events[0].Metadata["user_agent"])
    require.Equal(t, "session-xyz", events[0].Metadata["session_id"])
}
```

- [ ] **Step 2: Failing test — Slack posts to #security-alerts**

```go
func TestSlack_PostsToSecurityAlerts(t *testing.T) {
    var got map[string]any
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        _ = json.NewDecoder(r.Body).Decode(&got); w.WriteHeader(200)
    }))
    defer srv.Close()
    client := breakglass.NewSlackClient(srv.URL, "#security-alerts")
    require.NoError(t, client.PostLoginAlert(ctx, uuid.New(), true))
    require.Equal(t, "#security-alerts", got["channel"])
    require.Contains(t, got["text"], "break-glass login")
}
```

- [ ] **Step 3: Write `audit.go`** — `AuditEmitter{e *audit.Emitter, hmac HMACKey}`. `EmitLogin(c, tenantID, success, sessionID)`:
  - event type: `break_glass.login.success` or `break_glass.login.failed`
  - severity: `"critical"`
  - actor: `"break-glass:<tenant_id>"`
  - metadata: `tenant_id, success, ip_hash (HMAC-SHA256 of client IP under HMACKey), user_agent, session_id`
  - client IP resolved from `X-Forwarded-For` first (comma-split head), else `RemoteAddr` host
  - **raw IP never stored** anywhere in the event

When P8's `HMACIPHash` helper lands, swap the local HMAC for the shared helper — documented as a TODO comment referencing the P8 import path.

- [ ] **Step 4: Write `slack.go`** — `SlackClient{webhookURL, channel, http}` with 5s timeout. `PostLoginAlert(ctx, tenantID, success)` POSTs `{"channel":"#security-alerts", "username":"mark8ly-security", "text":":rotating_light: break-glass login {STATUS} — tenant=..."}`. Returns error on status >= 300.

- [ ] **Step 5: Run tests — expect PASS**

- [ ] **Step 6: Commit**

```bash
git commit -m "feat(break-glass): critical-severity audit emitter + Slack alert client"
```

---

## Task 14: Bootstrap — one break-glass per Pro+SSO tenant

**Files:** `internal/breakglass/{bootstrap,bootstrap_test}.go`

**Spec references:** §12.4.

- [ ] **Step 1: Failing tests**

```go
//go:build integration
func TestBootstrap_CreatesAccountAndSecretBlob(t *testing.T) {
    tenantID := uuid.New()
    require.NoError(t, bootstrap.Provision(ctx, tenantID))
    acc, _ := repo.GetByTenant(ctx, tenantID)
    require.True(t, acc.TOTPEnrolled)
    require.NotEmpty(t, acc.PasswordHash)
    blob, _ := secrets.Fetch(ctx, acc.SecretPath)
    require.Len(t, blob.Password, 20)
    require.NotEmpty(t, blob.TOTPSecret)
}

func TestBootstrap_IdempotentOnRetry(t *testing.T) {
    tenantID := uuid.New()
    require.NoError(t, bootstrap.Provision(ctx, tenantID))
    require.ErrorIs(t, bootstrap.Provision(ctx, tenantID), breakglass.ErrAlreadyProvisioned)
}
```

- [ ] **Step 2: Write `bootstrap.go`** — `Bootstrapper{repo, secrets, project}`. `Provision(ctx, tenantID) error`:
  1. Check existing account → `ErrAlreadyProvisioned` if present.
  2. `GeneratePassword()` + `GenerateTOTPSecret()` + `bcrypt.GenerateFromPassword([]byte(pw), 12)`.
  3. Write Secret Manager FIRST: path = `/projects/{project}/secrets/break-glass-{tenantID}`, blob = `{Password, TOTPSecret, GeneratedAt}`.
  4. Insert DB row with `PasswordHash = bcrypt output`, `TOTPSecretRef = "$.totp_secret"`, `TOTPEnrolled = true`.

- [ ] **Step 3: Wire into Pro+SSO signup** — in `internal/tenants/provisioner.go` (or equivalent), after a tenant transitions to Pro with SSO enabled, call `bootstrapper.Provision(ctx, tenantID)`. Failure does NOT roll back the Pro upgrade — emit `break_glass.bootstrap.failed` warning and schedule a retry. Signup still succeeds; the break-glass account backfills out-of-band.

- [ ] **Step 4: Run tests — expect PASS**

- [ ] **Step 5: Commit**

```bash
git commit -m "feat(break-glass): per-tenant bootstrap on Pro+SSO signup"
```

---

## Task 15: Break-glass login endpoint (rate-limited, dual-factor)

**Files:** `internal/handlers/admin/{break_glass_login,break_glass_login_test}.go` + modify `routes.go`

**Spec references:** §12.4 (login flow).

- [ ] **Step 1: Failing test — password alone fails**

```go
//go:build integration
func TestBreakGlass_PasswordOnly_Fails(t *testing.T) {
    tenantID, pw, _ := suite.SeedBreakGlass()
    w := suite.DoJSON("POST", "/admin/break-glass/login", map[string]any{
        "tenant_id": tenantID.String(), "password": pw,
        // totp_code deliberately omitted
    })
    require.Equal(t, 401, w.Code)
    require.Contains(t, w.Body.String(), "invalid_credentials")
}
```

- [ ] **Step 2: Failing test — TOTP alone fails** — symmetric to Step 1, omit password.

- [ ] **Step 3: Failing test — both correct succeeds + Slack + audit + rotation scheduled**

```go
func TestBreakGlass_PasswordPlusTOTP_Succeeds(t *testing.T) {
    tenantID, pw, totpSecret := suite.SeedBreakGlass()
    code, _ := breakglass.TOTPCode(totpSecret, time.Now())
    w := suite.DoJSON("POST", "/admin/break-glass/login", map[string]any{
        "tenant_id": tenantID.String(), "password": pw, "totp_code": code,
    })
    require.Equal(t, 200, w.Code)
    require.NotEmpty(t, w.Header().Get("Set-Cookie"))
    require.True(t, suite.Slack.Called())
    events := suite.Audit.EventsByType("break_glass.login.success")
    require.Equal(t, "critical", events[0].Severity)
    acc, _ := suite.Repo.GetByTenant(ctx, tenantID)
    require.WithinDuration(t, time.Now().Add(24*time.Hour), *acc.RotationScheduledAt, 5*time.Minute)
}
```

- [ ] **Step 4: Failing test — rate-limit locks after 3 failures**

```go
func TestBreakGlass_RateLimit_LocksOutAfter3Failures(t *testing.T) {
    tenantID, _, _ := suite.SeedBreakGlass()
    for i := 0; i < 3; i++ {
        w := suite.DoJSON("POST", "/admin/break-glass/login", map[string]any{
            "tenant_id": tenantID.String(), "password":"wrong", "totp_code":"000000",
        })
        require.Equal(t, 401, w.Code)
    }
    // 4th attempt: 429 even if credentials correct.
    w := suite.DoJSON("POST", "/admin/break-glass/login", map[string]any{
        "tenant_id": tenantID.String(), "password":"wrong", "totp_code":"000000",
    })
    require.Equal(t, 429, w.Code)
    require.Contains(t, w.Body.String(), "locked")
}
```

- [ ] **Step 5: Write `break_glass_login.go`**

Handler flow (one POST, dual-factor verified together):
1. Compute `ipHash = HMAC(hmacKey, clientIP)`.
2. `repo.IsIPLocked(ctx, ipHash)` → `locked=true` returns 429 early (before any DB lookup) to avoid timing leaks.
3. Bind `breakGlassLoginRequest{TenantID uuid.UUID, Password string, TOTPCode string (len=6)}`. Bind error → record failure + 401.
4. `acc, err := repo.GetByTenant(ctx, tenantID)` — no-account → record failure (reason=`no_account`) + 401.
5. `blob, err := secrets.Fetch(ctx, acc.SecretPath)` — fetch error → record failure (reason=`secret_fetch`) + 500.
6. `pwOK := bcrypt.CompareHashAndPassword([]byte(acc.PasswordHash), []byte(req.Password)) == nil`
7. `totpOK := breakglass.VerifyTOTP(blob.TOTPSecret, req.TOTPCode, time.Now())`
8. `if !(pwOK && totpOK)` → record failure with reason (`password_wrong` / `totp_wrong` / `both_wrong`) + 401.
9. Success path:
   - `setCookie, _ := session.Issue(ctx, tenantID, breakGlassUserID(tenantID), 2*time.Hour)` → write `Set-Cookie`.
   - `repo.UpdateAfterUse(ctx, tenantID)` — sets `rotation_scheduled_at=now()+24h`.
   - `slack.PostLoginAlert(ctx, tenantID, true)`.
   - `audit.EmitLogin(c, tenantID, true, sessionID)`.
   - Response: `{"session_ttl_seconds": 7200}`.

`recordFailure(c, ipHash, tenantID, reason)` — increment IP counter in Redis; on count ≥ 3 within window call `repo.LockIP(ctx, ipHash, &tenantID, reason, 24*time.Hour)`. Also post Slack + emit failed audit event.

- [ ] **Step 6: Mount route — OUTSIDE `RequireActive` group**

```go
// NOT inside the store-scoped subgroup. Break-glass must work even when subscription
// is expired/store_closed/pending_hard_delete — it IS the recovery path.
admin.POST("/break-glass/login", deps.BreakGlass.Login)
```

- [ ] **Step 7: Run tests — expect PASS**

- [ ] **Step 8: Commit**

```bash
git commit -m "feat(break-glass): rate-limited dual-factor login endpoint with 2h session"
```

---

## Task 16: Rotation — post-use 24h hook + 90-day cron

**Files:** `internal/breakglass/{rotation,rotation_test}.go` + `cmd/break-glass-rotation/main.go`

**Spec references:** §12.4 (90d rotation + post-use rotation).

- [ ] **Step 1: Failing test — rotation generates new credentials**

```go
func TestRotation_GeneratesNewCredentials(t *testing.T) {
    tenantID := uuid.New()
    require.NoError(t, bootstrap.Provision(ctx, tenantID))
    old, _ := secrets.Fetch(ctx, secretPath(tenantID))
    require.NoError(t, rotator.RotateOne(ctx, tenantID))
    new, _ := secrets.Fetch(ctx, secretPath(tenantID))
    require.NotEqual(t, old.Password, new.Password)
    require.NotEqual(t, old.TOTPSecret, new.TOTPSecret)
    acc, _ := repo.GetByTenant(ctx, tenantID)
    require.Nil(t, acc.RotationScheduledAt, "scheduled marker cleared after rotation")
}
```

- [ ] **Step 2: Failing test — RotateDue picks up both triggers**

```go
func TestRotation_RotateDue_PicksUpBothTriggers(t *testing.T) {
    a, b, c := uuid.New(), uuid.New(), uuid.New()
    for _, id := range []uuid.UUID{a, b, c} { _ = bootstrap.Provision(ctx, id) }

    // A: post-use rotation due now
    _ = repo.UpdateAfterUse(ctx, a)
    suite.MoveRotationScheduledBackwards(a, -time.Hour)
    // B: last_rotated_at 91d ago
    suite.BackdateLastRotated(b, -91*24*time.Hour)
    // C: healthy, not due

    n, _ := rotator.RotateDue(ctx)
    require.Equal(t, 2, n, "A and B should rotate, C should not")
}
```

- [ ] **Step 3: Write `rotation.go`** — `Rotator{repo, secrets, audit}`. `RotateOne(ctx, tenantID)`:
  1. `repo.GetByTenant(ctx, tenantID)`
  2. Generate new password + TOTP + bcrypt hash.
  3. **Secret Manager FIRST** — `secrets.Upsert(ctx, acc.SecretPath, new Blob)`. If this fails, DB is untouched; next `RotateDue` retries.
  4. Then `repo.ReplaceAfterRotation(ctx, tenantID, string(hash))` — clears `rotation_scheduled_at`.
  5. Emit `break_glass.rotation.success` info audit.

  `RotateDue(ctx)`:
  - `accs, _ := repo.FindDueForRotation(ctx, time.Now().Add(-90*24*time.Hour))`
  - For each, `RotateOne`; on error, emit `break_glass.rotation.failed` error audit with `reason`. Continue loop. Return count rotated successfully.

- [ ] **Step 4: Write `cmd/break-glass-rotation/main.go`** — standard main: load config, connect DB, build `SecretClient` (real GCP), build `Rotator`, call `RotateDue(ctx)` with 15-minute timeout, log `rotated=N`, exit 1 on error. Schedule via Kubernetes CronJob or GCP Cloud Scheduler daily 04:00 UTC.

- [ ] **Step 5: Run tests — expect PASS**

- [ ] **Step 6: Commit**

```bash
git commit -m "feat(break-glass): post-use + 90-day rotation cron with Secret-Manager-first ordering"
```

---

## Task 17: Security regression tests (mandatory)

**Files:** `internal/handlers/admin/security_regression_test.go`

**Objective:** Encode the three mandatory security properties from the plan brief. If any of these tests fail, CI blocks merge.

- [ ] **Step 1: Write regression tests**

```go
//go:build integration

// REGRESSION: break-glass login MUST require BOTH password AND TOTP.
func TestSecurityRegression_BreakGlass_RequiresBothFactors(t *testing.T) {
    tenantID, pw, totpSecret := suite.SeedBreakGlass()
    code, _ := breakglass.TOTPCode(totpSecret, time.Now())

    cases := []struct{ name string; body map[string]any; wantCode int }{
        {"password_only",       map[string]any{"tenant_id": tenantID.String(), "password": pw, "totp_code":""},    401},
        {"totp_only",           map[string]any{"tenant_id": tenantID.String(), "password":"", "totp_code": code}, 401},
        {"wrong_pw_right_totp", map[string]any{"tenant_id": tenantID.String(), "password":"wrong","totp_code":code}, 401},
        {"right_pw_wrong_totp", map[string]any{"tenant_id": tenantID.String(), "password": pw, "totp_code":"000000"}, 401},
        {"both_correct",        map[string]any{"tenant_id": tenantID.String(), "password": pw, "totp_code": code}, 200},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            w := suite.FreshIP().DoJSON("POST", "/admin/break-glass/login", tc.body)
            require.Equal(t, tc.wantCode, w.Code)
        })
    }
}

// REGRESSION: SSO endpoints gated by plangate.RequireFeature(FeatureSSO) — non-Pro = 403.
func TestSecurityRegression_SSO_RejectsNonProPlan(t *testing.T) {
    for _, plan := range []subscription.SubscriptionPlan{
        subscription.PlanTrial, subscription.PlanStarter, subscription.PlanStudio,
    } {
        t.Run(string(plan), func(t *testing.T) {
            tenantID := suite.SeedTenantWithPlan(plan)
            for _, route := range []struct{ m, p string }{
                {"POST",   "/admin/tenants/" + tenantID.String() + "/sso/config"},
                {"GET",    "/admin/tenants/" + tenantID.String() + "/sso/config"},
                {"DELETE", "/admin/tenants/" + tenantID.String() + "/sso/config"},
                {"POST",   "/admin/tenants/" + tenantID.String() + "/sso/test"},
            } {
                w := suite.AsTenant(tenantID).Do(route.m, route.p, nil)
                require.Equal(t, 403, w.Code, "route %s %s must 403 for %s", route.m, route.p, plan)
            }
        })
    }
}

// REGRESSION: SSO config is per-tenant — tenant A's config MUST NOT be readable by tenant B.
func TestSecurityRegression_SSO_PerTenantIsolation(t *testing.T) {
    a := suite.SeedTenantWithPlan(subscription.PlanPro)
    b := suite.SeedTenantWithPlan(subscription.PlanPro)
    suite.SeedSSOConfig(a, sso.ProviderSAML)

    // Tenant B trying to read / upsert / delete A's config → 403.
    require.Equal(t, 403, suite.AsTenant(b).Do("GET",    "/admin/tenants/"+a.String()+"/sso/config", nil).Code)
    require.Equal(t, 403, suite.AsTenant(b).Do("POST",   "/admin/tenants/"+a.String()+"/sso/config", validOIDCBody()).Code)
    require.Equal(t, 403, suite.AsTenant(b).Do("DELETE", "/admin/tenants/"+a.String()+"/sso/config", nil).Code)

    // Sanity: A's config untouched.
    cfg, err := suite.SSORepo.GetByTenant(ctx, a)
    require.NoError(t, err)
    require.Equal(t, sso.ProviderSAML, cfg.Provider)
}

// REGRESSION: break-glass rate limit — 3 failures → 24h lockout.
func TestSecurityRegression_BreakGlass_RateLimitLockout(t *testing.T) {
    tenantID, _, _ := suite.SeedBreakGlass()
    for i := 0; i < 3; i++ {
        require.Equal(t, 401, suite.DoJSON("POST", "/admin/break-glass/login", map[string]any{
            "tenant_id": tenantID.String(), "password":"wrong", "totp_code":"000000",
        }).Code)
    }
    require.Equal(t, 429, suite.DoJSON("POST", "/admin/break-glass/login", map[string]any{
        "tenant_id": tenantID.String(), "password":"wrong", "totp_code":"000000",
    }).Code)
}
```

- [ ] **Step 2: Run — expect PASS**

```bash
go test -tags=integration ./internal/handlers/admin/... -run TestSecurityRegression -v
```

- [ ] **Step 3: Commit**

```bash
git commit -m "test(security): SSO plan-gating + tenant isolation + break-glass dual-factor + rate-limit"
```

---

## Task 18: Final scrub

- [ ] **Step 1: No hardcoded secrets**

```bash
grep -RnE 'password\s*:=\s*"[^"]+"|client_secret\s*:=\s*"[^"]+"' internal/ cmd/ | grep -v "_test.go" || echo "clean"
```

- [ ] **Step 2: No plaintext password persistence**

```bash
grep -RnE 'INSERT.*password|UPDATE.*SET.*password' internal/ | grep -v "password_hash" | grep -v "_test.go" || echo "clean"
```

- [ ] **Step 3: All break-glass event types emitted from real code (not just tests)**

```bash
grep -Rn "break_glass\." internal/breakglass/ internal/handlers/admin/break_glass_login.go
# Expected to appear: break_glass.login.success / .failed / .rotation.success / .rotation.failed / .bootstrap.failed
```

- [ ] **Step 4: Every SSO repo query scopes by tenant_id**

```bash
grep -nE 'Where\(' internal/sso/repository.go | grep -v "tenant_id"
# Expected: zero hits — every Where carries tenant_id.
```

- [ ] **Step 5: RequireFeature(FeatureSSO) present on admin SSO group**

```bash
grep -n "plangate.RequireFeature" internal/handlers/admin/routes.go
# Expected: at least one hit on the /admin/tenants/:tenantId group.
```

- [ ] **Step 6: Break-glass login mounted OUTSIDE store-scoped RequireActive group**

```bash
grep -nB5 "break-glass/login" internal/handlers/admin/routes.go
# Expected: registered on base admin group, NOT inside /stores/:storeId subgroup.
```

- [ ] **Step 7: Full suite green**

```bash
go build ./...
go test -tags=integration ./... -count=1
```

- [ ] **Step 8: Final commit**

```bash
git add -u
git commit --allow-empty -m "chore(p13): scrub verified — no hardcoded secrets, tenant isolation, audit complete"
```

---

## Final verification

- [ ] `go build ./...` clean.
- [ ] `go test -tags=integration ./...` all green.
- [ ] All four migrations (`tenant_sso_configs`, `tenant_sso_user_mappings`, `break_glass_accounts`, `break_glass_lockouts`) reversible.
- [ ] Every `internal/sso/repository.go` method scopes by `tenant_id` (grep confirms).
- [ ] Admin SSO endpoints return **403** for Trial/Starter/Studio plans (`TestSecurityRegression_SSO_RejectsNonProPlan`).
- [ ] Cross-tenant SSO read/write/delete returns **403** (`TestSecurityRegression_SSO_PerTenantIsolation`).
- [ ] Break-glass login requires **both** password AND TOTP — every partial-factor combination fails (`TestSecurityRegression_BreakGlass_RequiresBothFactors`).
- [ ] Break-glass rate limit: 3 failures/hour/IP → 24h lockout (`TestSecurityRegression_BreakGlass_RateLimitLockout`).
- [ ] Break-glass login emits audit event with `severity="critical"` and `ip_hash` (HMAC, not raw IP).
- [ ] Break-glass login posts to Slack `#security-alerts` on both success and failure.
- [ ] Post-use rotation schedules new credentials within 24h.
- [ ] 90-day cron picks up both `last_rotated_at > 90d` AND `rotation_scheduled_at <= now()`.
- [ ] Secret Manager blob written BEFORE DB row updated on rotation.
- [ ] SAML SP uses `github.com/crewjam/saml`; OIDC RP reuses `github.com/coreos/go-oidc/v3`.
- [ ] GIP `providerId` deterministic from `tenant_id` (idempotent re-uploads).
- [ ] Session cookie issued via `authbffclient.SessionIssuer` — marketplace-api does NOT sign cookies.
- [ ] `/admin/break-glass/login` mounted OUTSIDE store-scoped `RequireActive` group (survives read-only subscription states).
- [ ] Plaintext break-glass password lives ONLY in Secret Manager; DB stores bcrypt hash only.
- [ ] Attribute mapping DSL rejects non-`claims.*` roots.
- [ ] JIT provisioner binds to existing user by email when one exists in the tenant.

## What's now unlocked

- **P16** (admin frontend) consumes `/admin/tenants/:tenantId/sso/config` for the SSO settings screen and `/admin/break-glass/login` for emergency recovery; renders the `otpauth://` URI as a QR code on first-time TOTP enrollment.
- **P17** (observability) dashboards + alerts on `sso.login.success`, `sso.login.failed`, `break_glass.login.*`, `break_glass.rotation.*` audit event types.
- **tesserix-infra terraform** (ops task) adds:
  - Google Group `break-glass-responders@mark8ly.com` (≤2 members)
  - IAM binding `roles/secretmanager.secretAccessor` on `projects/tesserix-prod/secrets/break-glass-*` to the group only
  - Kubernetes CronJob `break-glass-rotation` running `cmd/break-glass-rotation` daily 04:00 UTC
  - Secret Manager path `projects/tesserix-prod/secrets/saml-sp-keypair` (shared Mark8ly SP keypair)
  - Slack webhook secret `projects/tesserix-prod/secrets/slack-security-alerts-webhook`
- **Pro+App tenants** inherit the same SSO + break-glass surface — the add-on flag is orthogonal; Task 14's bootstrap fires on any Pro+SSO signup regardless of the app add-on.

## Execution handoff

Plan complete. Saved as `2026-04-18-p13-sso-break-glass.md`.

Execute with **superpowers:subagent-driven-development** (recommended) or **superpowers:executing-plans** AFTER P1 and P3 are merged (hard dependencies). P8's `HMACIPHash` helper is a soft dependency — `internal/breakglass/audit.go` carries a local fallback + TODO comment documenting the swap-point if P8 lands later.
