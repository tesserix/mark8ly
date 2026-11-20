package admin

import (
	"encoding/json"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/order"
)

func TestToAdminOrderResponse_IncludesDeliveryOption(t *testing.T) {
	name, est := "Same-day", "1-2 days"
	resp := ToAdminOrderResponse(&order.Order{ShippingOptionName: &name, ShippingOptionEstimate: &est}, nil, nil)
	b, _ := json.Marshal(resp)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if m["shipping_option_name"] != name || m["shipping_option_estimate"] != est {
		t.Fatalf("fields missing: %v", m)
	}
}
