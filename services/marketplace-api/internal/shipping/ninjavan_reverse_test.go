package shipping

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNinjaVan_CreateReverseShipment_ReturnOrder(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"tracking_number":"NVRET1","id":"nv-ret-1"}`)
	}))
	defer srv.Close()

	c := &NinjaVanCarrier{
		apiKey: "k", secretKey: "s", country: "sg", baseURL: srv.URL, client: srv.Client(),
		accessToken: "tok", tokenExpiry: time.Now().Add(time.Hour),
	}
	sh, err := c.CreateReverseShipment(context.Background(), ReverseShipmentRequest{
		OrderID:    "ORD-1",
		PickupFrom: Address{Name: "Jane", PostalCode: "159363", CountryCode: "SG", Phone: "9"},
		ReturnTo:   Address{Name: "WH", PostalCode: "159362", CountryCode: "SG", Phone: "8"},
	})
	if err != nil {
		t.Fatalf("CreateReverseShipment: %v", err)
	}
	if sh.TrackingNumber != "NVRET1" {
		t.Errorf("tracking = %q, want NVRET1", sh.TrackingNumber)
	}
	if gotPath != "/sg/4.2/orders" {
		t.Errorf("path = %q, want /sg/4.2/orders", gotPath)
	}
	if gotBody["service_type"] != "Return" {
		t.Errorf("service_type = %v, want Return", gotBody["service_type"])
	}
	// from = customer (pickup), to = warehouse (destination).
	from, _ := gotBody["from"].(map[string]any)
	if from == nil || from["name"] != "Jane" {
		t.Errorf("from = %v, want customer Jane", gotBody["from"])
	}
	pj, _ := gotBody["parcel_job"].(map[string]any)
	if pj == nil || pj["is_pickup_required"] != true {
		t.Errorf("parcel_job.is_pickup_required = %v, want true", pj)
	}
}
