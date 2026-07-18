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
	Name          string `json:"name"`
	AddressLine1  string `json:"address_line1"`
	AddressLine2  string `json:"address_line2,omitempty"`
	CityLocality  string `json:"city_locality"`
	StateProvince string `json:"state_province"`
	PostalCode    string `json:"postal_code"`
	CountryCode   string `json:"country_code"`
	Phone         string `json:"phone,omitempty"`
	// Email — required by CouriersPlease (and parts of Aramex). Empty
	// string is fine for carriers that don't validate it; ShipEngine
	// drops it from the wire when the field is omitted via omitempty.
	Email string `json:"email,omitempty"`
}

type sePackage struct {
	Weight     seWeight     `json:"weight"`
	Dimensions seDimensions `json:"dimensions"`
}

type seWeight struct {
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

// seDimensions is required by most carriers ShipEngine fronts —
// Australia Post returns 400 / VAL_DIMENSION_MAX without it. We
// default to a reasonable parcel envelope (30 × 20 × 10 cm) until
// per-variant dimensions land on the product model. The defaults
// are well within every carrier's max (≤180 cm / 200 cm).
type seDimensions struct {
	Unit   string  `json:"unit"`
	Length float64 `json:"length"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Default rate-quote package dimensions in centimeters. Used until
// the product model carries per-variant dimensions.
const (
	defaultLengthCM = 30.0
	defaultWidthCM  = 20.0
	defaultHeightCM = 10.0
)

func defaultPackageDimensions() seDimensions {
	return seDimensions{
		Unit:   "centimeter",
		Length: defaultLengthCM,
		Width:  defaultWidthCM,
		Height: defaultHeightCM,
	}
}

// resolvePackageDimensions picks the largest-of-each-axis across items
// that carry per-variant dims, falling back to the default envelope
// when nothing is set. Single-package shipment for now — multi-package
// optimisation is a future enhancement.
func resolvePackageDimensions(items []ParcelItem) seDimensions {
	d := seDimensions{Unit: "centimeter"}
	for _, it := range items {
		if it.LengthCM > d.Length {
			d.Length = it.LengthCM
		}
		if it.WidthCM > d.Width {
			d.Width = it.WidthCM
		}
		if it.HeightCM > d.Height {
			d.Height = it.HeightCM
		}
	}
	if d.Length == 0 {
		d.Length = defaultLengthCM
	}
	if d.Width == 0 {
		d.Width = defaultWidthCM
	}
	if d.Height == 0 {
		d.Height = defaultHeightCM
	}
	return d
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
	// ServiceCode is REQUIRED on POST /v1/labels and lives INSIDE the
	// shipment object, not at the root of the body. Putting it at the
	// root caused ShipEngine to fail validation with
	// `field_value_required: 'service_code' must not be empty.` even
	// though we did pass a valid code in the request — they just looked
	// for it in the wrong place. Empty on rate-quote bodies (where it
	// is omitted by `omitempty`) so /v1/rates still works.
	ServiceCode string      `json:"service_code,omitempty"`
	// ShipDate is the preferred pickup date in YYYY-MM-DD form. Aramex AU
	// (and most non-postal carriers) reject same-day pickup outside their
	// service window — without an explicit date ShipEngine sends "today"
	// and the carrier replies "preferred pickup date is not available".
	// We always send the next business day to dodge weekends + after-hours.
	// Empty on rate-quote bodies (omitempty) so /v1/rates is unchanged.
	ShipDate string      `json:"ship_date,omitempty"`
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
	Shipment seShipment `json:"shipment"`
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
				{
				Weight:     seWeight{Value: float64(totalWeightGrams) / 1000.0, Unit: "kilogram"},
				Dimensions: resolvePackageDimensions(in.Items),
			},
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
			ServiceCode: in.Service,
			ShipDate:    nextBusinessDay(time.Now().UTC()).Format("2006-01-02"),
			ShipFrom:    toSEAddress(in.FromAddress),
			ShipTo:      toSEAddress(in.ToAddress),
			Packages: []sePackage{
				{
					Weight:     seWeight{Value: float64(totalWeightGrams) / 1000.0, Unit: "kilogram"},
					Dimensions: resolvePackageDimensions(in.Items),
				},
			},
		},
	}

	resp, err := c.doJSON(ctx, http.MethodPost, "/v1/labels", body)
	if err != nil {
		return nil, fmt.Errorf("shipengine: create shipment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Surface the response body in the error so admin staff (and the
		// admin UI's verbose error surface) see ShipEngine's actual
		// rejection reason — bare "unexpected status 400" makes label
		// failures impossible to debug.
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf(
			"shipengine: create shipment: unexpected status %d: %s",
			resp.StatusCode, strings.TrimSpace(string(bodyBytes)),
		)
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

// CancelShipment voids the ShipEngine label for a shipment. Two ShipEngine
// specifics this pins:
//
//  1. Void takes ShipEngine's label_id, NOT the tracking number we persist on
//     the shipment row (the label_id is returned at creation but not stored).
//     So we first resolve the label_id via the List Labels query.
//  2. The void endpoint returns HTTP 200 even when the void is REFUSED (label
//     already used / shipped); the real outcome is in the `approved` flag. An
//     earlier version checked only the status code and voided by the tracking
//     number, so every cancel hit the wrong endpoint and a refused void would
//     have read as success (same class of bug as Delhivery's 200-on-failure).
func (c *ShipEngineCarrier) CancelShipment(ctx context.Context, trackingNumber string) error {
	if strings.TrimSpace(trackingNumber) == "" {
		return fmt.Errorf("shipengine: cancel shipment: tracking number is required")
	}
	labelID, err := c.labelIDForTracking(ctx, trackingNumber)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/v1/labels/%s/void", labelID)
	resp, err := c.doJSON(ctx, http.MethodPut, path, nil)
	if err != nil {
		return fmt.Errorf("shipengine: cancel shipment: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("shipengine: cancel shipment: %s", shipEngineErrorMessage(body, resp.StatusCode))
	}

	var vr struct {
		Approved bool   `json:"approved"`
		Message  string `json:"message"`
	}
	if err := json.Unmarshal(body, &vr); err != nil {
		return fmt.Errorf("shipengine: cancel shipment: decode void response: %w", err)
	}
	if !vr.Approved {
		msg := strings.TrimSpace(vr.Message)
		if msg == "" {
			msg = "the carrier refused the void"
		}
		return fmt.Errorf("shipengine: cancel shipment: %s", msg)
	}
	return nil
}

// labelIDForTracking resolves ShipEngine's label_id for a tracking number via
// the List Labels query. Needed because void takes the label_id, which we do
// not persist on the shipment row.
func (c *ShipEngineCarrier) labelIDForTracking(ctx context.Context, trackingNumber string) (string, error) {
	path := "/v1/labels?tracking_number=" + url.QueryEscape(trackingNumber)
	resp, err := c.doJSON(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", fmt.Errorf("shipengine: cancel shipment: resolve label: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("shipengine: cancel shipment: resolve label: unexpected status %d: %s",
			resp.StatusCode, readBodyTrimmed(resp.Body))
	}
	var lr struct {
		Labels []struct {
			LabelID string `json:"label_id"`
		} `json:"labels"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return "", fmt.Errorf("shipengine: cancel shipment: resolve label: decode: %w", err)
	}
	if len(lr.Labels) == 0 || lr.Labels[0].LabelID == "" {
		return "", fmt.Errorf("shipengine: cancel shipment: no label found for tracking number %s", trackingNumber)
	}
	return lr.Labels[0].LabelID, nil
}

// CreateReverseShipment creates a ShipEngine return label for a delivered
// shipment, using the return-from-label endpoint
// (POST /v1/labels/{label_id}/return). ShipEngine reverses the ship_from /
// ship_to addresses and reuses the original carrier + service automatically, so
// we only need the original label — resolved from the forward tracking number.
//
// NOTE: ShipEngine returns are DOMESTIC ONLY and not every carrier/service
// supports them; an unsupported route surfaces as a clean error the executor
// records as `failed` (a manual notice). Gated behind REVERSE_PICKUP_ENABLED at
// the executor.
func (c *ShipEngineCarrier) CreateReverseShipment(ctx context.Context, in ReverseShipmentRequest) (*Shipment, error) {
	if strings.TrimSpace(in.OriginalTrackingNumber) == "" {
		return nil, fmt.Errorf("shipengine: reverse shipment: original tracking number is required")
	}
	labelID, err := c.labelIDForTracking(ctx, in.OriginalTrackingNumber)
	if err != nil {
		return nil, err
	}

	// charge_event carrier_default defers to the carrier's standard billing
	// behaviour, matching how the forward label is charged.
	body := map[string]string{"charge_event": "carrier_default"}
	path := fmt.Sprintf("/v1/labels/%s/return", labelID)
	resp, err := c.doJSON(ctx, http.MethodPost, path, body)
	if err != nil {
		return nil, fmt.Errorf("shipengine: reverse shipment: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("shipengine: reverse shipment: %s", shipEngineErrorMessage(respBody, resp.StatusCode))
	}

	var lr seLabelResponse
	if err := json.Unmarshal(respBody, &lr); err != nil {
		return nil, fmt.Errorf("shipengine: reverse shipment: decode: %w", err)
	}
	if lr.TrackingNumber == "" {
		return nil, fmt.Errorf("shipengine: reverse shipment: no tracking number in return response")
	}
	return &Shipment{
		ProviderShipmentID: lr.LabelID,
		TrackingNumber:     lr.TrackingNumber,
		LabelURL:           lr.LabelDownload.PDF,
		Carrier:            "shipengine",
		Service:            "return",
	}, nil
}

// shipEngineErrorMessage pulls ShipEngine's short error text out of an error
// response body ({"errors":[{"message":...}]}) for a clean merchant-facing
// message, falling back to the status code.
func shipEngineErrorMessage(body []byte, status int) string {
	var e struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if json.Unmarshal(body, &e) == nil && len(e.Errors) > 0 && strings.TrimSpace(e.Errors[0].Message) != "" {
		return e.Errors[0].Message
	}
	return fmt.Sprintf("unexpected status %d", status)
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

// nextBusinessDay returns the next Mon–Fri date strictly *after or
// equal to* today. Saturday → +2 days (Mon), Sunday → +1 day (Mon),
// any weekday → unchanged. Saves us from Aramex / FedEx / UPS rejecting
// "preferred pickup date is not available" on weekend label-creation.
// Conservatively uses UTC — close enough for "what's a weekday" since
// no timezone shifts the day-of-week by more than 24h.
func nextBusinessDay(t time.Time) time.Time {
	switch t.Weekday() {
	case time.Saturday:
		return t.AddDate(0, 0, 2)
	case time.Sunday:
		return t.AddDate(0, 0, 1)
	default:
		return t
	}
}

func toSEAddress(a Address) seAddress {
	city := a.City
	region := a.Region
	country := strings.ToUpper(strings.TrimSpace(a.CountryCode))
	// Australia Post (and a few other ShipEngine carriers) match the
	// city against an internal suburb table that's case-sensitive on
	// AU addresses — "Sydney" returns PSC_PC_NF, "SYDNEY" works.
	// Cheap-but-correct normalisation: uppercase the city for AU.
	if country == "AU" {
		city = strings.ToUpper(strings.TrimSpace(city))
		region = strings.ToUpper(strings.TrimSpace(region))
	}
	return seAddress{
		Name:          ensureFirstLastName(a.Name),
		AddressLine1:  a.Line1,
		AddressLine2:  a.Line2,
		CityLocality:  city,
		StateProvince: region,
		PostalCode:    a.PostalCode,
		CountryCode:   country,
		Phone:         a.Phone,
		Email:         a.Email,
	}
}

// ensureFirstLastName guarantees the name string has at least two
// whitespace-separated tokens. CouriersPlease (and a few other
// ShipEngine carriers) require a separate "last name" and reject the
// label with "Please enter value contact/pickup/destination last name"
// when only one token is present. We duplicate the single token so the
// printed label still reads the original name, just twice — better
// than dead-ending the merchant with a label that won't generate.
//
// Doesn't touch already-multi-token names. The proper fix is to
// require first+last name at checkout/warehouse-settings input time;
// this padding is the defensive last line so existing data still works.
func ensureFirstLastName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, " ") {
		return trimmed
	}
	return trimmed + " " + trimmed
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
