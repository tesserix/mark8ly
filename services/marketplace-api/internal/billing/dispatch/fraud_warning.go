package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/metrics"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// ActionFraudWarningReceived records that Stripe Radar told us a charge is
// probably fraudulent (an "early fraud warning" — the issuer has reported the
// card as fraudulently used, usually days before any chargeback lands).
//
// It is deliberately NOT subscription.refunded_externally or any existing
// action: nothing has happened to the subscription. This is Stripe's opinion
// about a charge, and it is the earliest warning we ever get that a store may
// be a fraud liability. Severity is Warning for the same reason a Dashboard
// refund is — ops must be able to find it — and never Critical, because the
// warning is a signal to investigate, not a confirmed loss.
const ActionFraudWarningReceived = "subscription.fraud_warning_received"

// Attribution outcomes for fraud_warnings_total's reason label and the Error
// log that accompanies a failure. The vocabulary is closed and small so
// dashboards can reference fixed values.
const (
	fraudAttrOK              = "ok"
	fraudAttrMalformed       = "malformed_payload"
	fraudAttrNoChargeID      = "no_charge_id"
	fraudAttrNoChargeGetter  = "no_charge_getter"
	fraudAttrLookupFailed    = "charge_lookup_failed"
	fraudAttrNoCustomer      = "no_customer"
	fraudAttrNoSubscription  = "no_subscription"
	fraudAttrSubLookupFailed = "subscription_lookup_failed"
)

// fraudWarningPayload is the slice of a radar.early_fraud_warning event we
// record. Stripe sends no customer and no metadata on this object, which is
// why attribution needs a separate charge lookup.
type fraudWarningPayload struct {
	WarningID       string
	ChargeID        string
	PaymentIntentID string
	// FraudType is Stripe's closed vocabulary: card_never_received,
	// fraudulent_card_application, made_with_counterfeit_card,
	// made_with_lost_card, made_with_stolen_card, misc,
	// unauthorized_use_of_card.
	FraudType string
	// Actionable is Stripe's own judgement that refunding the charge would
	// still avoid a dispute. It drives what ops should do first, so it is
	// both a metric label and audit metadata.
	Actionable bool
}

// fraudWarningContext carries the identifiers an emitted event needs. They
// come from the store_subscriptions row matched on the charge's customer, so
// they are only meaningful when attribution succeeded.
type fraudWarningContext struct {
	SubscriptionID uuid.UUID
	TenantID       uuid.UUID
	StoreID        uuid.UUID
}

// stripeRef decodes a Stripe reference field that is a bare id string when
// unexpanded and a full object when expanded, and null when absent. Getting
// this wrong on `charge` would cost us the only attribution key the event has.
type stripeRef string

func (r *stripeRef) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*r = ""
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*r = stripeRef(s)
		return nil
	}
	var obj struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	*r = stripeRef(obj.ID)
	return nil
}

// parseFraudWarning extracts the recorded fields from a
// radar.early_fraud_warning event body.
func parseFraudWarning(raw []byte) (fraudWarningPayload, error) {
	var e struct {
		Data struct {
			Object struct {
				ID            string    `json:"id"`
				Charge        stripeRef `json:"charge"`
				PaymentIntent stripeRef `json:"payment_intent"`
				FraudType     string    `json:"fraud_type"`
				Actionable    bool      `json:"actionable"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return fraudWarningPayload{}, fmt.Errorf("dispatch: unmarshal radar.early_fraud_warning: %w", err)
	}
	obj := e.Data.Object
	return fraudWarningPayload{
		WarningID:       obj.ID,
		ChargeID:        string(obj.Charge),
		PaymentIntentID: string(obj.PaymentIntent),
		FraudType:       obj.FraudType,
		Actionable:      obj.Actionable,
	}, nil
}

// decideFraudWarningEvent returns the audit event a radar.early_fraud_warning
// webhook should produce, or ok=false when it should produce none. It is pure
// so the whole decision is unit-testable without a database or a Stripe
// client — the handler around it is a thin shell.
//
// attributed is false when the warning could not be tied to a store. No event
// is emitted then, because audit.Event is tenant-scoped and a tenant-less row
// would be unreadable in every audit surface we have. The Error log and the
// counter carry that case instead — it is not dropped, just not audited.
func decideFraudWarningEvent(p fraudWarningPayload, fc fraudWarningContext, attributed bool) (audit.Event, bool) {
	if !attributed {
		return audit.Event{}, false
	}
	return audit.Event{
		Action:         ActionFraudWarningReceived,
		ResourceType:   "subscription",
		ResourceID:     fc.SubscriptionID.String(),
		Severity:       audit.SeverityWarning,
		TenantID:       fc.TenantID,
		StoreID:        fc.StoreID,
		ForceActorType: audit.ActorSystem,
		Metadata: map[string]any{
			"reason":                        "radar.early_fraud_warning",
			"stripe_early_fraud_warning_id": p.WarningID,
			"stripe_charge_id":              p.ChargeID,
			"stripe_payment_intent_id":      p.PaymentIntentID,
			"fraud_type":                    p.FraudType,
			"actionable":                    p.Actionable,
			"store_id":                      fc.StoreID.String(),
		},
	}, true
}

// fraudWarningLabels maps an attribution outcome onto the three
// fraud_warnings_total label values. Pure, so the label vocabulary is
// testable without a registry.
func fraudWarningLabels(p fraudWarningPayload, attributed bool, reason string) (attribution, reasonLabel, actionable string) {
	attribution = "unattributed"
	if attributed {
		attribution = "attributed"
	}
	return attribution, reason, strconv.FormatBool(p.Actionable)
}

// attributeFraudWarning resolves the store behind a fraud warning.
//
// The event carries a charge id and no customer, and there is no local
// charge→store mapping (refund_audit.stripe_charge_id covers refunds only),
// so this costs one Stripe read. Every failure mode returns ok=false with a
// reason rather than an error: the caller must never fail the webhook over
// attribution.
func (d *Dispatcher) attributeFraudWarning(ctx context.Context, tx *gorm.DB, p fraudWarningPayload) (fraudWarningContext, bool, string) {
	if p.ChargeID == "" {
		return fraudWarningContext{}, false, fraudAttrNoChargeID
	}
	if d.charges == nil {
		// No Stripe billing client wired (local dev, or a deployment with no
		// billing key). Nothing to look up.
		return fraudWarningContext{}, false, fraudAttrNoChargeGetter
	}

	ch, err := d.charges.GetCharge(ctx, p.ChargeID)
	if err != nil {
		return fraudWarningContext{}, false, fraudAttrLookupFailed
	}
	if ch == nil || ch.CustomerID == "" {
		// A one-off charge with no customer attached: nothing to join on.
		return fraudWarningContext{}, false, fraudAttrNoCustomer
	}

	// Resolve inside the webhook transaction, exactly as handleChargeRefunded
	// does, so the read sees the same snapshot as the rest of the event.
	var sub subscription.StoreSubscription
	switch err := tx.WithContext(ctx).Where("stripe_customer_id = ?", ch.CustomerID).First(&sub).Error; {
	case err == nil:
	case errors.Is(err, gorm.ErrRecordNotFound):
		return fraudWarningContext{}, false, fraudAttrNoSubscription
	default:
		return fraudWarningContext{}, false, fraudAttrSubLookupFailed
	}

	return fraudWarningContext{
		SubscriptionID: sub.ID,
		TenantID:       sub.TenantID,
		StoreID:        sub.StoreID,
	}, true, fraudAttrOK
}

// handleFraudWarning records a Stripe Radar early fraud warning (#704). It
// used to be `return nil` with a comment claiming it was "audit-only in P2" —
// no audit row was written either, so the strongest fraud signal Stripe gives
// us reached nothing at all.
//
// It writes no columns. In particular it does NOT set arbitrage_flag: that is
// the other half of #704 and is deliberately deferred until the operator
// appeals queue (tesserix-home#144) exists, because arbitrage_flag is visible
// to the merchant on their own subscription response and flagging someone who
// can see it with no review path is worse than not flagging.
//
// It is a method (not the free function it used to be) purely so it can reach
// d.emitter and d.charges, exactly as handleChargeRefunded became one.
//
// It never returns an error. A fraud warning is observational: failing the
// webhook would make Stripe retry an event that cannot succeed any better on
// the second attempt, and would push the store's whole webhook stream into
// retry behind it.
func (d *Dispatcher) handleFraudWarning(ctx context.Context, tx *gorm.DB, raw []byte) error {
	p, err := parseFraudWarning(raw)
	if err != nil {
		// Still counted: a warning we could not even parse is a warning we
		// received, and silence here would look identical to no fraud.
		slog.Default().Error("dispatch: radar.early_fraud_warning payload unparseable; warning recorded only as a metric",
			"error", err)
		countFraudWarning(fraudWarningPayload{}, false, fraudAttrMalformed)
		return nil
	}

	fc, attributed, reason := d.attributeFraudWarning(ctx, tx, p)
	if !attributed {
		// Never silent. An unattributable fraud warning is still a fraud
		// warning, and the ids below are enough to finish the attribution by
		// hand in the Stripe Dashboard.
		slog.Default().Error("dispatch: could not attribute Stripe early fraud warning to a store; no audit event emitted",
			"stripe_early_fraud_warning_id", p.WarningID,
			"stripe_charge_id", p.ChargeID,
			"fraud_type", p.FraudType,
			"actionable", p.Actionable,
			"reason", reason)
	}

	if ev, ok := decideFraudWarningEvent(p, fc, attributed); ok {
		// d.emitter may be nil — Emitter.Emit is nil-receiver-safe.
		d.emitter.Emit(nil, ev)
	}
	countFraudWarning(p, attributed, reason)
	return nil
}

// countFraudWarning increments the fraud warning counter. Split out so the
// handler reads as one decision per line and so the label mapping stays in
// fraudWarningLabels.
func countFraudWarning(p fraudWarningPayload, attributed bool, reason string) {
	attribution, reasonLabel, actionable := fraudWarningLabels(p, attributed, reason)
	metrics.FraudWarningsTotal.WithLabelValues(attribution, reasonLabel, actionable).Inc()
}
