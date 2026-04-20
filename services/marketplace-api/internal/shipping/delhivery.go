package shipping

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// ErrPickupAlreadyScheduled is returned by SchedulePickup when
// Delhivery rejects the request because an equivalent pickup already
// exists for the (warehouse, date) pair. Callers treat this as a
// non-fatal signal — the pickup was already booked, by us or by a
// previous retry — and proceed as if success. This mirrors
// gorm.ErrRecordNotFound in shape: compare with errors.Is.
var ErrPickupAlreadyScheduled = errors.New("delhivery: pickup already scheduled for this warehouse + date")

const (
	delhiveryLiveURL    = "https://track.delhivery.com"
	delhiverySandboxURL = "https://staging-express.delhivery.com"
)

// DelhiveryCarrier implements Carrier for the Delhivery API (India domestic).
type DelhiveryCarrier struct {
	apiKey  string
	mode    string
	baseURL string
	client  *http.Client
}

// NewDelhiveryCarrier constructs a Delhivery carrier instance.
//
// Note: Delhivery's "staging-express" host rejects all tokens (tested
// 2026-04-14 — returns `<detail>Invalid token</detail>`), so we use the
// production host for both modes. The `mode` is still carried through
// so downstream shipment creation can flag pickups as test shipments,
// but rate calculation has no meaningful sandbox.
func NewDelhiveryCarrier(apiKey, mode string) *DelhiveryCarrier {
	base := delhiveryLiveURL
	_ = delhiverySandboxURL // retained for documentation; see note above
	return &DelhiveryCarrier{
		apiKey:  apiKey,
		mode:    mode,
		baseURL: base,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *DelhiveryCarrier) ProviderName() string { return "delhivery" }

func (c *DelhiveryCarrier) SupportedCountries() []string {
	return []string{"IN"}
}

// --- request / response types (private, match Delhivery JSON schema) ---

type dlRateRequest struct {
	OriginPin string  `json:"o_pin"`
	DestPin   string  `json:"d_pin"`
	WeightKg  float64 `json:"cgm"`
	Mode      string  `json:"md"`  // "S" for surface, "E" for express
	Service   string  `json:"ss"`  // e.g. "Delhivery"
	PayType   string  `json:"pt"`  // "Pre-paid" or "COD"
}

type dlRateResponse struct {
	TotalAmount float64 `json:"total_amount"`
	ETD         string  `json:"etd"` // e.g. "3-5 days"
}

type dlCreateRequest struct {
	Format      string `json:"format"`
	Data        string `json:"data"`
	PickupTime  string `json:"pickup_time,omitempty"`
}

type dlShipmentData struct {
	Shipments []dlShipmentEntry `json:"shipments"`
	PickupLocation struct {
		Name        string `json:"name"`
		AddLine1    string `json:"add"`
		City        string `json:"city"`
		PinCode     string `json:"pin_code"`
		Country     string `json:"country"`
		Phone       string `json:"phone"`
		State       string `json:"state"`
	} `json:"pickup_location"`
}

type dlShipmentEntry struct {
	Name          string  `json:"name"`
	Add           string  `json:"add"`
	City          string  `json:"city"`
	Pin           string  `json:"pin"`
	State         string  `json:"state"`
	Country       string  `json:"country"`
	Phone         string  `json:"phone"`
	OrderID       string  `json:"order"`
	PaymentMode   string  `json:"payment_mode"`
	ProductDesc   string  `json:"products_desc"`
	Weight        float64 `json:"weight"` // grams
	TotalAmount   float64 `json:"total_amount"`
}

// dlCreateResponse mirrors Delhivery's /api/cmu/create.json response.
// Two schema details that have bitten this code before:
//
//  1. There is NO top-level `success` boolean. Success is per-package
//     via Packages[i].Status ("Success" / "Fail") and a non-empty
//     Waybill. An earlier version read `success` and silently treated
//     every response as a failure because the field didn't exist.
//  2. Remarks is an ARRAY of strings, not a single string. Declaring
//     it as `string` here caused every live create to fail decode with
//     "cannot unmarshal array into Go struct field .packages.remarks
//     of type string" — masking whatever reason Delhivery actually
//     returned (serviceability, invalid warehouse, bad token, etc.).
type dlCreateResponse struct {
	UploadWBN string      `json:"upload_wbn"`
	Packages  []dlPackage `json:"packages"`
}

type dlPackage struct {
	Waybill     string   `json:"waybill"`
	Refnum      string   `json:"refnum"`
	Status      string   `json:"status"`
	Remarks     []string `json:"remarks"`
	Serviceable bool     `json:"serviceable"`
	SortCode    *string  `json:"sort_code"`
	Payment     string   `json:"payment"`
}

type dlTrackingResponse struct {
	ShipmentData []struct {
		Shipment struct {
			Status struct {
				Status      string `json:"Status"`
				StatusType  string `json:"StatusType"`
			} `json:"Status"`
			Scans []struct {
				ScanDetail struct {
					ScanType        string `json:"ScanType"`
					Instructions    string `json:"Instructions"`
					ScannedLocation string `json:"ScannedLocation"`
					ScanDateTime    string `json:"ScanDateTime"`
				} `json:"ScanDetail"`
			} `json:"Scans"`
		} `json:"Shipment"`
	} `json:"ShipmentData"`
}

// --- Carrier interface methods ---

func (c *DelhiveryCarrier) GetRates(ctx context.Context, in RateRequest) ([]Rate, error) {
	totalWeightGrams := 0
	for _, item := range in.Items {
		totalWeightGrams += item.WeightGrams * item.Quantity
	}

	// Delhivery /kinko/v1/invoice/charges query params:
	//   md    — mode: "S" (Surface, cheaper) or "E" (Express). Required.
	//   ss    — status: DTO/RTO/Delivered. Required; "Delivered" is the
	//           standard value for forward rate calculation.
	//   cgm   — chargeable weight in grams. Required.
	//   o_pin — origin pincode. Required.
	//   d_pin — destination pincode. Required.
	//   pt    — payment type: "Pre-paid" or "COD". Required.
	// Previously md/ss were being mis-populated with postal codes, which
	// made Delhivery return 400 ("md is mandatory field … can be E,S" /
	// "ss is mandatory field … DTO,RTO,Delivered") and blocked every
	// rate call, bubbling as HTTP 500 on /api/checkout/shipping-rates.
	params := url.Values{
		"md":    {"S"},
		"ss":    {"Delivered"},
		"cgm":   {fmt.Sprintf("%d", totalWeightGrams)},
		"o_pin": {in.FromAddress.PostalCode},
		"d_pin": {in.ToAddress.PostalCode},
		"pt":    {"Pre-paid"},
	}

	path := "/api/kinko/v1/invoice/charges/.json?" + params.Encode()
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("delhivery: get rates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("delhivery: get rates: unexpected status %d", resp.StatusCode)
	}

	var rateSlice []dlRateResponse
	if err := json.NewDecoder(resp.Body).Decode(&rateSlice); err != nil {
		return nil, fmt.Errorf("delhivery: get rates: decode: %w", err)
	}

	rates := make([]Rate, 0, len(rateSlice))
	for _, r := range rateSlice {
		rates = append(rates, Rate{
			Service:       "standard",
			Carrier:       "delhivery",
			Price:         decimal.NewFromFloat(r.TotalAmount),
			CurrencyCode:  "INR",
			EstimatedDays: parseDelhiveryETD(r.ETD),
		})
	}
	return rates, nil
}

func (c *DelhiveryCarrier) CreateShipment(ctx context.Context, in ShipmentRequest) (*Shipment, error) {
	totalWeightGrams := 0
	for _, item := range in.Items {
		totalWeightGrams += item.WeightGrams * item.Quantity
	}

	productDesc := ""
	for i, item := range in.Items {
		if i > 0 {
			productDesc += ", "
		}
		productDesc += item.Title
	}

	shipmentData := dlShipmentData{}
	shipmentData.PickupLocation.Name = in.FromAddress.Name
	shipmentData.PickupLocation.AddLine1 = in.FromAddress.Line1
	shipmentData.PickupLocation.City = in.FromAddress.City
	shipmentData.PickupLocation.PinCode = in.FromAddress.PostalCode
	shipmentData.PickupLocation.Country = in.FromAddress.CountryCode
	shipmentData.PickupLocation.Phone = in.FromAddress.Phone
	shipmentData.PickupLocation.State = in.FromAddress.Region
	shipmentData.Shipments = []dlShipmentEntry{
		{
			Name:        in.ToAddress.Name,
			Add:         in.ToAddress.Line1,
			City:        in.ToAddress.City,
			Pin:         in.ToAddress.PostalCode,
			State:       in.ToAddress.Region,
			Country:     in.ToAddress.CountryCode,
			Phone:       in.ToAddress.Phone,
			OrderID:     in.OrderID,
			PaymentMode: "Pre-paid",
			ProductDesc: productDesc,
			Weight:      float64(totalWeightGrams),
		},
	}

	dataJSON, err := json.Marshal(shipmentData)
	if err != nil {
		return nil, fmt.Errorf("delhivery: create shipment: marshal data: %w", err)
	}

	form := url.Values{
		"format": {"json"},
		"data":   {string(dataJSON)},
	}

	resp, err := c.doForm(ctx, "/api/cmu/create.json", form)
	if err != nil {
		return nil, fmt.Errorf("delhivery: create shipment: %w", err)
	}
	defer resp.Body.Close()

	// Read the raw body so we can include the provider's error text in
	// our error message — otherwise the operator sees a generic
	// "API returned failure" and has no way to self-serve.
	bodyBytes, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, fmt.Errorf("delhivery: create shipment: read body: %w", readErr)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("delhivery: create shipment: status=%d body=%s",
			resp.StatusCode, truncate(string(bodyBytes), 400))
	}

	var cr dlCreateResponse
	if err := json.Unmarshal(bodyBytes, &cr); err != nil {
		return nil, fmt.Errorf("delhivery: create shipment: decode: %w (body=%s)",
			err, truncate(string(bodyBytes), 400))
	}
	if len(cr.Packages) == 0 {
		return nil, fmt.Errorf("delhivery: create shipment: empty packages (body=%s)",
			truncate(string(bodyBytes), 400))
	}
	pkg := cr.Packages[0]
	// Delhivery signals failure two ways:
	//   - pkg.Status == "Fail" (string, case-insensitive in practice), OR
	//   - pkg.Waybill == ""    (success always includes a non-empty waybill).
	// We also flag pkg.Serviceable == false up-front since it has the
	// most specific remediation.
	if strings.EqualFold(pkg.Status, "Fail") || pkg.Waybill == "" {
		remark := joinRemarks(pkg.Remarks)
		return nil, classifyDelhiveryCreateError(remark, in.FromAddress.Name,
			in.FromAddress.PostalCode, in.ToAddress.PostalCode, pkg.Serviceable)
	}

	return &Shipment{
		ProviderShipmentID: pkg.Waybill,
		TrackingNumber:     pkg.Waybill,
		Carrier:            "delhivery",
		Service:            "standard",
	}, nil
}

// joinRemarks flattens Delhivery's remarks array into a single string
// suitable for error messages. Most responses carry exactly one
// element, but we guard for the multi-element case (and the empty
// case — Delhivery occasionally returns "packages":[{}] with no
// remarks key at all).
func joinRemarks(rs []string) string {
	cleaned := make([]string, 0, len(rs))
	for _, r := range rs {
		r = strings.TrimSpace(r)
		if r != "" {
			cleaned = append(cleaned, r)
		}
	}
	return strings.Join(cleaned, "; ")
}

// classifyDelhiveryCreateError turns Delhivery's opaque remarks string
// into an actionable error. The common failures in practice:
//
//  1. "No phone number provided" — the destination address on our end
//     had an empty phone. Delhivery ALSO returns serviceable:false
//     alongside this remark, so we check the specific remark patterns
//     BEFORE the serviceability branch below — otherwise every phone-
//     missing failure would be mis-classified as a pincode routing
//     issue, which is impossible for an operator to self-diagnose.
//  2. "ClientWarehouse matching query does not exist" — the warehouse
//     name we sent isn't registered on one.delhivery.com → Settings
//     → Pickup Locations. Case-sensitive: "Primary" ≠ "primary".
//  3. "Invalid token" — the API key is wrong, rotated, or the caller
//     sent the envelope-encrypted ciphertext through without
//     decrypting it (regression risk; pinned by delhivery_test.go).
//  4. serviceable == false — default branch, after all the specific
//     remarks above. Origin/destination pincode pair isn't covered by
//     the merchant's Delhivery service tier. Remediation is on the
//     merchant side (enable the route on one.delhivery.com → Pricing
//     / Services) or use a destination pincode they already serve.
//  5. Anything else — bubble the remarks verbatim so ops can triage.
func classifyDelhiveryCreateError(remarks, warehouseName, fromPin, toPin string, serviceable bool) error {
	lower := strings.ToLower(remarks)
	switch {
	case strings.Contains(lower, "no phone number") || strings.Contains(lower, "phone number provided"):
		return fmt.Errorf(
			"delhivery: customer phone number is required on the shipping address — ask the customer to add their phone and retry. (carrier: %s)", remarks)
	case strings.Contains(lower, "clientwarehouse") && strings.Contains(lower, "does not exist"):
		return fmt.Errorf(
			"delhivery: warehouse %q is not registered on your Delhivery account — open one.delhivery.com → Settings → Pickup Locations and add one named exactly %q (case-sensitive), then retry. (carrier: %s)",
			warehouseName, warehouseName, remarks)
	case strings.Contains(lower, "invalid token") || strings.Contains(lower, "unauthorized"):
		return fmt.Errorf(
			"delhivery: API token rejected — verify the token in Settings → Shipping matches the one on one.delhivery.com → Settings → API. (carrier: %s)",
			remarks)
	case !serviceable || strings.Contains(lower, "not serviceable") || strings.Contains(lower, "no serviceable"):
		return fmt.Errorf(
			"delhivery: pincode %q → %q is not serviceable on your Delhivery account — either enable this route on one.delhivery.com → Pricing, or test with a destination pincode you already serve. (carrier: %s)",
			fromPin, toPin, remarks)
	default:
		if remarks == "" {
			remarks = "no remarks returned"
		}
		return fmt.Errorf("delhivery: create shipment: %s", remarks)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func (c *DelhiveryCarrier) GetTracking(ctx context.Context, trackingNumber string) (*Tracking, error) {
	path := fmt.Sprintf("/api/v1/packages/json/?waybill=%s", url.QueryEscape(trackingNumber))

	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("delhivery: get tracking: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("delhivery: get tracking: unexpected status %d", resp.StatusCode)
	}

	var tr dlTrackingResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, fmt.Errorf("delhivery: get tracking: decode: %w", err)
	}

	if len(tr.ShipmentData) == 0 {
		return nil, fmt.Errorf("delhivery: get tracking: no shipment data returned")
	}

	shipment := tr.ShipmentData[0].Shipment

	events := make([]TrackingEvent, 0, len(shipment.Scans))
	for _, s := range shipment.Scans {
		ts, _ := time.Parse("2006-01-02T15:04:05", s.ScanDetail.ScanDateTime)
		events = append(events, TrackingEvent{
			Status:      s.ScanDetail.ScanType,
			Description: s.ScanDetail.Instructions,
			Location:    s.ScanDetail.ScannedLocation,
			Timestamp:   ts,
		})
	}

	return &Tracking{
		TrackingNumber: trackingNumber,
		Status:         MapDelhiveryStatus(shipment.Status.StatusType),
		Events:         events,
	}, nil
}

// FetchLabel pulls the packing-slip PDF for an already-created waybill.
//
// Delhivery's /api/p/packing_slip?wbns=<waybill>&pdf=true returns
// application/json — NOT a PDF — with a pre-signed S3 URL in
// packages[0].pdf_download_link that the caller must fetch to get
// the actual bytes. An earlier version of this code assumed the
// response was the PDF directly and passed the JSON envelope through
// to the browser, which saved it as "label.txt" or a broken PDF.
// We now follow the redirect ourselves so the carrier detail stays
// contained and the caller always gets the real PDF bytes.
func (c *DelhiveryCarrier) FetchLabel(ctx context.Context, trackingNumber string) ([]byte, string, error) {
	if trackingNumber == "" {
		return nil, "", fmt.Errorf("delhivery: fetch label: trackingNumber is required")
	}
	path := fmt.Sprintf("/api/p/packing_slip?wbns=%s&pdf=true", url.QueryEscape(trackingNumber))
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, "", fmt.Errorf("delhivery: fetch label: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("delhivery: fetch label: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("delhivery: fetch label: status=%d body=%s",
			resp.StatusCode, truncate(string(body), 400))
	}
	contentType := resp.Header.Get("Content-Type")
	// Happy path when Delhivery decides to return the PDF inline — rare
	// but historically seen on some older waybills. Keep for back-compat.
	if strings.HasPrefix(strings.ToLower(contentType), "application/pdf") ||
		bytes.HasPrefix(body, []byte("%PDF")) {
		if contentType == "" {
			contentType = "application/pdf"
		}
		return body, contentType, nil
	}
	// Modern Delhivery response: JSON with packages[].pdf_download_link.
	var env struct {
		Packages []struct {
			PDFDownloadLink string `json:"pdf_download_link"`
			Status          string `json:"status"`
			Error           string `json:"error"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, "", fmt.Errorf("delhivery: fetch label: decode envelope: %w (body=%s)",
			err, truncate(string(body), 400))
	}
	if len(env.Packages) == 0 {
		return nil, "", fmt.Errorf("delhivery: fetch label: empty packages (body=%s)",
			truncate(string(body), 400))
	}
	pkg := env.Packages[0]
	if pkg.PDFDownloadLink == "" {
		msg := pkg.Error
		if msg == "" {
			msg = pkg.Status
		}
		if msg == "" {
			msg = "no pdf_download_link returned"
		}
		return nil, "", fmt.Errorf("delhivery: fetch label: %s", msg)
	}
	// The pre-signed S3 URL is short-lived (≈24h) and self-authenticating
	// — no Authorization header needed. Using the same *http.Client
	// gives us timeouts and TLS verification for free.
	pdfReq, err := http.NewRequestWithContext(ctx, http.MethodGet, pkg.PDFDownloadLink, nil)
	if err != nil {
		return nil, "", fmt.Errorf("delhivery: fetch label: build s3 request: %w", err)
	}
	pdfResp, err := c.client.Do(pdfReq)
	if err != nil {
		return nil, "", fmt.Errorf("delhivery: fetch label: s3 fetch: %w", err)
	}
	defer pdfResp.Body.Close()
	pdf, err := io.ReadAll(pdfResp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("delhivery: fetch label: read s3 body: %w", err)
	}
	if pdfResp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("delhivery: fetch label: s3 status=%d body=%s",
			pdfResp.StatusCode, truncate(string(pdf), 200))
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF")) {
		return nil, "", fmt.Errorf("delhivery: fetch label: s3 body is not a PDF (first-bytes=%q)",
			string(pdf[:min(16, len(pdf))]))
	}
	pdfCT := pdfResp.Header.Get("Content-Type")
	if pdfCT == "" {
		pdfCT = "application/pdf"
	}
	return pdf, pdfCT, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (c *DelhiveryCarrier) CancelShipment(ctx context.Context, shipmentID string) error {
	body := map[string]string{
		"waybill": shipmentID,
		"cancellation": "true",
	}

	resp, err := c.doJSONRequest(ctx, http.MethodPost, "/api/p/edit", body)
	if err != nil {
		return fmt.Errorf("delhivery: cancel shipment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("delhivery: cancel shipment: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// UpsertWarehouse keeps one.delhivery.com → Settings → Pickup Locations
// in sync with the admin's Shipping settings. Delhivery exposes separate
// create and edit endpoints keyed by warehouse name; it does NOT expose
// an idempotent upsert. We try edit/ first and fall through to create/
// on the specific "does not exist" error — both legs return 200 on
// success, so the status code isn't enough to discriminate.
//
// Shared payload: Delhivery's clientwarehouse endpoints use the same
// JSON shape as the billing "register warehouse" form on their dashboard.
// We reuse the merchant's warehouse address for the return_* fields
// because Delhivery requires them on both create and edit and
// marketplace-api doesn't model a separate RTO address yet.
func (c *DelhiveryCarrier) UpsertWarehouse(ctx context.Context, wh Warehouse) error {
	if strings.TrimSpace(wh.Name) == "" {
		return fmt.Errorf("delhivery: upsert warehouse: name is required")
	}

	payload := map[string]any{
		"name":            wh.Name,
		"email":           wh.Email,
		"phone":           wh.Phone,
		"address":         wh.Address,
		"city":            wh.City,
		"country":         wh.CountryCode,
		"pin":             wh.PinCode,
		"return_address":  wh.Address,
		"return_pin":      wh.PinCode,
		"return_city":     wh.City,
		"return_state":    wh.Region,
		"return_country":  wh.CountryCode,
	}

	// Step 1: try edit/. If the warehouse is already registered under
	// this name, Delhivery returns success.
	editBody, editStatus, editErr := c.doWarehouseRequest(ctx, "/api/backend/clientwarehouse/edit/", payload)
	if editErr != nil {
		return fmt.Errorf("delhivery: upsert warehouse: edit request: %w", editErr)
	}
	if editStatus == http.StatusOK && !delhiveryWarehouseMissing(editBody) {
		return nil
	}

	// Step 2: edit reported the warehouse doesn't exist → create it.
	// Any other non-200 on edit is surfaced as-is so ops can see it.
	if editStatus != http.StatusOK && !delhiveryWarehouseMissing(editBody) {
		return fmt.Errorf("delhivery: upsert warehouse: edit status=%d body=%s",
			editStatus, truncate(editBody, 400))
	}

	createBody, createStatus, createErr := c.doWarehouseRequest(ctx, "/api/backend/clientwarehouse/create/", payload)
	if createErr != nil {
		return fmt.Errorf("delhivery: upsert warehouse: create request: %w", createErr)
	}
	if createStatus != http.StatusOK {
		return fmt.Errorf("delhivery: upsert warehouse: create status=%d body=%s",
			createStatus, truncate(createBody, 400))
	}
	return nil
}

// doWarehouseRequest is a small helper around the clientwarehouse/*
// endpoints: JSON POST + Token auth, returning the body bytes as a
// string so both callers can scan for Delhivery's "matching query does
// not exist" phrase. Returns (body, statusCode, transportErr).
func (c *DelhiveryCarrier) doWarehouseRequest(ctx context.Context, path string, payload any) (string, int, error) {
	resp, err := c.doJSONRequest(ctx, http.MethodPost, path, payload)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp.StatusCode, fmt.Errorf("read body: %w", err)
	}
	return string(body), resp.StatusCode, nil
}

// delhiveryWarehouseMissing detects the "fall through to create" signal.
// Delhivery's edit endpoint returns this exact phrase as part of a JSON
// envelope when the name doesn't resolve to an existing warehouse.
func delhiveryWarehouseMissing(body string) bool {
	lower := strings.ToLower(body)
	return strings.Contains(lower, "clientwarehouse matching query does not exist") ||
		strings.Contains(lower, "warehouse matching query does not exist")
}

// SchedulePickup requests a Delhivery pickup-executive dispatch for the
// merchant's warehouse on the given date and slot. Without this call,
// a freshly-created waybill stays at "Manifested" status on
// one.delhivery.com and the merchant has to click "Add to Pickup"
// manually before a pickup agent is routed. Calling this endpoint
// closes that gap so the admin "Create shipping label" click is
// genuinely one-shot.
//
// Delhivery quirks this implementation pins:
//
//  1. Endpoint is /fm/request/new/ with a trailing slash — omitting
//     the trailing slash 404s on production. The body is JSON, not
//     form-encoded (the /api/cmu/create.json path differs here).
//  2. Success path: HTTP 200 + {"pr_id": 12345, "pickup_id": 12345,
//     ...}. Delhivery aliases pr_id and pickup_id in different
//     responses; we accept either.
//  3. Duplicate path: HTTP 400 with a body containing "already" +
//     "scheduled"/"exists" in some field. Translated to
//     ErrPickupAlreadyScheduled so the caller can treat it as
//     success with unknown-id metadata.
//  4. Wallet / token failures: surface verbatim so the operator can
//     self-diagnose. Observed shapes:
//       {"prepaid":"Client wallet balance is 298.48 which is less than 500.0"}
//       {"detail":"Invalid token"}
//
// ExpectedPackageCount defaults to max(1, req.ExpectedPackageCount)
// because the endpoint rejects 0 with "expected_package_count must be
// a positive integer".
func (c *DelhiveryCarrier) SchedulePickup(ctx context.Context, req PickupRequest) (*Pickup, error) {
	if strings.TrimSpace(req.WarehouseName) == "" {
		return nil, fmt.Errorf("delhivery: schedule pickup: warehouse_name is required")
	}
	if req.Date.IsZero() {
		return nil, fmt.Errorf("delhivery: schedule pickup: date is required")
	}
	timeStart := strings.TrimSpace(req.TimeStart)
	if timeStart == "" {
		timeStart = "14:00:00"
	}
	pkgCount := req.ExpectedPackageCount
	if pkgCount <= 0 {
		pkgCount = 1
	}
	dateStr := req.Date.Format("2006-01-02")

	body := map[string]any{
		"pickup_location":        req.WarehouseName,
		"pickup_date":            dateStr,
		"pickup_time":            timeStart,
		"expected_package_count": pkgCount,
	}
	resp, err := c.doJSONRequest(ctx, http.MethodPost, "/fm/request/new/", body)
	if err != nil {
		return nil, fmt.Errorf("delhivery: schedule pickup: %w", err)
	}
	defer resp.Body.Close()
	raw, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, fmt.Errorf("delhivery: schedule pickup: read body: %w", readErr)
	}

	// Build the combined ScheduledFor timestamp once — both branches
	// reuse it. Falls back to the date-only midnight UTC if the time
	// slot is malformed, so callers never see a zero-value time.
	scheduledFor := combinePickupDateTime(req.Date, timeStart)

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		pickupID, parseErr := parseDelhiveryPickupID(raw)
		if parseErr != nil {
			return nil, fmt.Errorf("delhivery: schedule pickup: %w (body=%s)",
				parseErr, truncate(string(raw), 400))
		}
		return &Pickup{
			ProviderPickupID: pickupID,
			ScheduledFor:     scheduledFor,
			RawResponse:      raw,
		}, nil
	}

	// Duplicate detection. Delhivery sometimes returns 400, sometimes
	// 409 — treat any non-2xx whose body hints at duplication as the
	// idempotent path. We also accept the phrase "already added" which
	// Delhivery uses when the pickup has been rolled into an existing
	// request for the same warehouse + date.
	if isDelhiveryDuplicatePickup(raw) {
		return &Pickup{
			ProviderPickupID: "already-scheduled",
			ScheduledFor:     scheduledFor,
			RawResponse:      raw,
		}, ErrPickupAlreadyScheduled
	}

	return nil, fmt.Errorf("delhivery: schedule pickup: status=%d body=%s",
		resp.StatusCode, truncate(string(raw), 400))
}

// parseDelhiveryPickupID pulls Delhivery's pickup identifier out of a
// success body. The endpoint is inconsistent across docs and responses —
// real observed keys include pr_id (numeric), pickup_id (numeric), and
// pickup_request_id (numeric). We accept any of them. Numbers are
// coerced to string via json.Number so we don't lose precision on the
// larger IDs.
func parseDelhiveryPickupID(body []byte) (string, error) {
	var env struct {
		PrID            json.Number `json:"pr_id"`
		PickupID        json.Number `json:"pickup_id"`
		PickupRequestID json.Number `json:"pickup_request_id"`
		IncomingName    string      `json:"incoming_center_name"`
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&env); err != nil {
		return "", fmt.Errorf("decode pickup response: %w", err)
	}
	for _, candidate := range []json.Number{env.PrID, env.PickupID, env.PickupRequestID} {
		s := strings.TrimSpace(string(candidate))
		if s != "" && s != "0" {
			return s, nil
		}
	}
	// Some older Delhivery endpoints returned the pickup_id as a plain
	// string inside an "id" field. Fall through to a permissive scan.
	var fallback map[string]any
	if err := json.Unmarshal(body, &fallback); err == nil {
		for _, k := range []string{"id", "request_id"} {
			if v, ok := fallback[k]; ok {
				if s, ok := v.(string); ok && s != "" {
					return s, nil
				}
				if n, ok := v.(json.Number); ok && n != "" {
					return string(n), nil
				}
			}
		}
	}
	return "", fmt.Errorf("no pickup id in response")
}

// isDelhiveryDuplicatePickup scans the response body for the phrases
// Delhivery uses when a pickup already exists for the submitted
// (warehouse, date). Delhivery has used at least three wordings in the
// wild, hence the multi-pattern check.
func isDelhiveryDuplicatePickup(body []byte) bool {
	lower := strings.ToLower(string(body))
	if !strings.Contains(lower, "already") {
		return false
	}
	switch {
	case strings.Contains(lower, "already scheduled"),
		strings.Contains(lower, "already exists"),
		strings.Contains(lower, "already added"),
		strings.Contains(lower, "pickup already"):
		return true
	}
	return false
}

// combinePickupDateTime merges a YYYY-MM-DD date and an "HH:MM:SS" slot
// into a single time.Time. IST (Asia/Kolkata) is the source timezone
// because Delhivery expresses all pickup windows in India-local time on
// the dashboard, but we return UTC so the DB's timestamptz columns
// remain consistent. Falls back to the date at UTC midnight on parse
// failure — better a slightly-wrong display than a zero-value time
// that blows up downstream formatters.
func combinePickupDateTime(date time.Time, slot string) time.Time {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		loc = time.UTC
	}
	y, m, d := date.Date()
	parts := strings.Split(slot, ":")
	if len(parts) < 2 {
		return time.Date(y, m, d, 0, 0, 0, 0, loc).UTC()
	}
	hh, _ := strconvAtoi(parts[0])
	mm, _ := strconvAtoi(parts[1])
	ss := 0
	if len(parts) >= 3 {
		ss, _ = strconvAtoi(parts[2])
	}
	return time.Date(y, m, d, hh, mm, ss, 0, loc).UTC()
}

// strconvAtoi is a tiny local helper that swallows errors for the slot
// parser; invalid segments degrade gracefully to zero rather than
// failing the whole pickup write. Kept local to avoid importing
// "strconv" just for one two-line helper.
func strconvAtoi(s string) (int, error) {
	n := 0
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("not a digit: %q", r)
		}
		n = n*10 + int(r-'0')
	}
	if neg {
		n = -n
	}
	return n, nil
}

// --- helpers ---

func (c *DelhiveryCarrier) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Token "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	return c.client.Do(req)
}

func (c *DelhiveryCarrier) doJSONRequest(ctx context.Context, method, path string, payload any) (*http.Response, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return c.doRequest(ctx, method, path, bytes.NewReader(data))
}

func (c *DelhiveryCarrier) doForm(ctx context.Context, path string, form url.Values) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewBufferString(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Token "+c.apiKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.client.Do(req)
}

func MapDelhiveryStatus(statusType string) string {
	switch statusType {
	case "DL":
		return "delivered"
	case "RT", "UD":
		return "exception"
	default:
		return "in_transit"
	}
}

func parseDelhiveryETD(etd string) int {
	// etd is typically "3-5 days"; extract the upper bound.
	var lo, hi int
	if _, err := fmt.Sscanf(etd, "%d-%d", &lo, &hi); err == nil {
		return hi
	}
	if _, err := fmt.Sscanf(etd, "%d", &lo); err == nil {
		return lo
	}
	return 7 // safe fallback
}
