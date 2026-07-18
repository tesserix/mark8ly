package shipping

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Delhivery's clientwarehouse create requires contact_person and
// registered_name (per their API docs). We sent neither, so every
// auto-registration 400'd silently and the merchant's pickup location was
// never created — which surfaced later as "ClientWarehouse does not exist"
// on label creation. This pins the completed payload.
func TestDelhivery_UpsertWarehouse_CreatePayloadHasRequiredFields(t *testing.T) {
	var createBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/backend/clientwarehouse/edit/":
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"error":"ClientWarehouse matching query does not exist"}`)
		case "/api/backend/clientwarehouse/create/":
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &createBody)
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"success":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "k", mode: "live", baseURL: srv.URL, client: srv.Client()}
	if err := c.UpsertWarehouse(context.Background(), Warehouse{
		Name: "My Warehouse", Phone: "9999999999", Address: "1 Store Rd",
		City: "Bengaluru", PinCode: "560076", CountryCode: "IN", Region: "KA",
	}); err != nil {
		t.Fatalf("UpsertWarehouse: %v", err)
	}

	// Both required fields must be present and non-empty.
	if got, _ := createBody["contact_person"].(string); got == "" {
		t.Errorf("contact_person missing/empty in create payload: %v", createBody)
	}
	if got, _ := createBody["registered_name"].(string); got == "" {
		t.Errorf("registered_name missing/empty in create payload: %v", createBody)
	}
	// Default is the warehouse name when not explicitly provided.
	if got := createBody["registered_name"]; got != "My Warehouse" {
		t.Errorf("registered_name = %v, want the warehouse name", got)
	}

	// An empty email must be omitted, not sent as "" (Delhivery validates
	// the format and can bounce a blank value).
	if _, present := createBody["email"]; present {
		t.Errorf("empty email should be omitted, but payload has: %v", createBody["email"])
	}
}

// An explicit ContactPerson / RegisteredName overrides the name default.
func TestDelhivery_UpsertWarehouse_ExplicitFieldsOverrideDefault(t *testing.T) {
	var createBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/backend/clientwarehouse/edit/" {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"error":"ClientWarehouse matching query does not exist"}`)
			return
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &createBody)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"success":true}`)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "k", mode: "live", baseURL: srv.URL, client: srv.Client()}
	if err := c.UpsertWarehouse(context.Background(), Warehouse{
		Name: "WH", Phone: "9999999999", Address: "x", City: "y",
		PinCode: "560076", CountryCode: "IN", Region: "KA",
		ContactPerson: "Asha R", RegisteredName: "Acme Retail Pvt Ltd",
		Email: "ops@acme.example",
	}); err != nil {
		t.Fatalf("UpsertWarehouse: %v", err)
	}
	if createBody["contact_person"] != "Asha R" {
		t.Errorf("contact_person = %v, want Asha R", createBody["contact_person"])
	}
	if createBody["registered_name"] != "Acme Retail Pvt Ltd" {
		t.Errorf("registered_name = %v, want Acme Retail Pvt Ltd", createBody["registered_name"])
	}
	if createBody["email"] != "ops@acme.example" {
		t.Errorf("email = %v, want it sent when present", createBody["email"])
	}
}
