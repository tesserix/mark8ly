package admin

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/coupon"
)

// ---------- Request DTOs ----------

// CreateCouponRequest is the JSON body for POST /admin/stores/:storeId/coupons.
type CreateCouponRequest struct {
	Code         string           `json:"code"          binding:"required,max=50"`
	Title        string           `json:"title"         binding:"required,max=200"`
	Description  *string          `json:"description"`
	Type         string           `json:"type"          binding:"required,oneof=percentage fixed_amount free_shipping"`
	Value        decimal.Decimal  `json:"value"         binding:"required"`
	CurrencyCode *string          `json:"currency_code"`
	MinPurchase  *decimal.Decimal `json:"min_purchase"`
	MaxDiscount  *decimal.Decimal `json:"max_discount"`
	UsageLimit   *int             `json:"usage_limit"`
	PerCustomer  int              `json:"per_customer"`
	TargetType   string           `json:"target_type"`
	TargetIDs    []string         `json:"target_ids"`
	Stackable    bool             `json:"stackable"`
	StartsAt     *time.Time       `json:"starts_at"`
	EndsAt       *time.Time       `json:"ends_at"`
}

// PatchCouponRequest is the JSON body for PATCH /admin/stores/:storeId/coupons/:id.
type PatchCouponRequest struct {
	Title       *string          `json:"title"`
	Description *string          `json:"description"`
	MinPurchase *decimal.Decimal `json:"min_purchase"`
	MaxDiscount *decimal.Decimal `json:"max_discount"`
	UsageLimit  *int             `json:"usage_limit"`
	PerCustomer *int             `json:"per_customer"`
	Stackable   *bool            `json:"stackable"`
	StartsAt    *time.Time       `json:"starts_at"`
	EndsAt      *time.Time       `json:"ends_at"`
	Status      *string          `json:"status"`
}

// ---------- Response DTOs ----------

// AdminCouponResponse is the JSON envelope for a coupon returned by admin endpoints.
type AdminCouponResponse struct {
	ID           string           `json:"id"`
	Code         string           `json:"code"`
	Title        string           `json:"title"`
	Description  *string          `json:"description"`
	Type         string           `json:"type"`
	Value        decimal.Decimal  `json:"value"`
	CurrencyCode *string          `json:"currency_code"`
	MinPurchase  *decimal.Decimal `json:"min_purchase"`
	MaxDiscount  *decimal.Decimal `json:"max_discount"`
	UsageLimit   *int             `json:"usage_limit"`
	PerCustomer  int              `json:"per_customer"`
	TargetType   string           `json:"target_type"`
	TargetIDs    []string         `json:"target_ids"`
	Stackable    bool             `json:"stackable"`
	StartsAt     string           `json:"starts_at"`
	EndsAt       *string          `json:"ends_at"`
	Status       string           `json:"status"`
	UsageCount   int              `json:"usage_count"`
	CreatedAt    string           `json:"created_at"`
	UpdatedAt    string           `json:"updated_at"`
}

// AdminCouponUsageResponse is a single coupon_usage row for the detail page.
type AdminCouponUsageResponse struct {
	ID             string          `json:"id"`
	OrderID        string          `json:"order_id"`
	CustomerEmail  string          `json:"customer_email"`
	DiscountAmount decimal.Decimal `json:"discount_amount"`
	CurrencyCode   string          `json:"currency_code"`
	CreatedAt      string          `json:"created_at"`
}

// toAdminCouponResponse maps a domain Coupon to the admin JSON response.
func toAdminCouponResponse(c *coupon.Coupon) AdminCouponResponse {
	r := AdminCouponResponse{
		ID:           c.ID.String(),
		Code:         c.Code,
		Title:        c.Title,
		Description:  c.Description,
		Type:         string(c.Type),
		Value:        c.Value,
		CurrencyCode: c.CurrencyCode,
		MinPurchase:  c.MinPurchase,
		MaxDiscount:  c.MaxDiscount,
		UsageLimit:   c.UsageLimit,
		PerCustomer:  c.PerCustomer,
		TargetType:   string(c.TargetType),
		TargetIDs:    c.TargetIDs,
		Stackable:    c.Stackable,
		StartsAt:     c.StartsAt.Format(time.RFC3339),
		Status:       string(c.Status),
		UsageCount:   c.UsageCount,
		CreatedAt:    c.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    c.UpdatedAt.Format(time.RFC3339),
	}
	if c.EndsAt != nil {
		s := c.EndsAt.Format(time.RFC3339)
		r.EndsAt = &s
	}
	if r.TargetIDs == nil {
		r.TargetIDs = []string{}
	}
	return r
}

// toAdminCouponUsageResponse maps a CouponUsage to the admin JSON response.
func toAdminCouponUsageResponse(u *coupon.CouponUsage) AdminCouponUsageResponse {
	return AdminCouponUsageResponse{
		ID:             u.ID.String(),
		OrderID:        u.OrderID.String(),
		CustomerEmail:  u.CustomerEmail,
		DiscountAmount: u.DiscountAmount,
		CurrencyCode:   u.CurrencyCode,
		CreatedAt:      u.CreatedAt.Format(time.RFC3339),
	}
}
