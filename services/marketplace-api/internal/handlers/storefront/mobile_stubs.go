// Package storefront — mobile_stubs.go: Additional handler methods required
// by the mobile storefront route group. These methods are defined on existing
// handler types (OrderDetailHandler, CustomerAccountHandler, ReviewsHandler).
package storefront

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/customer"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// --- OrderDetailHandler.ListOrders ---

// listOrdersQuery holds the pagination parameters for listing orders.
type listOrdersQuery struct {
	Page     int `form:"page" binding:"omitempty,min=1"`
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=50"`
}

func (q *listOrdersQuery) defaults() {
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 20
	}
}

// ListOrders handles GET /mobile/storefront/stores/:storeSlug/orders.
// Reads customer_profile_id from context, queries orders by customer_id,
// returns a paginated list.
func (h *OrderDetailHandler) ListOrders(c *gin.Context) {
	storeVal, _ := c.Get("store")
	store, _ := storeVal.(*stores.Store)
	if store == nil {
		respondNotFound(c)
		return
	}

	profile := mustGetCustomerProfile(c)
	if profile == nil {
		return
	}

	var q listOrdersQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": err.Error(),
		})
		return
	}
	q.defaults()

	// Query orders by customer_id + store_id, paginated.
	var orders []order.Order
	var total int64

	base := h.db.WithContext(c.Request.Context()).
		Model(&order.Order{}).
		Where("customer_id = ? AND store_id = ? AND deleted_at IS NULL", profile.ID, store.ID)

	if err := base.Count(&total).Error; err != nil {
		respondInternal(c, h.logger, err)
		return
	}

	if err := base.
		Order("created_at DESC").
		Limit(q.PageSize).
		Offset((q.Page - 1) * q.PageSize).
		Find(&orders).Error; err != nil {
		respondInternal(c, h.logger, err)
		return
	}

	out := make([]storefrontOrderSummary, 0, len(orders))
	for i := range orders {
		out = append(out, toOrderSummary(&orders[i]))
	}

	totalPages := int64(0)
	if q.PageSize > 0 {
		totalPages = (total + int64(q.PageSize) - 1) / int64(q.PageSize)
	}

	c.JSON(http.StatusOK, gin.H{
		"data": out,
		"meta": gin.H{
			"page":        q.Page,
			"page_size":   q.PageSize,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}

// storefrontOrderSummary is a lightweight DTO for the order list (no items).
type storefrontOrderSummary struct {
	ID            string `json:"id"`
	OrderNumber   string `json:"order_number"`
	Status        string `json:"status"`
	PaymentStatus string `json:"payment_status"`
	GrandTotal    string `json:"grand_total"`
	CurrencyCode  string `json:"currency_code"`
	PlacedAt      string `json:"placed_at"`
}

func toOrderSummary(o *order.Order) storefrontOrderSummary {
	return storefrontOrderSummary{
		ID:            o.ID.String(),
		OrderNumber:   o.OrderNumber,
		Status:        string(o.Status),
		PaymentStatus: string(o.PaymentStatus),
		GrandTotal:    o.GrandTotal.StringFixed(2),
		CurrencyCode:  o.CurrencyCode,
		PlacedAt:      o.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// --- CustomerAccountHandler.Register ---

// registerRequest is the optional body for mobile customer registration.
type registerRequest struct {
	FirstName *string `json:"first_name" binding:"omitempty,max=200"`
	LastName  *string `json:"last_name" binding:"omitempty,max=200"`
}

// Register handles POST /mobile/storefront/stores/:storeSlug/account/register.
// Reads gip_uid + email from Bearer context, optional name from body, calls
// EnsureProfile, returns profile.
func (h *CustomerAccountHandler) Register(c *gin.Context) {
	gipUID := c.GetString(CustomerGipUIDKey)
	email := c.GetString(CustomerEmailKey)
	if gipUID == "" || email == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "Authentication required",
		})
		return
	}

	storeVal, _ := c.Get("store")
	store, _ := storeVal.(*stores.Store)
	if store == nil {
		respondNotFound(c)
		return
	}

	storeID, err := uuid.Parse(store.ID)
	if err != nil {
		respondNotFound(c)
		return
	}
	tenantID, err := uuid.Parse(store.TenantID)
	if err != nil {
		respondNotFound(c)
		return
	}

	// Bind optional name fields from body.
	var req registerRequest
	// Ignore bind errors — body is optional.
	_ = c.ShouldBindJSON(&req)

	firstName := ""
	lastName := ""
	if req.FirstName != nil {
		firstName = strings.TrimSpace(*req.FirstName)
	}
	if req.LastName != nil {
		lastName = strings.TrimSpace(*req.LastName)
	}

	profile, err := h.customerSvc.EnsureProfile(c.Request.Context(), customer.EnsureProfileInput{
		StoreID:   storeID,
		TenantID:  tenantID,
		GipUID:    gipUID,
		Email:     email,
		FirstName: firstName,
		LastName:  lastName,
	}, c)
	if err != nil {
		h.logger.Error("failed to register customer profile",
			"error", err,
			"email", email,
			"store_id", store.ID,
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "Failed to register profile",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": mapProfileToResponse(profile)})
}

// --- CustomerAccountHandler.CheckEmail ---

// CheckEmail handles GET /mobile/storefront/stores/:storeSlug/account/check-email?email=...
// Checks if a customer profile exists for this store with the given email.
// Returns 200 if found, 404 if not.
func (h *CustomerAccountHandler) CheckEmail(c *gin.Context) {
	email := strings.TrimSpace(strings.ToLower(c.Query("email")))
	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": "email query parameter is required",
		})
		return
	}

	storeVal, _ := c.Get("store")
	store, _ := storeVal.(*stores.Store)
	if store == nil {
		respondNotFound(c)
		return
	}

	storeID, err := uuid.Parse(store.ID)
	if err != nil {
		respondNotFound(c)
		return
	}

	var profile customer.CustomerProfile
	result := h.db.WithContext(c.Request.Context()).
		Where("store_id = ? AND email = ?", storeID, email).
		First(&profile)

	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "not_found",
			"message": "No account found for this email",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"exists": true,
		"email":  profile.Email,
	})
}

// --- ReviewsHandler.ListMyReviews ---

// ListMyReviews handles GET /mobile/storefront/stores/:storeSlug/my-reviews.
// Reads customer_profile_id from context, queries reviews by customer, returns list.
func (h *ReviewsHandler) ListMyReviews(c *gin.Context) {
	storeVal, _ := c.Get("store")
	store, _ := storeVal.(*stores.Store)
	if store == nil {
		respondNotFound(c)
		return
	}

	profile := mustGetCustomerProfile(c)
	if profile == nil {
		return
	}

	var q listReviewsQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": err.Error(),
		})
		return
	}
	q.defaults()

	// Query reviews by customer_profile_id + store_id.
	reviews, total, err := h.reviewRepo.ListByCustomer(
		c.Request.Context(), store.ID, profile.ID.String(), q.Page, q.PageSize,
	)
	if err != nil {
		respondInternal(c, h.logger, err)
		return
	}

	out := make([]storefrontReviewResponse, 0, len(reviews))
	for i := range reviews {
		out = append(out, toStorefrontReviewResponse(&reviews[i]))
	}

	totalPages := int64(0)
	if q.PageSize > 0 {
		totalPages = (total + int64(q.PageSize) - 1) / int64(q.PageSize)
	}

	c.JSON(http.StatusOK, gin.H{
		"data": out,
		"meta": gin.H{
			"page":        q.Page,
			"page_size":   q.PageSize,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}
