package shipping

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

func (c *ShipEngineCarrier) SupportedCountries() []string {
	return []string{"US", "CA", "GB", "DE", "FR", "IT", "ES", "NL", "AU"}
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
	CarrierIDs []string `json:"carrier_ids,omitempty"`
}

type seShipment struct {
	ShipFrom seAddress   `json:"ship_from"`
	ShipTo   seAddress   `json:"ship_to"`
	Packages []sePackage `json:"packages"`
}

type seRateResponse struct {
	RateResponse struct {
		Rates []seRate `json:"rates"`
	} `json:"rate_response"`
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

	body := seRateRequest{
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
		return nil, fmt.Errorf("shipengine: get rates: unexpected status %d", resp.StatusCode)
	}

	var rr seRateResponse
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		return nil, fmt.Errorf("shipengine: get rates: decode: %w", err)
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
