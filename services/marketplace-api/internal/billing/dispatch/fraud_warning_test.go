package dispatch

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/mark8ly/marketplace-api/internal/audit"
	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/metrics"
)

func TestParseFraudWarning(t *testing.T) {
	raw := []byte(`{
      "type": "radar.early_fraud_warning.created",
      "data": {
        "object": {
          "id": "issfr_123",
          "object": "radar.early_fraud_warning",
          "charge": "ch_123",
          "payment_intent": "pi_123",
          "fraud_type": "made_with_stolen_card",
          "actionable": true,
          "livemode": true
        }
      }
    }`)

	p, err := parseFraudWarning(raw)
	if err != nil {
		t.Fatalf("parseFraudWarning: %v", err)
	}
	if p.WarningID != "issfr_123" {
		t.Errorf("WarningID = %q, want issfr_123", p.WarningID)
	}
	if p.ChargeID != "ch_123" {
		t.Errorf("ChargeID = %q, want ch_123", p.ChargeID)
	}
	if p.PaymentIntentID != "pi_123" {
		t.Errorf("PaymentIntentID = %q, want pi_123", p.PaymentIntentID)
	}
	if p.FraudType != "made_with_stolen_card" {
		t.Errorf("FraudType = %q, want made_with_stolen_card", p.FraudType)
	}
	if !p.Actionable {
		t.Error("Actionable = false, want true")
	}
}

func TestParseFraudWarning_ExpandedRefs(t *testing.T) {
	// charge and payment_intent are string ids when unexpanded, but Stripe
	// sends full objects when the endpoint or webhook is configured to
	// expand them. Both shapes must yield the same id.
	raw := []byte(`{
      "data": {
        "object": {
          "id": "issfr_9",
          "charge": {"id": "ch_9", "object": "charge", "amount": 100},
          "payment_intent": {"id": "pi_9", "object": "payment_intent"},
          "fraud_type": "misc",
          "actionable": false
        }
      }
    }`)

	p, err := parseFraudWarning(raw)
	if err != nil {
		t.Fatalf("parseFraudWarning: %v", err)
	}
	if p.ChargeID != "ch_9" {
		t.Errorf("ChargeID = %q, want ch_9", p.ChargeID)
	}
	if p.PaymentIntentID != "pi_9" {
		t.Errorf("PaymentIntentID = %q, want pi_9", p.PaymentIntentID)
	}
	if p.Actionable {
		t.Error("Actionable = true, want false")
	}
}

func TestParseFraudWarning_NullRefs(t *testing.T) {
	raw := []byte(`{"data":{"object":{"id":"issfr_1","charge":null,"payment_intent":null,"fraud_type":"misc"}}}`)
	p, err := parseFraudWarning(raw)
	if err != nil {
		t.Fatalf("parseFraudWarning: %v", err)
	}
	if p.ChargeID != "" || p.PaymentIntentID != "" {
		t.Errorf("null refs should decode to empty, got %q / %q", p.ChargeID, p.PaymentIntentID)
	}
}

func TestParseFraudWarning_Malformed(t *testing.T) {
	if _, err := parseFraudWarning([]byte(`{`)); err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}

func newFraudCtx() fraudWarningContext {
	return fraudWarningContext{
		SubscriptionID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		TenantID:       uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		StoreID:        uuid.MustParse("33333333-3333-3333-3333-333333333333"),
	}
}

func sampleFraudPayload() fraudWarningPayload {
	return fraudWarningPayload{
		WarningID:       "issfr_123",
		ChargeID:        "ch_123",
		PaymentIntentID: "pi_123",
		FraudType:       "made_with_stolen_card",
		Actionable:      true,
	}
}

func TestDecideFraudWarningEvent_Emits(t *testing.T) {
	fc := newFraudCtx()
	ev, ok := decideFraudWarningEvent(sampleFraudPayload(), fc, true)
	if !ok {
		t.Fatal("expected an event for an attributed fraud warning")
	}
	if ev.Action != ActionFraudWarningReceived {
		t.Errorf("Action = %q, want %q", ev.Action, ActionFraudWarningReceived)
	}
	if ev.Severity != audit.SeverityWarning {
		t.Errorf("Severity = %q, want %q", ev.Severity, audit.SeverityWarning)
	}
	if ev.ResourceType != "subscription" {
		t.Errorf("ResourceType = %q, want subscription", ev.ResourceType)
	}
	if ev.ResourceID != fc.SubscriptionID.String() {
		t.Errorf("ResourceID = %q, want %q", ev.ResourceID, fc.SubscriptionID)
	}
	if ev.TenantID != fc.TenantID || ev.StoreID != fc.StoreID {
		t.Errorf("tenant/store not carried through: %v %v", ev.TenantID, ev.StoreID)
	}
	if ev.ForceActorType != audit.ActorSystem {
		t.Errorf("ForceActorType = %q, want system", ev.ForceActorType)
	}
	if ev.Metadata["stripe_early_fraud_warning_id"] != "issfr_123" {
		t.Errorf("metadata warning id = %v", ev.Metadata["stripe_early_fraud_warning_id"])
	}
	if ev.Metadata["stripe_charge_id"] != "ch_123" {
		t.Errorf("metadata charge id = %v", ev.Metadata["stripe_charge_id"])
	}
	if ev.Metadata["stripe_payment_intent_id"] != "pi_123" {
		t.Errorf("metadata payment intent id = %v", ev.Metadata["stripe_payment_intent_id"])
	}
	if ev.Metadata["fraud_type"] != "made_with_stolen_card" {
		t.Errorf("metadata fraud_type = %v", ev.Metadata["fraud_type"])
	}
	if ev.Metadata["actionable"] != true {
		t.Errorf("metadata actionable = %v", ev.Metadata["actionable"])
	}
}

func TestDecideFraudWarningEvent_NoEventWhenUnattributed(t *testing.T) {
	// audit.Event requires a TenantID; an unattributable warning has none.
	// It must not produce a tenant-less row — the Error log and the counter
	// carry that signal instead.
	if _, ok := decideFraudWarningEvent(sampleFraudPayload(), fraudWarningContext{}, false); ok {
		t.Fatal("expected no audit event when the warning could not be attributed")
	}
}

func TestFraudWarningLabels(t *testing.T) {
	tests := []struct {
		name           string
		actionable     bool
		attributed     bool
		reason         string
		wantAttr       string
		wantReason     string
		wantActionable string
	}{
		{
			name: "attributed actionable", actionable: true, attributed: true, reason: fraudAttrOK,
			wantAttr: "attributed", wantReason: fraudAttrOK, wantActionable: "true",
		},
		{
			name: "attributed not actionable", actionable: false, attributed: true, reason: fraudAttrOK,
			wantAttr: "attributed", wantReason: fraudAttrOK, wantActionable: "false",
		},
		{
			name: "unattributed", actionable: true, attributed: false, reason: fraudAttrNoChargeGetter,
			wantAttr: "unattributed", wantReason: fraudAttrNoChargeGetter, wantActionable: "true",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := sampleFraudPayload()
			p.Actionable = tc.actionable
			attr, reason, actionable := fraudWarningLabels(p, tc.attributed, tc.reason)
			if attr != tc.wantAttr {
				t.Errorf("attribution = %q, want %q", attr, tc.wantAttr)
			}
			if reason != tc.wantReason {
				t.Errorf("reason = %q, want %q", reason, tc.wantReason)
			}
			if actionable != tc.wantActionable {
				t.Errorf("actionable = %q, want %q", actionable, tc.wantActionable)
			}
		})
	}
}

// stubChargeGetter implements ChargeGetter without a Stripe client.
type stubChargeGetter struct {
	charge *billingstripe.Charge
	err    error
	calls  []string
}

func (s *stubChargeGetter) GetCharge(_ context.Context, id string) (*billingstripe.Charge, error) {
	s.calls = append(s.calls, id)
	return s.charge, s.err
}

func TestAttributeFraudWarning_NilGetterDoesNotPanic(t *testing.T) {
	// Production wiring may omit WithChargeGetter (no Stripe billing key).
	// The warning is then unattributable, but nothing may panic and the
	// reason must say why. tx is nil here precisely because this path must
	// return before touching the database.
	d := New(nil)
	_, ok, reason := d.attributeFraudWarning(context.Background(), nil, sampleFraudPayload())
	if ok {
		t.Fatal("expected attribution to fail with no charge getter")
	}
	if reason != fraudAttrNoChargeGetter {
		t.Errorf("reason = %q, want %q", reason, fraudAttrNoChargeGetter)
	}
}

func TestAttributeFraudWarning_NoChargeID(t *testing.T) {
	getter := &stubChargeGetter{}
	d := New(nil).withChargeGetter(getter)
	p := sampleFraudPayload()
	p.ChargeID = ""
	_, ok, reason := d.attributeFraudWarning(context.Background(), nil, p)
	if ok {
		t.Fatal("expected attribution to fail with no charge id")
	}
	if reason != fraudAttrNoChargeID {
		t.Errorf("reason = %q, want %q", reason, fraudAttrNoChargeID)
	}
	if len(getter.calls) != 0 {
		t.Errorf("must not call Stripe without a charge id, got %v", getter.calls)
	}
}

func TestAttributeFraudWarning_LookupFailure(t *testing.T) {
	// A Stripe outage must not swallow the warning: attribution fails, the
	// caller still logs and counts.
	d := New(nil).withChargeGetter(&stubChargeGetter{err: errors.New("stripe down")})
	_, ok, reason := d.attributeFraudWarning(context.Background(), nil, sampleFraudPayload())
	if ok {
		t.Fatal("expected attribution to fail when the Stripe lookup errors")
	}
	if reason != fraudAttrLookupFailed {
		t.Errorf("reason = %q, want %q", reason, fraudAttrLookupFailed)
	}
}

func TestAttributeFraudWarning_ChargeWithoutCustomer(t *testing.T) {
	// A one-off charge carries no customer, so there is nothing to join
	// store_subscriptions on. Must not reach the database.
	getter := &stubChargeGetter{charge: &billingstripe.Charge{ID: "ch_123"}}
	d := New(nil).withChargeGetter(getter)
	_, ok, reason := d.attributeFraudWarning(context.Background(), nil, sampleFraudPayload())
	if ok {
		t.Fatal("expected attribution to fail for a charge with no customer")
	}
	if reason != fraudAttrNoCustomer {
		t.Errorf("reason = %q, want %q", reason, fraudAttrNoCustomer)
	}
	if len(getter.calls) != 1 || getter.calls[0] != "ch_123" {
		t.Errorf("charge lookup calls = %v, want [ch_123]", getter.calls)
	}
}

func TestAttributeFraudWarning_NilChargeIsNotACustomer(t *testing.T) {
	// A getter returning (nil, nil) must be treated as "no customer", not
	// dereferenced.
	d := New(nil).withChargeGetter(&stubChargeGetter{})
	_, ok, reason := d.attributeFraudWarning(context.Background(), nil, sampleFraudPayload())
	if ok {
		t.Fatal("expected attribution to fail for a nil charge")
	}
	if reason != fraudAttrNoCustomer {
		t.Errorf("reason = %q, want %q", reason, fraudAttrNoCustomer)
	}
}

func TestHandleFraudWarning_UnattributedReturnsNil(t *testing.T) {
	// The webhook is never failed by a fraud warning: it is observational.
	// With no charge getter this runs end to end without touching tx.
	d := New(nil)
	raw := []byte(`{"data":{"object":{"id":"issfr_1","charge":"ch_1","fraud_type":"misc","actionable":true}}}`)
	if err := d.handleFraudWarning(context.Background(), nil, raw); err != nil {
		t.Fatalf("handleFraudWarning = %v, want nil", err)
	}
}

func TestHandleFraudWarning_MalformedReturnsNil(t *testing.T) {
	d := New(nil)
	if err := d.handleFraudWarning(context.Background(), nil, []byte(`{`)); err != nil {
		t.Fatalf("handleFraudWarning = %v, want nil", err)
	}
}

func TestFraudWarningHandlerIsRegistered(t *testing.T) {
	if _, ok := New(nil).handlers["radar.early_fraud_warning"]; !ok {
		t.Fatal("radar.early_fraud_warning must stay registered")
	}
}

func TestHandleFraudWarning_CountsEvenWhenUnattributed(t *testing.T) {
	// The whole point of #704's counter is that it fires on EVERY warning.
	// If it only counted attributed ones it would read healthiest exactly
	// when the Stripe lookup path is broken.
	labels := []string{"unattributed", fraudAttrNoChargeGetter, "true"}
	before := testutil.ToFloat64(metrics.FraudWarningsTotal.WithLabelValues(labels...))

	d := New(nil)
	raw := []byte(`{"data":{"object":{"id":"issfr_1","charge":"ch_1","fraud_type":"misc","actionable":true}}}`)
	if err := d.handleFraudWarning(context.Background(), nil, raw); err != nil {
		t.Fatalf("handleFraudWarning = %v, want nil", err)
	}

	after := testutil.ToFloat64(metrics.FraudWarningsTotal.WithLabelValues(labels...))
	if after != before+1 {
		t.Errorf("fraud_warnings_total{unattributed} = %v, want %v", after, before+1)
	}
}

func TestHandleFraudWarning_MalformedStillCounts(t *testing.T) {
	labels := []string{"unattributed", fraudAttrMalformed, "false"}
	before := testutil.ToFloat64(metrics.FraudWarningsTotal.WithLabelValues(labels...))

	if err := New(nil).handleFraudWarning(context.Background(), nil, []byte(`{`)); err != nil {
		t.Fatalf("handleFraudWarning = %v, want nil", err)
	}

	after := testutil.ToFloat64(metrics.FraudWarningsTotal.WithLabelValues(labels...))
	if after != before+1 {
		t.Errorf("fraud_warnings_total{malformed_payload} = %v, want %v", after, before+1)
	}
}
