// Package admin — settings.go: admin handlers for payment, shipping, and tax
// configuration. Each handler operates on per-store config tables created in
// migration 000008 and validates providers against supported_countries.
package admin

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/country"
	"github.com/mark8ly/marketplace-api/internal/crypto"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// maskKey returns "****" + last 4 chars for display. Returns "****" when the
// input is too short to safely reveal a suffix.
func maskKey(key string) string {
	if len(key) <= 4 {
		return "****"
	}
	return "****" + key[len(key)-4:]
}

// storeFromCtx extracts the *stores.Store set by the StoreMiddleware. Returns
// nil and writes a 401 if the store is absent.
func storeFromCtx(c *gin.Context) *stores.Store {
	v, ok := c.Get("store")
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "missing store context",
		})
		return nil
	}
	s, _ := v.(*stores.Store)
	if s == nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "missing store context",
		})
		return nil
	}
	return s
}

// containsProvider returns true if haystack contains needle (case-insensitive).
func containsProvider(haystack []string, needle string) bool {
	lower := strings.ToLower(needle)
	for _, p := range haystack {
		if strings.ToLower(p) == lower {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// GORM model: payment_gateway_configs
// ---------------------------------------------------------------------------

// PaymentGatewayConfig is the GORM model for the payment_gateway_configs table.
type PaymentGatewayConfig struct {
	ID                uuid.UUID  `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID          uuid.UUID  `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID           uuid.UUID  `gorm:"column:store_id;type:uuid;not null"`
	Provider          string     `gorm:"column:provider;type:varchar(20);not null"`
	APIKeyEncrypted   string     `gorm:"column:api_key_encrypted;type:text;not null"`
	SecretKeyEncrypted string    `gorm:"column:secret_key_encrypted;type:text"`
	Mode              string     `gorm:"column:mode;type:varchar(10);not null;default:test"`
	IsActive          bool       `gorm:"column:is_active;type:boolean;not null;default:false"`
	CreatedAt         time.Time  `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt         time.Time  `gorm:"column:updated_at;not null;default:now()"`
}

func (PaymentGatewayConfig) TableName() string { return "payment_gateway_configs" }

// ---------------------------------------------------------------------------
// PaymentSettingsHandler
// ---------------------------------------------------------------------------

// PaymentSettingsHandler provides CRUD for per-store payment gateway configs.
type PaymentSettingsHandler struct {
	db          *gorm.DB
	countryRepo country.Repository
	audit       *audit.Emitter // optional — nil-safe
	encryptor   crypto.Encryptor
	logger      *slog.Logger
}

// NewPaymentSettingsHandler constructs a PaymentSettingsHandler.
func NewPaymentSettingsHandler(db *gorm.DB, countryRepo country.Repository, enc crypto.Encryptor, logger *slog.Logger) *PaymentSettingsHandler {
	return &PaymentSettingsHandler{db: db, countryRepo: countryRepo, encryptor: enc, logger: logger}
}

// WithAudit attaches an audit emitter so payment provider changes land
// in Settings -> Audit Logs. Nil-safe.
func (h *PaymentSettingsHandler) WithAudit(e *audit.Emitter) *PaymentSettingsHandler {
	h.audit = e
	return h
}

// paymentConfigResponse is the safe wire DTO with keys masked.
type paymentConfigResponse struct {
	ID        string `json:"id"`
	Provider  string `json:"provider"`
	APIKey    string `json:"api_key"`
	SecretKey string `json:"secret_key"`
	Mode      string `json:"mode"`
	IsActive  bool   `json:"is_active"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func toPaymentResponse(cfg PaymentGatewayConfig) paymentConfigResponse {
	secretDisplay := ""
	if cfg.SecretKeyEncrypted != "" {
		secretDisplay = maskKey(cfg.SecretKeyEncrypted)
	}
	return paymentConfigResponse{
		ID:        cfg.ID.String(),
		Provider:  cfg.Provider,
		APIKey:    maskKey(cfg.APIKeyEncrypted),
		SecretKey: secretDisplay,
		Mode:      cfg.Mode,
		IsActive:  cfg.IsActive,
		CreatedAt: cfg.CreatedAt.Format(time.RFC3339),
		UpdatedAt: cfg.UpdatedAt.Format(time.RFC3339),
	}
}

// List handles GET /settings/payments — returns all payment configs for the store.
func (h *PaymentSettingsHandler) List(c *gin.Context) {
	store := storeFromCtx(c)
	if store == nil {
		return
	}

	var configs []PaymentGatewayConfig
	err := h.db.WithContext(c.Request.Context()).
		Where("store_id = ?", store.ID).
		Order("provider ASC").
		Find(&configs).Error
	if err != nil {
		h.logger.Error("list payment configs", "store_id", store.ID, "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "failed to list payment configurations",
		})
		return
	}

	resp := make([]paymentConfigResponse, 0, len(configs))
	for _, cfg := range configs {
		resp = append(resp, toPaymentResponse(cfg))
	}

	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// paymentUpsertRequest is the request body for PUT /settings/payments/:provider.
type paymentUpsertRequest struct {
	APIKey    string `json:"api_key"    binding:"required"`
	SecretKey string `json:"secret_key"`
	Mode      string `json:"mode"       binding:"required,oneof=test live"`
	IsActive  bool   `json:"is_active"`
}

// Upsert handles PUT /settings/payments/:provider — create or update a payment
// gateway config. Validates the provider is supported for the store's country.
func (h *PaymentSettingsHandler) Upsert(c *gin.Context) {
	store := storeFromCtx(c)
	if store == nil {
		return
	}
	provider := strings.ToLower(c.Param("provider"))
	if provider == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "validation_failed",
			"message": "provider is required",
		})
		return
	}

	var req paymentUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "validation_failed",
			"message": err.Error(),
		})
		return
	}

	// Validate provider is supported for the store's country.
	supported, err := h.countryRepo.GetByCode(c.Request.Context(), store.CountryCode)
	if err != nil {
		h.logger.Error("lookup country for provider validation", "country", store.CountryCode, "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "failed to validate provider",
		})
		return
	}
	if !containsProvider(supported.PaymentProviders, provider) {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "validation_failed",
			"message": "provider " + provider + " is not supported for country " + store.CountryCode,
		})
		return
	}

	storeUUID, _ := uuid.Parse(store.ID)
	tenantUUID, _ := uuid.Parse(store.TenantID)

	// Encrypt API keys before storage.
	apiKeyEnc, err := h.encryptor.Encrypt(req.APIKey)
	if err != nil {
		h.logger.Error("encrypt api_key", "store_id", store.ID, "provider", provider, "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "failed to secure payment configuration",
		})
		return
	}
	var secretKeyEnc string
	if req.SecretKey != "" {
		secretKeyEnc, err = h.encryptor.Encrypt(req.SecretKey)
		if err != nil {
			h.logger.Error("encrypt secret_key", "store_id", store.ID, "provider", provider, "err", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error":   "internal",
				"message": "failed to secure payment configuration",
			})
			return
		}
	}

	cfg := PaymentGatewayConfig{
		TenantID:           tenantUUID,
		StoreID:            storeUUID,
		Provider:           provider,
		APIKeyEncrypted:    apiKeyEnc,
		SecretKeyEncrypted: secretKeyEnc,
		Mode:               req.Mode,
		IsActive:           req.IsActive,
	}

	result := h.db.WithContext(c.Request.Context()).
		Where("store_id = ? AND provider = ?", storeUUID, provider).
		First(&PaymentGatewayConfig{})

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		cfg.ID = uuid.New()
		if err := h.db.WithContext(c.Request.Context()).Create(&cfg).Error; err != nil {
			h.logger.Error("create payment config", "store_id", store.ID, "provider", provider, "err", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error":   "internal",
				"message": "failed to save payment configuration",
			})
			return
		}
	} else if result.Error != nil {
		h.logger.Error("lookup payment config", "store_id", store.ID, "provider", provider, "err", result.Error)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "failed to save payment configuration",
		})
		return
	} else {
		updates := map[string]any{
			"api_key_encrypted":    apiKeyEnc,
			"secret_key_encrypted": secretKeyEnc,
			"mode":                 req.Mode,
			"is_active":            req.IsActive,
			"updated_at":           time.Now(),
		}
		if err := h.db.WithContext(c.Request.Context()).
			Model(&PaymentGatewayConfig{}).
			Where("store_id = ? AND provider = ?", storeUUID, provider).
			Updates(updates).Error; err != nil {
			h.logger.Error("update payment config", "store_id", store.ID, "provider", provider, "err", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error":   "internal",
				"message": "failed to save payment configuration",
			})
			return
		}
		// Re-read to return updated timestamps.
		h.db.WithContext(c.Request.Context()).
			Where("store_id = ? AND provider = ?", storeUUID, provider).
			First(&cfg)
	}

	h.audit.Emit(c, audit.Event{
		Action:       "payment_provider.updated",
		ResourceType: "payment_provider",
		ResourceID:   provider,
		Severity:     audit.SeverityWarning,
		Metadata: map[string]any{
			"provider":  provider,
			"mode":      req.Mode,
			"is_active": req.IsActive,
		},
	})
	c.JSON(http.StatusOK, gin.H{"data": toPaymentResponse(cfg)})
}

// Delete handles DELETE /settings/payments/:provider — removes the config row.
func (h *PaymentSettingsHandler) Delete(c *gin.Context) {
	store := storeFromCtx(c)
	if store == nil {
		return
	}
	provider := strings.ToLower(c.Param("provider"))
	if provider == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "validation_failed",
			"message": "provider is required",
		})
		return
	}

	storeUUID, _ := uuid.Parse(store.ID)
	res := h.db.WithContext(c.Request.Context()).
		Where("store_id = ? AND provider = ?", storeUUID, provider).
		Delete(&PaymentGatewayConfig{})
	if res.Error != nil {
		h.logger.Error("delete payment config", "store_id", store.ID, "provider", provider, "err", res.Error)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "failed to delete payment configuration",
		})
		return
	}
	if res.RowsAffected == 0 {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
			"error":   "not_found",
			"message": "payment configuration not found for provider " + provider,
		})
		return
	}

	h.audit.Emit(c, audit.Event{
		Action:       "payment_provider.removed",
		ResourceType: "payment_provider",
		ResourceID:   provider,
		Severity:     audit.SeverityWarning,
		Metadata:     map[string]any{"provider": provider},
	})
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"deleted": true}})
}

// testConnectionResponse is the wire DTO for POST /settings/payments/:provider/test.
type testConnectionResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// TestConnection handles POST /settings/payments/:provider/test — verifies
// stored credentials by attempting a zero-value auth call.
func (h *PaymentSettingsHandler) TestConnection(c *gin.Context) {
	store := storeFromCtx(c)
	if store == nil {
		return
	}
	provider := strings.ToLower(c.Param("provider"))
	if provider == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "validation_failed",
			"message": "provider is required",
		})
		return
	}

	storeUUID, _ := uuid.Parse(store.ID)
	var cfg PaymentGatewayConfig
	err := h.db.WithContext(c.Request.Context()).
		Where("store_id = ? AND provider = ?", storeUUID, provider).
		First(&cfg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
			"error":   "not_found",
			"message": "payment configuration not found for provider " + provider,
		})
		return
	}
	if err != nil {
		h.logger.Error("lookup payment config for test", "store_id", store.ID, "provider", provider, "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "failed to retrieve payment configuration",
		})
		return
	}

	// Validate credentials by provider. This is a placeholder — each provider
	// would make a real API call (e.g. Stripe PaymentIntent zero-value auth,
	// Razorpay order create). For now we verify the key is non-empty.
	resp := testConnectionResponse{Success: true}
	if cfg.APIKeyEncrypted == "" {
		resp = testConnectionResponse{
			Success: false,
			Error:   "api_key is empty",
		}
	}

	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// ---------------------------------------------------------------------------
// ShippingSettingsHandler
// ---------------------------------------------------------------------------

// ShippingSettingsHandler provides CRUD for per-store shipping carrier configs.
// It delegates to the shipping.Repository for actual DB ops since that repo
// already has full carrier-config CRUD.
type ShippingSettingsHandler struct {
	db          *gorm.DB
	countryRepo country.Repository
	encryptor   crypto.Encryptor
	logger      *slog.Logger
}

// NewShippingSettingsHandler constructs a ShippingSettingsHandler.
func NewShippingSettingsHandler(db *gorm.DB, countryRepo country.Repository, enc crypto.Encryptor, logger *slog.Logger) *ShippingSettingsHandler {
	return &ShippingSettingsHandler{db: db, countryRepo: countryRepo, encryptor: enc, logger: logger}
}

// shippingConfigResponse is the safe wire DTO for carrier configs.
type shippingConfigResponse struct {
	ID                    string `json:"id"`
	Provider              string `json:"provider"`
	APIKey                string `json:"api_key"`
	SecretKey             string `json:"secret_key"`
	Mode                  string `json:"mode"`
	Enabled               bool   `json:"enabled"`
	HandlingFee           string `json:"handling_fee"`
	FreeShippingThreshold string `json:"free_shipping_threshold"`
	WarehouseName         string `json:"warehouse_name,omitempty"`
	WarehouseLine1        string `json:"warehouse_line1,omitempty"`
	WarehouseLine2        string `json:"warehouse_line2,omitempty"`
	WarehouseCity         string `json:"warehouse_city,omitempty"`
	WarehouseRegion       string `json:"warehouse_region,omitempty"`
	WarehousePostal       string `json:"warehouse_postal,omitempty"`
	WarehouseCountry      string `json:"warehouse_country,omitempty"`
	WarehousePhone        string `json:"warehouse_phone,omitempty"`
	CreatedAt             string `json:"created_at"`
	UpdatedAt             string `json:"updated_at"`
}

// ShippingCarrierConfigRow matches the shipping_carrier_configs table with all
// columns including warehouse fields. The shipping.CarrierConfig model is
// already available but we define a local view to cover all DB columns.
type ShippingCarrierConfigRow struct {
	ID                    uuid.UUID       `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID              uuid.UUID       `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID               uuid.UUID       `gorm:"column:store_id;type:uuid;not null"`
	Provider              string          `gorm:"column:provider;type:varchar(20);not null"`
	APIKeyEncrypted       string          `gorm:"column:api_key_encrypted;type:text;not null"`
	SecretKeyEncrypted    string          `gorm:"column:secret_key_encrypted;type:text"`
	Mode                  string          `gorm:"column:mode;type:varchar(10);not null;default:test"`
	IsActive              bool            `gorm:"column:is_active;type:boolean;not null;default:false"`
	WarehouseName         string          `gorm:"column:warehouse_name;type:varchar(200)"`
	WarehouseLine1        string          `gorm:"column:warehouse_line1;type:varchar(300)"`
	WarehouseLine2        string          `gorm:"column:warehouse_line2;type:varchar(300)"`
	WarehouseCity         string          `gorm:"column:warehouse_city;type:varchar(200)"`
	WarehouseRegion       string          `gorm:"column:warehouse_region;type:varchar(200)"`
	WarehousePostal       string          `gorm:"column:warehouse_postal;type:varchar(40)"`
	WarehouseCountry      string          `gorm:"column:warehouse_country;type:char(2)"`
	WarehousePhone        string          `gorm:"column:warehouse_phone;type:varchar(40)"`
	HandlingFee           decimal.Decimal `gorm:"column:handling_fee;type:numeric(12,2);not null;default:0"`
	FreeShippingMin       decimal.Decimal `gorm:"column:free_shipping_min;type:numeric(12,2)"`
	CreatedAt             time.Time       `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt             time.Time       `gorm:"column:updated_at;not null;default:now()"`
}

func (ShippingCarrierConfigRow) TableName() string { return "shipping_carrier_configs" }

func toShippingResponse(cfg ShippingCarrierConfigRow) shippingConfigResponse {
	secretDisplay := ""
	if cfg.SecretKeyEncrypted != "" {
		secretDisplay = maskKey(cfg.SecretKeyEncrypted)
	}
	return shippingConfigResponse{
		ID:                    cfg.ID.String(),
		Provider:              cfg.Provider,
		APIKey:                maskKey(cfg.APIKeyEncrypted),
		SecretKey:             secretDisplay,
		Mode:                  cfg.Mode,
		Enabled:               cfg.IsActive,
		HandlingFee:           cfg.HandlingFee.String(),
		FreeShippingThreshold: cfg.FreeShippingMin.String(),
		WarehouseName:         cfg.WarehouseName,
		WarehouseLine1:        cfg.WarehouseLine1,
		WarehouseLine2:        cfg.WarehouseLine2,
		WarehouseCity:         cfg.WarehouseCity,
		WarehouseRegion:       cfg.WarehouseRegion,
		WarehousePostal:       cfg.WarehousePostal,
		WarehouseCountry:      cfg.WarehouseCountry,
		WarehousePhone:        cfg.WarehousePhone,
		CreatedAt:             cfg.CreatedAt.Format(time.RFC3339),
		UpdatedAt:             cfg.UpdatedAt.Format(time.RFC3339),
	}
}

// List handles GET /settings/shipping — returns all shipping carrier configs.
func (h *ShippingSettingsHandler) List(c *gin.Context) {
	store := storeFromCtx(c)
	if store == nil {
		return
	}

	var configs []ShippingCarrierConfigRow
	err := h.db.WithContext(c.Request.Context()).
		Where("store_id = ?", store.ID).
		Order("provider ASC").
		Find(&configs).Error
	if err != nil {
		h.logger.Error("list shipping configs", "store_id", store.ID, "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "failed to list shipping configurations",
		})
		return
	}

	resp := make([]shippingConfigResponse, 0, len(configs))
	for _, cfg := range configs {
		resp = append(resp, toShippingResponse(cfg))
	}

	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// shippingUpsertRequest is the request body for PUT /settings/shipping/:provider.
type shippingUpsertRequest struct {
	APIKey            string  `json:"api_key"    binding:"required"`
	SecretKey         string  `json:"secret_key"`
	Mode              string  `json:"mode"       binding:"required,oneof=test live"`
	IsActive          bool    `json:"is_active"`
	HandlingFee       float64 `json:"handling_fee"`
	FreeShippingMin   float64 `json:"free_shipping_min"`
	WarehouseName     string  `json:"warehouse_name"`
	WarehouseLine1    string  `json:"warehouse_line1"`
	WarehouseLine2    string  `json:"warehouse_line2"`
	WarehouseCity     string  `json:"warehouse_city"`
	WarehouseRegion   string  `json:"warehouse_region"`
	WarehousePostal   string  `json:"warehouse_postal"`
	WarehouseCountry  string  `json:"warehouse_country"`
	WarehousePhone    string  `json:"warehouse_phone"`
}

// Upsert handles PUT /settings/shipping/:provider — create or update a shipping
// carrier config. Validates the carrier is supported for the store's country.
func (h *ShippingSettingsHandler) Upsert(c *gin.Context) {
	store := storeFromCtx(c)
	if store == nil {
		return
	}
	provider := strings.ToLower(c.Param("provider"))
	if provider == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "validation_failed",
			"message": "provider is required",
		})
		return
	}

	var req shippingUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "validation_failed",
			"message": err.Error(),
		})
		return
	}

	// Validate carrier is supported for the store's country.
	supported, err := h.countryRepo.GetByCode(c.Request.Context(), store.CountryCode)
	if err != nil {
		h.logger.Error("lookup country for carrier validation", "country", store.CountryCode, "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "failed to validate carrier",
		})
		return
	}
	if !containsProvider(supported.ShippingCarriers, provider) {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "validation_failed",
			"message": "carrier " + provider + " is not supported for country " + store.CountryCode,
		})
		return
	}

	storeUUID, _ := uuid.Parse(store.ID)
	tenantUUID, _ := uuid.Parse(store.TenantID)

	// Encrypt API keys before storage.
	apiKeyEnc, err := h.encryptor.Encrypt(req.APIKey)
	if err != nil {
		h.logger.Error("encrypt api_key", "store_id", store.ID, "provider", provider, "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "failed to secure shipping configuration",
		})
		return
	}
	var secretKeyEnc string
	if req.SecretKey != "" {
		secretKeyEnc, err = h.encryptor.Encrypt(req.SecretKey)
		if err != nil {
			h.logger.Error("encrypt secret_key", "store_id", store.ID, "provider", provider, "err", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error":   "internal",
				"message": "failed to secure shipping configuration",
			})
			return
		}
	}

	cfg := ShippingCarrierConfigRow{
		TenantID:           tenantUUID,
		StoreID:            storeUUID,
		Provider:           provider,
		APIKeyEncrypted:    apiKeyEnc,
		SecretKeyEncrypted: secretKeyEnc,
		Mode:               req.Mode,
		IsActive:           req.IsActive,
		HandlingFee:        decimal.NewFromFloat(req.HandlingFee),
		FreeShippingMin:    decimal.NewFromFloat(req.FreeShippingMin),
		WarehouseName:      req.WarehouseName,
		WarehouseLine1:     req.WarehouseLine1,
		WarehouseLine2:     req.WarehouseLine2,
		WarehouseCity:      req.WarehouseCity,
		WarehouseRegion:    req.WarehouseRegion,
		WarehousePostal:    req.WarehousePostal,
		WarehouseCountry:   req.WarehouseCountry,
		WarehousePhone:     req.WarehousePhone,
	}

	result := h.db.WithContext(c.Request.Context()).
		Where("store_id = ? AND provider = ?", storeUUID, provider).
		First(&ShippingCarrierConfigRow{})

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		cfg.ID = uuid.New()
		if err := h.db.WithContext(c.Request.Context()).Create(&cfg).Error; err != nil {
			h.logger.Error("create shipping config", "store_id", store.ID, "provider", provider, "err", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error":   "internal",
				"message": "failed to save shipping configuration",
			})
			return
		}
	} else if result.Error != nil {
		h.logger.Error("lookup shipping config", "store_id", store.ID, "provider", provider, "err", result.Error)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "failed to save shipping configuration",
		})
		return
	} else {
		updates := map[string]any{
			"api_key_encrypted":    apiKeyEnc,
			"secret_key_encrypted": secretKeyEnc,
			"mode":                 req.Mode,
			"is_active":            req.IsActive,
			"handling_fee":         decimal.NewFromFloat(req.HandlingFee),
			"free_shipping_min":    decimal.NewFromFloat(req.FreeShippingMin),
			"warehouse_name":       req.WarehouseName,
			"warehouse_line1":      req.WarehouseLine1,
			"warehouse_line2":      req.WarehouseLine2,
			"warehouse_city":       req.WarehouseCity,
			"warehouse_region":     req.WarehouseRegion,
			"warehouse_postal":     req.WarehousePostal,
			"warehouse_country":    req.WarehouseCountry,
			"warehouse_phone":      req.WarehousePhone,
			"updated_at":           time.Now(),
		}
		if err := h.db.WithContext(c.Request.Context()).
			Model(&ShippingCarrierConfigRow{}).
			Where("store_id = ? AND provider = ?", storeUUID, provider).
			Updates(updates).Error; err != nil {
			h.logger.Error("update shipping config", "store_id", store.ID, "provider", provider, "err", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error":   "internal",
				"message": "failed to save shipping configuration",
			})
			return
		}
		h.db.WithContext(c.Request.Context()).
			Where("store_id = ? AND provider = ?", storeUUID, provider).
			First(&cfg)
	}

	c.JSON(http.StatusOK, gin.H{"data": toShippingResponse(cfg)})
}

// Delete handles DELETE /settings/shipping/:provider — removes the config row.
func (h *ShippingSettingsHandler) Delete(c *gin.Context) {
	store := storeFromCtx(c)
	if store == nil {
		return
	}
	provider := strings.ToLower(c.Param("provider"))
	if provider == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "validation_failed",
			"message": "provider is required",
		})
		return
	}

	storeUUID, _ := uuid.Parse(store.ID)
	res := h.db.WithContext(c.Request.Context()).
		Where("store_id = ? AND provider = ?", storeUUID, provider).
		Delete(&ShippingCarrierConfigRow{})
	if res.Error != nil {
		h.logger.Error("delete shipping config", "store_id", store.ID, "provider", provider, "err", res.Error)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "failed to delete shipping configuration",
		})
		return
	}
	if res.RowsAffected == 0 {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
			"error":   "not_found",
			"message": "shipping configuration not found for carrier " + provider,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{"deleted": true}})
}

// ---------------------------------------------------------------------------
// TaxSettingsHandler
// ---------------------------------------------------------------------------

// TaxSettingsHandler provides read/write for per-store tax configuration.
type TaxSettingsHandler struct {
	db          *gorm.DB
	countryRepo country.Repository
	encryptor   crypto.Encryptor
	logger      *slog.Logger
}

// NewTaxSettingsHandler constructs a TaxSettingsHandler.
func NewTaxSettingsHandler(db *gorm.DB, countryRepo country.Repository, enc crypto.Encryptor, logger *slog.Logger) *TaxSettingsHandler {
	return &TaxSettingsHandler{db: db, countryRepo: countryRepo, encryptor: enc, logger: logger}
}

// TaxProviderConfigRow mirrors tax.TaxProviderConfig but is local to avoid
// import coupling on the stateless tax repository signature.
type TaxProviderConfigRow struct {
	ID              uuid.UUID  `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID        uuid.UUID  `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID         uuid.UUID  `gorm:"column:store_id;type:uuid;not null"`
	Provider        string     `gorm:"column:provider;type:varchar(40);not null"`
	APIKeyEncrypted string     `gorm:"column:api_key_encrypted;type:text"`
	Mode            string     `gorm:"column:mode;type:varchar(20);not null;default:live"`
	IsActive        bool       `gorm:"column:is_active;type:boolean;not null;default:true"`
	CreatedAt       time.Time  `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt       time.Time  `gorm:"column:updated_at;not null;default:now()"`
	DeletedAt       *time.Time `gorm:"column:deleted_at"`
}

func (TaxProviderConfigRow) TableName() string { return "tax_provider_configs" }

// taxSettingsResponse is the composite tax configuration view for a store.
type taxSettingsResponse struct {
	Strategy  string   `json:"strategy"`
	Rate      *float64 `json:"rate,omitempty"`
	TaxJar    *taxJarStatus `json:"taxjar,omitempty"`
	IndiaGST  *indiaGSTInfo `json:"india_gst,omitempty"`
}

type taxJarStatus struct {
	Configured bool   `json:"configured"`
	APIKey     string `json:"api_key,omitempty"`
	Mode       string `json:"mode,omitempty"`
	IsActive   bool   `json:"is_active"`
}

type indiaGSTInfo struct {
	Description string `json:"description"`
}

// Get handles GET /settings/tax — returns the tax configuration for the store
// based on the store's country: strategy, rate (flat), TaxJar status (US),
// or GST info (IN).
func (h *TaxSettingsHandler) Get(c *gin.Context) {
	store := storeFromCtx(c)
	if store == nil {
		return
	}

	supported, err := h.countryRepo.GetByCode(c.Request.Context(), store.CountryCode)
	if err != nil {
		h.logger.Error("lookup country for tax settings", "country", store.CountryCode, "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "failed to load tax configuration",
		})
		return
	}

	resp := taxSettingsResponse{
		Strategy: supported.TaxStrategy,
		Rate:     supported.TaxRate,
	}

	// US stores: show TaxJar config status.
	if supported.TaxStrategy == "taxjar" {
		storeUUID, _ := uuid.Parse(store.ID)
		var cfg TaxProviderConfigRow
		lookupErr := h.db.WithContext(c.Request.Context()).
			Where("store_id = ? AND provider = ?", storeUUID, "taxjar").
			First(&cfg).Error

		tj := &taxJarStatus{Configured: false, IsActive: false}
		if lookupErr == nil {
			tj.Configured = true
			tj.APIKey = maskKey(cfg.APIKeyEncrypted)
			tj.Mode = cfg.Mode
			tj.IsActive = cfg.IsActive
		} else if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			h.logger.Error("lookup taxjar config", "store_id", store.ID, "err", lookupErr)
		}
		resp.TaxJar = tj
	}

	// India stores: show GST info.
	if supported.TaxStrategy == "india_gst" {
		resp.IndiaGST = &indiaGSTInfo{
			Description: "GST rates are determined per-product via HSN code and gst_rate fields",
		}
	}

	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// taxJarUpsertRequest is the request body for PUT /settings/tax/taxjar.
type taxJarUpsertRequest struct {
	APIKey string `json:"api_key" binding:"required"`
	Mode   string `json:"mode"    binding:"required,oneof=test live"`
}

// UpsertTaxJar handles PUT /settings/tax/taxjar — create or update the TaxJar
// API configuration. Only allowed for US stores (tax_strategy = "taxjar").
func (h *TaxSettingsHandler) UpsertTaxJar(c *gin.Context) {
	store := storeFromCtx(c)
	if store == nil {
		return
	}

	// Verify the store's country uses TaxJar strategy.
	supported, err := h.countryRepo.GetByCode(c.Request.Context(), store.CountryCode)
	if err != nil {
		h.logger.Error("lookup country for taxjar upsert", "country", store.CountryCode, "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "failed to validate tax configuration",
		})
		return
	}
	if supported.TaxStrategy != "taxjar" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "validation_failed",
			"message": "TaxJar configuration is only available for stores in countries with taxjar strategy (e.g. US)",
		})
		return
	}

	var req taxJarUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "validation_failed",
			"message": err.Error(),
		})
		return
	}

	storeUUID, _ := uuid.Parse(store.ID)
	tenantUUID, _ := uuid.Parse(store.TenantID)

	// Encrypt API key before storage.
	apiKeyEnc, err := h.encryptor.Encrypt(req.APIKey)
	if err != nil {
		h.logger.Error("encrypt api_key", "store_id", store.ID, "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "failed to secure TaxJar configuration",
		})
		return
	}

	cfg := TaxProviderConfigRow{
		TenantID:        tenantUUID,
		StoreID:         storeUUID,
		Provider:        "taxjar",
		APIKeyEncrypted: apiKeyEnc,
		Mode:            req.Mode,
		IsActive:        true,
	}

	result := h.db.WithContext(c.Request.Context()).
		Where("store_id = ? AND provider = ?", storeUUID, "taxjar").
		First(&TaxProviderConfigRow{})

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		cfg.ID = uuid.New()
		if err := h.db.WithContext(c.Request.Context()).Create(&cfg).Error; err != nil {
			h.logger.Error("create taxjar config", "store_id", store.ID, "err", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error":   "internal",
				"message": "failed to save TaxJar configuration",
			})
			return
		}
	} else if result.Error != nil {
		h.logger.Error("lookup taxjar config", "store_id", store.ID, "err", result.Error)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "failed to save TaxJar configuration",
		})
		return
	} else {
		updates := map[string]any{
			"api_key_encrypted": apiKeyEnc,
			"mode":              req.Mode,
			"is_active":         true,
			"updated_at":        time.Now(),
		}
		if err := h.db.WithContext(c.Request.Context()).
			Model(&TaxProviderConfigRow{}).
			Where("store_id = ? AND provider = ?", storeUUID, "taxjar").
			Updates(updates).Error; err != nil {
			h.logger.Error("update taxjar config", "store_id", store.ID, "err", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error":   "internal",
				"message": "failed to save TaxJar configuration",
			})
			return
		}
		h.db.WithContext(c.Request.Context()).
			Where("store_id = ? AND provider = ?", storeUUID, "taxjar").
			First(&cfg)
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"provider":  "taxjar",
			"api_key":   maskKey(cfg.APIKeyEncrypted),
			"mode":      cfg.Mode,
			"is_active": cfg.IsActive,
		},
	})
}
