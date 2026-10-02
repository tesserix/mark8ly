package dispatch

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/mark8ly/marketplace-api/pkg/config"
)

// The Stripe billing webhook is gated by THREE independent lists, and an
// event only does work if it appears in all three:
//
//  1. the enabled-events list on the Stripe endpoint (a dashboard setting),
//  2. STRIPE_ALLOWED_EVENT_TYPES, checked in StripeHandler.Handle,
//  3. the dispatcher's handler map.
//
// Only the last two live in this repo, and they had drifted apart in both
// directions at once:
//
//   - `invoice.finalized` had a handler (WithReverseChargeAnnotator) but was
//     absent from the allowlist, so it was stamped processed and dropped at
//     the gate — the §19.2 reverse-charge annotation had never once run.
//   - `radar.early_fraud_warning` appeared in both under a name Stripe does
//     not emit. The real event types are `.created` and `.updated`, the live
//     endpoint subscribed to those, and Dispatch keys on the exact
//     EventType — so #704's fraud attribution had never once run either.
//
// Neither failure raised anything. The handler existed, the allowlist looked
// populated, the endpoint reported a 0% error rate, and the events were
// answered 200 and recorded as processed. This test is the assertion that
// would have caught both.
//
// List (1) still cannot be checked from here. docs/ops/stripe-webhooks.md
// carries the endpoint's expected subscription set, and it is generated from
// TestAllowlistMatchesHandlers's own view of the truth.
func TestAllowlistMatchesHandlers(t *testing.T) {
	handled := productionDispatcher().HandledEventTypes()

	allowed := defaultAllowedEventTypes(t)
	sort.Strings(allowed)

	if !reflect.DeepEqual(handled, allowed) {
		t.Errorf("STRIPE_ALLOWED_EVENT_TYPES and the handler map disagree\n"+
			"  allowed but unhandled: %v\n"+
			"  handled but not allowed: %v",
			missing(allowed, handled), missing(handled, allowed))
	}
}

// Stripe has no bare `radar.early_fraud_warning` event type. Asserted
// separately from the parity check above because the two lists agreed with
// each other on this name for as long as it was wrong — parity alone would
// have stayed green.
func TestNoEventTypeStripeDoesNotEmit(t *testing.T) {
	for _, et := range productionDispatcher().HandledEventTypes() {
		if et == "radar.early_fraud_warning" {
			t.Error("radar.early_fraud_warning is not emitted by Stripe; " +
				"use radar.early_fraud_warning.created / .updated")
		}
		if strings.HasSuffix(et, ".") || strings.TrimSpace(et) != et {
			t.Errorf("malformed event type %q", et)
		}
	}
}

// productionDispatcher builds the dispatcher the way cmd/marketplace-api
// does. Only WithReverseChargeAnnotator registers a handler; the other
// builders attach collaborators, and all of them are nil-safe. A bare New()
// here would silently omit invoice.finalized and the parity check would
// assert the wrong thing.
func productionDispatcher() *Dispatcher {
	return New(nil).WithReverseChargeAnnotator(nil)
}

// defaultAllowedEventTypes reads the `default` struct tag rather than calling
// config.Load, which needs a populated environment. The tag IS the production
// value: no chart in tesserix-k8s sets STRIPE_ALLOWED_EVENT_TYPES.
func defaultAllowedEventTypes(t *testing.T) []string {
	t.Helper()
	f, ok := reflect.TypeOf(config.Config{}).FieldByName("StripeAllowedEventTypes")
	if !ok {
		t.Fatal("config.Config has no StripeAllowedEventTypes field")
	}
	raw := f.Tag.Get("default")
	if raw == "" {
		t.Fatal("StripeAllowedEventTypes has no default tag")
	}
	return strings.Split(raw, ",")
}

// missing returns the members of a that are absent from b.
func missing(a, b []string) []string {
	in := make(map[string]bool, len(b))
	for _, s := range b {
		in[s] = true
	}
	var out []string
	for _, s := range a {
		if !in[s] {
			out = append(out, s)
		}
	}
	return out
}
