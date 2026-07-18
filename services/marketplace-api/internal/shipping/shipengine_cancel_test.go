package shipping

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ShipEngine voids by label_id (not the tracking number we persist), and its
// void endpoint returns HTTP 200 even when the void is refused (the outcome is
// in `approved`). These tests pin both: resolve label_id from tracking number,
// then void, honouring `approved`.

func TestShipEngine_CancelShipment_ResolvesLabelThenVoids(t *testing.T) {
	var listQuery, voidPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/labels") && r.URL.Path == "/v1/labels":
			listQuery = r.URL.Query().Get("tracking_number")
			_, _ = w.Write([]byte(`{"labels":[{"label_id":"se-label-99","tracking_number":"TRACK1"}]}`))
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/void"):
			voidPath = r.URL.Path
			_, _ = w.Write([]byte(`{"approved":true,"message":"Label voided"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := &ShipEngineCarrier{apiKey: "k", mode: "live", baseURL: srv.URL, client: srv.Client()}
	if err := c.CancelShipment(context.Background(), "TRACK1"); err != nil {
		t.Fatalf("CancelShipment returned %v", err)
	}
	if listQuery != "TRACK1" {
		t.Errorf("list query tracking_number = %q, want TRACK1", listQuery)
	}
	if voidPath != "/v1/labels/se-label-99/void" {
		t.Errorf("void path = %q, want /v1/labels/se-label-99/void", voidPath)
	}
}

func TestShipEngine_CancelShipment_RefusedVoidIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"labels":[{"label_id":"se-label-1"}]}`))
			return
		}
		// 200 but approved:false — must NOT be treated as success.
		_, _ = w.Write([]byte(`{"approved":false,"message":"Label has already been used and cannot be voided"}`))
	}))
	defer srv.Close()

	c := &ShipEngineCarrier{apiKey: "k", mode: "live", baseURL: srv.URL, client: srv.Client()}
	err := c.CancelShipment(context.Background(), "TRACK1")
	if err == nil {
		t.Fatal("CancelShipment on approved:false returned nil, want error")
	}
	if !strings.Contains(err.Error(), "already been used") {
		t.Errorf("error = %q, want the void message surfaced", err.Error())
	}
}

func TestShipEngine_CancelShipment_NoLabelFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"labels":[]}`))
	}))
	defer srv.Close()

	c := &ShipEngineCarrier{apiKey: "k", mode: "live", baseURL: srv.URL, client: srv.Client()}
	if err := c.CancelShipment(context.Background(), "UNKNOWN"); err == nil {
		t.Fatal("CancelShipment with no matching label returned nil, want error")
	}
}

func TestShipEngine_CreateReverseShipment_ReturnFromLabel(t *testing.T) {
	var listQuery, returnPath, returnBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/labels":
			listQuery = r.URL.Query().Get("tracking_number")
			_, _ = w.Write([]byte(`{"labels":[{"label_id":"se-out-1","tracking_number":"FWD1"}]}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/return"):
			returnPath = r.URL.Path
			b, _ := io.ReadAll(r.Body)
			returnBody = string(b)
			_, _ = w.Write([]byte(`{"label_id":"se-ret-9","tracking_number":"RET9","is_return_label":true,"outbound_label_id":"se-out-1"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := &ShipEngineCarrier{apiKey: "k", mode: "live", baseURL: srv.URL, client: srv.Client()}
	sh, err := c.CreateReverseShipment(context.Background(), ReverseShipmentRequest{
		OriginalTrackingNumber: "FWD1",
		PickupFrom:             Address{Name: "Jane", PostalCode: "94105", CountryCode: "US"},
		ReturnTo:               Address{Name: "WH", PostalCode: "10001", CountryCode: "US"},
	})
	if err != nil {
		t.Fatalf("CreateReverseShipment: %v", err)
	}
	if sh.TrackingNumber != "RET9" || sh.ProviderShipmentID != "se-ret-9" {
		t.Errorf("shipment = %+v, want RET9 / se-ret-9", sh)
	}
	if listQuery != "FWD1" {
		t.Errorf("label lookup tracking = %q, want FWD1", listQuery)
	}
	if returnPath != "/v1/labels/se-out-1/return" {
		t.Errorf("return path = %q, want /v1/labels/se-out-1/return", returnPath)
	}
	if !strings.Contains(returnBody, "charge_event") {
		t.Errorf("return body missing charge_event: %s", returnBody)
	}
}

func TestShipEngine_CreateReverseShipment_RequiresOriginalTracking(t *testing.T) {
	c := &ShipEngineCarrier{apiKey: "k", mode: "live", baseURL: "http://unused", client: &http.Client{}}
	if _, err := c.CreateReverseShipment(context.Background(), ReverseShipmentRequest{}); err == nil {
		t.Fatal("CreateReverseShipment without OriginalTrackingNumber returned nil, want error")
	}
}
