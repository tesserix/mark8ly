package shipping

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDelhivery_CreateShipment_SendsTokenAsBearer pins the regression
// that caused zero orders to appear on one.delhivery.com: the admin
// settings handler encrypts the api_key before storage, but the
// shipments handler used to pass that ciphertext straight through to
// Delhivery as "Authorization: Token <ciphertext>". Every call would
// 401 silently while the admin UI reported success. This test
// locks in the plaintext-token contract at the carrier boundary —
// callers that forget to decrypt will now fail here.
func TestDelhivery_CreateShipment_SendsTokenAsBearer(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if !strings.Contains(r.URL.Path, "/api/cmu/create.json") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"success": true, "packages": [{"waybill": "TEST1234", "status": "Success"}]}`)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{
		apiKey:  "plaintext-token-123",
		mode:    "test",
		baseURL: srv.URL,
		client:  srv.Client(),
	}

	sh, err := c.CreateShipment(context.Background(), ShipmentRequest{
		OrderID:  "ORD-1",
		FromAddress: Address{
			Name: "Warehouse A", Line1: "1 Store Rd", City: "Bengaluru",
			Region: "Karnataka", PostalCode: "560001", CountryCode: "IN", Phone: "9000000000",
		},
		ToAddress: Address{
			Name: "Jane Doe", Line1: "42 Example Lane", City: "Mumbai",
			Region: "MH", PostalCode: "400001", CountryCode: "IN", Phone: "9111111111",
		},
		Items:   []ParcelItem{{Title: "Mug", Quantity: 1, WeightGrams: 500}},
		Service: "standard",
	})
	if err != nil {
		t.Fatalf("CreateShipment returned %v", err)
	}
	if sh.TrackingNumber != "TEST1234" {
		t.Errorf("TrackingNumber = %q, want TEST1234", sh.TrackingNumber)
	}
	if gotAuth != "Token plaintext-token-123" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Token plaintext-token-123")
	}
}

// TestDelhivery_CreateShipment_WarehouseNotRegistered verifies the
// remarks classifier turns Delhivery's cryptic error string into a
// self-service remediation message that names the warehouse and
// points the operator at one.delhivery.com → Settings → Warehouses.
func TestDelhivery_CreateShipment_WarehouseNotRegistered(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"success": false,
			"packages": [{
				"waybill": "",
				"status": "Fail",
				"remarks": "ClientWarehouse matching query does not exist"
			}]
		}`)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "k", mode: "live", baseURL: srv.URL, client: srv.Client()}

	_, err := c.CreateShipment(context.Background(), ShipmentRequest{
		OrderID:     "ORD-2",
		FromAddress: Address{Name: "Unregistered WH"},
		ToAddress:   Address{},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// The old test-mode stub would have swallowed this and returned
	// a fake waybill — confirm it doesn't anymore, and that the
	// actionable remediation copy is surfaced.
	msg := err.Error()
	if !strings.Contains(msg, "Unregistered WH") {
		t.Errorf("error missing warehouse name: %v", err)
	}
	if !strings.Contains(msg, "one.delhivery.com") {
		t.Errorf("error missing remediation pointer: %v", err)
	}
}

// TestDelhivery_CreateShipment_InvalidToken ensures a token-rejection
// response is classified with a pointer at the Settings → Shipping
// reconfiguration rather than the opaque upstream string.
func TestDelhivery_CreateShipment_InvalidToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"success": false, "packages": [{"remarks": "Invalid token"}]}`)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "k", mode: "live", baseURL: srv.URL, client: srv.Client()}
	_, err := c.CreateShipment(context.Background(), ShipmentRequest{OrderID: "O"})
	if err == nil || !strings.Contains(err.Error(), "token rejected") {
		t.Fatalf("expected 'token rejected' remediation, got %v", err)
	}
}

// TestDelhivery_FetchLabel_RequiresPDFBody guards against a subtle
// failure: Delhivery sometimes responds 200 OK with a JSON error
// envelope for un-manifested waybills. We must refuse to return those
// bytes as if they were a PDF — the admin UI would happily download
// "label.pdf" filled with JSON.
func TestDelhivery_FetchLabel_RequiresPDFBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"error": "waybill not found"}`)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "k", mode: "live", baseURL: srv.URL, client: srv.Client()}
	_, _, err := c.FetchLabel(context.Background(), "WB404")
	if err == nil {
		t.Fatal("expected error for non-PDF body, got nil")
	}
	if !strings.Contains(err.Error(), "not a PDF") {
		t.Errorf("error should mention 'not a PDF', got %v", err)
	}
}

// TestDelhivery_FetchLabel_StreamsPDF confirms the happy path: PDF
// magic bytes in the body are returned unchanged to the caller with
// the upstream content-type.
func TestDelhivery_FetchLabel_StreamsPDF(t *testing.T) {
	payload := []byte("%PDF-1.4\nfake binary\n%%EOF")
	var gotAuth, gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotURL = r.URL.String()
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "plain-token", mode: "live", baseURL: srv.URL, client: srv.Client()}
	bytes, ct, err := c.FetchLabel(context.Background(), "WB12345")
	if err != nil {
		t.Fatalf("FetchLabel returned %v", err)
	}
	if string(bytes) != string(payload) {
		t.Errorf("PDF body mismatch")
	}
	if ct != "application/pdf" {
		t.Errorf("content-type = %q, want application/pdf", ct)
	}
	if gotAuth != "Token plain-token" {
		t.Errorf("Authorization = %q, want Token plain-token", gotAuth)
	}
	if !strings.Contains(gotURL, "wbns=WB12345") {
		t.Errorf("URL missing wbns param: %q", gotURL)
	}
}
