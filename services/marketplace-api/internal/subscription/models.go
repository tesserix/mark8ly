// Package subscription implements store billing subscription management
// for Settings S3.
package subscription

import (
	"time"

	"github.com/google/uuid"
)

// SubscriptionPlan enumerates the available billing plans (v2.3).
type SubscriptionPlan string

const (
	PlanTrial       SubscriptionPlan = "trial"
	PlanStarter     SubscriptionPlan = "starter"
	PlanStudio      SubscriptionPlan = "studio"
	PlanPro         SubscriptionPlan = "pro"
	PlanMarketplace SubscriptionPlan = "marketplace" // hidden from UI
)

// AllPublicPlans returns the plans that are shown on the public pricing page.
// Marketplace is excluded — it's internal-only and assigned by platform staff.
func AllPublicPlans() []SubscriptionPlan {
	return []SubscriptionPlan{PlanTrial, PlanStarter, PlanStudio, PlanPro}
}

// AllStatuses returns every subscription lifecycle state.
//
// Single source for callers that must reason about the complete set. The
// platform console's billing filter derives its accepted values from this
// minus an explicit hidden set, so a status listed here becomes filterable
// by default and hiding one is a deliberate act.
//
// Go cannot enumerate the constants of the SubscriptionStatus type, so this
// slice is hand-maintained and does NOT automatically track new consts
// added to the block below. What keeps it honest is
// TestAllStatuses_MatchesDatabaseCheckConstraint
// (status_check_constraint_integration_test.go), which verifies this list
// against the store_subscriptions.status CHECK constraint — the one
// mechanically-enforced "every valid status" set in the system. Add a new
// status here (and to the const block) only alongside a migration that
// updates that constraint, or the integration test fails.
func AllStatuses() []SubscriptionStatus {
	return []SubscriptionStatus{
		StatusSignup,
		StatusTrialing,
		StatusActive,
		StatusPastDue,
		StatusPaymentActionRequired,
		StatusCancelScheduled,
		StatusExpired,
		StatusStoreClosed,
		StatusPendingHardDelete,
		StatusHardDeleted,
	}
}

// SubscriptionStatus enumerates subscription lifecycle states (v2.3).
// The state machine that gates transitions between these values lives in
// P2 — this package only defines the enum itself.
type SubscriptionStatus string

const (
	StatusSignup                SubscriptionStatus = "signup"
	StatusTrialing              SubscriptionStatus = "trialing"
	StatusActive                SubscriptionStatus = "active"
	StatusPastDue               SubscriptionStatus = "past_due"
	StatusPaymentActionRequired SubscriptionStatus = "payment_action_required"
	StatusCancelScheduled       SubscriptionStatus = "cancel_scheduled"
	StatusExpired               SubscriptionStatus = "expired"
	StatusStoreClosed           SubscriptionStatus = "store_closed"
	StatusPendingHardDelete     SubscriptionStatus = "pending_hard_delete"
	StatusHardDeleted           SubscriptionStatus = "hard_deleted"
)

// PriceTier encodes whether the merchant is billed at the developed-market
// list price or a purchasing-power-parity (PPP) discounted tier.
type PriceTier string

const (
	PriceTierDeveloped PriceTier = "developed"
	PriceTierPPP       PriceTier = "ppp"
)

// TaxIDNameMatch captures the result of Stripe's tax ID + registered-name
// cross-check, used for B2B reverse-charge VAT compliance.
type TaxIDNameMatch string

const (
	TaxIDNameMatchMatched    TaxIDNameMatch = "matched"
	TaxIDNameMatchUnmatched  TaxIDNameMatch = "unmatched"
	TaxIDNameMatchNotChecked TaxIDNameMatch = "not_checked"
)

// SubscriptionPeriod is the recurring billing interval.
type SubscriptionPeriod string

const (
	PeriodMonthly SubscriptionPeriod = "monthly"
	PeriodAnnual  SubscriptionPeriod = "annual"
)

// StoreSubscription is the GORM model for the store_subscriptions table.
// Column definitions must stay aligned with migrations 036-040.
type StoreSubscription struct {
	ID                   uuid.UUID          `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID             uuid.UUID          `gorm:"column:tenant_id;type:uuid;not null;index:ss_tenant_idx"`
	StoreID              uuid.UUID          `gorm:"column:store_id;type:uuid;not null;uniqueIndex"`
	StripeCustomerID     string             `gorm:"column:stripe_customer_id;type:varchar(100);not null"`
	StripeSubscriptionID *string            `gorm:"column:stripe_subscription_id;type:varchar(100)"`
	Plan                 SubscriptionPlan   `gorm:"column:plan;type:varchar(30);not null;default:trial"`
	Status               SubscriptionStatus `gorm:"column:status;type:varchar(30);not null;default:signup"`
	CurrentPeriodStart   *time.Time         `gorm:"column:current_period_start"`
	CurrentPeriodEnd     *time.Time         `gorm:"column:current_period_end"`
	CancelAtPeriodEnd    bool               `gorm:"column:cancel_at_period_end;not null;default:false"`

	// v2.3 — tax ID
	ReverseChargeTaxID *string        `gorm:"column:reverse_charge_tax_id;type:varchar(50)"`
	TaxIDCountry       *string        `gorm:"column:tax_id_country;type:char(2)"`
	TaxIDValidated     bool           `gorm:"column:tax_id_validated;not null;default:false"`
	TaxIDValidatedAt   *time.Time     `gorm:"column:tax_id_validated_at"`
	TaxIDNameMatch     TaxIDNameMatch `gorm:"column:tax_id_name_match;type:varchar(20);not null;default:not_checked"`

	// v2.3 — multi-currency
	BillingCurrency *string   `gorm:"column:billing_currency;type:char(3)"`
	PriceTier       PriceTier `gorm:"column:price_tier;type:varchar(20);not null;default:developed"`

	// v2.3 — add-on + flags
	HasWhiteLabelAppAddOn bool                 `gorm:"column:has_white_label_app_add_on;not null;default:false"`
	ArbitrageFlag         bool                 `gorm:"column:arbitrage_flag;not null;default:false"`
	AppLifecycleStatus    *WhiteLabelAppStatus `gorm:"column:app_lifecycle_status;type:varchar(30)"`

	// v2.3 P4 — billing period + pending-downgrade orchestration (§4.4, §4.5, §4.5.1).
	SubscriptionPeriod          SubscriptionPeriod  `gorm:"column:subscription_period;type:varchar(10);not null;default:monthly"`
	PendingDowngradePlan        *SubscriptionPlan   `gorm:"column:pending_downgrade_plan;type:varchar(30)"`
	PendingDowngradePeriod      *SubscriptionPeriod `gorm:"column:pending_downgrade_period;type:varchar(10)"`
	PendingDowngradeEffectiveAt *time.Time          `gorm:"column:pending_downgrade_effective_at"`
	LastPlanChangeAt            *time.Time          `gorm:"column:last_plan_change_at"`
	LastPlanChangeReason        *string             `gorm:"column:last_plan_change_reason;type:varchar(64)"`

	// P5 Task 14 — day-30 activation idempotency marker (migration 055).
	TrialActivationMarkedAt *time.Time `gorm:"column:trial_activation_marked_at"`

	// Migration 087 — drives trial reminder cron cadence selection.
	// Authoritative writer is the customer.updated webhook handler, which
	// reads invoice_settings.default_payment_method from the event payload.
	HasDefaultPaymentMethod bool `gorm:"column:has_default_payment_method;not null;default:false"`

	// v2.3 P6 — SCA fallback + dunning refund-window guard (§4.7, §16.5).
	// HostedInvoiceURL is populated by invoice.payment_action_required and
	// cleared on invoice.paid. FirstChargeAt is stamped once on the first
	// successful invoice.paid and never updated thereafter — it anchors the
	// 14-day refund window that blocks ladder-driven expiry.
	HostedInvoiceURL *string    `gorm:"column:hosted_invoice_url"`
	FirstChargeAt    *time.Time `gorm:"column:first_charge_at"`

	// P5 — migration fast-path window shortening (migration 053).
	// NULL = standard 14d window; non-NULL = 48h shortened window after CSM-approved fast-path.
	TaxIDWindowShortenedAt *time.Time `gorm:"column:tax_id_window_shortened_at"`

	// P7 — storefront publish gate + quarterly revalidation (migration 067, §5.3, §19.5).
	StorefrontPublished       bool       `gorm:"column:storefront_published;not null;default:false"`
	StorefrontUnpublishedAt   *time.Time `gorm:"column:storefront_unpublished_at"`
	StorefrontUnpublishReason *string    `gorm:"column:storefront_unpublish_reason;type:varchar(40)"`
	RevalidationAttemptedAt   *time.Time `gorm:"column:revalidation_attempted_at"`
	TaxRevalidationStartedAt  *time.Time `gorm:"column:tax_revalidation_started_at"`

	// TrialEndsAt is the operator-extended trial end (migration 103). NULL —
	// the common case — means the trial has never been extended and its end
	// is created_at + trial.TrialDays. Never read this field directly to
	// answer "when does this trial end": call trial.EndsAt, which is the only
	// definition of that.
	TrialEndsAt *time.Time `gorm:"column:trial_ends_at"`

	CreatedAt time.Time `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;default:now()"`
}

// TableName returns the database table name for GORM.
func (StoreSubscription) TableName() string { return "store_subscriptions" }
