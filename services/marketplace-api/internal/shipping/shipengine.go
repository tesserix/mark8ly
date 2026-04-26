package shipping

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const (
	shipEngineLiveURL    = "https://api.shipengine.com"
	shipEngineSandboxURL = "https://api.shipengine.com" // ShipEngine uses same host; sandbox mode is per-key
)

// ShipEngineCarrier implements Carrier for the ShipEngine REST API v1.
type ShipEngineCarrier struct {
	apiKey  string
	mode    string
	baseURL string
	client  *http.Client
}

// NewShipEngineCarrier constructs a ShipEngine carrier instance.
func NewShipEngineCarrier(apiKey, mode string) *ShipEngineCarrier {
	base := shipEngineLiveURL
	if mode == "test" {
		base = shipEngineSandboxURL
	}
	return &ShipEngineCarrier{
		apiKey:  apiKey,
		mode:    mode,
		baseURL: base,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *ShipEngineCarrier) ProviderName() string { return "shipengine" }

// SupportedCountries lists the ISO 3166-1 alpha-2 country codes routed through
// ShipEngine. IE + NZ added per P18 (§4.1). NZ addresses lack a state/province
// field — ShipEngine accepts the empty state_province on NZ requests, and our
// code has no pre-send state validation to relax.
//
// TODO(v2): Aramex AE integration — deferred per spec §2 last row and §25
// effort table ("AE / Aramex (deferred v2) — 1 week"). Do not add "AE" here.
func (c *ShipEngineCarrier) SupportedCountries() []string {
	return []string{"AU", "CA", "DE", "ES", "FR", "GB", "IE", "IT", "NL", "NZ", "US"}
}

// --- request / response types (private, match ShipEngine JSON schema) ---

type seAddress struct {
	Name        string `json:"name"`
	AddressLine1 string `json:"address_line1"`
	AddressLine2 string `json:"address_line2,omitempty"`
	CityLocality string `json:"city_locality"`
	StateProvince string `json:"state_province"`
	PostalCode   string `json:"postal_code"`
	CountryCode  string `json:"country_code"`
	Phone        string `json:"phone,omitempty"`
}

type sePackage struct {
	Weight seWeight `json:"weight"`
}

type seWeight struct {
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

type seRateRequest struct {
	RateOptions seRateOptions `json:"rate_options"`
	Shipment    seShipment    `json:"shipment"`
}

type seRateOptions struct {
	// CarrierIDs is REQUIRED by ShipEngine /v1/rates — without it the
	// server responds 400 (`carrier_ids is required`). We auto-fetch
	// from /v1/carriers (the merchant's connected carriers) so the
	// admin form doesn't need a per-carrier picker.
	CarrierIDs []string `json:"carrier_ids"`
}

type seCarriersResponse struct {
	Carriers []struct {
		CarrierID   string `json:"carrier_id"`
		CarrierCode string `json:"carrier_code"`
		Disabled    bool   `json:"disabled_by_billing_plan"`
	} `json:"carriers"`
}

type seShipment struct {
	ShipFrom seAddress   `json:"ship_from"`
	ShipTo   seAddress   `json:"ship_to"`
	Packages []sePackage `json:"packages"`
}

type seRateResponse struct {
	RateResponse struct {
		Rates        []seRate    `json:"rates"`
		InvalidRates []seRate    `json:"invalid_rates"`
		Errors       []seRateErr `json:"errors"`
		Status       string      `json:"status"`
	} `json:"rate_response"`
}

type seRateErr struct {
	ErrorSource string `json:"error_source"`
	ErrorType   string `json:"error_type"`
	ErrorCode   string `json:"error_code"`
	Message     string `json:"message"`
	CarrierID   string `json:"carrier_id,omitempty"`
}

type seRate struct {
	ServiceCode   string `json:"service_code"`
	CarrierCode   string `json:"carrier_code"`
	ShippingAmount struct {
		Currency string  `json:"currency"`
		Amount   float64 `json:"amount"`
	} `json:"shipping_amount"`
	DeliveryDays int `json:"delivery_days"`
}

type seLabelRequest struct {
	Shipment    seShipment `json:"shipment"`
	ServiceCode string     `json:"service_code"`
}

type seLabelResponse struct {
	LabelID        string `json:"label_id"`
	TrackingNumber string `json:"tracking_number"`
	LabelDownload  struct {
		PDF string `json:"pdf"`
	} `json:"label_download"`
	CarrierCode string `json:"carrier_code"`
	ServiceCode string `json:"service_code"`
}

type seTrackingResponse struct {
	TrackingNumber string    `json:"tracking_number"`
	StatusCode     string    `json:"status_code"`
	Events         []seEvent `json:"events"`
}

type seEvent struct {
	Description    string    `json:"description"`
	CityLocality   string    `json:"city_locality"`
	StateProvince  string    `json:"state_province"`
	OccurredAt     time.Time `json:"occurred_at"`
	StatusCode     string    `json:"status_code"`
}

// --- Carrier interface methods ---

func (c *ShipEngineCarrier) GetRates(ctx context.Context, in RateRequest) ([]Rate, error) {
	totalWeightGrams := 0
	for _, item := range in.Items {
		totalWeightGrams += item.WeightGrams * item.Quantity
	}

	carrierIDs, err := c.fetchCarrierIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("shipengine: get rates: %w", err)
	}
	if len(carrierIDs) == 0 {
		return nil, fmt.Errorf(
			"shipengine: get rates: ShipEngine account has no enabled carriers — connect at least one carrier (UPS / USPS / FedEx / sandbox carriers) in the ShipEngine dashboard before quoting rates",
		)
	}

	body := seRateRequest{
		RateOptions: seRateOptions{CarrierIDs: carrierIDs},
		Shipment: seShipment{
			ShipFrom: toSEAddress(in.FromAddress),
			ShipTo:   toSEAddress(in.ToAddress),
			Packages: []sePackage{
				{Weight: seWeight{Value: float64(totalWeightGrams) / 1000.0, Unit: "kilogram"}},
			},
		},
	}

	resp, err := c.doJSON(ctx, http.MethodPost, "/v1/rates", body)
	if err != nil {
		return nil, fmt.Errorf("shipengine: get rates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"shipengine: get rates: unexpected status %d: %s",
			resp.StatusCode, readBodyTrimmed(resp.Body),
		)
	}

	var rr seRateResponse
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		return nil, fmt.Errorf("shipengine: get rates: decode: %w", err)
	}

	// ShipEngine returns 200 with `rate_response.errors` when individual
	// carriers reject the route (origin not serviced, weight out of
	// range, address invalid, etc.). Surface those as a real error
	// when there are zero usable rates so the storefront can render a
	// clear "carrier doesn't ship this route" instead of the generic
	// "couldn't get a live rate".
	if len(rr.RateResponse.Rates) == 0 {
		if len(rr.RateResponse.Errors) > 0 {
			msgs := make([]string, 0, len(rr.RateResponse.Errors))
			for _, e := range rr.RateResponse.Errors {
				m := strings.TrimSpace(e.Message)
				if m == "" {
					m = e.ErrorCode
				}
				if m != "" {
					msgs = append(msgs, m)
				}
			}
			return nil, fmt.Errorf(
				"shipengine: get rates: no rates returned (status=%s, %d carrier(s) tried): %s",
				rr.RateResponse.Status, len(carrierIDs), strings.Join(msgs, "; "),
			)
		}
		return nil, fmt.Errorf(
			"shipengine: get rates: no rates returned and no errors reported — "+
				"likely the connected carriers do not service this origin/destination "+
				"(origin=%s, dest=%s, %d carrier(s) tried, status=%s)",
			in.FromAddress.CountryCode, in.ToAddress.CountryCode,
			len(carrierIDs), rr.RateResponse.Status,
		)
	}

	rates := make([]Rate, 0, len(rr.RateResponse.Rates))
	for _, r := range rr.RateResponse.Rates {
		rates = append(rates, Rate{
			Service:       r.ServiceCode,
			Carrier:       r.CarrierCode,
			Price:         decimal.NewFromFloat(r.ShippingAmount.Amount),
			CurrencyCode:  r.ShippingAmount.Currency,
			EstimatedDays: r.DeliveryDays,
		})
	}
	return rates, nil
}

func (c *ShipEngineCarrier) CreateShipment(ctx context.Context, in ShipmentRequest) (*Shipment, error) {
	totalWeightGrams := 0
	for _, item := range in.Items {
		totalWeightGrams += item.WeightGrams * item.Quantity
	}

	body := seLabelRequest{
		Shipment: seShipment{
			ShipFrom: toSEAddress(in.FromAddress),
			ShipTo:   toSEAddress(in.ToAddress),
			Packages: []sePackage{
				{Weight: seWeight{Value: float64(totalWeightGrams) / 1000.0, Unit: "kilogram"}},
			},
		},
		ServiceCode: in.Service,
	}

	resp, err := c.doJSON(ctx, http.MethodPost, "/v1/labels", body)
	if err != nil {
		return nil, fmt.Errorf("shipengine: create shipment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("shipengine: create shipment: unexpected status %d", resp.StatusCode)
	}

	var lr seLabelResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return nil, fmt.Errorf("shipengine: create shipment: decode: %w", err)
	}

	return &Shipment{
		ProviderShipmentID: lr.LabelID,
		TrackingNumber:     lr.TrackingNumber,
		LabelURL:           lr.LabelDownload.PDF,
		Carrier:            lr.CarrierCode,
		Service:            lr.ServiceCode,
	}, nil
}

func (c *ShipEngineCarrier) GetTracking(ctx context.Context, trackingNumber string) (*Tracking, error) {
	path := fmt.Sprintf("/v1/tracking?tracking_number=%s", trackingNumber)

	resp, err := c.doJSON(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("shipengine: get tracking: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("shipengine: get tracking: unexpected status %d", resp.StatusCode)
	}

	var tr seTrackingResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, fmt.Errorf("shipengine: get tracking: decode: %w", err)
	}

	events := make([]TrackingEvent, 0, len(tr.Events))
	for _, e := range tr.Events {
		location := e.CityLocality
		if e.StateProvince != "" {
			location += ", " + e.StateProvince
		}
		events = append(events, TrackingEvent{
			Status:      e.StatusCode,
			Description: e.Description,
			Location:    location,
			Timestamp:   e.OccurredAt,
		})
	}

	return &Tracking{
		TrackingNumber: tr.TrackingNumber,
		Status:         mapSEStatus(tr.StatusCode),
		Events:         events,
	}, nil
}

func (c *ShipEngineCarrier) CancelShipment(ctx context.Context, shipmentID string) error {
	path := fmt.Sprintf("/v1/labels/%s/void", shipmentID)

	resp, err := c.doJSON(ctx, http.MethodPut, path, nil)
	if err != nil {
		return fmt.Errorf("shipengine: cancel shipment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("shipengine: cancel shipment: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// --- helpers ---

// fetchCarrierIDs lists the carriers connected to the merchant's
// ShipEngine account. ShipEngine /v1/rates rejects requests without
// carrier_ids, so we have to ask the API which carriers to quote.
// Filters out carriers disabled by billing plan.
func (c *ShipEngineCarrier) fetchCarrierIDs(ctx context.Context) ([]string, error) {
	resp, err := c.doJSON(ctx, http.MethodGet, "/v1/carriers", nil)
	if err != nil {
		return nil, fmt.Errorf("list carriers: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"list carriers: unexpected status %d: %s",
			resp.StatusCode, readBodyTrimmed(resp.Body),
		)
	}

	var body seCarriersResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("list carriers: decode: %w", err)
	}

	ids := make([]string, 0, len(body.Carriers))
	for _, ca := range body.Carriers {
		if ca.Disabled || ca.CarrierID == "" {
			continue
		}
		ids = append(ids, ca.CarrierID)
	}
	return ids, nil
}

// readBodyTrimmed reads up to 1 KiB of the response body and returns
// it as a string. Used in error messages so an unexpected ShipEngine
// status code surfaces ShipEngine's own error explanation instead of a
// bare "unexpected status 400".
func readBodyTrimmed(r io.Reader) string {
	const cap = 1024
	b, err := io.ReadAll(io.LimitReader(r, cap))
	if err != nil {
		return ""
	}
	return string(bytes.TrimSpace(b))
}

func (c *ShipEngineCarrier) doJSON(ctx context.Context, method, path string, payload any) (*http.Response, error) {
	var bodyReader io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("API-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	return c.client.Do(req)
}

func toSEAddress(a Address) seAddress {
	return seAddress{
		Name:          a.Name,
		AddressLine1:  a.Line1,
		AddressLine2:  a.Line2,
		CityLocality:  a.City,
		StateProvince: a.Region,
		PostalCode:    a.PostalCode,
		CountryCode:   a.CountryCode,
		Phone:         a.Phone,
	}
}

func mapSEStatus(code string) string {
	switch code {
	case "DE":
		return "delivered"
	case "EX":
		return "exception"
	default:
		return "in_transit"
	}
}
