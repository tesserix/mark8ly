package email_test

import (
	"errors"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/email"
)

func TestValidateRecipient(t *testing.T) {
	cases := []struct {
		name       string
		to         string
		wantReason string // "" means deliverable
	}{
		{"plain address", "merchant@example.com", ""},
		{"address with display parts trimmed", "  merchant@example.com  ", ""},
		{"subaddressed", "billing+store@example.com", ""},
		{"empty", "", email.ReasonNoAddress},
		{"whitespace only", "   ", email.ReasonNoAddress},
		{"no at sign", "merchant.example.com", email.ReasonInvalidAddress},
		{"two at signs", "a@b@example.com", email.ReasonInvalidAddress},
		{"empty local part", "@example.com", email.ReasonInvalidAddress},
		{"empty domain", "merchant@", email.ReasonInvalidAddress},
		{"domain without dot", "merchant@localhost", email.ReasonPlaceholderAddress},
		{"bootstrap placeholder", "billing+7f3a@mark8ly.local", email.ReasonPlaceholderAddress},
		{"uppercase placeholder", "ops@MARK8LY.LOCAL", email.ReasonPlaceholderAddress},
		{"rfc2606 invalid", "ops@something.invalid", email.ReasonPlaceholderAddress},
		{"rfc2606 test", "ops@something.test", email.ReasonPlaceholderAddress},
		{"rfc2606 example", "ops@something.example", email.ReasonPlaceholderAddress},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := email.ValidateRecipient(tc.to)
			if tc.wantReason == "" {
				if err != nil {
					t.Fatalf("ValidateRecipient(%q) = %v, want nil", tc.to, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateRecipient(%q) = nil, want %s", tc.to, tc.wantReason)
			}
			if !errors.Is(err, email.ErrUndeliverable) {
				t.Errorf("errors.Is(err, ErrUndeliverable) = false for %q", tc.to)
			}
			reason, ok := email.UndeliverableReason(err)
			if !ok {
				t.Fatalf("UndeliverableReason(%v) not recognised", err)
			}
			if reason != tc.wantReason {
				t.Errorf("reason = %q, want %q", reason, tc.wantReason)
			}
		})
	}
}

func TestUndeliverableReason_UnrelatedError(t *testing.T) {
	if _, ok := email.UndeliverableReason(errors.New("boom")); ok {
		t.Error("unrelated error reported as undeliverable")
	}
}
