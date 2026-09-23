package webhookevents_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/webhookevents"
)

// A customer event's object IS the customer, so its id is data.object.id and
// there is no `customer` field. Reading only data.object.customer — which
// both routing call sites did — meant every customer.updated resolved to no
// store, took the orphan path, and never reached handleCustomerUpdated: the
// handler that mirrors the merchant's billing email onto store_subscriptions
// and records whether they have a default payment method.
func TestCustomerIDFromPayload(t *testing.T) {
	for _, tc := range []struct {
		name      string
		eventType string
		payload   string
		want      string
	}{
		{
			name:      "customer.updated carries the id on the object itself",
			eventType: "customer.updated",
			payload:   `{"id":"evt_1","type":"customer.updated","data":{"object":{"id":"cus_abc","email":"a@b.com"}}}`,
			want:      "cus_abc",
		},
		{
			name:      "real payloads also stamp object:customer",
			eventType: "customer.updated",
			payload:   `{"data":{"object":{"id":"cus_abc","object":"customer"}}}`,
			want:      "cus_abc",
		},
		{
			name:      "customer.deleted is the same shape",
			eventType: "customer.deleted",
			payload:   `{"data":{"object":{"id":"cus_gone"}}}`,
			want:      "cus_gone",
		},
		{
			// The object here is a subscription, which references a customer.
			// Taking data.object.id would yield sub_..., not cus_....
			name:      "customer.subscription.* references its customer",
			eventType: "customer.subscription.updated",
			payload:   `{"data":{"object":{"id":"sub_123","customer":"cus_xyz"}}}`,
			want:      "cus_xyz",
		},
		{
			name:      "invoices reference their customer",
			eventType: "invoice.paid",
			payload:   `{"data":{"object":{"id":"in_1","customer":"cus_inv"}}}`,
			want:      "cus_inv",
		},
		{
			name:      "an unrelated object with no customer resolves to nothing",
			eventType: "charge.refunded",
			payload:   `{"data":{"object":{"id":"ch_1"}}}`,
			want:      "",
		},
		{
			name:      "malformed json resolves to nothing rather than panicking",
			eventType: "invoice.paid",
			payload:   `{"data":`,
			want:      "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := webhookevents.CustomerIDFromPayload(tc.eventType, []byte(tc.payload))
			require.Equal(t, tc.want, got)
		})
	}
}

// An explicit customer reference always wins, even on a customer.* event, so
// a payload shape we have not anticipated cannot be mis-attributed to the
// object's own id.
func TestCustomerIDFromPayload_PrefersTheExplicitReference(t *testing.T) {
	got := webhookevents.CustomerIDFromPayload("customer.updated",
		[]byte(`{"data":{"object":{"id":"cus_object","customer":"cus_reference"}}}`))
	require.Equal(t, "cus_reference", got)
}
