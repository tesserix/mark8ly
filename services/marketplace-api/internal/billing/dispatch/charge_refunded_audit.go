package dispatch

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/audit"
)

// ActionRefundedExternally records a refund that reached Stripe WITHOUT going
// through our admin refund API — in practice, one issued from the Stripe
// Dashboard.
//
// It is deliberately NOT subscription.refund_issued. That action means "a
// refund passed our controls": the 14-day cooling-off gate, the card
// fingerprint fraud guard, and the Pro+App guard at
// internal/refund/service.go:80 (ErrProAppNotRefundable), which refuses to
// refund a subscription still holding the white-label app add-on. None of
// those guards exist on the Dashboard path — a Dashboard refund bypasses all
// three, including the Pro+App refusal, and this event is the only thing that
// would ever reveal that it happened. Collapsing the two actions into one
// would destroy exactly that signal.
const ActionRefundedExternally = "subscription.refunded_externally"

// chargeRefundedPayload is the slice of a charge.refunded event we audit.
type chargeRefundedPayload struct {
	ChargeID       string
	Customer       string
	AmountRefunded int64
	Currency       string
	// RefundIDs are the Stripe refund ids from the charge's expanded refunds
	// list. May be empty: Stripe omits the expanded list on some replays.
	RefundIDs []string
}

// externalRefundContext carries the identifiers an emitted event needs. They
// come from the store_subscriptions row matched on stripe_customer_id, so
// they are only meaningful when that row was found.
type externalRefundContext struct {
	SubscriptionID uuid.UUID
	TenantID       uuid.UUID
	StoreID        uuid.UUID
}

// parseChargeRefunded extracts the audited fields from a charge.refunded
// event body. A missing refunds list is not an error.
func parseChargeRefunded(raw []byte) (chargeRefundedPayload, error) {
	var e struct {
		Data struct {
			Object struct {
				ID             string `json:"id"`
				Customer       string `json:"customer"`
				AmountRefunded int64  `json:"amount_refunded"`
				Currency       string `json:"currency"`
				Refunds        struct {
					Data []struct {
						ID string `json:"id"`
					} `json:"data"`
				} `json:"refunds"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return chargeRefundedPayload{}, fmt.Errorf("dispatch: unmarshal charge.refunded: %w", err)
	}
	obj := e.Data.Object
	p := chargeRefundedPayload{
		ChargeID:       obj.ID,
		Customer:       obj.Customer,
		AmountRefunded: obj.AmountRefunded,
		Currency:       obj.Currency,
	}
	for _, r := range obj.Refunds.Data {
		if r.ID != "" {
			p.RefundIDs = append(p.RefundIDs, r.ID)
		}
	}
	return p, nil
}

// decideExternalRefundEvent returns the audit event a charge.refunded webhook
// should produce, or ok=false when it should produce none. It is pure so the
// whole decision is unit-testable without a database — the handler around it
// is a thin shell.
//
// ours is true when at least one of the payload's refund ids is already
// recorded in refund_audit, which means refund.Service issued it and
// EmitRefundIssued already audited it. Our own refunds fire this same webhook
// back at us, so without that discriminator every admin refund would be
// audited twice.
func decideExternalRefundEvent(p chargeRefundedPayload, ours bool, rc externalRefundContext) (audit.Event, bool) {
	if ours {
		return audit.Event{}, false
	}

	// Copy the ids so the event does not alias the parsed payload's slice.
	refundIDs := make([]string, len(p.RefundIDs))
	copy(refundIDs, p.RefundIDs)

	return audit.Event{
		Action:       ActionRefundedExternally,
		ResourceType: "subscription",
		ResourceID:   rc.SubscriptionID.String(),
		// Warning, not Info: a refund that skipped our guards is something
		// ops should be able to find, not routine bookkeeping.
		Severity:       audit.SeverityWarning,
		TenantID:       rc.TenantID,
		StoreID:        rc.StoreID,
		ForceActorType: audit.ActorSystem,
		Metadata: map[string]any{
			"reason":                "charge.refunded",
			"stripe_charge_id":      p.ChargeID,
			"stripe_customer_id":    p.Customer,
			"stripe_refund_ids":     refundIDs,
			"amount_refunded_minor": p.AmountRefunded,
			"currency":              p.Currency,
			"store_id":              rc.StoreID.String(),
		},
	}, true
}

// emitExternalRefund emits the decided event, if any.
//
// d.emitter may be nil — Emitter.Emit is nil-receiver-safe, and nothing here
// dereferences it, so the whole path is a no-op in that wiring.
func (d *Dispatcher) emitExternalRefund(p chargeRefundedPayload, ours bool, rc externalRefundContext) {
	if ev, ok := decideExternalRefundEvent(p, ours, rc); ok {
		d.emitter.Emit(nil, ev)
	}
}
