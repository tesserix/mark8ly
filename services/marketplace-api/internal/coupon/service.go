package coupon

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/discount"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// ServiceConfig groups dependencies for the coupon service.
type ServiceConfig struct {
	DB     *gorm.DB
	Repo   Repository
	Logger *slog.Logger
}

// Service implements coupon CRUD and validation logic.
type Service struct {
	db     *gorm.DB
	repo   Repository
	logger *slog.Logger
}

// NewService constructs a coupon Service.
func NewService(cfg ServiceConfig) *Service {
	return &Service{
		db:     cfg.DB,
		repo:   cfg.Repo,
		logger: cfg.Logger,
	}
}

// ---------- CRUD ----------

// CreateInput holds the fields for creating a coupon.
type CreateInput struct {
	TenantID     uuid.UUID
	StoreID      uuid.UUID
	Code         string
	Title        string
	Description  *string
	Type         string
	Value        decimal.Decimal
	CurrencyCode *string
	MinPurchase  *decimal.Decimal
	MaxDiscount  *decimal.Decimal
	UsageLimit   *int
	PerCustomer  int
	TargetType   string
	TargetIDs    []string
	Stackable    bool
	StartsAt     *time.Time
	EndsAt       *time.Time
}

// Create validates and persists a new coupon.
func (s *Service) Create(ctx context.Context, in CreateInput) (*Coupon, error) {
	if err := s.validateCreateInput(in); err != nil {
		return nil, err
	}

	now := time.Now()
	startsAt := now
	if in.StartsAt != nil {
		startsAt = *in.StartsAt
	}

	perCustomer := in.PerCustomer
	if perCustomer < 1 {
		perCustomer = 1
	}

	targetType := CouponTargetAll
	if in.TargetType != "" {
		targetType = CouponTargetType(in.TargetType)
	}

	c := &Coupon{
		TenantID:     in.TenantID,
		StoreID:      in.StoreID,
		Code:         strings.ToUpper(strings.TrimSpace(in.Code)),
		Title:        strings.TrimSpace(in.Title),
		Description:  in.Description,
		Type:         CouponType(in.Type),
		Value:        in.Value,
		CurrencyCode: in.CurrencyCode,
		MinPurchase:  in.MinPurchase,
		MaxDiscount:  in.MaxDiscount,
		UsageLimit:   in.UsageLimit,
		PerCustomer:  perCustomer,
		TargetType:   targetType,
		TargetIDs:    in.TargetIDs,
		Stackable:    in.Stackable,
		StartsAt:     startsAt,
		EndsAt:       in.EndsAt,
		Status:       CouponStatusActive,
		UsageCount:   0,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.repo.Create(ctx, s.db, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Service) validateCreateInput(in CreateInput) error {
	if strings.TrimSpace(in.Code) == "" {
		return apperrors.ValidationFailed("code", "code is required")
	}
	if len(in.Code) > 50 {
		return apperrors.ValidationFailed("code", "code must be 50 characters or fewer")
	}
	if strings.TrimSpace(in.Title) == "" {
		return apperrors.ValidationFailed("title", "title is required")
	}
	if !ValidateType(in.Type) {
		return apperrors.ValidationFailed("type", "type must be percentage, fixed_amount, or free_shipping")
	}
	if in.Value.IsNegative() {
		return apperrors.ValidationFailed("value", "value must be non-negative")
	}
	if CouponType(in.Type) == CouponTypePercentage && in.Value.GreaterThan(decimal.NewFromInt(100)) {
		return apperrors.ValidationFailed("value", "percentage value must be between 0 and 100")
	}
	if CouponType(in.Type) == CouponTypeFixedAmount && (in.CurrencyCode == nil || *in.CurrencyCode == "") {
		return apperrors.ValidationFailed("currency_code", "currency_code is required for fixed_amount coupons")
	}
	if in.EndsAt != nil && in.StartsAt != nil && in.EndsAt.Before(*in.StartsAt) {
		return apperrors.ValidationFailed("ends_at", "ends_at must be after starts_at")
	}
	return nil
}

// PatchInput holds the fields for updating a coupon. Nil fields are not updated.
type PatchInput struct {
	Title       *string
	Description *string
	MinPurchase *decimal.Decimal
	MaxDiscount *decimal.Decimal
	UsageLimit  *int
	PerCustomer *int
	Stackable   *bool
	StartsAt    *time.Time
	EndsAt      *time.Time
	Status      *string
}

// Patch updates mutable fields on an existing coupon.
func (s *Service) Patch(ctx context.Context, storeID, id uuid.UUID, in PatchInput) (*Coupon, error) {
	// Amendment FIX 5: validate status field — only "active" and "disabled"
	// are valid patch targets. "expired" is system-managed.
	if in.Status != nil {
		switch CouponStatus(*in.Status) {
		case CouponStatusActive, CouponStatusDisabled:
			// ok
		default:
			return nil, apperrors.ValidationFailed("status", "status must be 'active' or 'disabled'")
		}
	}

	c, err := s.repo.GetByID(ctx, s.db, storeID, id)
	if err != nil {
		return nil, err
	}

	if in.Title != nil {
		c.Title = strings.TrimSpace(*in.Title)
	}
	if in.Description != nil {
		c.Description = in.Description
	}
	if in.MinPurchase != nil {
		c.MinPurchase = in.MinPurchase
	}
	if in.MaxDiscount != nil {
		c.MaxDiscount = in.MaxDiscount
	}
	if in.UsageLimit != nil {
		c.UsageLimit = in.UsageLimit
	}
	if in.PerCustomer != nil {
		c.PerCustomer = *in.PerCustomer
	}
	if in.Stackable != nil {
		c.Stackable = *in.Stackable
	}
	if in.StartsAt != nil {
		c.StartsAt = *in.StartsAt
	}
	if in.EndsAt != nil {
		c.EndsAt = in.EndsAt
	}
	if in.Status != nil {
		c.Status = CouponStatus(*in.Status)
	}
	c.UpdatedAt = time.Now()

	if err := s.repo.Update(ctx, s.db, c); err != nil {
		return nil, err
	}
	return c, nil
}

// Get returns a single coupon by ID with store scope.
func (s *Service) Get(ctx context.Context, storeID, id uuid.UUID) (*Coupon, error) {
	return s.repo.GetByID(ctx, s.db, storeID, id)
}

// List returns a paginated list of coupons for a store.
func (s *Service) List(ctx context.Context, f ListFilter) (ListResult, error) {
	return s.repo.List(ctx, s.db, f)
}

// Delete soft-disables a coupon.
func (s *Service) Delete(ctx context.Context, storeID, id uuid.UUID) error {
	return s.repo.SoftDisable(ctx, s.db, storeID, id)
}

// ---------- Validation (storefront) ----------

// ValidateResult is the discount preview returned by Validate.
type ValidateResult struct {
	CouponID       uuid.UUID       `json:"coupon_id"`
	Code           string          `json:"code"`
	Type           CouponType      `json:"type"`
	Value          decimal.Decimal `json:"value"`
	DiscountAmount decimal.Decimal `json:"discount_amount"`
	FreeShipping   bool            `json:"free_shipping"`
	Title          string          `json:"title"`
}

// ValidateInput holds the parameters for storefront coupon validation.
type ValidateInput struct {
	TenantID      uuid.UUID
	StoreID       uuid.UUID
	Code          string
	CustomerEmail string
	Subtotal      decimal.Decimal
}

// Validate checks if a coupon code is valid for the given context and
// returns a discount preview. Does NOT apply or increment usage.
func (s *Service) Validate(ctx context.Context, in ValidateInput) (*ValidateResult, error) {
	c, err := s.repo.GetByCode(ctx, s.db, in.StoreID, in.Code)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	if !c.IsActive(now) {
		if c.EndsAt != nil && now.After(*c.EndsAt) {
			return nil, apperrors.CouponExpired(c.Code)
		}
		return nil, apperrors.CouponInvalid("coupon is not currently active")
	}

	if !c.HasUsageCapacity() {
		limit := 0
		if c.UsageLimit != nil {
			limit = *c.UsageLimit
		}
		return nil, apperrors.CouponUsageLimitReached(c.Code, limit)
	}

	// Per-customer check (amendment FIX 3: include tenant_id).
	if in.CustomerEmail != "" {
		count, err := s.repo.CountCustomerUsage(ctx, s.db, in.TenantID, c.ID, in.CustomerEmail)
		if err != nil {
			return nil, fmt.Errorf("coupon validate: %w", err)
		}
		if count >= int64(c.PerCustomer) {
			return nil, apperrors.CouponUsageLimitReached(c.Code, c.PerCustomer)
		}
	}

	// Min purchase check.
	if c.MinPurchase != nil && in.Subtotal.LessThan(*c.MinPurchase) {
		return nil, apperrors.CouponMinPurchaseNotMet(c.Code, c.MinPurchase.StringFixed(2), in.Subtotal.StringFixed(2))
	}

	discountAmount := c.CalculateDiscount(in.Subtotal)

	return &ValidateResult{
		CouponID:       c.ID,
		Code:           c.Code,
		Type:           c.Type,
		Value:          c.Value,
		DiscountAmount: discountAmount,
		FreeShipping:   c.Type == CouponTypeFreeShipping,
		Title:          c.Title,
	}, nil
}

// ---------- Checkout apply (discount.Applier) ----------

// CouponApplier implements discount.Applier for coupon discounts.
// Created by the checkout handler with the validated coupon code.
type CouponApplier struct {
	svc           *Service
	code          string
	customerEmail string
}

// NewCouponApplier creates an Applier that will validate and apply the
// given coupon code during checkout.
func NewCouponApplier(svc *Service, code, customerEmail string) *CouponApplier {
	return &CouponApplier{svc: svc, code: code, customerEmail: customerEmail}
}

// Apply implements discount.Applier. It validates the coupon, atomically
// increments usage_count, records a coupon_usage row, and returns the
// discount amount — all inside the caller's transaction.
//
// Amendment CRITICAL FIX 1+2: validate + apply atomically inside the
// caller's transaction. No separate Validate() call before this.
func (a *CouponApplier) Apply(ctx context.Context, tx *gorm.DB, in discount.ApplyInput) (discount.ApplyResult, error) {
	zero := discount.ApplyResult{}

	// Look up coupon inside the transaction to get a consistent snapshot.
	c, err := a.svc.repo.GetByCode(ctx, tx, in.StoreID, a.code)
	if err != nil {
		return zero, err
	}

	now := time.Now()
	if !c.IsActive(now) {
		if c.EndsAt != nil && now.After(*c.EndsAt) {
			return zero, apperrors.CouponExpired(c.Code)
		}
		return zero, apperrors.CouponInvalid("coupon is not currently active")
	}

	// Min purchase.
	if c.MinPurchase != nil && in.Subtotal.LessThan(*c.MinPurchase) {
		return zero, apperrors.CouponMinPurchaseNotMet(c.Code, c.MinPurchase.StringFixed(2), in.Subtotal.StringFixed(2))
	}

	// Per-customer check (amendment FIX 3: include tenant_id).
	if a.customerEmail != "" {
		count, err := a.svc.repo.CountCustomerUsage(ctx, tx, in.TenantID, c.ID, a.customerEmail)
		if err != nil {
			return zero, fmt.Errorf("coupon apply: %w", err)
		}
		if count >= int64(c.PerCustomer) {
			return zero, apperrors.CouponUsageLimitReached(c.Code, c.PerCustomer)
		}
	}

	// Atomic usage increment.
	if err := a.svc.repo.IncrementUsageInTx(tx, c.ID); err != nil {
		return zero, err
	}

	discountAmount := c.CalculateDiscount(in.Subtotal)

	// Record usage.
	usage := &CouponUsage{
		TenantID:       in.TenantID,
		CouponID:       c.ID,
		OrderID:        in.OrderID,
		CustomerEmail:  a.customerEmail,
		DiscountAmount: discountAmount,
		CurrencyCode:   in.CurrencyCode,
	}
	if err := a.svc.repo.RecordUsage(tx, usage); err != nil {
		return zero, err
	}

	desc := fmt.Sprintf("%s — %s", c.Code, c.Title)

	return discount.ApplyResult{
		DiscountAmount: discountAmount,
		Description:    desc,
	}, nil
}

// Unit runs fn inside a GORM transaction. Exposed so handlers can use
// the service's DB connection for transactional work.
func (s *Service) Unit(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return s.db.WithContext(ctx).Transaction(fn)
}

// ListUsage returns usage records for a coupon, scoped to tenant.
func (s *Service) ListUsage(ctx context.Context, tenantID, couponID uuid.UUID, page, perPage int) ([]CouponUsage, int64, error) {
	return s.repo.ListUsage(ctx, s.db, tenantID, couponID, page, perPage)
}
