package shipping

import (
	"strings"
	"testing"
)

// The merchant-facing error must carry Delhivery's short reason, NOT the
// full body — which echoes back the warehouse address, phone and business
// hours (a data leak into the admin UI, flagged 2026-07-18).
func TestDelhiveryWarehouseMessage_ExtractsReasonNotBody(t *testing.T) {
	xml := `<?xml version="1.0" encoding="utf-8"?><root><data>` +
		`<message>some error while creating/updating warehouse</message>` +
		`<name>demo-store</name><phone>9028889903</phone>` +
		`<address>F 103, Kalarahanga, Patia, Bubaneswar</address>` +
		`<pincode>751012</pincode></data></root>`

	got := delhiveryWarehouseMessage(xml)
	if got != "some error while creating/updating warehouse" {
		t.Errorf("message = %q, want the <message> text", got)
	}
	// None of the echoed request data must leak.
	for _, leak := range []string{"9028889903", "Kalarahanga", "751012", "demo-store", "<"} {
		if strings.Contains(got, leak) {
			t.Errorf("extracted message leaks %q: %s", leak, got)
		}
	}
}

func TestDelhiveryWarehouseMessage_JSONAndFallback(t *testing.T) {
	if got := delhiveryWarehouseMessage(`{"rmk":"ClientWarehouse matching query does not exist.","packages":[]}`); got != "ClientWarehouse matching query does not exist." {
		t.Errorf("json rmk = %q", got)
	}
	if got := delhiveryWarehouseMessage(`totally unparseable`); got != "the carrier returned an error" {
		t.Errorf("fallback = %q", got)
	}
}
