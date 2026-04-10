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

// Plan is a subscription tier.
type Plan string

const (
	PlanFree       Plan = "free"
	PlanStarter    Plan = "starter"
	PlanPro        Plan = "pro"
	PlanEnterprise Plan = "enterprise"
	PlanMarketplace Plan = "marketplace"
)

// planOrder defines the hierarchy for >= comparisons.
var planOrder = map[Plan]int{
	PlanFree:        0,
	PlanStarter:     1,
	PlanPro:         2,
	PlanEnterprise:  3,
	PlanMarketplace: 4,
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
	FeatureActiveCoupons    Feature = "active_coupons"
	FeatureGiftCards        Feature = "gift_cards"
	FeatureLoyalty          Feature = "loyalty"
	FeatureCampaignsPerMonth Feature = "campaigns_per_month"

	// Customers.
	FeatureReviews Feature = "reviews"

	// Support.
	FeatureTickets        Feature = "tickets"
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
var featureMatrix = map[Feature]map[Plan]int{
	FeatureProducts:          {PlanFree: 25, PlanStarter: 500, PlanPro: Unlimited, PlanEnterprise: Unlimited, PlanMarketplace: Unlimited},
	FeatureCategories:        {PlanFree: 5, PlanStarter: 25, PlanPro: Unlimited, PlanEnterprise: Unlimited, PlanMarketplace: Unlimited},
	FeatureStaff:             {PlanFree: 1, PlanStarter: 3, PlanPro: 10, PlanEnterprise: Unlimited, PlanMarketplace: Unlimited},
	FeatureStores:            {PlanFree: 1, PlanStarter: 1, PlanPro: 3, PlanEnterprise: 10, PlanMarketplace: 10},
	FeatureOrdersPerMonth:    {PlanFree: 50, PlanStarter: 500, PlanPro: Unlimited, PlanEnterprise: Unlimited, PlanMarketplace: Unlimited},
	FeatureReturns:           {PlanFree: 0, PlanStarter: 1, PlanPro: 1, PlanEnterprise: 1, PlanMarketplace: 1},
	FeatureFullColorPalette:  {PlanFree: 0, PlanStarter: 1, PlanPro: 1, PlanEnterprise: 1, PlanMarketplace: 1},
	FeatureAnnouncementBar:   {PlanFree: 0, PlanStarter: 1, PlanPro: 1, PlanEnterprise: 1, PlanMarketplace: 1},
	FeatureRemovePoweredBy:   {PlanFree: 0, PlanStarter: 0, PlanPro: 1, PlanEnterprise: 1, PlanMarketplace: 1},
	FeatureCustomCSS:         {PlanFree: 0, PlanStarter: 0, PlanPro: 0, PlanEnterprise: 1, PlanMarketplace: 1},
	FeatureCustomDomain:      {PlanFree: 0, PlanStarter: 0, PlanPro: 1, PlanEnterprise: 1, PlanMarketplace: 1},
	FeatureActiveCoupons:     {PlanFree: 5, PlanStarter: 50, PlanPro: Unlimited, PlanEnterprise: Unlimited, PlanMarketplace: Unlimited},
	FeatureGiftCards:         {PlanFree: 0, PlanStarter: 1, PlanPro: 1, PlanEnterprise: 1, PlanMarketplace: 1},
	FeatureLoyalty:           {PlanFree: 0, PlanStarter: 0, PlanPro: 1, PlanEnterprise: 1, PlanMarketplace: 1},
	FeatureCampaignsPerMonth: {PlanFree: 0, PlanStarter: 5, PlanPro: 50, PlanEnterprise: Unlimited, PlanMarketplace: Unlimited},
	FeatureReviews:           {PlanFree: 0, PlanStarter: 1, PlanPro: 1, PlanEnterprise: 1, PlanMarketplace: 1},
	FeatureTickets:           {PlanFree: 0, PlanStarter: 1, PlanPro: 1, PlanEnterprise: 1, PlanMarketplace: 1},
	FeaturePrioritySupport:   {PlanFree: 0, PlanStarter: 0, PlanPro: 0, PlanEnterprise: 1, PlanMarketplace: 1},
	FeatureAuditLogs:         {PlanFree: 0, PlanStarter: 0, PlanPro: 1, PlanEnterprise: 1, PlanMarketplace: 1},
	FeatureMobileApp:         {PlanFree: 0, PlanStarter: 0, PlanPro: 0, PlanEnterprise: 1, PlanMarketplace: 1},
	FeatureCSVImportExport:   {PlanFree: 0, PlanStarter: 1, PlanPro: 1, PlanEnterprise: 1, PlanMarketplace: 1},
	FeatureShippingLabels:    {PlanFree: 0, PlanStarter: 1, PlanPro: 1, PlanEnterprise: 1, PlanMarketplace: 1},
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

// Resolve returns the current plan for a store, defaulting to free.
func (r *PlanResolver) Resolve(ctx context.Context, storeID uuid.UUID) Plan {
	sub, err := r.repo.GetByStoreID(ctx, r.db, storeID)
	if err != nil {
		return PlanFree
	}
	p := Plan(sub.Plan)
	if _, ok := planOrder[p]; !ok {
		return PlanFree
	}
	return p
}

// --- Gin Middleware ---

// RequireFeature returns middleware that checks a boolean feature is enabled
// on the store's current plan. Responds 403 with upgrade message if not.
func RequireFeature(resolver *PlanResolver, feature Feature, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		storeID, err := uuid.Parse(c.Param("storeId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "invalid store id"})
			c.Abort()
			return
		}

		plan := resolver.Resolve(c.Request.Context(), storeID)
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
// given minimum plan.
func RequirePlan(resolver *PlanResolver, minPlan Plan, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		storeID, err := uuid.Parse(c.Param("storeId"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "invalid store id"})
			c.Abort()
			return
		}

		plan := resolver.Resolve(c.Request.Context(), storeID)
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
		return PlanEnterprise
	}
	for _, p := range []Plan{PlanFree, PlanStarter, PlanPro, PlanEnterprise, PlanMarketplace} {
		if v, ok := limits[p]; ok && v != 0 {
			return p
		}
	}
	return PlanEnterprise
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
