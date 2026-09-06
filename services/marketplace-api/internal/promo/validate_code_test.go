package promo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/promo"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// stubRepo implements promo.Repository over in-memory values. Only the three
// reads ValidateCode performs are meaningful; the write methods exist to
// satisfy the interface and fail loudly if ValidateCode ever calls one —
// which would mean it had started redeeming.
type stubRepo struct {
	code       *promo.PromoCode
	getErr     error
	total      int
	perEmail   int
	writeCalls int
}

func (r *stubRepo) GetByCode(context.Context, *gorm.DB, string) (*promo.PromoCode, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	return r.code, nil
}
func (r *stubRepo) GetByID(context.Context, *gorm.DB, uuid.UUID) (*promo.PromoCode, error) {
	return r.code, nil
}
func (r *stubRepo) Create(context.Context, *gorm.DB, *promo.PromoCode) error {
	r.writeCalls++
	return nil
}
func (r *stubRepo) CountRedemptions(context.Context, *gorm.DB, uuid.UUID) (int, error) {
	return r.total, nil
}
func (r *stubRepo) CountRedemptionsByEmail(context.Context, *gorm.DB, uuid.UUID, string) (int, error) {
	return r.perEmail, nil
}
func (r *stubRepo) GetRedemptionByStore(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) (*promo.Redemption, error) {
	return nil, apperrors.NotFound("redemption")
}
func (r *stubRepo) CreateRedemption(context.Context, *gorm.DB, *promo.Redemption) error {
	r.writeCalls++
	return nil
}
func (r *stubRepo) DeleteRedemptionByStore(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) error {
	r.writeCalls++
	return nil
}

func winBackRow() *promo.PromoCode {
	typ := promo.DiscountTypePercentage
	val := 2000
	months := 6
	coupon := "co_test"
	return &promo.PromoCode{
		ID:                uuid.New(),
		Code:              "WINBACK20OFF6MONTHS",
		StripeCouponID:    &coupon,
		DiscountType:      &typ,
		DiscountValue:     &val,
		MaxDurationMonths: &months,
		MaxPerEmail:       1,
		ValidFrom:         time.Now().UTC().Add(-time.Hour),
	}
}

func validateInput() promo.ApplyInput {
	return promo.ApplyInput{
		Code:           "WINBACK20OFF6MONTHS",
		MerchantEmail:  "merchant@example.com",
		Plan:           subscription.PlanStarter,
		Period:         subscription.PeriodMonthly,
		Currency:       "usd",
		BasePriceMinor: promo.BasePriceMinorFor(subscription.PlanStarter, subscription.PeriodMonthly, subscription.PriceTierDeveloped, "usd"),
	}
}

func TestValidateCode_ReportsTheRowsOwnTerms(t *testing.T) {
	repo := &stubRepo{code: winBackRow()}
	svc := promo.NewService(nil, repo, nil, nil)

	out, err := svc.ValidateCode(context.Background(), validateInput())
	if err != nil {
		t.Fatalf("ValidateCode: %v", err)
	}
	if out.PercentOffBps != 2000 {
		t.Errorf("PercentOffBps = %d, want 2000", out.PercentOffBps)
	}
	if out.MaxDurationMonths != 6 {
		t.Errorf("MaxDurationMonths = %d, want 6", out.MaxDurationMonths)
	}
	if out.StripeCouponID != "co_test" {
		t.Errorf("StripeCouponID = %q", out.StripeCouponID)
	}
}

// The property the win-back depends on: asking whether a code would be
// accepted must not consume it. max_per_email is 1, so a redemption recorded
// at email time is a code the merchant can never use.
func TestValidateCode_RecordsNothing(t *testing.T) {
	repo := &stubRepo{code: winBackRow()}
	svc := promo.NewService(nil, repo, nil, nil)

	if _, err := svc.ValidateCode(context.Background(), validateInput()); err != nil {
		t.Fatalf("ValidateCode: %v", err)
	}
	if repo.writeCalls != 0 {
		t.Fatalf("ValidateCode performed %d writes; it must record nothing", repo.writeCalls)
	}
}

func TestValidateCode_MissingRowIsInvalidOrExpired(t *testing.T) {
	repo := &stubRepo{getErr: apperrors.NotFound("promo code")}
	svc := promo.NewService(nil, repo, nil, nil)

	out, err := svc.ValidateCode(context.Background(), validateInput())
	if !errors.Is(err, promo.ErrInvalidOrExpired) {
		t.Fatalf("err = %v, want ErrInvalidOrExpired", err)
	}
	if out.RejectReason != promo.RejectReasonNotFound {
		t.Errorf("reject reason = %q", out.RejectReason)
	}
}

func TestValidateCode_ExhaustedGlobalCapIsRejected(t *testing.T) {
	row := winBackRow()
	max := 100
	row.MaxRedemptions = &max
	repo := &stubRepo{code: row, total: 100}
	svc := promo.NewService(nil, repo, nil, nil)

	out, err := svc.ValidateCode(context.Background(), validateInput())
	if !errors.Is(err, promo.ErrInvalidOrExpired) {
		t.Fatalf("err = %v, want ErrInvalidOrExpired", err)
	}
	if out.RejectReason != promo.RejectReasonMaxRedemptions {
		t.Errorf("reject reason = %q, want max_redemptions_reached", out.RejectReason)
	}
}

func TestValidateForSaveOffer_IsValidateCode(t *testing.T) {
	repo := &stubRepo{code: winBackRow()}
	svc := promo.NewService(nil, repo, nil, nil)

	a, errA := svc.ValidateForSaveOffer(context.Background(), validateInput())
	b, errB := svc.ValidateCode(context.Background(), validateInput())
	if errA != nil || errB != nil {
		t.Fatalf("errs: %v %v", errA, errB)
	}
	if a != b {
		t.Fatalf("ValidateForSaveOffer = %+v, ValidateCode = %+v", a, b)
	}
}
