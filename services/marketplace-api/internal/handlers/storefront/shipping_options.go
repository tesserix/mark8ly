// Package storefront — shipping_options.go: GET /storefront/stores/:storeSlug/shipping-options.
//
// Returns the merchant's configured carriers + service levels and the
// ISO-3166-1 alpha-2 country codes each carrier can deliver to, so the
// checkout UI can render a helpful fallback when a customer's address
// is outside the shipped region. Unlike /shipping-rates this endpoint
// does NOT call any carrier API — it's a pure metadata read.
package storefront

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/shipping"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// ShippingOptionsHandler serves the public "what does this store ship?"
// summary. Instantiated in main wire-up alongside ShippingRatesHandler.
type ShippingOptionsHandler struct {
	db     *gorm.DB
	logger *slog.Logger
}

// NewShippingOptionsHandler constructs a ShippingOptionsHandler.
func NewShippingOptionsHandler(db *gorm.DB, logger *slog.Logger) *ShippingOptionsHandler {
	return &ShippingOptionsHandler{db: db, logger: logger}
}

// shippingOptionResponse is one configured-carrier entry.
type shippingOptionResponse struct {
	Carrier            string   `json:"carrier"`
	Mode               string   `json:"mode,omitempty"`
	Services           []string `json:"services"`
	SupportedCountries []string `json:"supported_countries"`
}

// serviceLevelsByCarrier mirrors the admin SERVICE_LEVELS catalog.
// Kept in sync by hand because there's no central registry yet.
var serviceLevelsByCarrier = map[string][]string{
	"delhivery":  {"standard", "express"},
	"shipengine": {"standard", "express"},
	"ninjavan":   {"standard", "express"},
}

// carrierSupportedCountries maps each carrier to the countries its SDK
// can quote rates / create labels for. Conservative list; extend as
// coverage is verified.
var carrierSupportedCountries = map[string][]string{
	"delhivery":  {"IN"},
	"shipengine": {"US", "CA", "GB", "AU"},
	"ninjavan":   {"SG", "MY", "TH", "ID", "VN", "PH"},
}

// GetOptions handles GET /storefront/stores/:storeSlug/shipping-options.
func (h *ShippingOptionsHandler) GetOptions(c *gin.Context) {
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

	ctx := c.Request.Context()

	// Pull every active carrier config for this store. We project the
	// full shipping.CarrierConfig here; only Provider + Mode are
	// surfaced to the client, but reusing the model keeps column names
	// in sync with the migration.
	var cfgs []shipping.CarrierConfig
	if err := h.db.WithContext(ctx).
		Where("store_id = ? AND is_active = true", store.ID).
		Find(&cfgs).Error; err != nil {
		if h.logger != nil {
			h.logger.Error("shipping_options: query failed",
				"store_id", store.ID, "err", err)
		}
		c.JSON(http.StatusOK, gin.H{"data": []shippingOptionResponse{}})
		return
	}

	storeCountry := strings.ToUpper(strings.TrimSpace(store.CountryCode))

	out := make([]shippingOptionResponse, 0, len(cfgs))
	for _, cfg := range cfgs {
		services := serviceLevelsByCarrier[cfg.Provider]
		if services == nil {
			services = []string{"standard"}
		}
		// Restrict the advertised destinations to the merchant's own
		// store country — v1 only supports intra-country shipping
		// (admin has no per-store ship-to-countries setting yet).
		// Result: storefront's checkout locks the buyer's country to
		// the store's country instead of letting them pick any of the
		// carrier's regional capabilities.
		countries := []string{}
		for _, c := range carrierSupportedCountries[cfg.Provider] {
			if c == storeCountry {
				countries = []string{storeCountry}
				break
			}
		}
		out = append(out, shippingOptionResponse{
			Carrier:            cfg.Provider,
			Mode:               cfg.Mode,
			Services:           services,
			SupportedCountries: countries,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}
