package shipping

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

const (
	ninjaVanLiveURL    = "https://api.ninjavan.co"
	ninjaVanSandboxURL = "https://api-sandbox.ninjavan.co"
)

// NinjaVanCarrier implements Carrier for the NinjaVan API v2 (Southeast Asia).
type NinjaVanCarrier struct {
	apiKey    string
	secretKey string
	mode      string
	baseURL   string
	country   string // ISO 3166-1 alpha-2, lowercase (e.g. "sg", "my")
	client    *http.Client

	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time
}

// NewNinjaVanCarrier constructs a NinjaVan carrier instance. countryCode is the
// ISO 3166-1 alpha-2 code for the NinjaVan country endpoint (e.g. "SG", "MY").
func NewNinjaVanCarrier(apiKey, secretKey, mode, countryCode string) *NinjaVanCarrier {
	base := ninjaVanLiveURL
	if mode == "test" {
		base = ninjaVanSandboxURL
	}
	cc := strings.ToLower(countryCode)
	if cc == "" {
		cc = "sg"
	}
	return &NinjaVanCarrier{
		apiKey:    apiKey,
		secretKey: secretKey,
		mode:      mode,
		baseURL:   base,
		country:   cc,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *NinjaVanCarrier) ProviderName() string { return "ninjavan" }

// SupportedCountries lists the ISO 3166-1 alpha-2 country codes routed through
// NinjaVan. VN added per P18 (§4.1.1).
//
// TODO(v2): Aramex AE is NOT a NinjaVan country. NinjaVan serves SEA only.
// UAE coverage is deferred to v2 via the Aramex integration — see spec §2.
func (c *NinjaVanCarrier) SupportedCountries() []string {
	return []string{"ID", "MY", "PH", "SG", "TH", "VN"}
}

// --- request / response types (private, match NinjaVan JSON schema) ---

type nvAuthRequest struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	GrantType    string `json:"grant_type"`
}

type nvAuthResponse struct {
	AccessToken string `json:"access_token"`
}

type nvAddress struct {
	Name    string `json:"name"`
	Address struct {
		Address1   string `json:"address1"`
		Address2   string `json:"address2,omitempty"`
		City       string `json:"city"`
		State      string `json:"state"`
		PostalCode string `json:"postcode"`
		Country    string `json:"country"`
	} `json:"address"`
	Contact struct {
		Phone string `json:"phone_number"`
	} `json:"contact"`
}

type nvOrderRequest struct {
	ServiceType    string    `json:"service_type"`
	ServiceLevel   string    `json:"service_level"`
	From           nvAddress `json:"from"`
	To             nvAddress `json:"to"`
	ParcelJob      nvParcel  `json:"parcel_job"`
	RequestedTrackingNumber string `json:"requested_tracking_number,omitempty"`
}

type nvParcel struct {
	Dimensions nvDimensions `json:"dimensions"`
	Items      []nvItem     `json:"items"`
}

type nvDimensions struct {
	Weight float64 `json:"weight"` // kg
}

type nvItem struct {
	Description string `json:"item_description"`
	Quantity    int    `json:"quantity"`
}

type nvOrderResponse struct {
	TrackingNumber string `json:"tracking_number"`
	OrderID        string `json:"id"`
}

type nvTrackingResponse struct {
	TrackingNumber string    `json:"tracking_id"`
	Status         string    `json:"status"`
	Events         []nvEvent `json:"events"`
}

type nvEvent struct {
	Status    string `json:"status"`
	Comment   string `json:"comment"`
	Location  string `json:"address"`
	Timestamp string `json:"time"`
}

type nvRateResponse struct {
	Data struct {
		ServiceTypes []nvServiceRate `json:"service_types"`
	} `json:"data"`
}

type nvServiceRate struct {
	ServiceType  string  `json:"service_type"`
	ServiceLevel string  `json:"service_level"`
	Price        float64 `json:"price"`
	Currency     string  `json:"currency"`
	DeliveryDays int     `json:"delivery_days"`
}

// --- Carrier interface methods ---

func (c *NinjaVanCarrier) GetRates(ctx context.Context, in RateRequest) ([]Rate, error) {
	totalWeightGrams := 0
	for _, item := range in.Items {
		totalWeightGrams += item.WeightGrams * item.Quantity
	}

	token, err := c.authenticate(ctx)
	if err != nil {
		return nil, fmt.Errorf("ninjavan: get rates: auth: %w", err)
	}

	country := strings.ToLower(in.FromAddress.CountryCode)

	body := map[string]any{
		"from": map[string]string{
			"postcode": in.FromAddress.PostalCode,
			"country":  in.FromAddress.CountryCode,
		},
		"to": map[string]string{
			"postcode": in.ToAddress.PostalCode,
			"country":  in.ToAddress.CountryCode,
		},
		"weight": float64(totalWeightGrams) / 1000.0,
	}

	path := fmt.Sprintf("/%s/4.2/rates", country)
	resp, err := c.doJSON(ctx, http.MethodPost, path, body, token)
	if err != nil {
		return nil, fmt.Errorf("ninjavan: get rates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ninjavan: get rates: unexpected status %d", resp.StatusCode)
	}

	var rr nvRateResponse
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		return nil, fmt.Errorf("ninjavan: get rates: decode: %w", err)
	}

	rates := make([]Rate, 0, len(rr.Data.ServiceTypes))
	for _, sr := range rr.Data.ServiceTypes {
		rates = append(rates, Rate{
			Service:       sr.ServiceType + "_" + sr.ServiceLevel,
			Carrier:       "ninjavan",
			Price:         decimal.NewFromFloat(sr.Price),
			CurrencyCode:  sr.Currency,
			EstimatedDays: sr.DeliveryDays,
		})
	}
	return rates, nil
}

func (c *NinjaVanCarrier) CreateShipment(ctx context.Context, in ShipmentRequest) (*Shipment, error) {
	totalWeightGrams := 0
	for _, item := range in.Items {
		totalWeightGrams += item.WeightGrams * item.Quantity
	}

	token, err := c.authenticate(ctx)
	if err != nil {
		return nil, fmt.Errorf("ninjavan: create shipment: auth: %w", err)
	}

	country := strings.ToLower(in.FromAddress.CountryCode)

	items := make([]nvItem, 0, len(in.Items))
	for _, item := range in.Items {
		items = append(items, nvItem{
			Description: item.Title,
			Quantity:    item.Quantity,
		})
	}

	order := nvOrderRequest{
		ServiceType:  "Parcel",
		ServiceLevel: "Standard",
		From:         toNVAddress(in.FromAddress),
		To:           toNVAddress(in.ToAddress),
		ParcelJob: nvParcel{
			Dimensions: nvDimensions{Weight: float64(totalWeightGrams) / 1000.0},
			Items:      items,
		},
	}

	// Override service level from the selected service if provided.
	if in.Service != "" {
		parts := strings.SplitN(in.Service, "_", 2)
		if len(parts) == 2 {
			order.ServiceType = parts[0]
			order.ServiceLevel = parts[1]
		}
	}

	path := fmt.Sprintf("/%s/4.2/orders", country)
	resp, err := c.doJSON(ctx, http.MethodPost, path, order, token)
	if err != nil {
		return nil, fmt.Errorf("ninjavan: create shipment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("ninjavan: create shipment: unexpected status %d", resp.StatusCode)
	}

	var or nvOrderResponse
	if err := json.NewDecoder(resp.Body).Decode(&or); err != nil {
		return nil, fmt.Errorf("ninjavan: create shipment: decode: %w", err)
	}

	return &Shipment{
		ProviderShipmentID: or.OrderID,
		TrackingNumber:     or.TrackingNumber,
		Carrier:            "ninjavan",
		Service:            order.ServiceType + "_" + order.ServiceLevel,
	}, nil
}

func (c *NinjaVanCarrier) GetTracking(ctx context.Context, trackingNumber string) (*Tracking, error) {
	token, err := c.authenticate(ctx)
	if err != nil {
		return nil, fmt.Errorf("ninjavan: get tracking: auth: %w", err)
	}

	path := fmt.Sprintf("/%s/2.0/orders/tracking/%s", c.country, trackingNumber)
	resp, err := c.doJSON(ctx, http.MethodGet, path, nil, token)
	if err != nil {
		return nil, fmt.Errorf("ninjavan: get tracking: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ninjavan: get tracking: unexpected status %d", resp.StatusCode)
	}

	var tr nvTrackingResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, fmt.Errorf("ninjavan: get tracking: decode: %w", err)
	}

	events := make([]TrackingEvent, 0, len(tr.Events))
	for _, e := range tr.Events {
		ts, _ := time.Parse(time.RFC3339, e.Timestamp)
		events = append(events, TrackingEvent{
			Status:      e.Status,
			Description: e.Comment,
			Location:    e.Location,
			Timestamp:   ts,
		})
	}

	return &Tracking{
		TrackingNumber: tr.TrackingNumber,
		Status:         mapNVStatus(tr.Status),
		Events:         events,
	}, nil
}

func (c *NinjaVanCarrier) CancelShipment(ctx context.Context, shipmentID string) error {
	token, err := c.authenticate(ctx)
	if err != nil {
		return fmt.Errorf("ninjavan: cancel shipment: auth: %w", err)
	}

	path := fmt.Sprintf("/%s/2.0/orders/%s", c.country, shipmentID)
	resp, err := c.doJSON(ctx, http.MethodDelete, path, nil, token)
	if err != nil {
		return fmt.Errorf("ninjavan: cancel shipment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("ninjavan: cancel shipment: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// --- helpers ---

func (c *NinjaVanCarrier) authenticate(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Return cached token if still valid.
	if c.accessToken != "" && time.Now().Before(c.tokenExpiry) {
		return c.accessToken, nil
	}

	body := nvAuthRequest{
		ClientID:     c.apiKey,
		ClientSecret: c.secretKey,
		GrantType:    "client_credentials",
	}

	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	path := fmt.Sprintf("/%s/2.0/oauth/access_token", c.country)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("auth returned status %d", resp.StatusCode)
	}

	var ar nvAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		return "", err
	}

	c.accessToken = ar.AccessToken
	// NinjaVan tokens typically last 24h; expire 5 minutes early.
	c.tokenExpiry = time.Now().Add(23*time.Hour + 55*time.Minute)

	return c.accessToken, nil
}

func (c *NinjaVanCarrier) doJSON(ctx context.Context, method, path string, payload any, token string) (*http.Response, error) {
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
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	return c.client.Do(req)
}

func toNVAddress(a Address) nvAddress {
	nv := nvAddress{Name: a.Name}
	nv.Address.Address1 = a.Line1
	nv.Address.Address2 = a.Line2
	nv.Address.City = a.City
	nv.Address.State = a.Region
	nv.Address.PostalCode = a.PostalCode
	nv.Address.Country = a.CountryCode
	nv.Contact.Phone = a.Phone
	return nv
}

func mapNVStatus(status string) string {
	switch strings.ToLower(status) {
	case "completed", "delivered":
		return "delivered"
	case "cancelled", "returned", "lost":
		return "exception"
	default:
		return "in_transit"
	}
}
