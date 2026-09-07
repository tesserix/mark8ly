package promo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/promo"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

func signupInput() promo.SignupInput {
	return promo.SignupInput{
		Code:          "STAYLONGER",
		MerchantEmail: "founder@example.com",
		Currency:      "aud",
	}
}

// The whole point of the onboarding field (#620): tell the merchant what the
// code grants BEFORE they commit, without spending it.
func TestValidateForSignup_ReportsTheDaysAndRecordsNothing(t *testing.T) {
	repo := &trialRepo{code: extensionRow(30)}
	ext := &stubExtender{}
	svc := promo.NewService(nil, repo, nil, nil).WithTrialExtender(ext)

	offer, err := svc.ValidateForSignup(context.Background(), signupInput())
	if err != nil {
		t.Fatalf("ValidateForSignup: %v", err)
	}
	if offer.TrialExtensionDays != 30 {
		t.Errorf("TrialExtensionDays = %d, want 30", offer.TrialExtensionDays)
	}
	// Both halves of "records nothing". max_per_email is 1, so a redemption
	// written while the merchant is still typing is a code they can never use.
	if len(repo.created) != 0 {
		t.Fatal("validating a code at signup consumed it")
	}
	if len(ext.calls) != 0 {
		t.Fatal("validating a code extended a trial that does not exist yet")
	}
}

// A code carrying a discount cannot be delivered at signup: there is no
// subscription for Stripe to attach a coupon to, and redeeming would write the
// ledger row anyway — spending the merchant's one redemption on the half we
// cannot honour. Refuse it, and say where it DOES work.
func TestValidateForSignup_RefusesACodeCarryingADiscount(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  *promo.PromoCode
	}{
		{"discount only", winBackRow()},
		{"discount AND trial days", func() *promo.PromoCode {
			r := winBackRow()
			days := 14
			r.TrialExtensionDays = &days
			return r
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &trialRepo{code: tc.row}
			svc := promo.NewService(nil, repo, nil, nil).WithTrialExtender(&stubExtender{})

			in := signupInput()
			in.Code = tc.row.Code
			offer, err := svc.ValidateForSignup(context.Background(), in)
			if !errors.Is(err, promo.ErrInvalidOrExpired) {
				t.Fatalf("err = %v, want ErrInvalidOrExpired", err)
			}
			if offer.RejectReason != promo.RejectReasonRedeemInBilling {
				t.Errorf("reject reason = %q, want redeem_in_billing", offer.RejectReason)
			}
			if len(repo.created) != 0 {
				t.Error("a refused code still wrote a redemption row")
			}
		})
	}
}

// The public reason has to reach the merchant: "invalid or expired" would send
// someone holding a working code away from the field that would have taken it.
func TestPublicReasonFor_RedeemInBillingIsDisclosed(t *testing.T) {
	if got := promo.PublicReasonFor(promo.RejectReasonRedeemInBilling); got != promo.PublicReasonRedeemInBilling {
		t.Errorf("PublicReasonFor(redeem_in_billing) = %q", got)
	}
}

// Ordinary validation still applies — the per-email cap in particular, which is
// the one check that can be made at signup because the address is known.
func TestValidateForSignup_AppliesThePerEmailCap(t *testing.T) {
	repo := &trialRepo{code: extensionRow(14)}
	repo.perEmail = 1
	svc := promo.NewService(nil, repo, nil, nil).WithTrialExtender(&stubExtender{})

	offer, err := svc.ValidateForSignup(context.Background(), signupInput())
	if !errors.Is(err, promo.ErrInvalidOrExpired) {
		t.Fatalf("err = %v, want ErrInvalidOrExpired", err)
	}
	if offer.RejectReason != promo.RejectReasonMaxPerEmail {
		t.Errorf("reject reason = %q, want max_per_email_reached", offer.RejectReason)
	}
}

// ---------------------------------------------------------------------------
// RedeemAtSignup
// ---------------------------------------------------------------------------

func TestRedeemAtSignup_ExtendsTheBrandNewTrial(t *testing.T) {
	repo := &trialRepo{code: extensionRow(14)}
	ext := &stubExtender{}
	svc := promo.NewService(nil, repo, nil, nil).WithTrialExtender(ext)

	// A subscription created moments ago: no stored end, so the effective end
	// is created_at + TrialDays. This is the shape #827 now produces.
	sub := &subscription.StoreSubscription{
		ID: uuid.New(), StoreID: uuid.New(), TenantID: uuid.New(),
		Status: subscription.StatusSignup, Plan: subscription.PlanTrial,
		CreatedAt: time.Now().UTC(),
	}

	in := signupInput()
	in.Sub = sub
	offer, err := svc.RedeemAtSignup(context.Background(), in)
	if err != nil {
		t.Fatalf("RedeemAtSignup: %v", err)
	}
	if offer.TrialExtensionDays != 14 {
		t.Errorf("TrialExtensionDays = %d, want 14", offer.TrialExtensionDays)
	}
	if len(ext.calls) != 1 {
		t.Fatalf("extender called %d times, want 1", len(ext.calls))
	}
	if len(repo.created) != 1 {
		t.Errorf("wrote %d ledger rows, want 1", len(repo.created))
	}
}

// The same refusal as validate, at the moment it actually matters. If these
// two disagreed, the field would accept a code that signup then silently ate.
func TestRedeemAtSignup_RefusesACodeCarryingADiscount(t *testing.T) {
	repo := &trialRepo{code: winBackRow()}
	ext := &stubExtender{}
	svc := promo.NewService(nil, repo, nil, nil).WithTrialExtender(ext)

	sub := &subscription.StoreSubscription{
		ID: uuid.New(), StoreID: uuid.New(), TenantID: uuid.New(),
		Status: subscription.StatusSignup, Plan: subscription.PlanTrial,
		CreatedAt: time.Now().UTC(),
	}
	in := signupInput()
	in.Code = winBackRow().Code
	in.Sub = sub

	if _, err := svc.RedeemAtSignup(context.Background(), in); !errors.Is(err, promo.ErrInvalidOrExpired) {
		t.Fatalf("err = %v, want ErrInvalidOrExpired", err)
	}
	if len(repo.created) != 0 {
		t.Fatal("burned the merchant's redemption on a code signup cannot honour")
	}
	if len(ext.calls) != 0 {
		t.Error("extended a trial for a code that was refused")
	}
}

// An empty code is not a failure — most merchants type nothing. It must not
// look up a row, and must not read as a rejected code.
func TestRedeemAtSignup_AnEmptyCodeIsANoOp(t *testing.T) {
	repo := &trialRepo{code: extensionRow(14)}
	svc := promo.NewService(nil, repo, nil, nil).WithTrialExtender(&stubExtender{})

	in := signupInput()
	in.Code = "   "
	offer, err := svc.RedeemAtSignup(context.Background(), in)
	if err != nil {
		t.Fatalf("RedeemAtSignup: %v", err)
	}
	if offer.TrialExtensionDays != 0 || offer.RejectReason != "" {
		t.Errorf("offer = %+v, want the zero offer", offer)
	}
	if len(repo.created) != 0 {
		t.Error("an empty code wrote a redemption row")
	}
}
