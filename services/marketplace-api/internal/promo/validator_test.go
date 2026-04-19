package promo

import (
	"testing"
	"time"

	"github.com/mark8ly/marketplace-api/internal/subscription"
)

var baseCode = &PromoCode{
	Code:          "ABCDEF123456",
	DiscountType:  DiscountTypePercentage,
	DiscountValue: 1000, // 10% off
	MaxPerEmail:   1,
	ValidFrom:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
}

var now = time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)

func TestValidate(t *testing.T) {
	tests := []struct {
		name        string
		in          ValidationInput
		wantAccepted bool
		wantReason   ValidationRejectReason
	}{
		{
			name: "valid code accepted",
			in: ValidationInput{
				SubmittedCode:    "ABCDEF123456",
				PromoCode:        baseCode,
				Now:              now,
				TotalRedemptions: 0,
				EmailRedemptions: 0,
				Plan:             subscription.PlanStarter,
				Period:           subscription.PeriodMonthly,
				BasePriceMinor:   2000,
				Currency:         "usd",
			},
			wantAccepted: true,
		},
		{
			name: "wrong code rejected (timing-safe)",
			in: ValidationInput{
				SubmittedCode:    "ABCDEF12345X",
				PromoCode:        baseCode,
				Now:              now,
				TotalRedemptions: 0,
				EmailRedemptions: 0,
				Plan:             subscription.PlanStarter,
				Period:           subscription.PeriodMonthly,
				BasePriceMinor:   2000,
				Currency:         "usd",
			},
			wantAccepted: false,
			wantReason:   RejectReasonNotFound,
		},
		{
			name: "expired valid_until",
			in: ValidationInput{
				SubmittedCode: "ABCDEF123456",
				PromoCode: func() *PromoCode {
					exp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
					pc := *baseCode
					pc.ValidUntil = &exp
					return &pc
				}(),
				Now:              now,
				TotalRedemptions: 0,
				EmailRedemptions: 0,
				Plan:             subscription.PlanStarter,
				Period:           subscription.PeriodMonthly,
				BasePriceMinor:   2000,
				Currency:         "usd",
			},
			wantAccepted: false,
			wantReason:   RejectReasonExpired,
		},
		{
			name: "max redemptions reached",
			in: ValidationInput{
				SubmittedCode: "ABCDEF123456",
				PromoCode: func() *PromoCode {
					max := 5
					pc := *baseCode
					pc.MaxRedemptions = &max
					return &pc
				}(),
				Now:              now,
				TotalRedemptions: 5,
				EmailRedemptions: 0,
				Plan:             subscription.PlanStarter,
				Period:           subscription.PeriodMonthly,
				BasePriceMinor:   2000,
				Currency:         "usd",
			},
			wantAccepted: false,
			wantReason:   RejectReasonMaxRedemptions,
		},
		{
			name: "max per email reached",
			in: ValidationInput{
				SubmittedCode:    "ABCDEF123456",
				PromoCode:        baseCode,
				Now:              now,
				TotalRedemptions: 0,
				EmailRedemptions: 1, // already used once, MaxPerEmail=1
				Plan:             subscription.PlanStarter,
				Period:           subscription.PeriodMonthly,
				BasePriceMinor:   2000,
				Currency:         "usd",
			},
			wantAccepted: false,
			wantReason:   RejectReasonMaxPerEmail,
		},
		{
			name: "wrong plan",
			in: ValidationInput{
				SubmittedCode: "ABCDEF123456",
				PromoCode: func() *PromoCode {
					pc := *baseCode
					pc.AllowedPlans = []string{"pro"}
					return &pc
				}(),
				Now:              now,
				TotalRedemptions: 0,
				EmailRedemptions: 0,
				Plan:             subscription.PlanStarter,
				Period:           subscription.PeriodMonthly,
				BasePriceMinor:   2000,
				Currency:         "usd",
			},
			wantAccepted: false,
			wantReason:   RejectReasonWrongPlan,
		},
		{
			name: "annual-only on monthly billing",
			in: ValidationInput{
				SubmittedCode: "ABCDEF123456",
				PromoCode: func() *PromoCode {
					pc := *baseCode
					pc.AnnualOnly = true
					return &pc
				}(),
				Now:              now,
				TotalRedemptions: 0,
				EmailRedemptions: 0,
				Plan:             subscription.PlanStarter,
				Period:           subscription.PeriodMonthly,
				BasePriceMinor:   2000,
				Currency:         "usd",
			},
			wantAccepted: false,
			wantReason:   RejectReasonAnnualOnly,
		},
		{
			// Spec criterion #40: 50% off ₹999 Starter → below absolute floor ₹800.
			name: "spec criterion 40 — 50% off INR 999 Starter below floor",
			in: ValidationInput{
				SubmittedCode: "ABCDEF123456",
				PromoCode: func() *PromoCode {
					pc := *baseCode
					pc.DiscountValue = 5000 // 50% in basis points
					return &pc
				}(),
				Now:              now,
				TotalRedemptions: 0,
				EmailRedemptions: 0,
				Plan:             subscription.PlanStarter,
				Period:           subscription.PeriodMonthly,
				BasePriceMinor:   99900, // ₹999 in paise
				Currency:         "inr",
			},
			wantAccepted: false,
			wantReason:   RejectReasonBelowFloor,
		},
		{
			name: "currency not covered",
			in: ValidationInput{
				SubmittedCode:    "ABCDEF123456",
				PromoCode:        baseCode,
				Now:              now,
				TotalRedemptions: 0,
				EmailRedemptions: 0,
				Plan:             subscription.PlanStarter,
				Period:           subscription.PeriodMonthly,
				BasePriceMinor:   1000,
				Currency:         "eur",
			},
			wantAccepted: false,
			wantReason:   RejectReasonCurrencyNotCovered,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := Validate(tc.in)
			if result.Accepted != tc.wantAccepted {
				t.Errorf("Validate().Accepted = %v, want %v (reason: %s)", result.Accepted, tc.wantAccepted, result.RejectReason)
			}
			if !tc.wantAccepted && result.RejectReason != tc.wantReason {
				t.Errorf("Validate().RejectReason = %q, want %q", result.RejectReason, tc.wantReason)
			}
		})
	}
}
