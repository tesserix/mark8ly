package admin

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/loyalty"
)

// --- Request DTOs ---

type UpdateLoyaltyProgramRequest struct {
	IsActive        bool            `json:"is_active"`
	PointsPerDollar decimal.Decimal `json:"points_per_dollar" binding:"required"`
	PointsCurrency  string          `json:"points_currency"   binding:"required"`
	SignupBonus     int             `json:"signup_bonus"`
	ReferralBonus   int             `json:"referral_bonus"`
	RefereeBonus    int             `json:"referee_bonus"`
	PointExpiryDays *int            `json:"point_expiry_days"`
	MinRedeemPoints int             `json:"min_redeem_points" binding:"required"`
	PointsValue     decimal.Decimal `json:"points_value"      binding:"required"`
	Tiers           []TierRequest   `json:"tiers"             binding:"dive"`
}

type TierRequest struct {
	Name       string          `json:"name"       binding:"required"`
	MinPoints  int             `json:"min_points"`
	Multiplier decimal.Decimal `json:"multiplier" binding:"required"`
}

type AdjustPointsRequest struct {
	Points      int    `json:"points"      binding:"required"`
	Description string `json:"description" binding:"required"`
}

// --- Response DTOs ---

type LoyaltyProgramResponse struct {
	ID              string          `json:"id"`
	IsActive        bool            `json:"is_active"`
	PointsPerDollar decimal.Decimal `json:"points_per_dollar"`
	PointsCurrency  string          `json:"points_currency"`
	SignupBonus     int             `json:"signup_bonus"`
	ReferralBonus   int             `json:"referral_bonus"`
	RefereeBonus    int             `json:"referee_bonus"`
	PointExpiryDays *int            `json:"point_expiry_days"`
	MinRedeemPoints int             `json:"min_redeem_points"`
	PointsValue     decimal.Decimal `json:"points_value"`
	Tiers           []TierResponse  `json:"tiers"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type TierResponse struct {
	Name       string          `json:"name"`
	MinPoints  int             `json:"min_points"`
	Multiplier decimal.Decimal `json:"multiplier"`
}

type LoyaltyMemberResponse struct {
	ID             string    `json:"id"`
	CustomerEmail  string    `json:"customer_email"`
	CustomerName   *string   `json:"customer_name,omitempty"`
	PointsBalance  int       `json:"points_balance"`
	LifetimePoints int       `json:"lifetime_points"`
	Tier           string    `json:"tier"`
	ReferralCode   string    `json:"referral_code"`
	EnrolledAt     time.Time `json:"enrolled_at"`
}

type LoyaltyTransactionResponse struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	Points       int       `json:"points"`
	BalanceAfter int       `json:"balance_after"`
	Description  *string   `json:"description,omitempty"`
	AdjustedBy   *string   `json:"adjusted_by,omitempty"`
	OrderID      *string   `json:"order_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type ReferralResponse struct {
	ID            string     `json:"id"`
	ReferrerID    string     `json:"referrer_id"`
	RefereeID     string     `json:"referee_id"`
	Status        string     `json:"status"`
	ReferrerBonus int        `json:"referrer_bonus"`
	RefereeBonus  int        `json:"referee_bonus"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// --- Converters ---

func toLoyaltyProgramResponse(p *loyalty.LoyaltyProgram, tiers []loyalty.Tier) LoyaltyProgramResponse {
	tierResps := make([]TierResponse, 0, len(tiers))
	for _, t := range tiers {
		tierResps = append(tierResps, TierResponse{
			Name:       t.Name,
			MinPoints:  t.MinPoints,
			Multiplier: t.Multiplier,
		})
	}
	return LoyaltyProgramResponse{
		ID:              p.ID.String(),
		IsActive:        p.IsActive,
		PointsPerDollar: p.PointsPerDollar,
		PointsCurrency:  p.PointsCurrency,
		SignupBonus:     p.SignupBonus,
		ReferralBonus:   p.ReferralBonus,
		RefereeBonus:    p.RefereeBonus,
		PointExpiryDays: p.PointExpiryDays,
		MinRedeemPoints: p.MinRedeemPoints,
		PointsValue:     p.PointsValue,
		Tiers:           tierResps,
		CreatedAt:       p.CreatedAt,
		UpdatedAt:       p.UpdatedAt,
	}
}

func toLoyaltyMemberResponse(c *loyalty.CustomerLoyalty) LoyaltyMemberResponse {
	return LoyaltyMemberResponse{
		ID:             c.ID.String(),
		CustomerEmail:  c.CustomerEmail,
		CustomerName:   c.CustomerName,
		PointsBalance:  c.PointsBalance,
		LifetimePoints: c.LifetimePoints,
		Tier:           c.Tier,
		ReferralCode:   c.ReferralCode,
		EnrolledAt:     c.EnrolledAt,
	}
}

func toLoyaltyTransactionResponse(t *loyalty.LoyaltyTransaction) LoyaltyTransactionResponse {
	resp := LoyaltyTransactionResponse{
		ID:           t.ID.String(),
		Type:         string(t.Type),
		Points:       t.Points,
		BalanceAfter: t.BalanceAfter,
		Description:  t.Description,
		AdjustedBy:   t.AdjustedBy,
		CreatedAt:    t.CreatedAt,
	}
	if t.OrderID != nil {
		s := t.OrderID.String()
		resp.OrderID = &s
	}
	return resp
}

func toReferralResponse(r *loyalty.Referral) ReferralResponse {
	return ReferralResponse{
		ID:            r.ID.String(),
		ReferrerID:    r.ReferrerID.String(),
		RefereeID:     r.RefereeID.String(),
		Status:        string(r.Status),
		ReferrerBonus: r.ReferrerBonus,
		RefereeBonus:  r.RefereeBonus,
		CompletedAt:   r.CompletedAt,
		CreatedAt:     r.CreatedAt,
	}
}
