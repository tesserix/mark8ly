package loyalty

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Service is the loyalty business-logic layer.
type Service struct {
	db     *gorm.DB
	repo   Repository
	logger *slog.Logger
}

// NewService constructs a Service.
func NewService(db *gorm.DB, repo Repository, logger *slog.Logger) *Service {
	return &Service{db: db, repo: repo, logger: logger}
}

// --- Program Config ---

// GetProgram returns the store's loyalty program, or nil if not configured.
func (s *Service) GetProgram(ctx context.Context, storeID uuid.UUID) (*LoyaltyProgram, error) {
	return s.repo.GetProgram(ctx, s.db, storeID)
}

// UpdateProgramRequest is the validated input for updating a loyalty program.
type UpdateProgramRequest struct {
	TenantID        uuid.UUID
	StoreID         uuid.UUID
	IsActive        bool
	PointsPerUnit   decimal.Decimal
	PointsCurrency  string
	SignupBonus     int
	ReferralBonus   int
	RefereeBonus    int
	PointExpiryDays *int
	MinRedeemPoints int
	PointsValue     decimal.Decimal
	Tiers           []Tier
}

// UpdateProgram upserts the loyalty program config. Validates tiers
// before saving.
func (s *Service) UpdateProgram(ctx context.Context, req UpdateProgramRequest) (*LoyaltyProgram, error) {
	if err := validateTiers(req.Tiers); err != nil {
		return nil, err
	}

	tiersJSON, err := json.Marshal(req.Tiers)
	if err != nil {
		return nil, fmt.Errorf("marshal tiers: %w", err)
	}

	existing, err := s.repo.GetProgram(ctx, s.db, req.StoreID)
	if err != nil {
		return nil, err
	}

	program := &LoyaltyProgram{
		TenantID:        req.TenantID,
		StoreID:         req.StoreID,
		IsActive:        req.IsActive,
		PointsPerUnit:   req.PointsPerUnit,
		PointsCurrency:  req.PointsCurrency,
		SignupBonus:     req.SignupBonus,
		ReferralBonus:   req.ReferralBonus,
		RefereeBonus:    req.RefereeBonus,
		PointExpiryDays: req.PointExpiryDays,
		MinRedeemPoints: req.MinRedeemPoints,
		PointsValue:     req.PointsValue,
		Tiers:           tiersJSON,
		UpdatedAt:       time.Now(),
	}
	if existing != nil {
		program.ID = existing.ID
		program.CreatedAt = existing.CreatedAt
	} else {
		program.CreatedAt = time.Now()
	}

	if err := s.repo.UpsertProgram(s.db, program); err != nil {
		return nil, err
	}
	return program, nil
}

// validateTiers checks that tiers are well-formed: max 4, unique names,
// ascending min_points, positive multipliers.
func validateTiers(tiers []Tier) error {
	if len(tiers) > 4 {
		return apperrors.ValidationFailed("tiers", "maximum 4 tiers allowed")
	}
	names := make(map[string]bool, len(tiers))
	for i, t := range tiers {
		if t.Name == "" {
			return apperrors.ValidationFailed("tiers", fmt.Sprintf("tier %d: name is required", i))
		}
		if names[t.Name] {
			return apperrors.ValidationFailed("tiers", fmt.Sprintf("tier %d: duplicate name %q", i, t.Name))
		}
		names[t.Name] = true
		if t.MinPoints < 0 {
			return apperrors.ValidationFailed("tiers", fmt.Sprintf("tier %d: min_points must be >= 0", i))
		}
		if t.Multiplier.LessThanOrEqual(decimal.Zero) {
			return apperrors.ValidationFailed("tiers", fmt.Sprintf("tier %d: multiplier must be > 0", i))
		}
	}
	// Check ascending min_points
	for i := 1; i < len(tiers); i++ {
		if tiers[i].MinPoints <= tiers[i-1].MinPoints {
			return apperrors.ValidationFailed("tiers", "tiers must have strictly ascending min_points")
		}
	}
	return nil
}

// --- Enrollment ---

// EnrollRequest is the input for enrolling a customer.
type EnrollRequest struct {
	TenantID      uuid.UUID
	StoreID       uuid.UUID
	CustomerEmail string
	CustomerName  *string
	ReferralCode  *string // optional — code of the person who referred them
}

// Enroll registers a customer in the loyalty program. If the customer
// is already enrolled, returns the existing record. Awards signup_bonus
// if configured. Handles referral linkage.
func (s *Service) Enroll(ctx context.Context, req EnrollRequest) (*CustomerLoyalty, error) {
	// Check program exists and is active
	program, err := s.repo.GetProgram(ctx, s.db, req.StoreID)
	if err != nil {
		return nil, err
	}
	if program == nil || !program.IsActive {
		return nil, apperrors.New(apperrors.CodeLoyaltyNotEnrolled, "loyalty program is not active for this store")
	}

	// Check if already enrolled
	existing, err := s.repo.GetCustomerByEmail(ctx, s.db, req.StoreID, req.CustomerEmail)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil // already enrolled — idempotent
	}

	// Generate referral code
	code, err := GenerateReferralCode()
	if err != nil {
		return nil, err
	}

	// Resolve referrer if referral code provided
	var referredBy *uuid.UUID
	var referrer *CustomerLoyalty
	if req.ReferralCode != nil && *req.ReferralCode != "" {
		referrer, err = s.repo.GetCustomerByReferralCode(ctx, s.db, req.StoreID, *req.ReferralCode)
		if err != nil {
			return nil, err
		}
		if referrer != nil {
			// Amendment FIX 2: self-referral prevention
			if referrer.CustomerEmail == req.CustomerEmail {
				return nil, apperrors.ValidationFailed("referral_code", "cannot use your own referral code")
			}
			referredBy = &referrer.ID
		}
	}

	customer := &CustomerLoyalty{
		TenantID:      req.TenantID,
		StoreID:       req.StoreID,
		CustomerEmail: req.CustomerEmail,
		CustomerName:  req.CustomerName,
		PointsBalance: 0,
		Tier:          "bronze",
		ReferralCode:  code,
		ReferredBy:    referredBy,
		EnrolledAt:    time.Now(),
	}

	// Transaction: create customer + signup bonus + referral
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.repo.CreateCustomer(tx, customer); err != nil {
			return err
		}

		// Signup bonus
		if program.SignupBonus > 0 {
			newBalance, err := s.repo.CreditPoints(tx, customer.ID, program.SignupBonus)
			if err != nil {
				return err
			}
			desc := "Signup bonus"
			if err := s.repo.CreateTransaction(tx, &LoyaltyTransaction{
				TenantID:     req.TenantID,
				LoyaltyID:    customer.ID,
				Type:         TxTypeSignup,
				Points:       program.SignupBonus,
				BalanceAfter: newBalance,
				Description:  &desc,
				CreatedAt:    time.Now(),
			}); err != nil {
				return err
			}
			customer.PointsBalance = newBalance
			customer.LifetimePoints = program.SignupBonus
		}

		// Referral tracking
		if referrer != nil {
			referral := &Referral{
				TenantID:      req.TenantID,
				StoreID:       req.StoreID,
				ReferrerID:    referrer.ID,
				RefereeID:     customer.ID,
				Status:        ReferralStatusPending,
				ReferrerBonus: program.ReferralBonus,
				RefereeBonus:  program.RefereeBonus,
				CreatedAt:     time.Now(),
			}
			if err := s.repo.CreateReferral(tx, referral); err != nil {
				return err
			}

			// Award referee bonus immediately
			if program.RefereeBonus > 0 {
				newBal, err := s.repo.CreditPoints(tx, customer.ID, program.RefereeBonus)
				if err != nil {
					return err
				}
				desc := "Referral bonus (new member)"
				if err := s.repo.CreateTransaction(tx, &LoyaltyTransaction{
					TenantID:     req.TenantID,
					LoyaltyID:    customer.ID,
					Type:         TxTypeReferral,
					Points:       program.RefereeBonus,
					BalanceAfter: newBal,
					Description:  &desc,
					CreatedAt:    time.Now(),
				}); err != nil {
					return err
				}
				customer.PointsBalance = newBal
			}

			// Award referrer bonus
			if program.ReferralBonus > 0 {
				newBal, err := s.repo.CreditPoints(tx, referrer.ID, program.ReferralBonus)
				if err != nil {
					return err
				}
				desc := fmt.Sprintf("Referral bonus: %s joined", req.CustomerEmail)
				if err := s.repo.CreateTransaction(tx, &LoyaltyTransaction{
					TenantID:     req.TenantID,
					LoyaltyID:    referrer.ID,
					Type:         TxTypeReferral,
					Points:       program.ReferralBonus,
					BalanceAfter: newBal,
					Description:  &desc,
					CreatedAt:    time.Now(),
				}); err != nil {
					return err
				}
			}

			// Complete the referral
			if err := s.repo.CompleteReferral(tx, referral.ID); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return customer, nil
}

// --- Award Points (post-checkout) ---

// AwardPoints grants points based on an order total. Called after
// successful checkout. The formula: floor(orderTotal * pointsPerUnit * tierMultiplier).
func (s *Service) AwardPoints(ctx context.Context, tenantID, storeID uuid.UUID, customerEmail string, orderTotal decimal.Decimal, orderID uuid.UUID) error {
	program, err := s.repo.GetProgram(ctx, s.db, storeID)
	if err != nil || program == nil || !program.IsActive {
		return nil // silently skip if program not active
	}

	customer, err := s.repo.GetCustomerByEmail(ctx, s.db, storeID, customerEmail)
	if err != nil {
		return err
	}
	if customer == nil {
		return nil // not enrolled — skip
	}

	// Idempotency guard: webhook retries must not double-award.
	alreadyAwarded, err := s.repo.HasEarnForOrder(ctx, s.db, orderID)
	if err != nil {
		return err
	}
	if alreadyAwarded {
		return nil
	}

	// Calculate points: floor(orderTotal * pointsPerUnit * tierMultiplier).
	// orderTotal is in the store's currency — pointsPerUnit is the rate per unit
	// of that currency, so the math is currency-neutral.
	multiplier := s.getTierMultiplier(program, customer.LifetimePoints)
	rawPoints := orderTotal.Mul(program.PointsPerUnit).Mul(multiplier)
	points := int(rawPoints.IntPart())
	if points <= 0 {
		return nil
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		newBalance, err := s.repo.CreditPoints(tx, customer.ID, points)
		if err != nil {
			return err
		}
		desc := fmt.Sprintf("Order %s: earned %d points", orderID.String()[:8], points)
		if err := s.repo.CreateTransaction(tx, &LoyaltyTransaction{
			TenantID:     tenantID,
			LoyaltyID:    customer.ID,
			OrderID:      &orderID,
			Type:         TxTypeEarn,
			Points:       points,
			BalanceAfter: newBalance,
			Description:  &desc,
			CreatedAt:    time.Now(),
		}); err != nil {
			return err
		}

		// Recalculate tier after earning
		newTier := s.calculateTier(program, customer.LifetimePoints+points)
		if newTier != customer.Tier {
			if err := s.repo.UpdateTier(tx, customer.ID, newTier); err != nil {
				return err
			}
		}
		return nil
	})
}

// --- Redeem Points ---

// RedeemPoints deducts points from a customer's balance using its own
// transaction. Returns the monetary value of the redeemed points.
// For standalone redemption (not inside order creation).
func (s *Service) RedeemPoints(ctx context.Context, tenantID, storeID uuid.UUID, customerEmail string, points int, orderID *uuid.UUID) (decimal.Decimal, error) {
	return s.redeemPointsInternal(ctx, s.db, tenantID, storeID, customerEmail, points, orderID)
}

// RedeemPointsTx deducts points within a caller-provided transaction.
// Amendment FIX 1: MUST run inside the order creation transaction.
func (s *Service) RedeemPointsTx(ctx context.Context, tx *gorm.DB, tenantID, storeID uuid.UUID, customerEmail string, points int, orderID *uuid.UUID) (decimal.Decimal, error) {
	return s.redeemPointsInternal(ctx, tx, tenantID, storeID, customerEmail, points, orderID)
}

func (s *Service) redeemPointsInternal(ctx context.Context, db *gorm.DB, tenantID, storeID uuid.UUID, customerEmail string, points int, orderID *uuid.UUID) (decimal.Decimal, error) {
	program, err := s.repo.GetProgram(ctx, s.db, storeID)
	if err != nil {
		return decimal.Zero, err
	}
	if program == nil || !program.IsActive {
		return decimal.Zero, apperrors.New(apperrors.CodeLoyaltyNotEnrolled, "loyalty program is not active")
	}
	if points < program.MinRedeemPoints {
		return decimal.Zero, apperrors.ValidationFailed("points", fmt.Sprintf("minimum redemption is %d points", program.MinRedeemPoints))
	}

	customer, err := s.repo.GetCustomerByEmail(ctx, s.db, storeID, customerEmail)
	if err != nil {
		return decimal.Zero, err
	}
	if customer == nil {
		return decimal.Zero, apperrors.New(apperrors.CodeLoyaltyNotEnrolled, "customer is not enrolled in the loyalty program")
	}

	var newBalance int
	// Use the provided db handle (which may be a tx) for the debit+log.
	// If db is already a transaction, GORM's Transaction() is a nested savepoint.
	err = db.Transaction(func(innerTx *gorm.DB) error {
		var err error
		newBalance, err = s.repo.DebitPoints(innerTx, customer.ID, points)
		if err != nil {
			return err
		}
		desc := fmt.Sprintf("Redeemed %d points", points)
		return s.repo.CreateTransaction(innerTx, &LoyaltyTransaction{
			TenantID:     tenantID,
			LoyaltyID:    customer.ID,
			OrderID:      orderID,
			Type:         TxTypeRedeem,
			Points:       -points,
			BalanceAfter: newBalance,
			Description:  &desc,
			CreatedAt:    time.Now(),
		})
	})
	if err != nil {
		return decimal.Zero, err
	}

	// Amendment FIX 8: round to 2 decimal places
	value := decimal.NewFromInt(int64(points)).Mul(program.PointsValue).Round(2)
	return value, nil
}

// --- Manual Adjust (admin) ---

// AdjustPoints allows an admin to manually adjust a customer's points.
func (s *Service) AdjustPoints(ctx context.Context, tenantID uuid.UUID, loyaltyID uuid.UUID, points int, description string, adjustedBy string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var newBalance int
		var err error
		if points > 0 {
			newBalance, err = s.repo.CreditPoints(tx, loyaltyID, points)
		} else {
			newBalance, err = s.repo.DebitPoints(tx, loyaltyID, -points)
		}
		if err != nil {
			return err
		}
		return s.repo.CreateTransaction(tx, &LoyaltyTransaction{
			TenantID:     tenantID,
			LoyaltyID:    loyaltyID,
			Type:         TxTypeAdjust,
			Points:       points,
			BalanceAfter: newBalance,
			Description:  &description,
			AdjustedBy:   &adjustedBy,
			CreatedAt:    time.Now(),
		})
	})
}

// --- Helpers ---

// getTierMultiplier returns the points multiplier for the customer's
// current tier. Falls back to 1.0 if no tiers configured.
func (s *Service) getTierMultiplier(program *LoyaltyProgram, lifetimePoints int) decimal.Decimal {
	tiers := s.parseTiers(program)
	if len(tiers) == 0 {
		return decimal.NewFromInt(1)
	}
	// Amendment FIX 6: sort descending by MinPoints before iterating
	sort.Slice(tiers, func(i, j int) bool {
		return tiers[i].MinPoints > tiers[j].MinPoints
	})
	for _, t := range tiers {
		if lifetimePoints >= t.MinPoints {
			return t.Multiplier
		}
	}
	return decimal.NewFromInt(1)
}

// calculateTier returns the tier name for the given lifetime points.
func (s *Service) calculateTier(program *LoyaltyProgram, lifetimePoints int) string {
	tiers := s.parseTiers(program)
	if len(tiers) == 0 {
		return "bronze"
	}
	// Amendment FIX 6: sort descending by MinPoints before iterating
	sort.Slice(tiers, func(i, j int) bool {
		return tiers[i].MinPoints > tiers[j].MinPoints
	})
	for _, t := range tiers {
		if lifetimePoints >= t.MinPoints {
			return t.Name
		}
	}
	return "bronze"
}

func (s *Service) parseTiers(program *LoyaltyProgram) []Tier {
	var tiers []Tier
	if err := json.Unmarshal(program.Tiers, &tiers); err != nil {
		s.logger.Error("failed to parse tiers JSON", "err", err, "store_id", program.StoreID)
		return nil
	}
	return tiers
}

// GetCustomer returns a customer loyalty record by email.
func (s *Service) GetCustomer(ctx context.Context, storeID uuid.UUID, email string) (*CustomerLoyalty, error) {
	return s.repo.GetCustomerByEmail(ctx, s.db, storeID, email)
}

// GetCustomerByID returns a customer loyalty record by ID.
func (s *Service) GetCustomerByID(ctx context.Context, id uuid.UUID) (*CustomerLoyalty, error) {
	return s.repo.GetCustomerByID(ctx, s.db, id)
}

// ListMembers returns paginated loyalty members for a store.
func (s *Service) ListMembers(ctx context.Context, storeID uuid.UUID, page, limit int) ([]CustomerLoyalty, int64, error) {
	return s.repo.ListMembers(ctx, s.db, storeID, page, limit)
}

// ListTransactions returns paginated transactions for a loyalty member.
func (s *Service) ListTransactions(ctx context.Context, loyaltyID uuid.UUID, page, limit int) ([]LoyaltyTransaction, int64, error) {
	return s.repo.ListTransactions(ctx, s.db, loyaltyID, page, limit)
}

// ListReferrals returns paginated referrals for a store.
func (s *Service) ListReferrals(ctx context.Context, storeID uuid.UUID, page, limit int) ([]Referral, int64, error) {
	return s.repo.ListReferrals(ctx, s.db, storeID, page, limit)
}
