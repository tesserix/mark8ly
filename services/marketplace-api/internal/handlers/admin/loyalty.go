package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/loyalty"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// LoyaltyHandler bundles dependencies for admin loyalty endpoints.
type LoyaltyHandler struct {
	svc    *loyalty.Service
	logger *slog.Logger
}

// NewLoyaltyHandler constructs a LoyaltyHandler.
func NewLoyaltyHandler(svc *loyalty.Service, logger *slog.Logger) *LoyaltyHandler {
	return &LoyaltyHandler{svc: svc, logger: logger}
}

// GetProgram handles GET /admin/stores/:storeId/loyalty/program.
func (h *LoyaltyHandler) GetProgram(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid UUID"), h.logger)
		return
	}

	program, err := h.svc.GetProgram(c.Request.Context(), storeID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	if program == nil {
		c.JSON(http.StatusOK, gin.H{"data": nil})
		return
	}

	var tiers []loyalty.Tier
	_ = json.Unmarshal(program.Tiers, &tiers)

	c.JSON(http.StatusOK, gin.H{"data": toLoyaltyProgramResponse(program, tiers)})
}

// UpdateProgram handles PUT /admin/stores/:storeId/loyalty/program.
func (h *LoyaltyHandler) UpdateProgram(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid UUID"), h.logger)
		return
	}
	tenantID := c.GetString("tenant_id")
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("tenant_id", "invalid UUID"), h.logger)
		return
	}

	var req UpdateLoyaltyProgramRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	// Convert tier requests to domain tiers
	tiers := make([]loyalty.Tier, 0, len(req.Tiers))
	for _, t := range req.Tiers {
		tiers = append(tiers, loyalty.Tier{
			Name:       t.Name,
			MinPoints:  t.MinPoints,
			Multiplier: t.Multiplier,
		})
	}

	program, err := h.svc.UpdateProgram(c.Request.Context(), loyalty.UpdateProgramRequest{
		TenantID:        tenantUUID,
		StoreID:         storeID,
		IsActive:        req.IsActive,
		PointsPerDollar: req.PointsPerDollar,
		PointsCurrency:  req.PointsCurrency,
		SignupBonus:     req.SignupBonus,
		ReferralBonus:   req.ReferralBonus,
		RefereeBonus:    req.RefereeBonus,
		PointExpiryDays: req.PointExpiryDays,
		MinRedeemPoints: req.MinRedeemPoints,
		PointsValue:     req.PointsValue,
		Tiers:           tiers,
	})
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	var parsedTiers []loyalty.Tier
	_ = json.Unmarshal(program.Tiers, &parsedTiers)

	c.JSON(http.StatusOK, gin.H{"data": toLoyaltyProgramResponse(program, parsedTiers)})
}

// ListMembers handles GET /admin/stores/:storeId/loyalty/members.
func (h *LoyaltyHandler) ListMembers(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid UUID"), h.logger)
		return
	}

	page, limit := loyaltyParsePagination(c)
	members, total, err := h.svc.ListMembers(c.Request.Context(), storeID, page, limit)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	out := make([]LoyaltyMemberResponse, 0, len(members))
	for i := range members {
		out = append(out, toLoyaltyMemberResponse(&members[i]))
	}
	c.JSON(http.StatusOK, gin.H{
		"data": out,
		"meta": gin.H{"total": total, "page": page, "limit": limit},
	})
}

// GetMember handles GET /admin/stores/:storeId/loyalty/members/:id.
func (h *LoyaltyHandler) GetMember(c *gin.Context) {
	memberID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid UUID"), h.logger)
		return
	}

	member, err := h.svc.GetCustomerByID(c.Request.Context(), memberID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	page, limit := loyaltyParsePagination(c)
	txns, txnTotal, err := h.svc.ListTransactions(c.Request.Context(), memberID, page, limit)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	txnOut := make([]LoyaltyTransactionResponse, 0, len(txns))
	for i := range txns {
		txnOut = append(txnOut, toLoyaltyTransactionResponse(&txns[i]))
	}

	c.JSON(http.StatusOK, gin.H{
		"data": toLoyaltyMemberResponse(member),
		"transactions": gin.H{
			"data": txnOut,
			"meta": gin.H{"total": txnTotal, "page": page, "limit": limit},
		},
	})
}

// AdjustPoints handles POST /admin/stores/:storeId/loyalty/members/:id/adjust.
func (h *LoyaltyHandler) AdjustPoints(c *gin.Context) {
	memberID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "invalid UUID"), h.logger)
		return
	}
	tenantID := c.GetString("tenant_id")
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("tenant_id", "invalid UUID"), h.logger)
		return
	}
	userEmail := c.GetString("user_email")
	if userEmail == "" {
		userEmail = c.GetString("user_id") // fallback
	}

	var req AdjustPointsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}
	if req.Points == 0 {
		RespondErr(c, apperrors.ValidationFailed("points", "points must be non-zero"), h.logger)
		return
	}

	if err := h.svc.AdjustPoints(c.Request.Context(), tenantUUID, memberID, req.Points, req.Description, userEmail); err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "points adjusted"})
}

// ListReferrals handles GET /admin/stores/:storeId/loyalty/referrals.
func (h *LoyaltyHandler) ListReferrals(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid UUID"), h.logger)
		return
	}

	page, limit := loyaltyParsePagination(c)
	refs, total, err := h.svc.ListReferrals(c.Request.Context(), storeID, page, limit)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	out := make([]ReferralResponse, 0, len(refs))
	for i := range refs {
		out = append(out, toReferralResponse(&refs[i]))
	}
	c.JSON(http.StatusOK, gin.H{
		"data": out,
		"meta": gin.H{"total": total, "page": page, "limit": limit},
	})
}

// loyaltyParsePagination extracts page/limit from query params with defaults.
func loyaltyParsePagination(c *gin.Context) (int, int) {
	page := 1
	limit := 20
	if p, err := strconv.Atoi(c.DefaultQuery("page", "1")); err == nil && p > 0 {
		page = p
	}
	if l, err := strconv.Atoi(c.DefaultQuery("limit", "20")); err == nil && l > 0 && l <= 100 {
		limit = l
	}
	return page, limit
}
