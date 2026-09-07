package subscription_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// bootstrapRepo is the subset of subscription.Repository Bootstrap touches,
// with the rest present only to satisfy the interface.
type bootstrapRepo struct {
	existing  *subscription.StoreSubscription
	created   []subscription.StoreSubscription
	updated   []subscription.StoreSubscription
	createErr error
}

func (r *bootstrapRepo) GetByStoreID(_ context.Context, _ *gorm.DB, _, _ uuid.UUID) (*subscription.StoreSubscription, error) {
	if r.existing == nil {
		return nil, apperrors.NotFound("subscription")
	}
	return r.existing, nil
}

func (r *bootstrapRepo) Create(_ context.Context, _ *gorm.DB, s *subscription.StoreSubscription) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.created = append(r.created, *s)
	r.existing = s
	return nil
}

func (r *bootstrapRepo) Update(_ context.Context, _ *gorm.DB, s *subscription.StoreSubscription) error {
	r.updated = append(r.updated, *s)
	return nil
}

func (r *bootstrapRepo) GetByStripeCustomerID(context.Context, *gorm.DB, string) (*subscription.StoreSubscription, error) {
	return nil, apperrors.NotFound("subscription")
}
func (r *bootstrapRepo) SetPendingDowngrade(context.Context, *gorm.DB, uuid.UUID, uuid.UUID,
	subscription.SubscriptionPlan, subscription.SubscriptionPeriod, time.Time, string) error {
	return nil
}
func (r *bootstrapRepo) ClearPendingDowngrade(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) error {
	return nil
}
func (r *bootstrapRepo) CommitDowngrade(context.Context, *gorm.DB, uuid.UUID, uuid.UUID,
	subscription.SubscriptionPlan, subscription.SubscriptionPeriod) error {
	return nil
}
func (r *bootstrapRepo) CommitUpgrade(context.Context, *gorm.DB, uuid.UUID, uuid.UUID,
	subscription.SubscriptionPlan, subscription.SubscriptionPeriod, string) error {
	return nil
}
func (r *bootstrapRepo) FindPendingDowngradesReady(context.Context, *gorm.DB, time.Time) ([]subscription.StoreSubscription, error) {
	return nil, nil
}
func (r *bootstrapRepo) CountStoresForPlanSlot(context.Context, *gorm.DB, uuid.UUID) (int, error) {
	return 0, nil
}
func (r *bootstrapRepo) ListAllSubscriptions(context.Context, *gorm.DB, subscription.CrossTenantFilter) ([]subscription.StoreSubscription, int64, error) {
	return nil, 0, nil
}

// recordingStripe counts customer creations so a test can prove Bootstrap
// makes none.
type recordingStripe struct {
	created int
	err     error
}

func (s *recordingStripe) CreateCustomer(context.Context, string, string) (string, error) {
	s.created++
	if s.err != nil {
		return "", s.err
	}
	return "cus_test", nil
}
func (s *recordingStripe) CreateCheckoutSession(context.Context, string, subscription.SubscriptionPlan, string, string) (string, error) {
	return "", nil
}
func (s *recordingStripe) CreatePortalSession(context.Context, string, string) (string, error) {
	return "", nil
}

func newSvc(repo subscription.Repository, stripe subscription.StripeClient) *subscription.Service {
	return subscription.NewService(subscription.ServiceConfig{DB: nil, Repo: repo, Stripe: stripe})
}

func bootstrapInput() subscription.BootstrapInput {
	return subscription.BootstrapInput{
		TenantID: uuid.New(),
		StoreID:  uuid.New(),
		Email:    "founder@example.com",
		Name:     "Bondi Surf Co",
	}
}

// The point of #827. A free trial must not depend on a payment provider being
// reachable: that dependency is what turns a mis-scoped Stripe key into a
// merchant with no trial at all.
func TestBootstrap_CreatesTheRowWithNoStripeClientAtAll(t *testing.T) {
	repo := &bootstrapRepo{}
	svc := newSvc(repo, nil)

	sub, err := svc.Bootstrap(context.Background(), bootstrapInput())
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("created %d rows, want 1", len(repo.created))
	}
	if sub.Plan != subscription.PlanTrial {
		t.Errorf("plan = %q, want trial", sub.Plan)
	}
	if sub.Status != subscription.StatusSignup {
		t.Errorf("status = %q, want signup", sub.Status)
	}
	if sub.StripeCustomerID != "" {
		t.Errorf("StripeCustomerID = %q, want empty — Bootstrap must not mint one", sub.StripeCustomerID)
	}
}

// Even WITH a client. Attaching a customer is a separate responsibility with a
// separate failure mode; folding it in is what let a Stripe problem present as
// "no trial" instead of as a Stripe problem.
func TestBootstrap_MakesNoStripeCallEvenWhenAClientIsWired(t *testing.T) {
	stripe := &recordingStripe{}
	svc := newSvc(&bootstrapRepo{}, stripe)

	if _, err := svc.Bootstrap(context.Background(), bootstrapInput()); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if stripe.created != 0 {
		t.Fatalf("Bootstrap made %d Stripe customer calls, want 0", stripe.created)
	}
}

// The billing currency is known at signup and nothing else supplies it later.
// A row without one cannot have its price resolved.
func TestBootstrap_RecordsTheBillingCurrencyWhenTheCallerKnowsIt(t *testing.T) {
	repo := &bootstrapRepo{}
	svc := newSvc(repo, nil)

	in := bootstrapInput()
	in.BillingCurrency = "aud"
	sub, err := svc.Bootstrap(context.Background(), in)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if sub.BillingCurrency == nil || *sub.BillingCurrency != "aud" {
		t.Fatalf("BillingCurrency = %v, want aud", sub.BillingCurrency)
	}
}

// An unknown currency must stay NULL rather than becoming "". The column is
// CHAR(3) and every reader treats nil as "not known yet"; "" would be a third
// meaning nothing tests for.
func TestBootstrap_LeavesTheCurrencyNullWhenNotSupplied(t *testing.T) {
	svc := newSvc(&bootstrapRepo{}, nil)

	sub, err := svc.Bootstrap(context.Background(), bootstrapInput())
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if sub.BillingCurrency != nil {
		t.Fatalf("BillingCurrency = %q, want nil", *sub.BillingCurrency)
	}
}

// Unchanged from before #827, and load-bearing: onboarding retries, and the
// admin CTA can still be pressed on a row that already exists.
func TestBootstrap_IsIdempotent(t *testing.T) {
	existing := &subscription.StoreSubscription{
		ID: uuid.New(), Plan: subscription.PlanStarter, Status: subscription.StatusActive,
	}
	repo := &bootstrapRepo{existing: existing}
	svc := newSvc(repo, nil)

	sub, err := svc.Bootstrap(context.Background(), bootstrapInput())
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if len(repo.created) != 0 {
		t.Fatal("Bootstrap inserted a second row for a store that already had one")
	}
	if sub.Plan != subscription.PlanStarter {
		t.Errorf("returned plan = %q — the existing row was overwritten", sub.Plan)
	}
}

// ---------------------------------------------------------------------------
// EnsureStripeCustomer — the other half, failing on its own terms
// ---------------------------------------------------------------------------

func TestEnsureStripeCustomer_AttachesToARowThatHasNone(t *testing.T) {
	in := bootstrapInput()
	repo := &bootstrapRepo{existing: &subscription.StoreSubscription{
		TenantID: in.TenantID, StoreID: in.StoreID, StripeCustomerID: "",
	}}
	stripe := &recordingStripe{}
	svc := newSvc(repo, stripe)

	sub, err := svc.EnsureStripeCustomer(context.Background(), in.TenantID, in.StoreID, in.Email, in.Name)
	if err != nil {
		t.Fatalf("EnsureStripeCustomer: %v", err)
	}
	if sub.StripeCustomerID != "cus_test" {
		t.Errorf("StripeCustomerID = %q, want cus_test", sub.StripeCustomerID)
	}
	if len(repo.updated) != 1 {
		t.Errorf("persisted %d updates, want 1", len(repo.updated))
	}
}

func TestEnsureStripeCustomer_IsIdempotent(t *testing.T) {
	in := bootstrapInput()
	repo := &bootstrapRepo{existing: &subscription.StoreSubscription{
		TenantID: in.TenantID, StoreID: in.StoreID, StripeCustomerID: "cus_already",
	}}
	stripe := &recordingStripe{}
	svc := newSvc(repo, stripe)

	sub, err := svc.EnsureStripeCustomer(context.Background(), in.TenantID, in.StoreID, in.Email, in.Name)
	if err != nil {
		t.Fatalf("EnsureStripeCustomer: %v", err)
	}
	if stripe.created != 0 {
		t.Errorf("minted a second customer for a row that already had one")
	}
	if sub.StripeCustomerID != "cus_already" {
		t.Errorf("StripeCustomerID = %q", sub.StripeCustomerID)
	}
}

// The failure #827 exists to stop being invisible. A broken Stripe key must
// surface here, loudly, rather than being folded into "the trial did not start".
func TestEnsureStripeCustomer_SurfacesTheStripeError(t *testing.T) {
	in := bootstrapInput()
	repo := &bootstrapRepo{existing: &subscription.StoreSubscription{
		TenantID: in.TenantID, StoreID: in.StoreID,
	}}
	svc := newSvc(repo, &recordingStripe{err: errors.New("more_permissions_required")})

	if _, err := svc.EnsureStripeCustomer(context.Background(), in.TenantID, in.StoreID, in.Email, in.Name); err == nil {
		t.Fatal("EnsureStripeCustomer swallowed a Stripe failure")
	}
	if len(repo.updated) != 0 {
		t.Error("persisted an update despite the Stripe call failing")
	}
}

func TestEnsureStripeCustomer_RefusesWithNoStripeConfigured(t *testing.T) {
	in := bootstrapInput()
	repo := &bootstrapRepo{existing: &subscription.StoreSubscription{
		TenantID: in.TenantID, StoreID: in.StoreID,
	}}
	svc := newSvc(repo, nil)

	if _, err := svc.EnsureStripeCustomer(context.Background(), in.TenantID, in.StoreID, in.Email, in.Name); err == nil {
		t.Fatal("EnsureStripeCustomer succeeded with no Stripe client")
	}
}
