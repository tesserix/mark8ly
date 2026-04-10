# Branding B2 — Subscription Tiers Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Ship feature gating across all plan tiers (Free/Starter/Pro/Enterprise/Marketplace). Zero transaction fees. Regional pricing (US/India/SEA). Soft downgrade on trial expiry (read-only excess items, 30-day grace). GMV nudge on Starter dashboard. Frontend PlanGate component.

**Architecture:** New `internal/plangate/` package (feature definitions, plan lookup, Gin middleware). Extends existing store_subscriptions from S3. Admin + storefront frontend hooks for plan checking. No new migration — uses store_subscriptions.plan field.

**Tech Stack:** Go 1.26, Gin middleware. Next.js 16, React 19.

**Prerequisite:** S3 (Subscription/Billing) must be on main.

**Spec reference:** `docs/superpowers/specs/2026-04-10-branding-subscriptions-mobile-design.md` — sections §2, §3, §7.

---

## File structure produced by B2

```
services/marketplace-api/
├── internal/
│   ├── plangate/
│   │   ├── plans.go                        # NEW — Plan type, plan ordering, plan hierarchy
│   │   ├── features.go                     # NEW — Feature type, all feature constants
│   │   ├── limits.go                       # NEW — limits table, GetLimit, IsAllowed, limitFor
│   │   ├── limits_test.go                  # NEW — unit tests for limits table
│   │   ├── middleware.go                   # NEW — RequirePlan, EnforceLimit Gin middleware
│   │   ├── middleware_test.go              # NEW — unit tests for middleware
│   │   ├── downgrade.go                    # NEW — soft downgrade logic, grace period check
│   │   └── downgrade_test.go              # NEW — unit tests for downgrade
│   ├── subscription/
│   │   ├── models.go                       # MODIFY — add trial_ends_at, grace_period_ends_at, downgraded_from fields
│   │   └── repository.go                  # MODIFY — add GetPlanForStore, UpdateDowngrade
│   └── handlers/admin/
│       └── routes.go                       # MODIFY — wire plangate middleware on create endpoints
├── cmd/marketplace-api/
│   └── main.go                             # MODIFY — wire plangate into admin + storefront deps

apps/admin/
├── lib/
│   ├── api/
│   │   └── subscription-api.ts             # MODIFY — add plan limits + features to response type
│   └── hooks/
│       ├── use-subscription.ts             # NEW — useSubscription() hook
│       └── use-subscription.test.ts        # NEW — hook unit tests
├── components/
│   ├── plangate/
│   │   ├── PlanGate.tsx                    # NEW — <PlanGate feature="..." fallback={...}>
│   │   ├── PlanGate.test.tsx               # NEW — unit tests
│   │   ├── UpgradePrompt.tsx               # NEW — upgrade CTA with feature context
│   │   ├── UpgradePrompt.test.tsx          # NEW — unit tests
│   │   ├── TrialExpiryBanner.tsx           # NEW — 7-day, 1-day, expired banners
│   │   └── GmvNudge.tsx                    # NEW — GMV upgrade nudge for Starter
│   └── shell/
│       └── AdminShell.tsx                  # MODIFY — mount TrialExpiryBanner in header
├── app/
│   ├── dashboard/
│   │   └── page.tsx                        # MODIFY — add GmvNudge component
│   └── settings/subscription/
│       └── page.tsx                        # MODIFY — add feature comparison grid
```

---

## Task 0: Verify prerequisites

**Files:** none (read-only)

- [ ] **Step 1: Verify S3 subscription package exists**

```bash
ls services/marketplace-api/internal/subscription/
```

Expected: `models.go`, `repository.go`, `handler.go`, `stripe_billing.go` (shipped by S3). If missing, S3 must be completed first.

- [ ] **Step 2: Verify store_subscriptions table**

```bash
docker exec dev-postgres-1 psql -U dev -d marketplace_db -tAc \
  "SELECT column_name FROM information_schema.columns WHERE table_name='store_subscriptions' ORDER BY ordinal_position;"
```

Expected: includes `plan`, `status`, `current_period_end` columns.

- [ ] **Step 3: Verify current admin route pattern compiles**

```bash
cd services/marketplace-api && go build ./...
```

No commit. Task 0 is read-only.

---

## Task 1: plangate package — Plan type, Feature type, limits table

**Files:**
- Create: `services/marketplace-api/internal/plangate/plans.go`
- Create: `services/marketplace-api/internal/plangate/features.go`
- Create: `services/marketplace-api/internal/plangate/limits.go`
- Create: `services/marketplace-api/internal/plangate/limits_test.go`

### TDD: RED — Write tests first

- [ ] **Step 1: Create plans.go — Plan type + ordering**

Create `services/marketplace-api/internal/plangate/plans.go`:

```go
package plangate

// Plan represents a subscription tier.
type Plan string

const (
	PlanFree        Plan = "free"
	PlanStarter     Plan = "starter"
	PlanPro         Plan = "pro"
	PlanEnterprise  Plan = "enterprise"
	PlanMarketplace Plan = "marketplace"
)

// planOrder defines the hierarchy — higher index = more permissive.
var planOrder = map[Plan]int{
	PlanFree:        0,
	PlanStarter:     1,
	PlanPro:         2,
	PlanEnterprise:  3,
	PlanMarketplace: 4,
}

// Rank returns the numeric rank of a plan (0 = free, 4 = marketplace).
// Unknown plans return 0 (treated as free).
func (p Plan) Rank() int {
	r, ok := planOrder[p]
	if !ok {
		return 0
	}
	return r
}

// AtLeast returns true if p is at or above the given minimum plan.
func (p Plan) AtLeast(min Plan) bool {
	return p.Rank() >= min.Rank()
}

// ValidPlans is the exhaustive list, useful for validation.
var ValidPlans = []Plan{PlanFree, PlanStarter, PlanPro, PlanEnterprise, PlanMarketplace}

// IsValidPlan returns true if s is a recognized plan string.
func IsValidPlan(s string) bool {
	for _, p := range ValidPlans {
		if Plan(s) == p {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Create features.go — Feature constants**

Create `services/marketplace-api/internal/plangate/features.go`:

```go
package plangate

// Feature identifies a gated capability or resource.
type Feature string

// Numeric-limit features (Max value in limits table).
const (
	FeatureProducts     Feature = "products"
	FeatureCategories   Feature = "categories"
	FeatureStaff        Feature = "staff"
	FeatureStores       Feature = "stores"
	FeatureOrdersMonth  Feature = "orders_per_month"
	FeatureCoupons      Feature = "active_coupons"
	FeatureCampaigns    Feature = "campaigns_per_month"
)

// Boolean features (allowed = Max > 0).
const (
	FeatureFullPalette     Feature = "full_color_palette"
	FeatureAnnouncementBar Feature = "announcement_bar"
	FeatureRemovePoweredBy Feature = "remove_powered_by"
	FeatureCustomCSS       Feature = "custom_css"
	FeatureCustomDomain    Feature = "custom_domain"
	FeatureReturns         Feature = "returns"
	FeatureLabelTracking   Feature = "label_tracking"
	FeatureGiftCards       Feature = "gift_cards"
	FeatureLoyalty         Feature = "loyalty"
	FeatureReviews         Feature = "reviews"
	FeatureSupportTickets  Feature = "support_tickets"
	FeaturePrioritySupport Feature = "priority_support"
	FeatureAuditLogs       Feature = "audit_logs"
	FeatureMobileApp       Feature = "mobile_app"
	FeatureCSVImportExport Feature = "csv_import_export"
	FeatureVendorOnboard   Feature = "vendor_onboarding"
)
```

- [ ] **Step 3: Create limits.go — the limits table + IsAllowed + GetLimit**

Create `services/marketplace-api/internal/plangate/limits.go`:

```go
package plangate

// Unlimited is the sentinel for "no cap".
const Unlimited = -1

// Disabled is the sentinel for "feature not available on this plan".
const Disabled = 0

// limitEntry holds the max value for a feature on a specific plan.
type limitEntry struct {
	Feature Feature
	Plan    Plan
	Max     int // -1 = unlimited, 0 = disabled/not available
}

// limitsTable is the single source of truth for all plan × feature limits.
// Matches spec §3.1 feature matrix exactly.
var limitsTable = []limitEntry{
	// Products
	{FeatureProducts, PlanFree, 25},
	{FeatureProducts, PlanStarter, 500},
	{FeatureProducts, PlanPro, Unlimited},
	{FeatureProducts, PlanEnterprise, Unlimited},
	{FeatureProducts, PlanMarketplace, Unlimited},

	// Categories
	{FeatureCategories, PlanFree, 5},
	{FeatureCategories, PlanStarter, 25},
	{FeatureCategories, PlanPro, Unlimited},
	{FeatureCategories, PlanEnterprise, Unlimited},
	{FeatureCategories, PlanMarketplace, Unlimited},

	// Staff
	{FeatureStaff, PlanFree, 1},
	{FeatureStaff, PlanStarter, 3},
	{FeatureStaff, PlanPro, 10},
	{FeatureStaff, PlanEnterprise, Unlimited},
	{FeatureStaff, PlanMarketplace, Unlimited},

	// Stores
	{FeatureStores, PlanFree, 1},
	{FeatureStores, PlanStarter, 1},
	{FeatureStores, PlanPro, 3},
	{FeatureStores, PlanEnterprise, 10},
	{FeatureStores, PlanMarketplace, 10},

	// Orders per month
	{FeatureOrdersMonth, PlanFree, 50},
	{FeatureOrdersMonth, PlanStarter, 500},
	{FeatureOrdersMonth, PlanPro, Unlimited},
	{FeatureOrdersMonth, PlanEnterprise, Unlimited},
	{FeatureOrdersMonth, PlanMarketplace, Unlimited},

	// Active coupons
	{FeatureCoupons, PlanFree, 5},
	{FeatureCoupons, PlanStarter, 50},
	{FeatureCoupons, PlanPro, Unlimited},
	{FeatureCoupons, PlanEnterprise, Unlimited},
	{FeatureCoupons, PlanMarketplace, Unlimited},

	// Campaigns per month
	{FeatureCampaigns, PlanFree, Disabled},
	{FeatureCampaigns, PlanStarter, 5},
	{FeatureCampaigns, PlanPro, 50},
	{FeatureCampaigns, PlanEnterprise, Unlimited},
	{FeatureCampaigns, PlanMarketplace, Unlimited},

	// Boolean features — 1 = allowed, 0 = disabled
	{FeatureFullPalette, PlanFree, Disabled},
	{FeatureFullPalette, PlanStarter, 1},
	{FeatureFullPalette, PlanPro, 1},
	{FeatureFullPalette, PlanEnterprise, 1},
	{FeatureFullPalette, PlanMarketplace, 1},

	{FeatureAnnouncementBar, PlanFree, Disabled},
	{FeatureAnnouncementBar, PlanStarter, 1},
	{FeatureAnnouncementBar, PlanPro, 1},
	{FeatureAnnouncementBar, PlanEnterprise, 1},
	{FeatureAnnouncementBar, PlanMarketplace, 1},

	{FeatureRemovePoweredBy, PlanFree, Disabled},
	{FeatureRemovePoweredBy, PlanStarter, Disabled},
	{FeatureRemovePoweredBy, PlanPro, 1},
	{FeatureRemovePoweredBy, PlanEnterprise, 1},
	{FeatureRemovePoweredBy, PlanMarketplace, 1},

	{FeatureCustomCSS, PlanFree, Disabled},
	{FeatureCustomCSS, PlanStarter, Disabled},
	{FeatureCustomCSS, PlanPro, Disabled},
	{FeatureCustomCSS, PlanEnterprise, 1},
	{FeatureCustomCSS, PlanMarketplace, 1},

	{FeatureCustomDomain, PlanFree, Disabled},
	{FeatureCustomDomain, PlanStarter, Disabled},
	{FeatureCustomDomain, PlanPro, 1},
	{FeatureCustomDomain, PlanEnterprise, 1},
	{FeatureCustomDomain, PlanMarketplace, 1},

	{FeatureReturns, PlanFree, Disabled},
	{FeatureReturns, PlanStarter, 1},
	{FeatureReturns, PlanPro, 1},
	{FeatureReturns, PlanEnterprise, 1},
	{FeatureReturns, PlanMarketplace, 1},

	{FeatureLabelTracking, PlanFree, Disabled},
	{FeatureLabelTracking, PlanStarter, 1},
	{FeatureLabelTracking, PlanPro, 1},
	{FeatureLabelTracking, PlanEnterprise, 1},
	{FeatureLabelTracking, PlanMarketplace, 1},

	{FeatureGiftCards, PlanFree, Disabled},
	{FeatureGiftCards, PlanStarter, 1},
	{FeatureGiftCards, PlanPro, 1},
	{FeatureGiftCards, PlanEnterprise, 1},
	{FeatureGiftCards, PlanMarketplace, 1},

	{FeatureLoyalty, PlanFree, Disabled},
	{FeatureLoyalty, PlanStarter, Disabled},
	{FeatureLoyalty, PlanPro, 1},
	{FeatureLoyalty, PlanEnterprise, 1},
	{FeatureLoyalty, PlanMarketplace, 1},

	{FeatureReviews, PlanFree, Disabled},
	{FeatureReviews, PlanStarter, 1},
	{FeatureReviews, PlanPro, 1},
	{FeatureReviews, PlanEnterprise, 1},
	{FeatureReviews, PlanMarketplace, 1},

	{FeatureSupportTickets, PlanFree, Disabled},
	{FeatureSupportTickets, PlanStarter, 1},
	{FeatureSupportTickets, PlanPro, 1},
	{FeatureSupportTickets, PlanEnterprise, 1},
	{FeatureSupportTickets, PlanMarketplace, 1},

	{FeaturePrioritySupport, PlanFree, Disabled},
	{FeaturePrioritySupport, PlanStarter, Disabled},
	{FeaturePrioritySupport, PlanPro, Disabled},
	{FeaturePrioritySupport, PlanEnterprise, 1},
	{FeaturePrioritySupport, PlanMarketplace, 1},

	{FeatureAuditLogs, PlanFree, Disabled},
	{FeatureAuditLogs, PlanStarter, Disabled},
	{FeatureAuditLogs, PlanPro, 1},
	{FeatureAuditLogs, PlanEnterprise, 1},
	{FeatureAuditLogs, PlanMarketplace, 1},

	{FeatureMobileApp, PlanFree, Disabled},
	{FeatureMobileApp, PlanStarter, Disabled},
	{FeatureMobileApp, PlanPro, Disabled},
	{FeatureMobileApp, PlanEnterprise, 1},
	{FeatureMobileApp, PlanMarketplace, 1},

	{FeatureCSVImportExport, PlanFree, Disabled},
	{FeatureCSVImportExport, PlanStarter, 1},
	{FeatureCSVImportExport, PlanPro, 1},
	{FeatureCSVImportExport, PlanEnterprise, 1},
	{FeatureCSVImportExport, PlanMarketplace, 1},

	{FeatureVendorOnboard, PlanFree, Disabled},
	{FeatureVendorOnboard, PlanStarter, Disabled},
	{FeatureVendorOnboard, PlanPro, Disabled},
	{FeatureVendorOnboard, PlanEnterprise, Disabled},
	{FeatureVendorOnboard, PlanMarketplace, 1},
}

// index builds a O(1) lookup map on first access.
var limitsIndex map[Feature]map[Plan]int

func init() {
	limitsIndex = make(map[Feature]map[Plan]int)
	for _, entry := range limitsTable {
		if limitsIndex[entry.Feature] == nil {
			limitsIndex[entry.Feature] = make(map[Plan]int)
		}
		limitsIndex[entry.Feature][entry.Plan] = entry.Max
	}
}

// GetLimit returns the numeric limit for a feature on a plan.
// Returns Disabled (0) if the feature or plan is not found.
func GetLimit(plan Plan, feature Feature) int {
	planMap, ok := limitsIndex[feature]
	if !ok {
		return Disabled
	}
	max, ok := planMap[plan]
	if !ok {
		return Disabled
	}
	return max
}

// IsAllowed returns true if the feature is available on the given plan.
// For boolean features, any value > 0 means allowed.
// For numeric features, any non-zero limit means allowed.
// Unlimited (-1) counts as allowed.
func IsAllowed(plan Plan, feature Feature) bool {
	limit := GetLimit(plan, feature)
	return limit != Disabled
}

// IsUnlimited returns true if the feature has no numeric cap on this plan.
func IsUnlimited(plan Plan, feature Feature) bool {
	return GetLimit(plan, feature) == Unlimited
}

// AllFeatureLimits returns a map of all features and their limits for a given plan.
// Used by the /subscription API to send the full limits table to the frontend.
func AllFeatureLimits(plan Plan) map[Feature]int {
	result := make(map[Feature]int)
	for _, entry := range limitsTable {
		if entry.Plan == plan {
			result[entry.Feature] = entry.Max
		}
	}
	return result
}
```

- [ ] **Step 4: Create limits_test.go — unit tests**

Create `services/marketplace-api/internal/plangate/limits_test.go`:

```go
package plangate

import "testing"

func TestGetLimit_Products(t *testing.T) {
	tests := []struct {
		name string
		plan Plan
		want int
	}{
		{"free gets 25", PlanFree, 25},
		{"starter gets 500", PlanStarter, 500},
		{"pro gets unlimited", PlanPro, Unlimited},
		{"enterprise gets unlimited", PlanEnterprise, Unlimited},
		{"marketplace gets unlimited", PlanMarketplace, Unlimited},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetLimit(tt.plan, FeatureProducts)
			if got != tt.want {
				t.Errorf("GetLimit(%s, products) = %d, want %d", tt.plan, got, tt.want)
			}
		})
	}
}

func TestGetLimit_Categories(t *testing.T) {
	if got := GetLimit(PlanFree, FeatureCategories); got != 5 {
		t.Errorf("free categories = %d, want 5", got)
	}
	if got := GetLimit(PlanStarter, FeatureCategories); got != 25 {
		t.Errorf("starter categories = %d, want 25", got)
	}
}

func TestGetLimit_UnknownFeature(t *testing.T) {
	if got := GetLimit(PlanPro, Feature("nonexistent")); got != Disabled {
		t.Errorf("unknown feature = %d, want 0", got)
	}
}

func TestGetLimit_UnknownPlan(t *testing.T) {
	if got := GetLimit(Plan("bogus"), FeatureProducts); got != Disabled {
		t.Errorf("unknown plan = %d, want 0", got)
	}
}

func TestIsAllowed_Boolean(t *testing.T) {
	tests := []struct {
		name    string
		plan    Plan
		feature Feature
		want    bool
	}{
		{"free no loyalty", PlanFree, FeatureLoyalty, false},
		{"starter no loyalty", PlanStarter, FeatureLoyalty, false},
		{"pro has loyalty", PlanPro, FeatureLoyalty, true},
		{"free no custom_css", PlanFree, FeatureCustomCSS, false},
		{"enterprise has custom_css", PlanEnterprise, FeatureCustomCSS, true},
		{"free no full palette", PlanFree, FeatureFullPalette, false},
		{"starter has full palette", PlanStarter, FeatureFullPalette, true},
		{"free no powered_by removal", PlanFree, FeatureRemovePoweredBy, false},
		{"pro has powered_by removal", PlanPro, FeatureRemovePoweredBy, true},
		{"free no vendor onboard", PlanFree, FeatureVendorOnboard, false},
		{"enterprise no vendor onboard", PlanEnterprise, FeatureVendorOnboard, false},
		{"marketplace has vendor onboard", PlanMarketplace, FeatureVendorOnboard, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsAllowed(tt.plan, tt.feature)
			if got != tt.want {
				t.Errorf("IsAllowed(%s, %s) = %v, want %v", tt.plan, tt.feature, got, tt.want)
			}
		})
	}
}

func TestIsUnlimited(t *testing.T) {
	if !IsUnlimited(PlanPro, FeatureProducts) {
		t.Error("pro products should be unlimited")
	}
	if IsUnlimited(PlanFree, FeatureProducts) {
		t.Error("free products should not be unlimited")
	}
}

func TestPlanAtLeast(t *testing.T) {
	if !PlanPro.AtLeast(PlanStarter) {
		t.Error("pro should be at least starter")
	}
	if PlanFree.AtLeast(PlanStarter) {
		t.Error("free should not be at least starter")
	}
	if !PlanEnterprise.AtLeast(PlanEnterprise) {
		t.Error("enterprise should be at least enterprise")
	}
}

func TestAllFeatureLimits(t *testing.T) {
	limits := AllFeatureLimits(PlanFree)
	if limits[FeatureProducts] != 25 {
		t.Errorf("free products = %d, want 25", limits[FeatureProducts])
	}
	if limits[FeatureLoyalty] != Disabled {
		t.Errorf("free loyalty = %d, want 0", limits[FeatureLoyalty])
	}
}

func TestIsValidPlan(t *testing.T) {
	if !IsValidPlan("free") {
		t.Error("'free' should be valid")
	}
	if !IsValidPlan("marketplace") {
		t.Error("'marketplace' should be valid")
	}
	if IsValidPlan("bogus") {
		t.Error("'bogus' should not be valid")
	}
}
```

### GREEN

- [ ] **Step 5: Run tests**

```bash
cd services/marketplace-api && go test ./internal/plangate/... -v -count=1
```

Expected: all tests pass.

**Commit:** `feat(plangate): add plan/feature types with limits table and unit tests`

---

## Task 2: plangate middleware — RequirePlan + EnforceLimit

**Files:**
- Create: `services/marketplace-api/internal/plangate/middleware.go`
- Create: `services/marketplace-api/internal/plangate/middleware_test.go`

### TDD: RED — Write tests first

- [ ] **Step 1: Create middleware.go**

The middleware reads the store's plan from a lookup function (injected at wiring time) and runs the gate check. It does NOT import `subscription` directly — the lookup is a `func(ctx, storeID) Plan` injected via config to avoid circular deps.

Create `services/marketplace-api/internal/plangate/middleware.go`:

```go
package plangate

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// PlanLookup resolves the current plan for a store. Wired at startup
// from subscription.Repository.GetPlanForStore.
type PlanLookup func(ctx context.Context, storeID string) (Plan, error)

// CountLookup returns the current count of a resource for a store.
// For example, total products, total active coupons, etc.
type CountLookup func(ctx context.Context, storeID string) (int, error)

// GateMiddleware holds the shared dependencies for all plan-gate checks.
type GateMiddleware struct {
	PlanLookup PlanLookup
	Logger     *slog.Logger
}

// NewGateMiddleware creates a new GateMiddleware.
func NewGateMiddleware(lookup PlanLookup, logger *slog.Logger) *GateMiddleware {
	return &GateMiddleware{PlanLookup: lookup, Logger: logger}
}

// gateError is the standard JSON response when a gate blocks a request.
type gateError struct {
	Error       string `json:"error"`
	Message     string `json:"message"`
	Feature     string `json:"feature,omitempty"`
	CurrentPlan string `json:"current_plan"`
	RequiredPlan string `json:"required_plan,omitempty"`
	Limit       int    `json:"limit,omitempty"`
	CurrentCount int   `json:"current_count,omitempty"`
}

// RequirePlan returns 403 if the store's plan is below minPlan.
// Use for boolean feature checks: RequirePlan(PlanPro) blocks Free + Starter.
func (gm *GateMiddleware) RequirePlan(minPlan Plan) gin.HandlerFunc {
	return func(c *gin.Context) {
		plan, err := gm.resolvePlan(c)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError,
				gin.H{"error": "plan_lookup_failed", "message": "Unable to determine subscription plan"})
			return
		}
		if !plan.AtLeast(minPlan) {
			c.AbortWithStatusJSON(http.StatusForbidden, gateError{
				Error:        "plan_required",
				Message:      fmt.Sprintf("This feature requires the %s plan or higher", minPlan),
				CurrentPlan:  string(plan),
				RequiredPlan: string(minPlan),
			})
			return
		}
		c.Set("store_plan", plan)
		c.Next()
	}
}

// RequireFeature returns 403 if the feature is not available on the store's plan.
// Use for boolean features: RequireFeature(FeatureLoyalty) checks the limits table.
func (gm *GateMiddleware) RequireFeature(feature Feature) gin.HandlerFunc {
	return func(c *gin.Context) {
		plan, err := gm.resolvePlan(c)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError,
				gin.H{"error": "plan_lookup_failed", "message": "Unable to determine subscription plan"})
			return
		}
		if !IsAllowed(plan, feature) {
			c.AbortWithStatusJSON(http.StatusForbidden, gateError{
				Error:       "feature_not_available",
				Message:     fmt.Sprintf("The %s feature is not available on your current plan", feature),
				Feature:     string(feature),
				CurrentPlan: string(plan),
			})
			return
		}
		c.Set("store_plan", plan)
		c.Next()
	}
}

// EnforceLimit returns 403 if the current count for the feature has reached
// the plan's limit. The countFn is a closure that queries the actual count
// (e.g., "SELECT count(*) FROM products WHERE store_id = ?").
func (gm *GateMiddleware) EnforceLimit(feature Feature, countFn CountLookup) gin.HandlerFunc {
	return func(c *gin.Context) {
		plan, err := gm.resolvePlan(c)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError,
				gin.H{"error": "plan_lookup_failed", "message": "Unable to determine subscription plan"})
			return
		}

		limit := GetLimit(plan, feature)
		if limit == Unlimited {
			c.Set("store_plan", plan)
			c.Next()
			return
		}
		if limit == Disabled {
			c.AbortWithStatusJSON(http.StatusForbidden, gateError{
				Error:       "feature_not_available",
				Message:     fmt.Sprintf("The %s feature is not available on your current plan", feature),
				Feature:     string(feature),
				CurrentPlan: string(plan),
			})
			return
		}

		storeID := c.Param("storeId")
		count, err := countFn(c.Request.Context(), storeID)
		if err != nil {
			gm.Logger.Error("plangate: count lookup failed",
				"feature", feature, "store_id", storeID, "err", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError,
				gin.H{"error": "count_lookup_failed", "message": "Unable to verify resource limits"})
			return
		}

		if count >= limit {
			c.AbortWithStatusJSON(http.StatusForbidden, gateError{
				Error:        "limit_reached",
				Message:      fmt.Sprintf("You've reached the %s limit (%d) for your %s plan. Upgrade to add more.", feature, limit, plan),
				Feature:      string(feature),
				CurrentPlan:  string(plan),
				Limit:        limit,
				CurrentCount: count,
			})
			return
		}

		c.Set("store_plan", plan)
		c.Next()
	}
}

// resolvePlan reads the plan from gin context (cached by a prior gate) or
// performs a fresh lookup. Caches for subsequent middleware in the chain.
func (gm *GateMiddleware) resolvePlan(c *gin.Context) (Plan, error) {
	if cached, exists := c.Get("store_plan"); exists {
		if p, ok := cached.(Plan); ok {
			return p, nil
		}
	}

	storeID := c.Param("storeId")
	plan, err := gm.PlanLookup(c.Request.Context(), storeID)
	if err != nil {
		gm.Logger.Error("plangate: plan lookup failed", "store_id", storeID, "err", err)
		return PlanFree, err
	}
	c.Set("store_plan", plan)
	return plan, nil
}
```

- [ ] **Step 2: Create middleware_test.go — unit tests**

Create `services/marketplace-api/internal/plangate/middleware_test.go`:

```go
package plangate

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func testGateMW(plan Plan) *GateMiddleware {
	return NewGateMiddleware(
		func(_ context.Context, _ string) (Plan, error) {
			return plan, nil
		},
		slog.Default(),
	)
}

func setupRouter(middlewares ...gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	g := r.Group("/admin/stores/:storeId")
	for _, mw := range middlewares {
		g.Use(mw)
	}
	g.POST("/products", func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{"ok": true})
	})
	return r
}

func TestRequirePlan_Allowed(t *testing.T) {
	gm := testGateMW(PlanPro)
	r := setupRouter(gm.RequirePlan(PlanStarter))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/admin/stores/store-1/products", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", w.Code)
	}
}

func TestRequirePlan_Blocked(t *testing.T) {
	gm := testGateMW(PlanFree)
	r := setupRouter(gm.RequirePlan(PlanStarter))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/admin/stores/store-1/products", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
	var body gateError
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Error != "plan_required" {
		t.Errorf("error = %q, want plan_required", body.Error)
	}
	if body.CurrentPlan != "free" {
		t.Errorf("current_plan = %q, want free", body.CurrentPlan)
	}
}

func TestRequireFeature_Allowed(t *testing.T) {
	gm := testGateMW(PlanPro)
	r := setupRouter(gm.RequireFeature(FeatureLoyalty))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/admin/stores/store-1/products", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", w.Code)
	}
}

func TestRequireFeature_Blocked(t *testing.T) {
	gm := testGateMW(PlanFree)
	r := setupRouter(gm.RequireFeature(FeatureLoyalty))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/admin/stores/store-1/products", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

func TestEnforceLimit_UnderLimit(t *testing.T) {
	gm := testGateMW(PlanFree) // 25 products max
	countFn := func(_ context.Context, _ string) (int, error) { return 10, nil }
	r := setupRouter(gm.EnforceLimit(FeatureProducts, countFn))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/admin/stores/store-1/products", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", w.Code)
	}
}

func TestEnforceLimit_AtLimit(t *testing.T) {
	gm := testGateMW(PlanFree) // 25 products max
	countFn := func(_ context.Context, _ string) (int, error) { return 25, nil }
	r := setupRouter(gm.EnforceLimit(FeatureProducts, countFn))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/admin/stores/store-1/products", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
	var body gateError
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Error != "limit_reached" {
		t.Errorf("error = %q, want limit_reached", body.Error)
	}
	if body.Limit != 25 {
		t.Errorf("limit = %d, want 25", body.Limit)
	}
	if body.CurrentCount != 25 {
		t.Errorf("current_count = %d, want 25", body.CurrentCount)
	}
}

func TestEnforceLimit_UnlimitedPlan(t *testing.T) {
	gm := testGateMW(PlanPro) // unlimited products
	countFn := func(_ context.Context, _ string) (int, error) { return 99999, nil }
	r := setupRouter(gm.EnforceLimit(FeatureProducts, countFn))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/admin/stores/store-1/products", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201 (unlimited plan)", w.Code)
	}
}

func TestEnforceLimit_DisabledFeature(t *testing.T) {
	gm := testGateMW(PlanFree)
	countFn := func(_ context.Context, _ string) (int, error) { return 0, nil }
	r := setupRouter(gm.EnforceLimit(FeatureCampaigns, countFn))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/admin/stores/store-1/products", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (disabled on free)", w.Code)
	}
}
```

### GREEN

- [ ] **Step 3: Run tests**

```bash
cd services/marketplace-api && go test ./internal/plangate/... -v -count=1
```

Expected: all tests pass.

**Commit:** `feat(plangate): add RequirePlan, RequireFeature, EnforceLimit Gin middleware`

---

## Task 3: Soft downgrade logic

**Files:**
- Create: `services/marketplace-api/internal/plangate/downgrade.go`
- Create: `services/marketplace-api/internal/plangate/downgrade_test.go`
- Modify: `services/marketplace-api/internal/subscription/models.go` (add fields)
- Modify: `services/marketplace-api/internal/subscription/repository.go` (add methods)

### TDD: RED

- [ ] **Step 1: Add downgrade fields to subscription model**

Modify `services/marketplace-api/internal/subscription/models.go` — add three fields to `StoreSubscription`:

```go
// Add these fields after CancelAtPeriodEnd:
TrialEndsAt        *time.Time `gorm:"column:trial_ends_at"                                  json:"trial_ends_at,omitempty"`
GracePeriodEndsAt  *time.Time `gorm:"column:grace_period_ends_at"                           json:"grace_period_ends_at,omitempty"`
DowngradedFrom     *string    `gorm:"column:downgraded_from;type:varchar(30)"               json:"downgraded_from,omitempty"`
```

Also add these to `SubscriptionResponse`:

```go
TrialEndsAt       *string `json:"trial_ends_at,omitempty"`
GracePeriodEndsAt *string `json:"grace_period_ends_at,omitempty"`
DowngradedFrom    *string `json:"downgraded_from,omitempty"`
```

And update the `ToResponse()` method to map them.

- [ ] **Step 2: Add migration for new columns**

Determine the next migration number by checking `ls services/marketplace-api/migrations/ | tail -1`. Assuming S3 shipped migration 000016, create `000017_subscription_downgrade_fields.up.sql`:

```sql
BEGIN;

ALTER TABLE store_subscriptions
    ADD COLUMN IF NOT EXISTS trial_ends_at         TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS grace_period_ends_at  TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS downgraded_from       VARCHAR(30);

COMMIT;
```

Create `000017_subscription_downgrade_fields.down.sql`:

```sql
BEGIN;

ALTER TABLE store_subscriptions
    DROP COLUMN IF EXISTS trial_ends_at,
    DROP COLUMN IF EXISTS grace_period_ends_at,
    DROP COLUMN IF EXISTS downgraded_from;

COMMIT;
```

Update the `ExpectedSchemaVersion` constant in the root package to `17`.

- [ ] **Step 3: Add GetPlanForStore to repository**

Modify `services/marketplace-api/internal/subscription/repository.go` — add:

```go
// GetPlanForStore returns the plan string for a store. Returns "free" if no
// subscription record exists. This is the hot path used by plangate middleware.
func (r *Repository) GetPlanForStore(ctx context.Context, storeID string) (string, error) {
	var plan string
	err := r.db.WithContext(ctx).
		Model(&StoreSubscription{}).
		Select("plan").
		Where("store_id = ?", storeID).
		Scan(&plan).Error
	if err != nil {
		return "free", nil // No subscription = free tier
	}
	if plan == "" {
		return "free", nil
	}
	return plan, nil
}

// SetDowngrade marks a subscription as soft-downgraded with a 30-day grace period.
func (r *Repository) SetDowngrade(ctx context.Context, storeID string, previousPlan string, graceEndsAt time.Time) error {
	return r.db.WithContext(ctx).
		Model(&StoreSubscription{}).
		Where("store_id = ?", storeID).
		Updates(map[string]interface{}{
			"plan":                  "free",
			"downgraded_from":       previousPlan,
			"grace_period_ends_at":  graceEndsAt,
			"updated_at":            time.Now(),
		}).Error
}

// ClearGracePeriod clears the grace period fields after it expires.
func (r *Repository) ClearGracePeriod(ctx context.Context, storeID string) error {
	return r.db.WithContext(ctx).
		Model(&StoreSubscription{}).
		Where("store_id = ?", storeID).
		Updates(map[string]interface{}{
			"downgraded_from":      nil,
			"grace_period_ends_at": nil,
			"updated_at":           time.Now(),
		}).Error
}
```

- [ ] **Step 4: Create downgrade.go — soft downgrade logic**

Create `services/marketplace-api/internal/plangate/downgrade.go`:

```go
package plangate

import (
	"time"
)

// GracePeriodDays is the number of days after trial expiry before
// hard enforcement kicks in (excess items become truly read-only).
const GracePeriodDays = 30

// DowngradeState describes the current state of a store's downgrade.
type DowngradeState struct {
	// IsDowngraded is true if the store was previously on a higher plan.
	IsDowngraded bool
	// PreviousPlan is the plan the store was on before downgrade.
	PreviousPlan Plan
	// InGracePeriod is true if the 30-day grace window is still active.
	InGracePeriod bool
	// GraceEndsAt is when the grace period expires.
	GraceEndsAt time.Time
	// DaysRemaining in the grace period (-1 if not in grace).
	DaysRemaining int
}

// CheckDowngrade evaluates a store's downgrade state.
// downgradedFrom is nil if the store was never downgraded.
// graceEndsAt is nil if no grace period is set.
func CheckDowngrade(downgradedFrom *string, graceEndsAt *time.Time, now time.Time) DowngradeState {
	if downgradedFrom == nil || *downgradedFrom == "" {
		return DowngradeState{}
	}

	state := DowngradeState{
		IsDowngraded: true,
		PreviousPlan: Plan(*downgradedFrom),
	}

	if graceEndsAt != nil && now.Before(*graceEndsAt) {
		state.InGracePeriod = true
		state.GraceEndsAt = *graceEndsAt
		state.DaysRemaining = int(graceEndsAt.Sub(now).Hours() / 24)
		if state.DaysRemaining < 0 {
			state.DaysRemaining = 0
		}
	}

	return state
}

// TrialState describes the trial status.
type TrialState struct {
	// IsTrialing is true if the store is on an active trial.
	IsTrialing bool
	// TrialEndsAt is when the trial expires.
	TrialEndsAt time.Time
	// DaysRemaining until trial expiry (-1 if not trialing).
	DaysRemaining int
	// IsExpired is true if the trial has passed its end date.
	IsExpired bool
}

// CheckTrial evaluates a store's trial state.
func CheckTrial(status string, trialEndsAt *time.Time, now time.Time) TrialState {
	if status != "trialing" || trialEndsAt == nil {
		return TrialState{}
	}

	state := TrialState{
		IsTrialing:  true,
		TrialEndsAt: *trialEndsAt,
	}

	if now.After(*trialEndsAt) {
		state.IsExpired = true
		state.DaysRemaining = 0
	} else {
		state.DaysRemaining = int(trialEndsAt.Sub(now).Hours()/24) + 1
	}

	return state
}

// ShouldBlockCreate returns true if the store should be blocked from creating
// new resources. After grace period, downgraded stores cannot create items
// that exceed the new plan's limits. During grace, creates are still allowed.
func ShouldBlockCreate(downgrade DowngradeState) bool {
	if !downgrade.IsDowngraded {
		return false
	}
	// During grace period, allow creates (soft downgrade).
	if downgrade.InGracePeriod {
		return false
	}
	// Past grace period — hard enforcement.
	return true
}
```

- [ ] **Step 5: Create downgrade_test.go**

Create `services/marketplace-api/internal/plangate/downgrade_test.go`:

```go
package plangate

import (
	"testing"
	"time"
)

func ptr[T any](v T) *T { return &v }

func TestCheckDowngrade_NotDowngraded(t *testing.T) {
	state := CheckDowngrade(nil, nil, time.Now())
	if state.IsDowngraded {
		t.Error("should not be downgraded")
	}
}

func TestCheckDowngrade_InGrace(t *testing.T) {
	now := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	graceEnd := now.Add(15 * 24 * time.Hour)
	state := CheckDowngrade(ptr("starter"), &graceEnd, now)

	if !state.IsDowngraded {
		t.Error("should be downgraded")
	}
	if !state.InGracePeriod {
		t.Error("should be in grace period")
	}
	if state.DaysRemaining != 15 {
		t.Errorf("days remaining = %d, want 15", state.DaysRemaining)
	}
	if state.PreviousPlan != PlanStarter {
		t.Errorf("previous plan = %s, want starter", state.PreviousPlan)
	}
}

func TestCheckDowngrade_PastGrace(t *testing.T) {
	now := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	graceEnd := now.Add(-5 * 24 * time.Hour) // 5 days ago
	state := CheckDowngrade(ptr("pro"), &graceEnd, now)

	if !state.IsDowngraded {
		t.Error("should be downgraded")
	}
	if state.InGracePeriod {
		t.Error("should NOT be in grace period")
	}
}

func TestCheckTrial_Active(t *testing.T) {
	now := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	trialEnd := now.Add(7 * 24 * time.Hour)
	state := CheckTrial("trialing", &trialEnd, now)

	if !state.IsTrialing {
		t.Error("should be trialing")
	}
	if state.DaysRemaining != 8 { // 7 full days + 1
		t.Errorf("days remaining = %d, want 8", state.DaysRemaining)
	}
	if state.IsExpired {
		t.Error("should not be expired")
	}
}

func TestCheckTrial_Expired(t *testing.T) {
	now := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	trialEnd := now.Add(-1 * 24 * time.Hour)
	state := CheckTrial("trialing", &trialEnd, now)

	if !state.IsExpired {
		t.Error("should be expired")
	}
}

func TestCheckTrial_NotTrialing(t *testing.T) {
	state := CheckTrial("active", nil, time.Now())
	if state.IsTrialing {
		t.Error("should not be trialing")
	}
}

func TestShouldBlockCreate_NoDowngrade(t *testing.T) {
	if ShouldBlockCreate(DowngradeState{}) {
		t.Error("should not block when not downgraded")
	}
}

func TestShouldBlockCreate_InGrace(t *testing.T) {
	ds := DowngradeState{IsDowngraded: true, InGracePeriod: true}
	if ShouldBlockCreate(ds) {
		t.Error("should not block during grace period")
	}
}

func TestShouldBlockCreate_PastGrace(t *testing.T) {
	ds := DowngradeState{IsDowngraded: true, InGracePeriod: false}
	if !ShouldBlockCreate(ds) {
		t.Error("should block after grace period")
	}
}
```

### GREEN

- [ ] **Step 6: Apply migration and run all tests**

```bash
cd services/marketplace-api && \
  DATABASE_URL="postgres://dev:dev@localhost:5432/marketplace_db?sslmode=disable" go run ./cmd/migrate up && \
  go test ./internal/plangate/... ./internal/subscription/... -v -count=1
```

**Commit:** `feat(plangate): add soft downgrade logic with grace period and trial state`

---

## Task 4: Wire plangate middleware into admin routes

**Files:**
- Modify: `services/marketplace-api/internal/handlers/admin/routes.go`
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go`

- [ ] **Step 1: Add PlanGate to admin Deps struct**

Modify `services/marketplace-api/internal/handlers/admin/routes.go` — add to the `Deps` struct:

```go
// Add to Deps struct after AuthzMiddleware:
PlanGate *plangate.GateMiddleware
// Count functions for limit enforcement:
ProductCount  plangate.CountLookup
CategoryCount plangate.CountLookup
CouponCount   plangate.CountLookup
```

Add import:

```go
"github.com/mark8ly/marketplace-api/internal/plangate"
```

- [ ] **Step 2: Wire plangate on create/mutation endpoints**

In `RegisterAdmin`, add `EnforceLimit` middleware to create endpoints. The plangate middleware slots AFTER authz (RequireTenantRelation) and BEFORE the handler. This follows the existing middleware chain pattern: `authMW → StoreMiddleware → RequireTenantRelation → PlanGate → Handler`.

Modify the products create route:

```go
// Before:
products.POST("", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin), deps.ProductHandler.Create)

// After:
products.POST("", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
    deps.PlanGate.EnforceLimit(plangate.FeatureProducts, deps.ProductCount),
    deps.ProductHandler.Create)
```

Modify the categories create route:

```go
// Before:
categories.POST("", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin), deps.CategoryHandler.Create)

// After:
categories.POST("", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
    deps.PlanGate.EnforceLimit(plangate.FeatureCategories, deps.CategoryCount),
    deps.CategoryHandler.Create)
```

Modify the coupons create route:

```go
// Before:
coupons.POST("", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin), deps.CouponHandler.Create)

// After:
coupons.POST("", deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
    deps.PlanGate.EnforceLimit(plangate.FeatureCoupons, deps.CouponCount),
    deps.CouponHandler.Create)
```

Gate the loyalty group behind RequireFeature:

```go
// Add at the top of the loyalty group, before individual routes:
if deps.LoyaltyHandler != nil {
    loyaltyGroup := storeRoute.Group("/loyalty",
        deps.PlanGate.RequireFeature(plangate.FeatureLoyalty))
    // ... existing routes unchanged inside
}
```

Gate gift cards behind RequireFeature:

```go
if deps.GiftCardHandler != nil {
    gc := storeRoute.Group("/gift-cards",
        deps.PlanGate.RequireFeature(plangate.FeatureGiftCards))
    // ... existing routes unchanged inside
}
```

Gate CSV import/export behind RequireFeature:

```go
if deps.CSVImportsHandler != nil {
    // Add RequireFeature to both the export and import routes:
    products.GET("/export.csv",
        deps.AuthzMiddleware.RequireTenantRelation(authz.CSVExportRole),
        deps.PlanGate.RequireFeature(plangate.FeatureCSVImportExport),
        deps.CSVImportsHandler.Export)

    csvImports := storeRoute.Group("/csv-imports",
        deps.PlanGate.RequireFeature(plangate.FeatureCSVImportExport))
    // ... existing routes unchanged inside
}
```

Gate returns behind RequireFeature:

```go
if deps.ReturnsHandler != nil {
    storeRoute.POST("/orders/:id/returns",
        deps.AuthzMiddleware.RequireTenantRelation(authz.ReturnsEditRole),
        deps.PlanGate.RequireFeature(plangate.FeatureReturns),
        deps.ReturnsHandler.Request)
    returns := storeRoute.Group("/returns",
        deps.PlanGate.RequireFeature(plangate.FeatureReturns))
    // ... existing routes unchanged inside
}
```

- [ ] **Step 3: Wire plangate in main.go**

Modify `services/marketplace-api/cmd/marketplace-api/main.go` — add the plangate wiring inside the `if m == mode.Admin || m == mode.Both` block, after subscription repository setup:

```go
// Add import:
"github.com/mark8ly/marketplace-api/internal/plangate"

// Inside admin wiring block, after subscriptionRepo is created:
// Plan lookup function — bridges plangate to subscription.Repository.
planLookup := func(ctx context.Context, storeID string) (plangate.Plan, error) {
    plan, err := subscriptionRepo.GetPlanForStore(ctx, storeID)
    if err != nil {
        return plangate.PlanFree, err
    }
    return plangate.Plan(plan), nil
}
planGateMW := plangate.NewGateMiddleware(planLookup, log)

// Count functions for numeric limits.
productCountFn := func(ctx context.Context, storeID string) (int, error) {
    var count int64
    err := conn.WithContext(ctx).
        Model(&product.Product{}).
        Where("store_id = ?", storeID).
        Count(&count).Error
    return int(count), err
}

categoryCountFn := func(ctx context.Context, storeID string) (int, error) {
    var count int64
    err := conn.WithContext(ctx).
        Model(&category.Category{}).
        Where("store_id = ?", storeID).
        Count(&count).Error
    return int(count), err
}

couponCountFn := func(ctx context.Context, storeID string) (int, error) {
    var count int64
    err := conn.WithContext(ctx).
        Model(&coupon.Coupon{}).
        Where("store_id = ? AND status = 'active'", storeID).
        Count(&count).Error
    return int(count), err
}
```

Then add to adminDeps:

```go
adminDeps = admin.Deps{
    // ... existing fields ...
    PlanGate:      planGateMW,
    ProductCount:  productCountFn,
    CategoryCount: categoryCountFn,
    CouponCount:   couponCountFn,
}
```

### GREEN

- [ ] **Step 4: Verify build**

```bash
cd services/marketplace-api && go build ./...
```

- [ ] **Step 5: Run all tests**

```bash
cd services/marketplace-api && go test ./... -count=1
```

**Commit:** `feat(plangate): wire plan-gate middleware into admin routes for products, categories, coupons, loyalty, gift cards, returns, CSV`

---

## Task 5: Subscription API — add limits + plan info to response

**Files:**
- Modify: `services/marketplace-api/internal/subscription/handler.go`
- Modify: `apps/admin/lib/api/subscription-api.ts` (or create if S3 used a different path)

- [ ] **Step 1: Enhance the GET /admin/stores/:storeId/subscription response**

Modify `services/marketplace-api/internal/subscription/handler.go` — in the `GetSubscription` handler, enrich the response with plan limits and downgrade/trial state:

```go
// Add to the existing GetSubscription handler's response:
import "github.com/mark8ly/marketplace-api/internal/plangate"

type EnrichedSubscriptionResponse struct {
    SubscriptionResponse
    Limits        map[string]int   `json:"limits"`
    Trial         *TrialInfo       `json:"trial,omitempty"`
    Downgrade     *DowngradeInfo   `json:"downgrade,omitempty"`
}

type TrialInfo struct {
    IsTrialing    bool   `json:"is_trialing"`
    DaysRemaining int    `json:"days_remaining"`
    EndsAt        string `json:"ends_at"`
    IsExpired     bool   `json:"is_expired"`
}

type DowngradeInfo struct {
    IsDowngraded  bool   `json:"is_downgraded"`
    PreviousPlan  string `json:"previous_plan"`
    InGracePeriod bool   `json:"in_grace_period"`
    DaysRemaining int    `json:"days_remaining"`
    GraceEndsAt   string `json:"grace_ends_at,omitempty"`
}
```

In the handler body, after fetching the subscription:

```go
plan := plangate.Plan(sub.Plan)
limits := plangate.AllFeatureLimits(plan)
// Convert Feature keys to strings for JSON.
strLimits := make(map[string]int, len(limits))
for k, v := range limits {
    strLimits[string(k)] = v
}

now := time.Now()
trialState := plangate.CheckTrial(sub.Status, sub.TrialEndsAt, now)
downgradeState := plangate.CheckDowngrade(sub.DowngradedFrom, sub.GracePeriodEndsAt, now)

resp := EnrichedSubscriptionResponse{
    SubscriptionResponse: sub.ToResponse(),
    Limits:               strLimits,
}
if trialState.IsTrialing {
    resp.Trial = &TrialInfo{
        IsTrialing:    true,
        DaysRemaining: trialState.DaysRemaining,
        EndsAt:        trialState.TrialEndsAt.Format(time.RFC3339),
        IsExpired:     trialState.IsExpired,
    }
}
if downgradeState.IsDowngraded {
    di := &DowngradeInfo{
        IsDowngraded:  true,
        PreviousPlan:  string(downgradeState.PreviousPlan),
        InGracePeriod: downgradeState.InGracePeriod,
        DaysRemaining: downgradeState.DaysRemaining,
    }
    if !downgradeState.GraceEndsAt.IsZero() {
        di.GraceEndsAt = downgradeState.GraceEndsAt.Format(time.RFC3339)
    }
    resp.Downgrade = di
}

c.JSON(http.StatusOK, gin.H{"data": resp})
```

- [ ] **Step 2: Update the admin TypeScript types**

Modify (or create) `apps/admin/lib/api/subscription-api.ts`:

```typescript
const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

import type { SessionHeaders } from "./marketplace-api";

export interface SubscriptionPlan {
  id: string;
  plan: "free" | "starter" | "pro" | "enterprise" | "marketplace";
  status: "active" | "trialing" | "past_due" | "cancelled" | "incomplete";
  current_period_start: string | null;
  current_period_end: string | null;
  cancel_at_period_end: boolean;
  trial_ends_at: string | null;
  grace_period_ends_at: string | null;
  downgraded_from: string | null;
  limits: Record<string, number>;
  trial: {
    is_trialing: boolean;
    days_remaining: number;
    ends_at: string;
    is_expired: boolean;
  } | null;
  downgrade: {
    is_downgraded: boolean;
    previous_plan: string;
    in_grace_period: boolean;
    days_remaining: number;
    grace_ends_at: string | null;
  } | null;
  created_at: string;
  updated_at: string;
}

export type PlanTier = SubscriptionPlan["plan"];

// Feature keys — must match Go plangate.Feature constants.
export type Feature =
  | "products"
  | "categories"
  | "staff"
  | "stores"
  | "orders_per_month"
  | "active_coupons"
  | "campaigns_per_month"
  | "full_color_palette"
  | "announcement_bar"
  | "remove_powered_by"
  | "custom_css"
  | "custom_domain"
  | "returns"
  | "label_tracking"
  | "gift_cards"
  | "loyalty"
  | "reviews"
  | "support_tickets"
  | "priority_support"
  | "audit_logs"
  | "mobile_app"
  | "csv_import_export"
  | "vendor_onboarding";

const UNLIMITED = -1;
const DISABLED = 0;

export function isFeatureAllowed(
  limits: Record<string, number>,
  feature: Feature,
): boolean {
  const limit = limits[feature] ?? DISABLED;
  return limit !== DISABLED;
}

export function getFeatureLimit(
  limits: Record<string, number>,
  feature: Feature,
): number {
  return limits[feature] ?? DISABLED;
}

export function isUnlimited(
  limits: Record<string, number>,
  feature: Feature,
): boolean {
  return (limits[feature] ?? DISABLED) === UNLIMITED;
}

export async function fetchSubscription(
  storeId: string,
  session: SessionHeaders,
): Promise<SubscriptionPlan | null> {
  try {
    const res = await fetch(
      `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/subscription`,
      {
        cache: "no-store",
        headers: {
          "X-User-Id": session.userId,
          "X-Tenant-Id": session.tenantId,
        },
      },
    );
    if (!res.ok) return null;
    const body = (await res.json()) as { data: SubscriptionPlan };
    return body.data;
  } catch {
    return null;
  }
}
```

### GREEN

- [ ] **Step 3: Verify Go build + tests**

```bash
cd services/marketplace-api && go build ./... && go test ./internal/subscription/... -v -count=1
```

- [ ] **Step 4: Verify TypeScript compiles**

```bash
cd apps/admin && npx tsc --noEmit
```

**Commit:** `feat(plangate): enrich subscription API with limits, trial, and downgrade state`

---

## Task 6: Admin UI — useSubscription hook

**Files:**
- Create: `apps/admin/lib/hooks/use-subscription.ts`
- Create: `apps/admin/lib/hooks/use-subscription.test.ts`

- [ ] **Step 1: Create the useSubscription hook**

Create `apps/admin/lib/hooks/use-subscription.ts`:

```typescript
"use client";

import { createContext, useContext, type ReactNode } from "react";
import type {
  SubscriptionPlan,
  Feature,
  PlanTier,
} from "@/lib/api/subscription-api";
import {
  isFeatureAllowed,
  getFeatureLimit,
  isUnlimited,
} from "@/lib/api/subscription-api";

interface SubscriptionContextValue {
  /** The full subscription object, null while loading or on error. */
  subscription: SubscriptionPlan | null;
  /** Current plan tier. Defaults to "free" if subscription is null. */
  plan: PlanTier;
  /** Check if a feature is available on the current plan. */
  isAllowed: (feature: Feature) => boolean;
  /** Get the numeric limit for a feature. Returns 0 if not available. */
  getLimit: (feature: Feature) => number;
  /** Check if a feature is unlimited on the current plan. */
  isFeatureUnlimited: (feature: Feature) => boolean;
  /** Trial state, null if not trialing. */
  trial: SubscriptionPlan["trial"];
  /** Downgrade state, null if not downgraded. */
  downgrade: SubscriptionPlan["downgrade"];
}

const SubscriptionContext = createContext<SubscriptionContextValue>({
  subscription: null,
  plan: "free",
  isAllowed: () => false,
  getLimit: () => 0,
  isFeatureUnlimited: () => false,
  trial: null,
  downgrade: null,
});

interface SubscriptionProviderProps {
  children: ReactNode;
  subscription: SubscriptionPlan | null;
}

export function SubscriptionProvider({
  children,
  subscription,
}: SubscriptionProviderProps) {
  const limits = subscription?.limits ?? {};
  const plan: PlanTier = subscription?.plan ?? "free";

  const value: SubscriptionContextValue = {
    subscription,
    plan,
    isAllowed: (feature: Feature) => isFeatureAllowed(limits, feature),
    getLimit: (feature: Feature) => getFeatureLimit(limits, feature),
    isFeatureUnlimited: (feature: Feature) => isUnlimited(limits, feature),
    trial: subscription?.trial ?? null,
    downgrade: subscription?.downgrade ?? null,
  };

  return (
    <SubscriptionContext value={value}>
      {children}
    </SubscriptionContext>
  );
}

export function useSubscription(): SubscriptionContextValue {
  return useContext(SubscriptionContext);
}
```

- [ ] **Step 2: Create unit tests**

Create `apps/admin/lib/hooks/use-subscription.test.ts`:

```typescript
import { describe, it, expect } from "vitest";
import {
  isFeatureAllowed,
  getFeatureLimit,
  isUnlimited,
} from "@/lib/api/subscription-api";

const freeLimits: Record<string, number> = {
  products: 25,
  categories: 5,
  staff: 1,
  stores: 1,
  orders_per_month: 50,
  active_coupons: 5,
  campaigns_per_month: 0,
  full_color_palette: 0,
  loyalty: 0,
  custom_css: 0,
  csv_import_export: 0,
};

const proLimits: Record<string, number> = {
  products: -1,
  categories: -1,
  staff: 10,
  loyalty: 1,
  custom_css: 0,
  csv_import_export: 1,
};

describe("isFeatureAllowed", () => {
  it("returns true for features with numeric limits > 0", () => {
    expect(isFeatureAllowed(freeLimits, "products")).toBe(true);
  });

  it("returns false for disabled features (0)", () => {
    expect(isFeatureAllowed(freeLimits, "loyalty")).toBe(false);
    expect(isFeatureAllowed(freeLimits, "campaigns_per_month")).toBe(false);
  });

  it("returns true for unlimited features (-1)", () => {
    expect(isFeatureAllowed(proLimits, "products")).toBe(true);
  });

  it("returns false for unknown features", () => {
    expect(isFeatureAllowed(freeLimits, "nonexistent" as never)).toBe(false);
  });
});

describe("getFeatureLimit", () => {
  it("returns the numeric limit", () => {
    expect(getFeatureLimit(freeLimits, "products")).toBe(25);
    expect(getFeatureLimit(freeLimits, "staff")).toBe(1);
  });

  it("returns 0 for disabled features", () => {
    expect(getFeatureLimit(freeLimits, "loyalty")).toBe(0);
  });
});

describe("isUnlimited", () => {
  it("returns true for -1", () => {
    expect(isUnlimited(proLimits, "products")).toBe(true);
  });

  it("returns false for numeric limits", () => {
    expect(isUnlimited(freeLimits, "products")).toBe(false);
  });
});
```

### GREEN

- [ ] **Step 3: Run tests**

```bash
cd apps/admin && npx vitest run lib/hooks/use-subscription.test.ts
```

**Commit:** `feat(admin): add useSubscription hook and SubscriptionProvider context`

---

## Task 7: Admin UI — PlanGate + UpgradePrompt components

**Files:**
- Create: `apps/admin/components/plangate/PlanGate.tsx`
- Create: `apps/admin/components/plangate/PlanGate.test.tsx`
- Create: `apps/admin/components/plangate/UpgradePrompt.tsx`
- Create: `apps/admin/components/plangate/UpgradePrompt.test.tsx`

- [ ] **Step 1: Create PlanGate component**

Create `apps/admin/components/plangate/PlanGate.tsx`:

```tsx
"use client";

import type { ReactNode } from "react";
import { useSubscription } from "@/lib/hooks/use-subscription";
import type { Feature } from "@/lib/api/subscription-api";

interface PlanGateProps {
  /** The feature to check. */
  feature: Feature;
  /** Rendered when the feature is available. */
  children: ReactNode;
  /** Rendered when the feature is NOT available. Defaults to null. */
  fallback?: ReactNode;
}

/**
 * PlanGate conditionally renders children based on the store's subscription plan.
 * This is a UX component only — server-side middleware is the true enforcement.
 *
 * Usage:
 * ```tsx
 * <PlanGate feature="loyalty" fallback={<UpgradePrompt feature="loyalty" />}>
 *   <LoyaltySettings />
 * </PlanGate>
 * ```
 */
export function PlanGate({ feature, children, fallback = null }: PlanGateProps) {
  const { isAllowed } = useSubscription();

  if (!isAllowed(feature)) {
    return <>{fallback}</>;
  }

  return <>{children}</>;
}
```

- [ ] **Step 2: Create PlanGate tests**

Create `apps/admin/components/plangate/PlanGate.test.tsx`:

```tsx
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { PlanGate } from "./PlanGate";
import { SubscriptionProvider } from "@/lib/hooks/use-subscription";
import type { SubscriptionPlan } from "@/lib/api/subscription-api";

function makeSubscription(
  plan: SubscriptionPlan["plan"],
  limits: Record<string, number>,
): SubscriptionPlan {
  return {
    id: "sub-1",
    plan,
    status: "active",
    current_period_start: null,
    current_period_end: null,
    cancel_at_period_end: false,
    trial_ends_at: null,
    grace_period_ends_at: null,
    downgraded_from: null,
    limits,
    trial: null,
    downgrade: null,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

describe("PlanGate", () => {
  it("renders children when feature is allowed", () => {
    const sub = makeSubscription("pro", { loyalty: 1 });
    render(
      <SubscriptionProvider subscription={sub}>
        <PlanGate feature="loyalty">
          <p>Loyalty Settings</p>
        </PlanGate>
      </SubscriptionProvider>,
    );
    expect(screen.getByText("Loyalty Settings")).toBeTruthy();
  });

  it("renders fallback when feature is not allowed", () => {
    const sub = makeSubscription("free", { loyalty: 0 });
    render(
      <SubscriptionProvider subscription={sub}>
        <PlanGate feature="loyalty" fallback={<p>Upgrade needed</p>}>
          <p>Loyalty Settings</p>
        </PlanGate>
      </SubscriptionProvider>,
    );
    expect(screen.queryByText("Loyalty Settings")).toBeNull();
    expect(screen.getByText("Upgrade needed")).toBeTruthy();
  });

  it("renders nothing when feature is not allowed and no fallback", () => {
    const sub = makeSubscription("free", { loyalty: 0 });
    const { container } = render(
      <SubscriptionProvider subscription={sub}>
        <PlanGate feature="loyalty">
          <p>Loyalty Settings</p>
        </PlanGate>
      </SubscriptionProvider>,
    );
    expect(container.textContent).toBe("");
  });
});
```

- [ ] **Step 3: Create UpgradePrompt component**

Create `apps/admin/components/plangate/UpgradePrompt.tsx`:

```tsx
"use client";

import Link from "next/link";
import { ArrowUpRight } from "lucide-react";
import { useSubscription } from "@/lib/hooks/use-subscription";
import type { Feature } from "@/lib/api/subscription-api";

/** Human-readable labels for features shown in upgrade prompts. */
const featureLabels: Record<Feature, string> = {
  products: "More products",
  categories: "More categories",
  staff: "More staff members",
  stores: "More stores",
  orders_per_month: "More orders per month",
  active_coupons: "More active coupons",
  campaigns_per_month: "Marketing campaigns",
  full_color_palette: "Full color palette & fonts",
  announcement_bar: "Announcement bar",
  remove_powered_by: 'Remove "Powered by mark8ly"',
  custom_css: "Custom CSS",
  custom_domain: "Custom domain",
  returns: "Returns & refunds",
  label_tracking: "Shipping label creation & tracking",
  gift_cards: "Gift cards",
  loyalty: "Loyalty program",
  reviews: "Product reviews",
  support_tickets: "Support tickets",
  priority_support: "Priority support",
  audit_logs: "Audit logs",
  mobile_app: "Mobile app",
  csv_import_export: "CSV import/export",
  vendor_onboarding: "Vendor onboarding",
};

interface UpgradePromptProps {
  /** The feature that triggered the prompt — used for labeling + analytics. */
  feature: Feature;
  /** Optional override for the title text. */
  title?: string;
  /** Optional CSS className. */
  className?: string;
}

/**
 * UpgradePrompt — shown when a feature is gated by the current plan.
 * Links to /settings/subscription with a query param for analytics.
 */
export function UpgradePrompt({
  feature,
  title,
  className = "",
}: UpgradePromptProps) {
  const { plan } = useSubscription();
  const label = featureLabels[feature] ?? feature;
  const heading = title ?? `Unlock ${label}`;

  return (
    <div
      className={`rounded-md border border-border-subtle bg-background-elevated p-6 ${className}`}
    >
      <p className="font-serif text-lg font-medium text-foreground">
        {heading}
      </p>
      <p className="mt-2 text-sm text-foreground-secondary">
        {label} is available on a higher plan. Upgrade to unlock this feature
        and grow your store.
      </p>
      <Link
        href={`/settings/subscription?upgrade_trigger=${feature}&from_plan=${plan}`}
        className="mt-4 inline-flex items-center gap-1.5 rounded-md bg-foreground px-4 py-2.5 text-sm font-medium text-background transition-colors hover:bg-foreground/90"
      >
        View plans
        <ArrowUpRight className="h-3.5 w-3.5" aria-hidden="true" />
      </Link>
    </div>
  );
}
```

- [ ] **Step 4: Create UpgradePrompt tests**

Create `apps/admin/components/plangate/UpgradePrompt.test.tsx`:

```tsx
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { UpgradePrompt } from "./UpgradePrompt";
import { SubscriptionProvider } from "@/lib/hooks/use-subscription";
import type { SubscriptionPlan } from "@/lib/api/subscription-api";

function makeFreeSub(): SubscriptionPlan {
  return {
    id: "sub-1",
    plan: "free",
    status: "active",
    current_period_start: null,
    current_period_end: null,
    cancel_at_period_end: false,
    trial_ends_at: null,
    grace_period_ends_at: null,
    downgraded_from: null,
    limits: { loyalty: 0 },
    trial: null,
    downgrade: null,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

describe("UpgradePrompt", () => {
  it("renders with feature label and link", () => {
    render(
      <SubscriptionProvider subscription={makeFreeSub()}>
        <UpgradePrompt feature="loyalty" />
      </SubscriptionProvider>,
    );
    expect(screen.getByText("Unlock Loyalty program")).toBeTruthy();
    const link = screen.getByRole("link", { name: /view plans/i });
    expect(link.getAttribute("href")).toContain(
      "/settings/subscription?upgrade_trigger=loyalty&from_plan=free",
    );
  });

  it("renders with custom title", () => {
    render(
      <SubscriptionProvider subscription={makeFreeSub()}>
        <UpgradePrompt feature="loyalty" title="Get Loyalty" />
      </SubscriptionProvider>,
    );
    expect(screen.getByText("Get Loyalty")).toBeTruthy();
  });
});
```

### GREEN

- [ ] **Step 5: Run tests**

```bash
cd apps/admin && npx vitest run components/plangate/
```

**Commit:** `feat(admin): add PlanGate and UpgradePrompt components`

---

## Task 8: Trial expiry banners in admin header

**Files:**
- Create: `apps/admin/components/plangate/TrialExpiryBanner.tsx`
- Modify: `apps/admin/components/shell/AdminShell.tsx`

- [ ] **Step 1: Create TrialExpiryBanner component**

Create `apps/admin/components/plangate/TrialExpiryBanner.tsx`:

```tsx
"use client";

import { useState } from "react";
import Link from "next/link";
import { X } from "lucide-react";
import { useSubscription } from "@/lib/hooks/use-subscription";

/**
 * TrialExpiryBanner shows in the admin header when:
 * - Trial has <= 7 days remaining
 * - Trial has <= 1 day remaining (urgent variant)
 * - Trial has expired (post-downgrade variant)
 */
export function TrialExpiryBanner() {
  const { trial, downgrade } = useSubscription();
  const [dismissed, setDismissed] = useState(false);

  if (dismissed) return null;

  // Post-downgrade banner.
  if (downgrade?.is_downgraded) {
    return (
      <div className="border-b border-signal/20 bg-signal/5 px-4 py-2.5">
        <div className="flex items-center justify-between gap-4">
          <p className="text-sm text-foreground">
            <span className="font-medium">Your trial has ended.</span>{" "}
            {downgrade.in_grace_period
              ? `You have ${downgrade.days_remaining} days to upgrade before limits take effect.`
              : "Some features are now limited. Upgrade to restore full access."}
          </p>
          <div className="flex items-center gap-3">
            <Link
              href="/settings/subscription?upgrade_trigger=trial_expired"
              className="whitespace-nowrap rounded-md bg-foreground px-3 py-1.5 text-xs font-medium text-background hover:bg-foreground/90"
            >
              Upgrade now
            </Link>
            <button
              type="button"
              onClick={() => setDismissed(true)}
              className="text-foreground-secondary hover:text-foreground"
              aria-label="Dismiss banner"
            >
              <X className="h-4 w-4" aria-hidden="true" />
            </button>
          </div>
        </div>
      </div>
    );
  }

  // Trial active but expiring soon.
  if (!trial?.is_trialing || trial.days_remaining > 7) return null;

  const isUrgent = trial.days_remaining <= 1;

  return (
    <div
      className={`border-b px-4 py-2.5 ${
        isUrgent
          ? "border-signal/30 bg-signal/10"
          : "border-warning/20 bg-warning/5"
      }`}
    >
      <div className="flex items-center justify-between gap-4">
        <p className="text-sm text-foreground">
          {isUrgent ? (
            <>
              <span className="font-medium">Your trial ends today!</span>{" "}
              Upgrade now to keep all your features.
            </>
          ) : (
            <>
              <span className="font-medium">
                {trial.days_remaining} days left in your trial.
              </span>{" "}
              Upgrade to keep all features when it ends.
            </>
          )}
        </p>
        <div className="flex items-center gap-3">
          <Link
            href={`/settings/subscription?upgrade_trigger=trial_${isUrgent ? "urgent" : "reminder"}`}
            className="whitespace-nowrap rounded-md bg-foreground px-3 py-1.5 text-xs font-medium text-background hover:bg-foreground/90"
          >
            View plans
          </Link>
          <button
            type="button"
            onClick={() => setDismissed(true)}
            className="text-foreground-secondary hover:text-foreground"
            aria-label="Dismiss banner"
          >
            <X className="h-4 w-4" aria-hidden="true" />
          </button>
        </div>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Mount TrialExpiryBanner in AdminShell header**

Modify `apps/admin/components/shell/AdminShell.tsx` — add the banner inside the `<SidebarInset>`, immediately before the `<header>` element:

```tsx
// Add import at top:
import { TrialExpiryBanner } from "@/components/plangate/TrialExpiryBanner";

// Inside AdminShellFrame, inside <SidebarInset>, before <header>:
<SidebarInset className="relative bg-background">
  <TrialExpiryBanner />
  <header className="sticky top-0 z-30 border-b border-border-subtle bg-background">
    {/* ... existing header content ... */}
  </header>
  {/* ... existing main content ... */}
</SidebarInset>
```

### GREEN

- [ ] **Step 3: Verify TypeScript compiles**

```bash
cd apps/admin && npx tsc --noEmit
```

**Commit:** `feat(admin): add TrialExpiryBanner with 7-day, 1-day, and post-downgrade variants`

---

## Task 9: Plan comparison page enhancement

**Files:**
- Modify: `apps/admin/app/settings/subscription/page.tsx` (or create if S3 used `SubscriptionClient.tsx`)

- [ ] **Step 1: Add feature comparison grid**

The S3 subscription page shows current plan + upgrade/manage buttons. Enhance it with a visual feature comparison grid showing all tiers.

Add a new section below the current plan card in the subscription page:

```tsx
"use client";

import { Check, Minus } from "lucide-react";
import type { PlanTier, Feature } from "@/lib/api/subscription-api";

interface FeatureRow {
  label: string;
  feature: Feature;
  type: "boolean" | "numeric";
}

const featureRows: FeatureRow[] = [
  { label: "Products", feature: "products", type: "numeric" },
  { label: "Categories", feature: "categories", type: "numeric" },
  { label: "Staff members", feature: "staff", type: "numeric" },
  { label: "Stores", feature: "stores", type: "numeric" },
  { label: "Orders/month", feature: "orders_per_month", type: "numeric" },
  { label: "Active coupons", feature: "active_coupons", type: "numeric" },
  { label: "Campaigns/month", feature: "campaigns_per_month", type: "numeric" },
  { label: "Full color palette & fonts", feature: "full_color_palette", type: "boolean" },
  { label: "Announcement bar", feature: "announcement_bar", type: "boolean" },
  { label: 'Remove "Powered by mark8ly"', feature: "remove_powered_by", type: "boolean" },
  { label: "Custom CSS", feature: "custom_css", type: "boolean" },
  { label: "Custom domain", feature: "custom_domain", type: "boolean" },
  { label: "Returns & refunds", feature: "returns", type: "boolean" },
  { label: "Gift cards", feature: "gift_cards", type: "boolean" },
  { label: "Loyalty program", feature: "loyalty", type: "boolean" },
  { label: "Product reviews", feature: "reviews", type: "boolean" },
  { label: "Support tickets", feature: "support_tickets", type: "boolean" },
  { label: "Audit logs", feature: "audit_logs", type: "boolean" },
  { label: "CSV import/export", feature: "csv_import_export", type: "boolean" },
  { label: "Mobile app", feature: "mobile_app", type: "boolean" },
];

const planColumns: { plan: PlanTier; label: string; price: string }[] = [
  { plan: "free", label: "Free", price: "$0" },
  { plan: "starter", label: "Starter", price: "$9.99/mo" },
  { plan: "pro", label: "Pro", price: "$29.99/mo" },
  { plan: "enterprise", label: "Enterprise", price: "$99.99/mo" },
];

// Static limits per plan for display — mirrors Go limitsTable.
const displayLimits: Record<PlanTier, Record<string, number>> = {
  free: { products: 25, categories: 5, staff: 1, stores: 1, orders_per_month: 50, active_coupons: 5, campaigns_per_month: 0, full_color_palette: 0, announcement_bar: 0, remove_powered_by: 0, custom_css: 0, custom_domain: 0, returns: 0, label_tracking: 0, gift_cards: 0, loyalty: 0, reviews: 0, support_tickets: 0, priority_support: 0, audit_logs: 0, mobile_app: 0, csv_import_export: 0, vendor_onboarding: 0 },
  starter: { products: 500, categories: 25, staff: 3, stores: 1, orders_per_month: 500, active_coupons: 50, campaigns_per_month: 5, full_color_palette: 1, announcement_bar: 1, remove_powered_by: 0, custom_css: 0, custom_domain: 0, returns: 1, label_tracking: 1, gift_cards: 1, loyalty: 0, reviews: 1, support_tickets: 1, priority_support: 0, audit_logs: 0, mobile_app: 0, csv_import_export: 1, vendor_onboarding: 0 },
  pro: { products: -1, categories: -1, staff: 10, stores: 3, orders_per_month: -1, active_coupons: -1, campaigns_per_month: 50, full_color_palette: 1, announcement_bar: 1, remove_powered_by: 1, custom_css: 0, custom_domain: 1, returns: 1, label_tracking: 1, gift_cards: 1, loyalty: 1, reviews: 1, support_tickets: 1, priority_support: 0, audit_logs: 1, mobile_app: 0, csv_import_export: 1, vendor_onboarding: 0 },
  enterprise: { products: -1, categories: -1, staff: -1, stores: 10, orders_per_month: -1, active_coupons: -1, campaigns_per_month: -1, full_color_palette: 1, announcement_bar: 1, remove_powered_by: 1, custom_css: 1, custom_domain: 1, returns: 1, label_tracking: 1, gift_cards: 1, loyalty: 1, reviews: 1, support_tickets: 1, priority_support: 1, audit_logs: 1, mobile_app: 1, csv_import_export: 1, vendor_onboarding: 0 },
  marketplace: { products: -1, categories: -1, staff: -1, stores: 10, orders_per_month: -1, active_coupons: -1, campaigns_per_month: -1, full_color_palette: 1, announcement_bar: 1, remove_powered_by: 1, custom_css: 1, custom_domain: 1, returns: 1, label_tracking: 1, gift_cards: 1, loyalty: 1, reviews: 1, support_tickets: 1, priority_support: 1, audit_logs: 1, mobile_app: 1, csv_import_export: 1, vendor_onboarding: 1 },
};

function formatCellValue(value: number, type: "boolean" | "numeric"): React.ReactNode {
  if (value === 0) {
    return <Minus className="mx-auto h-4 w-4 text-foreground-tertiary" aria-label="Not available" />;
  }
  if (type === "boolean") {
    return <Check className="mx-auto h-4 w-4 text-moss-700" aria-label="Included" />;
  }
  if (value === -1) {
    return <span className="text-sm font-medium text-foreground">Unlimited</span>;
  }
  return <span className="text-sm text-foreground">{value.toLocaleString()}</span>;
}

interface PlanComparisonGridProps {
  currentPlan: PlanTier;
}

export function PlanComparisonGrid({ currentPlan }: PlanComparisonGridProps) {
  return (
    <section className="mt-12">
      <h2 className="font-serif text-xl font-medium text-foreground">
        Compare plans
      </h2>
      <p className="mt-1 text-sm text-foreground-secondary">
        0% transaction fees on every plan. Your revenue is yours.
      </p>
      <div className="mt-6 overflow-x-auto">
        <table className="w-full min-w-[640px] text-left">
          <thead>
            <tr className="border-b border-border-subtle">
              <th className="py-3 pr-4 text-xs font-semibold uppercase tracking-wider text-foreground-tertiary">
                Feature
              </th>
              {planColumns.map((col) => (
                <th
                  key={col.plan}
                  className={`px-4 py-3 text-center text-xs font-semibold uppercase tracking-wider ${
                    col.plan === currentPlan
                      ? "bg-moss-700/5 text-moss-700"
                      : "text-foreground-tertiary"
                  }`}
                >
                  <div>{col.label}</div>
                  <div className="mt-0.5 text-[10px] font-normal normal-case tracking-normal">
                    {col.price}
                  </div>
                  {col.plan === currentPlan && (
                    <div className="mt-1 text-[10px] font-medium normal-case tracking-normal">
                      Current
                    </div>
                  )}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {featureRows.map((row) => (
              <tr key={row.feature} className="border-b border-border-subtle/50">
                <td className="py-3 pr-4 text-sm text-foreground">
                  {row.label}
                </td>
                {planColumns.map((col) => (
                  <td
                    key={col.plan}
                    className={`px-4 py-3 text-center ${
                      col.plan === currentPlan ? "bg-moss-700/5" : ""
                    }`}
                  >
                    {formatCellValue(
                      displayLimits[col.plan]?.[row.feature] ?? 0,
                      row.type,
                    )}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
```

- [ ] **Step 2: Import PlanComparisonGrid in the subscription page**

In the existing subscription page component, add:

```tsx
import { PlanComparisonGrid } from "@/components/settings/PlanComparisonGrid";

// Below the existing plan card / upgrade buttons:
<PlanComparisonGrid currentPlan={subscription.plan} />
```

### GREEN

- [ ] **Step 3: Verify build**

```bash
cd apps/admin && npx tsc --noEmit
```

**Commit:** `feat(admin): add plan comparison grid to subscription settings page`

---

## Task 10: GMV nudge on Starter dashboard

**Files:**
- Create: `apps/admin/components/plangate/GmvNudge.tsx`
- Modify: `apps/admin/app/dashboard/page.tsx`

- [ ] **Step 1: Create GmvNudge component**

Create `apps/admin/components/plangate/GmvNudge.tsx`:

```tsx
"use client";

import Link from "next/link";
import { TrendingUp, ArrowUpRight } from "lucide-react";
import { useSubscription } from "@/lib/hooks/use-subscription";

interface GmvNudgeProps {
  /** Current month's GMV in the store's currency (e.g., 5400.00). */
  monthlyGmv: number;
  /** Store's currency code for formatting. */
  currencyCode: string;
}

/** GMV threshold for showing the nudge (spec §2.4: $5K/month). */
const GMV_THRESHOLD = 5000;

/**
 * GmvNudge — shown on the dashboard when a Starter merchant crosses $5K GMV.
 * Not a hard cap — just a visible nudge per spec §2.4.
 */
export function GmvNudge({ monthlyGmv, currencyCode }: GmvNudgeProps) {
  const { plan } = useSubscription();

  // Only show for Starter plan merchants above threshold.
  if (plan !== "starter" || monthlyGmv < GMV_THRESHOLD) {
    return null;
  }

  const formattedGmv = new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: currencyCode,
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(monthlyGmv);

  return (
    <div className="rounded-md border border-moss-700/20 bg-moss-700/5 p-5">
      <div className="flex items-start gap-4">
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-moss-700/10">
          <TrendingUp className="h-5 w-5 text-moss-700" aria-hidden="true" />
        </div>
        <div className="flex-1">
          <p className="font-serif text-lg font-medium text-foreground">
            You processed {formattedGmv} this month
          </p>
          <p className="mt-1 text-sm text-foreground-secondary">
            Pro merchants get unlimited orders, advanced analytics, and loyalty
            programs. Upgrade to unlock your store's full potential.
          </p>
          <Link
            href="/settings/subscription?upgrade_trigger=gmv_nudge&from_plan=starter"
            className="mt-3 inline-flex items-center gap-1.5 text-sm font-medium text-moss-700 hover:text-moss-700/80"
          >
            Explore Pro
            <ArrowUpRight className="h-3.5 w-3.5" aria-hidden="true" />
          </Link>
        </div>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Wire GmvNudge into dashboard page**

Modify `apps/admin/app/dashboard/page.tsx` — fetch the current month's order total and pass it to GmvNudge. The exact integration depends on how the dashboard currently fetches data. Add:

```tsx
import { GmvNudge } from "@/components/plangate/GmvNudge";

// Inside the dashboard render, above or below the analytics overview:
<GmvNudge
  monthlyGmv={dashboardData.monthlyGmv ?? 0}
  currencyCode={store.currency_code}
/>
```

If the dashboard does not yet have a `monthlyGmv` field, add a new API endpoint or compute it from the existing orders API:

```go
// In the admin handler or a dedicated dashboard handler:
// GET /admin/stores/:storeId/dashboard/gmv
// Returns: { "monthly_gmv": 5400.50 }
func (h *DashboardHandler) GetMonthlyGMV(c *gin.Context) {
    storeID := c.Param("storeId")
    now := time.Now()
    startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

    var total float64
    err := h.db.WithContext(c.Request.Context()).
        Model(&order.Order{}).
        Where("store_id = ? AND status NOT IN ('cancelled','refunded') AND created_at >= ?", storeID, startOfMonth).
        Select("COALESCE(SUM(total), 0)").
        Scan(&total).Error
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "query_failed"})
        return
    }
    c.JSON(http.StatusOK, gin.H{"data": gin.H{"monthly_gmv": total}})
}
```

Wire this endpoint in `routes.go`:

```go
// Inside storeRoute:
storeRoute.GET("/dashboard/gmv",
    deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff),
    deps.DashboardHandler.GetMonthlyGMV)
```

### GREEN

- [ ] **Step 3: Verify build**

```bash
cd services/marketplace-api && go build ./...
cd apps/admin && npx tsc --noEmit
```

**Commit:** `feat(admin): add GMV nudge banner on Starter dashboard at $5K threshold`

---

## Task 11: Upgrade trigger event tracking

**Files:**
- Create: `apps/admin/lib/analytics/track-upgrade-trigger.ts`
- Modify: `apps/admin/components/plangate/UpgradePrompt.tsx`
- Modify: `apps/admin/components/plangate/GmvNudge.tsx`
- Modify: `apps/admin/components/plangate/TrialExpiryBanner.tsx`

- [ ] **Step 1: Create tracking utility**

Create `apps/admin/lib/analytics/track-upgrade-trigger.ts`:

```typescript
import type { Feature } from "@/lib/api/subscription-api";

/**
 * Upgrade trigger types that identify what drove the merchant to the
 * upgrade page. Spec §2.4 and §7.3 require instrumenting these.
 */
export type UpgradeTrigger =
  | `feature_gate:${Feature}`
  | "limit_reached:products"
  | "limit_reached:categories"
  | "limit_reached:staff"
  | "limit_reached:coupons"
  | "limit_reached:orders"
  | "gmv_nudge"
  | "trial_reminder"
  | "trial_urgent"
  | "trial_expired"
  | "plan_comparison"
  | "manual";

/**
 * Track an upgrade trigger event. Currently logs to the OpenPanel
 * analytics client (already wired in admin via @openpanel/nextjs).
 * Falls back to console in dev if OpenPanel is not available.
 */
export function trackUpgradeTrigger(
  trigger: UpgradeTrigger,
  metadata?: Record<string, string>,
): void {
  // OpenPanel is initialized globally via the provider in layout.tsx.
  // Access it via the window object or import if available.
  if (typeof window !== "undefined" && "op" in window) {
    const op = (window as Record<string, unknown>).op as (
      event: string,
      props: Record<string, string>,
    ) => void;
    op("upgrade_trigger", {
      trigger,
      ...metadata,
    });
  }
}
```

- [ ] **Step 2: Add tracking to UpgradePrompt**

Modify `apps/admin/components/plangate/UpgradePrompt.tsx` — add an onClick handler to the link:

```tsx
import { trackUpgradeTrigger } from "@/lib/analytics/track-upgrade-trigger";

// On the Link in UpgradePrompt:
<Link
  href={`/settings/subscription?upgrade_trigger=${feature}&from_plan=${plan}`}
  onClick={() =>
    trackUpgradeTrigger(`feature_gate:${feature}`, { from_plan: plan })
  }
  className="..."
>
```

- [ ] **Step 3: Add tracking to GmvNudge**

```tsx
import { trackUpgradeTrigger } from "@/lib/analytics/track-upgrade-trigger";

// On the Link in GmvNudge:
onClick={() => trackUpgradeTrigger("gmv_nudge", { from_plan: "starter", gmv: String(monthlyGmv) })}
```

- [ ] **Step 4: Add tracking to TrialExpiryBanner**

```tsx
import { trackUpgradeTrigger } from "@/lib/analytics/track-upgrade-trigger";

// On each "Upgrade now" / "View plans" link, use the appropriate trigger:
// For expired: trackUpgradeTrigger("trial_expired")
// For urgent (<=1 day): trackUpgradeTrigger("trial_urgent")
// For reminder (<=7 days): trackUpgradeTrigger("trial_reminder")
```

### GREEN

- [ ] **Step 5: Verify TypeScript compiles**

```bash
cd apps/admin && npx tsc --noEmit
```

**Commit:** `feat(admin): add upgrade trigger event tracking across all gate surfaces`

---

## Task 12: Wire SubscriptionProvider into app layout

**Files:**
- Modify: `apps/admin/app/layout.tsx` (or the layout that wraps authenticated pages)

- [ ] **Step 1: Fetch subscription in layout and provide context**

The SubscriptionProvider needs to wrap all admin pages so `useSubscription()` works everywhere. In the root authenticated layout:

```tsx
import { SubscriptionProvider } from "@/lib/hooks/use-subscription";
import { fetchSubscription } from "@/lib/api/subscription-api";

// In the server component that wraps AdminShell:
const subscription = await fetchSubscription(storeId, session);

// Wrap the children:
<SubscriptionProvider subscription={subscription}>
  <AdminShell {...shellProps}>
    {children}
  </AdminShell>
</SubscriptionProvider>
```

The exact file depends on how the admin layout is structured. Look for the component that creates `<AdminShell>` and has access to `storeId` + `session`. Wrap that component's children with `<SubscriptionProvider>`.

### GREEN

- [ ] **Step 2: Verify build and dev server**

```bash
cd apps/admin && npx tsc --noEmit
cd apps/admin && npm run dev &
# Navigate to http://localhost:3001/dashboard and verify no console errors
```

**Commit:** `feat(admin): wire SubscriptionProvider into authenticated layout`

---

## Task 13: Build verification

- [ ] **Step 1: Go — full test suite**

```bash
cd services/marketplace-api && go test ./... -count=1 -race
```

Expected: all tests pass, including new plangate tests.

- [ ] **Step 2: Go — build**

```bash
cd services/marketplace-api && go build -o /dev/null ./cmd/marketplace-api/
```

- [ ] **Step 3: Admin — TypeScript check**

```bash
cd apps/admin && npx tsc --noEmit
```

- [ ] **Step 4: Admin — lint**

```bash
cd apps/admin && npx next lint
```

- [ ] **Step 5: Admin — component tests**

```bash
cd apps/admin && npx vitest run
```

- [ ] **Step 6: Manual smoke test**

1. Start dev: `make dev` (marketplace-api + admin)
2. Navigate to admin dashboard
3. Verify TrialExpiryBanner does NOT show for active subscriptions
4. Verify PlanGate hides loyalty page for free plan
5. Attempt to create product #26 on free plan — expect 403 with upgrade message
6. Navigate to /settings/subscription — verify comparison grid renders
7. Check browser console for no errors

No commit for verification — this validates the preceding commits.

---

## Summary of commits

| # | Message | Files |
|---|---------|-------|
| 1 | `feat(plangate): add plan/feature types with limits table and unit tests` | `internal/plangate/plans.go`, `features.go`, `limits.go`, `limits_test.go` |
| 2 | `feat(plangate): add RequirePlan, RequireFeature, EnforceLimit Gin middleware` | `internal/plangate/middleware.go`, `middleware_test.go` |
| 3 | `feat(plangate): add soft downgrade logic with grace period and trial state` | `internal/plangate/downgrade.go`, `downgrade_test.go`, `subscription/models.go`, `subscription/repository.go`, migration |
| 4 | `feat(plangate): wire plan-gate middleware into admin routes` | `handlers/admin/routes.go`, `cmd/main.go` |
| 5 | `feat(plangate): enrich subscription API with limits, trial, and downgrade state` | `subscription/handler.go`, `subscription-api.ts` |
| 6 | `feat(admin): add useSubscription hook and SubscriptionProvider context` | `use-subscription.ts`, `use-subscription.test.ts` |
| 7 | `feat(admin): add PlanGate and UpgradePrompt components` | `PlanGate.tsx`, `UpgradePrompt.tsx` + tests |
| 8 | `feat(admin): add TrialExpiryBanner with 7-day, 1-day, and post-downgrade variants` | `TrialExpiryBanner.tsx`, `AdminShell.tsx` |
| 9 | `feat(admin): add plan comparison grid to subscription settings page` | `PlanComparisonGrid.tsx`, subscription page |
| 10 | `feat(admin): add GMV nudge banner on Starter dashboard at $5K threshold` | `GmvNudge.tsx`, dashboard page, GMV endpoint |
| 11 | `feat(admin): add upgrade trigger event tracking across all gate surfaces` | `track-upgrade-trigger.ts`, UpgradePrompt, GmvNudge, TrialExpiryBanner |
| 12 | `feat(admin): wire SubscriptionProvider into authenticated layout` | layout file |
