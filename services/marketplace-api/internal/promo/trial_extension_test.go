package promo_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/promo"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// extendCall records one invocation of the trial extender, so a test can
// assert the DATE that was asked for. The date is the whole point: #620's
// hazard is an extension computed from the signup date, which silently
// discards an extension an operator already granted.
type extendCall struct {
	storeID uuid.UUID
	newEnd  time.Time
	idemKey string
}

type stubExtender struct {
	calls []extendCall
	err   error
}

func (e *stubExtender) Extend(_ context.Context, _ *gorm.DB, storeID uuid.UUID,
	newEnd, _ time.Time, callerIdemKey string) (trial.ExtendResult, error) {
	e.calls = append(e.calls, extendCall{storeID: storeID, newEnd: newEnd, idemKey: callerIdemKey})
	if e.err != nil {
		return trial.ExtendResult{}, e.err
	}
	return trial.ExtendResult{StoreID: storeID, NewEndsAt: newEnd}, nil
}

// trialRepo is stubRepo plus the redemption bookkeeping ApplyPromo performs,
// so a test can tell a redemption that was written from one that was rolled
// back — the difference between "try again" and "your code is spent".
type trialRepo struct {
	code           *promo.PromoCode
	perEmail       int
	created        []promo.Redemption
	deletes        int
	createErr      error
	alreadyRedeems bool
}

func (r *trialRepo) GetByCode(context.Context, *gorm.DB, string) (*promo.PromoCode, error) {
	return r.code, nil
}
func (r *trialRepo) GetByID(context.Context, *gorm.DB, uuid.UUID) (*promo.PromoCode, error) {
	return r.code, nil
}
func (r *trialRepo) Create(context.Context, *gorm.DB, *promo.PromoCode) error { return nil }
func (r *trialRepo) CountRedemptions(context.Context, *gorm.DB, uuid.UUID) (int, error) {
	return 0, nil
}
func (r *trialRepo) CountRedemptionsByEmail(context.Context, *gorm.DB, uuid.UUID, string) (int, error) {
	return r.perEmail, nil
}
func (r *trialRepo) GetRedemptionByStore(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) (*promo.Redemption, error) {
	if r.alreadyRedeems {
		return &promo.Redemption{}, nil
	}
	return nil, apperrors.NotFound("redemption")
}
func (r *trialRepo) CreateRedemption(_ context.Context, _ *gorm.DB, red *promo.Redemption) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.created = append(r.created, *red)
	return nil
}
func (r *trialRepo) DeleteRedemptionByStore(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) error {
	r.deletes++
	return nil
}

// extensionRow is a trial-extension-ONLY code: no discount, no Stripe coupon.
// That shape is normal, not degenerate — the console mints no Coupon for a
// code that touches no Stripe object (#726).
func extensionRow(days int) *promo.PromoCode {
	return &promo.PromoCode{
		ID:                 uuid.New(),
		Code:               "STAYLONGER",
		TrialExtensionDays: &days,
		MaxPerEmail:        1,
		ValidFrom:          time.Now().UTC().Add(-time.Hour),
	}
}

// trialingSub is a store still on trial, ending `in` from now. storedEnd is
// written to TrialEndsAt so a test can pin the EFFECTIVE end rather than the
// derived one.
func trialingSub(storedEnd time.Time) *subscription.StoreSubscription {
	return &subscription.StoreSubscription{
		ID:          uuid.New(),
		StoreID:     uuid.New(),
		TenantID:    uuid.New(),
		Status:      subscription.StatusTrialing,
		Plan:        subscription.PlanStarter,
		CreatedAt:   time.Now().UTC().Add(-30 * 24 * time.Hour),
		TrialEndsAt: &storedEnd,
	}
}

func extensionInput(sub *subscription.StoreSubscription) promo.ApplyInput {
	in := validateInput()
	in.Code = "STAYLONGER"
	in.Sub = sub
	if sub != nil {
		in.StoreID = sub.StoreID
		in.TenantID = sub.TenantID
		in.SubscriptionID = sub.ID
	}
	return in
}

// The core of #620: redeeming a trial-extension code must actually move the
// trial end. Before this, ApplyPromo returned 200, wrote a ledger row, and
// granted nothing.
func TestApplyPromo_TrialExtensionCodeMovesTheTrialEnd(t *testing.T) {
	end := time.Now().UTC().Add(10 * 24 * time.Hour)
	sub := trialingSub(end)
	repo := &trialRepo{code: extensionRow(14)}
	ext := &stubExtender{}
	svc := promo.NewService(nil, repo, nil, nil).WithTrialExtender(ext)

	out, err := svc.ApplyPromo(context.Background(), extensionInput(sub))
	if err != nil {
		t.Fatalf("ApplyPromo: %v", err)
	}
	if len(ext.calls) != 1 {
		t.Fatalf("extender called %d times, want 1", len(ext.calls))
	}
	want := end.AddDate(0, 0, 14)
	if !ext.calls[0].newEnd.Equal(want) {
		t.Errorf("newEnd = %s, want %s", ext.calls[0].newEnd, want)
	}
	if ext.calls[0].storeID != sub.StoreID {
		t.Errorf("storeID = %s, want %s", ext.calls[0].storeID, sub.StoreID)
	}
	if out.TrialExtensionDays != 14 {
		t.Errorf("TrialExtensionDays = %d, want 14", out.TrialExtensionDays)
	}
	if !out.TrialEndsAt.Equal(want) {
		t.Errorf("TrialEndsAt = %s, want %s", out.TrialEndsAt, want)
	}
}

// #620 names this hazard outright: Extend takes an ABSOLUTE end, so an
// extension computed from created_at + TrialDays silently overwrites an
// extension an operator already granted. The base must be trial.EndsAt.
func TestApplyPromo_ExtendsFromTheStoredEndNotTheSignupDate(t *testing.T) {
	// Operator already pushed this trial far past created_at + 90d.
	operatorEnd := time.Now().UTC().Add(200 * 24 * time.Hour)
	sub := trialingSub(operatorEnd)
	repo := &trialRepo{code: extensionRow(14)}
	ext := &stubExtender{}
	svc := promo.NewService(nil, repo, nil, nil).WithTrialExtender(ext)

	if _, err := svc.ApplyPromo(context.Background(), extensionInput(sub)); err != nil {
		t.Fatalf("ApplyPromo: %v", err)
	}
	want := operatorEnd.AddDate(0, 0, 14)
	if !ext.calls[0].newEnd.Equal(want) {
		t.Fatalf("newEnd = %s, want %s — the promo overwrote the operator's extension",
			ext.calls[0].newEnd, want)
	}
	derived := sub.CreatedAt.AddDate(0, 0, trial.TrialDays+14)
	if ext.calls[0].newEnd.Equal(derived) {
		t.Fatal("newEnd was computed from created_at + TrialDays")
	}
}

// The extension is keyed on the ledger row, per #620: "using the redemption id
// as the callerIdemKey so a retry after a partial failure re-applies cleanly
// rather than double-extending".
func TestApplyPromo_ExtensionIsKeyedOnTheRedemptionRow(t *testing.T) {
	sub := trialingSub(time.Now().UTC().Add(10 * 24 * time.Hour))
	repo := &trialRepo{code: extensionRow(7)}
	ext := &stubExtender{}
	svc := promo.NewService(nil, repo, nil, nil).WithTrialExtender(ext)

	if _, err := svc.ApplyPromo(context.Background(), extensionInput(sub)); err != nil {
		t.Fatalf("ApplyPromo: %v", err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("wrote %d redemption rows, want 1", len(repo.created))
	}
	id := repo.created[0].ID
	if id == uuid.Nil {
		t.Fatal("redemption row has no id, so the idempotency key cannot be derived from it")
	}
	if want := "promo_redeem:" + id.String(); ext.calls[0].idemKey != want {
		t.Errorf("idemKey = %q, want %q", ext.calls[0].idemKey, want)
	}
}

// A lapsed or converted subscription is refused BEFORE anything is written, so
// the merchant keeps their one redemption. #620 open question 1, answered the
// way extend.go already answers it: reinstating an expired trial is out of
// scope.
func TestApplyPromo_TrialThatCannotMoveIsRefusedWithoutBurningTheCode(t *testing.T) {
	cases := []struct {
		name string
		sub  *subscription.StoreSubscription
	}{
		{"already lapsed", trialingSub(time.Now().UTC().Add(-24 * time.Hour))},
		{"already converted", func() *subscription.StoreSubscription {
			s := trialingSub(time.Now().UTC().Add(10 * 24 * time.Hour))
			s.Status = subscription.StatusActive
			return s
		}()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &trialRepo{code: extensionRow(14)}
			ext := &stubExtender{}
			svc := promo.NewService(nil, repo, nil, nil).WithTrialExtender(ext)

			out, err := svc.ApplyPromo(context.Background(), extensionInput(tc.sub))
			if !errors.Is(err, promo.ErrInvalidOrExpired) {
				t.Fatalf("err = %v, want ErrInvalidOrExpired", err)
			}
			if out.RejectReason != promo.RejectReasonTrialNotExtendable {
				t.Errorf("reject reason = %q, want trial_not_extendable", out.RejectReason)
			}
			if len(repo.created) != 0 {
				t.Error("a refused redemption still wrote a ledger row — the code is now spent")
			}
			if len(ext.calls) != 0 {
				t.Error("extender was called for a trial that cannot move")
			}
		})
	}
}

// The merchant is told a fact about their own subscription, which is safe to
// disclose — it confirms nothing about a code they were not already given.
func TestPublicReasonFor_TrialNotExtendableIsDisclosed(t *testing.T) {
	if got := promo.PublicReasonFor(promo.RejectReasonTrialNotExtendable); got != promo.PublicReasonTrialNotExtendable {
		t.Errorf("PublicReasonFor(trial_not_extendable) = %q, want trial_not_extendable", got)
	}
}

// A missing extender is OUR fault, not the merchant's. Reporting it as
// "invalid or expired" would tell a merchant holding a perfectly good code to
// stop trying, and would hide a wiring regression behind a 422.
func TestApplyPromo_MissingExtenderIsAServerFaultNotARejection(t *testing.T) {
	sub := trialingSub(time.Now().UTC().Add(10 * 24 * time.Hour))
	repo := &trialRepo{code: extensionRow(14)}
	svc := promo.NewService(nil, repo, nil, nil) // no extender wired

	_, err := svc.ApplyPromo(context.Background(), extensionInput(sub))
	if err == nil {
		t.Fatal("ApplyPromo succeeded with no extender — the extension was silently skipped")
	}
	if errors.Is(err, promo.ErrInvalidOrExpired) {
		t.Fatalf("err = %v, want a server error rather than a merchant-facing rejection", err)
	}
	if len(repo.created) != 0 {
		t.Error("wrote a ledger row for an extension that could not be delivered")
	}
}

// Same reasoning for a caller that forgot the subscription: it is a call-site
// bug, and failing closed keeps it from reading as a bad code.
func TestApplyPromo_MissingSubscriptionIsAServerFault(t *testing.T) {
	repo := &trialRepo{code: extensionRow(14)}
	ext := &stubExtender{}
	svc := promo.NewService(nil, repo, nil, nil).WithTrialExtender(ext)

	in := extensionInput(nil)
	in.StoreID = uuid.New()
	_, err := svc.ApplyPromo(context.Background(), in)
	if err == nil || errors.Is(err, promo.ErrInvalidOrExpired) {
		t.Fatalf("err = %v, want a server error", err)
	}
	if len(repo.created) != 0 {
		t.Error("wrote a ledger row without a subscription to extend")
	}
}

// A failed extension must not leave the code spent: roll the ledger row back so
// the merchant can retry.
func TestApplyPromo_FailedExtensionRollsBackTheRedemption(t *testing.T) {
	sub := trialingSub(time.Now().UTC().Add(10 * 24 * time.Hour))
	repo := &trialRepo{code: extensionRow(14)}
	ext := &stubExtender{err: errors.New("boom")}
	svc := promo.NewService(nil, repo, nil, nil).WithTrialExtender(ext)

	if _, err := svc.ApplyPromo(context.Background(), extensionInput(sub)); err == nil {
		t.Fatal("ApplyPromo succeeded despite the extension failing")
	}
	if repo.deletes != 1 {
		t.Fatalf("redemption deletes = %d, want 1 — the merchant's code is spent on nothing", repo.deletes)
	}
}

// The one failure that must NOT roll back. Stripe has already moved the
// merchant's billing date; deleting the ledger row would erase the only local
// record that it happened and invite a retry that Stripe then refuses.
func TestApplyPromo_StripeAlreadyMovedIsNotRolledBack(t *testing.T) {
	sub := trialingSub(time.Now().UTC().Add(10 * 24 * time.Hour))
	repo := &trialRepo{code: extensionRow(14)}
	ext := &stubExtender{err: fmt.Errorf("%w: sub_123", trial.ErrStripeAppliedLocalWriteFailed)}
	svc := promo.NewService(nil, repo, nil, nil).WithTrialExtender(ext)

	_, err := svc.ApplyPromo(context.Background(), extensionInput(sub))
	if !errors.Is(err, trial.ErrStripeAppliedLocalWriteFailed) {
		t.Fatalf("err = %v, want the divergence surfaced to the caller", err)
	}
	if repo.deletes != 0 {
		t.Fatalf("redemption deletes = %d, want 0 — rolling back hides that stripe already moved", repo.deletes)
	}
}

// A discount-only code must not touch the trial, and must still work with no
// extender wired at all — which is every existing call site.
func TestApplyPromo_DiscountOnlyCodeNeverTouchesTheTrial(t *testing.T) {
	sub := trialingSub(time.Now().UTC().Add(10 * 24 * time.Hour))
	repo := &trialRepo{code: winBackRow()}
	ext := &stubExtender{}
	svc := promo.NewService(nil, repo, nil, nil).WithTrialExtender(ext)

	in := extensionInput(sub)
	in.Code = "WINBACK20OFF6MONTHS"
	out, err := svc.ApplyPromo(context.Background(), in)
	if err != nil {
		t.Fatalf("ApplyPromo: %v", err)
	}
	if len(ext.calls) != 0 {
		t.Errorf("extender called %d times for a discount-only code", len(ext.calls))
	}
	if out.TrialExtensionDays != 0 || !out.TrialEndsAt.IsZero() {
		t.Errorf("output claims a trial extension: days=%d end=%s", out.TrialExtensionDays, out.TrialEndsAt)
	}
}

// ValidateCode answers "would this be accepted" and must describe the SAME
// terms ApplyPromo would grant, without granting them — the rule the win-back
// email already relies on for PercentOffBps (#727).
func TestValidateCode_DescribesTheTrialExtensionAndGrantsNothing(t *testing.T) {
	sub := trialingSub(time.Now().UTC().Add(10 * 24 * time.Hour))
	repo := &trialRepo{code: extensionRow(30)}
	ext := &stubExtender{}
	svc := promo.NewService(nil, repo, nil, nil).WithTrialExtender(ext)

	out, err := svc.ValidateCode(context.Background(), extensionInput(sub))
	if err != nil {
		t.Fatalf("ValidateCode: %v", err)
	}
	if out.TrialExtensionDays != 30 {
		t.Errorf("TrialExtensionDays = %d, want 30", out.TrialExtensionDays)
	}
	if len(ext.calls) != 0 || len(repo.created) != 0 {
		t.Error("ValidateCode granted the extension it was only asked about")
	}
}
