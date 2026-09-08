package subscription

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// StripeClient abstracts Stripe API calls so the service can be tested
// without real Stripe credentials. Production wiring injects a real
// implementation; tests inject a stub.
type StripeClient interface {
	// CreateCustomer creates a Stripe customer and returns its ID.
	CreateCustomer(ctx context.Context, email, name string) (customerID string, err error)

	// CreateCheckoutSession creates a Stripe Checkout Session and returns the URL.
	CreateCheckoutSession(ctx context.Context, customerID string, plan SubscriptionPlan, successURL, cancelURL string) (sessionURL string, err error)

	// CreatePortalSession creates a Stripe Billing Portal Session and returns the URL.
	CreatePortalSession(ctx context.Context, customerID, returnURL string) (portalURL string, err error)
}

// ServiceConfig groups dependencies for the subscription service.
type ServiceConfig struct {
	DB     *gorm.DB
	Repo   Repository
	Stripe StripeClient
	Logger *slog.Logger
}

// Service implements subscription CRUD and Stripe integration.
type Service struct {
	db     *gorm.DB
	repo   Repository
	stripe StripeClient
	logger *slog.Logger
}

// NewService constructs a subscription Service.
func NewService(cfg ServiceConfig) *Service {
	return &Service{
		db:     cfg.DB,
		repo:   cfg.Repo,
		stripe: cfg.Stripe,
		logger: cfg.Logger,
	}
}

// GetSubscription returns the subscription for a (tenant, store) pair.
// tenantID is mandatory: the repo filters by both columns so a foreign
// tenant cannot read another tenant's subscription by guessing its store_id.
func (s *Service) GetSubscription(ctx context.Context, tenantID, storeID uuid.UUID) (*StoreSubscription, error) {
	return s.repo.GetByStoreID(ctx, s.db, tenantID, storeID)
}

// BootstrapInput holds the parameters for lazy-initialising a subscription
// row for a store that was created before the v2.3 signup → row pipeline
// shipped. Email + name feed the Stripe customer record.
type BootstrapInput struct {
	TenantID uuid.UUID
	StoreID  uuid.UUID
	Email    string
	Name     string
	// BillingCurrency is the store's ISO 4217 currency, lower-case, when the
	// caller knows it — onboarding does. Left empty it stays NULL, which is
	// what every reader already treats as "not known yet"; "" would be a
	// third meaning nothing tests for.
	BillingCurrency string
	// TaxID and TaxIDCountry are what the merchant typed in onboarding's Tax
	// ID field, unvalidated.
	//
	// Stored here because nothing else ever stored them. The field has been
	// on the form since §5.1.1 and was written to the onboarding draft and
	// read by nobody; reverse_charge_tax_id had no writer anywhere in the
	// tree, while the revalidation cron and the reverse-charge invoice
	// annotation both READ it. This closes that gap at the only point the
	// value is known.
	//
	// tax_id_validated stays false — its default. Both downstream consumers
	// gate on it (invoice_finalized.go, revalidation/cron.go), so an
	// unvalidated id changes no behaviour until the tax service validates it.
	// Recording an unchecked claim as checked is the one thing that would.
	TaxID        string
	TaxIDCountry string
}

// Bootstrap idempotently initialises a store_subscriptions row: plan=trial,
// status=signup. The status machine (P2) owns the signup → trialing
// transition once the merchant confirms a plan.
//
// IT MAKES NO STRIPE CALL. That is the fix for #827, and it is deliberate in
// both directions:
//
//   - The row's created_at IS the trial clock (trial.EndsAt derives from it
//     when trial_ends_at is NULL). Making a free trial depend on a payment
//     provider being reachable is what turns a Stripe outage — or the
//     mis-scoped key #696 describes — into a merchant with no trial at all.
//   - Attaching a customer has its own failure mode, and folding it in here
//     is what let a Stripe problem present as "the trial did not start"
//     rather than as a Stripe problem. See EnsureStripeCustomer.
//
// A row with an empty stripe_customer_id is a state this codebase already
// expects: the reconciler, hard-delete, the billing archive and the admin
// payment-method lookup all guard it and degrade, and the PAID path refuses
// it outright (planchange.go, "run bootstrap first"). That refusal is what
// makes a customer-less trial row safe to create.
//
// Called at onboarding completion, which is what makes the trial start at
// signup, and still by the admin billing page for a store that predates it.
func (s *Service) Bootstrap(ctx context.Context, in BootstrapInput) (*StoreSubscription, error) {
	if in.TenantID == uuid.Nil {
		return nil, apperrors.ValidationFailed("tenant_id", "tenant_id is required")
	}
	if in.StoreID == uuid.Nil {
		return nil, apperrors.ValidationFailed("store_id", "store_id is required")
	}
	// Idempotency — return the existing row if one is already in place.
	existing, err := s.repo.GetByStoreID(ctx, s.db, in.TenantID, in.StoreID)
	if err == nil {
		return existing, nil
	}
	if ae, ok := err.(*apperrors.Error); !ok || ae.Code != apperrors.CodeNotFound {
		return nil, err
	}

	row := &StoreSubscription{
		TenantID: in.TenantID,
		StoreID:  in.StoreID,
		Plan:     PlanTrial,
		// trialing, not signup. §17.2's own trigger for this transition is
		// "email verified", and the only caller reaches here from onboarding
		// completion, which cannot happen until the merchant has clicked the
		// magic link.
		//
		// signup would be worse than merely inaccurate. It is documented as a
		// RESTING STATE that only the Stripe checkout webhook promotes, and
		// ExpiryCron selects `trialing` — so a row left in signup has a
		// trial_ends_at that NOTHING acts on. #827 started the clock; this is
		// what makes it ring. Four backfilled stores sat in exactly that
		// state, two of them weeks past their end date and invisible to
		// every sweep.
		//
		// trialing is also what the rest of the table expects of these rows:
		// `trialing → expired (day 90, no card)` describes them precisely,
		// and `trialing → active (card added)` is where the deferred-charge
		// flow takes them next.
		Status: StatusTrialing,
	}
	if cur := strings.ToLower(strings.TrimSpace(in.BillingCurrency)); cur != "" {
		row.BillingCurrency = &cur
	}
	// Both or neither: a tax id without its country cannot be validated (the
	// validator dispatches on country) and cannot be rendered on a
	// reverse-charge invoice, which prints the pair. Storing half of it would
	// leave a value that reads as present and can never be used.
	taxID := strings.TrimSpace(in.TaxID)
	taxCountry := strings.ToUpper(strings.TrimSpace(in.TaxIDCountry))
	if taxID != "" && len(taxCountry) == 2 {
		row.ReverseChargeTaxID = &taxID
		row.TaxIDCountry = &taxCountry
	}
	if err := s.repo.Create(ctx, s.db, row); err != nil {
		return nil, fmt.Errorf("bootstrap: create subscription row: %w", err)
	}
	return row, nil
}

// EnsureStripeCustomer attaches a Stripe customer to a subscription row that
// has none, and returns the row either way. Idempotent: a row that already
// carries a customer id is returned untouched and no customer is minted.
//
// Split out of Bootstrap by #827 so the two can fail independently. A Stripe
// failure here is returned, never logged-and-swallowed: an unusable billing
// key is the thing that must be visible, and #827 exists because a billing
// precondition failed quietly for months.
//
// Callers: the admin bootstrap CTA (after Bootstrap, so the trial has started
// even when this half fails) and the card-add path, which cannot create a
// Stripe subscription without one.
func (s *Service) EnsureStripeCustomer(ctx context.Context, tenantID, storeID uuid.UUID,
	email, name string) (*StoreSubscription, error) {

	sub, err := s.repo.GetByStoreID(ctx, s.db, tenantID, storeID)
	if err != nil {
		return nil, err
	}
	if sub.StripeCustomerID != "" {
		return sub, nil
	}
	if s.stripe == nil {
		return nil, fmt.Errorf("ensure stripe customer: stripe client not configured")
	}

	if email == "" {
		// Stripe requires non-empty customer email for reliable dedup;
		// fall back to a deterministic tenant-scoped placeholder so the
		// customer record is still unique per tenant/store.
		email = fmt.Sprintf("billing+%s@mark8ly.local", storeID.String())
	}
	if name == "" {
		name = storeID.String()
	}

	customerID, err := s.stripe.CreateCustomer(ctx, email, name)
	if err != nil {
		return nil, fmt.Errorf("ensure stripe customer: create stripe customer: %w", err)
	}

	sub.StripeCustomerID = customerID
	if err := s.repo.Update(ctx, s.db, sub); err != nil {
		// The customer exists at Stripe but we did not record it. Saying so
		// is the point: a retry mints a SECOND customer for this store, and
		// whoever reads this line is the only one who can tell that the
		// duplicate came from here.
		return nil, fmt.Errorf("ensure stripe customer: stripe customer %s created but not recorded: %w",
			customerID, err)
	}
	return sub, nil
}

// CheckoutInput holds the parameters for creating a checkout session.
type CheckoutInput struct {
	TenantID   uuid.UUID
	StoreID    uuid.UUID
	Plan       string
	SuccessURL string
	CancelURL  string
}

// CreateCheckoutSession creates or retrieves a Stripe customer and returns
// a Checkout Session URL for upgrading/subscribing.
func (s *Service) CreateCheckoutSession(ctx context.Context, in CheckoutInput) (string, error) {
	if s.stripe == nil {
		return "", fmt.Errorf("stripe client not configured")
	}

	plan := SubscriptionPlan(in.Plan)
	if !isValidPlan(plan) {
		return "", apperrors.ValidationFailed("plan", "plan must be trial, starter, studio, or pro")
	}
	if in.SuccessURL == "" || in.CancelURL == "" {
		return "", apperrors.ValidationFailed("urls", "success_url and cancel_url are required")
	}

	// Get or create subscription record with Stripe customer.
	sub, err := s.repo.GetByStoreID(ctx, s.db, in.TenantID, in.StoreID)
	if err != nil {
		// If no subscription exists yet, we need the caller to create
		// a Stripe customer first — handled in the handler layer.
		return "", err
	}

	url, err := s.stripe.CreateCheckoutSession(ctx, sub.StripeCustomerID, plan, in.SuccessURL, in.CancelURL)
	if err != nil {
		return "", fmt.Errorf("stripe checkout session: %w", err)
	}
	return url, nil
}

// CreatePortalSession returns a Stripe Billing Portal URL for the (tenant,
// store) pair. tenantID is mandatory — the repo filters by both columns so
// a foreign tenant cannot open another tenant's billing portal.
func (s *Service) CreatePortalSession(ctx context.Context, tenantID, storeID uuid.UUID, returnURL string) (string, error) {
	if s.stripe == nil {
		return "", fmt.Errorf("stripe client not configured")
	}
	if returnURL == "" {
		return "", apperrors.ValidationFailed("return_url", "return_url is required")
	}

	sub, err := s.repo.GetByStoreID(ctx, s.db, tenantID, storeID)
	if err != nil {
		return "", err
	}

	url, err := s.stripe.CreatePortalSession(ctx, sub.StripeCustomerID, returnURL)
	if err != nil {
		return "", fmt.Errorf("stripe portal session: %w", err)
	}
	return url, nil
}

// WebhookEvent represents a parsed Stripe webhook event.
type WebhookEvent struct {
	Type                 string
	StripeCustomerID     string
	StripeSubscriptionID string
	Plan                 string
	Status               string
	CurrentPeriodStart   *time.Time
	CurrentPeriodEnd     *time.Time
	CancelAtPeriodEnd    bool
}

// HandleWebhookEvent processes a Stripe billing webhook event and updates
// the local subscription record.
func (s *Service) HandleWebhookEvent(ctx context.Context, evt WebhookEvent) error {
	sub, err := s.repo.GetByStripeCustomerID(ctx, s.db, evt.StripeCustomerID)
	if err != nil {
		return fmt.Errorf("webhook: find subscription: %w", err)
	}

	if evt.StripeSubscriptionID != "" {
		sub.StripeSubscriptionID = &evt.StripeSubscriptionID
	}
	if evt.Plan != "" {
		sub.Plan = SubscriptionPlan(evt.Plan)
	}
	if evt.Status != "" {
		sub.Status = SubscriptionStatus(evt.Status)
	}
	if evt.CurrentPeriodStart != nil {
		sub.CurrentPeriodStart = evt.CurrentPeriodStart
	}
	if evt.CurrentPeriodEnd != nil {
		sub.CurrentPeriodEnd = evt.CurrentPeriodEnd
	}
	sub.CancelAtPeriodEnd = evt.CancelAtPeriodEnd
	sub.UpdatedAt = time.Now()

	return s.repo.Update(ctx, s.db, sub)
}

func isValidPlan(p SubscriptionPlan) bool {
	switch p {
	case PlanTrial, PlanStarter, PlanStudio, PlanPro:
		return true
	}
	return false
}
