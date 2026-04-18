package subscription

import (
	"context"
	"fmt"
	"log/slog"
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
	Type               string
	StripeCustomerID   string
	StripeSubscriptionID string
	Plan               string
	Status             string
	CurrentPeriodStart *time.Time
	CurrentPeriodEnd   *time.Time
	CancelAtPeriodEnd  bool
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
