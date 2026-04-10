# Mobile Admin App Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an Expo React Native admin app that lets merchants manage orders, products (with camera CRUD), customers, and receive push notifications — all plans, one App Store listing.

**Architecture:** Expo Router app at `apps/mobile-admin/` with shared mobile logic in `packages/mobile-shared/`. Backend gets a new `GIPBearerAuth` middleware and `/api/v1/mobile/admin/` route group that reuses all existing admin handlers. Push notifications via Pub/Sub push subscription + Expo Push API.

**Tech Stack:** Expo ~52, Expo Router, React Native, TypeScript, @tesserix/native, @tesserix/tokens, @tesserix/hooks, @tesserix/icons, @react-native-firebase/auth, TanStack React Query, Zustand, Zod. Backend: Go 1.26, Gin, GORM, GIP token verification.

**Spec:** `docs/superpowers/specs/2026-04-10-mobile-admin-app-design.md`

---

## File Structure

### Backend (services/marketplace-api)

| File | Action | Responsibility |
|------|--------|---------------|
| `internal/auth/gip_bearer.go` | Create | GIP Bearer token verification middleware |
| `internal/auth/gip_bearer_test.go` | Create | Tests for Bearer auth middleware |
| `internal/handlers/admin/mobile_routes.go` | Create | `RegisterAdminMobile()` — mobile route group with Bearer auth |
| `internal/handlers/admin/push_tokens.go` | Create | Push token CRUD handler |
| `internal/handlers/admin/push_tokens_test.go` | Create | Push token handler tests |
| `internal/push/sender.go` | Create | Expo Push API client |
| `internal/push/webhook.go` | Create | Pub/Sub push subscription HTTP handler |
| `internal/push/sender_test.go` | Create | Push sender tests |
| `migrations/000020_push_tokens.up.sql` | Create | Push tokens table migration |
| `migrations/000020_push_tokens.down.sql` | Create | Push tokens rollback |
| `migrations.go:17` | Modify | Bump `ExpectedSchemaVersion` from 19 to 20 |
| `pkg/config/config.go:14` | Modify | Add `GIPProjectID` config field |
| `cmd/marketplace-api/main.go:553` | Modify | Wire `RegisterAdminMobile` + push handlers |

### Shared Mobile Package (packages/mobile-shared)

| File | Action | Responsibility |
|------|--------|---------------|
| `package.json` | Create | Package manifest with deps |
| `tsconfig.json` | Create | TypeScript config |
| `api/client.ts` | Create | Base HTTP client — Bearer auth, tenant context, error handling |
| `api/types.ts` | Create | Shared API request/response types |
| `api/orders.ts` | Create | Order list + detail endpoints |
| `api/products.ts` | Create | Product list + detail endpoints |
| `api/customers.ts` | Create | Customer list + detail endpoints |
| `api/dashboard.ts` | Create | Dashboard endpoint |
| `api/notifications.ts` | Create | Notification list + push token registration |
| `auth/gip.ts` | Create | Firebase/GIP auth wrapper (configurable tenant pool) |
| `auth/provider.tsx` | Create | AuthProvider context |
| `auth/token-storage.ts` | Create | expo-secure-store wrapper |
| `push/registration.ts` | Create | Expo push token registration + permission flow |
| `stores/auth-store.ts` | Create | Zustand auth state |
| `stores/tenant-store.ts` | Create | Zustand tenant/store context |

### Mobile Admin App (apps/mobile-admin)

| File | Action | Responsibility |
|------|--------|---------------|
| `package.json` | Create | Expo app manifest |
| `tsconfig.json` | Create | TypeScript config |
| `app.json` | Create | Expo config (name, scheme, plugins) |
| `app/_layout.tsx` | Create | Root layout — providers, auth gate, splash |
| `app/login.tsx` | Create | Login screen |
| `app/(tabs)/_layout.tsx` | Create | TabBar with 5 tabs |
| `app/(tabs)/index.tsx` | Create | Dashboard screen |
| `app/(tabs)/orders/_layout.tsx` | Create | Orders layout with SegmentedControl |
| `app/(tabs)/orders/index.tsx` | Create | Order list |
| `app/(tabs)/orders/[id].tsx` | Create | Order detail + actions |
| `app/(tabs)/products/_layout.tsx` | Create | Products layout with SegmentedControl |
| `app/(tabs)/products/index.tsx` | Create | Product list |
| `app/(tabs)/products/[id].tsx` | Create | Product detail + edit |
| `app/(tabs)/products/new.tsx` | Create | Product creation wizard |
| `app/(tabs)/customers/index.tsx` | Create | Customer list |
| `app/(tabs)/customers/[id].tsx` | Create | Customer detail |
| `app/(tabs)/more/index.tsx` | Create | More menu |
| `app/(tabs)/more/notifications.tsx` | Create | Notification feed |
| `app/(tabs)/more/account.tsx` | Create | Account + store switcher |
| `components/OrderRow.tsx` | Create | Order list row component |
| `components/ProductRow.tsx` | Create | Product list row component |
| `components/CustomerRow.tsx` | Create | Customer list row component |
| `components/DashboardStats.tsx` | Create | Dashboard stats cards |
| `components/ProductMediaPicker.tsx` | Create | Camera + gallery picker |
| `components/StoreSelector.tsx` | Create | Store switcher bottom sheet |
| `lib/admin-api/order-actions.ts` | Create | Confirm, fulfill, cancel, refund |
| `lib/admin-api/product-crud.ts` | Create | Create, update, archive, media upload |
| `lib/admin-api/customer-actions.ts` | Create | Block/unblock |
| `lib/hooks/use-store.ts` | Create | Active store hook |
| `lib/hooks/use-push.ts` | Create | Push notification setup hook |

---

## Phase 1: Backend — GIP Bearer Auth + Mobile Routes

### Task 1: GIP Bearer token verification middleware

**Files:**
- Create: `services/marketplace-api/internal/auth/gip_bearer.go`
- Create: `services/marketplace-api/internal/auth/gip_bearer_test.go`
- Modify: `services/marketplace-api/pkg/config/config.go:14`

- [ ] **Step 1: Add GIPProjectID to Config**

In `services/marketplace-api/pkg/config/config.go`, add after the `CustomerSessionSecret` field:

```go
// GIPProjectID is the Google Identity Platform project ID used to verify
// mobile Bearer tokens. When empty, GIPBearerAuth rejects all requests —
// fine for dev environments that don't use mobile auth.
GIPProjectID string `envconfig:"GIP_PROJECT_ID" default:""`
```

- [ ] **Step 2: Write failing test for GIPBearerAuth**

Create `services/marketplace-api/internal/auth/gip_bearer_test.go`:

```go
package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mark8ly/marketplace-api/internal/auth"
	"github.com/stretchr/testify/assert"
)

func gipRouter(verifier auth.TokenVerifier) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(auth.GIPBearerAuth(verifier))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"user_id":   c.GetString("user_id"),
			"tenant_id": c.GetString("tenant_id"),
		})
	})
	return r
}

func TestGIPBearerAuth_NoAuthHeader_Returns401(t *testing.T) {
	r := gipRouter(&auth.FakeVerifier{Err: auth.ErrNoToken})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, 401, w.Code)
}

func TestGIPBearerAuth_ValidToken_SetsContext(t *testing.T) {
	r := gipRouter(&auth.FakeVerifier{
		UserID:   "user-123",
		TenantID: "tenant-456",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "user-123")
	assert.Contains(t, w.Body.String(), "tenant-456")
}

func TestGIPBearerAuth_InvalidToken_Returns401(t *testing.T) {
	r := gipRouter(&auth.FakeVerifier{Err: auth.ErrInvalidToken})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer bad-token")
	r.ServeHTTP(w, req)
	assert.Equal(t, 401, w.Code)
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd services/marketplace-api && go test ./internal/auth/ -run TestGIPBearer -v`
Expected: compilation errors (types don't exist yet)

- [ ] **Step 4: Implement GIPBearerAuth middleware**

Create `services/marketplace-api/internal/auth/gip_bearer.go`:

```go
package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

var (
	ErrNoToken     = errors.New("no bearer token")
	ErrInvalidToken = errors.New("invalid token")
)

// TokenClaims holds the verified claims extracted from a GIP ID token.
type TokenClaims struct {
	UserID   string
	TenantID string
}

// TokenVerifier verifies a GIP ID token and returns its claims.
// In production this wraps Firebase Admin SDK; in tests a FakeVerifier.
type TokenVerifier interface {
	Verify(ctx context.Context, idToken string) (*TokenClaims, error)
}

// FakeVerifier is a test double for TokenVerifier.
type FakeVerifier struct {
	UserID   string
	TenantID string
	Err      error
}

func (f *FakeVerifier) Verify(_ context.Context, _ string) (*TokenClaims, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return &TokenClaims{UserID: f.UserID, TenantID: f.TenantID}, nil
}

// GIPBearerAuth returns a gin middleware that validates a GIP Bearer token.
// On success it sets "user_id" and "tenant_id" on the gin context — same
// contract as HeaderTrustAuth so downstream handlers work unchanged.
func GIPBearerAuth(verifier TokenVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "bearer token required",
			})
			return
		}
		idToken := strings.TrimPrefix(header, "Bearer ")

		claims, err := verifier.Verify(c.Request.Context(), idToken)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "invalid or expired token",
			})
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("tenant_id", claims.TenantID)
		c.Next()
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd services/marketplace-api && go test ./internal/auth/ -run TestGIPBearer -v`
Expected: all 3 tests PASS

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/auth/gip_bearer.go services/marketplace-api/internal/auth/gip_bearer_test.go services/marketplace-api/pkg/config/config.go
git commit -m "feat(marketplace-api): add GIPBearerAuth middleware for mobile app"
```

---

### Task 2: GIP token verifier production implementation

**Files:**
- Create: `services/marketplace-api/internal/auth/gip_verifier.go`

- [ ] **Step 1: Implement GIPVerifier wrapping Firebase Admin SDK**

Create `services/marketplace-api/internal/auth/gip_verifier.go`:

```go
package auth

import (
	"context"
	"fmt"

	firebaseAuth "firebase.google.com/go/v4/auth"
)

// GIPVerifier verifies GIP ID tokens using the Firebase Admin Auth client.
type GIPVerifier struct {
	client *firebaseAuth.Client
}

// NewGIPVerifier creates a GIPVerifier from a Firebase Auth client.
func NewGIPVerifier(client *firebaseAuth.Client) *GIPVerifier {
	return &GIPVerifier{client: client}
}

// Verify checks the token signature and extracts user_id and tenant_id claims.
func (v *GIPVerifier) Verify(ctx context.Context, idToken string) (*TokenClaims, error) {
	token, err := v.client.VerifyIDToken(ctx, idToken)
	if err != nil {
		return nil, fmt.Errorf("verify GIP token: %w", err)
	}

	userID := token.UID
	if userID == "" {
		return nil, ErrInvalidToken
	}

	// tenant_id is stored as a custom claim by auth-bff during login.
	tenantID, _ := token.Claims["tenant_id"].(string)
	if tenantID == "" {
		return nil, fmt.Errorf("token missing tenant_id claim")
	}

	return &TokenClaims{
		UserID:   userID,
		TenantID: tenantID,
	}, nil
}
```

- [ ] **Step 2: Add Firebase dependency and verify it compiles**

```bash
cd services/marketplace-api && go get firebase.google.com/go/v4
cd services/marketplace-api && go build ./internal/auth/
```
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/auth/gip_verifier.go services/marketplace-api/go.mod services/marketplace-api/go.sum
git commit -m "feat(marketplace-api): add GIPVerifier for production token verification"
```

---

### Task 3: Mobile admin route group

**Files:**
- Create: `services/marketplace-api/internal/handlers/admin/mobile_routes.go`
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go:553`

- [ ] **Step 1: Create RegisterAdminMobile**

Create `services/marketplace-api/internal/handlers/admin/mobile_routes.go`:

```go
package admin

import (
	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/auth"
	"github.com/mark8ly/marketplace-api/internal/authz"
)

// MobileDeps extends Deps with the mobile-specific GIP token verifier.
type MobileDeps struct {
	Deps
	TokenVerifier auth.TokenVerifier
}

// RegisterAdminMobile mounts the mobile admin route group. Uses GIPBearerAuth
// instead of HeaderTrustAuth. Same handlers, same authz, different auth.
// Includes per-user rate limiting since these routes are public-internet-facing.
func RegisterAdminMobile(router *gin.RouterGroup, deps MobileDeps) {
	if deps.TokenVerifier == nil {
		return // mobile routes disabled when no GIP config
	}

	bearerAuth := auth.GIPBearerAuth(deps.TokenVerifier)

	// Rate limiter: 60 requests/min per user_id (extracted post-auth).
	// Uses golang.org/x/time/rate or a simple in-memory token bucket.
	// For production, replace with Redis-backed limiter.
	rateLimiter := auth.NewPerUserRateLimiter(60, 1) // 60 req/min, burst 1

	// Tenant-wide routes
	if deps.StoresHandler != nil {
		mobileRoot := router.Group("/mobile/admin", bearerAuth, rateLimiter)
		mobileRoot.GET("/stores",
			deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff),
			deps.StoresHandler.List)
	}

	storeRoute := router.Group("/mobile/admin/stores/:storeId", bearerAuth, rateLimiter, deps.StoresMiddleware)
	{
		// Dashboard
		if deps.DashboardHandler != nil {
			storeRoute.GET("/dashboard",
				deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff),
				deps.DashboardHandler.Get)
		}

		// Products (full CRUD)
		products := storeRoute.Group("/products")
		{
			products.GET("", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff), deps.ProductHandler.List)
			products.POST("", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin), deps.ProductHandler.Create)
			products.GET("/:id", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff), deps.ProductHandler.Get)
			products.PATCH("/:id", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin), deps.ProductHandler.Patch)
		}

		// Product media
		if deps.MediaHandler != nil {
			products.POST("/:id/media", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin), deps.MediaHandler.Upload)
			products.DELETE("/:id/media/:mediaId", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin), deps.MediaHandler.Delete)
			products.PATCH("/:id/media/reorder", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin), deps.MediaHandler.Reorder)
		}

		// Product variants
		if deps.VariantHandler != nil {
			variants := products.Group("/:id/variants")
			{
				variants.GET("", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff), deps.VariantHandler.List)
				variants.POST("", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin), deps.VariantHandler.Create)
				variants.PATCH("/:variantId", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin), deps.VariantHandler.Patch)
			}
		}

		// Product categories
		if deps.CategoryHandler != nil {
			cats := storeRoute.Group("/categories")
			{
				cats.GET("", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff), deps.CategoryHandler.List)
			}
		}

		// Orders (list, detail, actions)
		if deps.OrdersHandler != nil {
			orders := storeRoute.Group("/orders")
			{
				orders.GET("", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff), deps.OrdersHandler.List)
				orders.GET("/:id", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff), deps.OrdersHandler.Get)
				orders.POST("/:id/confirm", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin), deps.OrdersHandler.Confirm)
				orders.POST("/:id/fulfill", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin), deps.OrdersHandler.Fulfill)
				orders.POST("/:id/cancel", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin), deps.OrdersHandler.Cancel)
				orders.POST("/:id/refund", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleOwner), deps.OrdersHandler.Refund)
			}
		}

		// Customers (list, detail, block/unblock)
		if deps.CustomersHandler != nil {
			customers := storeRoute.Group("/customers")
			{
				customers.GET("", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff), deps.CustomersHandler.List)
				customers.GET("/:id", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff), deps.CustomersHandler.Get)
				customers.POST("/:id/block", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin), deps.CustomersHandler.Block)
				customers.POST("/:id/unblock", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin), deps.CustomersHandler.Unblock)
			}
		}

		// Notifications
		if deps.NotificationsHandler != nil {
			notifs := storeRoute.Group("/notifications")
			{
				notifs.GET("", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff), deps.NotificationsHandler.List)
				notifs.POST("/mark-all-read", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff), deps.NotificationsHandler.MarkAllRead)
			}
		}
	}
}
```

- [ ] **Step 2: Wire into main.go**

Add the Firebase import at the top of main.go:

```go
firebase "firebase.google.com/go/v4"
```

**Important:** The mobile deps construction MUST be inside the admin mode guard (where `adminDeps` is built, ~line 139-339), not before the switch statement. Mobile routes only make sense when admin handlers exist.

Inside the admin deps construction block (after `adminDeps` is fully populated, before the switch at ~line 540), add:

```go
// Mobile admin deps — Bearer auth for external mobile clients.
// Only built when admin handlers exist (mode.Admin or mode.Both).
var tokenVerifier auth.TokenVerifier
if cfg.GIPProjectID != "" {
    firebaseApp, err := firebase.NewApp(context.Background(), &firebase.Config{
        ProjectID: cfg.GIPProjectID,
    })
    if err != nil {
        log.Error("failed to init Firebase app for mobile auth", "error", err)
    } else {
        authClient, err := firebaseApp.Auth(context.Background())
        if err != nil {
            log.Error("failed to init Firebase Auth client", "error", err)
        } else {
            tokenVerifier = auth.NewGIPVerifier(authClient)
        }
    }
}
mobileDeps := admin.MobileDeps{
    Deps:          adminDeps,
    TokenVerifier: tokenVerifier,
}
```

Then wire `RegisterAdminMobile` in **both** places where `RegisterAdmin` is called:

In the `mode.Both` case (~line 553), after `admin.RegisterAdmin(r.Group("/api/v1"), adminDeps)`:
```go
admin.RegisterAdminMobile(r.Group("/api/v1"), mobileDeps)
```

In the `mode.Admin` case (~line 568), after `admin.RegisterAdmin(engine.Group("/api/v1"), adminDeps)`:
```go
admin.RegisterAdminMobile(engine.Group("/api/v1"), mobileDeps)
```

Do NOT add mobile routes in the `mode.Storefront` case — mobile admin routes require admin handlers.

- [ ] **Step 3: Verify it compiles**

Run: `cd services/marketplace-api && go build ./cmd/marketplace-api/`
Expected: compiles successfully. If `firebase.google.com/go/v4` is missing, run `go get firebase.google.com/go/v4`.

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/handlers/admin/mobile_routes.go services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "feat(marketplace-api): add mobile admin route group with GIP Bearer auth"
```

---

### Task 4: Push token migration + handler

**Files:**
- Create: `services/marketplace-api/migrations/000020_push_tokens.up.sql`
- Create: `services/marketplace-api/migrations/000020_push_tokens.down.sql`
- Create: `services/marketplace-api/internal/push/model.go`
- Create: `services/marketplace-api/internal/push/repository.go`
- Create: `services/marketplace-api/internal/handlers/admin/push_tokens.go`
- Modify: `services/marketplace-api/migrations.go:17`
- Modify: `services/marketplace-api/internal/handlers/admin/mobile_routes.go`

- [ ] **Step 1: Create migration**

Create `services/marketplace-api/migrations/000020_push_tokens.up.sql`:

```sql
CREATE TABLE admin_push_tokens (
    id          UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID          NOT NULL,
    store_id    UUID          NOT NULL,
    user_id     UUID          NOT NULL,
    device_id   VARCHAR(100)  NOT NULL,
    token       TEXT          NOT NULL,
    platform    VARCHAR(10)   NOT NULL CHECK (platform IN ('ios', 'android')),
    created_at  TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (user_id, device_id),
    UNIQUE (token)
);
CREATE INDEX apt_store_idx ON admin_push_tokens (store_id);
```

Create `services/marketplace-api/migrations/000020_push_tokens.down.sql`:

```sql
DROP TABLE IF EXISTS admin_push_tokens;
```

- [ ] **Step 2: Bump expected schema version**

In `services/marketplace-api/migrations.go:17`, change:

```go
const ExpectedSchemaVersion uint = 19
```
to:
```go
const ExpectedSchemaVersion uint = 20
```

- [ ] **Step 3: Create push token model**

Create `services/marketplace-api/internal/push/model.go`:

```go
package push

import (
	"time"

	"github.com/google/uuid"
)

type Token struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID  uuid.UUID `gorm:"type:uuid;not null" json:"tenant_id"`
	StoreID   uuid.UUID `gorm:"type:uuid;not null" json:"store_id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null" json:"user_id"`
	DeviceID  string    `gorm:"type:varchar(100);not null" json:"device_id"`
	TokenStr  string    `gorm:"column:token;type:text;not null" json:"token"`
	Platform  string    `gorm:"type:varchar(10);not null" json:"platform"`
	CreatedAt time.Time `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null;default:now()" json:"updated_at"`
}

func (Token) TableName() string { return "admin_push_tokens" }
```

- [ ] **Step 4: Create push token repository**

Create `services/marketplace-api/internal/push/repository.go`:

```go
package push

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Upsert inserts or updates a push token by (user_id, device_id).
func (r *Repository) Upsert(t *Token) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "device_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"token", "platform", "store_id", "updated_at"}),
	}).Create(t).Error
}

// Delete removes a push token by ID scoped to a user.
func (r *Repository) Delete(userID, tokenID uuid.UUID) error {
	return r.db.Where("id = ? AND user_id = ?", tokenID, userID).Delete(&Token{}).Error
}

// DeleteByToken removes a push token by the token string (for stale cleanup).
func (r *Repository) DeleteByToken(tokenStr string) error {
	return r.db.Where("token = ?", tokenStr).Delete(&Token{}).Error
}

// ListByStore returns all push tokens for a given store.
func (r *Repository) ListByStore(storeID uuid.UUID) ([]Token, error) {
	var tokens []Token
	err := r.db.Where("store_id = ?", storeID).Find(&tokens).Error
	return tokens, err
}

// DeleteAllForUser removes all push tokens for a user (logout).
func (r *Repository) DeleteAllForUser(userID uuid.UUID) error {
	return r.db.Where("user_id = ?", userID).Delete(&Token{}).Error
}
```

- [ ] **Step 5: Create push token handler**

Create `services/marketplace-api/internal/handlers/admin/push_tokens.go`:

```go
package admin

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mark8ly/marketplace-api/internal/push"
)

type PushTokenHandler struct {
	repo   *push.Repository
	logger *slog.Logger
}

func NewPushTokenHandler(repo *push.Repository, logger *slog.Logger) *PushTokenHandler {
	return &PushTokenHandler{repo: repo, logger: logger}
}

type registerPushTokenRequest struct {
	Token    string `json:"token" binding:"required"`
	Platform string `json:"platform" binding:"required,oneof=ios android"`
	DeviceID string `json:"device_id" binding:"required,max=100"`
}

func (h *PushTokenHandler) Register(c *gin.Context) {
	var req registerPushTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": err.Error()})
		return
	}

	userID, _ := uuid.Parse(c.GetString("user_id"))
	tenantID, _ := uuid.Parse(c.GetString("tenant_id"))
	storeID, _ := uuid.Parse(c.Param("storeId"))

	token := &push.Token{
		TenantID: tenantID,
		StoreID:  storeID,
		UserID:   userID,
		DeviceID: req.DeviceID,
		TokenStr: req.Token,
		Platform: req.Platform,
	}

	if err := h.repo.Upsert(token); err != nil {
		h.logger.Error("push token upsert failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": "failed to register push token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": token.ID, "message": "registered"})
}

func (h *PushTokenHandler) Delete(c *gin.Context) {
	userID, _ := uuid.Parse(c.GetString("user_id"))
	tokenID, err := uuid.Parse(c.Param("tokenId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "invalid token ID"})
		return
	}

	if err := h.repo.Delete(userID, tokenID); err != nil {
		h.logger.Error("push token delete failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": "failed to delete push token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
```

- [ ] **Step 6: Add push token routes to mobile_routes.go**

In `services/marketplace-api/internal/handlers/admin/mobile_routes.go`, add `PushTokenHandler` to `MobileDeps`:

```go
type MobileDeps struct {
	Deps
	TokenVerifier    auth.TokenVerifier
	PushTokenHandler *PushTokenHandler
}
```

Add routes inside the `storeRoute` block:

```go
// Push tokens
if deps.PushTokenHandler != nil {
    pushTokens := storeRoute.Group("/push-tokens")
    {
        pushTokens.POST("", deps.PushTokenHandler.Register)
        pushTokens.DELETE("/:tokenId", deps.PushTokenHandler.Delete)
    }
}
```

- [ ] **Step 7: Wire PushTokenHandler in main.go**

In main.go, after push.Repository creation:

```go
pushRepo := push.NewRepository(conn)
pushTokenHandler := admin.NewPushTokenHandler(pushRepo, log)
```

Add to mobileDeps:

```go
mobileDeps := admin.MobileDeps{
    Deps:             adminDeps,
    TokenVerifier:    tokenVerifier,
    PushTokenHandler: pushTokenHandler,
}
```

- [ ] **Step 8: Verify compilation**

Run: `cd services/marketplace-api && go build ./cmd/marketplace-api/`
Expected: compiles

- [ ] **Step 9: Commit**

```bash
git add services/marketplace-api/migrations/000020_push_tokens.up.sql services/marketplace-api/migrations/000020_push_tokens.down.sql services/marketplace-api/migrations.go services/marketplace-api/internal/push/ services/marketplace-api/internal/handlers/admin/push_tokens.go services/marketplace-api/internal/handlers/admin/mobile_routes.go services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "feat(marketplace-api): add push token migration, repository, handler + mobile routes"
```

---

## Phase 2: Shared Mobile Package

### Task 5: packages/mobile-shared scaffold

**Files:**
- Create: `packages/mobile-shared/package.json`
- Create: `packages/mobile-shared/tsconfig.json`

- [ ] **Step 1: Create package.json**

Create `packages/mobile-shared/package.json`:

```json
{
  "name": "@repo/mobile-shared",
  "version": "0.0.0",
  "private": true,
  "main": "./api/client.ts",
  "types": "./api/client.ts",
  "exports": {
    "./api/*": "./api/*.ts",
    "./auth/*": "./auth/*.ts",
    "./auth/provider": "./auth/provider.tsx",
    "./push/*": "./push/*.ts",
    "./stores/*": "./stores/*.ts"
  },
  "dependencies": {
    "zustand": "^5.0.0",
    "zod": "^3.23.0"
  },
  "peerDependencies": {
    "react": "^19.0.0",
    "react-native": "*",
    "expo-secure-store": "*",
    "expo-notifications": "*",
    "expo-device": "*",
    "@react-native-firebase/auth": "*"
  },
  "devDependencies": {
    "typescript": "^5.9.0"
  }
}
```

- [ ] **Step 2: Create tsconfig.json**

Create `packages/mobile-shared/tsconfig.json`:

```json
{
  "extends": "../../tsconfig.json",
  "compilerOptions": {
    "jsx": "react-jsx",
    "strict": true,
    "noEmit": true,
    "paths": {
      "@/*": ["./*"]
    }
  },
  "include": ["**/*.ts", "**/*.tsx"],
  "exclude": ["node_modules"]
}
```

- [ ] **Step 3: Commit**

```bash
git add packages/mobile-shared/package.json packages/mobile-shared/tsconfig.json
git commit -m "chore: scaffold packages/mobile-shared"
```

---

### Task 6: API client + types

**Files:**
- Create: `packages/mobile-shared/api/client.ts`
- Create: `packages/mobile-shared/api/types.ts`

- [ ] **Step 1: Create base API client**

Create `packages/mobile-shared/api/client.ts`:

```typescript
import { z } from "zod";

export interface ApiClientConfig {
  baseUrl: string;
  getToken: () => Promise<string | null>;
  getStoreId: () => string | null;
}

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export function createApiClient(config: ApiClientConfig) {
  async function request<T>(
    method: string,
    path: string,
    options?: { body?: unknown; schema?: z.ZodType<T>; params?: Record<string, string> },
  ): Promise<T> {
    const token = await config.getToken();
    if (!token) throw new ApiError(401, "unauthorized", "Not authenticated");

    const storeId = config.getStoreId();
    const url = new URL(
      `/api/v1/mobile/admin${storeId ? `/stores/${storeId}` : ""}${path}`,
      config.baseUrl,
    );

    if (options?.params) {
      for (const [k, v] of Object.entries(options.params)) {
        url.searchParams.set(k, v);
      }
    }

    const res = await fetch(url.toString(), {
      method,
      headers: {
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json",
        Accept: "application/json",
      },
      body: options?.body ? JSON.stringify(options.body) : undefined,
    });

    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: "unknown", message: res.statusText }));
      throw new ApiError(res.status, err.error ?? "unknown", err.message ?? res.statusText);
    }

    const data = await res.json();
    if (options?.schema) return options.schema.parse(data);
    return data as T;
  }

  return {
    get: <T>(path: string, params?: Record<string, string>, schema?: z.ZodType<T>) =>
      request<T>("GET", path, { params, schema }),
    post: <T>(path: string, body?: unknown, schema?: z.ZodType<T>) =>
      request<T>("POST", path, { body, schema }),
    patch: <T>(path: string, body?: unknown, schema?: z.ZodType<T>) =>
      request<T>("PATCH", path, { body, schema }),
    delete: <T>(path: string) => request<T>("DELETE", path),
    uploadMedia: async (path: string, formData: FormData) => {
      const token = await config.getToken();
      if (!token) throw new ApiError(401, "unauthorized", "Not authenticated");
      const storeId = config.getStoreId();
      const url = `${config.baseUrl}/api/v1/mobile/admin/stores/${storeId}${path}`;
      const res = await fetch(url, {
        method: "POST",
        headers: { Authorization: `Bearer ${token}` },
        body: formData,
      });
      if (!res.ok) throw new ApiError(res.status, "upload_failed", "Media upload failed");
      return res.json();
    },
  };
}
```

- [ ] **Step 2: Create shared API types**

Create `packages/mobile-shared/api/types.ts`:

```typescript
export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  next_cursor: string | null;
  has_more: boolean;
}

export interface DashboardStats {
  revenue_today: number;
  revenue_week: number;
  revenue_month: number;
  revenue_change_pct: number;
  revenue_trend: number[];
  orders_today: number;
  orders_pending: number;
  orders_fulfilled: number;
  orders_cancelled: number;
  customers_total: number;
  customers_new_this_week: number;
  pending_reviews: number;
}

export interface RecentOrder {
  id: string;
  order_number: string;
  customer_email: string;
  grand_total: number;
  status: string;
  created_at: string;
}

export interface TopProduct {
  id: string;
  name: string;
  total_sold: number;
  revenue: number;
}

export interface LowStockItem {
  id: string;
  name: string;
  stock: number;
  thumbnail_url: string | null;
}

export interface SetupChecklist {
  has_products: boolean;
  has_payment: boolean;
  has_shipping: boolean;
  has_domain: boolean;
  has_branding: boolean;
}

export interface DashboardResponse {
  stats: DashboardStats;
  recent_orders: RecentOrder[];
  top_products: TopProduct[];
  low_stock: LowStockItem[];
  setup_checklist: SetupChecklist;
}

export interface Order {
  id: string;
  order_number: string;
  status: string;
  customer_email: string;
  customer_name: string;
  grand_total: number;
  item_count: number;
  created_at: string;
  updated_at: string;
}

export interface OrderDetail extends Order {
  line_items: LineItem[];
  shipping_address: Address | null;
  shipping_method: string | null;
  tracking_number: string | null;
  payment_method: string | null;
  payment_transaction_id: string | null;
  timeline: TimelineEvent[];
}

export interface LineItem {
  id: string;
  product_id: string;
  product_name: string;
  variant_name: string | null;
  quantity: number;
  unit_price: number;
  thumbnail_url: string | null;
}

export interface Address {
  line1: string;
  line2: string | null;
  city: string;
  state: string;
  postal_code: string;
  country: string;
}

export interface TimelineEvent {
  type: string;
  message: string;
  created_at: string;
}

export interface Product {
  id: string;
  name: string;
  status: string;
  price: number;
  compare_at_price: number | null;
  sku: string | null;
  stock: number;
  thumbnail_url: string | null;
  created_at: string;
}

export interface ProductDetail extends Product {
  description: string | null;
  barcode: string | null;
  category_id: string | null;
  category_name: string | null;
  tags: string[];
  media: MediaItem[];
  variants: Variant[];
}

export interface MediaItem {
  id: string;
  url: string;
  position: number;
}

export interface Variant {
  id: string;
  name: string;
  sku: string | null;
  price: number;
  stock: number;
}

export interface Customer {
  id: string;
  email: string;
  first_name: string;
  last_name: string;
  phone: string | null;
  order_count: number;
  total_spent: number;
  status: string;
  created_at: string;
}

export interface CustomerDetail extends Customer {
  avatar_url: string | null;
  average_order_value: number;
  recent_orders: RecentOrder[];
  review_count: number;
}

export interface Notification {
  id: string;
  type: string;
  title: string;
  body: string;
  read: boolean;
  deep_link: string | null;
  created_at: string;
}

export interface Store {
  id: string;
  name: string;
  slug: string;
}
```

- [ ] **Step 3: Commit**

```bash
git add packages/mobile-shared/api/
git commit -m "feat(mobile-shared): add API client and shared types"
```

---

### Task 7: Auth module

**Files:**
- Create: `packages/mobile-shared/auth/gip.ts`
- Create: `packages/mobile-shared/auth/token-storage.ts`
- Create: `packages/mobile-shared/auth/provider.tsx`

- [ ] **Step 1: Create GIP auth wrapper**

Create `packages/mobile-shared/auth/gip.ts`:

```typescript
import auth, { type FirebaseAuthTypes } from "@react-native-firebase/auth";

export interface GIPAuthConfig {
  tenantId: string; // 'mp-internal' for admin, 'mp-customer' for storefront
}

export function createGIPAuth(config: GIPAuthConfig) {
  const firebaseAuth = auth();
  firebaseAuth.tenantId = config.tenantId;

  return {
    signIn: (email: string, password: string) =>
      firebaseAuth.signInWithEmailAndPassword(email, password),

    signOut: () => firebaseAuth.signOut(),

    getIdToken: async (): Promise<string | null> => {
      const user = firebaseAuth.currentUser;
      if (!user) return null;
      return user.getIdToken(false);
    },

    getIdTokenForced: async (): Promise<string | null> => {
      const user = firebaseAuth.currentUser;
      if (!user) return null;
      return user.getIdToken(true);
    },

    getCurrentUser: (): FirebaseAuthTypes.User | null => firebaseAuth.currentUser,

    onAuthStateChanged: (callback: (user: FirebaseAuthTypes.User | null) => void) =>
      firebaseAuth.onAuthStateChanged(callback),

    sendPasswordResetEmail: (email: string) =>
      firebaseAuth.sendPasswordResetEmail(email),
  };
}

export type GIPAuth = ReturnType<typeof createGIPAuth>;
```

- [ ] **Step 2: Create token storage**

Create `packages/mobile-shared/auth/token-storage.ts`:

```typescript
import * as SecureStore from "expo-secure-store";

const KEYS = {
  TENANT_ID: "mark8ly_tenant_id",
  STORE_ID: "mark8ly_store_id",
  DEVICE_ID: "mark8ly_device_id",
} as const;

export const tokenStorage = {
  getTenantId: () => SecureStore.getItemAsync(KEYS.TENANT_ID),
  setTenantId: (id: string) => SecureStore.setItemAsync(KEYS.TENANT_ID, id),

  getStoreId: () => SecureStore.getItemAsync(KEYS.STORE_ID),
  setStoreId: (id: string) => SecureStore.setItemAsync(KEYS.STORE_ID, id),

  getDeviceId: () => SecureStore.getItemAsync(KEYS.DEVICE_ID),
  setDeviceId: (id: string) => SecureStore.setItemAsync(KEYS.DEVICE_ID, id),

  clearAll: async () => {
    await SecureStore.deleteItemAsync(KEYS.TENANT_ID);
    await SecureStore.deleteItemAsync(KEYS.STORE_ID);
    // Keep device_id — it persists across logins
  },
};
```

- [ ] **Step 3: Create auth provider**

Create `packages/mobile-shared/auth/provider.tsx`:

```tsx
import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import type { FirebaseAuthTypes } from "@react-native-firebase/auth";
import { type GIPAuth, createGIPAuth } from "./gip";
import { tokenStorage } from "./token-storage";

interface AuthState {
  user: FirebaseAuthTypes.User | null;
  loading: boolean;
  signIn: (email: string, password: string) => Promise<void>;
  signOut: () => Promise<void>;
  getToken: () => Promise<string | null>;
}

const AuthContext = createContext<AuthState | null>(null);

interface AuthProviderProps {
  tenantId: string;
  children: ReactNode;
}

export function AuthProvider({ tenantId, children }: AuthProviderProps) {
  const [gipAuth] = useState<GIPAuth>(() => createGIPAuth({ tenantId }));
  const [user, setUser] = useState<FirebaseAuthTypes.User | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const unsubscribe = gipAuth.onAuthStateChanged((firebaseUser) => {
      setUser(firebaseUser);
      setLoading(false);
    });
    return unsubscribe;
  }, [gipAuth]);

  const signIn = async (email: string, password: string) => {
    await gipAuth.signIn(email, password);
  };

  const signOut = async () => {
    await tokenStorage.clearAll();
    await gipAuth.signOut();
  };

  const getToken = () => gipAuth.getIdToken();

  return (
    <AuthContext.Provider value={{ user, loading, signIn, signOut, getToken }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
```

- [ ] **Step 4: Commit**

```bash
git add packages/mobile-shared/auth/
git commit -m "feat(mobile-shared): add auth module — GIP wrapper, token storage, AuthProvider"
```

---

### Task 8: Zustand stores + push registration

**Files:**
- Create: `packages/mobile-shared/stores/auth-store.ts`
- Create: `packages/mobile-shared/stores/tenant-store.ts`
- Create: `packages/mobile-shared/push/registration.ts`

- [ ] **Step 1: Create auth store**

Create `packages/mobile-shared/stores/auth-store.ts`:

```typescript
import { create } from "zustand";

interface AuthStoreState {
  isAuthenticated: boolean;
  userId: string | null;
  email: string | null;
  setAuthenticated: (userId: string, email: string) => void;
  clearAuth: () => void;
}

export const useAuthStore = create<AuthStoreState>((set) => ({
  isAuthenticated: false,
  userId: null,
  email: null,
  setAuthenticated: (userId, email) => set({ isAuthenticated: true, userId, email }),
  clearAuth: () => set({ isAuthenticated: false, userId: null, email: null }),
}));
```

- [ ] **Step 2: Create tenant store**

Create `packages/mobile-shared/stores/tenant-store.ts`:

```typescript
import { create } from "zustand";
import type { Store } from "../api/types";

interface TenantStoreState {
  tenantId: string | null;
  activeStore: Store | null;
  stores: Store[];
  setTenantId: (id: string) => void;
  setActiveStore: (store: Store) => void;
  setStores: (stores: Store[]) => void;
}

export const useTenantStore = create<TenantStoreState>((set) => ({
  tenantId: null,
  activeStore: null,
  stores: [],
  setTenantId: (id) => set({ tenantId: id }),
  setActiveStore: (store) => set({ activeStore: store }),
  setStores: (stores) => set({ stores }),
}));
```

- [ ] **Step 3: Create push registration**

Create `packages/mobile-shared/push/registration.ts`:

```typescript
import * as Notifications from "expo-notifications";
import { Platform } from "react-native";
import * as Device from "expo-device";
import { tokenStorage } from "../auth/token-storage";

export async function registerForPushNotifications(
  registerFn: (token: string, platform: string, deviceId: string) => Promise<void>,
): Promise<string | null> {
  if (!Device.isDevice) return null; // push doesn't work on simulators

  const { status: existing } = await Notifications.getPermissionsAsync();
  let finalStatus = existing;

  if (existing !== "granted") {
    const { status } = await Notifications.requestPermissionsAsync();
    finalStatus = status;
  }

  if (finalStatus !== "granted") return null;

  const pushToken = await Notifications.getExpoPushTokenAsync();
  const platform = Platform.OS === "ios" ? "ios" : "android";

  let deviceId = await tokenStorage.getDeviceId();
  if (!deviceId) {
    deviceId = `${platform}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    await tokenStorage.setDeviceId(deviceId);
  }

  await registerFn(pushToken.data, platform, deviceId);

  return pushToken.data;
}
```

- [ ] **Step 4: Commit**

```bash
git add packages/mobile-shared/stores/ packages/mobile-shared/push/
git commit -m "feat(mobile-shared): add Zustand stores and push notification registration"
```

---

### Task 9: Domain API modules

**Files:**
- Create: `packages/mobile-shared/api/dashboard.ts`
- Create: `packages/mobile-shared/api/orders.ts`
- Create: `packages/mobile-shared/api/products.ts`
- Create: `packages/mobile-shared/api/customers.ts`
- Create: `packages/mobile-shared/api/notifications.ts`

- [ ] **Step 1: Create domain API modules**

Create `packages/mobile-shared/api/dashboard.ts`:

```typescript
import type { createApiClient } from "./client";
import type { DashboardResponse } from "./types";

export function createDashboardApi(client: ReturnType<typeof createApiClient>) {
  return {
    get: () => client.get<DashboardResponse>("/dashboard"),
  };
}
```

Create `packages/mobile-shared/api/orders.ts`:

```typescript
import type { createApiClient } from "./client";
import type { Order, OrderDetail, PaginatedResponse } from "./types";

export interface ListOrdersParams {
  status?: string;
  search?: string;
  page?: string;
  limit?: string;
}

export function createOrdersApi(client: ReturnType<typeof createApiClient>) {
  return {
    list: (params?: ListOrdersParams) =>
      client.get<PaginatedResponse<Order>>("/orders", params as Record<string, string>),

    get: (id: string) => client.get<OrderDetail>(`/orders/${id}`),

    confirm: (id: string) => client.post(`/orders/${id}/confirm`),

    fulfill: (id: string, trackingNumber: string) =>
      client.post(`/orders/${id}/fulfill`, { tracking_number: trackingNumber }),

    cancel: (id: string, reason?: string) =>
      client.post(`/orders/${id}/cancel`, { reason }),

    refund: (id: string, amount: number) =>
      client.post(`/orders/${id}/refund`, { amount }),
  };
}
```

Create `packages/mobile-shared/api/products.ts`:

```typescript
import type { createApiClient } from "./client";
import type { Product, ProductDetail, PaginatedResponse } from "./types";

export interface ListProductsParams {
  status?: string;
  low_stock?: string;
  search?: string;
  page?: string;
  limit?: string;
}

export interface CreateProductBody {
  name: string;
  description?: string;
  price: number;
  compare_at_price?: number;
  sku?: string;
  stock: number;
  category_id?: string;
  tags?: string[];
  status: string;
}

export function createProductsApi(client: ReturnType<typeof createApiClient>) {
  return {
    list: (params?: ListProductsParams) =>
      client.get<PaginatedResponse<Product>>("/products", params as Record<string, string>),

    get: (id: string) => client.get<ProductDetail>(`/products/${id}`),

    create: (body: CreateProductBody) => client.post<ProductDetail>("/products", body),

    update: (id: string, body: Partial<CreateProductBody>) =>
      client.patch<ProductDetail>(`/products/${id}`, body),

    uploadMedia: async (productId: string, uri: string) => {
      const formData = new FormData();
      const filename = uri.split("/").pop() ?? "photo.jpg";
      formData.append("file", { uri, name: filename, type: "image/jpeg" } as unknown as Blob);
      return client.uploadMedia(`/products/${productId}/media`, formData);
    },

    deleteMedia: (productId: string, mediaId: string) =>
      client.delete(`/products/${productId}/media/${mediaId}`),

    reorderMedia: (productId: string, mediaIds: string[]) =>
      client.patch(`/products/${productId}/media/reorder`, { media_ids: mediaIds }),

    listVariants: (productId: string) => client.get(`/products/${productId}/variants`),

    createVariant: (productId: string, body: { name: string; sku?: string; price: number; stock: number }) =>
      client.post(`/products/${productId}/variants`, body),

    updateVariant: (productId: string, variantId: string, body: { price?: number; stock?: number }) =>
      client.patch(`/products/${productId}/variants/${variantId}`, body),
  };
}
```

Create `packages/mobile-shared/api/customers.ts`:

```typescript
import type { createApiClient } from "./client";
import type { Customer, CustomerDetail, PaginatedResponse } from "./types";

export interface ListCustomersParams {
  search?: string;
  page?: string;
  limit?: string;
}

export function createCustomersApi(client: ReturnType<typeof createApiClient>) {
  return {
    list: (params?: ListCustomersParams) =>
      client.get<PaginatedResponse<Customer>>("/customers", params as Record<string, string>),

    get: (id: string) => client.get<CustomerDetail>(`/customers/${id}`),

    block: (id: string) => client.post(`/customers/${id}/block`),

    unblock: (id: string) => client.post(`/customers/${id}/unblock`),
  };
}
```

Create `packages/mobile-shared/api/notifications.ts`:

```typescript
import type { createApiClient } from "./client";
import type { Notification, PaginatedResponse } from "./types";

export function createNotificationsApi(client: ReturnType<typeof createApiClient>) {
  return {
    list: (params?: { page?: string; limit?: string }) =>
      client.get<PaginatedResponse<Notification>>("/notifications", params as Record<string, string>),

    markAllRead: () => client.post("/notifications/mark-all-read"),

    registerPushToken: (token: string, platform: string, deviceId: string) =>
      client.post("/push-tokens", { token, platform, device_id: deviceId }),

    deletePushToken: (tokenId: string) => client.delete(`/push-tokens/${tokenId}`),
  };
}
```

- [ ] **Step 2: Commit**

```bash
git add packages/mobile-shared/api/
git commit -m "feat(mobile-shared): add domain API modules — dashboard, orders, products, customers, notifications"
```

---

## Phase 3: Expo App Scaffold + Navigation

### Task 10: Expo app scaffold

**Files:**
- Create: `apps/mobile-admin/package.json`
- Create: `apps/mobile-admin/tsconfig.json`
- Create: `apps/mobile-admin/app.json`
- Create: `apps/mobile-admin/babel.config.js`

- [ ] **Step 1: Create package.json**

Create `apps/mobile-admin/package.json`:

```json
{
  "name": "@repo/mobile-admin",
  "version": "0.0.0",
  "private": true,
  "main": "expo-router/entry",
  "scripts": {
    "dev": "expo start",
    "build": "echo 'use eas build'",
    "lint": "eslint .",
    "check-types": "tsc --noEmit",
    "test": "jest"
  },
  "dependencies": {
    "expo": "~52.0.0",
    "expo-router": "~4.0.0",
    "expo-camera": "~16.0.0",
    "expo-image-picker": "~16.0.0",
    "expo-image-manipulator": "~13.0.0",
    "expo-secure-store": "~14.0.0",
    "expo-notifications": "~0.29.0",
    "expo-device": "~7.0.0",
    "expo-linking": "~7.0.0",
    "expo-splash-screen": "~0.29.0",
    "expo-status-bar": "~2.0.0",
    "@react-native-firebase/app": "^21.0.0",
    "@react-native-firebase/auth": "^21.0.0",
    "@tanstack/react-query": "^5.83.0",
    "zustand": "^5.0.0",
    "zod": "^3.23.0",
    "react": "^19.0.0",
    "react-native": "~0.76.0",
    "react-native-safe-area-context": "~5.0.0",
    "react-native-screens": "~4.4.0",
    "react-native-gesture-handler": "~2.20.0",
    "react-native-reanimated": "~3.16.0",
    "react-native-svg": "~15.8.0",
    "@tesserix/native": "*",
    "@tesserix/tokens": "*",
    "@tesserix/hooks": "*",
    "@tesserix/icons": "*",
    "@repo/mobile-shared": "*"
  },
  "devDependencies": {
    "typescript": "^5.9.0",
    "@types/react": "^19.0.0",
    "jest": "^29.0.0",
    "jest-expo": "~52.0.0",
    "@testing-library/react-native": "^12.0.0"
  }
}
```

- [ ] **Step 2: Create app.json**

Create `apps/mobile-admin/app.json`:

```json
{
  "expo": {
    "name": "Mark8ly Admin",
    "slug": "mark8ly-admin",
    "version": "1.0.0",
    "orientation": "portrait",
    "icon": "./assets/icon.png",
    "scheme": "mark8ly-admin",
    "userInterfaceStyle": "light",
    "splash": {
      "image": "./assets/splash.png",
      "resizeMode": "contain",
      "backgroundColor": "#F7F6F2"
    },
    "ios": {
      "supportsTablet": false,
      "bundleIdentifier": "com.mark8ly.admin",
      "infoPlist": {
        "NSCameraUsageDescription": "Take product photos for your store",
        "NSPhotoLibraryUsageDescription": "Select product images from your library"
      },
      "associatedDomains": ["applinks:admin.mark8ly.com"]
    },
    "android": {
      "adaptiveIcon": {
        "foregroundImage": "./assets/adaptive-icon.png",
        "backgroundColor": "#F7F6F2"
      },
      "package": "com.mark8ly.admin",
      "intentFilters": [
        {
          "action": "VIEW",
          "autoVerify": true,
          "data": [{ "scheme": "https", "host": "admin.mark8ly.com", "pathPrefix": "/" }],
          "category": ["BROWSABLE", "DEFAULT"]
        }
      ]
    },
    "plugins": [
      "expo-router",
      "expo-camera",
      "expo-image-picker",
      "expo-secure-store",
      "expo-notifications",
      "@react-native-firebase/app"
    ],
    "extra": {
      "eas": { "projectId": "your-eas-project-id" },
      "apiBaseUrl": "https://api.mark8ly.com"
    }
  }
}
```

- [ ] **Step 3: Create tsconfig.json and babel.config.js**

Create `apps/mobile-admin/tsconfig.json`:

```json
{
  "extends": "expo/tsconfig.base",
  "compilerOptions": {
    "strict": true,
    "paths": {
      "@/*": ["./*"]
    }
  },
  "include": ["**/*.ts", "**/*.tsx", ".expo/types/**/*.ts", "expo-env.d.ts"]
}
```

Create `apps/mobile-admin/babel.config.js`:

```javascript
module.exports = function (api) {
  api.cache(true);
  return {
    presets: ["babel-preset-expo"],
    plugins: ["react-native-reanimated/plugin"],
  };
};
```

- [ ] **Step 4: Create placeholder assets**

```bash
mkdir -p apps/mobile-admin/assets
```

Generate valid placeholder PNGs (not empty files — Expo build fails on zero-byte PNGs). Use a 1024x1024 solid `#F7F6F2` PNG for `icon.png` and `adaptive-icon.png`, and a 1284x2778 solid `#F7F6F2` PNG for `splash.png`. You can generate these with ImageMagick:

```bash
convert -size 1024x1024 xc:'#F7F6F2' apps/mobile-admin/assets/icon.png
convert -size 1024x1024 xc:'#F7F6F2' apps/mobile-admin/assets/adaptive-icon.png
convert -size 1284x2778 xc:'#F7F6F2' apps/mobile-admin/assets/splash.png
```

If ImageMagick is not available, copy a valid PNG from the existing admin or storefront app's public directory and resize.

- [ ] **Step 5: Create .gitignore**

Create `apps/mobile-admin/.gitignore`:

```
node_modules/
.expo/
dist/
ios/
android/
*.jks
*.p8
*.p12
*.key
*.mobileprovision
*.orig.*
web-build/
```

- [ ] **Step 6: Commit**

```bash
git add apps/mobile-admin/package.json apps/mobile-admin/app.json apps/mobile-admin/tsconfig.json apps/mobile-admin/babel.config.js apps/mobile-admin/assets/ apps/mobile-admin/.gitignore
git commit -m "chore: scaffold Expo app at apps/mobile-admin"
```

---

### Task 11: Root layout + auth gate

**Files:**
- Create: `apps/mobile-admin/app/_layout.tsx`
- Create: `apps/mobile-admin/app/login.tsx`

- [ ] **Step 1: Create root layout with providers and auth gate**

Create `apps/mobile-admin/app/_layout.tsx`:

```tsx
import { useEffect } from "react";
import { Slot, useRouter, useSegments } from "expo-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AuthProvider, useAuth } from "@repo/mobile-shared/auth/provider";
import * as SplashScreen from "expo-splash-screen";

SplashScreen.preventAutoHideAsync();

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { staleTime: 30_000, retry: 2 },
  },
});

function AuthGate() {
  const { user, loading } = useAuth();
  const segments = useSegments();
  const router = useRouter();

  useEffect(() => {
    if (loading) return;

    const inAuthGroup = segments[0] === "login";

    if (!user && !inAuthGroup) {
      router.replace("/login");
    } else if (user && inAuthGroup) {
      router.replace("/");
    }

    SplashScreen.hideAsync();
  }, [user, loading, segments]);

  return <Slot />;
}

export default function RootLayout() {
  return (
    <QueryClientProvider client={queryClient}>
      <AuthProvider tenantId="mp-internal">
        <AuthGate />
      </AuthProvider>
    </QueryClientProvider>
  );
}
```

- [ ] **Step 2: Create login screen**

Create `apps/mobile-admin/app/login.tsx`:

```tsx
import { useState } from "react";
import { View, StyleSheet, Image, KeyboardAvoidingView, Platform } from "react-native";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { Text, Input, Pressable, Toast, Spinner } from "@tesserix/native";
import { tokens } from "@tesserix/tokens";
import * as Linking from "expo-linking";

export default function LoginScreen() {
  const { signIn } = useAuth();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleLogin = async () => {
    if (!email.trim() || !password) {
      setError("Email and password are required");
      return;
    }
    setLoading(true);
    setError(null);
    try {
      await signIn(email.trim(), password);
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : "Login failed";
      if (message.includes("wrong-password") || message.includes("user-not-found")) {
        setError("Invalid email or password");
      } else if (message.includes("network")) {
        setError("Network error — check your connection");
      } else {
        setError("Login failed — please try again");
      }
    } finally {
      setLoading(false);
    }
  };

  return (
    <KeyboardAvoidingView
      style={styles.container}
      behavior={Platform.OS === "ios" ? "padding" : "height"}
    >
      <View style={styles.form}>
        <Text style={styles.title}>Mark8ly</Text>
        <Text style={styles.subtitle}>Admin</Text>

        {error && <Text style={styles.error}>{error}</Text>}

        <Input
          placeholder="Email"
          value={email}
          onChangeText={setEmail}
          autoCapitalize="none"
          keyboardType="email-address"
          autoComplete="email"
          style={styles.input}
        />

        <Input
          placeholder="Password"
          value={password}
          onChangeText={setPassword}
          secureTextEntry
          autoComplete="password"
          style={styles.input}
        />

        <Pressable
          onPress={handleLogin}
          disabled={loading}
          style={styles.button}
        >
          {loading ? (
            <Spinner size="small" color={tokens.colors.paper[200]} />
          ) : (
            <Text style={styles.buttonText}>Sign in</Text>
          )}
        </Pressable>

        <Pressable
          onPress={() => Linking.openURL("https://admin.mark8ly.com/forgot-password")}
          style={styles.forgotLink}
        >
          <Text style={styles.forgotText}>Forgot password?</Text>
        </Pressable>
      </View>
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#F7F6F2",
    justifyContent: "center",
    paddingHorizontal: 24,
  },
  form: { gap: 16 },
  title: {
    fontSize: 32,
    fontWeight: "700",
    color: "#0E0E0C",
    fontFamily: "SourceSerif4-Bold",
  },
  subtitle: {
    fontSize: 18,
    color: "#0E0E0C",
    opacity: 0.6,
    marginBottom: 16,
  },
  error: { color: "#8B2020", fontSize: 14 },
  input: { backgroundColor: "#FFFFFF", borderRadius: 6 },
  button: {
    backgroundColor: "#0E0E0C",
    height: 48,
    borderRadius: 6,
    alignItems: "center",
    justifyContent: "center",
    marginTop: 8,
  },
  buttonText: { color: "#F7F6F2", fontSize: 16, fontWeight: "600" },
  forgotLink: { alignItems: "center", marginTop: 8 },
  forgotText: { color: "#2D4A2B", fontSize: 14 },
});
```

- [ ] **Step 3: Commit**

```bash
git add apps/mobile-admin/app/_layout.tsx apps/mobile-admin/app/login.tsx
git commit -m "feat(mobile-admin): add root layout with auth gate and login screen"
```

---

### Task 12: Tab navigation

**Files:**
- Create: `apps/mobile-admin/app/(tabs)/_layout.tsx`
- Create: `apps/mobile-admin/app/(tabs)/index.tsx` (dashboard placeholder)
- Create: `apps/mobile-admin/app/(tabs)/orders/_layout.tsx`
- Create: `apps/mobile-admin/app/(tabs)/products/_layout.tsx`
- Create: `apps/mobile-admin/app/(tabs)/customers/_layout.tsx`
- Create: `apps/mobile-admin/app/(tabs)/customers/index.tsx` (placeholder)
- Create: `apps/mobile-admin/app/(tabs)/more/_layout.tsx`
- Create: `apps/mobile-admin/app/(tabs)/more/index.tsx` (placeholder)

- [ ] **Step 1: Create tab layout**

Create `apps/mobile-admin/app/(tabs)/_layout.tsx`:

```tsx
import { Tabs } from "expo-router";
import { LayoutDashboard, ShoppingBag, Package, Users, MoreHorizontal } from "@tesserix/icons/native";

export default function TabLayout() {
  return (
    <Tabs
      screenOptions={{
        tabBarActiveTintColor: "#0E0E0C",
        tabBarInactiveTintColor: "#0E0E0C80",
        tabBarStyle: {
          backgroundColor: "#FFFFFF",
          borderTopColor: "#0E0E0C10",
          borderTopWidth: 0.5,
        },
        headerStyle: { backgroundColor: "#F7F6F2" },
        headerTintColor: "#0E0E0C",
        headerShadowVisible: false,
      }}
    >
      <Tabs.Screen
        name="index"
        options={{
          title: "Dashboard",
          tabBarIcon: ({ color, size }) => <LayoutDashboard color={color} size={size} />,
        }}
      />
      <Tabs.Screen
        name="orders"
        options={{
          title: "Orders",
          headerShown: false,
          tabBarIcon: ({ color, size }) => <ShoppingBag color={color} size={size} />,
        }}
      />
      <Tabs.Screen
        name="products"
        options={{
          title: "Products",
          headerShown: false,
          tabBarIcon: ({ color, size }) => <Package color={color} size={size} />,
        }}
      />
      <Tabs.Screen
        name="customers"
        options={{
          title: "Customers",
          tabBarIcon: ({ color, size }) => <Users color={color} size={size} />,
        }}
      />
      <Tabs.Screen
        name="more"
        options={{
          title: "More",
          headerShown: false,
          tabBarIcon: ({ color, size }) => <MoreHorizontal color={color} size={size} />,
        }}
      />
    </Tabs>
  );
}
```

- [ ] **Step 2: Create placeholder screens**

Create `apps/mobile-admin/app/(tabs)/index.tsx` (dashboard):

```tsx
import { View, Text, StyleSheet } from "react-native";

export default function DashboardScreen() {
  return (
    <View style={styles.container}>
      <Text style={styles.text}>Dashboard — coming in Task 13</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: "#F7F6F2", justifyContent: "center", alignItems: "center" },
  text: { color: "#0E0E0C", fontSize: 16 },
});
```

Create `apps/mobile-admin/app/(tabs)/orders/_layout.tsx`:

```tsx
import { Stack } from "expo-router";

export default function OrdersLayout() {
  return (
    <Stack screenOptions={{ headerStyle: { backgroundColor: "#F7F6F2" }, headerShadowVisible: false }}>
      <Stack.Screen name="index" options={{ title: "Orders" }} />
      <Stack.Screen name="[id]" options={{ title: "Order Detail" }} />
    </Stack>
  );
}
```

Create `apps/mobile-admin/app/(tabs)/products/_layout.tsx`:

```tsx
import { Stack } from "expo-router";

export default function ProductsLayout() {
  return (
    <Stack screenOptions={{ headerStyle: { backgroundColor: "#F7F6F2" }, headerShadowVisible: false }}>
      <Stack.Screen name="index" options={{ title: "Products" }} />
      <Stack.Screen name="[id]" options={{ title: "Product" }} />
      <Stack.Screen name="new" options={{ title: "New Product" }} />
    </Stack>
  );
}
```

Create `apps/mobile-admin/app/(tabs)/customers/_layout.tsx`:

```tsx
import { Stack } from "expo-router";

export default function CustomersLayout() {
  return (
    <Stack screenOptions={{ headerStyle: { backgroundColor: "#F7F6F2" }, headerShadowVisible: false }}>
      <Stack.Screen name="index" options={{ title: "Customers" }} />
      <Stack.Screen name="[id]" options={{ title: "Customer" }} />
    </Stack>
  );
}
```

Create `apps/mobile-admin/app/(tabs)/more/_layout.tsx`:

```tsx
import { Stack } from "expo-router";

export default function MoreLayout() {
  return (
    <Stack screenOptions={{ headerStyle: { backgroundColor: "#F7F6F2" }, headerShadowVisible: false }}>
      <Stack.Screen name="index" options={{ title: "More" }} />
      <Stack.Screen name="notifications" options={{ title: "Notifications" }} />
      <Stack.Screen name="account" options={{ title: "Account" }} />
    </Stack>
  );
}
```

Create placeholder `apps/mobile-admin/app/(tabs)/customers/index.tsx` and `apps/mobile-admin/app/(tabs)/more/index.tsx` (same pattern as dashboard placeholder).

- [ ] **Step 3: Commit**

```bash
git add apps/mobile-admin/app/
git commit -m "feat(mobile-admin): add tab navigation with 5 tabs and placeholder screens"
```

---

## Phase 4: Screens — Dashboard, Orders, Products, Customers, More

> **Note to implementer:** Tasks 13-18 build out the actual screens. Each screen follows the same pattern:
> 1. Create the screen component using `@tesserix/native` components
> 2. Wire up TanStack React Query hooks calling `@repo/mobile-shared` API modules
> 3. Add pull-to-refresh, infinite scroll, search, and error/empty states
>
> These tasks are **independent** — they can be implemented in parallel by separate agents.

### Task 13: Dashboard screen

**Files:**
- Create: `apps/mobile-admin/components/DashboardStats.tsx`
- Create: `apps/mobile-admin/lib/hooks/use-dashboard.ts`
- Modify: `apps/mobile-admin/app/(tabs)/index.tsx`

- [ ] **Step 1: Create use-dashboard hook**

Create `apps/mobile-admin/lib/hooks/use-dashboard.ts` using `@tanstack/react-query` calling `createDashboardApi(client).get()`. Query key: `["dashboard"]`. Refetch on focus.

- [ ] **Step 2: Create DashboardStats component**

Create `apps/mobile-admin/components/DashboardStats.tsx` rendering:
- 3 MetricCards for revenue (today/week/month) with `revenue_change_pct` delta
- LineChart for `revenue_trend`
- 4 compact MetricCards for order counts (today/pending/fulfilled/cancelled)
- Customers total + new this week

- [ ] **Step 3: Replace dashboard placeholder**

Replace `apps/mobile-admin/app/(tabs)/index.tsx` with full dashboard:
- DashboardStats at top
- Recent orders FlatList (tap → router.push to order detail)
- Low stock alerts section
- Setup checklist (collapsible)
- PullToRefresh wrapping the whole ScrollView

- [ ] **Step 4: Commit**

```bash
git add apps/mobile-admin/components/DashboardStats.tsx apps/mobile-admin/lib/hooks/use-dashboard.ts apps/mobile-admin/app/\(tabs\)/index.tsx
git commit -m "feat(mobile-admin): implement dashboard screen with stats, charts, recent orders"
```

---

### Task 14: Orders list + detail screens

**Files:**
- Create: `apps/mobile-admin/components/OrderRow.tsx`
- Create: `apps/mobile-admin/lib/hooks/use-orders.ts`
- Create: `apps/mobile-admin/lib/admin-api/order-actions.ts`
- Create: `apps/mobile-admin/app/(tabs)/orders/index.tsx`
- Create: `apps/mobile-admin/app/(tabs)/orders/[id].tsx`

- [ ] **Step 1: Create order hooks + admin actions**

Create `apps/mobile-admin/lib/hooks/use-orders.ts` with `useOrders(status?)` and `useOrder(id)` query hooks. Create `apps/mobile-admin/lib/admin-api/order-actions.ts` with mutation hooks: `useConfirmOrder`, `useFulfillOrder`, `useCancelOrder`, `useRefundOrder` — each uses `useMutation` with optimistic updates that invalidate `["orders"]` on success.

- [ ] **Step 2: Create OrderRow component**

Create `apps/mobile-admin/components/OrderRow.tsx` using `@tesserix/native` ListItem, Badge. Shows: order number, customer name, total, status badge, timestamp. Swipeable with quick confirm action for pending orders.

- [ ] **Step 3: Build orders list screen**

Create `apps/mobile-admin/app/(tabs)/orders/index.tsx`:
- SegmentedControl at top: All / Active / Completed / Cancelled
- SearchBar
- FlatList of OrderRow with InfiniteScroll pagination
- PullToRefresh
- EmptyState when no results

- [ ] **Step 4: Build order detail screen**

Create `apps/mobile-admin/app/(tabs)/orders/[id].tsx`:
- Order header with status badge
- Line items list with thumbnails
- Customer section (tap → customer detail)
- Shipping + payment info
- Actions BottomSheet (Confirm / Fulfill / Cancel / Refund based on status)
- Timeline at bottom

- [ ] **Step 5: Commit**

```bash
git add apps/mobile-admin/components/OrderRow.tsx apps/mobile-admin/lib/ apps/mobile-admin/app/\(tabs\)/orders/
git commit -m "feat(mobile-admin): implement orders list and detail screens with actions"
```

---

### Task 15: Products list + detail + creation screens

**Files:**
- Create: `apps/mobile-admin/components/ProductRow.tsx`
- Create: `apps/mobile-admin/components/ProductMediaPicker.tsx`
- Create: `apps/mobile-admin/lib/hooks/use-products.ts`
- Create: `apps/mobile-admin/lib/admin-api/product-crud.ts`
- Create: `apps/mobile-admin/app/(tabs)/products/index.tsx`
- Create: `apps/mobile-admin/app/(tabs)/products/[id].tsx`
- Create: `apps/mobile-admin/app/(tabs)/products/new.tsx`

- [ ] **Step 1: Create product hooks + CRUD actions**

Create `apps/mobile-admin/lib/hooks/use-products.ts` with `useProducts(params)` and `useProduct(id)`. Create `apps/mobile-admin/lib/admin-api/product-crud.ts` with mutation hooks: `useCreateProduct`, `useUpdateProduct`, `useUploadMedia`, `useDeleteMedia`, `useReorderMedia`, `useCreateVariant`, `useUpdateVariant`.

- [ ] **Step 2: Create ProductRow component**

Create `apps/mobile-admin/components/ProductRow.tsx`: thumbnail, name, price, stock count, status indicator. Swipeable for archive/activate toggle.

- [ ] **Step 3: Create ProductMediaPicker component**

Create `apps/mobile-admin/components/ProductMediaPicker.tsx`:
- Camera capture via `expo-camera` (or `expo-image-picker` launchCameraAsync)
- Gallery selection via `expo-image-picker` launchImageLibraryAsync (multi-select)
- Crop/rotate via `expo-image-manipulator`
- SortableList for reordering
- Delete button per image
- Upload progress indicator

- [ ] **Step 4: Build products list screen**

Create `apps/mobile-admin/app/(tabs)/products/index.tsx`:
- SegmentedControl: All / Low Stock / Inactive
- SearchBar
- FlatList of ProductRow with InfiniteScroll
- FAB for "Add Product"
- PullToRefresh + EmptyState

- [ ] **Step 5: Build product detail/edit screen**

Create `apps/mobile-admin/app/(tabs)/products/[id].tsx`:
- Media gallery at top (ImageGallery)
- Editable fields: name, description, price, compare_at_price, SKU, stock, status Switch
- Category picker (BottomSheet + TreeView via categories API)
- Tags input
- Variants list with inline price/stock editing
- "Add variant" button
- ProductMediaPicker for adding new photos
- Save button in header

- [ ] **Step 6: Build product creation wizard**

Create `apps/mobile-admin/app/(tabs)/products/new.tsx`:
- FormWizard with ProgressSteps:
  1. Photos (ProductMediaPicker — camera leads)
  2. Details (name, description, price, compare_at_price, SKU, stock)
  3. Organization (category picker, tags, status draft/active)
  4. Variants (optional — skip or add simple variants)
- "Save as draft" available at any step

- [ ] **Step 7: Commit**

```bash
git add apps/mobile-admin/components/Product* apps/mobile-admin/lib/ apps/mobile-admin/app/\(tabs\)/products/
git commit -m "feat(mobile-admin): implement products list, detail, and creation wizard with camera"
```

---

### Task 16: Customers list + detail screens

**Files:**
- Create: `apps/mobile-admin/components/CustomerRow.tsx`
- Create: `apps/mobile-admin/lib/hooks/use-customers.ts`
- Create: `apps/mobile-admin/lib/admin-api/customer-actions.ts`
- Modify: `apps/mobile-admin/app/(tabs)/customers/index.tsx`
- Create: `apps/mobile-admin/app/(tabs)/customers/[id].tsx`

- [ ] **Step 1: Create customer hooks + actions**

Create `apps/mobile-admin/lib/hooks/use-customers.ts` with `useCustomers(params)` and `useCustomer(id)`. Create `apps/mobile-admin/lib/admin-api/customer-actions.ts` with `useBlockCustomer`, `useUnblockCustomer` mutation hooks.

- [ ] **Step 2: Create CustomerRow and screens**

Create `apps/mobile-admin/components/CustomerRow.tsx`: Avatar, name, email, order count, total spent.

Replace customers `index.tsx` placeholder: SearchBar + FlatList + PullToRefresh + InfiniteScroll + EmptyState.

Create `apps/mobile-admin/app/(tabs)/customers/[id].tsx`: profile header, stats row, order history list, Block/Unblock action with Alert confirmation.

- [ ] **Step 3: Commit**

```bash
git add apps/mobile-admin/components/CustomerRow.tsx apps/mobile-admin/lib/ apps/mobile-admin/app/\(tabs\)/customers/
git commit -m "feat(mobile-admin): implement customers list and detail screens"
```

---

### Task 17: More screen — notifications + account + store switcher

**Files:**
- Create: `apps/mobile-admin/components/StoreSelector.tsx`
- Create: `apps/mobile-admin/lib/hooks/use-notifications.ts`
- Create: `apps/mobile-admin/lib/hooks/use-store.ts`
- Create: `apps/mobile-admin/lib/hooks/use-push.ts`
- Modify: `apps/mobile-admin/app/(tabs)/more/index.tsx`
- Create: `apps/mobile-admin/app/(tabs)/more/notifications.tsx`
- Create: `apps/mobile-admin/app/(tabs)/more/account.tsx`

- [ ] **Step 1: Create hooks**

Create `use-notifications.ts` (query + markAllRead mutation), `use-store.ts` (loads stores list, manages active store via tenant-store), `use-push.ts` (registers push token on mount, handles notification tap → router.push deep link).

- [ ] **Step 2: Create StoreSelector component**

Create `apps/mobile-admin/components/StoreSelector.tsx`: BottomSheet with list of stores. Active store highlighted. Tap switches store, invalidates all queries.

- [ ] **Step 3: Build more/notifications/account screens**

Replace `more/index.tsx`: menu list with Notifications (unread badge), Account, "Open in browser" link, app version.

Create `more/notifications.tsx`: chronological notification feed, tap → deep link, "Mark all as read" button.

Create `more/account.tsx`: profile info (read-only), StoreSelector, logout button.

- [ ] **Step 4: Commit**

```bash
git add apps/mobile-admin/components/StoreSelector.tsx apps/mobile-admin/lib/hooks/ apps/mobile-admin/app/\(tabs\)/more/
git commit -m "feat(mobile-admin): implement notifications, account, and store switcher"
```

---

## Phase 5: Push Notifications + Polish

### Task 18: Push notification wiring

**Files:**
- Modify: `apps/mobile-admin/app/(tabs)/_layout.tsx`
- Review: Push hook from Task 17

- [ ] **Step 1: Wire push registration into tabs layout (NOT root layout)**

In `apps/mobile-admin/app/(tabs)/_layout.tsx`, call the `use-push` hook. This component is only mounted when the user is authenticated (the root layout's AuthGate redirects unauthenticated users to `/login`), so hooks here can safely assume a logged-in user. Do NOT put push hooks in `_layout.tsx` root — hooks cannot be called conditionally and the root layout renders for both authenticated and unauthenticated states.

Set up the notification tap handler that navigates via `router.push(notification.deep_link)`.

Configure `Notifications.setNotificationHandler` for foreground notification display at the top of the file (outside the component).

- [ ] **Step 2: Test push flow manually**

Using Expo's push notification tool (https://expo.dev/notifications), send a test push to the device token. Verify:
- Foreground: notification appears as banner
- Background: tap navigates to correct screen
- Token registration persists across app restarts

- [ ] **Step 3: Commit**

```bash
git add apps/mobile-admin/app/_layout.tsx
git commit -m "feat(mobile-admin): wire push notifications — registration, foreground display, deep link nav"
```

---

### Task 19: Install dependencies + verify build

- [ ] **Step 1: Install npm dependencies**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly
npm install
```

- [ ] **Step 2: Type-check the mobile packages**

```bash
npx turbo check-types --filter=@repo/mobile-shared --filter=@repo/mobile-admin
```

Fix any type errors.

- [ ] **Step 3: Verify Go backend compiles**

```bash
cd services/marketplace-api && go build ./cmd/marketplace-api/
```

- [ ] **Step 4: Run existing tests to ensure no regressions**

```bash
cd services/marketplace-api && go test ./internal/auth/ -v
```

- [ ] **Step 5: Commit any fixes**

```bash
git add -A
git commit -m "fix: resolve type and build errors from mobile admin integration"
```

---

### Task 20: Backend push sender (Pub/Sub webhook)

**Files:**
- Create: `services/marketplace-api/internal/push/sender.go`
- Create: `services/marketplace-api/internal/push/sender_test.go`
- Create: `services/marketplace-api/internal/push/webhook.go`
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go`

- [ ] **Step 1: Write failing test for push sender**

Create `services/marketplace-api/internal/push/sender_test.go` testing `Send(tokens []string, title, body, deepLink string)`. Use a mock HTTP client. Verify it batches requests to `https://exp.host/--/api/v2/push/send` correctly.

- [ ] **Step 2: Implement Expo Push sender**

Create `services/marketplace-api/internal/push/sender.go`:
- `type Sender struct { httpClient *http.Client }`
- `func (s *Sender) Send(tokens []string, title, body, deepLink string) error` — POST to Expo Push API
- Handle `DeviceNotRegistered` errors by returning stale tokens for cleanup
- Batch up to 100 tokens per request (Expo limit)

- [ ] **Step 3: Implement Pub/Sub webhook handler**

Create `services/marketplace-api/internal/push/webhook.go`:
- `func NewWebhookHandler(sender *Sender, tokenRepo *Repository, logger *slog.Logger) gin.HandlerFunc`
- Parses Pub/Sub push message (JSON envelope with base64-encoded data)
- **Verify the Pub/Sub OIDC token**: Google Pub/Sub push subscriptions include an `Authorization: Bearer <oidc-token>` header. Verify the token's `audience` matches the webhook URL and `email` matches the Pub/Sub service account. Use `google.golang.org/api/idtoken` package's `Validate()` function. Without this, anyone discovering the endpoint can inject fake push events.
- Routes by event type: `order.created` → "New order" push, `inventory.low_stock` → "Low stock" push
- Looks up push tokens for the relevant store
- Sends via Sender, cleans up stale tokens

- [ ] **Step 4: Wire webhook into main.go**

Add route: `router.POST("/internal/push-webhook", pushWebhookHandler)` (outside user auth middleware — Pub/Sub authenticates via OIDC token verified in the handler).

- [ ] **Step 5: Run tests**

```bash
cd services/marketplace-api && go test ./internal/push/ -v
```

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/push/ services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "feat(marketplace-api): add Expo push sender and Pub/Sub webhook handler"
```

---

## Summary

| Phase | Tasks | Description |
|-------|-------|-------------|
| **1: Backend** | 1-4 | GIP Bearer auth middleware, mobile routes, push token migration/handler |
| **2: Shared Package** | 5-9 | packages/mobile-shared — API client, auth, stores, push, domain APIs |
| **3: App Scaffold** | 10-12 | Expo project, root layout, auth gate, tab navigation |
| **4: Screens** | 13-17 | Dashboard, Orders, Products (with camera), Customers, More/Notifications/Account |
| **5: Push + Polish** | 18-20 | Push notification wiring, dependency install, backend push sender |

**Parallelization:** Tasks 13-17 (Phase 4 screens) are fully independent and can be dispatched to parallel agents. Phase 1 and 2 are sequential (backend before frontend). Phase 3 must complete before Phase 4.
