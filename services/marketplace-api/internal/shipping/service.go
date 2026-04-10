package shipping

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
)

// ShippingConfig holds store-level shipping policy read from the
// shipping_carrier_configs table (or a sensible default).
type ShippingConfig struct {
	// HandlingFee is added on top of every carrier rate.
	HandlingFee decimal.Decimal
	// FreeShippingThreshold — orders with subtotal >= this value pay zero
	// shipping. Zero means free shipping is disabled.
	FreeShippingThreshold decimal.Decimal
}

// ShippingService orchestrates carrier calls with store-level business
// rules (handling fee, free shipping threshold).
type ShippingService struct {
	repo Repository
}

// NewShippingService constructs a ShippingService.
func NewShippingService(repo Repository) *ShippingService {
	return &ShippingService{repo: repo}
}

// GetShippingRates calls the carrier for rates and applies the store's
// handling fee and free-shipping threshold. orderSubtotal is used for the
// free-shipping check; pass zero to skip.
func (s *ShippingService) GetShippingRates(
	ctx context.Context,
	storeID string,
	orderSubtotal decimal.Decimal,
	fromAddress Address,
	toAddress Address,
	items []ParcelItem,
	currency string,
	carrier Carrier,
) ([]Rate, error) {
	rates, err := carrier.GetRates(ctx, RateRequest{
		FromAddress:  fromAddress,
		ToAddress:    toAddress,
		Items:        items,
		CurrencyCode: currency,
	})
	if err != nil {
		return nil, fmt.Errorf("shipping: get rates: %w", err)
	}

	cfg, err := s.loadConfig(ctx, storeID, carrier.ProviderName())
	if err != nil {
		return nil, fmt.Errorf("shipping: get rates: load config: %w", err)
	}

	// Apply free-shipping threshold: if the order subtotal meets or exceeds
	// the threshold, zero out shipping prices.
	freeShipping := !cfg.FreeShippingThreshold.IsZero() &&
		orderSubtotal.GreaterThanOrEqual(cfg.FreeShippingThreshold)

	adjusted := make([]Rate, 0, len(rates))
	for _, r := range rates {
		price := r.Price.Add(cfg.HandlingFee)
		if freeShipping {
			price = decimal.Zero
		}
		adjusted = append(adjusted, Rate{
			Service:       r.Service,
			Carrier:       r.Carrier,
			Price:         price,
			CurrencyCode:  r.CurrencyCode,
			EstimatedDays: r.EstimatedDays,
		})
	}
	return adjusted, nil
}

// CreateShipment delegates to the carrier and persists the resulting
// shipment in the local shipments table.
func (s *ShippingService) CreateShipment(
	ctx context.Context,
	orderID string,
	fromAddress Address,
	toAddress Address,
	items []ParcelItem,
	service string,
	currency string,
	carrier Carrier,
) (*Shipment, error) {
	shipment, err := carrier.CreateShipment(ctx, ShipmentRequest{
		OrderID:      orderID,
		FromAddress:  fromAddress,
		ToAddress:    toAddress,
		Items:        items,
		Service:      service,
		CurrencyCode: currency,
	})
	if err != nil {
		return nil, fmt.Errorf("shipping: create shipment: %w", err)
	}
	return shipment, nil
}

// GetTracking delegates to the carrier for real-time tracking.
func (s *ShippingService) GetTracking(
	ctx context.Context,
	trackingNumber string,
	carrier Carrier,
) (*Tracking, error) {
	tracking, err := carrier.GetTracking(ctx, trackingNumber)
	if err != nil {
		return nil, fmt.Errorf("shipping: get tracking: %w", err)
	}
	return tracking, nil
}

// loadConfig reads the store's carrier config from the repository.
// Returns a zero-value config if no row exists (no handling fee,
// no free shipping threshold).
func (s *ShippingService) loadConfig(ctx context.Context, storeID, providerName string) (ShippingConfig, error) {
	cfg, err := s.repo.GetCarrierConfig(ctx, storeID, providerName)
	if err != nil {
		// No config row is not an error — return defaults.
		return ShippingConfig{}, nil //nolint:nilerr
	}
	return ShippingConfig{
		HandlingFee:           cfg.HandlingFee,
		FreeShippingThreshold: cfg.FreeShippingThreshold,
	}, nil
}
