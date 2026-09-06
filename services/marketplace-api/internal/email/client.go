// Package email is a minimal facade for transactional email sending. Adapters
// (SendGrid, notification-service) implement Client; dunning + trial crons
// depend on the Client interface so real email wiring can be swapped in
// without touching caller code.
package email

import "context"

// TemplateID is a stable identifier for a transactional template.
type TemplateID string

const (
	TemplateDunningDay5           TemplateID = "dunning_day_5"
	TemplateDunningDay7           TemplateID = "dunning_day_7"
	TemplatePaymentActionReminder TemplateID = "payment_action_reminder"

	// Trial-end reminder cadence (migration 088). The "no_pm" templates
	// nudge merchants without a default payment method five times during
	// the final 15 days of the 90-day trial. The "has_pm" template is a
	// single heads-up the day before Stripe auto-bills the chosen plan.
	TemplateTrialNoPMT15 TemplateID = "trial_no_pm_t15"
	TemplateTrialNoPMT10 TemplateID = "trial_no_pm_t10"
	TemplateTrialNoPMT7  TemplateID = "trial_no_pm_t7"
	TemplateTrialNoPMT3  TemplateID = "trial_no_pm_t3"
	TemplateTrialNoPMT1  TemplateID = "trial_no_pm_t1"
	TemplateTrialHasPMT1 TemplateID = "trial_has_pm_t1"
	// Sent from invoice.paid webhook on the first successful charge —
	// confirms the chosen plan is now active.
	TemplateTrialStartedBilled TemplateID = "trial_started_billed"

	// Sent by the trial expiry cron once a cardless trial has actually
	// ended and the store has moved to "expired". The T-1 reminder warned
	// the merchant it would happen; this confirms that it did and says how
	// to reactivate.
	TemplateTrialExpired TemplateID = "trial_expired"

	// Migration fast-path decisions (#703). A merchant submits evidence of
	// trading on a prior platform and a CSM approves or rejects it; these
	// two close that loop. The CSM's review notes are NOT rendered into
	// either template — they are written for internal review, not for the
	// merchant. See migration.Handler.notifyDecision.
	TemplateMigrationFastPathApproved TemplateID = "migration_fast_path_approved"
	TemplateMigrationFastPathRejected TemplateID = "migration_fast_path_rejected"

	// Day-30 post-expiry win-back. Lived in the lifecycle package until
	// #381; moved here so the billing catalog is complete in one place and
	// the email package can register its fallback without importing
	// lifecycle (which imports email).
	//
	// TWO keys, one campaign. TemplateWinBack states a discount and is sent
	// only when the cron has confirmed a redeemable promo code; the copy
	// interpolates that code and its terms. TemplateWinBackNoOffer states no
	// discount at all and is what goes out when there is none to state.
	//
	// The choice is made in Go (lifecycle.WinBackTemplate) rather than by a
	// {{if}} inside one template, because these keys are overridable from
	// the operator console: a single template would put the guard on the
	// discount claim into a text box an operator can edit, and deleting it
	// would restore #727's unconditional promise with no code change and no
	// review. Two keys make the offer-less copy a separate row, which cannot
	// grow a discount claim by accident.
	TemplateWinBack        TemplateID = "win_back_day30"
	TemplateWinBackNoOffer TemplateID = "win_back_day30_no_offer"
)

// Client is the narrow interface every caller uses.
type Client interface {
	Send(ctx context.Context, template TemplateID, to string, data map[string]any) error
}
