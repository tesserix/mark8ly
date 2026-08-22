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
	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/plangate"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// SubscriptionHandler handles /admin/stores/:storeId/subscription endpoints.
type SubscriptionHandler struct {
	svc       *subscription.Service
	audit     *audit.Emitter        // optional — nil-safe
	db        *gorm.DB              // optional — nil skips arbitrage audit enrichment
	stripe    *billingstripe.Client // optional — nil skips payment method enrichment
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

// WithStripe attaches the billing Stripe client so GetSubscription can enrich
// the response with the customer's default card (brand + last4). Nil-safe —
// omitting it causes payment_method fields to be omitted from the response.
func (h *SubscriptionHandler) WithStripe(c *billingstripe.Client) *SubscriptionHandler {
	h.stripe = c
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
	// Billing UI — summary of the customer's default payment method.
	// PaymentMethodType is "card" | "link" | "" when present; callers render
	// differently per type. For Type=card, Last4 is the card's last 4 digits;
	// for Type=link, Last4 carries the Link account email (display only).
	// All three fields omitted when the customer has no PM on file yet.
	PaymentMethodType  *string `json:"payment_method_type,omitempty"`
	PaymentMethodBrand *string `json:"payment_method_brand,omitempty"`
	PaymentMethodLast4 *string `json:"payment_method_last4,omitempty"`

	// Trial banner state — drives the in-admin trial countdown banner.
	// HasDefaultPaymentMethod is sourced from the row (mirrored by the
	// customer.updated webhook) — it's the same signal the trial reminder
	// cron uses, so the UI banner and the email cadence stay aligned.
	// TrialEndsAt / DaysRemaining are populated only while in trial (status
	// in {signup, trialing}); on active/expired/etc. they're omitted.
	// TrialCTA is one of: "pick_plan" | "add_card" | "billing_imminent" |
	// "all_set" | null. The frontend uses it to choose copy + button label
	// without re-deriving the same logic from days/PM/plan triplets.
	HasDefaultPaymentMethod bool    `json:"has_default_payment_method"`
	TrialEndsAt             *string `json:"trial_ends_at,omitempty"`
	DaysRemainingInTrial    *int    `json:"days_remaining_in_trial,omitempty"`
	TrialCTA                *string `json:"trial_cta,omitempty"`

	// FeatureLimits is the JSON-ready snapshot of the per-plan limits for
	// the current plan — see plangate.AllFeatureLimits. Sentinel values:
	// -1 = Unlimited, -2 = Negotiated, 0 = Disabled, otherwise the numeric
	// cap. The admin UI uses this to disable gated controls and render
	// "upgrade to {plan}" tooltips without re-encoding the matrix client-side.
	FeatureLimits map[string]int `json:"feature_limits"`
	// MinPlanForFeature maps each feature key to the lowest plan that
	// enables it, so the frontend can render "Upgrade to Studio" / "Upgrade
	// to Pro" CTAs accurately. For features enabled on the current plan
	// the entry is the current plan; for features the matrix has no plan
	// for, the entry is "pro" (conservative — see plangate.MinPlanForFeature).
	MinPlanForFeature map[string]string `json:"min_plan_for_feature"`
}

func toSubscriptionResponse(s subscription.StoreSubscription) SubscriptionResponse {
	resp := SubscriptionResponse{
		ID:                      s.ID.String(),
		StoreID:                 s.StoreID.String(),
		Plan:                    string(s.Plan),
		Status:                  string(s.Status),
		CancelAtPeriodEnd:       s.CancelAtPeriodEnd,
		CreatedAt:               s.CreatedAt.Format("2006-01-02T15:04:05Z"),
		ArbitrageFlag:           s.ArbitrageFlag,
		HasDefaultPaymentMethod: s.HasDefaultPaymentMethod,
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
	enrichTrialBanner(&resp, s, time.Now().UTC())
	enrichFeatureLimits(&resp, s.Plan)
	return resp
}

// enrichFeatureLimits populates FeatureLimits and MinPlanForFeature on resp
// from plangate. Single source of truth: every gated UI control reads its
// permission from this map, eliminating drift between the server-side
// gate (plangate.RequireFeature) and the client-side rendering. Static
// per plan, so no per-request computation cost beyond a map clone.
func enrichFeatureLimits(resp *SubscriptionResponse, plan subscription.SubscriptionPlan) {
	resp.FeatureLimits = plangate.AllFeatureLimits(plan)

	features := plangate.AllFeatures()
	resp.MinPlanForFeature = make(map[string]string, len(features))
	for _, f := range features {
		resp.MinPlanForFeature[string(f)] = string(plangate.MinPlanForFeature(f))
	}
}

// enrichTrialBanner populates the trial countdown fields on resp when the
// subscription is in a pre-billing state. Computed server-side so the admin
// UI doesn't need to duplicate the day-90 / PM / plan-chosen logic that
// drives the trial reminder cron — they share this single derivation.
//
// CTA matrix:
//
//	signup  (no plan picked yet)         → pick_plan
//	trialing + !has_pm + days > 1        → add_card
//	trialing + !has_pm + days <= 1       → add_card  (frontend uses days_remaining for urgency)
//	trialing + has_pm  + days > 3        → all_set
//	trialing + has_pm  + days <= 3       → billing_imminent
//	any other status (active, expired …) → CTA stays nil; banner is hidden
func enrichTrialBanner(resp *SubscriptionResponse, s subscription.StoreSubscription, now time.Time) {
	if s.Status != subscription.StatusSignup && s.Status != subscription.StatusTrialing {
		return
	}

	endsAt := s.CreatedAt.Add(trial.TrialDays * 24 * time.Hour).UTC()
	endsAtStr := endsAt.Format("2006-01-02T15:04:05Z")
	resp.TrialEndsAt = &endsAtStr

	// Days remaining — clamp at zero so the UI never shows a negative number
	// during the brief window between expiry-time and the expiry cron run.
	hoursLeft := endsAt.Sub(now).Hours()
	days := int(hoursLeft / 24)
	if hoursLeft > 0 && hoursLeft < 24 {
		days = 1 // round up the final partial day so "0 days" never appears mid-trial
	}
	if days < 0 {
		days = 0
	}
	resp.DaysRemainingInTrial = &days

	cta := computeTrialCTA(s.Status, s.HasDefaultPaymentMethod, days)
	if cta != "" {
		resp.TrialCTA = &cta
	}
}

func computeTrialCTA(status subscription.SubscriptionStatus, hasPM bool, daysRemaining int) string {
	if status == subscription.StatusSignup {
		return "pick_plan"
	}
	// status == trialing
	if !hasPM {
		return "add_card"
	}
	if daysRemaining <= 3 {
		return "billing_imminent"
	}
	return "all_set"
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

	// Enrich with the customer's default payment method when Stripe is wired.
	// Handles both card and Stripe Link payment methods. Degrade gracefully
	// on error — payment method display is not load-bearing.
	if h.stripe != nil && sub.StripeCustomerID != "" {
		pm, ok, pmErr := billingstripe.GetCustomerDefaultPaymentMethod(c.Request.Context(), h.stripe, sub.StripeCustomerID)
		if pmErr != nil {
			h.logger.Warn("payment method load failed; omitting from response", "err", pmErr)
		} else if ok {
			brand := pm.Brand
			last4 := pm.Last4
			pmType := pm.Type
			resp.PaymentMethodBrand = &brand
			resp.PaymentMethodLast4 = &last4
			resp.PaymentMethodType = &pmType
		}
	}

	c.JSON(http.StatusOK, resp)
}

// Bootstrap handles POST /admin/stores/:storeId/subscription/bootstrap.
//
// Idempotently initialises a store_subscriptions row for stores created
// before the v2.3 signup pipeline shipped. Returns the subscription in
// the same shape as GetSubscription.
func (h *SubscriptionHandler) Bootstrap(c *gin.Context) {
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

	email := c.GetString("user_email")
	name := ""
	if storeVal, ok := c.Get("store"); ok {
		if storeRow, ok := storeVal.(*stores.Store); ok && storeRow != nil {
			name = storeRow.Name
		}
	}

	sub, err := h.svc.Bootstrap(c.Request.Context(), subscription.BootstrapInput{
		TenantID: tenantID,
		StoreID:  storeID,
		Email:    email,
		Name:     name,
	})
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	if h.audit != nil {
		h.audit.Emit(c, audit.Event{
			Action:       "subscription.bootstrap",
			ResourceType: "subscription",
			Metadata:     map[string]any{"store_id": storeID.String()},
		})
	}

	c.JSON(http.StatusOK, toSubscriptionResponse(*sub))
}

// InvoiceDTO is the wire shape for a single invoice row rendered in the
// admin billing page's invoice history table.
type InvoiceDTO struct {
	ID          string `json:"id"`
	Number      string `json:"number"`
	CreatedAt   string `json:"created_at"`
	AmountPaid  int64  `json:"amount_paid"`
	AmountDue   int64  `json:"amount_due"`
	Currency    string `json:"currency"`
	Status      string `json:"status"`
	HostedURL   string `json:"hosted_invoice_url"`
	PDFURL      string `json:"invoice_pdf"`
	PeriodStart string `json:"period_start"`
	PeriodEnd   string `json:"period_end"`
}

// ListInvoicesResponse is the envelope returned by GET .../subscription/invoices.
type ListInvoicesResponse struct {
	Data []InvoiceDTO `json:"data"`
}

// ListInvoices handles GET /admin/stores/:storeId/subscription/invoices.
//
// Returns up to 25 most-recent invoices for the store's Stripe customer.
// When no Stripe customer is wired yet (pre-bootstrap row), responds with an
// empty list rather than an error so the UI can render "No invoices yet".
func (h *SubscriptionHandler) ListInvoices(c *gin.Context) {
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

	if h.stripe == nil || sub.StripeCustomerID == "" {
		c.JSON(http.StatusOK, ListInvoicesResponse{Data: []InvoiceDTO{}})
		return
	}

	invoices, err := billingstripe.ListCustomerInvoices(c.Request.Context(), h.stripe, sub.StripeCustomerID, 25)
	if err != nil {
		h.logger.Warn("stripe list invoices failed", "err", err)
		RespondErr(c, err, h.logger)
		return
	}

	resp := ListInvoicesResponse{Data: make([]InvoiceDTO, 0, len(invoices))}
	for _, inv := range invoices {
		resp.Data = append(resp.Data, InvoiceDTO{
			ID:          inv.ID,
			Number:      inv.Number,
			CreatedAt:   inv.Created.Format(time.RFC3339),
			AmountPaid:  inv.AmountPaid,
			AmountDue:   inv.AmountDue,
			Currency:    inv.Currency,
			Status:      inv.Status,
			HostedURL:   inv.HostedURL,
			PDFURL:      inv.PDFURL,
			PeriodStart: inv.PeriodStart.Format(time.RFC3339),
			PeriodEnd:   inv.PeriodEnd.Format(time.RFC3339),
		})
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
