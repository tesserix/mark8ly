package trial

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/statemachine"
)

// ExpirySpec is the cron expression for the expiry cron: 00:15 UTC daily.
// Late enough that any last-second card-add webhook from the prior day has
// settled; early enough that merchants see expiry at start-of-day.
const ExpirySpec = "15 0 * * *"

// ExpiryCron transitions trialing stores that have no card (stripe_subscription_id
// IS NULL) and whose trial has passed its effective end (EndsAt — normally
// TrialDays, extended if an operator has set trial_ends_at) to the "expired"
// status via the state machine. It is idempotent: stores already in "expired"
// are never selected.
type ExpiryCron struct {
	db      *gorm.DB
	emitter *audit.Emitter
	logger  *slog.Logger
	clock   func() time.Time
	mailer  email.Client
	sent    SentCounter
	skip    SkipCounter
}

// CounterIncrementer is a one-method counter so tests can stub it.
type CounterIncrementer interface{ Inc() }

// SentCounter counts expiry notices actually delivered, labeled by template.
// Declared here rather than imported from the dunning package, which already
// imports this one.
type SentCounter interface {
	WithTemplate(template string) CounterIncrementer
}

// SkipCounter counts expiry notices deliberately not sent, labeled by
// template and reason (see email.SkipReason for the reason vocabulary).
type SkipCounter interface {
	WithTemplateReason(template, reason string) CounterIncrementer
}

// NewExpiryCron constructs an ExpiryCron. If clock is nil, time.Now().UTC() is
// used. If logger is nil, slog.Default() is used.
func NewExpiryCron(db *gorm.DB, em *audit.Emitter, logger *slog.Logger, clock func() time.Time) *ExpiryCron {
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ExpiryCron{db: db, emitter: em, logger: logger, clock: clock}
}

// WithEmail attaches the transactional email client used for the
// trial-expired notice, plus the optional delivered/skipped counters.
//
// Chainable rather than a constructor parameter so the existing callers —
// main.go and five integration tests — compile untouched. A cron with no
// client simply sends nothing: trial expiry must never fail because email
// is unconfigured.
func (c *ExpiryCron) WithEmail(cl email.Client, sent SentCounter, skip SkipCounter) *ExpiryCron {
	c.mailer = cl
	c.sent = sent
	c.skip = skip
	return c
}

// Run selects all trialing stores without a card whose effective trial end
// has passed and transitions each one to "expired" via statemachine.Transition.
// Row-level errors are logged and skipped so one bad row never blocks the rest.
func (c *ExpiryCron) Run(ctx context.Context) error {
	// Effective trial end, not created_at + TrialDays: a trial an operator
	// extended must survive its original day 90. EndedBeforeScope carries
	// both branches — see endsat.go.
	now := c.clock().UTC()
	var rows []subscription.StoreSubscription
	err := EndedBeforeScope(
		c.db.WithContext(ctx).
			Where("status = ?", subscription.StatusTrialing).
			Where("stripe_subscription_id IS NULL"),
		now,
	).Find(&rows).Error
	if err != nil {
		return err
	}
	for i := range rows {
		c.expireOne(ctx, &rows[i])
	}
	return nil
}

func (c *ExpiryCron) expireOne(ctx context.Context, row *subscription.StoreSubscription) {
	c.afterTransition(ctx, row, statemachine.Transition(ctx, statemachine.TransitionInput{
		DB:       c.db,
		Emitter:  c.emitter,
		TenantID: row.TenantID,
		StoreID:  row.StoreID,
		From:     subscription.StatusTrialing,
		To:       subscription.StatusExpired,
		Actor:    "system:cron:trial_expiry",
		Reason:   "trial_ended_no_card",
	}))
}

// afterTransition decides what follows the state-machine call. Split out from
// expireOne so the email decision is testable without a database.
func (c *ExpiryCron) afterTransition(ctx context.Context, row *subscription.StoreSubscription, err error) {
	switch {
	case err == nil:
		c.logger.Info("trial expired",
			"store_id", row.StoreID.String(),
			"tenant_id", row.TenantID.String())
		c.notifyExpired(ctx, row)
	case errors.Is(err, statemachine.ErrCASConflict):
		// Another writer moved it — likely a last-second webhook from card-add.
		// That store did NOT expire, so it gets no notice, and no error log.
	default:
		c.logger.Error("trial expiry: transition failed",
			"store_id", row.StoreID.String(), "err", err)
	}
}

// notifyExpired tells the merchant their trial has ended. It is best-effort:
// the transition has already been committed, so a send failure is logged and
// counted, never returned.
func (c *ExpiryCron) notifyExpired(ctx context.Context, row *subscription.StoreSubscription) {
	if c.mailer == nil {
		// Email is not configured for this deployment (and is not wired in
		// the integration tests). Expiry itself has already succeeded.
		return
	}

	to := ""
	if row.Email != nil {
		to = *row.Email
	}
	// Classify before handing the address to the provider: the placeholder
	// billing+<uuid>@mark8ly.local addresses minted at bootstrap would
	// hard-bounce and cost sender reputation.
	if err := email.ValidateRecipient(to); err != nil {
		c.countSkip(email.SkipReason(err))
		c.logger.Warn("trial expiry notice not sent",
			"store_id", row.StoreID.String(),
			"tenant_id", row.TenantID.String(),
			"reason", email.SkipReason(err))
		return
	}

	if err := c.mailer.Send(ctx, email.TemplateTrialExpired, to, map[string]any{
		"store_id":   row.StoreID.String(),
		"tenant_id":  row.TenantID.String(),
		"store_name": c.storeName(ctx, row),
	}); err != nil {
		c.countSkip(email.SkipReason(err))
		c.logger.Warn("trial expiry notice not sent",
			"store_id", row.StoreID.String(),
			"tenant_id", row.TenantID.String(),
			"reason", email.SkipReason(err),
			"err", err.Error())
		return
	}

	if c.sent != nil {
		c.sent.WithTemplate(string(email.TemplateTrialExpired)).Inc()
	}
	c.logger.Info("trial expiry notice sent",
		"store_id", row.StoreID.String(),
		"tenant_id", row.TenantID.String())
}

func (c *ExpiryCron) countSkip(reason string) {
	if c.skip != nil {
		c.skip.WithTemplateReason(string(email.TemplateTrialExpired), reason).Inc()
	}
}

// storeName resolves the merchant-facing store name, tolerating a cron built
// without a database handle (unit tests) by falling back to the same generic
// wording subscription.StoreNameFor uses when the lookup misses.
func (c *ExpiryCron) storeName(ctx context.Context, row *subscription.StoreSubscription) string {
	if c.db == nil {
		return "your store"
	}
	return subscription.StoreNameFor(ctx, c.db, row.StoreID)
}
