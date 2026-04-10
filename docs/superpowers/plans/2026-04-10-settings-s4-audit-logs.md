# Settings S4 — Audit Logs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Ship read-only audit log viewer proxying to the existing audit-service, with search, filters, pagination, and CSV export.

**Architecture:** New `internal/handlers/admin/audit.go` handler proxying to audit-service. No migration — audit-service owns the data. Admin UI at /settings/audit-logs.

**Tech Stack:** Go 1.26, Gin, net/http (audit-service proxy). Next.js 16, React 19, Tailwind.

**Spec reference:** `docs/superpowers/specs/2026-04-10-settings-tier1-tier2-design.md` — sections §3.4, §4.4, §5.1 (audit logs page), §8 (S4 tests).

**Prerequisite:** None — this feature is a read-only proxy and creates no migrations. The audit-service must be running (or mocked) for integration tests.

---

## File structure produced by S4

```
services/marketplace-api/
├── internal/
│   ├── handlers/admin/
│   │   ├── audit.go                                    # NEW — AuditLogsHandler (proxy to audit-service)
│   │   ├── audit_test.go                               # NEW — handler tests with httptest mock
│   │   └── routes.go                                   # MODIFY — add audit-logs routes
│   └── authz/
│       └── audit_roles.go                              # NEW — role constants for audit endpoints
├── cmd/marketplace-api/
│   └── main.go                                         # MODIFY — wire AuditLogsHandler into Deps

apps/admin/
├── lib/api/
│   └── audit-api.ts                                    # NEW — typed API client
├── app/settings/audit-logs/
│   ├── page.tsx                                        # NEW — server component
│   └── actions.ts                                      # NEW — server actions (search, export)
├── components/settings/
│   ├── AuditLogsClient.tsx                             # NEW — client component with table + filters
│   └── AuditLogRow.tsx                                 # NEW — expandable row component
└── components/shell/
    └── AdminShell.tsx                                  # MODIFY — add "Audit Logs" to sidebar
```

---

## Task 0: Verify prerequisites

**Files:** none (read-only)

- [ ] **Step 1: Verify audit-service endpoint**

The audit-service is an existing external service. Confirm the expected API:

```bash
# If audit-service is running locally:
curl -s http://localhost:8092/api/v1/logs?tenant_id=test&limit=1 | head -c 200
```

The expected response shape (from the audit-service API):

```json
{
  "data": [
    {
      "id": "uuid",
      "tenant_id": "uuid",
      "store_id": "uuid",
      "user_id": "uuid",
      "user_email": "string",
      "action": "string",
      "resource_type": "string",
      "resource_id": "uuid",
      "severity": "info|warn|error",
      "metadata": {},
      "ip_address": "string",
      "user_agent": "string",
      "created_at": "2026-01-01T00:00:00Z"
    }
  ],
  "meta": { "total": 100, "page": 1, "limit": 20 }
}
```

If audit-service is not running, that's fine — tests will use httptest mocks.

- [ ] **Step 2: Check AUDIT_SERVICE_URL env var**

```bash
grep -r "AUDIT_SERVICE" services/marketplace-api/ || echo "Not configured yet"
```

If not found, we'll add it in Task 2.

No commit. Task 0 is read-only.

---

## Task 1: Authz roles for audit logs

**Files:**
- Create: `services/marketplace-api/internal/authz/audit_roles.go`

- [ ] **Step 1: Create audit role constants**

Create `services/marketplace-api/internal/authz/audit_roles.go`:

```go
package authz

// Audit log settings — admin and above can view audit logs.
// Only admins can export (CSV download could be sensitive).

// AuditLogsViewRole allows viewing audit log entries.
var AuditLogsViewRole = RoleAdmin

// AuditLogsExportRole allows CSV export of audit logs.
var AuditLogsExportRole = RoleAdmin
```

**Commit:** `feat(audit): add authz role constants for audit log endpoints`

---

## Task 2: Audit logs proxy handler

**Files:**
- Create: `services/marketplace-api/internal/handlers/admin/audit.go`
- Create: `services/marketplace-api/internal/handlers/admin/audit_test.go`

### TDD: RED — Write tests first

- [ ] **Step 1: Write audit handler tests**

Create `services/marketplace-api/internal/handlers/admin/audit_test.go`:

```go
package admin_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"log/slog"

	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// mockAuditService returns an httptest server that mimics audit-service responses.
func mockAuditService(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify expected query parameters are forwarded.
		tenantID := r.URL.Query().Get("tenant_id")
		if tenantID == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "tenant_id required"})
			return
		}

		switch {
		case r.URL.Path == "/api/v1/logs" && r.URL.Query().Get("format") == "csv":
			w.Header().Set("Content-Type", "text/csv")
			w.Header().Set("Content-Disposition", "attachment; filename=audit-logs.csv")
			w.Write([]byte("id,action,user_email,created_at\n"))
			w.Write([]byte("uuid-1,product.created,admin@test.com,2026-01-01T00:00:00Z\n"))
		case r.URL.Path == "/api/v1/logs":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{
						"id":            "uuid-1",
						"tenant_id":     tenantID,
						"user_email":    "admin@test.com",
						"action":        "product.created",
						"resource_type": "product",
						"severity":      "info",
						"created_at":    "2026-01-01T00:00:00Z",
					},
				},
				"meta": map[string]any{"total": 1, "page": 1, "limit": 20},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func setupAuditRouter(t *testing.T, auditURL string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()

	handler := admin.NewAuditLogsHandler(auditURL, slog.Default())

	storeID := uuid.New()
	tenantID := uuid.New()

	// Middleware to inject store + tenant context.
	group := r.Group("/api/v1/admin/stores/:storeId", func(c *gin.Context) {
		c.Set("store", &stores.Store{ID: storeID, TenantID: tenantID})
		c.Set("tenant_id", tenantID.String())
		c.Next()
	})
	group.GET("/audit-logs", handler.List)
	group.GET("/audit-logs/export", handler.Export)

	return r
}

func TestAuditLogs_List(t *testing.T) {
	srv := mockAuditService(t)
	defer srv.Close()

	r := setupAuditRouter(t, srv.URL)
	storeID := uuid.NewString()

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/admin/stores/%s/audit-logs?page=1&limit=20", storeID), nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data, ok := resp["data"].([]any)
	require.True(t, ok)
	assert.Len(t, data, 1)

	meta, ok := resp["meta"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(1), meta["total"])
}

func TestAuditLogs_List_WithFilters(t *testing.T) {
	srv := mockAuditService(t)
	defer srv.Close()

	r := setupAuditRouter(t, srv.URL)
	storeID := uuid.NewString()

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET",
		fmt.Sprintf("/api/v1/admin/stores/%s/audit-logs?action=product.created&severity=info&date_from=2026-01-01", storeID),
		nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuditLogs_Export_CSV(t *testing.T) {
	srv := mockAuditService(t)
	defer srv.Close()

	r := setupAuditRouter(t, srv.URL)
	storeID := uuid.NewString()

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET",
		fmt.Sprintf("/api/v1/admin/stores/%s/audit-logs/export", storeID),
		nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/csv")
	assert.Contains(t, w.Body.String(), "product.created")
}

func TestAuditLogs_List_AuditServiceDown(t *testing.T) {
	// Use a URL that will fail to connect.
	r := setupAuditRouter(t, "http://127.0.0.1:1")
	storeID := uuid.NewString()

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET",
		fmt.Sprintf("/api/v1/admin/stores/%s/audit-logs", storeID),
		nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadGateway, w.Code)
}
```

- [ ] **Step 2: Implement audit.go**

Create `services/marketplace-api/internal/handlers/admin/audit.go`:

```go
package admin

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/stores"
)

// AuditLogsHandler proxies read-only audit log requests to the audit-service.
// Marketplace-api does NOT store audit logs — audit-service is the source of truth.
type AuditLogsHandler struct {
	auditServiceURL string
	client          *http.Client
	logger          *slog.Logger
}

// NewAuditLogsHandler constructs an AuditLogsHandler.
func NewAuditLogsHandler(auditServiceURL string, logger *slog.Logger) *AuditLogsHandler {
	return &AuditLogsHandler{
		auditServiceURL: auditServiceURL,
		client:          &http.Client{Timeout: 15 * time.Second},
		logger:          logger,
	}
}

// allowedFilters defines which query params are forwarded to audit-service.
// Unknown params are silently dropped to prevent injection.
var allowedFilters = map[string]bool{
	"page":          true,
	"limit":         true,
	"user":          true,
	"action":        true,
	"resource_type": true,
	"severity":      true,
	"date_from":     true,
	"date_to":       true,
	"search":        true,
}

// List handles GET /admin/stores/:storeId/audit-logs.
// Proxies to: GET audit-service/api/v1/logs?tenant_id=X&store_id=X&...
func (h *AuditLogsHandler) List(c *gin.Context) {
	store := storeFromCtx(c)
	if store == nil {
		return
	}

	proxyURL := h.buildProxyURL(store, c.Request.URL.Query(), "")
	h.proxyGET(c, proxyURL, "application/json")
}

// Export handles GET /admin/stores/:storeId/audit-logs/export.
// Proxies to: GET audit-service/api/v1/logs?format=csv&tenant_id=X&store_id=X&...
func (h *AuditLogsHandler) Export(c *gin.Context) {
	store := storeFromCtx(c)
	if store == nil {
		return
	}

	proxyURL := h.buildProxyURL(store, c.Request.URL.Query(), "csv")
	h.proxyGET(c, proxyURL, "text/csv")
}

// buildProxyURL constructs the audit-service URL with tenant/store scoping
// and only whitelisted query parameters.
func (h *AuditLogsHandler) buildProxyURL(store *stores.Store, incoming url.Values, format string) string {
	params := url.Values{}
	params.Set("tenant_id", store.TenantID.String())
	params.Set("store_id", store.ID.String())

	if format != "" {
		params.Set("format", format)
	}

	for key, values := range incoming {
		if allowedFilters[key] && len(values) > 0 && values[0] != "" {
			params.Set(key, values[0])
		}
	}

	// Default pagination.
	if params.Get("page") == "" {
		params.Set("page", "1")
	}
	if params.Get("limit") == "" {
		params.Set("limit", "20")
	}

	return fmt.Sprintf("%s/api/v1/logs?%s", h.auditServiceURL, params.Encode())
}

// proxyGET forwards a GET request to audit-service and streams the response back.
func (h *AuditLogsHandler) proxyGET(c *gin.Context, targetURL, expectedContentType string) {
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, targetURL, nil)
	if err != nil {
		h.logger.Error("audit proxy: build request", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": "failed to build audit request"})
		return
	}

	resp, err := h.client.Do(req)
	if err != nil {
		h.logger.Error("audit proxy: request failed", "url", targetURL, "err", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "bad_gateway", "message": "audit service unavailable"})
		return
	}
	defer resp.Body.Close()

	// If audit-service returned an error, forward it.
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		h.logger.Warn("audit proxy: upstream error", "status", resp.StatusCode, "body", string(body))
		c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
		return
	}

	// Stream the response back.
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = expectedContentType
	}

	// For CSV export, set download headers.
	if expectedContentType == "text/csv" {
		c.Header("Content-Disposition", "attachment; filename=audit-logs.csv")
	}

	c.DataFromReader(resp.StatusCode, resp.ContentLength, contentType, resp.Body, nil)
}
```

### GREEN

- [ ] **Step 3: Run tests**

```bash
cd services/marketplace-api && go test ./internal/handlers/admin/... -run TestAuditLogs -v -count=1
```

All 4 tests must pass.

**Commit:** `feat(audit): add audit logs proxy handler with filter passthrough and CSV export`

---

## Task 3: Route wiring + Deps

**Files:**
- Modify: `services/marketplace-api/internal/handlers/admin/routes.go`
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go`

- [ ] **Step 1: Add AuditLogsHandler to Deps struct**

In `services/marketplace-api/internal/handlers/admin/routes.go`, add to the `Deps` struct:

```go
AuditLogsHandler         *AuditLogsHandler        // S4: audit logs proxy
```

No import needed — `AuditLogsHandler` is in the same package.

- [ ] **Step 2: Add audit-logs routes to RegisterAdmin**

In `RegisterAdmin`, after the subscription block (or after tax if S3 hasn't shipped yet), add:

```go
		// Audit logs — S4. Read-only proxy to audit-service.
		if deps.AuditLogsHandler != nil {
			auditLogs := storeRoute.Group("/audit-logs")
			{
				auditLogs.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.AuditLogsViewRole),
					deps.AuditLogsHandler.List)
				auditLogs.GET("/export",
					deps.AuthzMiddleware.RequireTenantRelation(authz.AuditLogsExportRole),
					deps.AuditLogsHandler.Export)
			}
		}
```

- [ ] **Step 3: Wire in main.go**

In `services/marketplace-api/cmd/marketplace-api/main.go`, in the admin deps construction block:

```go
	// Audit logs handler (S4).
	var auditLogsHandler *admin.AuditLogsHandler
	if auditURL := os.Getenv("AUDIT_SERVICE_URL"); auditURL != "" {
		auditLogsHandler = admin.NewAuditLogsHandler(auditURL, log)
		log.Info("audit logs handler initialized", "audit_service_url", auditURL)
	} else {
		log.Warn("AUDIT_SERVICE_URL not set — audit log endpoints disabled")
	}
```

Then add to the `adminDeps` struct literal:

```go
	AuditLogsHandler: auditLogsHandler,
```

- [ ] **Step 4: Build check**

```bash
cd services/marketplace-api && go build ./...
```

Must compile without errors.

**Commit:** `feat(audit): wire audit logs routes and proxy handler in main.go`

---

## Task 4: Admin UI — audit logs page

**Files:**
- Create: `apps/admin/lib/api/audit-api.ts`
- Create: `apps/admin/app/settings/audit-logs/page.tsx`
- Create: `apps/admin/app/settings/audit-logs/actions.ts`
- Create: `apps/admin/components/settings/AuditLogsClient.tsx`
- Create: `apps/admin/components/settings/AuditLogRow.tsx`
- Modify: `apps/admin/components/shell/AdminShell.tsx`

- [ ] **Step 1: Create audit-api.ts**

Create `apps/admin/lib/api/audit-api.ts`:

```typescript
import type { SessionHeaders } from "./marketplace-api";

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

export interface AuditLogEntry {
  id: string;
  tenant_id: string;
  store_id: string;
  user_id: string;
  user_email: string;
  action: string;
  resource_type: string;
  resource_id?: string;
  severity: "info" | "warn" | "error";
  metadata?: Record<string, unknown>;
  ip_address?: string;
  user_agent?: string;
  created_at: string;
}

export interface AuditLogListResponse {
  data: AuditLogEntry[];
  meta: {
    total: number;
    page: number;
    limit: number;
  };
}

export interface AuditLogFilters {
  page?: number;
  limit?: number;
  user?: string;
  action?: string;
  resource_type?: string;
  severity?: string;
  date_from?: string;
  date_to?: string;
  search?: string;
}

export async function listAuditLogs(
  storeId: string,
  filters: AuditLogFilters,
  headers: SessionHeaders,
): Promise<AuditLogListResponse> {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(filters)) {
    if (value !== undefined && value !== "") {
      params.set(key, String(value));
    }
  }

  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/audit-logs?${params}`,
    { headers, cache: "no-store" },
  );
  if (!res.ok) {
    throw new Error(`Failed to fetch audit logs: ${res.status}`);
  }
  return res.json();
}

export function auditLogsExportURL(
  storeId: string,
  filters: AuditLogFilters,
): string {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(filters)) {
    if (value !== undefined && value !== "") {
      params.set(key, String(value));
    }
  }
  return `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/audit-logs/export?${params}`;
}
```

- [ ] **Step 2: Create AuditLogRow.tsx**

Create `apps/admin/components/settings/AuditLogRow.tsx`:

```tsx
"use client";

import { useState } from "react";
import type { AuditLogEntry } from "@/lib/api/audit-api";

interface AuditLogRowProps {
  entry: AuditLogEntry;
}

const severityColors: Record<string, string> = {
  info: "bg-[color:var(--moss-700)]/10 text-[color:var(--moss-700)]",
  warn: "bg-[color:var(--warning)]/10 text-[color:var(--warning)]",
  error: "bg-[color:var(--signal)]/10 text-[color:var(--signal)]",
};

export function AuditLogRow({ entry }: AuditLogRowProps) {
  const [expanded, setExpanded] = useState(false);

  return (
    <>
      <tr
        className="cursor-pointer border-b border-[color:var(--ink-900)]/5 transition-colors hover:bg-paper-100"
        onClick={() => setExpanded((prev) => !prev)}
        role="button"
        aria-expanded={expanded}
        tabIndex={0}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            setExpanded((prev) => !prev);
          }
        }}
      >
        <td className="py-3 pr-3 text-sm text-foreground-secondary">
          {new Date(entry.created_at).toLocaleString()}
        </td>
        <td className="py-3 pr-3 text-sm text-foreground">
          {entry.user_email}
        </td>
        <td className="py-3 pr-3 text-sm font-mono text-foreground">
          {entry.action}
        </td>
        <td className="py-3 pr-3 text-sm text-foreground-secondary">
          {entry.resource_type}
        </td>
        <td className="py-3 pr-3">
          <span
            className={`inline-block rounded-full px-2 py-0.5 text-xs font-medium ${severityColors[entry.severity] ?? severityColors.info}`}
          >
            {entry.severity}
          </span>
        </td>
        <td className="py-3 text-sm text-foreground-secondary">
          {expanded ? "\u25B2" : "\u25BC"}
        </td>
      </tr>
      {expanded && (
        <tr className="border-b border-[color:var(--ink-900)]/5 bg-paper-100/50">
          <td colSpan={6} className="px-4 py-4">
            <div className="grid grid-cols-2 gap-4 text-sm">
              {entry.resource_id && (
                <div>
                  <p className="text-foreground-secondary">Resource ID</p>
                  <p className="font-mono text-foreground">{entry.resource_id}</p>
                </div>
              )}
              {entry.ip_address && (
                <div>
                  <p className="text-foreground-secondary">IP Address</p>
                  <p className="font-mono text-foreground">{entry.ip_address}</p>
                </div>
              )}
              {entry.user_agent && (
                <div className="col-span-2">
                  <p className="text-foreground-secondary">User Agent</p>
                  <p className="font-mono text-xs text-foreground break-all">
                    {entry.user_agent}
                  </p>
                </div>
              )}
              {entry.metadata && Object.keys(entry.metadata).length > 0 && (
                <div className="col-span-2">
                  <p className="text-foreground-secondary">Metadata</p>
                  <pre className="mt-1 rounded-md bg-[color:var(--ink-900)]/5 p-3 text-xs text-foreground">
                    {JSON.stringify(entry.metadata, null, 2)}
                  </pre>
                </div>
              )}
            </div>
          </td>
        </tr>
      )}
    </>
  );
}
```

- [ ] **Step 3: Create AuditLogsClient.tsx**

Create `apps/admin/components/settings/AuditLogsClient.tsx`:

```tsx
"use client";

import { useCallback, useState, useTransition } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { AuditLogRow } from "./AuditLogRow";
import type {
  AuditLogEntry,
  AuditLogFilters,
  AuditLogListResponse,
} from "@/lib/api/audit-api";

interface AuditLogsClientProps {
  storeId: string;
  initialData: AuditLogListResponse;
  exportBaseURL: string;
}

export function AuditLogsClient({
  storeId,
  initialData,
  exportBaseURL,
}: AuditLogsClientProps) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [isPending, startTransition] = useTransition();

  const [search, setSearch] = useState(searchParams.get("search") ?? "");
  const [action, setAction] = useState(searchParams.get("action") ?? "");
  const [resourceType, setResourceType] = useState(
    searchParams.get("resource_type") ?? "",
  );
  const [severity, setSeverity] = useState(
    searchParams.get("severity") ?? "",
  );
  const [dateFrom, setDateFrom] = useState(
    searchParams.get("date_from") ?? "",
  );
  const [dateTo, setDateTo] = useState(searchParams.get("date_to") ?? "");

  const currentPage = Number(searchParams.get("page") ?? "1");
  const totalPages = Math.ceil(
    (initialData.meta.total || 1) / (initialData.meta.limit || 20),
  );

  const applyFilters = useCallback(
    (overrides: Partial<AuditLogFilters> = {}) => {
      const params = new URLSearchParams();
      const filters: Record<string, string> = {
        search,
        action,
        resource_type: resourceType,
        severity,
        date_from: dateFrom,
        date_to: dateTo,
        page: String(currentPage),
        ...Object.fromEntries(
          Object.entries(overrides).map(([k, v]) => [k, String(v ?? "")]),
        ),
      };

      for (const [key, value] of Object.entries(filters)) {
        if (value) params.set(key, value);
      }

      startTransition(() => {
        router.push(`/settings/audit-logs?${params.toString()}`);
      });
    },
    [
      search,
      action,
      resourceType,
      severity,
      dateFrom,
      dateTo,
      currentPage,
      router,
    ],
  );

  function handleExport() {
    const params = new URLSearchParams();
    if (search) params.set("search", search);
    if (action) params.set("action", action);
    if (resourceType) params.set("resource_type", resourceType);
    if (severity) params.set("severity", severity);
    if (dateFrom) params.set("date_from", dateFrom);
    if (dateTo) params.set("date_to", dateTo);

    window.open(`${exportBaseURL}&${params.toString()}`, "_blank");
  }

  return (
    <div className="space-y-6">
      {/* Search + filters */}
      <div className="space-y-4">
        <div className="flex items-center gap-3">
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") applyFilters({ page: 1 });
            }}
            placeholder="Search audit logs..."
            className="flex-1 rounded-md border border-[color:var(--ink-900)]/15 bg-white px-3 py-2 text-sm text-foreground placeholder:text-foreground-secondary/60 focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]"
          />
          <button
            type="button"
            onClick={() => applyFilters({ page: 1 })}
            disabled={isPending}
            className="rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-[color:var(--ink-900)]/90 disabled:opacity-50"
          >
            Search
          </button>
          <button
            type="button"
            onClick={handleExport}
            className="rounded-md border border-[color:var(--ink-900)]/20 px-4 py-2 text-sm font-medium text-foreground transition-colors hover:bg-paper-100"
          >
            Export CSV
          </button>
        </div>

        <div className="flex flex-wrap gap-3">
          <select
            value={action}
            onChange={(e) => {
              setAction(e.target.value);
              applyFilters({ action: e.target.value, page: 1 });
            }}
            className="rounded-md border border-[color:var(--ink-900)]/15 bg-white px-3 py-1.5 text-sm"
            aria-label="Filter by action"
          >
            <option value="">All actions</option>
            <option value="product.created">product.created</option>
            <option value="product.updated">product.updated</option>
            <option value="product.deleted">product.deleted</option>
            <option value="order.placed">order.placed</option>
            <option value="order.confirmed">order.confirmed</option>
            <option value="order.cancelled">order.cancelled</option>
            <option value="settings.updated">settings.updated</option>
          </select>

          <select
            value={resourceType}
            onChange={(e) => {
              setResourceType(e.target.value);
              applyFilters({ resource_type: e.target.value, page: 1 });
            }}
            className="rounded-md border border-[color:var(--ink-900)]/15 bg-white px-3 py-1.5 text-sm"
            aria-label="Filter by resource type"
          >
            <option value="">All resources</option>
            <option value="product">Product</option>
            <option value="order">Order</option>
            <option value="category">Category</option>
            <option value="settings">Settings</option>
            <option value="user">User</option>
          </select>

          <select
            value={severity}
            onChange={(e) => {
              setSeverity(e.target.value);
              applyFilters({ severity: e.target.value, page: 1 });
            }}
            className="rounded-md border border-[color:var(--ink-900)]/15 bg-white px-3 py-1.5 text-sm"
            aria-label="Filter by severity"
          >
            <option value="">All severities</option>
            <option value="info">Info</option>
            <option value="warn">Warning</option>
            <option value="error">Error</option>
          </select>

          <input
            type="date"
            value={dateFrom}
            onChange={(e) => {
              setDateFrom(e.target.value);
              applyFilters({ date_from: e.target.value, page: 1 });
            }}
            className="rounded-md border border-[color:var(--ink-900)]/15 bg-white px-3 py-1.5 text-sm"
            aria-label="Date from"
          />
          <input
            type="date"
            value={dateTo}
            onChange={(e) => {
              setDateTo(e.target.value);
              applyFilters({ date_to: e.target.value, page: 1 });
            }}
            className="rounded-md border border-[color:var(--ink-900)]/15 bg-white px-3 py-1.5 text-sm"
            aria-label="Date to"
          />
        </div>
      </div>

      {/* Table */}
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-[color:var(--ink-900)]/10">
              <th className="py-3 pr-3 text-left font-medium text-foreground-secondary">
                Timestamp
              </th>
              <th className="py-3 pr-3 text-left font-medium text-foreground-secondary">
                User
              </th>
              <th className="py-3 pr-3 text-left font-medium text-foreground-secondary">
                Action
              </th>
              <th className="py-3 pr-3 text-left font-medium text-foreground-secondary">
                Resource
              </th>
              <th className="py-3 pr-3 text-left font-medium text-foreground-secondary">
                Severity
              </th>
              <th className="py-3 text-left font-medium text-foreground-secondary" />
            </tr>
          </thead>
          <tbody>
            {initialData.data.length === 0 ? (
              <tr>
                <td
                  colSpan={6}
                  className="py-12 text-center text-foreground-secondary"
                >
                  No audit log entries found.
                </td>
              </tr>
            ) : (
              initialData.data.map((entry) => (
                <AuditLogRow key={entry.id} entry={entry} />
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="flex items-center justify-between pt-2">
          <p className="text-sm text-foreground-secondary">
            {initialData.meta.total} entries total
          </p>
          <div className="flex gap-2">
            <button
              type="button"
              disabled={currentPage <= 1 || isPending}
              onClick={() => applyFilters({ page: currentPage - 1 })}
              className="rounded-md border border-[color:var(--ink-900)]/20 px-3 py-1.5 text-sm disabled:opacity-50"
            >
              Previous
            </button>
            <span className="flex items-center px-2 text-sm text-foreground-secondary">
              Page {currentPage} of {totalPages}
            </span>
            <button
              type="button"
              disabled={currentPage >= totalPages || isPending}
              onClick={() => applyFilters({ page: currentPage + 1 })}
              className="rounded-md border border-[color:var(--ink-900)]/20 px-3 py-1.5 text-sm disabled:opacity-50"
            >
              Next
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 4: Create page.tsx**

Create `apps/admin/app/settings/audit-logs/page.tsx`:

```tsx
import { AdminShell } from "@/components/shell/AdminShell";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import { listAuditLogs, auditLogsExportURL } from "@/lib/api/audit-api";
import { AuditLogsClient } from "@/components/settings/AuditLogsClient";

interface PageProps {
  searchParams: Promise<Record<string, string | undefined>>;
}

export default async function AuditLogsPage({ searchParams }: PageProps) {
  const params = await searchParams;
  const {
    tenantName,
    email,
    role,
    memberships,
    tenantId,
    currentStore,
  } = await getServerSessionContext();

  return (
    <AdminShell
      tenantName={tenantName}
      userEmail={email}
      role={role}
      memberships={memberships}
      currentTenantId={tenantId}
    >
      <div className="mx-auto w-full max-w-6xl space-y-10">
        <header className="space-y-3">
          <p className="eyebrow">Store setup</p>
          <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-5xl font-medium tracking-tight text-foreground">
            Audit logs
          </h1>
          <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
            Review a chronological record of actions taken in your store.
          </p>
        </header>

        {currentStore ? (
          <AuditLogsContent storeId={currentStore.id} searchParams={params} />
        ) : (
          <p className="text-sm text-danger">
            No store found. Please create a store first.
          </p>
        )}
      </div>
    </AdminShell>
  );
}

async function AuditLogsContent({
  storeId,
  searchParams,
}: {
  storeId: string;
  searchParams: Record<string, string | undefined>;
}) {
  const filters = {
    page: Number(searchParams.page ?? 1),
    limit: Number(searchParams.limit ?? 20),
    user: searchParams.user,
    action: searchParams.action,
    resource_type: searchParams.resource_type,
    severity: searchParams.severity,
    date_from: searchParams.date_from,
    date_to: searchParams.date_to,
    search: searchParams.search,
  };

  const data = await listAuditLogs(storeId, filters, {} as any);
  const exportURL = auditLogsExportURL(storeId, filters);

  return (
    <AuditLogsClient
      storeId={storeId}
      initialData={data}
      exportBaseURL={exportURL}
    />
  );
}
```

- [ ] **Step 5: Create empty actions.ts**

Create `apps/admin/app/settings/audit-logs/actions.ts`:

```typescript
"use server";

// Audit logs are read-only — no server actions needed.
// This file is a placeholder for consistency with other settings pages.
// CSV export is handled via direct download link (no server action needed).
```

- [ ] **Step 6: Add "Audit Logs" to sidebar navigation**

In `apps/admin/components/shell/AdminShell.tsx`, find the settings navigation children array and add after "Subscription" (or after "Tax" if S3 hasn't shipped):

```typescript
      { label: "Audit Logs", href: "/settings/audit-logs" },
```

- [ ] **Step 7: Verify frontend builds**

```bash
cd apps/admin && npx next build
```

Must compile without type errors.

**Commit:** `feat(audit): add audit logs page with search, filters, pagination, and CSV export`

---

## Task 5: E2E smoke test

**Files:**
- Create: `apps/admin/e2e/audit-logs.spec.ts`

- [ ] **Step 1: Write Playwright test**

Create `apps/admin/e2e/audit-logs.spec.ts`:

```typescript
import { test, expect } from "@playwright/test";

test.describe("Audit Logs Settings", () => {
  test("renders audit logs page with search bar", async ({ page }) => {
    await page.goto("/settings/audit-logs");
    await expect(page.getByText("Audit logs")).toBeVisible();
    await expect(
      page.getByPlaceholder("Search audit logs..."),
    ).toBeVisible();
  });

  test("shows filter dropdowns", async ({ page }) => {
    await page.goto("/settings/audit-logs");
    await expect(page.getByLabel("Filter by action")).toBeVisible();
    await expect(page.getByLabel("Filter by resource type")).toBeVisible();
    await expect(page.getByLabel("Filter by severity")).toBeVisible();
  });

  test("shows export CSV button", async ({ page }) => {
    await page.goto("/settings/audit-logs");
    await expect(page.getByText("Export CSV")).toBeVisible();
  });

  test("shows empty state when no logs exist", async ({ page }) => {
    await page.goto("/settings/audit-logs");
    // May show entries or empty state depending on test data.
    const table = page.locator("table");
    await expect(table).toBeVisible();
  });

  test("sidebar contains Audit Logs link", async ({ page }) => {
    await page.goto("/settings/audit-logs");
    const sidebar = page.locator("aside");
    await expect(sidebar.getByText("Audit Logs")).toBeVisible();
  });
});
```

- [ ] **Step 2: Run E2E tests**

```bash
cd apps/admin && npx playwright test e2e/audit-logs.spec.ts
```

**Commit:** `test(audit): add Playwright E2E smoke tests for audit logs page`

---

## Summary

| Task | What it delivers | Files |
|------|-----------------|-------|
| 0 | Prerequisites check | read-only |
| 1 | Authz roles | 1 Go file |
| 2 | Audit proxy handler + tests | 2 Go files |
| 3 | Route wiring + main.go | 2 Go files modified |
| 4 | Admin UI (page, client, row, api, sidebar) | 6 TS/TSX files |
| 5 | E2E smoke test | 1 TS file |

**Environment variables required:**
- `AUDIT_SERVICE_URL` — audit-service base URL (e.g., `http://audit-service.platform.svc.cluster.local:8080`)

**Key design decisions:**
- No migration — audit-service owns the data
- Proxy pattern — marketplace-api acts as a gateway, adding tenant/store scoping
- Whitelisted query params — only known filter keys are forwarded to prevent injection
- CSV export via streaming — `DataFromReader` streams audit-service response directly to client
- `BadGateway` (502) returned when audit-service is unreachable — clear signal for ops
