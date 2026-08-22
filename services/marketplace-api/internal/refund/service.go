package refund

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/metrics"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

const coolingOffWindow = 14 * 24 * time.Hour

// IssueInput holds the parameters for Service.IssueRefund.
type IssueInput struct {
	TenantID uuid.UUID
	StoreID  uuid.UUID
	// StripeChargeID is the Stripe charge to validate the 14-day gate against
	// and to issue the refund on. Must be non-empty.
	StripeChargeID       string
	StripeSubscriptionID string
	// Reason is the merchant-provided refund reason.
	Reason string
	// DeviceFingerprint is the X-Device-Fingerprint header value (optional; logged for fraud audit).
	DeviceFingerprint string
	// Actor is the audit actor ("user:<uuid>").
	Actor string
}

// IssueOutput is returned by Service.IssueRefund on success.
type IssueOutput struct {
	StripeRefundID  string
	AmountMinor     int64
	Currency        string
	CardFingerprint string
}

// Service orchestrates the 14-day cooling-off refund flow (§8).
type Service struct {
	db      *gorm.DB
	repo    Repository
	subRepo subscription.Repository
	stripe  *billingstripe.Client
	logger  *slog.Logger
}

// NewService constructs a Service.
func NewService(
	db *gorm.DB,
	repo Repository,
	subRepo subscription.Repository,
	stripe *billingstripe.Client,
	logger *slog.Logger,
) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{db: db, repo: repo, subRepo: subRepo, stripe: stripe, logger: logger}
}

// IssueRefund runs the full §8 refund pipeline:
//  1. Fetch store subscription — refuse if Pro+App add-on (§8).
//  2. GetCharge from Stripe to read authoritative created_at timestamp.
//  3. 14-day gate check (§8 pattern E).
//  4. Card-fingerprint uniqueness check (§8 pattern D).
//  5. Issue refund via Stripe.
//  6. Persist refund_audit row.
func (s *Service) IssueRefund(ctx context.Context, in IssueInput) (IssueOutput, error) {
	// 1. Fetch subscription — Pro+App guard.
	sub, err := s.subRepo.GetByStoreID(ctx, s.db, in.TenantID, in.StoreID)
	if err != nil {
		return IssueOutput{}, fmt.Errorf("refund: fetch subscription: %w", err)
	}
	if sub.HasWhiteLabelAppAddOn {
		return IssueOutput{}, ErrProAppNotRefundable
	}

	// 2. Fetch the Stripe charge to get authoritative created_at (§8 pattern E).
	now := time.Now().UTC()
	var chargeCreated time.Time
	var cardFingerprint string
	var chargeAmount int64
	var chargeCurrency string

	if s.stripe != nil && in.StripeChargeID != "" {
		charge, chargeErr := billingstripe.GetCharge(ctx, s.stripe, in.StripeChargeID)
		if chargeErr != nil {
			if errors.Is(chargeErr, billingstripe.ErrNotFound) {
				// Stripe charge not accessible — fall back to local first_charge_at.
				s.logger.Warn("refund: stripe charge not found, falling back to first_charge_at",
					"store_id", in.StoreID,
					"charge_id", in.StripeChargeID)
				if sub.FirstChargeAt == nil {
					return IssueOutput{}, fmt.Errorf("refund: no charge record available for 14-day gate")
				}
				chargeCreated = *sub.FirstChargeAt
			} else {
				return IssueOutput{}, fmt.Errorf("refund: fetch stripe charge: %w", chargeErr)
			}
		} else {
			chargeCreated = time.Unix(charge.Created, 0).UTC()
			cardFingerprint = charge.CardFingerprint
			chargeAmount = charge.Amount
			chargeCurrency = charge.Currency
		}
	} else {
		// No Stripe client — use local first_charge_at.
		if sub.FirstChargeAt == nil {
			return IssueOutput{}, fmt.Errorf("refund: no stripe client and no first_charge_at for 14-day gate")
		}
		chargeCreated = *sub.FirstChargeAt
		s.logger.Warn("refund: no stripe client, using local first_charge_at for 14-day gate",
			"store_id", in.StoreID)
	}

	// 3. 14-day cooling-off gate.
	windowClosed := now.After(chargeCreated.Add(coolingOffWindow))
	if windowClosed {
		return IssueOutput{}, ErrOutsideCoolingOff
	}

	// 4. Card-fingerprint uniqueness check (only when fingerprint is known).
	if cardFingerprint != "" {
		exists, fpErr := s.repo.ExistsByCardFingerprint(ctx, s.db, cardFingerprint)
		if fpErr != nil {
			return IssueOutput{}, fmt.Errorf("refund: card fingerprint check: %w", fpErr)
		}
		if exists {
			// Log the fraud signal internally but don't leak the fingerprint-match
			// detail in the external error (§8 pattern D).
			s.logger.Warn("refund: card fingerprint already has a refund — rejecting",
				"store_id", in.StoreID,
				"fingerprint_prefix", cardFingerprint[:min(6, len(cardFingerprint))])
			return IssueOutput{}, ErrCardFingerprintExists
		}
	}

	// 5. Issue refund via Stripe.
	var stripeRefundID string
	if s.stripe != nil && in.StripeChargeID != "" {
		r, refErr := billingstripe.CreateRefund(ctx, s.stripe, billingstripe.CreateRefundInput{
			ChargeID:       in.StripeChargeID,
			Reason:         "requested_by_customer",
			IdempotencyKey: billingstripe.RefundIdempotencyKey(in.StripeChargeID),
		})
		if refErr != nil {
			return IssueOutput{}, fmt.Errorf("refund: stripe create refund: %w", refErr)
		}
		stripeRefundID = r.ID
		if chargeAmount == 0 {
			chargeAmount = r.Amount
		}
		if chargeCurrency == "" {
			chargeCurrency = r.Currency
		}
	} else {
		s.logger.Warn("refund: no stripe client — skipping Stripe refund call",
			"store_id", in.StoreID)
		stripeRefundID = "no-stripe-" + uuid.New().String()
	}

	// 6. Persist refund_audit row.
	var fp *string
	if cardFingerprint != "" {
		fp = &cardFingerprint
	}
	var dfp *string
	if in.DeviceFingerprint != "" {
		dfp = &in.DeviceFingerprint
	}
	audit := &RefundAudit{
		SubscriptionID:    sub.ID,
		StoreID:           in.StoreID,
		TenantID:          in.TenantID,
		StripeRefundID:    stripeRefundID,
		StripeChargeID:    in.StripeChargeID,
		AmountMinor:       chargeAmount,
		Currency:          chargeCurrency,
		Reason:            in.Reason,
		CardFingerprint:   fp,
		DeviceFingerprint: dfp,
		IssuedBy:          in.Actor,
		IssuedAt:          now,
	}
	if err := s.repo.Create(ctx, s.db, audit); err != nil {
		return IssueOutput{}, fmt.Errorf("refund: persist audit: %w", err)
	}

	if metrics.Subscription != nil {
		currency := chargeCurrency
		if currency == "" {
			currency = "unknown"
		}
		metrics.Subscription.RefundIssuedTotal.WithLabelValues(currency, in.Reason).Inc()
	}

	return IssueOutput{
		StripeRefundID:  stripeRefundID,
		AmountMinor:     chargeAmount,
		Currency:        chargeCurrency,
		CardFingerprint: cardFingerprint,
	}, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
