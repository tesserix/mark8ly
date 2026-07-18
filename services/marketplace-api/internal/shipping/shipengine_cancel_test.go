package shipping

import (
	"context"
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
