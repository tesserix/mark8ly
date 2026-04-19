// Package admin — subscription.go: HTTP handler for store billing
// subscription endpoints (Settings S3).
package admin

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/arbitrage"
	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// SubscriptionHandler handles /admin/stores/:storeId/subscription endpoints.
type SubscriptionHandler struct {
	svc       *subscription.Service
	audit     *audit.Emitter // optional — nil-safe
	db        *gorm.DB       // optional — nil skips arbitrage audit enrichment
	piiLogger arbitrage.PIILogger
	logger    *slog.Logger
}

// NewSubscriptionHandler constructs a SubscriptionHandler.
func NewSubscriptionHandler(svc *subscription.Service, logger *slog.Logger) *SubscriptionHandler {
	return &SubscriptionHandler{
		svc:       svc,
		piiLogger: arbitrage.NopPIILogger{},
		logger:    logger,
	}
}

// WithAudit attaches an audit emitter so subscription/billing actions
// land in Settings -> Audit Logs. Nil-safe.
func (h *SubscriptionHandler) WithAudit(e *audit.Emitter) *SubscriptionHandler {
	h.audit = e
	return h
}

// WithDB attaches a DB handle so GetSubscription can enrich the response with
// the latest arbitrage audit row (P8 §18.8.1). Nil-safe — omitting it causes
// GetSubscription to return arbitrage_flag=false with no audit payload.
func (h *SubscriptionHandler) WithDB(db *gorm.DB) *SubscriptionHandler {
	h.db = db
	return h
}

// WithPIILogger attaches a PIILogger for arbitrage audit reads.
func (h *SubscriptionHandler) WithPIILogger(pii arbitrage.PIILogger) *SubscriptionHandler {
	h.piiLogger = pii
	return h
}

// ArbitrageAuditSummary is the public subset of a SubscriptionArbitrageAudit
// row returned on the GetSubscription endpoint. Intentionally omits ip_hash,
// reviewed_by, and reviewed_at — those are billing-ops-only via internal tooling.
type ArbitrageAuditSummary struct {
	CardCountry    string    `json:"card_country"`
	BillingCountry string    `json:"billing_country"`
	IPCountry      string    `json:"ip_country"`
	Resolution     string    `json:"resolution"`
	FlaggedAt      time.Time `json:"flagged_at"`
	MismatchReason string    `json:"mismatch_reason"`
}

// SubscriptionResponse is the wire DTO for a store subscription.
type SubscriptionResponse struct {
	ID                   string  `json:"id"`
	StoreID              string  `json:"store_id"`
	Plan                 string  `json:"plan"`
	Status               string  `json:"status"`
	CurrentPeriodStart   *string `json:"current_period_start,omitempty"`
	CurrentPeriodEnd     *string `json:"current_period_end,omitempty"`
	CancelAtPeriodEnd    bool    `json:"cancel_at_period_end"`
	StripeSubscriptionID *string `json:"stripe_subscription_id,omitempty"`
	CreatedAt            string  `json:"created_at"`
	// P8 — geo-pricing anti-arbitrage fields (§18.8.1). Always present;
	// LatestArbitrageAudit is null when no flag has ever been raised.
	ArbitrageFlag        bool                   `json:"arbitrage_flag"`
	LatestArbitrageAudit *ArbitrageAuditSummary `json:"latest_arbitrage_audit,omitempty"`
}

func toSubscriptionResponse(s subscription.StoreSubscription) SubscriptionResponse {
	resp := SubscriptionResponse{
		ID:                s.ID.String(),
		StoreID:           s.StoreID.String(),
		Plan:              string(s.Plan),
		Status:            string(s.Status),
		CancelAtPeriodEnd: s.CancelAtPeriodEnd,
		CreatedAt:         s.CreatedAt.Format("2006-01-02T15:04:05Z"),
		ArbitrageFlag:     s.ArbitrageFlag,
	}
	if s.StripeSubscriptionID != nil {
		resp.StripeSubscriptionID = s.StripeSubscriptionID
	}
	if s.CurrentPeriodStart != nil {
		t := s.CurrentPeriodStart.Format("2006-01-02T15:04:05Z")
		resp.CurrentPeriodStart = &t
	}
	if s.CurrentPeriodEnd != nil {
		t := s.CurrentPeriodEnd.Format("2006-01-02T15:04:05Z")
		resp.CurrentPeriodEnd = &t
	}
	return resp
}

// GetSubscription handles GET /admin/stores/:storeId/subscription.
func (h *SubscriptionHandler) GetSubscription(c *gin.Context) {
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

	sub, err := h.svc.GetSubscription(c.Request.Context(), tenantID, storeID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	resp := toSubscriptionResponse(*sub)

	// P8 §18.8.1 — enrich with the latest arbitrage audit row when DB is wired.
	// Degrade gracefully on error: arbitrage data is not load-bearing for billing.
	if h.db != nil {
		var auditRow arbitrage.SubscriptionArbitrageAudit
		auditErr := h.db.WithContext(c.Request.Context()).
			Where("tenant_id = ? AND store_id = ?", tenantID, storeID).
			Order("flagged_at DESC").
			Limit(1).
			First(&auditRow).Error
		if auditErr == nil {
			// Log PII access before returning the row.
			userIDStr := c.GetString("user_id")
			userID, _ := uuid.Parse(userIDStr)
			h.piiLogger.LogPIIAccess(c.Request.Context(), arbitrage.PIIAccessEvent{
				Actor:     userID,
				StoreID:   storeID,
				TenantID:  tenantID,
				Operation: "arbitrage_audit_read_admin_subscription",
			})

			cardCountry := ""
			if auditRow.CardCountry != nil {
				cardCountry = *auditRow.CardCountry
			}
			billingCountry := ""
			if auditRow.BillingCountry != nil {
				billingCountry = *auditRow.BillingCountry
			}
			ipCountry := ""
			if auditRow.IPCountry != nil {
				ipCountry = *auditRow.IPCountry
			}
			mismatchReason := ""
			if auditRow.MismatchReason != nil {
				mismatchReason = *auditRow.MismatchReason
			}
			resp.LatestArbitrageAudit = &ArbitrageAuditSummary{
				CardCountry:    cardCountry,
				BillingCountry: billingCountry,
				IPCountry:      ipCountry,
				Resolution:     string(auditRow.Resolution),
				FlaggedAt:      auditRow.FlaggedAt,
				MismatchReason: mismatchReason,
			}
		} else if !errors.Is(auditErr, gorm.ErrRecordNotFound) {
			// Non-404 errors are logged but don't fail the request.
			h.logger.Warn("arbitrage audit load failed; omitting from response", "err", auditErr)
		}
	}

	c.JSON(http.StatusOK, resp)
}

// CreateCheckoutRequest is the request body for POST .../subscription/checkout.
type CreateCheckoutRequest struct {
	Plan       string `json:"plan" binding:"required"`
	SuccessURL string `json:"success_url" binding:"required"`
	CancelURL  string `json:"cancel_url" binding:"required"`
}

// CreateCheckout handles POST /admin/stores/:storeId/subscription/checkout.
func (h *SubscriptionHandler) CreateCheckout(c *gin.Context) {
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

	var req CreateCheckoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", "invalid request body"), h.logger)
		return
	}

	url, err := h.svc.CreateCheckoutSession(c.Request.Context(), subscription.CheckoutInput{
		TenantID:   tenantID,
		StoreID:    storeID,
		Plan:       req.Plan,
		SuccessURL: req.SuccessURL,
		CancelURL:  req.CancelURL,
	})
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	// Records the *intent* to change plan. The actual plan transition
	// fires from the Stripe webhook once the customer completes payment
	// via the /webhooks/stripe-billing handler.
	h.audit.Emit(c, audit.Event{
		Action:       "subscription.checkout_started",
		ResourceType: "subscription",
		Metadata:     map[string]any{"plan": req.Plan},
	})
	c.JSON(http.StatusOK, gin.H{"url": url})
}

// CreatePortalRequest is the request body for POST .../subscription/portal.
type CreatePortalRequest struct {
	ReturnURL string `json:"return_url" binding:"required"`
}

// CreatePortal handles POST /admin/stores/:storeId/subscription/portal.
func (h *SubscriptionHandler) CreatePortal(c *gin.Context) {
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

	var req CreatePortalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", "invalid request body"), h.logger)
		return
	}

	url, err := h.svc.CreatePortalSession(c.Request.Context(), tenantID, storeID, req.ReturnURL)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	h.audit.Emit(c, audit.Event{
		Action:       "subscription.billing_portal_opened",
		ResourceType: "subscription",
	})
	c.JSON(http.StatusOK, gin.H{"url": url})
}

