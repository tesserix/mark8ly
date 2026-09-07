// Package subscription — internal_handler.go
//
// POST /internal/stores/:storeID/ensure-subscription is the platform-api
// callback that gives a brand-new store its subscription row, and with it its
// trial clock.
//
// It exists because that clock had no other start (#827). Bootstrap was
// reachable from exactly one place — a CTA on the admin Billing page — so the
// 90-day trial began when a merchant happened to open Billing, and a merchant
// who never opened it had no row at all: no trial, no expiry, and invisible to
// every billing cron.
//
// Called from onboarding.Complete AFTER EnsureSelfStore, because
// store_subscriptions.store_id is a foreign key onto stores(id) — the
// projection has to land first. Idempotent, so a retry of onboarding is safe.
package subscription

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/auth"
)

// Bootstrapper is the one method this handler needs. *Service satisfies it;
// the interface keeps the handler testable without a database.
type Bootstrapper interface {
	Bootstrap(ctx context.Context, in BootstrapInput) (*StoreSubscription, error)
}

// SignupPromoRedeemer redeems a promo code against a subscription that has
// just been created. *promo.Service satisfies it through a small adapter.
//
// An interface rather than the concrete type because internal/promo already
// imports this package for its plan and status constants; depending on it back
// would be an import cycle.
type SignupPromoRedeemer interface {
	RedeemAtSignup(ctx context.Context, in SignupPromoInput) (SignupPromoOffer, error)
	// ValidateForSignup answers the same question without spending the code,
	// for the onboarding field to call as the merchant types.
	ValidateForSignup(ctx context.Context, in SignupPromoInput) (SignupPromoOffer, error)
}

// SignupPromoInput and SignupPromoOffer mirror promo.SignupInput /
// promo.SignupOffer across that same boundary. The adapter that converts
// between them lives in main.go, which imports both.
type SignupPromoInput struct {
	Code          string
	MerchantEmail string
	Currency      string
	Sub           *StoreSubscription
}

// SignupPromoOffer is what the code granted. RejectReason is already the
// PUBLIC reason — the adapter runs promo.PublicReasonFor — because this value
// crosses to platform-api and on to a merchant.
type SignupPromoOffer struct {
	TrialExtensionDays int
	RejectReason       string
}

// InternalHandler serves the platform-api subscription callback.
type InternalHandler struct {
	svc   Bootstrapper
	promo SignupPromoRedeemer
}

// NewInternalHandler constructs an InternalHandler.
func NewInternalHandler(svc Bootstrapper) *InternalHandler {
	return &InternalHandler{svc: svc}
}

// WithPromo wires promo redemption at signup (#620). Nil-safe: without it a
// code sent by the caller is REFUSED rather than ignored, so a merchant is
// never told their code applied when nothing could have applied it.
func (h *InternalHandler) WithPromo(p SignupPromoRedeemer) *InternalHandler {
	h.promo = p
	return h
}

// RegisterRoutes mounts the callback behind the shared internal secret.
//
// InternalSecretAuth, not HeaderTrustAuth: this call is made during
// onboarding, when the merchant has not signed in anywhere, so there is no
// X-User-Id or X-Tenant-Id to present and HeaderTrustAuth would refuse every
// legitimate call. platform-api's VendorClient already sends X-Internal-Auth,
// so no new configuration is needed on the calling side.
func (h *InternalHandler) RegisterRoutes(g *gin.RouterGroup, internalSecret string) {
	g.POST("/stores/:storeID/ensure-subscription",
		auth.InternalSecretAuth(internalSecret), h.ensureSubscription)
	// Promo validation for the onboarding field (#620). Deliberately on
	// /internal rather than the public group: #620 calls an open validate
	// endpoint "an oracle for guessing valid codes", and platform-api — which
	// owns the onboarding session and already holds this secret — is the only
	// caller that needs it. Keeping it internal removes the oracle instead of
	// rate-limiting one into existence.
	g.POST("/promo/validate-for-signup",
		auth.InternalSecretAuth(internalSecret), h.validatePromoForSignup)
}

type validatePromoRequest struct {
	Code string `json:"code" binding:"required"`
	// Email is the address the per-email cap counts against — the one abuse
	// control available at signup, and available because onboarding has
	// already verified the address.
	Email    string `json:"email"`
	Currency string `json:"currency"`
}

// validatePromoForSignup reports what a code would grant a merchant who is
// signing up, and records nothing.
//
// The response deliberately carries only the days and a reason. No code
// echo, no terms, no indication of whether an unknown code exists: the
// reject reason is already collapsed by promo.PublicReasonFor, which merges
// not_found and expired precisely so a caller cannot tell a real code from a
// guessed one.
func (h *InternalHandler) validatePromoForSignup(c *gin.Context) {
	var req validatePromoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "invalid_request", "detail": err.Error(),
		})
		return
	}
	if h.promo == nil {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "promo_unavailable"})
		return
	}

	offer, err := h.promo.ValidateForSignup(c.Request.Context(), SignupPromoInput{
		Code:          req.Code,
		MerchantEmail: req.Email,
		Currency:      req.Currency,
	})
	// A refused code is a 200 with valid:false, not a 4xx. The merchant asked
	// a question and got an answer; "your code will not work" is a successful
	// answer, and giving it its own status invites a caller to treat it as a
	// transport failure and retry.
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"valid":                err == nil,
		"trial_extension_days": offer.TrialExtensionDays,
		"reject_reason":        offer.RejectReason,
	}})
}

type ensureSubscriptionRequest struct {
	TenantID string `json:"tenant_id" binding:"required"`
	// Email and Name are used only if a Stripe customer is minted later; they
	// are carried now so the caller does not have to be asked again.
	Email string `json:"email"`
	Name  string `json:"name"`
	// Currency is the store's ISO 4217 billing currency. Known at signup and
	// supplied by nothing else afterwards.
	Currency string `json:"currency"`
	// PromoCode is what the merchant typed at onboarding, if anything (#620).
	// Empty is the normal case.
	PromoCode string `json:"promo_code"`
	// TaxID is what the merchant typed in onboarding's Tax ID field,
	// unvalidated. Its country comes from the store, so the caller sends only
	// the id.
	TaxID string `json:"tax_id"`
	// CountryCode is the store's ISO 3166-1 alpha-2 country, used as the tax
	// id's jurisdiction. Distinct from Currency: a store can bill in a
	// currency other than its own country's.
	CountryCode string `json:"country_code"`
}

func (h *InternalHandler) ensureSubscription(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeID"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid_store_id"})
		return
	}

	var req ensureSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error": "invalid_request", "detail": err.Error(),
		})
		return
	}
	tenantID, err := uuid.Parse(req.TenantID)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid_tenant_id"})
		return
	}

	sub, err := h.svc.Bootstrap(c.Request.Context(), BootstrapInput{
		TenantID:        tenantID,
		StoreID:         storeID,
		Email:           req.Email,
		Name:            req.Name,
		BillingCurrency: req.Currency,
		TaxID:           req.TaxID,
		TaxIDCountry:    req.CountryCode,
	})
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "ensure_subscription_failed", "detail": err.Error(),
		})
		return
	}

	// Redeem the promo code, if one was typed. AFTER Bootstrap, because the
	// ledger row needs a subscription id and the trial extension needs a row
	// to move — this is the earliest moment redemption is possible at all.
	//
	// A refusal does NOT fail the call. The store exists and the trial has
	// started; onboarding must complete either way. The outcome travels in
	// the response so the caller can tell the merchant what happened.
	offer := SignupPromoOffer{}
	if code := strings.TrimSpace(req.PromoCode); code != "" {
		if h.promo == nil {
			// Refuse rather than ignore. Silently dropping the code would
			// tell a merchant it applied while nothing could have applied
			// it — the shape of #620's original defect.
			offer.RejectReason = "promo_unavailable"
		} else {
			granted, promoErr := h.promo.RedeemAtSignup(c.Request.Context(), SignupPromoInput{
				Code:          code,
				MerchantEmail: req.Email,
				Currency:      req.Currency,
				Sub:           sub,
			})
			offer = granted
			if promoErr != nil && offer.RejectReason == "" {
				offer.RejectReason = "promo_failed"
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"id":         sub.ID.String(),
		"plan":       string(sub.Plan),
		"status":     string(sub.Status),
		"created_at": sub.CreatedAt,
		"promo": gin.H{
			"trial_extension_days": offer.TrialExtensionDays,
			"reject_reason":        offer.RejectReason,
		},
	}})
}
