package subscription_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// portalStripe records what customer id the portal call actually received.
type portalStripe struct {
	recordingStripe
	portalCustomerID string
	portalCalls      int
}

func (s *portalStripe) CreatePortalSession(_ context.Context, customerID, _ string) (string, error) {
	s.portalCalls++
	s.portalCustomerID = customerID
	return "https://billing.stripe.test/session", nil
}

func subWithoutCustomer() *subscription.StoreSubscription {
	email := "merchant@example.com"
	return &subscription.StoreSubscription{
		TenantID:         uuid.New(),
		StoreID:          uuid.New(),
		Status:           subscription.StatusStoreClosed,
		Plan:             subscription.PlanTrial,
		Email:            &email,
		StripeCustomerID: "",
	}
}

// Opening the billing portal for a subscription with no Stripe customer must
// create one first.
//
// Since #827 a subscription row is deliberately created WITHOUT a Stripe
// customer, so EVERY store on the platform is in this state until money is
// first discussed. This path handed Stripe the empty string, which it
// rejects — surfacing as "Add a card", "Add payment method" and "Manage in
// portal" all returning 500.
func TestCreatePortalSession_CreatesTheCustomerWhenThereIsNone(t *testing.T) {
	stripe := &portalStripe{}
	repo := &bootstrapRepo{existing: subWithoutCustomer()}
	svc := newSvc(repo, stripe)

	url, err := svc.CreatePortalSession(
		context.Background(), repo.existing.TenantID, repo.existing.StoreID,
		"https://admin.example.com/settings/billing",
	)
	if err != nil {
		t.Fatalf("CreatePortalSession: %v", err)
	}
	if url == "" {
		t.Error("no portal url returned")
	}
	if stripe.created != 1 {
		t.Errorf("CreateCustomer called %d times, want 1", stripe.created)
	}
	if stripe.portalCustomerID != "cus_test" {
		t.Errorf("portal opened for customer %q; an empty value here is what Stripe rejects",
			stripe.portalCustomerID)
	}
}

// A subscription that already has a customer must not get a second one —
// EnsureStripeCustomer's own doc warns that a duplicate is unrecoverable
// from our side.
func TestCreatePortalSession_DoesNotMintASecondCustomer(t *testing.T) {
	existing := subWithoutCustomer()
	existing.StripeCustomerID = "cus_already_there"

	stripe := &portalStripe{}
	repo := &bootstrapRepo{existing: existing}
	svc := newSvc(repo, stripe)

	if _, err := svc.CreatePortalSession(
		context.Background(), existing.TenantID, existing.StoreID,
		"https://admin.example.com/settings/billing",
	); err != nil {
		t.Fatalf("CreatePortalSession: %v", err)
	}
	if stripe.created != 0 {
		t.Errorf("CreateCustomer called %d times for a subscription that already had one", stripe.created)
	}
	if stripe.portalCustomerID != "cus_already_there" {
		t.Errorf("portal opened for %q, want the existing customer", stripe.portalCustomerID)
	}
}
