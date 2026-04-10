package coupon

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

func TestService_validateCreateInput(t *testing.T) {
	svc := &Service{}

	tests := []struct {
		name    string
		input   CreateInput
		wantErr bool
		errCode apperrors.Code
	}{
		{
			name:    "empty code",
			input:   CreateInput{Code: "", Title: "Test", Type: "percentage", Value: decimal.NewFromInt(10)},
			wantErr: true,
			errCode: apperrors.CodeValidationFailed,
		},
		{
			name:    "code too long",
			input:   CreateInput{Code: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", Title: "T", Type: "percentage", Value: decimal.NewFromInt(10)},
			wantErr: true,
			errCode: apperrors.CodeValidationFailed,
		},
		{
			name:    "empty title",
			input:   CreateInput{Code: "SAVE10", Title: "", Type: "percentage", Value: decimal.NewFromInt(10)},
			wantErr: true,
			errCode: apperrors.CodeValidationFailed,
		},
		{
			name:    "invalid type",
			input:   CreateInput{Code: "SAVE10", Title: "T", Type: "bogus", Value: decimal.NewFromInt(10)},
			wantErr: true,
			errCode: apperrors.CodeValidationFailed,
		},
		{
			name:    "negative value",
			input:   CreateInput{Code: "SAVE10", Title: "T", Type: "percentage", Value: decimal.NewFromInt(-5)},
			wantErr: true,
			errCode: apperrors.CodeValidationFailed,
		},
		{
			name:    "percentage over 100",
			input:   CreateInput{Code: "SAVE10", Title: "T", Type: "percentage", Value: decimal.NewFromInt(150)},
			wantErr: true,
			errCode: apperrors.CodeValidationFailed,
		},
		{
			name:    "fixed_amount without currency",
			input:   CreateInput{Code: "FLAT10", Title: "T", Type: "fixed_amount", Value: decimal.NewFromInt(10)},
			wantErr: true,
			errCode: apperrors.CodeValidationFailed,
		},
		{
			name: "valid percentage",
			input: CreateInput{
				Code:  "SAVE20",
				Title: "Save 20%",
				Type:  "percentage",
				Value: decimal.NewFromInt(20),
			},
			wantErr: false,
		},
		{
			name: "valid fixed_amount",
			input: func() CreateInput {
				cur := "USD"
				return CreateInput{
					Code:         "FLAT10",
					Title:        "Flat $10 off",
					Type:         "fixed_amount",
					Value:        decimal.NewFromInt(10),
					CurrencyCode: &cur,
				}
			}(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.validateCreateInput(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				var ae *apperrors.Error
				if !errors.As(err, &ae) {
					t.Fatalf("expected *apperrors.Error, got %T", err)
				}
				if ae.Code != tt.errCode {
					t.Errorf("expected code %q, got %q", tt.errCode, ae.Code)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateResult_FreeShipping(t *testing.T) {
	r := ValidateResult{
		Code:         "FREESHIP",
		Type:         CouponTypeFreeShipping,
		Value:        decimal.Zero,
		FreeShipping: true,
	}
	if !r.FreeShipping {
		t.Error("expected FreeShipping to be true")
	}
}

func TestNewCouponApplier(t *testing.T) {
	svc := &Service{}
	a := NewCouponApplier(svc, "SAVE20", "test@example.com")
	if a == nil {
		t.Fatal("expected non-nil applier")
	}
	if a.code != "SAVE20" {
		t.Errorf("expected code SAVE20, got %q", a.code)
	}
	if a.customerEmail != "test@example.com" {
		t.Errorf("expected email test@example.com, got %q", a.customerEmail)
	}
}

func TestNewService(t *testing.T) {
	svc := NewService(ServiceConfig{})
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestPatch_StatusValidation(t *testing.T) {
	svc := &Service{}

	// Invalid status "expired" should fail validation before DB lookup.
	expired := "expired"
	_, err := svc.Patch(nil, [16]byte{}, [16]byte{}, PatchInput{Status: &expired})
	if err == nil {
		t.Fatal("expected error for expired status, got nil")
	}
	var ae *apperrors.Error
	if !errors.As(err, &ae) {
		t.Fatalf("expected *apperrors.Error, got %T", err)
	}
	if ae.Code != apperrors.CodeValidationFailed {
		t.Errorf("expected code %q, got %q", apperrors.CodeValidationFailed, ae.Code)
	}
}
