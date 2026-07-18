package shipping

import "testing"

// The edit/ endpoint returned this VERBATIM in prod (store my-god,
// 2026-07-18) and the old detector missed it, so create/ never ran and the
// warehouse was never registered despite a "successful" admin save.
func TestDelhiveryWarehouseMissing_MatchesLiveResponses(t *testing.T) {
	missing := []string{
		`<?xml version="1.0" encoding="utf-8"?>` +
			`<root><success>False</success><message>warehouse does not exists</message><error></error><result></result></root>`,
		`{"error":"ClientWarehouse matching query does not exist"}`,
		`{"rmk":"ClientWarehouse matching query does not exist.","packages":[]}`,
		`{"message":"Warehouse does not exist"}`,
	}
	for _, b := range missing {
		if !delhiveryWarehouseMissing(b) {
			t.Errorf("should detect missing warehouse in: %s", b)
		}
	}

	notMissing := []string{
		`{"success":true}`,
		`{"error":"Invalid token"}`,
		`{"message":"pin not serviceable"}`,
		``,
	}
	for _, b := range notMissing {
		if delhiveryWarehouseMissing(b) {
			t.Errorf("should NOT flag as missing: %s", b)
		}
	}
}
