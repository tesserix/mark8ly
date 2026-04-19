// Package plangate implements subscription-tier feature gating for B2.
// The feature matrix itself lives in matrix.go; this file provides the
// plan resolver and Gin middleware that consume it.
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
			minPlan := MinPlanForFeature(feature)
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

// ResolveByTenant returns the highest plan across any active store owned by
// tenantID. It uses a direct DB query to find the first matching row so that
// tenant-scoped routes (e.g. /admin/tenants/:tenantId/sso/config) don't need a
// storeId in the URL. Falls back to PlanTrial on any error (fail-closed).
func (r *PlanResolver) ResolveByTenant(ctx context.Context, tenantID uuid.UUID) Plan {
	var plan subscription.SubscriptionPlan
	err := r.db.WithContext(ctx).
		Table("store_subscriptions").
		Select("plan").
		Where("tenant_id = ?", tenantID).
		Order("CASE plan "+
			"WHEN 'marketplace' THEN 4 "+
			"WHEN 'pro' THEN 3 "+
			"WHEN 'studio' THEN 2 "+
			"WHEN 'starter' THEN 1 "+
			"ELSE 0 END DESC").
		Limit(1).
		Scan(&plan).Error
	if err != nil || plan == "" {
		return subscription.PlanTrial
	}
	if _, ok := planOrder[plan]; !ok {
		return subscription.PlanTrial
	}
	return plan
}

// RequireFeatureByTenant is like RequireFeature but resolves the plan from any
// store belonging to the authenticated tenant rather than a specific :storeId.
// Use this for tenant-scoped admin routes (e.g. SSO config) where no single
// store owns the feature.
//
// It reads the tenant_id set by the upstream auth middleware; it does NOT read
// the :tenantId path param — callers that want to enforce path ownership must
// add a separate check in the handler.
func RequireFeatureByTenant(resolver *PlanResolver, feature Feature, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID, err := uuid.Parse(c.GetString("tenant_id"))
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "message": "missing tenant_id"})
			c.Abort()
			return
		}

		plan := resolver.ResolveByTenant(c.Request.Context(), tenantID)
		if !IsAllowed(plan, feature) {
			minPlan := MinPlanForFeature(feature)
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

		c.Set("tenant_plan", string(plan))
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
