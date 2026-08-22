package appaddon

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/attestations"
	"github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// InvoiceAPI is the narrow Stripe surface this handler needs. Tests
// inject a fake; production wires *stripe.Client via the adapter at
// the bottom of this file.
type InvoiceAPI interface {
	CreateOneOffInvoice(ctx context.Context, in stripe.OneOffInvoiceInput) (*stripe.Invoice, error)
}

// StripeClientAdapter wraps a *stripe.Client so it satisfies InvoiceAPI.
// The package-level function form keeps *stripe.Client as the source
// of truth; this adapter only bridges call shape.
type StripeClientAdapter struct{ Client *stripe.Client }

// CreateOneOffInvoice delegates to the package function.
func (a *StripeClientAdapter) CreateOneOffInvoice(ctx context.Context, in stripe.OneOffInvoiceInput) (*stripe.Invoice, error) {
	return stripe.CreateOneOffInvoice(ctx, a.Client, in)
}

// subscriptionLookup is the narrow subscription.Repository slice this
// handler uses. Kept local so tests only stub what's needed.
type subscriptionLookup interface {
	GetByStoreID(ctx context.Context, db *gorm.DB, tenantID, storeID uuid.UUID) (*subscription.StoreSubscription, error)
}

// Handler mounts POST /admin/stores/:storeId/subscription/add-on/white-label-app.
// Pro-gated. Records the Apple 4.2.6 attestation at purchase time
// (not at invoice.paid) so abandoned Stripe flows still capture the ack.
type Handler struct {
	db      *gorm.DB
	stripe  InvoiceAPI
	subRepo subscriptionLookup
	clock   func() time.Time
}

// Config groups Handler construction params.
type Config struct {
	DB      *gorm.DB
	Stripe  InvoiceAPI
	SubRepo subscriptionLookup
	Clock   func() time.Time // default: time.Now
}

// NewHandler constructs the purchase handler. All fields in cfg are
// required except Clock (defaults to time.Now).
func NewHandler(cfg Config) *Handler {
	clock := cfg.Clock
	if clock == nil {
		clock = time.Now
	}
	return &Handler{db: cfg.DB, stripe: cfg.Stripe, subRepo: cfg.SubRepo, clock: clock}
}

// PurchaseRequest is the JSON body for POST .../add-on/white-label-app.
type PurchaseRequest struct {
	Apple426Acknowledged bool   `json:"apple_4_2_6_acknowledged"`
	AttestationText      string `json:"attestation_text"`
}

// PurchaseResponse is returned on 201.
type PurchaseResponse struct {
	HostedInvoiceURL string `json:"hosted_invoice_url"`
	StripeInvoiceID  string `json:"stripe_invoice_id"`
	AmountDueCents   int64  `json:"amount_due_cents"`
	Currency         string `json:"currency"`
}

// Purchase creates the Stripe invoice + records the attestation row.
//
//	POST /admin/stores/:storeId/subscription/add-on/white-label-app
//	Body: {apple_4_2_6_acknowledged, attestation_text}
//
//	201 → PurchaseResponse
//	400 → invalid_body / apple_4_2_6_ack_required
//	403 → pro_plan_required
//	409 → already_active
//	500 → subscription_lookup_failed / stripe_invoice_failed / attestation_record_failed
func (h *Handler) Purchase(c *gin.Context) {
	var req PurchaseRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body"})
		return
	}
	if !req.Apple426Acknowledged {
		c.JSON(http.StatusBadRequest, gin.H{"error": "apple_4_2_6_ack_required"})
		return
	}
	if req.AttestationText == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "attestation_text_required"})
		return
	}

	tenantID, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_store_id"})
		return
	}

	sub, err := h.subRepo.GetByStoreID(c.Request.Context(), h.db, tenantID, storeID)
	if err != nil || sub == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "subscription_lookup_failed"})
		return
	}
	if sub.Plan != subscription.PlanPro {
		c.JSON(http.StatusForbidden, gin.H{"error": "pro_plan_required"})
		return
	}
	if sub.HasWhiteLabelAppAddOn {
		c.JSON(http.StatusConflict, gin.H{"error": "already_active"})
		return
	}
	if sub.StripeCustomerID == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "stripe_customer_missing"})
		return
	}

	// Compute proration against the Pro anchor renewal. If the Pro
	// sub has no current_period_end (e.g. trialing edge case), fall
	// back to "full year remaining" so we don't accidentally short-
	// charge the setup fee alone.
	now := h.clock().UTC()
	renewalAt := now.Add(365 * 24 * time.Hour) // default: full year
	if sub.CurrentPeriodEnd != nil {
		renewalAt = sub.CurrentPeriodEnd.UTC()
	}
	amountCents := ProrationCents(now, renewalAt)

	// Create the Stripe invoice. IdempotencyKey is day-bucketed so a
	// user who double-submits within the same day converges on one
	// invoice; a retry next day creates a fresh invoice (expected —
	// the first one will auto-advance through Stripe if left unpaid).
	dayBucket := now.Unix() / 86400
	idem := fmt.Sprintf("addon:%s:%d", storeID, dayBucket)

	inv, err := h.stripe.CreateOneOffInvoice(c.Request.Context(), stripe.OneOffInvoiceInput{
		CustomerID:  sub.StripeCustomerID,
		Currency:    "usd",
		AmountCents: amountCents,
		Description: "White-label mobile app add-on (prorated + $2000 setup)",
		Metadata: map[string]string{
			"kind":      "white_label_app_add_on",
			"tenant_id": tenantID.String(),
			"store_id":  storeID.String(),
		},
		IdempotencyKey: idem,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "stripe_invoice_failed"})
		return
	}

	// Attestation recorded BEFORE we return to the user. A user who
	// abandons the Stripe hosted page still has an auditable record
	// of their ack (§14.2).
	actor := c.GetString("user_id")
	userID, _ := uuid.Parse(actor)
	if _, err := attestations.Record(c.Request.Context(), h.db, attestations.Input{
		TenantID: tenantID, StoreID: storeID, SubscriptionID: sub.ID,
		AttestationType:  attestations.TypeApple426,
		AttestedByUserID: userID,
		AttestationText:  req.AttestationText,
		IPAddress:        c.ClientIP(),
		UserAgent:        c.GetHeader("User-Agent"),
		StripeInvoiceID:  inv.ID,
	}); err != nil && !errors.Is(err, attestations.ErrDuplicateInvoice) {
		// Duplicate invoice means the webhook already fired for the
		// same invoice ID via the idempotency-key path — that's OK.
		// Any other error is a real problem; return 500 so the
		// caller retries the whole flow.
		c.JSON(http.StatusInternalServerError, gin.H{"error": "attestation_record_failed"})
		return
	}

	c.JSON(http.StatusCreated, PurchaseResponse{
		HostedInvoiceURL: inv.HostedInvoiceURL,
		StripeInvoiceID:  inv.ID,
		AmountDueCents:   amountCents,
		Currency:         "usd",
	})
}
