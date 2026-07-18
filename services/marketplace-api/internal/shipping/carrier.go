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

// Warehouse is the carrier-side representation of a pickup location.
// Populated by the admin settings handler from the warehouse_* columns
// on shipping_carrier_configs and pushed to the carrier via
// WarehouseSyncer so merchants don't have to mirror the address on two
// sides.
type Warehouse struct {
	Name        string
	Email       string
	Phone       string
	Address     string
	City        string
	PinCode     string
	CountryCode string
	Region      string
	// ContactPerson and RegisteredName are Delhivery clientwarehouse
	// requirements. Optional here — carriers that don't need them ignore
	// them, and Delhivery defaults both to Name when empty.
	ContactPerson  string
	RegisteredName string
}

// WarehouseSyncer is implemented by carriers that can accept an upsert
// of the merchant's pickup / warehouse address. Delhivery is the first
// implementor: its /clientwarehouse/edit/ + /clientwarehouse/create/
// endpoints let us keep one.delhivery.com's Pickup Locations in lockstep
// with the admin's Shipping settings, so a fresh merchant doesn't have
// to hand-enter the same warehouse in two places.
type WarehouseSyncer interface {
	UpsertWarehouse(ctx context.Context, wh Warehouse) error
}

// PickupScheduler is implemented by carriers that support requesting
// a pickup-agent dispatch for a pickup location + date + time slot.
// Without this call Delhivery leaves the waybill at "Manifested" state
// and the merchant has to click "Add to Pickup" on one.delhivery.com
// before a pickup executive is dispatched — which is exactly the
// friction this interface is designed to eliminate.
type PickupScheduler interface {
	SchedulePickup(ctx context.Context, req PickupRequest) (*Pickup, error)
}

// ReturnToOriginer is implemented by carriers that can proactively return an
// already-picked-up / in-transit shipment to its origin (RTO). Delhivery is
// the first implementor: it has no distinct RTO endpoint — cancelling an
// in-transit prepaid/COD shipment via /api/p/edit moves it to "Returned"
// (RTO to the client warehouse). Carriers that don't implement this record an
// "unsupported" outcome so the merchant arranges the return manually.
type ReturnToOriginer interface {
	ReturnToOrigin(ctx context.Context, trackingNumber string) error
}

// PickupRequest is the minimal input for SchedulePickup. Delhivery is
// the pinning implementation: the carrier only cares about the date
// (YYYY-MM-DD) and a start-of-slot time; any time-of-day on Date is
// ignored and TimeStart takes precedence.
type PickupRequest struct {
	WarehouseName        string
	Date                 time.Time // YYYY-MM-DD used; time-of-day ignored
	TimeStart            string    // "HH:MM:SS" — Delhivery's pickup_time
	ExpectedPackageCount int
}

// Pickup is the carrier's response after a successful (or idempotent
// duplicate) pickup request. ProviderPickupID is the carrier's own
// identifier — numeric for Delhivery success, "already-scheduled" for
// the duplicate-tolerance path where the carrier tells us a pickup
// already exists for (warehouse, date) but won't echo back the pr_id.
// ScheduledFor combines Date + TimeStart so callers can render and
// persist a single timestamptz.
type Pickup struct {
	ProviderPickupID string
	ScheduledFor     time.Time
	RawResponse      []byte // for audit / debugging
}

// Address represents a physical address for shipping origin/destination.
//
// Email is optional but required by some carriers (CouriersPlease, parts
// of Aramex) for pickup + destination contact. Populate from the buyer's
// order email for ship_to, and from a configured warehouse email (or the
// buyer's email as a defensive fallback) for ship_from.
type Address struct {
	Name        string
	Line1       string
	Line2       string
	City        string
	Region      string
	PostalCode  string
	CountryCode string
	Phone       string
	Email       string
}

// ParcelItem describes a single item in the shipment for rate calculation.
// Length/Width/Height are optional per-variant package dimensions in
// centimetres. ShipEngineCarrier folds multi-item shipments into a
// single envelope by taking the max along each axis (good enough until
// real packing-cube logic lands).
type ParcelItem struct {
	Title       string
	SKU         string
	Quantity    int
	WeightGrams int
	LengthCM    float64
	WidthCM     float64
	HeightCM    float64
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
