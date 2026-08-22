package promo

import (
	"crypto/subtle"
	"strings"
	"time"

	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// ValidationInput holds all the context needed to validate a promo code
// application. The submitted code is compared against the stored code using
// constant-time comparison to prevent timing-based enumeration (§7.3 pattern A).
type ValidationInput struct {
	// SubmittedCode is the raw code string from the HTTP request.
	SubmittedCode string
	// PromoCode is the row fetched from DB (looked up by code, not by this field).
	PromoCode *PromoCode
	// Now is the validation wall-clock time; injected for testability.
	Now time.Time
	// TotalRedemptions is the current global redemption count for this code.
	TotalRedemptions int
	// EmailRedemptions is the number of times the merchant's email has used this code.
	EmailRedemptions int
	// Plan is the store's current subscription plan.
	Plan subscription.SubscriptionPlan
	// Period is the store's current billing period.
	Period subscription.SubscriptionPeriod
	// BasePriceMinor is the undiscounted price in minor units (for floor check).
	BasePriceMinor int64
	// Currency is the store's billing currency (for floor check).
	Currency string
}

// ValidationRejectReason records the internal reason for audit events.
// Never sent to the HTTP client — the external response is always ErrInvalidOrExpired.
type ValidationRejectReason string

const (
	RejectReasonNotFound           ValidationRejectReason = "not_found"
	RejectReasonExpired            ValidationRejectReason = "expired"
	RejectReasonMaxRedemptions     ValidationRejectReason = "max_redemptions_reached"
	RejectReasonMaxPerEmail        ValidationRejectReason = "max_per_email_reached"
	RejectReasonWrongPlan          ValidationRejectReason = "wrong_plan"
	RejectReasonAnnualOnly         ValidationRejectReason = "annual_only"
	RejectReasonBelowFloor         ValidationRejectReason = "below_absolute_floor"
	RejectReasonCurrencyNotCovered ValidationRejectReason = "currency_not_covered"
)

// ValidationResult is returned by Validate. On success Accepted is true and
// RejectReason is empty. On failure Accepted is false and RejectReason records
// the internal reason for audit logging.
type ValidationResult struct {
	Accepted     bool
	RejectReason ValidationRejectReason
	// EffectiveMinor is the price after the discount is applied (set only on success).
	EffectiveMinor int64
}

// Validate performs constant-time code matching plus all business-rule checks
// for a promo application (§7.3). The order of checks deliberately
// runs constant-time comparison first so timing signals cannot distinguish
// "code not found" from "code found but invalid" at the HTTP layer.
func Validate(in ValidationInput) ValidationResult {
	// Constant-time comparison of submitted code vs stored code (§7.3 pattern A).
	// Even though the DB lookup is indexed (leaking length timing), the comparison
	// itself must be constant-time to prevent byte-by-byte enumeration on valid prefixes.
	storedBytes := []byte(strings.ToUpper(strings.TrimSpace(in.PromoCode.Code)))
	submittedBytes := []byte(strings.ToUpper(strings.TrimSpace(in.SubmittedCode)))
	if subtle.ConstantTimeCompare(storedBytes, submittedBytes) != 1 {
		return ValidationResult{Accepted: false, RejectReason: RejectReasonNotFound}
	}

	now := in.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	// valid_from / valid_until window.
	if in.PromoCode.ValidFrom.After(now) {
		return ValidationResult{Accepted: false, RejectReason: RejectReasonExpired}
	}
	if in.PromoCode.ValidUntil != nil && now.After(*in.PromoCode.ValidUntil) {
		return ValidationResult{Accepted: false, RejectReason: RejectReasonExpired}
	}

	// Global max redemptions check.
	if in.PromoCode.MaxRedemptions != nil && in.TotalRedemptions >= *in.PromoCode.MaxRedemptions {
		return ValidationResult{Accepted: false, RejectReason: RejectReasonMaxRedemptions}
	}

	// Per-email max redemptions.
	if in.EmailRedemptions >= in.PromoCode.MaxPerEmail {
		return ValidationResult{Accepted: false, RejectReason: RejectReasonMaxPerEmail}
	}

	// Allowed plan check.
	if len(in.PromoCode.AllowedPlans) > 0 {
		planAllowed := false
		for _, p := range in.PromoCode.AllowedPlans {
			if strings.EqualFold(p, string(in.Plan)) {
				planAllowed = true
				break
			}
		}
		if !planAllowed {
			return ValidationResult{Accepted: false, RejectReason: RejectReasonWrongPlan}
		}
	}

	// Annual-only shape check.
	if in.PromoCode.AnnualOnly && in.Period != subscription.PeriodAnnual {
		return ValidationResult{Accepted: false, RejectReason: RejectReasonAnnualOnly}
	}

	// Compute effective price and run floor check.
	var effectiveMinor int64
	switch in.PromoCode.DiscountType {
	case DiscountTypePercentage:
		effectiveMinor = ComputeEffective(in.BasePriceMinor, in.PromoCode.DiscountValue, 0)
	case DiscountTypeAmount:
		effectiveMinor = ComputeEffective(in.BasePriceMinor, 0, int64(in.PromoCode.DiscountValue))
	default:
		effectiveMinor = in.BasePriceMinor
	}

	if err := CheckFloor(in.Plan, in.Currency, effectiveMinor); err != nil {
		switch err {
		case ErrCurrencyNotCovered:
			return ValidationResult{Accepted: false, RejectReason: RejectReasonCurrencyNotCovered}
		default:
			return ValidationResult{Accepted: false, RejectReason: RejectReasonBelowFloor}
		}
	}

	return ValidationResult{Accepted: true, EffectiveMinor: effectiveMinor}
}
