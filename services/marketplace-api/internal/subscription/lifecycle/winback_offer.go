package lifecycle

import (
	"context"
	"strconv"
	"strings"

	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/promo"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// WinBackPromoCode is the promo code the day-30 win-back email offers.
//
// A promo_codes row whose code is exactly this string is what switches the
// win-back offer on. mark8ly does not mint codes — rows arrive from the
// tesserix-home promo catalog through internal/billing/consolepromo (#726) —
// so authoring it is a console action. Until it exists, and on every run
// where it does not currently validate, the cron sends
// email.TemplateWinBackNoOffer instead and states nothing (#727).
//
// It is deliberately NOT cancel.SaveOfferPromoCode. The two are separate
// campaigns with separate caps and separate redemption histories, and
// max_per_email is 1: sharing one code would mean a merchant who took the
// save offer could never take the win-back, and neither campaign's redemption
// count would mean anything on its own.
const WinBackPromoCode = "WINBACK20OFF6MONTHS"

// WinBackPromoChecker is the narrow slice of promo.Service the win-back needs.
//
// ValidateCode and not ApplyPromo, because the merchant has not asked for
// anything yet. Redeeming on their behalf at email time would attach a coupon
// to a subscription they have already lost and — max_per_email being 1 —
// consume the only redemption their address gets, so the code in the email
// would be rejected the moment they tried to use it.
type WinBackPromoChecker interface {
	ValidateCode(ctx context.Context, in promo.ApplyInput) (promo.ApplyOutput, error)
}

// winBackOffer is a discount the email is entitled to state. The zero value
// is "no offer", and every field is populated from the promo row rather than
// from prose.
type winBackOffer struct {
	Code string
	// PercentOff is the discount as a display string without the sign:
	// "20", or "12.5" for a rate that is not a whole percentage.
	PercentOff     string
	DurationMonths int
}

// WithPromo returns the cron with p attached. A cron without one never states
// an offer; passing nil leaves it unchanged. Mirrors cancel.Service.WithPromo.
func (c *WinBackCron) WithPromo(p WinBackPromoChecker) *WinBackCron {
	if c == nil || p == nil {
		return c
	}
	c.promo = p
	return c
}

// WinBackTemplate maps "is there an offer to state" onto the template key.
//
// This decision lives in Go on purpose. Both keys are overridable from the
// operator console, so expressing it as a {{if}} inside a single template
// would put the guard on the discount claim into an editable text box —
// deleting the guard would restore #727's unconditional promise with no code
// change, no review and no test able to see it.
func WinBackTemplate(offered bool) email.TemplateID {
	if offered {
		return email.TemplateWinBack
	}
	return email.TemplateWinBackNoOffer
}

// offerFor reports the discount this row's email may state, if any.
//
// Like cancel.applySaveOfferDiscount it returns no error: the win-back is
// worth sending either way — a merchant a month past expiry does not know
// their products, orders and settings survived — so a promo problem must
// degrade the copy, never suppress the mail. Every failure path logs and
// returns "no offer", and the copy then follows honestly.
func (c *WinBackCron) offerFor(ctx context.Context, row *subscription.StoreSubscription, to string) (winBackOffer, bool) {
	if c.promo == nil {
		c.logger.Info("lifecycle: win-back has no promo service — sending without an offer",
			"store_id", row.StoreID)
		return winBackOffer{}, false
	}
	if to == "" {
		// Per-email redemption caps cannot be evaluated without an address,
		// and this send is about to be refused as undeliverable anyway.
		return winBackOffer{}, false
	}

	out, err := c.promo.ValidateCode(ctx, promo.ApplyInput{
		TenantID:       row.TenantID,
		StoreID:        row.StoreID,
		SubscriptionID: row.ID,
		Code:           WinBackPromoCode,
		MerchantEmail:  to,
		Plan:           row.Plan,
		Period:         row.SubscriptionPeriod,
		BasePriceMinor: promo.BasePriceMinorFor(
			row.Plan, row.SubscriptionPeriod, row.PriceTier, derefString(row.BillingCurrency)),
		Currency: derefString(row.BillingCurrency),
		Actor:    "system:winback",
	})
	if err != nil {
		c.logger.Info("lifecycle: win-back offer not available — sending without one",
			"store_id", row.StoreID, "code", WinBackPromoCode,
			"reason", string(out.RejectReason), "err", err.Error())
		return winBackOffer{}, false
	}

	pct, ok := formatPercentBps(out.PercentOffBps)
	if !ok || out.MaxDurationMonths <= 0 {
		// The copy says "N% off your first M months". A code that carries a
		// flat amount, or no bound on how long the discount runs, cannot be
		// described by that sentence — and inventing a sentence for it is
		// how the hardcoded promise got there in the first place.
		c.logger.Warn("lifecycle: win-back code is valid but not describable as a bounded percentage — sending without an offer",
			"store_id", row.StoreID, "code", WinBackPromoCode,
			"percent_off_bps", out.PercentOffBps, "max_duration_months", out.MaxDurationMonths)
		return winBackOffer{}, false
	}

	return winBackOffer{
		Code:           WinBackPromoCode,
		PercentOff:     pct,
		DurationMonths: out.MaxDurationMonths,
	}, true
}

// formatPercentBps renders basis points as a display percentage: 2000 → "20",
// 1250 → "12.5". It reports false for anything outside (0, 100), which the
// caller treats as "not describable" rather than rounding it into a number
// the merchant would be quoted.
func formatPercentBps(bps int) (string, bool) {
	if bps <= 0 || bps >= 10000 {
		return "", false
	}
	s := strconv.FormatFloat(float64(bps)/100, 'f', 2, 64)
	s = strings.TrimRight(s, "0")
	return strings.TrimRight(s, "."), true
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
