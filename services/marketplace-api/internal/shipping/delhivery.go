package shipping

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

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

type dlCreateResponse struct {
	Success       bool   `json:"success"`
	Packages      []struct {
		Waybill   string `json:"waybill"`
		Status    string `json:"status"`
		Remarks   string `json:"remarks"`
	} `json:"packages"`
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
	if !cr.Success || len(cr.Packages) == 0 {
		remarks := ""
		if len(cr.Packages) > 0 {
			remarks = cr.Packages[0].Remarks
		}
		if remarks == "" {
			remarks = truncate(string(bodyBytes), 400)
		}
		// Previously, test mode stubbed a fake waybill here so the rest
		// of the journey (admin UI, customer timeline) could be
		// exercised end-to-end. That stub actively masked real
		// integration failures: the Delhivery dashboard stayed at zero
		// orders while the admin UI reported success. We always bubble
		// the carrier error now so missing warehouse registration /
		// invalid token issues are visible and actionable.
		return nil, classifyDelhiveryCreateError(remarks, in.FromAddress.Name)
	}

	pkg := cr.Packages[0]
	return &Shipment{
		ProviderShipmentID: pkg.Waybill,
		TrackingNumber:     pkg.Waybill,
		Carrier:            "delhivery",
		Service:            "standard",
	}, nil
}

// classifyDelhiveryCreateError turns Delhivery's opaque remarks string
// into an actionable error. The two most common failures in practice:
//
//  1. "ClientWarehouse matching query does not exist" — the warehouse
//     name we sent isn't registered on the merchant's one.delhivery.com
//     dashboard. Remediation is a one-time operator step on the
//     Delhivery side.
//  2. "Invalid token" — the API key is wrong or the caller sent the
//     ciphertext through without decrypting it.
func classifyDelhiveryCreateError(remarks, warehouseName string) error {
	lower := strings.ToLower(remarks)
	switch {
	case strings.Contains(lower, "clientwarehouse") && strings.Contains(lower, "does not exist"):
		return fmt.Errorf(
			"delhivery: warehouse %q is not registered on your Delhivery account — open one.delhivery.com → Settings → Warehouses and add a warehouse with this exact name, then retry. (carrier: %s)",
			warehouseName, remarks)
	case strings.Contains(lower, "invalid token") || strings.Contains(lower, "unauthorized"):
		return fmt.Errorf(
			"delhivery: API token rejected — verify the token in Settings → Shipping matches the one on one.delhivery.com → Settings → API. (carrier: %s)",
			remarks)
	default:
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
		Status:         mapDelhiveryStatus(shipment.Status.StatusType),
		Events:         events,
	}, nil
}

// FetchLabel pulls the packing-slip PDF for an already-created waybill.
// Delhivery's /api/p/packing_slip?wbns=<waybill>&pdf=true responds with
// a binary PDF when the Authorization header is valid; the body ends up
// as a small HTML / JSON error page otherwise. The caller is
// responsible for streaming the returned bytes to the end user (or
// attaching them to an email) — we don't persist them.
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
	// Delhivery sometimes returns 200 with a JSON error envelope instead
	// of a PDF when the waybill isn't yet manifested. If the response
	// doesn't smell like a PDF, surface the body so the operator can see
	// what happened instead of downloading a broken file.
	if !strings.HasPrefix(strings.ToLower(contentType), "application/pdf") &&
		!bytes.HasPrefix(body, []byte("%PDF")) {
		return nil, "", fmt.Errorf("delhivery: fetch label: not a PDF (content-type=%q body=%s)",
			contentType, truncate(string(body), 400))
	}
	if contentType == "" {
		contentType = "application/pdf"
	}
	return body, contentType, nil
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

func mapDelhiveryStatus(statusType string) string {
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
