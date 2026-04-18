// Package admin — subscription_change_plan.go: HTTP handler for merchant-initiated
// plan changes (§4.4) and the read-only preflight check (§4.5).
package admin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/planchange"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// ChangePlanOrchestrator abstracts the two operations the handler needs.
// The concrete *planchange.Orchestrator satisfies this interface.
// Exported so tests in the admin_test package can declare fakes without
// importing the concrete type.
type ChangePlanOrchestrator interface {
	Execute(ctx context.Context, in planchange.Input) (planchange.Output, error)
	Preflight(ctx context.Context, in planchange.Input) (planchange.PreflightReport, error)
}

// ChangePlanOrchestratorForTest is an alias kept for test readability;
// it is the same interface as ChangePlanOrchestrator.
type ChangePlanOrchestratorForTest = ChangePlanOrchestrator

// ChangePlanHandler exposes merchant-initiated plan change + preflight endpoints.
type ChangePlanHandler struct {
	orch   ChangePlanOrchestrator
	logger *slog.Logger
}

// NewChangePlanHandler constructs a ChangePlanHandler. The orch parameter
// accepts the concrete *planchange.Orchestrator in production and a fake in tests.
func NewChangePlanHandler(orch ChangePlanOrchestrator, logger *slog.Logger) *ChangePlanHandler {
	return &ChangePlanHandler{orch: orch, logger: logger}
}

type changePlanRequest struct {
	TargetPlan        string `json:"target_plan" binding:"required"`
	TargetPeriod      string `json:"target_period" binding:"required"`
	RequestedCurrency string `json:"requested_currency,omitempty"`
	Reason            string `json:"reason,omitempty"`
}

type changePlanResponse struct {
	Result        string `json:"result"`
	EffectiveAt   string `json:"effective_at"` // RFC3339
	StripeUpdated bool   `json:"stripe_updated"`
}

// ChangePlan handles POST /admin/stores/:storeId/subscription/change-plan.
func (h *ChangePlanHandler) ChangePlan(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	tenantID, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("tenant_id", "invalid uuid"), h.logger)
		return
	}

	var req changePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	actor := "user:" + c.GetString("user_id")

	out, err := h.orch.Execute(c.Request.Context(), planchange.Input{
		TenantID:          tenantID,
		StoreID:           storeID,
		TargetPlan:        subscription.SubscriptionPlan(req.TargetPlan),
		TargetPeriod:      subscription.SubscriptionPeriod(req.TargetPeriod),
		RequestedCurrency: req.RequestedCurrency,
		Actor:             actor,
		Reason:            req.Reason,
		GinCtx:            c,
	})
	if err != nil {
		mapChangePlanErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, changePlanResponse{
		Result:        string(out.Result),
		EffectiveAt:   out.EffectiveAt.Format(time.RFC3339),
		StripeUpdated: out.StripeUpdated,
	})
}

// ChangePlanPreflight handles GET /admin/stores/:storeId/subscription/change-plan/preflight.
func (h *ChangePlanHandler) ChangePlanPreflight(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	tenantID, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("tenant_id", "invalid uuid"), h.logger)
		return
	}

	report, err := h.orch.Preflight(c.Request.Context(), planchange.Input{
		TenantID:          tenantID,
		StoreID:           storeID,
		TargetPlan:        subscription.SubscriptionPlan(c.Query("target_plan")),
		TargetPeriod:      subscription.SubscriptionPeriod(c.Query("target_period")),
		RequestedCurrency: c.Query("requested_currency"),
	})
	if err != nil {
		mapChangePlanErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, report)
}

// mapChangePlanErr maps domain errors from planchange.Execute / Preflight to
// HTTP status codes with stable error slugs. Non-mapped errors fall through to
// a 500 with a sanitised message (actual error is logged).
func mapChangePlanErr(c *gin.Context, err error, logger *slog.Logger) {
	switch {
	case errors.Is(err, planchange.ErrSubscriptionReadOnly):
		c.AbortWithStatusJSON(http.StatusPaymentRequired, gin.H{"error": "subscription_read_only"})
	case errors.Is(err, planchange.ErrCurrencyLocked):
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": "currency_locked"})
	case errors.Is(err, planchange.ErrStoreCountOverQuota):
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{"error": "store_count_over_quota"})
	case errors.Is(err, planchange.ErrNoChange):
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": "no_change"})
	case errors.Is(err, planchange.ErrInvalidTargetPlan):
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid_target_plan"})
	default:
		if logger != nil {
			logger.Error("change_plan: unmapped error", "err", err)
		}
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal"})
	}
}
