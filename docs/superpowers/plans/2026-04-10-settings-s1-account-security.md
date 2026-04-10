# Settings S1 — Account & Security Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Ship account profile editing, MFA toggle (proxied through auth-bff), active sessions list, and delete tenant flow. No migration needed — uses existing tenant/auth infrastructure.

**Architecture:** New `internal/handlers/admin/account.go` handler proxying MFA/session calls to auth-bff. Delete tenant calls tenant-service. Admin UI at /settings/account.

**Tech Stack:** Go 1.26, Gin, net/http (auth-bff proxy). Next.js 16, React 19, Tailwind.

**Design Authority:** `docs/superpowers/specs/2026-04-10-settings-tier1-tier2-design.md` §3.1, §4.1, §6.1.

---

## Status

> **Pending.** All tasks open.

---

## Scope check

Adds `services/marketplace-api/internal/handlers/admin/account.go` (account handler), `services/marketplace-api/internal/authbff/client.go` (auth-bff HTTP proxy client), wiring in `cmd/marketplace-api/main.go` + `internal/handlers/admin/routes.go`, and frontend pages + components under `apps/admin/`. No new migration — S1 reads profile data from the existing `stores` projection + proxies MFA/sessions to auth-bff.

Spec sections authoritative:
- Design spec §3.1 (Account & Security architecture)
- Design spec §4.1 (API endpoints)
- Design spec §5.1 (page layout)
- Design spec §6.1 (security)
- Design spec §8 (testing)

**Out of scope (deferred):**
- Backup codes for MFA
- Forced MFA policy for team members
- Password change (GIP handles via auth-bff login flow)

---

## Decisions locked (from the spec — do NOT re-debate)

1. **MFA proxy pattern:** marketplace-api proxies to auth-bff. marketplace-api NEVER touches GIP directly.
2. **Session management:** Proxied through auth-bff's internal API. marketplace-api reads session list and forwards revocation requests.
3. **Delete tenant:** Requires owner role + typed confirmation string "delete my store". Calls tenant-service DELETE endpoint.
4. **Auth middleware:** Reuses existing `auth.HeaderTrustAuth` + `stores.StoreMiddleware` chain.
5. **Account routes mount point:** `GET/PATCH /admin/account` (tenant-wide, outside /stores/:storeId). MFA/sessions sub-routes under `/admin/account/mfa/*` and `/admin/account/sessions/*`.
6. **Delete route:** `DELETE /admin/account` requires owner role via OpenFGA check.
7. **Design system:** Paper · Ink · Moss tokens. Danger zone uses `border-[color:var(--danger)]` with ink-900 delete button. Status badges: moss-700 for enabled, ink-900/40 for disabled.

---

## File structure produced by S1

### New backend files

```
services/marketplace-api/
  internal/authbff/
    client.go                    auth-bff HTTP proxy client interface + implementation
    client_test.go               unit tests with httptest.Server mock
  internal/handlers/admin/
    account.go                   AccountHandler: profile get/update, MFA proxy, sessions, delete
    account_test.go              unit tests for handler logic
```

### Modified backend files

```
services/marketplace-api/
  cmd/marketplace-api/main.go              Wire AccountHandler + authbff.Client
  internal/handlers/admin/routes.go        Add account route group + Deps field
  pkg/config/config.go                     Add AUTH_BFF_URL + TENANT_SERVICE_URL env vars
```

### New frontend files

```
apps/admin/
  app/settings/account/
    page.tsx                               Server component: account settings page
    actions.ts                             Server actions: updateProfile, enableMFA, disableMFA, revokeSes, deleteTenant
  components/settings/
    AccountProfileForm.tsx                 Name + email inline-edit form
    AccountMfaSection.tsx                  MFA status badge + enable/disable + QR code modal
    AccountSessionsList.tsx                Active sessions table + revoke buttons
    AccountDangerZone.tsx                  Delete tenant with typed confirmation dialog
  lib/api/
    account-api.ts                         Typed API client for /admin/account/* endpoints
```

### Modified frontend files

```
apps/admin/
  components/shell/AdminShell.tsx          Add "Account" nav leaf under Settings section
```

---

## Tasks

### Task 1 — Auth-BFF proxy client (`internal/authbff/client.go`)

**TDD: RED first.** Write `client_test.go` testing all proxy methods against `httptest.Server` mocks, then implement.

- [ ] **1a.** Create `services/marketplace-api/internal/authbff/client.go`

Define the interface and concrete HTTP client:

```go
// Package authbff provides an HTTP client that proxies account-related
// operations to the auth-bff service. marketplace-api never touches GIP
// directly — all MFA and session operations go through auth-bff.
package authbff

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// MFAEnrollment is the response from auth-bff when starting MFA enrollment.
type MFAEnrollment struct {
	QRCodeURL string `json:"qr_code_url"`
	Secret    string `json:"secret"` // only shown once during enrollment
}

// Session represents an active login session.
type Session struct {
	ID         string `json:"id"`
	Device     string `json:"device"`
	IP         string `json:"ip"`
	LastActive string `json:"last_active"`
	Current    bool   `json:"current"`
}

// AccountProfile is the user profile as returned by auth-bff.
type AccountProfile struct {
	UserID     string `json:"user_id"`
	Email      string `json:"email"`
	Name       string `json:"name"`
	MFAEnabled bool   `json:"mfa_enabled"`
	CreatedAt  string `json:"created_at"`
}

// Client defines the auth-bff proxy operations.
type Client interface {
	GetProfile(ctx context.Context, userID, tenantID string) (*AccountProfile, error)
	UpdateProfile(ctx context.Context, userID, tenantID, name, email string) error
	EnableMFA(ctx context.Context, userID, tenantID string) (*MFAEnrollment, error)
	VerifyMFA(ctx context.Context, userID, tenantID, code string) error
	DisableMFA(ctx context.Context, userID, tenantID string) error
	ListSessions(ctx context.Context, userID, tenantID string) ([]Session, error)
	RevokeSession(ctx context.Context, userID, tenantID, sessionID string) error
	DeleteTenant(ctx context.Context, userID, tenantID, confirmationText string) error
}

// HTTPClient is the concrete auth-bff proxy client.
type HTTPClient struct {
	baseURL    string
	tenantURL  string // tenant-service base URL for delete-tenant
	httpClient *http.Client
	secret     string // X-Internal-Auth header value
}

// New creates a new auth-bff proxy client.
func New(authBFFURL, tenantServiceURL, internalSecret string) *HTTPClient {
	return &HTTPClient{
		baseURL:   authBFFURL,
		tenantURL: tenantServiceURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		secret: internalSecret,
	}
}
```

Implement each method following this pattern (shown for `EnableMFA`):

```go
func (c *HTTPClient) EnableMFA(ctx context.Context, userID, tenantID string) (*MFAEnrollment, error) {
	url := fmt.Sprintf("%s/internal/users/%s/mfa/enable", c.baseURL, userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, fmt.Errorf("authbff: build request: %w", err)
	}
	req.Header.Set("X-Internal-Auth", c.secret)
	req.Header.Set("X-Tenant-Id", tenantID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("authbff: enable mfa: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("authbff: enable mfa: status %d: %s", resp.StatusCode, body)
	}

	var enrollment MFAEnrollment
	if err := json.NewDecoder(resp.Body).Decode(&enrollment); err != nil {
		return nil, fmt.Errorf("authbff: decode mfa enrollment: %w", err)
	}
	return &enrollment, nil
}
```

Implement all methods: `GetProfile`, `UpdateProfile`, `EnableMFA`, `VerifyMFA`, `DisableMFA`, `ListSessions`, `RevokeSession`, `DeleteTenant` (this one calls `tenantURL`).

- [ ] **1b.** Create `services/marketplace-api/internal/authbff/client_test.go`

Write tests FIRST (RED), then make them pass:

```go
package authbff_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/authbff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetProfile_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Contains(t, r.URL.Path, "/internal/users/")
		assert.Equal(t, "test-secret", r.Header.Get("X-Internal-Auth"))
		assert.Equal(t, "tenant-1", r.Header.Get("X-Tenant-Id"))
		json.NewEncoder(w).Encode(authbff.AccountProfile{
			UserID:     "user-1",
			Email:      "test@example.com",
			Name:       "Test User",
			MFAEnabled: false,
			CreatedAt:  "2026-01-01T00:00:00Z",
		})
	}))
	defer srv.Close()

	client := authbff.New(srv.URL, "", "test-secret")
	profile, err := client.GetProfile(context.Background(), "user-1", "tenant-1")
	require.NoError(t, err)
	assert.Equal(t, "test@example.com", profile.Email)
	assert.False(t, profile.MFAEnabled)
}

func TestEnableMFA_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		json.NewEncoder(w).Encode(authbff.MFAEnrollment{
			QRCodeURL: "otpauth://totp/mark8ly:test@example.com?secret=ABC",
			Secret:    "ABC",
		})
	}))
	defer srv.Close()

	client := authbff.New(srv.URL, "", "test-secret")
	enrollment, err := client.EnableMFA(context.Background(), "user-1", "tenant-1")
	require.NoError(t, err)
	assert.Contains(t, enrollment.QRCodeURL, "otpauth://")
}

func TestListSessions_AuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer srv.Close()

	client := authbff.New(srv.URL, "", "test-secret")
	_, err := client.ListSessions(context.Background(), "user-1", "tenant-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status 401")
}

func TestDeleteTenant_CallsTenantService(t *testing.T) {
	tenantSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Contains(t, r.URL.Path, "/internal/tenants/")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer tenantSrv.Close()

	client := authbff.New("http://unused", tenantSrv.URL, "test-secret")
	err := client.DeleteTenant(context.Background(), "user-1", "tenant-1", "delete my store")
	require.NoError(t, err)
}
```

Add tests for: `UpdateProfile` success/failure, `VerifyMFA` success/invalid code, `DisableMFA`, `RevokeSession`, `DeleteTenant` wrong confirmation text (rejected client-side before HTTP call), network timeout handling.

**Verification:**
```bash
cd services/marketplace-api && go test ./internal/authbff/... -v -count=1
```

---

### Task 2 — Account handler (`internal/handlers/admin/account.go`)

**TDD: RED first.** Write `account_test.go` with handler tests using `httptest.ResponseRecorder` + mock `authbff.Client`, then implement.

- [ ] **2a.** Create `services/marketplace-api/internal/handlers/admin/account_test.go`

```go
package admin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mark8ly/marketplace-api/internal/authbff"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockAuthBFFClient implements authbff.Client for testing.
type mockAuthBFFClient struct {
	getProfileFn    func(ctx context.Context, userID, tenantID string) (*authbff.AccountProfile, error)
	updateProfileFn func(ctx context.Context, userID, tenantID, name, email string) error
	enableMFAFn     func(ctx context.Context, userID, tenantID string) (*authbff.MFAEnrollment, error)
	verifyMFAFn     func(ctx context.Context, userID, tenantID, code string) error
	disableMFAFn    func(ctx context.Context, userID, tenantID string) error
	listSessionsFn  func(ctx context.Context, userID, tenantID string) ([]authbff.Session, error)
	revokeSessionFn func(ctx context.Context, userID, tenantID, sessionID string) error
	deleteTenantFn  func(ctx context.Context, userID, tenantID, confirmation string) error
}
// Implement all interface methods delegating to Fn fields ...

func TestAccountHandler_GetProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mock := &mockAuthBFFClient{
		getProfileFn: func(ctx context.Context, userID, tenantID string) (*authbff.AccountProfile, error) {
			return &authbff.AccountProfile{
				UserID: userID, Email: "test@example.com", Name: "Test", MFAEnabled: false,
			}, nil
		},
	}
	handler := admin.NewAccountHandler(mock, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", "user-1")
	c.Set("tenant_id", "tenant-1")
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/account", nil)

	handler.GetProfile(c)
	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "test@example.com", body["email"])
}

func TestAccountHandler_UpdateProfile_Validation(t *testing.T) {
	// Test: empty name returns 400
	// Test: invalid email returns 400
	// Test: valid input returns 200
}

func TestAccountHandler_EnableMFA(t *testing.T) {
	// Test: returns QR code URL + secret
}

func TestAccountHandler_DeleteTenant_OwnerOnly(t *testing.T) {
	// Test: non-owner role returns 403
	// Test: wrong confirmation text returns 400
	// Test: correct owner + confirmation returns 204
}
```

- [ ] **2b.** Create `services/marketplace-api/internal/handlers/admin/account.go`

```go
package admin

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/authbff"
)

// AccountHandler handles account & security endpoints. All MFA and
// session operations are proxied to auth-bff — this handler never
// touches GIP directly.
type AccountHandler struct {
	client authbff.Client
	logger *slog.Logger
}

// NewAccountHandler constructs an AccountHandler.
func NewAccountHandler(client authbff.Client, logger *slog.Logger) *AccountHandler {
	return &AccountHandler{client: client, logger: logger}
}

// GetProfile handles GET /admin/account.
func (h *AccountHandler) GetProfile(c *gin.Context) {
	userID, _ := c.Get("user_id")
	tenantID, _ := c.Get("tenant_id")

	profile, err := h.client.GetProfile(c.Request.Context(), userID.(string), tenantID.(string))
	if err != nil {
		h.logger.Error("get profile", "user_id", userID, "err", err)
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{
			"error":   "upstream_error",
			"message": "Failed to fetch account profile",
		})
		return
	}
	c.JSON(http.StatusOK, profile)
}

// updateProfileRequest is the JSON body for PATCH /admin/account.
type updateProfileRequest struct {
	Name  *string `json:"name"`
	Email *string `json:"email"`
}

// UpdateProfile handles PATCH /admin/account.
func (h *AccountHandler) UpdateProfile(c *gin.Context) {
	userID, _ := c.Get("user_id")
	tenantID, _ := c.Get("tenant_id")

	var req updateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "validation",
			"message": "Invalid request body",
		})
		return
	}

	// At least one field must be provided.
	if req.Name == nil && req.Email == nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "validation",
			"message": "At least one of name or email must be provided",
		})
		return
	}

	name := ""
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
		if name == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": "validation", "message": "Name cannot be empty",
			})
			return
		}
	}
	email := ""
	if req.Email != nil {
		email = strings.TrimSpace(*req.Email)
		if email == "" || !strings.Contains(email, "@") {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": "validation", "message": "Valid email is required",
			})
			return
		}
	}

	if err := h.client.UpdateProfile(c.Request.Context(), userID.(string), tenantID.(string), name, email); err != nil {
		h.logger.Error("update profile", "user_id", userID, "err", err)
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{
			"error": "upstream_error", "message": "Failed to update profile",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Profile updated"})
}

// EnableMFA handles POST /admin/account/mfa/enable.
func (h *AccountHandler) EnableMFA(c *gin.Context) {
	userID, _ := c.Get("user_id")
	tenantID, _ := c.Get("tenant_id")

	enrollment, err := h.client.EnableMFA(c.Request.Context(), userID.(string), tenantID.(string))
	if err != nil {
		h.logger.Error("enable mfa", "user_id", userID, "err", err)
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{
			"error": "upstream_error", "message": "Failed to enable MFA",
		})
		return
	}
	c.JSON(http.StatusOK, enrollment)
}

// verifyMFARequest is the JSON body for POST /admin/account/mfa/verify.
type verifyMFARequest struct {
	Code string `json:"code" binding:"required"`
}

// VerifyMFA handles POST /admin/account/mfa/verify.
func (h *AccountHandler) VerifyMFA(c *gin.Context) {
	userID, _ := c.Get("user_id")
	tenantID, _ := c.Get("tenant_id")

	var req verifyMFARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "validation", "message": "TOTP code is required",
		})
		return
	}

	if err := h.client.VerifyMFA(c.Request.Context(), userID.(string), tenantID.(string), req.Code); err != nil {
		h.logger.Error("verify mfa", "user_id", userID, "err", err)
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "invalid_code", "message": "Invalid verification code",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "MFA enabled successfully"})
}

// DisableMFA handles POST /admin/account/mfa/disable.
func (h *AccountHandler) DisableMFA(c *gin.Context) {
	userID, _ := c.Get("user_id")
	tenantID, _ := c.Get("tenant_id")

	if err := h.client.DisableMFA(c.Request.Context(), userID.(string), tenantID.(string)); err != nil {
		h.logger.Error("disable mfa", "user_id", userID, "err", err)
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{
			"error": "upstream_error", "message": "Failed to disable MFA",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "MFA disabled"})
}

// ListSessions handles GET /admin/account/sessions.
func (h *AccountHandler) ListSessions(c *gin.Context) {
	userID, _ := c.Get("user_id")
	tenantID, _ := c.Get("tenant_id")

	sessions, err := h.client.ListSessions(c.Request.Context(), userID.(string), tenantID.(string))
	if err != nil {
		h.logger.Error("list sessions", "user_id", userID, "err", err)
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{
			"error": "upstream_error", "message": "Failed to fetch sessions",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sessions": sessions})
}

// RevokeSession handles DELETE /admin/account/sessions/:id.
func (h *AccountHandler) RevokeSession(c *gin.Context) {
	userID, _ := c.Get("user_id")
	tenantID, _ := c.Get("tenant_id")
	sessionID := c.Param("id")
	if sessionID == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "validation", "message": "Session ID is required",
		})
		return
	}

	if err := h.client.RevokeSession(c.Request.Context(), userID.(string), tenantID.(string), sessionID); err != nil {
		h.logger.Error("revoke session", "user_id", userID, "session_id", sessionID, "err", err)
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{
			"error": "upstream_error", "message": "Failed to revoke session",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Session revoked"})
}

// deleteAccountRequest is the JSON body for DELETE /admin/account.
type deleteAccountRequest struct {
	Confirmation string `json:"confirmation" binding:"required"`
}

// DeleteAccount handles DELETE /admin/account.
// Owner-only — enforced by authz middleware at route level.
func (h *AccountHandler) DeleteAccount(c *gin.Context) {
	userID, _ := c.Get("user_id")
	tenantID, _ := c.Get("tenant_id")

	var req deleteAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "validation", "message": "Confirmation text is required",
		})
		return
	}

	if req.Confirmation != "delete my store" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "confirmation_mismatch",
			"message": "Please type 'delete my store' to confirm",
		})
		return
	}

	if err := h.client.DeleteTenant(c.Request.Context(), userID.(string), tenantID.(string), req.Confirmation); err != nil {
		h.logger.Error("delete tenant", "user_id", userID, "tenant_id", tenantID, "err", err)
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{
			"error": "upstream_error", "message": "Failed to delete account",
		})
		return
	}
	c.Status(http.StatusNoContent)
}
```

**Verification:**
```bash
cd services/marketplace-api && go test ./internal/handlers/admin/... -run TestAccount -v -count=1
```

---

### Task 3 — Config + wiring in `main.go` and `routes.go`

- [ ] **3a.** Add env vars to `services/marketplace-api/pkg/config/config.go`:

```go
// AuthBFFURL is the internal base URL for auth-bff. Empty disables the
// account handler (fine for dev if auth-bff is not running).
AuthBFFURL string `envconfig:"AUTH_BFF_URL" default:""`
// TenantServiceURL is the internal base URL for tenant-service. Empty
// disables the delete-tenant flow.
TenantServiceURL string `envconfig:"TENANT_SERVICE_URL" default:""`
```

- [ ] **3b.** Add `AccountHandler` to `admin.Deps` in `services/marketplace-api/internal/handlers/admin/routes.go`:

```go
type Deps struct {
	// ... existing fields ...
	AccountHandler *AccountHandler // S1: account & security
}
```

- [ ] **3c.** Add account route group in `RegisterAdmin` (in `routes.go`):

```go
// Account & Security (S1) — tenant-wide routes, not store-scoped.
if deps.AccountHandler != nil {
	account := router.Group("/admin/account", authMW)
	{
		account.GET("",
			deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff),
			deps.AccountHandler.GetProfile)
		account.PATCH("",
			deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
			deps.AccountHandler.UpdateProfile)

		mfa := account.Group("/mfa")
		{
			mfa.POST("/enable",
				deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
				deps.AccountHandler.EnableMFA)
			mfa.POST("/verify",
				deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
				deps.AccountHandler.VerifyMFA)
			mfa.POST("/disable",
				deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
				deps.AccountHandler.DisableMFA)
		}

		sessions := account.Group("/sessions")
		{
			sessions.GET("",
				deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff),
				deps.AccountHandler.ListSessions)
			sessions.DELETE("/:id",
				deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
				deps.AccountHandler.RevokeSession)
		}

		account.DELETE("",
			deps.AuthzMiddleware.RequireTenantRelation(authz.RoleOwner),
			deps.AccountHandler.DeleteAccount)
	}
}
```

- [ ] **3d.** Wire in `cmd/marketplace-api/main.go` (inside the `if m == mode.Admin || m == mode.Both` block, after the settings handlers):

```go
// Account & Security handler (S1).
var accountHandler *admin.AccountHandler
if cfg.AuthBFFURL != "" {
	authBFFClient := authbff.New(cfg.AuthBFFURL, cfg.TenantServiceURL, cfg.InternalAuthSecret)
	accountHandler = admin.NewAccountHandler(authBFFClient, log)
	log.Info("account: wired auth-bff proxy", "url", cfg.AuthBFFURL)
} else {
	log.Info("account: auth-bff proxy disabled (AUTH_BFF_URL is empty)")
}
```

Then add to `adminDeps`:
```go
adminDeps = admin.Deps{
	// ... existing ...
	AccountHandler: accountHandler,
}
```

**Verification:**
```bash
cd services/marketplace-api && go build ./cmd/marketplace-api/
```

---

### Task 4 — Frontend API client (`apps/admin/lib/api/account-api.ts`)

- [ ] **4a.** Create `apps/admin/lib/api/account-api.ts`:

```typescript
// apps/admin/lib/api/account-api.ts
//
// Typed API client for account & security endpoints (S1).
// Follows the same calling convention as settings-api.ts.

import type { SessionHeaders } from "./marketplace-api";

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

// ─────────────────────────────────────────────────────────────────────
// Wire DTOs
// ─────────────────────────────────────────────────────────────────────

export interface AccountProfile {
  user_id: string;
  email: string;
  name: string;
  mfa_enabled: boolean;
  created_at: string;
}

export interface MFAEnrollment {
  qr_code_url: string;
  secret: string;
}

export interface ActiveSession {
  id: string;
  device: string;
  ip: string;
  last_active: string;
  current: boolean;
}

// ─────────────────────────────────────────────────────────────────────
// API functions
// ─────────────────────────────────────────────────────────────────────

function buildHeaders(session: SessionHeaders): HeadersInit {
  return {
    "Content-Type": "application/json",
    "X-User-Id": session.userId,
    "X-Tenant-Id": session.tenantId,
  };
}

export async function getAccountProfile(
  session: SessionHeaders,
): Promise<AccountProfile> {
  const res = await fetch(`${MARKETPLACE_API_URL}/api/v1/admin/account`, {
    headers: buildHeaders(session),
    cache: "no-store",
  });
  if (!res.ok) throw new Error(`Failed to fetch profile: ${res.status}`);
  return res.json();
}

export async function updateAccountProfile(
  session: SessionHeaders,
  data: { name?: string; email?: string },
): Promise<void> {
  const res = await fetch(`${MARKETPLACE_API_URL}/api/v1/admin/account`, {
    method: "PATCH",
    headers: buildHeaders(session),
    body: JSON.stringify(data),
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.message ?? `Failed to update profile: ${res.status}`);
  }
}

export async function enableMFA(
  session: SessionHeaders,
): Promise<MFAEnrollment> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/account/mfa/enable`,
    { method: "POST", headers: buildHeaders(session) },
  );
  if (!res.ok) throw new Error(`Failed to enable MFA: ${res.status}`);
  return res.json();
}

export async function verifyMFA(
  session: SessionHeaders,
  code: string,
): Promise<void> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/account/mfa/verify`,
    {
      method: "POST",
      headers: buildHeaders(session),
      body: JSON.stringify({ code }),
    },
  );
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.message ?? "Invalid verification code");
  }
}

export async function disableMFA(session: SessionHeaders): Promise<void> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/account/mfa/disable`,
    { method: "POST", headers: buildHeaders(session) },
  );
  if (!res.ok) throw new Error(`Failed to disable MFA: ${res.status}`);
}

export async function listSessions(
  session: SessionHeaders,
): Promise<ActiveSession[]> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/account/sessions`,
    { headers: buildHeaders(session), cache: "no-store" },
  );
  if (!res.ok) throw new Error(`Failed to list sessions: ${res.status}`);
  const data = await res.json();
  return data.sessions;
}

export async function revokeSession(
  session: SessionHeaders,
  sessionId: string,
): Promise<void> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/account/sessions/${sessionId}`,
    { method: "DELETE", headers: buildHeaders(session) },
  );
  if (!res.ok) throw new Error(`Failed to revoke session: ${res.status}`);
}

export async function deleteAccount(
  session: SessionHeaders,
  confirmation: string,
): Promise<void> {
  const res = await fetch(`${MARKETPLACE_API_URL}/api/v1/admin/account`, {
    method: "DELETE",
    headers: buildHeaders(session),
    body: JSON.stringify({ confirmation }),
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.message ?? `Failed to delete account: ${res.status}`);
  }
}
```

**Verification:** TypeScript compiles without errors.
```bash
cd apps/admin && npx tsc --noEmit
```

---

### Task 5 — Server actions (`apps/admin/app/settings/account/actions.ts`)

- [ ] **5a.** Create `apps/admin/app/settings/account/actions.ts`:

```typescript
"use server";

import { headers } from "next/headers";
import { revalidatePath } from "next/cache";

import {
  updateAccountProfile,
  enableMFA,
  verifyMFA,
  disableMFA,
  revokeSession,
  deleteAccount,
} from "@/lib/api/account-api";
import { canEditSettings } from "@/lib/auth/serverSession";
import type { TenantRole } from "@/lib/api/platform-api";

export type ActionResult =
  | { ok: true }
  | { ok: false; code: string; message: string };

export type MFAResult =
  | { ok: true; qr_code_url: string; secret: string }
  | { ok: false; code: string; message: string };

async function getSession() {
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  const role = (h.get("x-session-role") ?? "viewer") as TenantRole;
  return { userId, tenantId, role };
}

export async function saveProfile(
  name: string,
  email: string,
): Promise<ActionResult> {
  const { userId, tenantId, role } = await getSession();
  if (!userId || !tenantId) {
    return { ok: false, code: "no_session", message: "Session expired." };
  }
  if (!canEditSettings(role)) {
    return { ok: false, code: "forbidden", message: "Insufficient permissions." };
  }
  try {
    await updateAccountProfile({ userId, tenantId }, { name, email });
    revalidatePath("/settings/account");
    return { ok: true };
  } catch (error: unknown) {
    const msg = error instanceof Error ? error.message : "Update failed";
    return { ok: false, code: "error", message: msg };
  }
}

export async function startMFAEnrollment(): Promise<MFAResult> {
  const { userId, tenantId, role } = await getSession();
  if (!userId || !tenantId) {
    return { ok: false, code: "no_session", message: "Session expired." };
  }
  if (!canEditSettings(role)) {
    return { ok: false, code: "forbidden", message: "Insufficient permissions." };
  }
  try {
    const enrollment = await enableMFA({ userId, tenantId });
    return { ok: true, qr_code_url: enrollment.qr_code_url, secret: enrollment.secret };
  } catch (error: unknown) {
    const msg = error instanceof Error ? error.message : "MFA enrollment failed";
    return { ok: false, code: "error", message: msg };
  }
}

export async function completeMFAVerification(code: string): Promise<ActionResult> {
  const { userId, tenantId } = await getSession();
  if (!userId || !tenantId) {
    return { ok: false, code: "no_session", message: "Session expired." };
  }
  try {
    await verifyMFA({ userId, tenantId }, code);
    revalidatePath("/settings/account");
    return { ok: true };
  } catch (error: unknown) {
    const msg = error instanceof Error ? error.message : "Verification failed";
    return { ok: false, code: "error", message: msg };
  }
}

export async function turnOffMFA(): Promise<ActionResult> {
  const { userId, tenantId, role } = await getSession();
  if (!userId || !tenantId) {
    return { ok: false, code: "no_session", message: "Session expired." };
  }
  if (!canEditSettings(role)) {
    return { ok: false, code: "forbidden", message: "Insufficient permissions." };
  }
  try {
    await disableMFA({ userId, tenantId });
    revalidatePath("/settings/account");
    return { ok: true };
  } catch (error: unknown) {
    const msg = error instanceof Error ? error.message : "Disable MFA failed";
    return { ok: false, code: "error", message: msg };
  }
}

export async function revokeSessionAction(sessionId: string): Promise<ActionResult> {
  const { userId, tenantId } = await getSession();
  if (!userId || !tenantId) {
    return { ok: false, code: "no_session", message: "Session expired." };
  }
  try {
    await revokeSession({ userId, tenantId }, sessionId);
    revalidatePath("/settings/account");
    return { ok: true };
  } catch (error: unknown) {
    const msg = error instanceof Error ? error.message : "Revoke failed";
    return { ok: false, code: "error", message: msg };
  }
}

export async function deleteAccountAction(confirmation: string): Promise<ActionResult> {
  const { userId, tenantId, role } = await getSession();
  if (!userId || !tenantId) {
    return { ok: false, code: "no_session", message: "Session expired." };
  }
  if (role !== "owner") {
    return { ok: false, code: "forbidden", message: "Only the account owner can delete the store." };
  }
  if (confirmation !== "delete my store") {
    return { ok: false, code: "confirmation", message: "Please type 'delete my store' to confirm." };
  }
  try {
    await deleteAccount({ userId, tenantId }, confirmation);
    return { ok: true };
  } catch (error: unknown) {
    const msg = error instanceof Error ? error.message : "Delete failed";
    return { ok: false, code: "error", message: msg };
  }
}
```

---

### Task 6 — Account settings page (`apps/admin/app/settings/account/page.tsx`)

- [ ] **6a.** Create `apps/admin/app/settings/account/page.tsx`:

```tsx
import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { getAccountProfile, listSessions } from "@/lib/api/account-api";
import { AccountProfileForm } from "@/components/settings/AccountProfileForm";
import { AccountMfaSection } from "@/components/settings/AccountMfaSection";
import { AccountSessionsList } from "@/components/settings/AccountSessionsList";
import { AccountDangerZone } from "@/components/settings/AccountDangerZone";

export default async function AccountSettingsPage() {
  const {
    tenantName,
    email,
    role,
    memberships,
    tenantId,
    userId,
  } = await getServerSessionContext();

  const editable = role === "owner" || role === "admin";

  // Parallel fetch — profile and sessions are independent.
  const [profile, sessions] = await Promise.all([
    getAccountProfile({ userId, tenantId }).catch(() => null),
    listSessions({ userId, tenantId }).catch(() => []),
  ]);

  return (
    <AdminShell
      tenantName={tenantName}
      userEmail={email}
      role={role}
      memberships={memberships}
      currentTenantId={tenantId}
    >
      <div className="mx-auto w-full max-w-5xl space-y-10">
        <header className="space-y-3">
          <p className="eyebrow">Account</p>
          <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-5xl font-medium tracking-tight text-foreground">
            Account & Security
          </h1>
          <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
            Manage your profile, multi-factor authentication, and active sessions.
          </p>
        </header>

        {/* Profile section */}
        <section>
          <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-foreground">
            Profile
          </h2>
          <div className="mt-1 border-t border-border-subtle" />
          <div className="mt-6">
            <AccountProfileForm
              name={profile?.name ?? ""}
              email={profile?.email ?? email ?? ""}
              editable={editable}
            />
          </div>
        </section>

        {/* MFA section */}
        <section>
          <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-foreground">
            Multi-factor authentication
          </h2>
          <div className="mt-1 border-t border-border-subtle" />
          <div className="mt-6">
            <AccountMfaSection
              mfaEnabled={profile?.mfa_enabled ?? false}
              editable={editable}
            />
          </div>
        </section>

        {/* Sessions section */}
        <section>
          <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-foreground">
            Active sessions
          </h2>
          <div className="mt-1 border-t border-border-subtle" />
          <div className="mt-6">
            <AccountSessionsList sessions={sessions} />
          </div>
        </section>

        {/* Danger zone — owner only */}
        {role === "owner" && (
          <section>
            <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-[color:var(--danger)]">
              Danger zone
            </h2>
            <div className="mt-1 border-t border-[color:var(--danger)]" />
            <div className="mt-6">
              <AccountDangerZone />
            </div>
          </section>
        )}
      </div>
    </AdminShell>
  );
}
```

---

### Task 7 — Client components

- [ ] **7a.** Create `apps/admin/components/settings/AccountProfileForm.tsx` — inline-edit form with name + email fields, save button, loading/error states. Uses `saveProfile` server action.

- [ ] **7b.** Create `apps/admin/components/settings/AccountMfaSection.tsx` — shows MFA status badge (moss-700 "Enabled" / ink-900/40 "Disabled"), enable button opens modal with QR code from `startMFAEnrollment`, TOTP input field calls `completeMFAVerification`, disable button calls `turnOffMFA` with confirmation dialog.

- [ ] **7c.** Create `apps/admin/components/settings/AccountSessionsList.tsx` — table with columns: Device, IP, Last Active, Status (current badge), Revoke button. Uses `revokeSessionAction`.

- [ ] **7d.** Create `apps/admin/components/settings/AccountDangerZone.tsx` — red-bordered card with "Delete store" button. Opens a Dialog from `@tesserix/web` requiring typed confirmation "delete my store". Calls `deleteAccountAction`, then redirects to `/logout` on success.

Each component follows Paper · Ink · Moss editorial: Source Serif 4 headings, hairline rules, ink-900 buttons, `@tesserix/web` Dialog/Input/Button primitives.

---

### Task 8 — Sidebar nav update

- [ ] **8a.** Edit `apps/admin/components/shell/AdminShell.tsx` — add three new nav items under the settings section:

```typescript
// In the navigation array, settings.children:
{ label: "Domains", href: "/settings/domains" },
{ label: "Subscription", href: "/settings/subscription" },
{ label: "Account", href: "/settings/account" },
{ label: "Audit Logs", href: "/settings/audit-logs" },
{ label: "Notifications", href: "/settings/notifications" },
```

For S1 specifically, add `{ label: "Account", href: "/settings/account" }` at the end of the settings children array (after Tax).

- [ ] **8b.** Update `getPageTitle` in `AdminShell.tsx`:

```typescript
if (pathname.startsWith("/settings/account")) {
  return { eyebrow: "Account", title: "Account & Security" };
}
```

---

### Task 9 — Verification

- [ ] **9a.** Run backend tests:
```bash
cd services/marketplace-api && go test ./internal/authbff/... ./internal/handlers/admin/... -v -count=1
```

- [ ] **9b.** Run frontend type check:
```bash
cd apps/admin && npx tsc --noEmit
```

- [ ] **9c.** Run backend build:
```bash
cd services/marketplace-api && go build ./cmd/marketplace-api/
```

- [ ] **9d.** Smoke test: start marketplace-api with `AUTH_BFF_URL=http://localhost:8081` and verify `GET /api/v1/admin/account` returns 502 (auth-bff not running) rather than 404 (route not registered).

---

## Estimated effort

| Task | Description | Estimate |
|------|-------------|----------|
| 1 | Auth-BFF proxy client | 45 min |
| 2 | Account handler | 45 min |
| 3 | Config + wiring | 20 min |
| 4 | Frontend API client | 20 min |
| 5 | Server actions | 25 min |
| 6 | Account settings page | 30 min |
| 7 | Client components (4 files) | 60 min |
| 8 | Sidebar nav update | 10 min |
| 9 | Verification | 15 min |
| **Total** | | **~4.5 hours** |
