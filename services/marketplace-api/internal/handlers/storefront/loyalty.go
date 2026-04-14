package storefront

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/loyalty"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// LoyaltyHandler bundles dependencies for storefront loyalty endpoints.
type LoyaltyHandler struct {
	svc    *loyalty.Service
	logger *slog.Logger
}

// NewLoyaltyHandler constructs a storefront LoyaltyHandler.
func NewLoyaltyHandler(svc *loyalty.Service, logger *slog.Logger) *LoyaltyHandler {
	return &LoyaltyHandler{svc: svc, logger: logger}
}

// --- Request/Response DTOs ---

type sfEnrollRequest struct {
	Email        string  `json:"email"         binding:"required,email"`
	Name         *string `json:"name"`
	ReferralCode *string `json:"referral_code"`
}

type sfRedeemRequest struct {
	Email  string `json:"email"  binding:"required,email"`
	Points int    `json:"points" binding:"required,min=1"`
}

type sfProgramResponse struct {
	IsActive        bool             `json:"is_active"`
	PointsPerUnit   decimal.Decimal  `json:"points_per_unit"`
	PointsCurrency  string           `json:"points_currency"`
	SignupBonus     int              `json:"signup_bonus"`
	ReferralBonus   int              `json:"referral_bonus"`
	RefereeBonus    int              `json:"referee_bonus"`
	MinRedeemPoints int              `json:"min_redeem_points"`
	PointsValue     decimal.Decimal  `json:"points_value"`
	Tiers           []sfTierResponse `json:"tiers"`
}

type sfTierResponse struct {
	Name       string          `json:"name"`
	MinPoints  int             `json:"min_points"`
	Multiplier decimal.Decimal `json:"multiplier"`
}

type sfCustomerResponse struct {
	PointsBalance  int    `json:"points_balance"`
	LifetimePoints int    `json:"lifetime_points"`
	Tier           string `json:"tier"`
	ReferralCode   string `json:"referral_code"`
}

type sfRedeemResponse struct {
	PointsRedeemed int             `json:"points_redeemed"`
	Value          decimal.Decimal `json:"value"`
}

// --- Handlers ---

// GetProgram handles GET /storefront/stores/:storeSlug/loyalty/program.
func (h *LoyaltyHandler) GetProgram(c *gin.Context) {
	store := h.resolveStore(c)
	if store == nil {
		return
	}
	storeID, _ := uuid.Parse(store.ID)

	program, err := h.svc.GetProgram(c.Request.Context(), storeID)
	if err != nil {
		loyaltyRespondInternal(c, h.logger, err)
		return
	}
	if program == nil {
		c.JSON(http.StatusOK, gin.H{"data": nil})
		return
	}

	var tiers []loyalty.Tier
	_ = json.Unmarshal(program.Tiers, &tiers)

	tierResps := make([]sfTierResponse, 0, len(tiers))
	for _, t := range tiers {
		tierResps = append(tierResps, sfTierResponse{
			Name:       t.Name,
			MinPoints:  t.MinPoints,
			Multiplier: t.Multiplier,
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": sfProgramResponse{
		IsActive:        program.IsActive,
		PointsPerUnit:   program.PointsPerUnit,
		PointsCurrency:  program.PointsCurrency,
		SignupBonus:     program.SignupBonus,
		ReferralBonus:   program.ReferralBonus,
		RefereeBonus:    program.RefereeBonus,
		MinRedeemPoints: program.MinRedeemPoints,
		PointsValue:     program.PointsValue,
		Tiers:           tierResps,
	}})
}

// Enroll handles POST /storefront/stores/:storeSlug/loyalty/enroll.
func (h *LoyaltyHandler) Enroll(c *gin.Context) {
	store := h.resolveStore(c)
	if store == nil {
		return
	}
	storeID, _ := uuid.Parse(store.ID)
	tenantID, _ := uuid.Parse(store.TenantID)

	var req sfEnrollRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest,
			gin.H{"error": "validation_failed", "message": err.Error()})
		return
	}

	customer, err := h.svc.Enroll(c.Request.Context(), loyalty.EnrollRequest{
		TenantID:      tenantID,
		StoreID:       storeID,
		CustomerEmail: req.Email,
		CustomerName:  req.Name,
		ReferralCode:  req.ReferralCode,
	})
	if err != nil {
		loyaltyRespondAppError(c, h.logger, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": sfCustomerResponse{
		PointsBalance:  customer.PointsBalance,
		LifetimePoints: customer.LifetimePoints,
		Tier:           customer.Tier,
		ReferralCode:   customer.ReferralCode,
	}})
}

// GetMe handles GET /storefront/stores/:storeSlug/loyalty/me?email=.
func (h *LoyaltyHandler) GetMe(c *gin.Context) {
	store := h.resolveStore(c)
	if store == nil {
		return
	}
	storeID, _ := uuid.Parse(store.ID)

	email := c.Query("email")
	if email == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest,
			gin.H{"error": "validation_failed", "message": "email query parameter is required"})
		return
	}

	customer, err := h.svc.GetCustomer(c.Request.Context(), storeID, email)
	if err != nil {
		loyaltyRespondInternal(c, h.logger, err)
		return
	}
	if customer == nil {
		c.JSON(http.StatusOK, gin.H{"data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": sfCustomerResponse{
		PointsBalance:  customer.PointsBalance,
		LifetimePoints: customer.LifetimePoints,
		Tier:           customer.Tier,
		ReferralCode:   customer.ReferralCode,
	}})
}

// Redeem handles POST /storefront/stores/:storeSlug/loyalty/redeem.
func (h *LoyaltyHandler) Redeem(c *gin.Context) {
	store := h.resolveStore(c)
	if store == nil {
		return
	}
	storeID, _ := uuid.Parse(store.ID)
	tenantID, _ := uuid.Parse(store.TenantID)

	var req sfRedeemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest,
			gin.H{"error": "validation_failed", "message": err.Error()})
		return
	}

	value, err := h.svc.RedeemPoints(c.Request.Context(), tenantID, storeID, req.Email, req.Points, nil)
	if err != nil {
		loyaltyRespondAppError(c, h.logger, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": sfRedeemResponse{
		PointsRedeemed: req.Points,
		Value:          value,
	}})
}

// --- Helpers ---

func (h *LoyaltyHandler) resolveStore(c *gin.Context) *stores.Store {
	storeVal, ok := c.Get("store")
	if !ok {
		respondNotFound(c)
		return nil
	}
	store, ok := storeVal.(*stores.Store)
	if !ok || store == nil {
		respondNotFound(c)
		return nil
	}
	return store
}

func loyaltyRespondAppError(c *gin.Context, logger *slog.Logger, err error) {
	var ae *apperrors.Error
	if errors.As(err, &ae) {
		status := http.StatusBadRequest
		switch ae.Code {
		case apperrors.CodeInsufficientLoyaltyPoints:
			status = http.StatusUnprocessableEntity
		case apperrors.CodeLoyaltyNotEnrolled:
			status = http.StatusBadRequest
		case apperrors.CodeNotFound:
			status = http.StatusNotFound
		case apperrors.CodeValidationFailed:
			status = http.StatusBadRequest
		}
		c.AbortWithStatusJSON(status, gin.H{"error": string(ae.Code), "message": ae.Message})
		return
	}
	loyaltyRespondInternal(c, logger, err)
}

func loyaltyRespondInternal(c *gin.Context, logger *slog.Logger, err error) {
	if logger != nil {
		logger.Error("storefront loyalty handler error", "err", err)
	}
	c.AbortWithStatusJSON(http.StatusInternalServerError,
		gin.H{"error": "internal", "message": "internal server error"})
}
