// Package storefront — payment_methods.go: GET /storefront/stores/:storeSlug/payment-methods.
// Returns the list of payment providers with active gateway configs for the
// store's country. No auth required (public storefront endpoint).
package storefront

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/carriersecrets"
	"github.com/mark8ly/marketplace-api/internal/country"
	"github.com/mark8ly/marketplace-api/internal/crypto"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// paymentGatewayConfig is the read-only projection of the
// payment_gateway_configs table used to check which providers are configured.
// ID + TenantID are loaded so MaybeRewrap can scope the lazy migration
// and we know which row to UPDATE when rewriting a legacy inline reference.
type paymentGatewayConfig struct {
	ID       string `gorm:"column:id"`
	TenantID string `gorm:"column:tenant_id"`
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
	db          *gorm.DB
	encryptor   crypto.Encryptor
	secretStore carriersecrets.Store
	logger      *slog.Logger
}

// NewPaymentMethodsHandler constructs a PaymentMethodsHandler.
func NewPaymentMethodsHandler(db *gorm.DB, enc crypto.Encryptor, logger *slog.Logger) *PaymentMethodsHandler {
	return &PaymentMethodsHandler{db: db, encryptor: enc, logger: logger}
}

// WithSecretStore wires a carriersecrets.Store so the payment methods
// endpoint resolves gsm:// references and lazily rewrites legacy
// noop:/aes: rows to gsm://. Chainable.
func (h *PaymentMethodsHandler) WithSecretStore(s carriersecrets.Store) *PaymentMethodsHandler {
	h.secretStore = s
	return h
}

// resolveCred routes a stored reference to plaintext via the Store
// when wired, falling back to the Encryptor path otherwise.
func (h *PaymentMethodsHandler) resolveCred(ctx context.Context, ref string) (string, error) {
	if ref == "" {
		return "", nil
	}
	if h.secretStore != nil {
		return h.secretStore.Get(ctx, ref)
	}
	return h.encryptor.Decrypt(ref)
}

// maybeRewrapRow lazily migrates a legacy inline api_key_encrypted on a
// payment_gateway_configs row to gsm://. Best-effort; persist failures
// only log because the read has already succeeded.
func (h *PaymentMethodsHandler) maybeRewrapRow(ctx context.Context, cfg paymentGatewayConfig, apiKey string) {
	if h.secretStore == nil || cfg.ID == "" || cfg.TenantID == "" {
		return
	}
	rw, ok := h.secretStore.(carriersecrets.Rewrapper)
	if !ok {
		return
	}
	if newRef, changed := rw.MaybeRewrap(ctx, cfg.APIKey, carriersecrets.Scope{
		TenantID: cfg.TenantID,
		Domain:   "payment",
		Provider: cfg.Provider,
		Field:    "api_key",
	}, apiKey); changed {
		if err := h.db.WithContext(ctx).
			Table("payment_gateway_configs").
			Where("id = ?", cfg.ID).
			Update("api_key_encrypted", newRef).Error; err != nil && h.logger != nil {
			h.logger.Warn("payment_methods: rewrap api_key persist failed", "id", cfg.ID, "err", err)
		}
	}
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
		// Resolve the public key before exposing to the client. Routes
		// through the carriersecrets.Store when wired (handles gsm:// +
		// legacy inline), otherwise falls back to the Encryptor path.
		pubKey, err := h.resolveCred(ctx, cfg.APIKey)
		if err != nil {
			if h.logger != nil {
				h.logger.Error("resolve payment public_key", "provider", cfg.Provider, "err", err)
			}
			continue // skip this provider on resolution error
		}
		// Lazy migration: rewrite legacy inline refs to gsm://.
		h.maybeRewrapRow(ctx, cfg, pubKey)
		result = append(result, paymentMethodResponse{
			Provider:  cfg.Provider,
			Methods:   methodsForProvider(cfg.Provider),
			PublicKey: pubKey,
			Mode:      cfg.Mode,
		})
	}

	sortByPreference(result, sc.PaymentProviders)

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// sortByPreference orders the payment-method list by each provider's position
// in the country's payment_providers array, making that array the single source
// of truth for preference: the first entry is the gateway the storefront
// pre-selects at checkout, and the rest render beneath it as alternatives.
//
// The gateway-config query has no ORDER BY, so without this the picker order was
// whatever order Postgres happened to return rows in — stable enough to look
// intentional in testing, and free to change under a plan flip or a row update.
// Sorting here makes "which gateway is the default in India" a property of the
// seed data rather than an accident of row layout, which is what lets the
// decision be reversed by a one-row migration: 000099 promoted Cashfree, 000100
// put Razorpay back in front with Cashfree kept as a selectable option.
func sortByPreference(methods []paymentMethodResponse, preference []string) {
	rank := make(map[string]int, len(preference))
	for i, p := range preference {
		rank[strings.ToLower(p)] = i
	}
	sort.SliceStable(methods, func(i, j int) bool {
		ri, iOK := rank[strings.ToLower(methods[i].Provider)]
		rj, jOK := rank[strings.ToLower(methods[j].Provider)]
		// A provider absent from the allowlist cannot normally reach here (the
		// query filters on it), but if one ever does it sorts last rather than
		// silently landing at position 0 and becoming the pre-selected default.
		if !iOK || !jOK {
			return iOK && !jOK
		}
		return ri < rj
	})
}

// methodsForProvider returns the payment methods available for a provider.
func methodsForProvider(provider string) []string {
	switch provider {
	case "stripe":
		return []string{"card"}
	case "razorpay":
		return []string{"card", "upi", "netbanking"}
	case "cashfree":
		return []string{"card", "upi", "netbanking", "wallet"}
	case "paypal":
		return []string{"paypal"}
	default:
		return []string{"card"}
	}
}
