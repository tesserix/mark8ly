package stripe_test

import (
	"context"
	"testing"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
)

func TestGetCustomerEmail_EmptyCustomerID(t *testing.T) {
	got, err := billingstripe.GetCustomerEmail(context.Background(), nil, "")
	if err != nil {
		t.Fatalf("err = %v, want nil for an empty customer id", err)
	}
	if got != "" {
		t.Errorf("email = %q, want empty", got)
	}
}
