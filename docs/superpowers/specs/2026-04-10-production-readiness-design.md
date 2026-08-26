# Production Readiness & Security Hardening — Design Spec

**Date:** 2026-04-10
**Status:** Approved

## 1. Overview

Comprehensive production readiness pass covering security hardening, observability, performance optimization, dependency hygiene, and operational tooling. Organized into 4 tiers by priority. All work targets the existing `mark8ly` repo (`marketplace-api` + `apps/admin` + `apps/storefront`). Infrastructure in `tesserix-k8s` is out of scope except for migration automation notes.

### Build order

1. **P0 — Critical Security** (secrets cleanup, encryption, webhooks, headers, CORS)
2. **P1 — Observability** (Prometheus metrics, Sentry error tracking, structured logging audit)
3. **P2 — Performance** (Next.js Image optimization, N+1 query audit, bundle monitoring)
4. **P3 — Dependencies & Tooling** (dependency updates, Dependabot, migration automation, API docs, load testing)

### Constraints

- Same repos (`mark8ly` for app code, `tesserix-k8s` for infra)
- Budget: db-f1-micro shared Postgres, GKE Autopilot
- No new external services (use existing GCP KMS, existing Prometheus/Grafana stack)
- Infra changes (Helm, K8s manifests) tracked as notes for `tesserix-k8s` — not executed here

## 2. P0 — Critical Security

### 2.1 Secrets cleanup — remove committed .env.local

**Problem:** `infra/dev/.env.local` contains real GIP API keys, OAuth secrets, session encryption keys committed to git.

**Fix:**
- Add `infra/dev/.env.local` to `.gitignore`
- Remove from git history: `git filter-repo --path infra/dev/.env.local --invert-paths` (or BFG)
- Rotate ALL keys found in the file:
  - GIP Web API Key (`AIzaSy...`)
  - OAuth Client Secret (`GOCSPX-...`)
  - Session Encrypt Key
- Create `infra/dev/.env.local.example` with placeholder values
- Document in README: "Copy `.env.local.example` to `.env.local` and fill in real values"

### 2.2 API key encryption at rest

**Problem:** `payment_gateway_configs.api_key_encrypted`, `shipping_carrier_configs.api_key_encrypted`, `tax_provider_configs.api_key_encrypted`, `custom_domains.cf_api_token_encrypted` store plaintext despite column names.

**Fix:** AES-256-GCM envelope encryption using GCP KMS.

New package: `internal/crypto/envelope.go`
```go
type Encryptor interface {
    Encrypt(plaintext string) (ciphertext string, error)
    Decrypt(ciphertext string) (plaintext string, error)
}
```

Implementations:
- `KMSEncryptor` — production: uses GCP KMS to wrap/unwrap a DEK, AES-256-GCM for data
- `NoopEncryptor` — local dev: base64 encode/decode (no real encryption, but makes the interface consistent)

Config: `ENCRYPTION_MODE=kms|noop`, `KMS_KEY_RESOURCE_NAME=projects/tesserix-app/locations/asia-south1/keyRings/.../cryptoKeys/...`

Migration: one-time script to encrypt existing plaintext values in-place.

All handlers that read/write `*_encrypted` columns go through the Encryptor.

### 2.3 Webhook scoping by store

**Problem:** Webhook handler queries `payment_gateway_configs WHERE provider = ?` without store_id — first active config wins across all tenants.

**Fix:**
- Change webhook URL pattern: `/api/v1/webhooks/:storeSlug/:provider`
- Handler resolves store from slug, then queries `WHERE store_id = ? AND provider = ?`
- Update provider dashboard instructions (Stripe/Razorpay/PayPal webhook URLs include store slug)

### 2.4 Security headers middleware

**Problem:** No CSP, HSTS, X-Frame-Options, X-Content-Type-Options on any service.

**Fix:** New Gin middleware `internal/middleware/security_headers.go`:
```go
func SecurityHeaders() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Header("X-Content-Type-Options", "nosniff")
        c.Header("X-Frame-Options", "DENY")
        c.Header("X-XSS-Protection", "0") // modern browsers, CSP preferred
        c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
        c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
        c.Header("Content-Security-Policy", buildCSP())
        c.Next()
    }
}
```

CSP policy: `default-src 'self'; script-src 'self' https://js.stripe.com https://checkout.razorpay.com; frame-src https://js.stripe.com https://api.razorpay.com; img-src 'self' https://storage.googleapis.com data:; style-src 'self' 'unsafe-inline'; connect-src 'self' https://api.stripe.com`

Next.js apps: add security headers in `next.config.ts`:
```typescript
headers: async () => [{
  source: '/:path*',
  headers: [
    { key: 'X-Content-Type-Options', value: 'nosniff' },
    { key: 'X-Frame-Options', value: 'DENY' },
    { key: 'Referrer-Policy', value: 'strict-origin-when-cross-origin' },
    { key: 'Strict-Transport-Security', value: 'max-age=31536000; includeSubDomains' },
  ],
}]
```

### 2.5 CORS configuration

**Problem:** No explicit CORS on marketplace-api. Relies on infrastructure-level CORS (Cloudflare/Istio) which may not be configured.

**Fix:** Gin CORS middleware on storefront engine only (admin uses same-origin):
```go
cors.Config{
    AllowOrigins:     []string{"https://*.mark8ly.com"},
    AllowMethods:     []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"},
    AllowHeaders:     []string{"Content-Type", "X-Storefront-Key", "Authorization"},
    AllowCredentials: true,
    MaxAge:           12 * time.Hour,
}
```

Config: `CORS_ALLOWED_ORIGINS` env var for flexibility.

### 2.6 Razorpay webhook secret separation

**Problem:** Razorpay uses the same `secret_key` for API auth and webhook verification.

**Fix:**
- Add `webhook_secret` column to `payment_gateway_configs` (migration 000019)
- Update Razorpay gateway to use `webhook_secret` for `VerifyWebhook`, `secret_key` for API calls
- Admin settings UI: add "Webhook Secret" field for Razorpay config

### 2.7 PII sanitization in logs

**Problem:** Provider error response bodies (potentially containing card data, PII) logged verbatim.

**Fix:** New `internal/providerlog/sanitize.go`:
```go
func SanitizeProviderError(provider string, statusCode int, body []byte) string {
    return fmt.Sprintf("%s: HTTP %d (response body redacted, %d bytes)", provider, statusCode, len(body))
}
```

Apply to all provider HTTP error paths in `stripe.go`, `razorpay.go`, `paypal.go`, `shipengine.go`, `delhivery.go`, `ninjavan.go`, `taxjar.go`.

## 3. P1 — Observability

### 3.1 Prometheus metrics

New package: `internal/metrics/registry.go`

Metrics to expose:
```
# HTTP request metrics (auto via middleware)
http_requests_total{method, path, status}           counter
http_request_duration_seconds{method, path}          histogram

# Business metrics
orders_created_total{store_id}                       counter
checkout_duration_seconds                            histogram
payment_intent_created_total{provider, status}       counter
webhook_received_total{provider, event_type}         counter
tax_calculation_fallback_total{provider}             counter

# System metrics
db_query_duration_seconds{operation}                 histogram
outbox_events_failed_total                           counter
outbox_events_published_total                        counter
```

Gin middleware: `internal/middleware/prometheus.go` — auto-instruments all HTTP routes.

Expose at `/metrics` on a separate port (9090) — not on the main HTTP port.

### 3.2 Sentry error tracking

**Go:** Add `github.com/getsentry/sentry-go` + `github.com/getsentry/sentry-go/gin`
- Init in main.go with `SENTRY_DSN` env var
- Gin middleware captures panics + 5xx responses
- Manual `sentry.CaptureException` for critical business errors

**Next.js:** Add `@sentry/nextjs`
- `sentry.client.config.ts` + `sentry.server.config.ts`
- Auto-captures unhandled errors, React component errors
- Source maps uploaded in CI for readable stack traces

Config: `SENTRY_DSN` (Go), `NEXT_PUBLIC_SENTRY_DSN` (Next.js), `SENTRY_AUTH_TOKEN` (CI for source maps)

### 3.3 Structured logging audit

Verify all Go services use `slog.Logger` consistently:
- No `fmt.Println` or `log.Println` in production paths
- All errors include structured context (`slog.String("order_id", id)`)
- No PII in log messages (customer emails OK in debug, not info)
- Log levels: Error for failures, Warn for degraded, Info for lifecycle, Debug for verbose

Next.js apps: verify no `console.log` in production code (existing hook should catch this).

## 4. P2 — Performance

### 4.1 Next.js Image optimization

**Problem:** Storefront uses raw `<img>` tags — no lazy loading, no responsive srcset, no WebP conversion.

**Fix:** Replace all `<img>` in storefront with `next/image`:
```tsx
<Image
  src={product.media[0].url}
  alt={product.title}
  width={600}
  height={600}
  className="object-cover"
  sizes="(max-width: 640px) 100vw, (max-width: 1024px) 50vw, 33vw"
/>
```

`next.config.ts` remotePatterns:
```typescript
images: {
  remotePatterns: [
    { protocol: 'https', hostname: 'storage.googleapis.com', pathname: '/mark8ly-*/**' },
  ],
}
```

Files to update: `products/page.tsx`, `products/[handle]/page.tsx`, `categories/[slug]/page.tsx`, `cart/page.tsx`, `checkout/page.tsx`, `MediaGallery.tsx`, `FeaturedProducts.tsx`, `ProductDetails.tsx`.

### 4.2 N+1 query audit

Audit all GORM repository methods for N+1 patterns:
- `ListAdmin` in product repository — verify `Preload` for variants, media, categories
- `List` in order repository — already fixed (LoadChildrenBatch)
- Customer list — uses subqueries (OK, not N+1)
- Review list — verify media + replies preloaded

Add GORM query logging in test mode to detect unexpected query counts:
```go
db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Info)})
```

### 4.3 Bundle size monitoring

Add `@next/bundle-analyzer` to admin and storefront:
```bash
ANALYZE=true npm run build
```

CI step: record bundle size per build, warn if >10% increase.

Recharts is the biggest dependency (~200KB gzipped). Verify tree-shaking works (import `LineChart` not `recharts`).

## 5. P3 — Dependencies & Tooling

### 5.1 Dependency updates

**Go:**
```bash
go get -u golang.org/x/crypto golang.org/x/net golang.org/x/sys golang.org/x/text golang.org/x/sync
go mod tidy
```

**Node:**
```bash
npm audit fix
npm update
```

Run full test suite after updates to verify no regressions.

### 5.2 Dependabot configuration

Create `.github/dependabot.yml`:
```yaml
version: 2
updates:
  - package-ecosystem: gomod
    directory: /services/marketplace-api
    schedule:
      interval: weekly
    open-pull-requests-limit: 5
  - package-ecosystem: npm
    directory: /apps/admin
    schedule:
      interval: weekly
    open-pull-requests-limit: 5
  - package-ecosystem: npm
    directory: /apps/storefront
    schedule:
      interval: weekly
    open-pull-requests-limit: 5
  - package-ecosystem: github-actions
    directory: /
    schedule:
      interval: monthly
```

### 5.3 Database migration automation

Document the pattern for `tesserix-k8s`:
- Init container runs `marketplace-api migrate up` before the main container starts
- Schema version assertion on startup prevents running with wrong schema
- Rollback: `marketplace-api migrate down 1` — each migration has a down file

Create `docs/runbooks/database-migrations.md` with:
- How to add a new migration
- How to run migrations locally
- How to run migrations in production (via init container)
- How to rollback
- Schema version check behavior

### 5.4 API documentation

Generate OpenAPI spec from marketplace-api routes:
- Use `swaggo/swag` to generate from Go handler comments
- Or maintain a manual `docs/api/openapi.yaml`
- Serve at `/api/docs` in dev mode (disabled in production)

Minimum: document all public storefront + webhook endpoints. Admin endpoints are internal.

### 5.5 Load testing baseline

Create `scripts/loadtest/` with k6 scripts:
```javascript
// k6 scripts for:
// - Storefront product list (read-heavy)
// - Storefront checkout flow (write-heavy)
// - Admin product CRUD (mixed)
// - Webhook ingestion (burst)
```

Establish baseline: requests/sec, p50/p95/p99 latency, error rate on db-f1-micro.

Document results in `docs/runbooks/performance-baseline.md`.

### 5.6 Secrets rotation runbook

Create `docs/runbooks/secrets-rotation.md`:
- GIP API keys: how to rotate in GCP console + update External Secrets
- Stripe/Razorpay/PayPal: per-merchant rotation via admin settings
- Session encryption key: rolling rotation strategy (accept old + new for 24h)
- KMS key: automatic rotation via GCP KMS policy
- Database credentials: rotate via Cloud SQL + update ESO

## 6. Milestones

| Milestone | Tasks | Scope |
|-----------|-------|-------|
| **P0** | 7 tasks | Secrets cleanup, encryption package + migration, webhook scoping, security headers, CORS, Razorpay webhook secret, PII log sanitization |
| **P1** | 3 tasks | Prometheus metrics + middleware, Sentry integration (Go + Next.js), logging audit |
| **P2** | 3 tasks | Next.js Image migration, N+1 query audit, bundle size monitoring |
| **P3** | 6 tasks | Go dep updates, Node dep updates, Dependabot config, migration runbook, API docs, load testing + secrets runbook |

## 7. Testing

- **P0:** Encryption round-trip test (encrypt → store → read → decrypt). Webhook scoping test (store A webhook doesn't match store B config). Security headers present in responses. CORS preflight handled correctly.
- **P1:** `/metrics` endpoint returns Prometheus format. Sentry captures test exception. No `fmt.Println` in Go production code. No `console.log` in TS production code.
- **P2:** Next.js Image renders with srcset. No N+1 detected in query log for list endpoints. Bundle size under threshold.
- **P3:** `go mod tidy` clean. `npm audit` returns 0 vulnerabilities. Dependabot creates PRs. Migration up + down round-trip works.

## 8. Out of Scope

- Infrastructure changes in `tesserix-k8s` (Helm charts, K8s manifests) — documented as notes only
- WAF rules (Cloudflare handles this)
- DDoS protection (Cloudflare handles this)
- Penetration testing (follow-up with external firm)
- SOC 2 / GDPR compliance audit (follow-up)
- Automated security scanning in CI beyond Trivy (follow-up: Snyk, CodeQL)
