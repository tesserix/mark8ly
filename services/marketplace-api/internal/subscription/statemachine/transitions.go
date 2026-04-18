// Package statemachine encodes the v2.3 subscription state machine (§17.2).
// Every transition is explicit; no fallthrough, no branch-on-status sprinkled
// across the codebase. Callers wire webhook + cron + merchant actions through
// one Transition entry point.
package statemachine

import (
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// transitionMeta documents what kind of transition this is.
type transitionMeta struct {
	Severity  AuditSeverity
	ActorKind ActorKind
	SpecRef   string
}

// AuditSeverity classifies the operational weight of a transition for audit
// logging and alerting purposes.
type AuditSeverity string

const (
	SeverityInfo    AuditSeverity = "info"
	SeverityWarning AuditSeverity = "warning"
	SeverityError   AuditSeverity = "error"
)

// ActorKind identifies who or what initiates a transition.
type ActorKind string

const (
	ActorSystem ActorKind = "system"
	ActorUser   ActorKind = "user"
	ActorAny    ActorKind = "any"
)

// transitionTable encodes §17.2. Every allowed move lives here; nothing else
// is a valid transition.
var transitionTable = map[subscription.SubscriptionStatus]map[subscription.SubscriptionStatus]transitionMeta{
	subscription.StatusSignup: {
		subscription.StatusTrialing: {SeverityInfo, ActorSystem, "§17.2 signup → trialing (email verified)"},
	},
	subscription.StatusTrialing: {
		subscription.StatusActive:  {SeverityInfo, ActorSystem, "§17.2 trialing → active (card added; first charge day 90)"},
		subscription.StatusExpired: {SeverityWarning, ActorSystem, "§17.2 trialing → expired (day 90, no card)"},
	},
	subscription.StatusActive: {
		subscription.StatusPastDue:               {SeverityWarning, ActorSystem, "§17.2 active → past_due (invoice.payment_failed)"},
		subscription.StatusPaymentActionRequired: {SeverityWarning, ActorSystem, "§17.2 active → payment_action_required (invoice.payment_action_required)"},
		subscription.StatusCancelScheduled:       {SeverityInfo, ActorUser, "§17.2 active → cancel_scheduled (merchant cancel)"},
	},
	subscription.StatusPastDue: {
		subscription.StatusActive:  {SeverityInfo, ActorSystem, "§17.2 past_due → active (retry succeeds)"},
		subscription.StatusExpired: {SeverityError, ActorSystem, "§17.2 past_due → expired (dunning final fail)"},
	},
	subscription.StatusPaymentActionRequired: {
		subscription.StatusActive:  {SeverityInfo, ActorSystem, "§17.2 payment_action_required → active (invoice.paid)"},
		subscription.StatusPastDue: {SeverityWarning, ActorSystem, "§17.2 payment_action_required → past_due (invoice unpaid past reminder)"},
	},
	subscription.StatusCancelScheduled: {
		subscription.StatusActive:  {SeverityInfo, ActorUser, "§17.2 cancel_scheduled → active (save-offer reversal or card re-added)"},
		subscription.StatusExpired: {SeverityWarning, ActorSystem, "§17.2 cancel_scheduled → expired (current_period_end)"},
	},
	subscription.StatusExpired: {
		subscription.StatusActive:      {SeverityInfo, ActorUser, "§17.2 expired → active (card re-added during grace)"},
		subscription.StatusStoreClosed: {SeverityWarning, ActorSystem, "§17.2 expired → store_closed (day 14 post-expiry)"},
	},
	subscription.StatusStoreClosed: {
		subscription.StatusActive:            {SeverityInfo, ActorUser, "§17.2 store_closed → active (card re-added during grace)"},
		subscription.StatusPendingHardDelete: {SeverityError, ActorSystem, "§17.2 store_closed → pending_hard_delete (day 90 post-expiry)"},
	},
	subscription.StatusPendingHardDelete: {
		subscription.StatusHardDeleted: {SeverityError, ActorSystem, "§17.2 pending_hard_delete → hard_deleted (deletion job)"},
	},
	// StatusHardDeleted is terminal — no outbound transitions.
}

// IsValidTransition reports whether (from → to) is one of the 17 allowed moves.
func IsValidTransition(from, to subscription.SubscriptionStatus) bool {
	toMap, ok := transitionTable[from]
	if !ok {
		return false
	}
	_, ok = toMap[to]
	return ok
}

// TransitionRecord is a concrete (from, to, meta) triple for diagnostic tools
// and documentation generation. The entry-point function that enforces
// transitions at the DB layer lives in machine.go.
type TransitionRecord struct {
	From subscription.SubscriptionStatus
	To   subscription.SubscriptionStatus
	Meta transitionMeta
}

// AllValidTransitions returns every (from, to) pair defined by §17.2.
// Use for tests and documentation; not intended for hot paths.
func AllValidTransitions() []TransitionRecord {
	out := make([]TransitionRecord, 0, 32)
	for from, tos := range transitionTable {
		for to, meta := range tos {
			out = append(out, TransitionRecord{From: from, To: to, Meta: meta})
		}
	}
	return out
}

// ReadOnlyStatuses returns the statuses in which RequireActive rejects
// non-allowlisted admin routes (§17.3).
// NOTE: payment_action_required is deliberately excluded — Council finding #3
// confirms merchants in that state retain full admin access.
func ReadOnlyStatuses() []subscription.SubscriptionStatus {
	return []subscription.SubscriptionStatus{
		subscription.StatusExpired,
		subscription.StatusStoreClosed,
		subscription.StatusPendingHardDelete,
	}
}

// IsReadOnly reports whether a subscription in status s is subject to
// read-only admin restrictions per §17.3.
func IsReadOnly(s subscription.SubscriptionStatus) bool {
	for _, r := range ReadOnlyStatuses() {
		if r == s {
			return true
		}
	}
	return false
}
