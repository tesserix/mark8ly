package dispatch

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/refund"
)

func TestParseChargeRefunded(t *testing.T) {
	raw := []byte(`{
      "type": "charge.refunded",
      "data": {
        "object": {
          "id": "ch_123",
          "object": "charge",
          "customer": "cus_123",
          "amount": 9900,
          "amount_refunded": 9900,
          "currency": "aud",
          "refunds": {
            "object": "list",
            "data": [
              {"id": "re_1", "object": "refund"},
              {"id": "re_2", "object": "refund"}
            ]
          }
        }
      }
    }`)

	p, err := parseChargeRefunded(raw)
	if err != nil {
		t.Fatalf("parseChargeRefunded: %v", err)
	}
	if p.ChargeID != "ch_123" {
		t.Errorf("ChargeID = %q, want ch_123", p.ChargeID)
	}
	if p.Customer != "cus_123" {
		t.Errorf("Customer = %q, want cus_123", p.Customer)
	}
	if p.AmountRefunded != 9900 {
		t.Errorf("AmountRefunded = %d, want 9900", p.AmountRefunded)
	}
	if p.Currency != "aud" {
		t.Errorf("Currency = %q, want aud", p.Currency)
	}
	if len(p.RefundIDs) != 2 || p.RefundIDs[0] != "re_1" || p.RefundIDs[1] != "re_2" {
		t.Errorf("RefundIDs = %v, want [re_1 re_2]", p.RefundIDs)
	}
}

func TestParseChargeRefunded_MissingRefundsList(t *testing.T) {
	// Some replays omit the expanded refunds list entirely. That must not be
	// an error: an unknown refund id simply cannot be matched against
	// refund_audit, and the event is still worth recording.
	raw := []byte(`{"data":{"object":{"id":"ch_1","customer":"cus_1","amount_refunded":100,"currency":"usd"}}}`)
	p, err := parseChargeRefunded(raw)
	if err != nil {
		t.Fatalf("parseChargeRefunded: %v", err)
	}
	if len(p.RefundIDs) != 0 {
		t.Errorf("RefundIDs = %v, want empty", p.RefundIDs)
	}
	if p.ChargeID != "ch_1" {
		t.Errorf("ChargeID = %q, want ch_1", p.ChargeID)
	}
}

func TestParseChargeRefunded_Malformed(t *testing.T) {
	if _, err := parseChargeRefunded([]byte(`{`)); err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}

func newRefundCtx() externalRefundContext {
	return externalRefundContext{
		SubscriptionID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		TenantID:       uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		StoreID:        uuid.MustParse("33333333-3333-3333-3333-333333333333"),
	}
}

func samplePayload() chargeRefundedPayload {
	return chargeRefundedPayload{
		ChargeID:       "ch_123",
		Customer:       "cus_123",
		AmountRefunded: 9900,
		Currency:       "aud",
		RefundIDs:      []string{"re_1"},
	}
}

func TestDecideExternalRefundEvent_Emits(t *testing.T) {
	rc := newRefundCtx()
	ev, ok := decideExternalRefundEvent(samplePayload(), false, rc)
	if !ok {
		t.Fatal("expected an event for a refund we did not issue")
	}
	if ev.Action != ActionRefundedExternally {
		t.Errorf("Action = %q, want %q", ev.Action, ActionRefundedExternally)
	}
	if ev.Action == "subscription.refund_issued" {
		t.Error("must not reuse the admin-path action")
	}
	if ev.ResourceType != "subscription" {
		t.Errorf("ResourceType = %q, want subscription", ev.ResourceType)
	}
	if ev.ResourceID != rc.SubscriptionID.String() {
		t.Errorf("ResourceID = %q, want %q", ev.ResourceID, rc.SubscriptionID)
	}
	if ev.TenantID != rc.TenantID || ev.StoreID != rc.StoreID {
		t.Errorf("tenant/store not carried through: %v %v", ev.TenantID, ev.StoreID)
	}
	if ev.Metadata["stripe_charge_id"] != "ch_123" {
		t.Errorf("metadata stripe_charge_id = %v", ev.Metadata["stripe_charge_id"])
	}
	if ev.Metadata["stripe_customer_id"] != "cus_123" {
		t.Errorf("metadata stripe_customer_id = %v", ev.Metadata["stripe_customer_id"])
	}
	if ev.Metadata["amount_refunded_minor"] != int64(9900) {
		t.Errorf("metadata amount_refunded_minor = %v", ev.Metadata["amount_refunded_minor"])
	}
	if ev.Metadata["currency"] != "aud" {
		t.Errorf("metadata currency = %v", ev.Metadata["currency"])
	}
	ids, _ := ev.Metadata["stripe_refund_ids"].([]string)
	if len(ids) != 1 || ids[0] != "re_1" {
		t.Errorf("metadata stripe_refund_ids = %v", ev.Metadata["stripe_refund_ids"])
	}
}

func TestDecideExternalRefundEvent_SuppressedWhenOurs(t *testing.T) {
	// The refund id is already in refund_audit, so refund.Service issued it
	// and EmitRefundIssued already recorded it. Emitting here would
	// double-audit every admin refund.
	if _, ok := decideExternalRefundEvent(samplePayload(), true, newRefundCtx()); ok {
		t.Fatal("expected no event for a refund we issued ourselves")
	}
}

func TestDecideExternalRefundEvent_NoRefundIDsStillEmits(t *testing.T) {
	p := samplePayload()
	p.RefundIDs = nil
	ev, ok := decideExternalRefundEvent(p, false, newRefundCtx())
	if !ok {
		t.Fatal("expected an event even when the payload carries no refund ids")
	}
	if ids, _ := ev.Metadata["stripe_refund_ids"].([]string); len(ids) != 0 {
		t.Errorf("stripe_refund_ids = %v, want empty", ids)
	}
}

func TestEmitExternalRefund_NilEmitterDoesNotPanic(t *testing.T) {
	// Production wiring may pass a nil *audit.Emitter. Emit is nil-safe, and
	// building the event must not panic either.
	var d *Dispatcher = New(nil)
	d.emitExternalRefund(samplePayload(), false, newRefundCtx())
}

// stubRefundRepo implements refund.Repository over an in-memory id set so
// refundIsOurs can be tested without a database. Only the read used by
// handleChargeRefunded is meaningful; the write path must never be reached.
type stubRefundRepo struct {
	known map[string]bool
	err   error
	calls []string
}

func (s *stubRefundRepo) Create(context.Context, *gorm.DB, *refund.RefundAudit) error {
	panic("charge.refunded must never write refund_audit")
}

func (s *stubRefundRepo) ExistsByCardFingerprint(context.Context, *gorm.DB, string) (bool, error) {
	panic("unexpected fingerprint lookup")
}

func (s *stubRefundRepo) ExistsByStore(context.Context, *gorm.DB, string) (bool, error) {
	panic("unexpected store lookup")
}

func (s *stubRefundRepo) ExistsByStripeRefundID(_ context.Context, _ *gorm.DB, id string) (bool, error) {
	s.calls = append(s.calls, id)
	if s.err != nil {
		return false, s.err
	}
	return s.known[id], nil
}

func TestRefundIsOurs(t *testing.T) {
	tests := []struct {
		name      string
		known     map[string]bool
		ids       []string
		want      bool
		wantCalls int
	}{
		{name: "no ids", ids: nil, want: false, wantCalls: 0},
		{name: "unknown id", known: map[string]bool{}, ids: []string{"re_x"}, want: false, wantCalls: 1},
		{name: "known id", known: map[string]bool{"re_1": true}, ids: []string{"re_1"}, want: true, wantCalls: 1},
		{
			name:  "short-circuits on the first match",
			known: map[string]bool{"re_1": true},
			ids:   []string{"re_1", "re_2"},
			want:  true, wantCalls: 1,
		},
		{
			name:  "checks every id before concluding it is external",
			known: map[string]bool{},
			ids:   []string{"re_1", "re_2"},
			want:  false, wantCalls: 2,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &stubRefundRepo{known: tc.known}
			d := New(nil)
			d.refunds = repo
			got, err := d.refundIsOurs(context.Background(), nil, tc.ids)
			if err != nil {
				t.Fatalf("refundIsOurs: %v", err)
			}
			if got != tc.want {
				t.Errorf("refundIsOurs = %v, want %v", got, tc.want)
			}
			if len(repo.calls) != tc.wantCalls {
				t.Errorf("lookups = %v, want %d", repo.calls, tc.wantCalls)
			}
		})
	}
}

func TestRefundIsOurs_NilRepo(t *testing.T) {
	d := New(nil)
	d.refunds = nil
	got, err := d.refundIsOurs(context.Background(), nil, []string{"re_1"})
	if err != nil {
		t.Fatalf("refundIsOurs: %v", err)
	}
	if got {
		t.Error("a nil repo must make the refund look externally initiated, not ours")
	}
}

func TestRefundIsOurs_LookupErrorPropagates(t *testing.T) {
	// A failed lookup must NOT be swallowed into "not ours" — that would
	// double-audit an admin refund.
	d := New(nil)
	d.refunds = &stubRefundRepo{err: errors.New("boom")}
	if _, err := d.refundIsOurs(context.Background(), nil, []string{"re_1"}); err == nil {
		t.Fatal("expected the lookup error to propagate")
	}
}
