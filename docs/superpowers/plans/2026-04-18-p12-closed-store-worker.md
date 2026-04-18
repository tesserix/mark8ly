# P12 — Cloudflare Worker Closed-Store Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Intercept storefront requests for subscriptions in `store_closed`, `pending_hard_delete`, or `hard_deleted` states at the Cloudflare edge and serve a branded, fully self-contained "store closed" HTML page with **HTTP 200 + `X-Robots-Tag: noindex`** (§5.4) — not a 307 redirect. Past day 150 hard-delete, return 404. Live stores and grace-period expired stores pass through unchanged.

**Architecture:** A new Cloudflare Worker (`storefront-gate`) sits in front of the existing `cloudflared` tunnel for `*.mark8ly.com`. On every request, the Worker extracts the host, checks a per-host KV cache for subscription status + branding (15-min TTL), and either (a) returns a template-rendered `closed.html` for closed states, (b) returns a 404 for `hard_deleted` past day 150, or (c) `fetch`es the origin for live/expired-in-grace stores. Status is sourced from a new internal endpoint `GET /internal/storefront-status/:host` on `marketplace-api`, protected by `gosharedmw.IstioAuth()` internal-tokens and rate-limited. The template is a single file with inlined Paper · Ink · Moss CSS tokens, zero external fetches. Interpolation uses explicit `escapeHTML` per field — no template engine.

**Tech Stack:** Cloudflare Workers (V8 isolate runtime, TypeScript via Wrangler + esbuild), Miniflare for local tests, KV binding for status cache, Go 1.26 + Gin for the internal endpoint.

**Spec:** [`docs/superpowers/specs/2026-04-17-subscription-model-design.md`](../specs/2026-04-17-subscription-model-design.md) — §5.3 (day 90–149 grace / day 150 hard delete), §5.4 (HTTP 200 + noindex), §17.2 (state enum).

**Depends on:** P1 (column shape on `store_subscriptions` — `status`, `plan`, `current_period_end`, `store_id`), P3 (canonical state enum — Worker reads the string values), P11 (cancellation cron that drives stores into `store_closed` / `pending_hard_delete`).

**Related plans:**
- **P11** (cancellation cron) — writes the states this Worker reads
- **P13** (hard-delete job) — flips state to `hard_deleted`; Worker then serves 404
- **P16** (admin UI closed-store preview) — reuses the same template rendered server-side

---

## Scope Check

In scope:
1. New Cloudflare Worker `storefront-gate` under `tesserix-infra/workers/storefront-gate/`.
2. Branded `closed.html` template with 4 interpolation fields: `{{STORE_NAME}}`, `{{LOGO_URL}}`, `{{SUPPORT_EMAIL}}`, `{{RETURN_URL}}` — fully self-contained (inline CSS, no external requests).
3. Worker routing: lookup status → serve closed page / 404 / pass through.
4. HTTP response per §5.4: 200 OK, `X-Robots-Tag: noindex`, `Content-Type: text/html; charset=utf-8`, `Cache-Control: public, max-age=300`.
5. New marketplace-api endpoint `GET /internal/storefront-status/:host` returning `{status, plan, branding:{name, logo_url, support_email}}`.
6. KV cache with 15-min TTL for status lookups (reduces origin load; honours same-day reopen).
7. HTML-escape helper and template interpolator with unit tests.
8. Observability: `Mark8ly-Closed-Store: true` debug header on closed responses; Cloudflare Analytics Engine write on each close-page render.
9. Miniflare integration tests for the Worker; Go integration test for the internal endpoint.
10. Cloudflared configmap update to front the tunnel with the new Worker for `*.mark8ly.com`.

Out of scope:
- Live storefront rendering (existing `marketplace-storefront` Next.js service).
- SEO / robots.txt tweaks beyond the `X-Robots-Tag: noindex` header.
- Admin-side preview of the closed page (P16).
- Customer-facing copy beyond the minimal editorial layout — copy review lands in P16.
- Dunning banners / live-store warnings — those are admin concerns, not storefront edge.

---

## File Structure

### Create

- `tesserix-infra/workers/storefront-gate/wrangler.toml` — Worker config (name, KV bindings, routes, vars).
- `tesserix-infra/workers/storefront-gate/package.json` — scripts: `dev`, `test`, `deploy`.
- `tesserix-infra/workers/storefront-gate/tsconfig.json` — TS config targeting Workers runtime.
- `tesserix-infra/workers/storefront-gate/src/index.ts` — Worker entry (`fetch` handler).
- `tesserix-infra/workers/storefront-gate/src/status.ts` — status lookup + KV cache helpers.
- `tesserix-infra/workers/storefront-gate/src/interpolate.ts` — `escapeHTML` + `render(template, fields)`.
- `tesserix-infra/workers/storefront-gate/src/closed.html` — self-contained HTML template.
- `tesserix-infra/workers/storefront-gate/src/types.ts` — shared types (`StorefrontStatus`, `Branding`, `Env`).
- `tesserix-infra/workers/storefront-gate/test/interpolate.test.ts` — Vitest/Miniflare unit tests.
- `tesserix-infra/workers/storefront-gate/test/worker.test.ts` — Miniflare integration tests.
- `tesserix-infra/workers/storefront-gate/README.md` — deploy notes (allowed because the Worker repo folder is a first-class sub-project, not general docs).
- `services/marketplace-api/internal/handlers/internal/storefront_status.go` — new handler.
- `services/marketplace-api/internal/handlers/internal/storefront_status_test.go` — integration test.

### Modify

- `services/marketplace-api/cmd/marketplace-api/main.go` — mount `/internal/storefront-status/:host` behind internal-auth.
- `services/marketplace-api/internal/handlers/internal/routes.go` (if present; otherwise add route inline in main.go) — register the endpoint.
- `tesserix-infra/k8s/cluster/cloudflared/configmap.yaml` — document that `*.mark8ly.com` is fronted by the `storefront-gate` Worker before the tunnel.

### Delete

- None.

---

## Task Sequence Overview

| # | Task | Depends on |
|---|---|---|
| 1 | Internal endpoint `GET /internal/storefront-status/:host` on marketplace-api | P1, P3 |
| 2 | Worker scaffolding: `wrangler.toml`, `package.json`, `tsconfig`, `types.ts` | — |
| 3 | `closed.html` template + `interpolate.ts` (safe HTML escaping, unit tests) | 2 |
| 4 | Worker `status.ts` — KV cache wrapper around the internal endpoint | 1, 2 |
| 5 | Worker `index.ts` — routing logic + response shaping (§5.4) | 3, 4 |
| 6 | Cloudflared configmap note + deploy docs in README | 5 |

---

## Reusable patterns

**A. HTML escape (allowlist, not blocklist)** — `escapeHTML` replaces exactly 5 chars: `&`, `<`, `>`, `"`, `'`. No `innerHTML`-style injection is possible when every interpolated field is escaped. Separate `escapeAttr` is not needed — the template uses quoted attributes and the same 5-char escape covers both contexts safely.

**B. Self-contained template** — `closed.html` has zero external assets. Fonts fall back to system serif/sans stack (no Source Serif 4 over the wire — the edge must render even if the storefront CDN is down). Paper · Ink · Moss tokens (from `mark8ly/packages/ui/src/styles/mark8ly-tokens.css`) are inlined into a `<style>` block: `--paper-200: #F7F6F2; --ink-900: #0E0E0C; --moss-700: #2D4A2B;`. Logo is the one external reference; if `LOGO_URL` is empty, the template omits the `<img>` and renders the store name in serif display instead.

**C. KV cache shape** — key: `status:${host}`; value: JSON of `{status, plan, branding, cached_at}`; TTL: 900 s (15 min). Read path: `env.STATUS_KV.get(key, "json")`. Write path: `env.STATUS_KV.put(key, JSON.stringify(payload), { expirationTtl: 900 })`. On cache miss, fetch the internal endpoint; on fetch failure, **fail open by passing through to the origin** — a broken edge check must never take a live store down. Log the failure to Analytics Engine so it surfaces on dashboards.

**D. Internal endpoint contract** — `GET /internal/storefront-status/:host` returns:
```json
{
  "status": "store_closed",
  "plan": "starter",
  "branding": {
    "name": "Acme Roasters",
    "logo_url": "https://cdn.mark8ly.com/tenants/acme/logo.png",
    "support_email": "hi@acmeroasters.com"
  },
  "current_period_end": "2026-03-01T00:00:00Z",
  "hard_deleted_at": null
}
```
`host` is the full subdomain (`acme.mark8ly.com`). Resolution: subscription lookup by tenant slug derived from host. If host not found → 404 `{error: "not_found"}` (Worker treats as live-pass-through; unknown host is the storefront's problem, not the edge's).

**E. Worker routing decision tree** — single function `decide(status, hard_deleted_at, now) → "serve_closed" | "serve_404" | "pass_through"`. Pure; easy to unit-test. States:
- `status in {store_closed, pending_hard_delete}` → `serve_closed`
- `status == hard_deleted` AND `now >= hard_deleted_at` → `serve_404`
- `status == expired` → `pass_through` (day 0–13 grace; storefront still live per §5.3 timeline)
- `status == active | trialing | past_due | payment_action_required | cancel_scheduled | signup` → `pass_through`
- unknown status / fetch error → `pass_through` (fail open)

**F. Response shaping** — one helper `closedResponse(html: string): Response` returns:
```
Status: 200
Content-Type: text/html; charset=utf-8
X-Robots-Tag: noindex
Cache-Control: public, max-age=300
Mark8ly-Closed-Store: true
```
`notFoundResponse()` returns 404 with the same `X-Robots-Tag: noindex`.

**G. Internal auth for the Go endpoint** — reuse `gosharedmw.IstioAuth()` (mTLS header check) + an explicit `internal-only` flag in the route group so external Cloudflare-terminated requests get 404. The Worker authenticates using the platform service-token injected as a Worker secret (`env.INTERNAL_API_TOKEN`), sent as `Authorization: Bearer ...`; the Go handler validates it against `INTERNAL_API_TOKENS` env (space-separated allowlist).

---

## Task 1: Internal endpoint `GET /internal/storefront-status/:host`

**Files:**
- Create: `services/marketplace-api/internal/handlers/internal/storefront_status.go`
- Create: `services/marketplace-api/internal/handlers/internal/storefront_status_test.go`
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go`

**Spec references:** §5.4, §17.2.

- [ ] **Step 1: Failing test — happy path returns closed status + branding**

```go
//go:build integration

package internal_test

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "github.com/stretchr/testify/require"

    internalh "github.com/tesserix/marketplace-api/internal/handlers/internal"
    "github.com/tesserix/marketplace-api/internal/subscription"
    "github.com/tesserix/marketplace-api/pkg/testdb"
)

func TestStorefrontStatus_ReturnsClosedStateWithBranding(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions", "stores", "tenants")
    tenantID := uuid.New()
    storeID := uuid.New()
    // Seed: tenant + store + subscription in store_closed
    testdb.SeedTenant(t, db, tenantID, "acme", "Acme Roasters", "hi@acmeroasters.com")
    testdb.SeedStore(t, db, tenantID, storeID, "acme.mark8ly.com", "https://cdn.mark8ly.com/acme/logo.png")
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_x",
        Plan: subscription.PlanStarter, Status: subscription.StatusStoreClosed,
    }).Error)

    r := gin.New()
    r.GET("/internal/storefront-status/:host",
        internalh.NewStorefrontStatusHandler(db).Get)

    req := httptest.NewRequest(http.MethodGet, "/internal/storefront-status/acme.mark8ly.com", nil)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    require.Equal(t, http.StatusOK, w.Code)
    var resp internalh.StorefrontStatusResponse
    require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
    require.Equal(t, "store_closed", resp.Status)
    require.Equal(t, "starter", resp.Plan)
    require.Equal(t, "Acme Roasters", resp.Branding.Name)
    require.Equal(t, "hi@acmeroasters.com", resp.Branding.SupportEmail)
    require.NotEmpty(t, resp.Branding.LogoURL)
}

func TestStorefrontStatus_UnknownHost_Returns404(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions", "stores", "tenants")
    r := gin.New()
    r.GET("/internal/storefront-status/:host",
        internalh.NewStorefrontStatusHandler(db).Get)

    req := httptest.NewRequest(http.MethodGet, "/internal/storefront-status/ghost.mark8ly.com", nil)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    require.Equal(t, http.StatusNotFound, w.Code)
}

func TestStorefrontStatus_LiveStore_ReturnsActive(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions", "stores", "tenants")
    tenantID, storeID := uuid.New(), uuid.New()
    testdb.SeedTenant(t, db, tenantID, "live", "Live Co", "ops@liveco.com")
    testdb.SeedStore(t, db, tenantID, storeID, "live.mark8ly.com", "")
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_y",
        Plan: subscription.PlanStarter, Status: subscription.StatusActive,
    }).Error)

    r := gin.New()
    r.GET("/internal/storefront-status/:host",
        internalh.NewStorefrontStatusHandler(db).Get)

    req := httptest.NewRequest(http.MethodGet, "/internal/storefront-status/live.mark8ly.com", nil)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    require.Equal(t, http.StatusOK, w.Code)
    var resp internalh.StorefrontStatusResponse
    require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
    require.Equal(t, "active", resp.Status)
}
```

- [ ] **Step 2: Run — expect FAIL (package doesn't exist).**

```bash
cd services/marketplace-api
go test -tags=integration ./internal/handlers/internal/... -v -run TestStorefrontStatus
```

- [ ] **Step 3: Write `storefront_status.go`.**

```go
package internal

import (
    "errors"
    "net/http"

    "github.com/gin-gonic/gin"
    "gorm.io/gorm"
)

// StorefrontStatusResponse is the contract consumed by the Cloudflare Worker.
// Keep it flat and small — edge caches it as JSON for 15 minutes.
type StorefrontStatusResponse struct {
    Status           string    `json:"status"`
    Plan             string    `json:"plan"`
    Branding         Branding  `json:"branding"`
    CurrentPeriodEnd *string   `json:"current_period_end,omitempty"`
    HardDeletedAt    *string   `json:"hard_deleted_at,omitempty"`
}

type Branding struct {
    Name         string `json:"name"`
    LogoURL      string `json:"logo_url"`
    SupportEmail string `json:"support_email"`
}

type StorefrontStatusHandler struct {
    db *gorm.DB
}

func NewStorefrontStatusHandler(db *gorm.DB) *StorefrontStatusHandler {
    return &StorefrontStatusHandler{db: db}
}

// Get answers GET /internal/storefront-status/:host.
// Resolves host → tenant (via stores.custom_domain OR stores.default_host) →
// subscription + branding. Returns 404 if the host is not recognised.
func (h *StorefrontStatusHandler) Get(c *gin.Context) {
    host := c.Param("host")
    if host == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "missing_host"})
        return
    }

    var row struct {
        Status           string
        Plan             string
        Name             string
        LogoURL          string
        SupportEmail     string
        CurrentPeriodEnd *string
        HardDeletedAt    *string
    }
    err := h.db.Raw(`
        SELECT ss.status, ss.plan,
               t.name, s.logo_url, t.support_email,
               ss.current_period_end::text AS current_period_end,
               ss.hard_deleted_at::text   AS hard_deleted_at
        FROM stores s
        JOIN tenants t              ON t.id = s.tenant_id
        JOIN store_subscriptions ss ON ss.store_id = s.id
        WHERE s.custom_domain = ? OR s.default_host = ?
        LIMIT 1`, host, host).Scan(&row).Error
    if errors.Is(err, gorm.ErrRecordNotFound) {
        c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
        return
    }
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup_failed"})
        return
    }
    if row.Status == "" { // Scan succeeds with 0 rows on Raw()
        c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
        return
    }

    c.JSON(http.StatusOK, StorefrontStatusResponse{
        Status: row.Status,
        Plan:   row.Plan,
        Branding: Branding{
            Name: row.Name, LogoURL: row.LogoURL, SupportEmail: row.SupportEmail,
        },
        CurrentPeriodEnd: row.CurrentPeriodEnd,
        HardDeletedAt:    row.HardDeletedAt,
    })
}
```

- [ ] **Step 4: Wire into `cmd/marketplace-api/main.go` behind internal-auth.**

```go
// inside router setup, alongside other /internal mounts
internalGrp := r.Group("/internal", gosharedmw.IstioAuth(istioCfg), gosharedmw.InternalTokenAuth(internalTokens))
internalGrp.GET("/storefront-status/:host",
    internalh.NewStorefrontStatusHandler(db).Get)
```

`InternalTokenAuth` is the existing middleware in `go-shared` that validates `Authorization: Bearer <token>` against `INTERNAL_API_TOKENS`. If `go-shared` doesn't export it yet, add a local 10-line middleware in `services/marketplace-api/internal/middleware/internal_token.go` and promote to `go-shared` in a follow-up.

- [ ] **Step 5: Run tests — expect PASS.**

- [ ] **Step 6: Commit.**

```bash
git add services/marketplace-api/internal/handlers/internal/storefront_status{,_test}.go \
        services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "feat(marketplace-api): internal storefront-status endpoint for edge worker"
```

---

## Task 2: Worker scaffolding

**Files:**
- Create: `tesserix-infra/workers/storefront-gate/wrangler.toml`
- Create: `tesserix-infra/workers/storefront-gate/package.json`
- Create: `tesserix-infra/workers/storefront-gate/tsconfig.json`
- Create: `tesserix-infra/workers/storefront-gate/src/types.ts`

- [ ] **Step 1: `wrangler.toml` — name, routes, KV binding, vars.**

```toml
name = "storefront-gate"
main = "src/index.ts"
compatibility_date = "2026-04-15"
compatibility_flags = ["nodejs_compat"]

# Routes: all storefront subdomains. Apex (mark8ly.com) is handled by a
# separate marketing Worker and is NOT included here.
routes = [
  { pattern = "*.mark8ly.com/*", zone_name = "mark8ly.com" },
]

[[kv_namespaces]]
binding = "STATUS_KV"
id = "REPLACE_AT_DEPLOY_TIME"
preview_id = "REPLACE_AT_DEPLOY_TIME"

[vars]
INTERNAL_API_BASE = "https://marketplace-api.internal.mark8ly.com"
STATUS_CACHE_TTL_SECONDS = "900"

# Analytics Engine binding for close-page render counts.
[[analytics_engine_datasets]]
binding = "CLOSED_PAGE_ANALYTICS"
dataset = "storefront_closed_events"
```

- [ ] **Step 2: `package.json` — minimal.**

```json
{
  "name": "storefront-gate",
  "version": "0.1.0",
  "private": true,
  "scripts": {
    "dev":    "wrangler dev --local",
    "test":   "vitest run",
    "deploy": "wrangler deploy"
  },
  "devDependencies": {
    "@cloudflare/workers-types": "^4.20260415.0",
    "@cloudflare/vitest-pool-workers": "^0.5.0",
    "typescript": "^5.6.0",
    "vitest": "^2.1.0",
    "wrangler": "^3.80.0"
  }
}
```

- [ ] **Step 3: `tsconfig.json`.**

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ES2022",
    "moduleResolution": "bundler",
    "lib": ["ES2022"],
    "types": ["@cloudflare/workers-types"],
    "strict": true,
    "noUncheckedIndexedAccess": true,
    "esModuleInterop": true,
    "skipLibCheck": true
  },
  "include": ["src/**/*", "test/**/*"]
}
```

- [ ] **Step 4: `src/types.ts`.**

```ts
export type StorefrontStatus =
  | "signup"
  | "trialing"
  | "active"
  | "past_due"
  | "payment_action_required"
  | "cancel_scheduled"
  | "expired"
  | "store_closed"
  | "pending_hard_delete"
  | "hard_deleted";

export interface Branding {
  name: string;
  logo_url: string;
  support_email: string;
}

export interface StorefrontStatusPayload {
  status: StorefrontStatus;
  plan: string;
  branding: Branding;
  current_period_end?: string | null;
  hard_deleted_at?: string | null;
}

export interface Env {
  STATUS_KV: KVNamespace;
  INTERNAL_API_BASE: string;
  INTERNAL_API_TOKEN: string; // injected as secret
  STATUS_CACHE_TTL_SECONDS: string;
  CLOSED_PAGE_ANALYTICS: AnalyticsEngineDataset;
}
```

- [ ] **Step 5: Install + verify tooling.**

```bash
cd tesserix-infra/workers/storefront-gate
npm install
npx tsc --noEmit
```

- [ ] **Step 6: Commit.**

```bash
git add tesserix-infra/workers/storefront-gate/{wrangler.toml,package.json,package-lock.json,tsconfig.json,src/types.ts}
git commit -m "feat(workers): scaffold storefront-gate Worker"
```

---

## Task 3: `closed.html` template + `interpolate.ts`

**Files:**
- Create: `tesserix-infra/workers/storefront-gate/src/closed.html`
- Create: `tesserix-infra/workers/storefront-gate/src/interpolate.ts`
- Create: `tesserix-infra/workers/storefront-gate/test/interpolate.test.ts`

- [ ] **Step 1: Write failing tests — escape + render.**

```ts
// test/interpolate.test.ts
import { describe, it, expect } from "vitest";
import { escapeHTML, render } from "../src/interpolate";

describe("escapeHTML", () => {
  it("escapes the five HTML-sensitive characters", () => {
    expect(escapeHTML(`<script>alert("x&y'z")</script>`))
      .toBe("&lt;script&gt;alert(&quot;x&amp;y&#39;z&quot;)&lt;/script&gt;");
  });

  it("leaves safe text untouched", () => {
    expect(escapeHTML("Acme Roasters · Specialty coffee")).toBe("Acme Roasters · Specialty coffee");
  });

  it("handles empty string", () => {
    expect(escapeHTML("")).toBe("");
  });
});

describe("render", () => {
  const template = `<h1>{{STORE_NAME}}</h1><a href="mailto:{{SUPPORT_EMAIL}}">Contact</a>`;

  it("interpolates escaped fields", () => {
    const out = render(template, {
      STORE_NAME: `<b>Acme</b>`,
      SUPPORT_EMAIL: `x"@y.com`,
    });
    expect(out).toContain("&lt;b&gt;Acme&lt;/b&gt;");
    expect(out).toContain(`mailto:x&quot;@y.com`);
  });

  it("replaces missing fields with empty string", () => {
    const out = render(template, {});
    expect(out).toContain("<h1></h1>");
    expect(out).toContain(`mailto:`);
  });

  it("ignores unknown placeholders (leaves them literal)", () => {
    const out = render(`{{UNKNOWN}} {{STORE_NAME}}`, { STORE_NAME: "X" });
    expect(out).toBe("{{UNKNOWN}} X");
  });
});
```

- [ ] **Step 2: Run — expect FAIL.**

```bash
cd tesserix-infra/workers/storefront-gate
npm test
```

- [ ] **Step 3: Write `src/interpolate.ts`.**

```ts
// Allowlist-based HTML escape. Safe for both element content and quoted
// attributes. We never interpolate into <script>, <style>, or unquoted attrs
// so these five replacements are sufficient.
const ESCAPE_MAP: Record<string, string> = {
  "&": "&amp;",
  "<": "&lt;",
  ">": "&gt;",
  '"': "&quot;",
  "'": "&#39;",
};

export function escapeHTML(input: string): string {
  return input.replace(/[&<>"']/g, (ch) => ESCAPE_MAP[ch] ?? ch);
}

// Explicit allowlist of supported placeholders — unknown ones are left as
// literal {{FOO}} so a typo fails loudly in review rather than rendering blank.
const ALLOWED_KEYS = new Set([
  "STORE_NAME",
  "LOGO_URL",
  "SUPPORT_EMAIL",
  "RETURN_URL",
]);

export function render(template: string, fields: Record<string, string>): string {
  return template.replace(/\{\{([A-Z_]+)\}\}/g, (match, key: string) => {
    if (!ALLOWED_KEYS.has(key)) return match;
    return escapeHTML(fields[key] ?? "");
  });
}
```

- [ ] **Step 4: Write `src/closed.html`.** Zero external fetches; system font fallback; inlined Paper · Ink · Moss tokens; asymmetric left-aligned layout (no centered hero, per project design context).

```html
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<title>{{STORE_NAME}} · Store closed</title>
<meta name="robots" content="noindex" />
<style>
  :root {
    --paper-200: #F7F6F2;
    --ink-900:   #0E0E0C;
    --ink-600:   #4A4A46;
    --moss-700:  #2D4A2B;
    --rule:      rgba(14, 14, 12, 0.12);
  }
  *, *::before, *::after { box-sizing: border-box; }
  html, body { margin: 0; padding: 0; background: var(--paper-200); color: var(--ink-900); }
  body {
    font-family: ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, sans-serif;
    font-size: 16px; line-height: 1.5; min-height: 100vh;
    display: grid; grid-template-rows: auto 1fr auto;
  }
  header, main, footer { padding: 2rem clamp(1.5rem, 6vw, 6rem); }
  header { border-bottom: 1px solid var(--rule); }
  .brand { display: flex; align-items: center; gap: 0.875rem; }
  .brand img { height: 32px; width: auto; display: block; }
  .brand-name {
    font-family: ui-serif, Georgia, "Times New Roman", serif;
    font-size: 1.125rem; letter-spacing: 0.01em;
  }
  main { max-width: 48rem; }
  h1 {
    font-family: ui-serif, Georgia, "Times New Roman", serif;
    font-weight: 400;
    font-size: clamp(2rem, 4.5vw, 3.25rem);
    line-height: 1.1; margin: 0 0 1.5rem 0;
    letter-spacing: -0.01em;
  }
  p { color: var(--ink-600); margin: 0 0 1rem 0; max-width: 36rem; }
  a { color: var(--moss-700); text-decoration: underline; text-underline-offset: 3px; }
  a:focus-visible { outline: 2px solid var(--moss-700); outline-offset: 3px; }
  hr { border: 0; border-top: 1px solid var(--rule); margin: 2.5rem 0; }
  footer { color: var(--ink-600); font-size: 0.875rem; border-top: 1px solid var(--rule); }
  @media (prefers-reduced-motion: no-preference) {
    main { animation: fade 420ms ease-out both; }
    @keyframes fade { from { opacity: 0; transform: translateY(4px); } to { opacity: 1; transform: none; } }
  }
</style>
</head>
<body>
  <header>
    <div class="brand">
      <img src="{{LOGO_URL}}" alt="" onerror="this.style.display='none'" />
      <span class="brand-name">{{STORE_NAME}}</span>
    </div>
  </header>
  <main>
    <h1>This store is currently closed.</h1>
    <p>{{STORE_NAME}} is temporarily unavailable. If you believe this is a mistake, please reach out to the store directly.</p>
    <p><a href="mailto:{{SUPPORT_EMAIL}}">{{SUPPORT_EMAIL}}</a></p>
    <hr />
    <p>Looking for a different shop? Visit <a href="{{RETURN_URL}}">Mark8ly</a>.</p>
  </main>
  <footer>
    Powered by Mark8ly
  </footer>
</body>
</html>
```

- [ ] **Step 5: Run tests — expect PASS.**

- [ ] **Step 6: Commit.**

```bash
git add tesserix-infra/workers/storefront-gate/src/{closed.html,interpolate.ts} \
        tesserix-infra/workers/storefront-gate/test/interpolate.test.ts
git commit -m "feat(workers): closed-store template + safe HTML interpolation"
```

---

## Task 4: `status.ts` — KV cache wrapper around the internal endpoint

**Files:**
- Create: `tesserix-infra/workers/storefront-gate/src/status.ts`

- [ ] **Step 1: Write failing test — cache-miss then cache-hit.**

```ts
// test/status.test.ts
import { describe, it, expect, vi } from "vitest";
import { env, createExecutionContext } from "cloudflare:test";
import { fetchStorefrontStatus } from "../src/status";

describe("fetchStorefrontStatus", () => {
  it("hits origin on cache miss, writes KV, returns payload", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(JSON.stringify({
        status: "store_closed", plan: "starter",
        branding: { name: "Acme", logo_url: "", support_email: "x@y.com" },
      }), { status: 200 })
    );

    const out = await fetchStorefrontStatus(env, "acme.mark8ly.com");
    expect(out?.status).toBe("store_closed");
    expect(fetchSpy).toHaveBeenCalledOnce();

    // Second call — served from KV.
    const out2 = await fetchStorefrontStatus(env, "acme.mark8ly.com");
    expect(out2?.status).toBe("store_closed");
    expect(fetchSpy).toHaveBeenCalledOnce(); // still 1
  });

  it("returns null on origin 404 (unknown host)", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(null, { status: 404 })
    );
    const out = await fetchStorefrontStatus(env, "ghost.mark8ly.com");
    expect(out).toBeNull();
  });

  it("fails open (returns null) on origin 5xx", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(null, { status: 503 })
    );
    const out = await fetchStorefrontStatus(env, "flaky.mark8ly.com");
    expect(out).toBeNull();
  });
});
```

- [ ] **Step 2: Run — expect FAIL.**

- [ ] **Step 3: Write `src/status.ts`.**

```ts
import type { Env, StorefrontStatusPayload } from "./types";

const KEY_PREFIX = "status:";

export async function fetchStorefrontStatus(
  env: Env,
  host: string,
): Promise<StorefrontStatusPayload | null> {
  const key = `${KEY_PREFIX}${host}`;
  const cached = await env.STATUS_KV.get<StorefrontStatusPayload>(key, "json");
  if (cached) return cached;

  const url = `${env.INTERNAL_API_BASE}/internal/storefront-status/${encodeURIComponent(host)}`;
  let res: Response;
  try {
    res = await fetch(url, {
      headers: { Authorization: `Bearer ${env.INTERNAL_API_TOKEN}` },
      cf: { cacheTtl: 0 },
    });
  } catch {
    // Fail open — never take a live store down because the edge check errored.
    return null;
  }

  if (res.status === 404) return null;
  if (!res.ok) return null;

  const payload = (await res.json()) as StorefrontStatusPayload;
  const ttl = Number.parseInt(env.STATUS_CACHE_TTL_SECONDS || "900", 10);
  // KV writes must not block the response — fire-and-forget.
  env.STATUS_KV.put(key, JSON.stringify(payload), { expirationTtl: ttl })
    .catch(() => { /* swallow — KV is best-effort */ });
  return payload;
}

// Exposed for same-day-reopen: P11's reactivation path can punch the cache
// by calling an internal admin-only invalidation endpoint (out of scope here;
// tracked in P11 follow-up).
export function cacheKey(host: string): string {
  return `${KEY_PREFIX}${host}`;
}
```

- [ ] **Step 4: Run tests — expect PASS.**

- [ ] **Step 5: Commit.**

```bash
git add tesserix-infra/workers/storefront-gate/src/status.ts \
        tesserix-infra/workers/storefront-gate/test/status.test.ts
git commit -m "feat(workers): KV-cached storefront-status lookup with fail-open"
```

---

## Task 5: `index.ts` — routing logic + response shaping

**Files:**
- Create: `tesserix-infra/workers/storefront-gate/src/index.ts`
- Create: `tesserix-infra/workers/storefront-gate/test/worker.test.ts`

- [ ] **Step 1: Write failing integration tests — all three routing branches.**

```ts
// test/worker.test.ts
import { describe, it, expect, vi, beforeEach } from "vitest";
import { env, SELF } from "cloudflare:test";

// Helper: mock the internal endpoint response for a given host.
function mockStatus(payload: unknown, status = 200) {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const url = typeof input === "string" ? input : (input as Request).url;
    if (url.includes("/internal/storefront-status/")) {
      return new Response(payload ? JSON.stringify(payload) : null, { status });
    }
    // Origin passthrough — return a sentinel.
    return new Response("ORIGIN", { status: 200 });
  });
}

beforeEach(() => { vi.restoreAllMocks(); });

describe("storefront-gate Worker", () => {
  it("serves closed.html with 200 + noindex for store_closed", async () => {
    mockStatus({
      status: "store_closed", plan: "starter",
      branding: { name: "Acme", logo_url: "", support_email: "x@y.com" },
    });
    const res = await SELF.fetch("https://acme.mark8ly.com/");
    expect(res.status).toBe(200);
    expect(res.headers.get("X-Robots-Tag")).toBe("noindex");
    expect(res.headers.get("Content-Type")).toContain("text/html");
    expect(res.headers.get("Mark8ly-Closed-Store")).toBe("true");
    expect(res.headers.get("Cache-Control")).toBe("public, max-age=300");
    const body = await res.text();
    expect(body).toContain("Acme");
    expect(body).toContain("This store is currently closed.");
  });

  it("serves closed.html for pending_hard_delete", async () => {
    mockStatus({
      status: "pending_hard_delete", plan: "starter",
      branding: { name: "Zephyr", logo_url: "", support_email: "z@y.com" },
    });
    const res = await SELF.fetch("https://zephyr.mark8ly.com/");
    expect(res.status).toBe(200);
    expect(res.headers.get("Mark8ly-Closed-Store")).toBe("true");
  });

  it("serves 404 for hard_deleted past day 150", async () => {
    const past = new Date(Date.now() - 24 * 3600_000).toISOString();
    mockStatus({
      status: "hard_deleted", plan: "starter",
      branding: { name: "Gone", logo_url: "", support_email: "" },
      hard_deleted_at: past,
    });
    const res = await SELF.fetch("https://gone.mark8ly.com/");
    expect(res.status).toBe(404);
    expect(res.headers.get("X-Robots-Tag")).toBe("noindex");
  });

  it("passes through to origin for expired (grace window)", async () => {
    mockStatus({
      status: "expired", plan: "starter",
      branding: { name: "Grace", logo_url: "", support_email: "g@y.com" },
    });
    const res = await SELF.fetch("https://grace.mark8ly.com/products/widget");
    expect(res.status).toBe(200);
    expect(await res.text()).toBe("ORIGIN");
    expect(res.headers.get("Mark8ly-Closed-Store")).toBeNull();
  });

  it("passes through for active stores", async () => {
    mockStatus({
      status: "active", plan: "starter",
      branding: { name: "Live", logo_url: "", support_email: "l@y.com" },
    });
    const res = await SELF.fetch("https://live.mark8ly.com/");
    expect(res.status).toBe(200);
    expect(await res.text()).toBe("ORIGIN");
  });

  it("fails open on origin 5xx (passes through)", async () => {
    mockStatus(null, 503);
    const res = await SELF.fetch("https://flaky.mark8ly.com/");
    expect(res.status).toBe(200);
    expect(await res.text()).toBe("ORIGIN");
  });

  it("escapes store name in rendered HTML", async () => {
    mockStatus({
      status: "store_closed", plan: "starter",
      branding: { name: `<script>alert(1)</script>`, logo_url: "", support_email: "x@y.com" },
    });
    const res = await SELF.fetch("https://xss.mark8ly.com/");
    const body = await res.text();
    expect(body).not.toContain("<script>alert(1)</script>");
    expect(body).toContain("&lt;script&gt;alert(1)&lt;/script&gt;");
  });
});
```

- [ ] **Step 2: Run — expect FAIL.**

- [ ] **Step 3: Write `src/index.ts`.**

```ts
import type { Env, StorefrontStatusPayload, StorefrontStatus } from "./types";
import { fetchStorefrontStatus } from "./status";
import { render } from "./interpolate";
import CLOSED_HTML from "./closed.html";

const RETURN_URL = "https://mark8ly.com";
const CLOSED_STATES: StorefrontStatus[] = ["store_closed", "pending_hard_delete"];

type Decision = "serve_closed" | "serve_404" | "pass_through";

export function decide(
  payload: StorefrontStatusPayload | null,
  now: Date,
): Decision {
  if (!payload) return "pass_through"; // unknown host / fetch error → fail open
  if (payload.status === "hard_deleted") {
    if (payload.hard_deleted_at && new Date(payload.hard_deleted_at) <= now) {
      return "serve_404";
    }
    // Row marked hard_deleted but timestamp missing/future → treat as closed
    // rather than leaking 404 early. Should not happen per P13 contract.
    return "serve_closed";
  }
  if (CLOSED_STATES.includes(payload.status)) return "serve_closed";
  return "pass_through";
}

function closedResponse(html: string): Response {
  return new Response(html, {
    status: 200,
    headers: {
      "Content-Type": "text/html; charset=utf-8",
      "X-Robots-Tag": "noindex",
      "Cache-Control": "public, max-age=300",
      "Mark8ly-Closed-Store": "true",
    },
  });
}

function notFoundResponse(): Response {
  return new Response("Not Found", {
    status: 404,
    headers: {
      "Content-Type": "text/plain; charset=utf-8",
      "X-Robots-Tag": "noindex",
      "Cache-Control": "public, max-age=300",
    },
  });
}

function renderClosed(payload: StorefrontStatusPayload): string {
  return render(CLOSED_HTML, {
    STORE_NAME:    payload.branding.name,
    LOGO_URL:      payload.branding.logo_url,
    SUPPORT_EMAIL: payload.branding.support_email,
    RETURN_URL,
  });
}

function logClosedRender(env: Env, host: string, status: StorefrontStatus): void {
  try {
    env.CLOSED_PAGE_ANALYTICS?.writeDataPoint({
      blobs: [host, status],
      doubles: [1],
      indexes: [host],
    });
  } catch { /* analytics is best-effort */ }
}

export default {
  async fetch(request: Request, env: Env, _ctx: ExecutionContext): Promise<Response> {
    const url = new URL(request.url);
    const host = url.hostname;

    const payload = await fetchStorefrontStatus(env, host);
    const decision = decide(payload, new Date());

    if (decision === "serve_404") {
      logClosedRender(env, host, "hard_deleted");
      return notFoundResponse();
    }
    if (decision === "serve_closed" && payload) {
      logClosedRender(env, host, payload.status);
      return closedResponse(renderClosed(payload));
    }
    // pass_through: forward to origin unchanged.
    return fetch(request);
  },
};
```

- [ ] **Step 4: Wire `closed.html` as a module import.** Add to `wrangler.toml`:

```toml
rules = [
  { type = "Text", globs = ["**/*.html"], fallthrough = true }
]
```

- [ ] **Step 5: Run tests — expect PASS.**

- [ ] **Step 6: Commit.**

```bash
git add tesserix-infra/workers/storefront-gate/src/index.ts \
        tesserix-infra/workers/storefront-gate/test/worker.test.ts \
        tesserix-infra/workers/storefront-gate/wrangler.toml
git commit -m "feat(workers): storefront-gate routing with closed-page rendering + analytics"
```

---

## Task 6: Cloudflared configmap note + deploy README

**Files:**
- Modify: `tesserix-infra/k8s/cluster/cloudflared/configmap.yaml`
- Create: `tesserix-infra/workers/storefront-gate/README.md`

- [ ] **Step 1: Add a comment header to the cloudflared configmap** explaining that `*.mark8ly.com` traffic is intercepted by the `storefront-gate` Worker before hitting the tunnel. No functional change to the tunnel routes — the Worker either serves its own response or `fetch()`es the origin, which resolves via the existing Istio gateway.

```yaml
# tesserix-infra/k8s/cluster/cloudflared/configmap.yaml
# ...existing config...
# NOTE (P12, 2026-04-18): `*.mark8ly.com` requests are fronted by the
# `storefront-gate` Cloudflare Worker (tesserix-infra/workers/storefront-gate/).
# The Worker short-circuits closed / hard-deleted stores with an HTTP 200 + noindex
# page (or 404 past day 150). Live stores and expired-in-grace stores are
# forwarded to the origin via `fetch(request)`, which hits this tunnel
# unchanged. See docs/superpowers/specs/2026-04-17-subscription-model-design.md §5.4.
```

- [ ] **Step 2: Write a short README** — deploy-only notes, not user docs.

````markdown
# storefront-gate Cloudflare Worker

Intercepts `*.mark8ly.com` and serves a branded "store closed" page for
subscriptions in `store_closed` / `pending_hard_delete`, a 404 for
`hard_deleted` past day 150, and passes through for everyone else.

## Local dev
```bash
npm install
npm run dev     # wrangler dev --local
npm test        # vitest
```

## Deploy
```bash
# Set secrets (one-time, per environment):
wrangler secret put INTERNAL_API_TOKEN

# Create the KV namespace and paste the ID into wrangler.toml:
wrangler kv:namespace create STATUS_KV
wrangler kv:namespace create STATUS_KV --preview

npm run deploy
```

## Cache invalidation (same-day reopen)
KV entries live for 15 minutes. When P11's reactivation path moves a store
back to `active`, it must also `DELETE` the cache key `status:<host>` via
the internal admin endpoint (tracked as P11 follow-up; not gated on this PR).
````

- [ ] **Step 3: Commit.**

```bash
git add tesserix-infra/k8s/cluster/cloudflared/configmap.yaml \
        tesserix-infra/workers/storefront-gate/README.md
git commit -m "docs(workers): document storefront-gate deploy + cloudflared interaction"
```

---

## Final verification

- [ ] `cd services/marketplace-api && go test -tags=integration ./internal/handlers/internal/... -run TestStorefrontStatus` — all PASS.
- [ ] `cd tesserix-infra/workers/storefront-gate && npm test` — all PASS (interpolate, status, worker).
- [ ] `npx tsc --noEmit` in the Worker directory — no type errors.
- [ ] Manual smoke via `wrangler dev --local`: `curl -H "Host: acme.mark8ly.com" http://127.0.0.1:8787/` with a locally-seeded `store_closed` subscription returns 200 + `X-Robots-Tag: noindex` + the escaped brand name in the HTML.
- [ ] Spec §5.4 requirements audited in code:
  - HTTP 200 OK (not 307) ✓
  - `X-Robots-Tag: noindex` ✓
  - `Content-Type: text/html; charset=utf-8` ✓
  - `Cache-Control: public, max-age=300` ✓
  - Branded (store name, logo, support email) ✓
  - Self-contained (zero external resources except optional logo) ✓
  - Paper · Ink · Moss tokens inlined ✓
- [ ] §5.3 timeline requirements audited:
  - `expired` (day 0–13 post-expiry) → pass-through ✓
  - `store_closed` → closed page ✓
  - `pending_hard_delete` → closed page ✓
  - `hard_deleted` past `hard_deleted_at` → 404 ✓
- [ ] No direct `UPDATE store_subscriptions` — this plan only reads.
- [ ] No secrets committed — `INTERNAL_API_TOKEN` is a Wrangler secret, not in `wrangler.toml`.

---

## What's unlocked

- **P13** (hard-delete job) can flip state to `hard_deleted` knowing the edge will start returning 404 after `hard_deleted_at`.
- **P11** (cancellation cron) — its transitions into `store_closed` / `pending_hard_delete` now have visible customer-facing effect at the edge.
- **P16** (admin closed-store preview) can reuse `closed.html` and the `render()` helper; the template becomes the canonical closed-page artifact.
- **P6** (dunning + `payment_action_required`) — confirmed by design that `payment_action_required` stores remain in pass-through; storefront stays live while merchant completes 3DS.

---

## Execution handoff

When executing this plan:
1. Start with Task 1 (Go endpoint) — Worker tests need a contract to code against.
2. Tasks 2–5 are Worker-only and can be done by a single agent in one session; no cross-repo coordination.
3. Task 6 is docs-only and can ship in the same PR as Task 5.
4. Do **not** deploy the Worker to production until P3 is merged (state enum must be stable) and P11 is at least in preview (so there are real `store_closed` rows to test against). In dev, seed `store_closed` manually via a SQL fixture.
5. Cache TTL is 15 min by default — document in the P11 reactivation path that the cache must be punched on reopen (tracked as a P11 follow-up; not a blocker for this PR).
6. Fail-open is a deliberate design choice: a broken edge check must never take a live store down. Every failure mode (KV miss, fetch error, 5xx origin, JSON parse error) falls through to `fetch(request)`.
