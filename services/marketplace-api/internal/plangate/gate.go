// Package plangate implements subscription-tier feature gating for B2.
// It defines the feature matrix, provides boolean and numeric limit
// checks, and exposes Gin middleware for route-level enforcement.
package plangate

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// Plan is a subscription tier. It aliases subscription.SubscriptionPlan so the
// canonical plan identity lives in one place (the data-model package). The
// type alias — not a defined type — keeps conversions transparent for
// callers that pass values across the package boundary.
type Plan = subscription.SubscriptionPlan

// planOrder defines the hierarchy for >= comparisons.
// Marketplace is a hidden internal tier; it ranks above Pro so platform-owned
// stores implicitly pass every RequirePlan check.
var planOrder = map[Plan]int{
	subscription.PlanTrial:       0,
	subscription.PlanStarter:     1,
	subscription.PlanStudio:      2,
	subscription.PlanPro:         3,
	subscription.PlanMarketplace: 4,
}

// Feature is an enumerated feature key.
type Feature string

const (
	// Store limits.
	FeatureProducts   Feature = "products"
	FeatureCategories Feature = "categories"
	FeatureStaff      Feature = "staff"
	FeatureStores     Feature = "stores"

	// Orders.
	FeatureOrdersPerMonth Feature = "orders_per_month"
	FeatureReturns        Feature = "returns"

	// Branding.
	FeatureFullColorPalette Feature = "full_color_palette"
	FeatureAnnouncementBar  Feature = "announcement_bar"
	FeatureRemovePoweredBy  Feature = "remove_powered_by"
	FeatureCustomCSS        Feature = "custom_css"
	FeatureCustomDomain     Feature = "custom_domain"

	// Marketing.
	FeatureActiveCoupons     Feature = "active_coupons"
	FeatureGiftCards         Feature = "gift_cards"
	FeatureLoyalty           Feature = "loyalty"
	FeatureCampaignsPerMonth Feature = "campaigns_per_month"

	// Customers.
	FeatureReviews Feature = "reviews"

	// Support.
	FeatureTickets         Feature = "tickets"
	FeaturePrioritySupport Feature = "priority_support"

	// Analytics.
	FeatureAuditLogs Feature = "audit_logs"

	// Apps.
	FeatureMobileApp Feature = "mobile_app"

	// Import/Export.
	FeatureCSVImportExport Feature = "csv_import_export"

	// Shipping.
	FeatureShippingLabels Feature = "shipping_labels"
)

// Unlimited is the sentinel for "no numeric limit".
const Unlimited = -1

// featureMatrix defines the complete plan × feature matrix.
// Boolean features: 0 = disabled, 1 = enabled, -1 = unlimited.
// Numeric features: the limit value, -1 = unlimited.
//
// NOTE: The numbers below are v1 carry-overs rethreaded onto the v2.3 tier
// names (trial / starter / studio / pro / marketplace). They are intentionally
// placeholders — the real v2.3 feature matrix rewrite lands in P3 Task 2.
// Don't tune individual cells here; wait for the P3 pass.
var featureMatrix = map[Feature]map[Plan]int{
	FeatureProducts:          {subscription.PlanTrial: 25, subscription.PlanStarter: 500, subscription.PlanStudio: Unlimited, subscription.PlanPro: Unlimited, subscription.PlanMarketplace: Unlimited},
	FeatureCategories:        {subscription.PlanTrial: 5, subscription.PlanStarter: 25, subscription.PlanStudio: Unlimited, subscription.PlanPro: Unlimited, subscription.PlanMarketplace: Unlimited},
	FeatureStaff:             {subscription.PlanTrial: 1, subscription.PlanStarter: 3, subscription.PlanStudio: 10, subscription.PlanPro: 10, subscription.PlanMarketplace: Unlimited},
	FeatureStores:            {subscription.PlanTrial: 1, subscription.PlanStarter: 1, subscription.PlanStudio: 3, subscription.PlanPro: 3, subscription.PlanMarketplace: 10},
	FeatureOrdersPerMonth:    {subscription.PlanTrial: 50, subscription.PlanStarter: 500, subscription.PlanStudio: Unlimited, subscription.PlanPro: Unlimited, subscription.PlanMarketplace: Unlimited},
	FeatureReturns:           {subscription.PlanTrial: 0, subscription.PlanStarter: 1, subscription.PlanStudio: 1, subscription.PlanPro: 1, subscription.PlanMarketplace: 1},
	FeatureFullColorPalette:  {subscription.PlanTrial: 0, subscription.PlanStarter: 1, subscription.PlanStudio: 1, subscription.PlanPro: 1, subscription.PlanMarketplace: 1},
	FeatureAnnouncementBar:   {subscription.PlanTrial: 0, subscription.PlanStarter: 1, subscription.PlanStudio: 1, subscription.PlanPro: 1, subscription.PlanMarketplace: 1},
	FeatureRemovePoweredBy:   {subscription.PlanTrial: 0, subscription.PlanStarter: 0, subscription.PlanStudio: 1, subscription.PlanPro: 1, subscription.PlanMarketplace: 1},
	FeatureCustomCSS:         {subscription.PlanTrial: 0, subscription.PlanStarter: 0, subscription.PlanStudio: 0, subscription.PlanPro: 1, subscription.PlanMarketplace: 1},
	FeatureCustomDomain:      {subscription.PlanTrial: 0, subscription.PlanStarter: 0, subscription.PlanStudio: 1, subscription.PlanPro: 1, subscription.PlanMarketplace: 1},
	FeatureActiveCoupons:     {subscription.PlanTrial: 5, subscription.PlanStarter: 50, subscription.PlanStudio: Unlimited, subscription.PlanPro: Unlimited, subscription.PlanMarketplace: Unlimited},
	FeatureGiftCards:         {subscription.PlanTrial: 0, subscription.PlanStarter: 1, subscription.PlanStudio: 1, subscription.PlanPro: 1, subscription.PlanMarketplace: 1},
	FeatureLoyalty:           {subscription.PlanTrial: 0, subscription.PlanStarter: 0, subscription.PlanStudio: 1, subscription.PlanPro: 1, subscription.PlanMarketplace: 1},
	FeatureCampaignsPerMonth: {subscription.PlanTrial: 0, subscription.PlanStarter: 5, subscription.PlanStudio: 50, subscription.PlanPro: Unlimited, subscription.PlanMarketplace: Unlimited},
	FeatureReviews:           {subscription.PlanTrial: 0, subscription.PlanStarter: 1, subscription.PlanStudio: 1, subscription.PlanPro: 1, subscription.PlanMarketplace: 1},
	FeatureTickets:           {subscription.PlanTrial: 0, subscription.PlanStarter: 1, subscription.PlanStudio: 1, subscription.PlanPro: 1, subscription.PlanMarketplace: 1},
	FeaturePrioritySupport:   {subscription.PlanTrial: 0, subscription.PlanStarter: 0, subscription.PlanStudio: 0, subscription.PlanPro: 1, subscription.PlanMarketplace: 1},
	FeatureAuditLogs:         {subscription.PlanTrial: 0, subscription.PlanStarter: 0, subscription.PlanStudio: 1, subscription.PlanPro: 1, subscription.PlanMarketplace: 1},
	FeatureMobileApp:         {subscription.PlanTrial: 0, subscription.PlanStarter: 0, subscription.PlanStudio: 0, subscription.PlanPro: 1, subscription.PlanMarketplace: 1},
	FeatureCSVImportExport:   {subscription.PlanTrial: 0, subscription.PlanStarter: 1, subscription.PlanStudio: 1, subscription.PlanPro: 1, subscription.PlanMarketplace: 1},
	FeatureShippingLabels:    {subscription.PlanTrial: 0, subscription.PlanStarter: 1, subscription.PlanStudio: 1, subscription.PlanPro: 1, subscription.PlanMarketplace: 1},
}

// IsAllowed checks if a boolean feature is available on the given plan.
func IsAllowed(plan Plan, feature Feature) bool {
	limits, ok := featureMatrix[feature]
	if !ok {
		return false
	}
	v, ok := limits[plan]
	if !ok {
		return false
	}
	return v != 0
}

// GetLimit returns the numeric limit for a feature on a plan.
// Returns -1 (Unlimited) if the feature has no cap, 0 if disabled.
func GetLimit(plan Plan, feature Feature) int {
	limits, ok := featureMatrix[feature]
	if !ok {
		return 0
	}
	v, ok := limits[plan]
	if !ok {
		return 0
	}
	return v
}

// PlanAtLeast returns true if plan >= minPlan in the tier hierarchy.
func PlanAtLeast(plan, minPlan Plan) bool {
	return planOrder[plan] >= planOrder[minPlan]
}

// PlanResolver looks up the current plan for a store.
type PlanResolver struct {
	db   *gorm.DB
	repo subscription.Repository
}

// NewPlanResolver constructs a PlanResolver.
func NewPlanResolver(db *gorm.DB, repo subscription.Repository) *PlanResolver {
	return &PlanResolver{db: db, repo: repo}
}

// Resolve returns the current plan for a (tenant, store) pair, defaulting
// to trial. tenantID is mandatory — passing a mismatched tenant falls back
// to trial (fail-closed) rather than leaking another tenant's plan.
func (r *PlanResolver) Resolve(ctx context.Context, tenantID, storeID uuid.UUID) Plan {
	sub, err := r.repo.GetByStoreID(ctx, r.db, tenantID, storeID)
	if err != nil {
		return subscription.PlanTrial
	}
	p := sub.Plan
	if _, ok := planOrder[p]; !ok {
		return subscription.PlanTrial
	}
	return p
}

// --- Gin Middleware ---

// RequireFeature returns middleware that checks a boolean feature is enabled
// on the store's current plan. Responds 403 with upgrade message if not.
// Requires the upstream auth middleware to have set "tenant_id" on the Gin
// context — otherwise the request is rejected 401 rather than falling back
// to trial-plan behavior (fail-closed).
func RequireFeature(resolver *PlanResolver, feature Feature, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID, err := uuid.Parse(c.GetString("tenant_id"))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "message": "missing tenant_id"})
			c.Abort()
			return
		}
		storeID, err := uuid.Parse(c.Param("storeId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "invalid store id"})
			c.Abort()
			return
		}

		plan := resolver.Resolve(c.Request.Context(), tenantID, storeID)
		if !IsAllowed(plan, feature) {
			minPlan := minPlanForFeature(feature)
			c.JSON(http.StatusForbidden, gin.H{
				"error":    "plan_required",
				"message":  fmt.Sprintf("This feature requires the %s plan or higher", minPlan),
				"required": string(minPlan),
				"current":  string(plan),
				"feature":  string(feature),
			})
			c.Abort()
			return
		}

		c.Set("store_plan", string(plan))
		c.Next()
	}
}

// RequirePlan returns middleware that checks the store is on at least the
// given minimum plan. Requires upstream auth middleware to have set
// "tenant_id" on the Gin context.
func RequirePlan(resolver *PlanResolver, minPlan Plan, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID, err := uuid.Parse(c.GetString("tenant_id"))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "message": "missing tenant_id"})
			c.Abort()
			return
		}
		storeID, err := uuid.Parse(c.Param("storeId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "invalid store id"})
			c.Abort()
			return
		}

		plan := resolver.Resolve(c.Request.Context(), tenantID, storeID)
		if !PlanAtLeast(plan, minPlan) {
			c.JSON(http.StatusForbidden, gin.H{
				"error":    "plan_required",
				"message":  fmt.Sprintf("This feature requires the %s plan or higher", minPlan),
				"required": string(minPlan),
				"current":  string(plan),
			})
			c.Abort()
			return
		}

		c.Set("store_plan", string(plan))
		c.Next()
	}
}

// minPlanForFeature returns the lowest plan that has access to a feature.
func minPlanForFeature(feature Feature) Plan {
	limits, ok := featureMatrix[feature]
	if !ok {
		return subscription.PlanPro
	}
	for _, p := range []Plan{
		subscription.PlanTrial,
		subscription.PlanStarter,
		subscription.PlanStudio,
		subscription.PlanPro,
		subscription.PlanMarketplace,
	} {
		if v, ok := limits[p]; ok && v != 0 {
			return p
		}
	}
	return subscription.PlanPro
}

// AllFeatureLimits returns the full feature matrix for a given plan,
// suitable for returning to the frontend via the subscription endpoint.
func AllFeatureLimits(plan Plan) map[string]int {
	result := make(map[string]int, len(featureMatrix))
	for feature, limits := range featureMatrix {
		if v, ok := limits[plan]; ok {
			result[string(feature)] = v
		} else {
			result[string(feature)] = 0
		}
	}
	return result
}
