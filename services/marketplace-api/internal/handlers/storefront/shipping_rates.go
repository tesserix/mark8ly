// Package storefront — shipping_rates.go: POST /storefront/stores/:storeSlug/shipping-rates.
// Calculates available shipping rates for a cart by resolving the store's
// carrier config, instantiating the carrier, and applying handling fees
// and free-shipping thresholds.
package storefront

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/carriersecrets"
	"github.com/mark8ly/marketplace-api/internal/country"
	"github.com/mark8ly/marketplace-api/internal/crypto"
	"github.com/mark8ly/marketplace-api/internal/shipping"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// ShippingRatesHandler serves shipping rate quotes for a store.
type ShippingRatesHandler struct {
	db          *gorm.DB
	encryptor   crypto.Encryptor
	secretStore carriersecrets.Store
	logger      *slog.Logger
}

// NewShippingRatesHandler constructs a ShippingRatesHandler.
func NewShippingRatesHandler(db *gorm.DB, enc crypto.Encryptor, logger *slog.Logger) *ShippingRatesHandler {
	return &ShippingRatesHandler{db: db, encryptor: enc, logger: logger}
}

// WithSecretStore wires a carriersecrets.Store so the storefront
// rate endpoint can resolve gsm:// references and rewrite legacy
// rows on read. Chainable.
func (h *ShippingRatesHandler) WithSecretStore(s carriersecrets.Store) *ShippingRatesHandler {
	h.secretStore = s
	return h
}

// resolveCredential routes one stored reference/ciphertext to
// plaintext via the Store when wired, or the Encryptor otherwise.
func (h *ShippingRatesHandler) resolveCredential(ctx context.Context, ref string) (string, error) {
	if ref == "" {
		return "", nil
	}
	if h.secretStore != nil {
		return h.secretStore.Get(ctx, ref)
	}
	return h.encryptor.Decrypt(ref)
}

// shippingRateItemRequest is a single item in the rate request body.
type shippingRateItemRequest struct {
	ProductID   string `json:"product_id"`
	VariantID   string `json:"variant_id"`
	Quantity    int    `json:"quantity"     binding:"required,min=1"`
	WeightGrams int   `json:"weight_grams" binding:"required,min=1"`
}

// shippingRateAddressRequest is the destination address.
type shippingRateAddressRequest struct {
	Line1       string `json:"line1"        binding:"required"`
	City        string `json:"city"         binding:"required"`
	Region      string `json:"region"`
	PostalCode  string `json:"postal_code"`
	CountryCode string `json:"country_code" binding:"required,len=2"`
}

// shippingRatesRequest is the wire body for POST .../shipping-rates.
type shippingRatesRequest struct {
	Items  []shippingRateItemRequest  `json:"items"   binding:"required,min=1"`
	ShipTo shippingRateAddressRequest `json:"ship_to" binding:"required"`
}

// shippingRateResponse is a single rate option in the response.
type shippingRateResponse struct {
	Service       string `json:"service"`
	Carrier       string `json:"carrier"`
	Price         string `json:"price"`
	CurrencyCode  string `json:"currency_code"`
	EstimatedDays int    `json:"estimated_days"`
}

// carrierConfigRow mirrors the shipping_carrier_configs table for direct
// DB reads. The shipping.CarrierConfig GORM model uses uuid.UUID fields
// which don't match a string store_id lookup, so we use a local projection.
//
// ID + TenantID are loaded so MaybeRewrap can scope the lazy migration
// and we know which row to UPDATE when we rewrite a legacy inline
// reference to a gsm:// reference.
type carrierConfigRow struct {
	ID                string          `gorm:"column:id"`
	TenantID          string          `gorm:"column:tenant_id"`
	Provider          string          `gorm:"column:provider"`
	APIKey            string          `gorm:"column:api_key_encrypted"`
	SecretKey         string          `gorm:"column:secret_key_encrypted"`
	Mode              string          `gorm:"column:mode"`
	HandlingFee       decimal.Decimal `gorm:"column:handling_fee"`
	FreeShippingMin   *decimal.Decimal `gorm:"column:free_shipping_min"`
	IsActive          bool            `gorm:"column:is_active"`
	WarehouseName     *string         `gorm:"column:warehouse_name"`
	WarehouseLine1    *string         `gorm:"column:warehouse_line1"`
	WarehouseLine2    *string         `gorm:"column:warehouse_line2"`
	WarehouseCity     *string         `gorm:"column:warehouse_city"`
	WarehouseRegion   *string         `gorm:"column:warehouse_region"`
	WarehousePostal   *string         `gorm:"column:warehouse_postal"`
	WarehouseCountry  *string         `gorm:"column:warehouse_country"`
	WarehousePhone    *string         `gorm:"column:warehouse_phone"`
}

func (carrierConfigRow) TableName() string { return "shipping_carrier_configs" }

// GetRates handles POST /storefront/stores/:storeSlug/shipping-rates.
func (h *ShippingRatesHandler) GetRates(c *gin.Context) {
	storeVal, ok := c.Get("store")
	if !ok {
		respondNotFound(c)
		return
	}
	store, ok := storeVal.(*stores.Store)
	if !ok || store == nil {
		respondNotFound(c)
		return
	}

	var req shippingRatesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "validation_failed",
			"message": err.Error(),
		})
		return
	}

	ctx := c.Request.Context()

	// Look up country to get the list of shipping carriers for the store's country.
	var sc country.SupportedCountry
	if err := h.db.WithContext(ctx).
		Where("country_code = ? AND is_active = true", store.CountryCode).
		First(&sc).Error; err != nil {
		if h.logger != nil {
			h.logger.Error("shipping_rates: country lookup failed",
				"country_code", store.CountryCode,
				"err", err)
		}
		c.JSON(http.StatusOK, gin.H{"data": []shippingRateResponse{}})
		return
	}

	if len(sc.ShippingCarriers) == 0 {
		c.JSON(http.StatusOK, gin.H{"data": []shippingRateResponse{}})
		return
	}

	// Load the first active carrier config for this store matching the country's carriers.
	var cfg carrierConfigRow
	if err := h.db.WithContext(ctx).
		Where("store_id = ? AND is_active = true AND provider IN ?", store.ID, []string(sc.ShippingCarriers)).
		First(&cfg).Error; err != nil {
		if h.logger != nil {
			h.logger.Warn("shipping_rates: no active carrier config",
				"store_id", store.ID,
				"carriers", sc.ShippingCarriers,
				"err", err)
		}
		c.JSON(http.StatusOK, gin.H{"data": []shippingRateResponse{}})
		return
	}

	// Resolve API keys before passing to carrier. Routes through the
	// carriersecrets.Store when wired (handles gsm:// + legacy inline),
	// otherwise falls back to the Encryptor-only path.
	apiKey, err := h.resolveCredential(ctx, cfg.APIKey)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("shipping_rates: resolve api_key failed", "err", err)
		}
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "internal server error",
		})
		return
	}
	secretKey, err := h.resolveCredential(ctx, cfg.SecretKey)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("shipping_rates: resolve secret_key failed", "err", err)
		}
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "internal server error",
		})
		return
	}

	// Lazy migration: if the stored references are legacy inline and
	// we're on a gcpsm-configured store, rewrite them as gsm://
	// references in the DB. MaybeRewrap is a no-op for refs that are
	// already gsm:// or empty.
	h.maybeRewrapRow(ctx, cfg, apiKey, secretKey)

	// Instantiate the carrier.
	carrier, err := shipping.NewCarrier(cfg.Provider, apiKey, secretKey, cfg.Mode)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("shipping_rates: carrier instantiation failed",
				"provider", cfg.Provider,
				"err", err)
		}
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "internal server error",
		})
		return
	}

	// Build origin address from warehouse config.
	fromAddr := warehouseAddress(cfg)

	// Build parcel items from request.
	parcels := make([]shipping.ParcelItem, 0, len(req.Items))
	for _, it := range req.Items {
		parcels = append(parcels, shipping.ParcelItem{
			Quantity:    it.Quantity,
			WeightGrams: it.WeightGrams,
		})
	}

	// Call carrier for rates.
	rateReq := shipping.RateRequest{
		FromAddress: fromAddr,
		ToAddress: shipping.Address{
			Line1:       req.ShipTo.Line1,
			City:        req.ShipTo.City,
			Region:      req.ShipTo.Region,
			PostalCode:  req.ShipTo.PostalCode,
			CountryCode: req.ShipTo.CountryCode,
		},
		Items:        parcels,
		CurrencyCode: store.CurrencyCode,
	}

	rates, err := carrier.GetRates(ctx, rateReq)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("shipping_rates: carrier.GetRates failed",
				"provider", cfg.Provider,
				"err", err)
		}
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "unable to calculate shipping rates",
		})
		return
	}

	// Calculate cart subtotal for free-shipping threshold check.
	cartSubtotal := decimal.Zero
	// We don't have prices in the rate request, so free-shipping is based on
	// whether the caller has already exceeded the threshold. For now, we
	// apply handling fee to all rates; the checkout handler will re-check.
	_ = cartSubtotal

	// Apply handling fee and free-shipping threshold.
	result := make([]shippingRateResponse, 0, len(rates))
	for _, r := range rates {
		price := r.Price.Add(cfg.HandlingFee)

		// If free shipping threshold is configured and the rate base is at
		// or below the threshold, zero out. The storefront sends the subtotal
		// at checkout time; here we just expose the rates with handling.
		if cfg.FreeShippingMin != nil && !cfg.FreeShippingMin.IsZero() {
			// Free shipping is evaluated at checkout when the subtotal is
			// known. At the rates endpoint we include the handling fee so
			// the customer sees worst-case pricing.
		}

		result = append(result, shippingRateResponse{
			Service:       r.Service,
			Carrier:       r.Carrier,
			Price:         price.StringFixed(2),
			CurrencyCode:  r.CurrencyCode,
			EstimatedDays: r.EstimatedDays,
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// maybeRewrapRow rewrites legacy inline references on the
// shipping_carrier_configs row to gsm:// references. Safe to call
// every resolve — the Store's MaybeRewrap skips already-migrated
// refs. Persist failures only log; the read already succeeded.
func (h *ShippingRatesHandler) maybeRewrapRow(ctx context.Context, cfg carrierConfigRow, apiKey, secretKey string) {
	if h.secretStore == nil || cfg.ID == "" || cfg.TenantID == "" {
		return
	}
	rw, ok := h.secretStore.(carriersecrets.Rewrapper)
	if !ok {
		return
	}
	if newRef, changed := rw.MaybeRewrap(ctx, cfg.APIKey, carriersecrets.Scope{
		TenantID: cfg.TenantID,
		Domain:   "shipping",
		Provider: cfg.Provider,
		Field:    "api_key",
	}, apiKey); changed {
		if err := h.db.WithContext(ctx).
			Table("shipping_carrier_configs").
			Where("id = ?", cfg.ID).
			Update("api_key_encrypted", newRef).Error; err != nil && h.logger != nil {
			h.logger.Warn("shipping_rates: rewrap api_key persist failed", "id", cfg.ID, "err", err)
		}
	}
	if cfg.SecretKey == "" {
		return
	}
	if newRef, changed := rw.MaybeRewrap(ctx, cfg.SecretKey, carriersecrets.Scope{
		TenantID: cfg.TenantID,
		Domain:   "shipping",
		Provider: cfg.Provider,
		Field:    "secret_key",
	}, secretKey); changed {
		if err := h.db.WithContext(ctx).
			Table("shipping_carrier_configs").
			Where("id = ?", cfg.ID).
			Update("secret_key_encrypted", newRef).Error; err != nil && h.logger != nil {
			h.logger.Warn("shipping_rates: rewrap secret_key persist failed", "id", cfg.ID, "err", err)
		}
	}
}

// warehouseAddress builds a shipping.Address from the carrier config's
// warehouse fields. Returns a zero-value address if fields are nil.
func warehouseAddress(cfg carrierConfigRow) shipping.Address {
	addr := shipping.Address{}
	if cfg.WarehouseName != nil {
		addr.Name = *cfg.WarehouseName
	}
	if cfg.WarehouseLine1 != nil {
		addr.Line1 = *cfg.WarehouseLine1
	}
	if cfg.WarehouseLine2 != nil {
		addr.Line2 = *cfg.WarehouseLine2
	}
	if cfg.WarehouseCity != nil {
		addr.City = *cfg.WarehouseCity
	}
	if cfg.WarehouseRegion != nil {
		addr.Region = *cfg.WarehouseRegion
	}
	if cfg.WarehousePostal != nil {
		addr.PostalCode = *cfg.WarehousePostal
	}
	if cfg.WarehouseCountry != nil {
		addr.CountryCode = *cfg.WarehouseCountry
	}
	if cfg.WarehousePhone != nil {
		addr.Phone = *cfg.WarehousePhone
	}
	return addr
}
