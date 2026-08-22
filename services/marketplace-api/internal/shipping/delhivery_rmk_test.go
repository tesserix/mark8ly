package shipping

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Regression: Delhivery rejects a whole request with a TOP-LEVEL "rmk" and
// packages:[] — no per-package remarks. The empty-packages guard returned
// before classifyDelhiveryCreateError could run, so the operator saw
//
//	delhivery: create shipment: empty packages (body={"rmk":"ClientWarehouse ...
//
// instead of the actionable message the classifier already had for exactly
// this case. Body below is the verbatim prod response (store my-god,
// 2026-07-17).
func TestDelhivery_CreateShipment_TopLevelRmkIsClassified(t *testing.T) {
	const prodBody = `{"rmk":"ClientWarehouse matching query does not exist.","error":true,"success":false,"cash_pickups_count":0,"cash_pickups":0,"cod_count":0,"package_count":0,"upload_wbn":null,"replacement_count":0,"cod_amount":0,"prepaid_count":0,"pickups_count":0,"packages":[]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(prodBody))
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "tok", mode: "test", baseURL: srv.URL, client: srv.Client()}

	_, err := c.CreateShipment(context.Background(), ShipmentRequest{
		OrderID:     "ORD-RMK",
		FromAddress: Address{Name: "My Warehouse", Line1: "1 Rd", City: "Bengaluru", Region: "KA", PostalCode: "560076", CountryCode: "IN", Phone: "9000000000"},
		ToAddress:   Address{Name: "Buyer", Line1: "2 Rd", City: "Bengaluru", Region: "KA", PostalCode: "560001", CountryCode: "IN", Phone: "9876543210"},
		Items:       []ParcelItem{{Title: "Item", Quantity: 1, WeightGrams: 500}},
	})
	if err == nil {
		t.Fatal("expected an error")
	}

	// Must name the warehouse and point at the fix, not dump the body.
	if !strings.Contains(err.Error(), "My Warehouse") {
		t.Errorf("error should name the warehouse, got: %v", err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "pickup locations") {
		t.Errorf("error should point to Delhivery Pickup Locations, got: %v", err)
	}
	if strings.Contains(err.Error(), "empty packages") {
		t.Errorf("should not fall through to the raw body dump, got: %v", err)
	}
}
