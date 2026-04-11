// Package admin — settings_meta.go: exposes per-store settings metadata
// derived from the supported_countries table. The admin UI uses this to
// render one configuration card per provider allowed for the store's country,
// rather than dead-ending merchants in an empty state.
package admin

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/country"
)

// SettingsMetaHandler serves read-only settings metadata (supported providers,
// carriers, and tax strategy) for the store's country.
type SettingsMetaHandler struct {
	countryRepo country.Repository
	logger      *slog.Logger
}

// NewSettingsMetaHandler constructs a SettingsMetaHandler.
func NewSettingsMetaHandler(countryRepo country.Repository, logger *slog.Logger) *SettingsMetaHandler {
	return &SettingsMetaHandler{countryRepo: countryRepo, logger: logger}
}

// supportedProvidersResponse is the wire DTO for the admin supported-providers
// endpoint. PaymentProviders/ShippingCarriers are always non-nil arrays.
type supportedProvidersResponse struct {
	CountryCode      string   `json:"country_code"`
	CountryName      string   `json:"country_name"`
	CurrencyCode     string   `json:"currency_code"`
	PaymentProviders []string `json:"payment_providers"`
	ShippingCarriers []string `json:"shipping_carriers"`
	TaxStrategy      string   `json:"tax_strategy"`
	TaxRate          *float64 `json:"tax_rate,omitempty"`
}

// GetSupportedProviders handles GET /settings/supported-providers.
//
// Returns the payment providers, shipping carriers, and tax strategy allowed
// for the store's country (read from the supported_countries seed data). The
// admin UI renders one configuration card per entry in payment_providers /
// shipping_carriers, turning the previously empty state into a live picker.
func (h *SettingsMetaHandler) GetSupportedProviders(c *gin.Context) {
	store := storeFromCtx(c)
	if store == nil {
		return
	}

	supported, err := h.countryRepo.GetByCode(c.Request.Context(), store.CountryCode)
	if err != nil {
		h.logger.Error("lookup country for supported providers", "country", store.CountryCode, "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "failed to load supported providers",
		})
		return
	}

	resp := supportedProvidersResponse{
		CountryCode:      supported.CountryCode,
		CountryName:      supported.Name,
		CurrencyCode:     supported.CurrencyCode,
		PaymentProviders: []string(supported.PaymentProviders),
		ShippingCarriers: []string(supported.ShippingCarriers),
		TaxStrategy:      supported.TaxStrategy,
		TaxRate:          supported.TaxRate,
	}
	if resp.PaymentProviders == nil {
		resp.PaymentProviders = []string{}
	}
	if resp.ShippingCarriers == nil {
		resp.ShippingCarriers = []string{}
	}

	c.JSON(http.StatusOK, gin.H{"data": resp})
}
