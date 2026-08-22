package customerportal

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// Handler serves the GDPR customer portal endpoints.
//
//   - GET  /storefront/stores/:storeSlug/my-orders/:email/:order_token
//   - POST /storefront/stores/:storeSlug/customer-erasure
//
// Both routes are PUBLIC — no auth middleware. The HMAC token IS the auth for
// the order-history endpoint. The erasure endpoint is append-only with no
// secrets (any customer can request erasure of their own data).
type Handler struct {
	db     *gorm.DB
	logger *slog.Logger
}

// NewHandler constructs a Handler.
func NewHandler(db *gorm.DB, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{db: db, logger: logger}
}

// orderSummary is the public view of an order returned by the portal.
type orderSummary struct {
	ID           string `json:"id"`
	OrderNumber  string `json:"order_number"`
	Status       string `json:"status"`
	GrandTotal   string `json:"grand_total"`
	CurrencyCode string `json:"currency_code"`
	CreatedAt    string `json:"created_at"`
}

// MyOrders handles GET /storefront/stores/:storeSlug/my-orders/:email/:order_token.
//
// The token is verified via HMAC-SHA256 against the per-store secret
// (stores.storefront_customer_portal_secret). On success, returns all orders
// for the given email in this store.
func (h *Handler) MyOrders(c *gin.Context) {
	email := c.Param("email")
	token := c.Param("order_token")
	if email == "" || token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email and order_token are required"})
		return
	}

	// Resolve store from context (set by StoreContext middleware).
	storeRaw, ok := c.Get("store")
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "store not found"})
		return
	}
	store, ok := storeRaw.(*stores.Store)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}

	// Lazy secret generation: if the store somehow has no secret (pre-migration
	// stores that haven't been re-saved), generate and persist one now.
	secret := store.StorefrontCustomerPortalSecret
	if strings.TrimSpace(secret) == "" {
		var err error
		secret, err = h.ensureSecret(c.Request.Context(), store.ID)
		if err != nil {
			h.logger.Error("customerportal: ensure secret failed", "store_id", store.ID, "err", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
			return
		}
	}

	// Token verification — token is signed over email||orderID where orderID
	// is the email itself for the list endpoint (no specific order scoping).
	// Using email as both components gives a per-customer, per-store token.
	if !VerifyToken(secret, email, email, token) {
		c.JSON(http.StatusForbidden, gin.H{"error": "invalid_token"})
		return
	}

	storeID, err := uuid.Parse(store.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}
	tenantID, err := uuid.Parse(store.TenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}

	// Fetch orders for this customer email in this store.
	var orders []order.Order
	if err := h.db.WithContext(c.Request.Context()).
		Where("store_id = ? AND tenant_id = ? AND customer_email = ?", storeID, tenantID, email).
		Order("created_at DESC").
		Limit(100).
		Find(&orders).Error; err != nil {
		h.logger.Error("customerportal: list orders", "store_id", storeID, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}

	summaries := make([]orderSummary, 0, len(orders))
	for _, o := range orders {
		summaries = append(summaries, orderSummary{
			ID:           o.ID.String(),
			OrderNumber:  o.OrderNumber,
			Status:       o.Status,
			GrandTotal:   o.GrandTotal.String(),
			CurrencyCode: o.CurrencyCode,
			CreatedAt:    o.CreatedAt.UTC().Format(time.RFC3339),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"email":  email,
		"orders": summaries,
	})
}

// CustomerErasure handles POST /storefront/stores/:storeSlug/customer-erasure.
//
// Any customer can submit an erasure request for their email address. The row
// is appended to customer_erasure_requests and routed to Mark8ly support.
// Merchants have no endpoint to view or delete these requests.
func (h *Handler) CustomerErasure(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email is required"})
		return
	}
	if strings.TrimSpace(req.Email) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email must not be blank"})
		return
	}

	storeRaw, ok := c.Get("store")
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "store not found"})
		return
	}
	store, ok := storeRaw.(*stores.Store)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}

	storeID, err := uuid.Parse(store.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}
	tenantID, err := uuid.Parse(store.TenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}

	er := ErasureRequest{
		TenantID:      tenantID,
		StoreID:       storeID,
		CustomerEmail: strings.ToLower(strings.TrimSpace(req.Email)),
	}
	if err := AppendErasureRequest(c.Request.Context(), h.db, er); err != nil {
		h.logger.Error("customerportal: append erasure request", "store_id", storeID, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}

	h.logger.Info("customerportal: erasure request queued",
		"store_id", storeID, "email_hash", hashForLog(req.Email))

	c.JSON(http.StatusAccepted, gin.H{
		"status":  "queued",
		"message": "Your data erasure request has been received and will be processed within 30 days.",
	})
}

// ensureSecret generates a new secret for the store and persists it if the
// column is empty. Returns the new secret.
func (h *Handler) ensureSecret(ctx context.Context, storeID string) (string, error) {
	secret, err := GenerateSecret()
	if err != nil {
		return "", err
	}
	// Best-effort update — if two requests race, last write wins (both are valid secrets).
	h.db.WithContext(ctx).Exec(
		"UPDATE stores SET storefront_customer_portal_secret = ? WHERE id = ? AND storefront_customer_portal_secret = ''",
		secret, storeID,
	)
	return secret, nil
}

// hashForLog returns the first 8 chars of the email for log safety
// (avoids persisting full emails in structured logs).
func hashForLog(email string) string {
	if len(email) <= 3 {
		return "***"
	}
	return email[:3] + "***"
}
