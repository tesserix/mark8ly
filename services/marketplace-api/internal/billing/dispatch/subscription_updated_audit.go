package dispatch

import (
	"time"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/audit"
)

// Audit actions emitted by customer.subscription.updated, in the same
// subscription.* namespace as lifecycle.ActionProAppCancelled.
//
// Scheduling a cancellation and reversing one are DELIBERATELY separate
// actions. They are opposite merchant intents, and #701's save offer works by
// reversing a scheduled cancellation — an audit trail that collapsed the two
// into one action could not answer "did this merchant cancel and then stay?",
// which is the question a billing dispute actually asks.
const (
	// ActionCancellationScheduled: cancel_at_period_end went false -> true.
	ActionCancellationScheduled = "subscription.cancellation_scheduled"
	// ActionCancellationReversed: cancel_at_period_end went true -> false.
	ActionCancellationReversed = "subscription.cancellation_reversed"
	// ActionPeriodRolled: current_period_start advanced to a later value.
	ActionPeriodRolled = "subscription.period_rolled"
)

// subscriptionPeriodState is the slice of a subscription row that
// customer.subscription.updated can change and that we audit. It is used for
// both sides of the comparison: the row as it stood before the UPDATE, and
// the values the Stripe payload carries.
type subscriptionPeriodState struct {
	PeriodStart       *time.Time
	CancelAtPeriodEnd bool
}

// periodTransitionContext carries the identifiers an emitted event needs.
// These come from the pre-UPDATE row, so they are only meaningful when that
// row was found.
type periodTransitionContext struct {
	Customer       string
	SubscriptionID uuid.UUID
	TenantID       uuid.UUID
	StoreID        uuid.UUID
}

// decidePeriodTransitions returns the audit actions a customer.subscription.updated
// webhook should produce, given the row before the UPDATE and the values the
// webhook carries. It is pure so the whole decision is unit-testable without
// a database — the handler around it is a thin shell.
//
// It returns nil when nothing we audit actually changed. That is the common
// case: Stripe emits customer.subscription.updated for many reasons, so
// emitting unconditionally would produce one event per webhook rather than
// one per transition, which would make the audit trail noisier and less
// useful, not more.
func decidePeriodTransitions(before, after subscriptionPeriodState) []string {
	var actions []string

	if before.CancelAtPeriodEnd != after.CancelAtPeriodEnd {
		if after.CancelAtPeriodEnd {
			actions = append(actions, ActionCancellationScheduled)
		} else {
			actions = append(actions, ActionCancellationReversed)
		}
	}

	// A period rolls only when the payload carries a start that is strictly
	// later than the one we held. An absent start (Stripe omits the field on
	// some replays) and a start that moves backwards are both treated as "no
	// roll" rather than as transitions.
	//
	// nil -> a value counts as a roll: it is the first billing period this
	// subscription has ever had a start for, which happens once in its life
	// and is worth a row.
	if after.PeriodStart != nil &&
		(before.PeriodStart == nil || after.PeriodStart.After(*before.PeriodStart)) {
		actions = append(actions, ActionPeriodRolled)
	}

	return actions
}

// emitPeriodTransitions emits one audit event per decided transition.
//
// d.emitter may be nil — Emitter.Emit is nil-receiver-safe, and nothing here
// dereferences it, so the whole path is a no-op in that wiring.
func (d *Dispatcher) emitPeriodTransitions(pc periodTransitionContext, before, after subscriptionPeriodState) {
	for _, action := range decidePeriodTransitions(before, after) {
		severity := audit.SeverityInfo
		if action == ActionCancellationScheduled {
			// A merchant on their way out is worth surfacing above the
			// routine period roll.
			severity = audit.SeverityWarning
		}
		d.emitter.Emit(nil, audit.Event{
			Action:         action,
			ResourceType:   "subscription",
			ResourceID:     pc.SubscriptionID.String(),
			Severity:       severity,
			TenantID:       pc.TenantID,
			StoreID:        pc.StoreID,
			ForceActorType: audit.ActorSystem,
			Metadata: map[string]any{
				"reason":                      "customer.subscription.updated",
				"stripe_customer_id":          pc.Customer,
				"store_id":                    pc.StoreID.String(),
				"cancel_at_period_end_before": before.CancelAtPeriodEnd,
				"cancel_at_period_end_after":  after.CancelAtPeriodEnd,
				"current_period_start_before": formatPeriod(before.PeriodStart),
				"current_period_start_after":  formatPeriod(after.PeriodStart),
			},
		})
	}
}

// formatPeriod renders a nullable period boundary for audit metadata. A nil
// boundary becomes "" rather than a typed nil, so the JSON blob stays flat.
func formatPeriod(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
