package shipping

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
		_, _ = io.WriteString(w, `{"upload_wbn":"UPL1","packages":[{"waybill":"TEST1234","status":"Success","serviceable":true,"remarks":[]}]}`)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{
		apiKey:  "plaintext-token-123",
		mode:    "test",
		baseURL: srv.URL,
		client:  srv.Client(),
	}

	sh, err := c.CreateShipment(context.Background(), ShipmentRequest{
		OrderID: "ORD-1",
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
			"upload_wbn": "UPL-X",
			"packages": [{
				"waybill": "",
				"status": "Fail",
				"serviceable": true,
				"remarks": ["ClientWarehouse matching query does not exist"]
			}]
		}`)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "k", mode: "live", baseURL: srv.URL, client: srv.Client()}

	_, err := c.CreateShipment(context.Background(), ShipmentRequest{
		OrderID:     "ORD-2",
		FromAddress: Address{Name: "Unregistered WH", PostalCode: "411011"},
		ToAddress:   Address{PostalCode: "560100"},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
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
		_, _ = io.WriteString(w, `{"upload_wbn":"U","packages":[{"waybill":"","status":"Fail","serviceable":true,"remarks":["Invalid token"]}]}`)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "k", mode: "live", baseURL: srv.URL, client: srv.Client()}
	_, err := c.CreateShipment(context.Background(), ShipmentRequest{OrderID: "O"})
	if err == nil || !strings.Contains(err.Error(), "token rejected") {
		t.Fatalf("expected 'token rejected' remediation, got %v", err)
	}
}

// TestDelhivery_CreateShipment_NotServiceable pins the classification
// for the real-world failure that blocked the Playwright flow: Pune →
// Bengaluru 560100 returned serviceable:false on the merchant's tier.
// The old code had remarks declared as string — it failed JSON decode
// before even getting to this branch, so the user just saw an
// unmarshal error. This test guards the whole path:
//   - remarks array decodes cleanly
//   - serviceable:false is the signal we branch on
//   - the error names the origin/destination pincode so the operator
//     knows which leg to enable on Delhivery.
func TestDelhivery_CreateShipment_NotServiceable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"upload_wbn": "UPL865",
			"packages": [{
				"waybill": "",
				"refnum": "abc",
				"status": "Fail",
				"serviceable": false,
				"remarks": ["No serviceable pincodes for this route"]
			}]
		}`)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "k", mode: "live", baseURL: srv.URL, client: srv.Client()}
	_, err := c.CreateShipment(context.Background(), ShipmentRequest{
		OrderID:     "ORD-3",
		FromAddress: Address{Name: "Primary", PostalCode: "411011"},
		ToAddress:   Address{PostalCode: "560100"},
	})
	if err == nil {
		t.Fatal("expected serviceability error, got nil")
	}
	msg := err.Error()
	for _, want := range []string{`"411011"`, `"560100"`, "not serviceable"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error missing %q: %v", want, err)
		}
	}
}

// TestDelhivery_CreateShipment_RemarksIsArray is a tight regression
// guard for the root cause of the last blocker: an earlier struct
// declared remarks as string, which made every real Delhivery
// response fail decode with "cannot unmarshal array into Go struct
// field .packages.remarks of type string". No classification happened;
// the operator saw the raw unmarshal error in the admin UI.
func TestDelhivery_CreateShipment_RemarksIsArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"upload_wbn":"U","packages":[{"waybill":"","status":"Fail","serviceable":false,"remarks":["first","second"]}]}`)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "k", mode: "live", baseURL: srv.URL, client: srv.Client()}
	_, err := c.CreateShipment(context.Background(), ShipmentRequest{OrderID: "O"})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "cannot unmarshal") {
		t.Fatalf("remarks array still broke decode: %v", err)
	}
}

// TestDelhivery_CreateShipment_PhoneMissing_TakesPrecedenceOverServiceable
// pins the fix for the real-world Playwright blocker: Delhivery returns
// serviceable:false AND a remark of "No phone number provided." when the
// shipping address on our side has an empty phone. The previous
// classifier checked !serviceable first, so every phone-missing failure
// was mis-reported as a pincode routing problem. This test asserts the
// specific phone error wins, NOT the serviceability error.
func TestDelhivery_CreateShipment_PhoneMissing_TakesPrecedenceOverServiceable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"upload_wbn": "UPL-PHONE",
			"packages": [{
				"waybill": "",
				"status": "Fail",
				"serviceable": false,
				"remarks": ["No phone number provided."]
			}]
		}`)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "k", mode: "live", baseURL: srv.URL, client: srv.Client()}
	_, err := c.CreateShipment(context.Background(), ShipmentRequest{
		OrderID:     "ORD-PHONE",
		FromAddress: Address{Name: "Primary", PostalCode: "411011"},
		ToAddress:   Address{PostalCode: "400001"},
	})
	if err == nil {
		t.Fatal("expected phone-missing error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "phone number is required") {
		t.Errorf("error should mention 'phone number is required', got %v", err)
	}
	if strings.Contains(msg, "not serviceable") {
		t.Errorf("error should NOT be classified as serviceability issue, got %v", err)
	}
}

// TestDelhivery_UpsertWarehouse_CreatesWhenMissing exercises the
// create-fallback leg: Delhivery's edit/ endpoint returns the "matching
// query does not exist" envelope, and the carrier is expected to retry
// via create/ before returning success.
func TestDelhivery_UpsertWarehouse_CreatesWhenMissing(t *testing.T) {
	var editHits, createHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/backend/clientwarehouse/edit/":
			editHits++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"error":"ClientWarehouse matching query does not exist"}`)
		case "/api/backend/clientwarehouse/create/":
			createHits++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"success":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "k", mode: "live", baseURL: srv.URL, client: srv.Client()}
	err := c.UpsertWarehouse(context.Background(), Warehouse{
		Name:        "Primary",
		Phone:       "9999999999",
		Address:     "1 Store Rd",
		City:        "Pune",
		PinCode:     "411011",
		CountryCode: "IN",
		Region:      "MH",
	})
	if err != nil {
		t.Fatalf("UpsertWarehouse returned %v", err)
	}
	if editHits != 1 {
		t.Errorf("expected 1 edit hit, got %d", editHits)
	}
	if createHits != 1 {
		t.Errorf("expected 1 create hit, got %d", createHits)
	}
}

// TestDelhivery_UpsertWarehouse_EditsWhenPresent pins the happy path:
// the warehouse is already registered under this name, edit/ returns
// 200 with no "does not exist" marker, and create/ must NOT be called.
func TestDelhivery_UpsertWarehouse_EditsWhenPresent(t *testing.T) {
	var editHits, createHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/backend/clientwarehouse/edit/":
			editHits++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"success":true,"data":{"id":42}}`)
		case "/api/backend/clientwarehouse/create/":
			createHits++
			http.Error(w, "create should not be called", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "k", mode: "live", baseURL: srv.URL, client: srv.Client()}
	err := c.UpsertWarehouse(context.Background(), Warehouse{
		Name:        "Primary",
		Address:     "1 Store Rd",
		City:        "Pune",
		PinCode:     "411011",
		CountryCode: "IN",
	})
	if err != nil {
		t.Fatalf("UpsertWarehouse returned %v", err)
	}
	if editHits != 1 {
		t.Errorf("expected 1 edit hit, got %d", editHits)
	}
	if createHits != 0 {
		t.Errorf("expected 0 create hits, got %d", createHits)
	}
}

// TestDelhivery_UpsertWarehouse_SurfacesOtherErrors confirms that a
// non-"does not exist" failure on edit/ bubbles up verbatim instead of
// silently falling through to create/. This matters because it's the
// only way ops can tell "misconfigured token" from "new warehouse."
func TestDelhivery_UpsertWarehouse_SurfacesOtherErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/backend/clientwarehouse/edit/" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"error":"internal server blew up"}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "k", mode: "live", baseURL: srv.URL, client: srv.Client()}
	err := c.UpsertWarehouse(context.Background(), Warehouse{
		Name:        "Primary",
		Address:     "1 Store Rd",
		City:        "Pune",
		PinCode:     "411011",
		CountryCode: "IN",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// The error surfaces Delhivery's reason cleanly — no raw body dump, no
	// status code noise (merchant-facing; see delhiveryWarehouseMessage).
	if !strings.Contains(err.Error(), "internal server blew up") {
		t.Errorf("expected the carrier reason in error, got %v", err)
	}
	if strings.Contains(err.Error(), "{") || strings.Contains(err.Error(), "body=") {
		t.Errorf("error should not dump the raw body, got %v", err)
	}
}

// TestDelhivery_FetchLabel_EmptyEnvelope guards against a subtle
// failure: Delhivery sometimes responds 200 OK with a JSON envelope
// that has no packages / no pdf_download_link. We must refuse to
// return those bytes as if they were a PDF — the admin UI would
// happily download "label.pdf" filled with JSON.
func TestDelhivery_FetchLabel_EmptyEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"packages": []}`)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "k", mode: "live", baseURL: srv.URL, client: srv.Client()}
	_, _, err := c.FetchLabel(context.Background(), "WB404")
	if err == nil {
		t.Fatal("expected error for empty packages, got nil")
	}
	if !strings.Contains(err.Error(), "empty packages") {
		t.Errorf("error should mention 'empty packages', got %v", err)
	}
}

// TestDelhivery_FetchLabel_S3Indirection pins the real production
// behaviour observed with live Delhivery responses: the
// /api/p/packing_slip endpoint returns application/json with a
// pre-signed S3 URL in packages[0].pdf_download_link. An earlier
// version of this code treated the JSON envelope as the PDF and
// shipped it straight through to the browser — labels saved as
// "label.txt". The carrier now follows the S3 redirect and returns
// the real PDF bytes.
func TestDelhivery_FetchLabel_S3Indirection(t *testing.T) {
	pdfPayload := []byte("%PDF-1.4\nreal s3 content\n%%EOF")
	// Stand up a second httptest server to play the role of S3; the
	// packing_slip endpoint returns a URL pointing at it.
	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Pre-signed S3 URLs are self-authenticating — assert no
		// Authorization header is sent on the follow.
		if h := r.Header.Get("Authorization"); h != "" {
			t.Errorf("S3 fetch should not carry Authorization, got %q", h)
		}
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(pdfPayload)
	}))
	defer s3.Close()

	delhivery := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"packages":[{"pdf_download_link":"`+s3.URL+`/label.pdf","status":"ready"}]}`)
	}))
	defer delhivery.Close()

	c := &DelhiveryCarrier{apiKey: "plain-token", mode: "live", baseURL: delhivery.URL, client: delhivery.Client()}
	got, ct, err := c.FetchLabel(context.Background(), "WB12345")
	if err != nil {
		t.Fatalf("FetchLabel returned %v", err)
	}
	if string(got) != string(pdfPayload) {
		t.Errorf("PDF body mismatch")
	}
	if ct != "application/pdf" {
		t.Errorf("content-type = %q, want application/pdf", ct)
	}
}

// TestDelhivery_FetchLabel_S3NotPDF guards a second edge: the S3
// link itself could (in extremely rare pre-provision cases) return
// something that isn't a PDF. We must refuse that too rather than
// hand a junk file to the admin.
func TestDelhivery_FetchLabel_S3NotPDF(t *testing.T) {
	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<html><body>expired</body></html>`)
	}))
	defer s3.Close()

	delhivery := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"packages":[{"pdf_download_link":"`+s3.URL+`/expired"}]}`)
	}))
	defer delhivery.Close()

	c := &DelhiveryCarrier{apiKey: "k", mode: "live", baseURL: delhivery.URL, client: delhivery.Client()}
	_, _, err := c.FetchLabel(context.Background(), "WB404")
	if err == nil || !strings.Contains(err.Error(), "not a PDF") {
		t.Fatalf("expected 'not a PDF' error, got %v", err)
	}
}

// TestDelhivery_SchedulePickup_Success pins the happy path. Delhivery
// returns HTTP 200 with a pr_id / pickup_id numeric body; we must
// populate ProviderPickupID, ScheduledFor, and carry the raw body
// through for audit.
func TestDelhivery_SchedulePickup_Success(t *testing.T) {
	var gotPath, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"pr_id":987654,"pickup_id":987654,"incoming_center_name":"Pune_H","success":"Request received"}`)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "plaintext-token", mode: "live", baseURL: srv.URL, client: srv.Client()}

	date, _ := time.Parse("2006-01-02", "2026-04-25")
	p, err := c.SchedulePickup(context.Background(), PickupRequest{
		WarehouseName:        "Primary",
		Date:                 date,
		TimeStart:            "14:00:00",
		ExpectedPackageCount: 1,
	})
	if err != nil {
		t.Fatalf("SchedulePickup returned %v", err)
	}
	if p.ProviderPickupID != "987654" {
		t.Errorf("ProviderPickupID = %q, want 987654", p.ProviderPickupID)
	}
	if p.ScheduledFor.IsZero() {
		t.Error("ScheduledFor should be populated")
	}
	if len(p.RawResponse) == 0 {
		t.Error("RawResponse should carry the upstream body")
	}
	if gotPath != "/fm/request/new/" {
		t.Errorf("path = %q, want /fm/request/new/", gotPath)
	}
	if gotAuth != "Token plaintext-token" {
		t.Errorf("Authorization = %q, want Token plaintext-token", gotAuth)
	}
	// Verify the request body shape — names match Delhivery's API.
	var parsed map[string]any
	if err := json.Unmarshal([]byte(gotBody), &parsed); err != nil {
		t.Fatalf("request body is not valid JSON: %v (body=%s)", err, gotBody)
	}
	if parsed["pickup_location"] != "Primary" {
		t.Errorf("pickup_location = %v, want Primary", parsed["pickup_location"])
	}
	if parsed["pickup_date"] != "2026-04-25" {
		t.Errorf("pickup_date = %v, want 2026-04-25", parsed["pickup_date"])
	}
	if parsed["pickup_time"] != "14:00:00" {
		t.Errorf("pickup_time = %v, want 14:00:00", parsed["pickup_time"])
	}
	// expected_package_count arrives as float64 from JSON unmarshal.
	if cnt, ok := parsed["expected_package_count"].(float64); !ok || cnt != 1 {
		t.Errorf("expected_package_count = %v, want 1", parsed["expected_package_count"])
	}
}

// TestDelhivery_SchedulePickup_DuplicateIsNotFatal confirms that when
// Delhivery rejects the request with the "already scheduled" signal we
// return ErrPickupAlreadyScheduled AND a populated Pickup with the
// sentinel ID, so the caller can keep the shipment flow moving without
// having to parse the error string. This is the whole point of the
// interface contract — upstream must distinguish "already booked" from
// "failed to book" because the remediation is opposite (persist the
// schedule vs. retry later).
func TestDelhivery_SchedulePickup_DuplicateIsNotFatal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// 400 is what Delhivery actually returns for this case on
		// production — the earlier fm/request endpoint used 200 with
		// success:false but the new path is status-coded.
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"pr_exists":"pickup already scheduled for this client warehouse on this date"}`)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "k", mode: "live", baseURL: srv.URL, client: srv.Client()}
	date, _ := time.Parse("2006-01-02", "2026-04-25")
	p, err := c.SchedulePickup(context.Background(), PickupRequest{
		WarehouseName: "Primary",
		Date:          date,
		TimeStart:     "14:00:00",
	})
	if !errors.Is(err, ErrPickupAlreadyScheduled) {
		t.Fatalf("expected ErrPickupAlreadyScheduled, got %v", err)
	}
	if p == nil {
		t.Fatal("Pickup should be non-nil even on duplicate")
	}
	if p.ProviderPickupID != "already-scheduled" {
		t.Errorf("ProviderPickupID = %q, want already-scheduled", p.ProviderPickupID)
	}
	if p.ScheduledFor.IsZero() {
		t.Error("ScheduledFor should still be populated on duplicate")
	}
}

// TestDelhivery_SchedulePickup_AuthHeader locks in the bearer-token
// contract for the pickup endpoint. The shipments handler used to pass
// the envelope-encrypted ciphertext through as the bearer token for
// create calls, which failed silently with 401 — we guard against the
// same regression on pickup by pinning the exact header shape.
func TestDelhivery_SchedulePickup_AuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"pr_id":1}`)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "plaintext-123", mode: "live", baseURL: srv.URL, client: srv.Client()}
	date, _ := time.Parse("2006-01-02", "2026-04-25")
	if _, err := c.SchedulePickup(context.Background(), PickupRequest{
		WarehouseName: "Primary", Date: date, TimeStart: "14:00:00",
	}); err != nil {
		t.Fatalf("SchedulePickup returned %v", err)
	}
	if gotAuth != "Token plaintext-123" {
		t.Errorf("Authorization = %q, want Token plaintext-123", gotAuth)
	}
}

// TestDelhivery_SchedulePickup_WalletError captures the real-world
// non-duplicate failure mode observed from production probes: Delhivery
// blocks pickups when the prepaid wallet balance is below threshold.
// We must NOT silently swallow that as success; callers want the raw
// error so ops can top up the wallet. Verbatim body from the probe:
//
//	{"prepaid":"Client wallet balance is 298.48 which is less than 500.0"}
func TestDelhivery_SchedulePickup_WalletError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"prepaid":"Client wallet balance is 298.48 which is less than 500.0"}`)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "k", mode: "live", baseURL: srv.URL, client: srv.Client()}
	date, _ := time.Parse("2006-01-02", "2026-04-25")
	_, err := c.SchedulePickup(context.Background(), PickupRequest{
		WarehouseName: "Primary", Date: date, TimeStart: "14:00:00",
	})
	if err == nil {
		t.Fatal("expected error for wallet-balance rejection")
	}
	if errors.Is(err, ErrPickupAlreadyScheduled) {
		t.Errorf("wallet error must NOT be classified as duplicate: %v", err)
	}
	if !strings.Contains(err.Error(), "wallet") {
		t.Errorf("error should surface the wallet-balance body: %v", err)
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

func TestDelhivery_CancelShipment_Success(t *testing.T) {
	var gotPath, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="utf-8"?><root><error></error><status>Success</status><waybill>WBN123</waybill></root>`)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "tok", mode: "live", baseURL: srv.URL, client: srv.Client()}
	if err := c.CancelShipment(context.Background(), "WBN123"); err != nil {
		t.Fatalf("CancelShipment success returned %v", err)
	}
	if gotPath != "/api/p/edit" {
		t.Errorf("path = %q, want /api/p/edit", gotPath)
	}
	if gotAuth != "Token tok" {
		t.Errorf("auth = %q, want Token tok", gotAuth)
	}
	if !strings.Contains(gotBody, `"cancellation"`) || !strings.Contains(gotBody, "WBN123") {
		t.Errorf("body = %q, want waybill + cancellation", gotBody)
	}
}

func TestDelhivery_CancelShipment_FailureBodyOn200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK) // Delhivery returns 200 even on failure
		_, _ = io.WriteString(w, `<?xml version="1.0"?><root><error>Incorrect Waybill/OrderID, please try again</error><status>Failure</status><waybill></waybill></root>`)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "tok", mode: "live", baseURL: srv.URL, client: srv.Client()}
	err := c.CancelShipment(context.Background(), "BAD")
	if err == nil {
		t.Fatal("CancelShipment on <status>Failure</status> returned nil, want error")
	}
	if !strings.Contains(err.Error(), "Incorrect Waybill") {
		t.Errorf("error = %q, want the <error> text", err.Error())
	}
}

func TestDelhivery_CancelShipment_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `<root><error>Invalid token</error></root>`)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "tok", mode: "live", baseURL: srv.URL, client: srv.Client()}
	if err := c.CancelShipment(context.Background(), "WBN"); err == nil {
		t.Fatal("CancelShipment on 401 returned nil, want error")
	}
}

func TestDelhivery_ReturnToOrigin_UsesCancelEndpoint(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `<root><status>Success</status><waybill>WBN9</waybill></root>`)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "tok", mode: "live", baseURL: srv.URL, client: srv.Client()}
	if err := c.ReturnToOrigin(context.Background(), "WBN9"); err != nil {
		t.Fatalf("ReturnToOrigin returned %v", err)
	}
	if gotPath != "/api/p/edit" {
		t.Errorf("path = %q, want /api/p/edit", gotPath)
	}
	if !strings.Contains(gotBody, `"cancellation"`) || !strings.Contains(gotBody, "WBN9") {
		t.Errorf("body = %q, want cancellation + waybill", gotBody)
	}
}

func TestDelhivery_ReturnToOrigin_SurfacesFailureBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `<root><error>Not cancellable in current state</error><status>Failure</status></root>`)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "tok", mode: "live", baseURL: srv.URL, client: srv.Client()}
	err := c.ReturnToOrigin(context.Background(), "WBN9")
	if err == nil || !strings.Contains(err.Error(), "Not cancellable") {
		t.Fatalf("ReturnToOrigin err = %v, want the <error> text", err)
	}
}

func TestDelhivery_CreateReverseShipment_PickupPayload(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = r.ParseForm()
		gotBody = r.FormValue("data")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"packages":[{"waybill":"REV123","status":"Success","serviceable":true,"remarks":[]}]}`)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "tok", mode: "live", baseURL: srv.URL, client: srv.Client()}
	sh, err := c.CreateReverseShipment(context.Background(), ReverseShipmentRequest{
		OrderID:       "ORD-1-RET",
		PickupFrom:    Address{Name: "Jane Doe", Line1: "42 Example Lane", City: "Mumbai", Region: "MH", PostalCode: "400001", CountryCode: "IN", Phone: "9111111111"},
		ReturnTo:      Address{Name: "Warehouse A", Line1: "1 Store Rd", City: "Bengaluru", Region: "Karnataka", PostalCode: "560001", CountryCode: "IN", Phone: "9000000000"},
		WarehouseName: "Warehouse A",
		Items:         []ParcelItem{{Title: "Mug", Quantity: 1, WeightGrams: 500}},
	})
	if err != nil {
		t.Fatalf("CreateReverseShipment: %v", err)
	}
	if sh.TrackingNumber != "REV123" {
		t.Errorf("waybill = %q, want REV123", sh.TrackingNumber)
	}
	if gotPath != "/api/cmu/create.json" {
		t.Errorf("path = %q, want /api/cmu/create.json", gotPath)
	}
	for _, want := range []string{`"payment_mode":"Pickup"`, `"name":"Jane Doe"`, `"pin":"400001"`,
		`"return_add":"1 Store Rd"`, `"return_pin":"560001"`, `"return_phone":"9000000000"`} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("data payload missing %s\ngot: %s", want, gotBody)
		}
	}
}

func TestDelhivery_CreateReverseShipment_FailureRemark(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"packages":[{"waybill":"","status":"Fail","serviceable":true,"remarks":["Return pin not serviceable"]}]}`)
	}))
	defer srv.Close()

	c := &DelhiveryCarrier{apiKey: "tok", mode: "live", baseURL: srv.URL, client: srv.Client()}
	_, err := c.CreateReverseShipment(context.Background(), ReverseShipmentRequest{
		OrderID: "ORD-2", PickupFrom: Address{PostalCode: "400001", Phone: "9"}, ReturnTo: Address{PostalCode: "560001"},
		WarehouseName: "W", Items: []ParcelItem{{Quantity: 1, WeightGrams: 500}},
	})
	if err == nil || !strings.Contains(err.Error(), "serviceable") {
		t.Fatalf("err = %v, want the remark surfaced", err)
	}
}
