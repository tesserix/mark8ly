// Package shipping defines the shipping carrier provider abstraction.
// Concrete implementations (ShipEngine, Delhivery, NinjaVan) live in
// separate files and are wired in P3.
package shipping

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

// Carrier is the interface every shipping provider must implement.
type Carrier interface {
	GetRates(ctx context.Context, in RateRequest) ([]Rate, error)
	CreateShipment(ctx context.Context, in ShipmentRequest) (*Shipment, error)
	GetTracking(ctx context.Context, trackingNumber string) (*Tracking, error)
	CancelShipment(ctx context.Context, shipmentID string) error
	ProviderName() string
	SupportedCountries() []string
}

// LabelFetcher is implemented by carriers whose shipping label must be
// pulled separately (behind the same API token that signs create/track
// calls) instead of being returned inline by CreateShipment. Delhivery
// is the canonical example — its packing-slip endpoint returns a PDF
// that the browser cannot fetch directly because it requires the secret
// token as an Authorization header. The admin API proxies through a
// handler that type-asserts this interface.
type LabelFetcher interface {
	FetchLabel(ctx context.Context, trackingNumber string) (pdf []byte, contentType string, err error)
}

// Address represents a physical address for shipping origin/destination.
type Address struct {
	Name        string
	Line1       string
	Line2       string
	City        string
	Region      string
	PostalCode  string
	CountryCode string
	Phone       string
}

// ParcelItem describes a single item in the shipment for rate calculation.
type ParcelItem struct {
	Title       string
	SKU         string
	Quantity    int
	WeightGrams int
}

// RateRequest is sent to the carrier to get available shipping rates.
type RateRequest struct {
	FromAddress  Address
	ToAddress    Address
	Items        []ParcelItem
	CurrencyCode string
}

// Rate is a single shipping rate option returned by the carrier.
type Rate struct {
	Service       string
	Carrier       string
	Price         decimal.Decimal
	CurrencyCode  string
	EstimatedDays int
}

// ShipmentRequest creates a shipment with a selected service.
type ShipmentRequest struct {
	OrderID      string
	FromAddress  Address
	ToAddress    Address
	Items        []ParcelItem
	Service      string
	CurrencyCode string
}

// Shipment is the carrier's response after creating a shipment.
type Shipment struct {
	ProviderShipmentID string
	TrackingNumber     string
	LabelURL           string
	Carrier            string
	Service            string
	EstimatedDelivery  *time.Time
}

// Tracking represents the current tracking state of a shipment.
type Tracking struct {
	TrackingNumber    string
	Status            string // "in_transit", "delivered", "exception"
	Events            []TrackingEvent
	EstimatedDelivery *time.Time
}

// TrackingEvent is a single event in the shipment's tracking history.
type TrackingEvent struct {
	Status      string
	Description string
	Location    string
	Timestamp   time.Time
}
