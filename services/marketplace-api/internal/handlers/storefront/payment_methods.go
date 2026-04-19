// Package storefront — payment_methods.go: GET /storefront/stores/:storeSlug/payment-methods.
// Returns the list of payment providers with active gateway configs for the
// store's country. No auth required (public storefront endpoint).
package storefront

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/country"
	"github.com/mark8ly/marketplace-api/internal/crypto"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// paymentGatewayConfig is the read-only projection of the
// payment_gateway_configs table used to check which providers are configured.
type paymentGatewayConfig struct {
	Provider string `gorm:"column:provider"`
	IsActive bool   `gorm:"column:is_active"`
	// APIKey is the public-facing key (e.g. rzp_test_*** for Razorpay).
	// Required by client-side checkout widgets and safe to expose.
	APIKey string `gorm:"column:api_key_encrypted"`
	Mode   string `gorm:"column:mode"`
}

func (paymentGatewayConfig) TableName() string { return "payment_gateway_configs" }

// PaymentMethodsHandler serves available payment methods for a store.
type PaymentMethodsHandler struct {
	db        *gorm.DB
	encryptor crypto.Encryptor
	logger    *slog.Logger
}

// NewPaymentMethodsHandler constructs a PaymentMethodsHandler.
func NewPaymentMethodsHandler(db *gorm.DB, enc crypto.Encryptor, logger *slog.Logger) *PaymentMethodsHandler {
	return &PaymentMethodsHandler{db: db, encryptor: enc, logger: logger}
}

// paymentMethodResponse is a single provider entry in the response.
type paymentMethodResponse struct {
	Provider string   `json:"provider"`
	Methods  []string `json:"methods"`
	// PublicKey is the provider's publishable key (Razorpay key_id /
	// Stripe publishable key) — needed by client-side checkout widgets.
	// Razorpay treats key_id as fully public; safe to expose.
	PublicKey string `json:"public_key,omitempty"`
	Mode      string `json:"mode,omitempty"`
}

// ListPaymentMethods handles GET /storefront/stores/:storeSlug/payment-methods.
func (h *PaymentMethodsHandler) ListPaymentMethods(c *gin.Context) {
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

	// Look up country to get the list of payment providers supported in that country.
	var sc country.SupportedCountry
	if err := h.db.WithContext(ctx).
		Where("country_code = ? AND is_active = true", store.CountryCode).
		First(&sc).Error; err != nil {
		if h.logger != nil {
			h.logger.Error("payment_methods: country lookup failed",
				"country_code", store.CountryCode,
				"err", err)
		}
		c.JSON(http.StatusOK, gin.H{"data": []paymentMethodResponse{}})
		return
	}

	if len(sc.PaymentProviders) == 0 {
		c.JSON(http.StatusOK, gin.H{"data": []paymentMethodResponse{}})
		return
	}

	// Check which of those providers have active configs for this store.
	var configs []paymentGatewayConfig
	if err := h.db.WithContext(ctx).
		Where("store_id = ? AND is_active = true AND provider IN ?", store.ID, []string(sc.PaymentProviders)).
		Find(&configs).Error; err != nil {
		if h.logger != nil {
			h.logger.Error("payment_methods: gateway config lookup failed",
				"store_id", store.ID,
				"err", err)
		}
		c.JSON(http.StatusOK, gin.H{"data": []paymentMethodResponse{}})
		return
	}

	// Build response: each active provider gets a "card" method by default.
	// Provider-specific methods can be expanded later.
	result := make([]paymentMethodResponse, 0, len(configs))
	for _, cfg := range configs {
		// Decrypt the public key before exposing to the client.
		pubKey, err := h.encryptor.Decrypt(cfg.APIKey)
		if err != nil {
			h.logger.Error("decrypt payment public_key", "provider", cfg.Provider, "err", err)
			continue // skip this provider on decryption error
		}
		result = append(result, paymentMethodResponse{
			Provider:  cfg.Provider,
			Methods:   methodsForProvider(cfg.Provider),
			PublicKey: pubKey,
			Mode:      cfg.Mode,
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// methodsForProvider returns the payment methods available for a provider.
func methodsForProvider(provider string) []string {
	switch provider {
	case "stripe":
		return []string{"card"}
	case "razorpay":
		return []string{"card", "upi", "netbanking"}
	case "paypal":
		return []string{"paypal"}
	default:
		return []string{"card"}
	}
}
