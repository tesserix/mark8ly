// Package admin — shipments.go: admin handler for creating and retrieving
// shipments on orders. Mounted under /api/v1/admin/stores/:storeId/orders/:id/shipments.
package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/carriersecrets"
	"github.com/mark8ly/marketplace-api/internal/crypto"
	"github.com/mark8ly/marketplace-api/internal/notification"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/orderdoc"
	"github.com/mark8ly/marketplace-api/internal/shipping"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// ShipmentsHandler bundles dependencies for the admin shipment endpoints.
type ShipmentsHandler struct {
	db        *gorm.DB
	svc       *shipping.ShippingService
	repo      shipping.Repository
	docMailer *orderdoc.Service     // optional — nil disables receipt-on-delivery email
	notify    *notification.Service // optional — nil-safe
	// encryptor decrypts carrier_config.api_key_encrypted / secret_key_encrypted
	// before the carrier instance is constructed. The settings handler encrypts
	// on write; if the shipments handler forgets to decrypt on read, Delhivery
	// receives the ciphertext as the bearer token and rejects every request,
	// which is exactly how this flow silently broke before.
	encryptor crypto.Encryptor
	// secretStore, when non-nil, resolves gsm:// references as well as
	// legacy inline ciphertext. When SHIPPING_SECRET_STORE=gcpsm, it also
	// rewrites legacy rows to gsm:// on read via MaybeRewrap so the
	// migration is lazy and zero-downtime.
	secretStore carriersecrets.Store
	// labelMailer sends the shipping-label PDF as an email attachment. Optional —
	// when nil, the email endpoint returns a 503 so the admin can see that
	// label email is not wired in this deployment.
	labelMailer LabelMailer
	// carrierFactory, when non-nil, overrides the normal
	// shipping.NewCarrier() construction inside the sync loop. Lives
	// here (rather than as an argument on runTrackingSync) so tests
	// can attach a fake carrier to a handler the same way production
	// attaches a labelMailer via WithLabelMailer. Nil on production
	// builds — the sync loop goes straight to shipping.NewCarrier.
	// The opaque second arg is the current open-shipment projection;
	// `any` keeps the tuple-struct unexported while still letting
	// cross-package tests register a factory.
	carrierFactory func(provider string, sh any) (shipping.Carrier, bool)
	logger         *slog.Logger
}

// NewShipmentsHandler constructs a ShipmentsHandler. docMailer is
// optional; when nil, the receipt email on delivery is skipped.
func NewShipmentsHandler(
	db *gorm.DB,
	svc *shipping.ShippingService,
	repo shipping.Repository,
	docMailer *orderdoc.Service,
	logger *slog.Logger,
) *ShipmentsHandler {
	return &ShipmentsHandler{db: db, svc: svc, repo: repo, docMailer: docMailer, logger: logger}
}

// WithNotifier attaches the notification service so delivery events fire
// in-app notifications. Nil-safe.
func (h *ShipmentsHandler) WithNotifier(n *notification.Service) *ShipmentsHandler {
	h.notify = n
	return h
}

// WithEncryptor attaches the envelope encryptor used to decrypt carrier
// config credentials before instantiating the carrier client.
func (h *ShipmentsHandler) WithEncryptor(enc crypto.Encryptor) *ShipmentsHandler {
	h.encryptor = enc
	return h
}

// WithSecretStore attaches the carrier secret Store. When set, read
// paths resolve gsm:// references via GCP SM and automatically
// rewrite legacy noop:/aes: rows to gsm:// references. Chainable.
func (h *ShipmentsHandler) WithSecretStore(s carriersecrets.Store) *ShipmentsHandler {
	h.secretStore = s
	return h
}

// WithLabelMailer attaches a mailer that delivers the label PDF as an
// email attachment. Optional — without it, POST /shipments/:id/label/email
// returns 503.
func (h *ShipmentsHandler) WithLabelMailer(m LabelMailer) *ShipmentsHandler {
	h.labelMailer = m
	return h
}

// decryptCarrierCreds resolves the carrier-config's stored ciphertext
// back to the plaintext API key + secret. When no encryptor is wired
// (legacy / misconfigured), we fall back to the raw column value — that
// preserves behaviour for deployments that never turned on envelope
// encryption, while still letting tests inject a NoopEncryptor.
func (h *ShipmentsHandler) decryptCarrierCreds(cfg *shipping.CarrierConfig) (apiKey, secretKey string, err error) {
	if h.encryptor == nil {
		return cfg.APIKey, cfg.SecretKey, nil
	}
	apiKey, err = h.encryptor.Decrypt(cfg.APIKey)
	if err != nil {
		return "", "", fmt.Errorf("decrypt api_key: %w", err)
	}
	if cfg.SecretKey != "" {
		secretKey, err = h.encryptor.Decrypt(cfg.SecretKey)
		if err != nil {
			return "", "", fmt.Errorf("decrypt secret_key: %w", err)
		}
	}
	return apiKey, secretKey, nil
}

// resolveCarrierCreds is the store-aware sibling of decryptCarrierCreds.
// Uses the carriersecrets.Store when wired (handles both gsm:// and
// legacy inline refs) and falls back to the Encryptor path so
// existing tests and deployments continue to work.
func (h *ShipmentsHandler) resolveCarrierCreds(ctx context.Context, cfg *shipping.CarrierConfig) (apiKey, secretKey string, err error) {
	if h.secretStore == nil {
		return h.decryptCarrierCreds(cfg)
	}
	apiKey, err = h.secretStore.Get(ctx, cfg.APIKey)
	if err != nil {
		return "", "", fmt.Errorf("resolve api_key: %w", err)
	}
	if cfg.SecretKey != "" {
		secretKey, err = h.secretStore.Get(ctx, cfg.SecretKey)
		if err != nil {
			return "", "", fmt.Errorf("resolve secret_key: %w", err)
		}
	}
	return apiKey, secretKey, nil
}

// maybeRewrapCarrierCreds lazily migrates legacy inline refs to gsm://.
// Safe to call on every successful resolve — the Store's MaybeRewrap
// is a no-op for refs that are already gsm:// or empty. Any write
// errors are logged and swallowed; the read already succeeded and
// the next call will retry.
func (h *ShipmentsHandler) maybeRewrapCarrierCreds(ctx context.Context, cfg *shipping.CarrierConfig, apiKey, secretKey string) {
	if h.secretStore == nil {
		return
	}
	rw, ok := h.secretStore.(carriersecrets.Rewrapper)
	if !ok {
		return
	}
	tenantID := cfg.TenantID.String()
	if newRef, changed := rw.MaybeRewrap(ctx, cfg.APIKey, carriersecrets.Scope{
		TenantID: tenantID,
		Domain:   "shipping",
		Provider: cfg.Provider,
		Field:    "api_key",
	}, apiKey); changed {
		if err := h.db.WithContext(ctx).
			Table("shipping_carrier_configs").
			Where("id = ?", cfg.ID).
			Update("api_key_encrypted", newRef).Error; err != nil && h.logger != nil {
			h.logger.Warn("shipments: rewrap api_key persist failed", "id", cfg.ID, "err", err)
		}
	}
	if cfg.SecretKey == "" {
		return
	}
	if newRef, changed := rw.MaybeRewrap(ctx, cfg.SecretKey, carriersecrets.Scope{
		TenantID: tenantID,
		Domain:   "shipping",
		Provider: cfg.Provider,
		Field:    "secret_key",
	}, secretKey); changed {
		if err := h.db.WithContext(ctx).
			Table("shipping_carrier_configs").
			Where("id = ?", cfg.ID).
			Update("secret_key_encrypted", newRef).Error; err != nil && h.logger != nil {
			h.logger.Warn("shipments: rewrap secret_key persist failed", "id", cfg.ID, "err", err)
		}
	}
}

// dispatchReceiptEmail fires the receipt email on a detached background
// context. Caller has already verified the shipment transitioned to
// "delivered".
func (h *ShipmentsHandler) dispatchReceiptEmail(orderID uuid.UUID) {
	if h.docMailer == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := h.docMailer.SendReceipt(ctx, orderID); err != nil {
			h.logger.Warn("orderdoc: receipt email dispatch failed",
				"order_id", orderID, "err", err)
		}
	}()
}

// dispatchShipmentDispatchedEmail fires the customer-facing "your
// order has shipped" email on a detached background context. Caller
// has already verified the shipment transitioned to "in_transit".
//
// Idempotent at the DB layer via shipments.dispatched_email_sent_at:
// the first transition to win the atomic UPDATE sends the email; any
// subsequent caller (e.g. carrier webhook arriving after admin marked
// shipped, or vice-versa) sees 0 rows affected and silently skips.
//
// If h.db is nil (test paths), we fall back to firing without the gate
// — tests that need dedup behavior should set up the DB.
func (h *ShipmentsHandler) dispatchShipmentDispatchedEmail(orderID uuid.UUID, rec *shipping.ShipmentRecord) {
	if h.docMailer == nil || rec == nil {
		return
	}

	if h.db != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		res := h.db.WithContext(ctx).
			Table("shipments").
			Where("id = ? AND dispatched_email_sent_at IS NULL", rec.ID).
			Update("dispatched_email_sent_at", time.Now().UTC())
		cancel()
		if res.Error != nil {
			h.logger.Warn("orderdoc: dispatched_email_sent_at gate update failed; firing anyway",
				"order_id", orderID, "shipment_id", rec.ID, "err", res.Error)
		} else if res.RowsAffected == 0 {
			// Another path already sent the email. Skip silently — this
			// is the desired dedup behavior.
			return
		}
	}

	info := orderdoc.ShipmentInfo{
		Carrier:        rec.Carrier,
		TrackingNumber: rec.TrackingNumber,
	}
	if rec.EstimatedDelivery != nil {
		info.EstimatedDelivery = rec.EstimatedDelivery.Format("January 2, 2006")
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := h.docMailer.SendShipmentDispatched(ctx, orderID, info); err != nil {
			h.logger.Warn("orderdoc: shipment-dispatched email failed",
				"order_id", orderID, "err", err)
		}
	}()
}

// ─────────────────────────────────────────────────────────────────────────
// DTOs
// ─────────────────────────────────────────────────────────────────────────

// CreateShipmentRequest is the wire body for POST .../orders/:id/shipments.
type CreateShipmentRequest struct {
	Provider string `json:"provider" binding:"required"`
	Service  string `json:"service"  binding:"required"`
}

// ShipmentResponse is the wire shape returned by shipment endpoints.
type ShipmentResponse struct {
	ID                 string     `json:"id"`
	OrderID            string     `json:"order_id"`
	Provider           string     `json:"provider"`
	ProviderShipmentID string     `json:"provider_shipment_id"`
	TrackingNumber     string     `json:"tracking_number"`
	LabelURL           string     `json:"label_url"`
	Service            string     `json:"service"`
	Status             string     `json:"status"`
	CurrencyCode       string     `json:"currency_code"`
	EstimatedDelivery  *time.Time `json:"estimated_delivery,omitempty"`
	// ShippedAt + DeliveredAt are stamped by the status-transition
	// handler when the shipment moves into the corresponding state.
	// Both omitempty — pending shipments leave them null. DeliveredAt
	// is the canonical "package handed to buyer" timestamp consumed by
	// the receipt PDF; the older proxy was order.updated_at, which
	// drifted whenever any field on the order changed.
	ShippedAt   *time.Time `json:"shipped_at,omitempty"`
	DeliveredAt *time.Time `json:"delivered_at,omitempty"`
	// Pickup scheduling. PickupRequestID is Delhivery's pr_id (or the
	// "already-scheduled" sentinel) and PickupScheduledFor combines
	// the date + slot-start into a single UTC timestamp so the admin
	// UI can render "Pickup: Fri, Apr 21, 14:00" without having to
	// re-derive the slot. Both omitempty — legacy shipments written
	// before the pickup flow shipped leave them null.
	PickupRequestID    string     `json:"pickup_request_id,omitempty"`
	PickupScheduledFor *time.Time `json:"pickup_scheduled_for,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
}

func toShipmentResponse(rec *shipping.ShipmentRecord) ShipmentResponse {
	return ShipmentResponse{
		ID:                 rec.ID.String(),
		OrderID:            rec.OrderID.String(),
		Provider:           rec.Provider,
		ProviderShipmentID: rec.ProviderShipmentID,
		TrackingNumber:     rec.TrackingNumber,
		LabelURL:           rec.LabelURL,
		Service:            rec.Service,
		Status:             rec.Status,
		CurrencyCode:       rec.CurrencyCode,
		EstimatedDelivery:  rec.EstimatedDelivery,
		ShippedAt:          rec.ShippedAt,
		DeliveredAt:        rec.DeliveredAt,
		PickupRequestID:    rec.PickupRequestID,
		PickupScheduledFor: rec.PickupScheduledFor,
		CreatedAt:          rec.CreatedAt,
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Handlers
// ─────────────────────────────────────────────────────────────────────────

// Create handles POST /admin/stores/:storeId/orders/:id/shipments.
func (h *ShipmentsHandler) Create(c *gin.Context) {
	ctx := c.Request.Context()
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")

	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "must be a uuid"), h.logger)
		return
	}

	var req CreateShipmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	// Load the order to get destination address + items + currency.
	var o order.Order
	if err := h.db.WithContext(ctx).
		Where("id = ? AND store_id = ? AND tenant_id = ? AND deleted_at IS NULL", orderID, storeID, tenantID).
		First(&o).Error; err != nil {
		RespondErr(c, apperrors.NotFound("order"), h.logger)
		return
	}

	// Load shipping address.
	var shippingAddr order.OrderAddress
	if err := h.db.WithContext(ctx).
		Where("order_id = ? AND kind = ?", orderID, "shipping").
		First(&shippingAddr).Error; err != nil {
		RespondErr(c, apperrors.ValidationFailed("order", "order has no shipping address"), h.logger)
		return
	}

	// Load order items for parcel info.
	var items []order.OrderItem
	if err := h.db.WithContext(ctx).
		Where("order_id = ?", orderID).
		Find(&items).Error; err != nil {
		RespondErr(c, fmt.Errorf("shipments: load order items: %w", err), h.logger)
		return
	}

	// Load carrier config for credentials + warehouse address.
	provider := strings.ToLower(req.Provider)
	carrierCfg, err := h.repo.GetCarrierConfig(ctx, storeID, provider)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("shipments: carrier config not found",
				"store_id", storeID, "provider", provider, "err", err)
		}
		RespondErr(c, apperrors.ValidationFailed("provider",
			fmt.Sprintf("no active carrier config for %q. Configure it under Settings → Shipping.", provider)), h.logger)
		return
	}

	// Validate warehouse address is configured.
	if carrierCfg.WarehouseLine1 == "" || carrierCfg.WarehouseCity == "" || carrierCfg.WarehouseCountry == "" {
		RespondErr(c, apperrors.ValidationFailed("provider",
			"warehouse address is not configured for this carrier"), h.logger)
		return
	}

	// Decrypt credentials before instantiating the carrier. The admin
	// settings handler stores api_key_encrypted / secret_key_encrypted via
	// the envelope encryptor; passing the ciphertext straight through would
	// land in the carrier as "Authorization: Token <ciphertext>" and every
	// request would fail auth — which is how the Delhivery dashboard was
	// showing zero orders despite this endpoint reporting success.
	apiKey, secretKey, err := h.resolveCarrierCreds(ctx, carrierCfg)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("shipments: resolve carrier creds",
				"store_id", storeID, "provider", provider, "err", err)
		}
		RespondErr(c, fmt.Errorf("shipments: %w", err), h.logger)
		return
	}
	h.maybeRewrapCarrierCreds(ctx, carrierCfg, apiKey, secretKey)

	// Create the carrier instance.
	carrier, err := shipping.NewCarrier(provider, apiKey, secretKey, carrierCfg.Mode)
	if err != nil {
		RespondErr(c, fmt.Errorf("shipments: create carrier: %w", err), h.logger)
		return
	}

	// Build origin address from warehouse config. Email defaults to the
	// buyer's order email so CouriersPlease (and any other carrier that
	// validates pickup_email format) accepts the label. Long-term we
	// should add a dedicated warehouse_email column on
	// shipping_carrier_configs and surface it in admin → Settings →
	// Shipping; this fallback unblocks label generation in the meantime.
	fromAddress := shipping.Address{
		Name:        carrierCfg.WarehouseName,
		Line1:       carrierCfg.WarehouseLine1,
		Line2:       carrierCfg.WarehouseLine2,
		City:        carrierCfg.WarehouseCity,
		Region:      carrierCfg.WarehouseRegion,
		PostalCode:  carrierCfg.WarehousePostal,
		CountryCode: carrierCfg.WarehouseCountry,
		Phone:       carrierCfg.WarehousePhone,
		Email:       o.CustomerEmail,
	}

	// Build destination address from order. Email comes from the order
	// (always present — required at checkout) so the destination contact
	// validation passes.
	toAddress := shipping.Address{
		Name:        shippingAddr.Name,
		Line1:       shippingAddr.Line1,
		City:        shippingAddr.City,
		CountryCode: shippingAddr.CountryCode,
		Email:       o.CustomerEmail,
	}
	if shippingAddr.Line2 != nil {
		toAddress.Line2 = *shippingAddr.Line2
	}
	if shippingAddr.Region != nil {
		toAddress.Region = *shippingAddr.Region
	}
	if shippingAddr.PostalCode != nil {
		toAddress.PostalCode = *shippingAddr.PostalCode
	}
	if shippingAddr.Phone != nil {
		toAddress.Phone = *shippingAddr.Phone
	}

	// Build parcel items from order items. ShipEngine (and every other
	// real carrier) rejects label creation when total weight is 0 —
	// "Weight is required for the package." So look up per-variant
	// weight + dims from product_variants and stamp them on the parcel
	// items. Mirrors the same lookup the storefront checkout does in
	// services/marketplace-api/internal/handlers/storefront/checkout_ext.go
	// (commit 8acdf31). Falls back to a 500g default per unit when the
	// merchant left weight blank, so we still produce a label rather
	// than dead-end the merchant.
	variantIDs := make([]string, 0, len(items))
	for _, it := range items {
		if it.VariantID != nil {
			variantIDs = append(variantIDs, it.VariantID.String())
		}
	}
	type variantShipRow struct {
		ID          string
		WeightGrams *int
		LengthCM    *decimal.Decimal
		WidthCM     *decimal.Decimal
		HeightCM    *decimal.Decimal
	}
	shipByVariant := map[string]variantShipRow{}
	if len(variantIDs) > 0 {
		var rows []variantShipRow
		if err := h.db.WithContext(ctx).
			Table("product_variants").
			Select("id", "weight_grams", "length_cm", "width_cm", "height_cm").
			Where("id IN ?", variantIDs).
			Find(&rows).Error; err == nil {
			for _, r := range rows {
				shipByVariant[r.ID] = r
			}
		}
	}

	const defaultWeightGramsPerUnit = 500
	parcelItems := make([]shipping.ParcelItem, 0, len(items))
	for _, it := range items {
		p := shipping.ParcelItem{
			Title:       it.TitleSnapshot,
			SKU:         it.SKUSnapshot,
			Quantity:    it.Quantity,
			WeightGrams: defaultWeightGramsPerUnit,
		}
		if it.VariantID != nil {
			if vs, ok := shipByVariant[it.VariantID.String()]; ok {
				if vs.WeightGrams != nil && *vs.WeightGrams > 0 {
					p.WeightGrams = *vs.WeightGrams
				}
				if vs.LengthCM != nil {
					f, _ := vs.LengthCM.Float64()
					p.LengthCM = f
				}
				if vs.WidthCM != nil {
					f, _ := vs.WidthCM.Float64()
					p.WidthCM = f
				}
				if vs.HeightCM != nil {
					f, _ := vs.HeightCM.Float64()
					p.HeightCM = f
				}
			}
		}
		parcelItems = append(parcelItems, p)
	}

	// Call the carrier to create shipment.
	shipment, err := h.svc.CreateShipment(
		ctx,
		orderID.String(),
		fromAddress,
		toAddress,
		parcelItems,
		req.Service,
		o.CurrencyCode,
		carrier,
	)
	// Self-heal: Delhivery rejects a label when its pickup location isn't
	// registered on the carrier's side. The admin save is supposed to
	// register it, but that sync is best-effort and can silently fail
	// (it did in prod: warehouse saved locally, never reached Delhivery).
	// Rather than dead-end the merchant, register the warehouse on demand
	// and retry the shipment once.
	var whErr *shipping.WarehouseNotRegisteredError
	if errors.As(err, &whErr) {
		if syncer, ok := carrier.(shipping.WarehouseSyncer); ok {
			wh := shipping.Warehouse{
				Name:        carrierCfg.WarehouseName,
				Phone:       carrierCfg.WarehousePhone,
				Email:       o.CustomerEmail,
				Address:     strings.TrimSpace(carrierCfg.WarehouseLine1 + " " + carrierCfg.WarehouseLine2),
				City:        carrierCfg.WarehouseCity,
				PinCode:     carrierCfg.WarehousePostal,
				CountryCode: carrierCfg.WarehouseCountry,
				Region:      carrierCfg.WarehouseRegion,
			}
			if regErr := syncer.UpsertWarehouse(ctx, wh); regErr != nil {
				// Registration itself failed — that's the real blocker, so
				// surface it instead of the downstream "not registered".
				if h.logger != nil {
					h.logger.Error("shipments: warehouse auto-register failed",
						"order_id", orderID.String(), "provider", provider,
						"warehouse", carrierCfg.WarehouseName, "err", regErr.Error())
				}
				c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{
					"error":   "warehouse_register_failed",
					"message": "Couldn't register your pickup location with " + provider + ": " + regErr.Error(),
				})
				return
			}
			if h.logger != nil {
				h.logger.Info("shipments: warehouse auto-registered, retrying label",
					"order_id", orderID.String(), "provider", provider,
					"warehouse", carrierCfg.WarehouseName)
			}
			shipment, err = h.svc.CreateShipment(
				ctx, orderID.String(), fromAddress, toAddress,
				parcelItems, req.Service, o.CurrencyCode, carrier,
			)
		}
	}

	if err != nil {
		// Temporary verbose surface — bubble the upstream carrier error
		// so the admin UI shows something actionable instead of "internal
		// server error". Keep for now; trim after delivery is stable.
		if h.logger != nil {
			h.logger.Error("shipments: carrier create failed",
				"order_id", orderID.String(),
				"provider", provider,
				"service", req.Service,
				"err", err.Error())
		}
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{
			"error":   "carrier_create_failed",
			"message": err.Error(),
		})
		return
	}

	// Persist the shipment record.
	storeUUID, _ := uuid.Parse(storeID)
	tenantUUID, _ := uuid.Parse(tenantID)

	// ship_from and ship_to are NOT NULL JSONB on the migration-defined
	// shipments table. Build them from the warehouse + customer address
	// we already loaded.
	fromJSON, _ := json.Marshal(map[string]any{
		"name":         carrierCfg.WarehouseName,
		"line1":        carrierCfg.WarehouseLine1,
		"line2":        carrierCfg.WarehouseLine2,
		"city":         carrierCfg.WarehouseCity,
		"region":       carrierCfg.WarehouseRegion,
		"postal_code":  carrierCfg.WarehousePostal,
		"country_code": carrierCfg.WarehouseCountry,
		"phone":        carrierCfg.WarehousePhone,
	})
	toJSON, _ := json.Marshal(map[string]any{
		"name":         shippingAddr.Name,
		"line1":        shippingAddr.Line1,
		"line2":        derefStringPtr(shippingAddr.Line2),
		"city":         shippingAddr.City,
		"region":       derefStringPtr(shippingAddr.Region),
		"postal_code":  derefStringPtr(shippingAddr.PostalCode),
		"country_code": shippingAddr.CountryCode,
		"phone":        derefStringPtr(shippingAddr.Phone),
	})

	rec := &shipping.ShipmentRecord{
		TenantID:          tenantUUID,
		StoreID:           storeUUID,
		OrderID:           orderID,
		Carrier:           provider,
		TrackingNumber:    shipment.TrackingNumber,
		LabelURL:          shipment.LabelURL,
		Status:            "pending",
		ShipFrom:          datatypes.JSON(fromJSON),
		ShipTo:            datatypes.JSON(toJSON),
		CurrencyCode:      o.CurrencyCode,
		EstimatedDelivery: shipment.EstimatedDelivery,
		// Response-only fields — not persisted, but carried so the wire
		// DTO can surface them.
		Provider:           provider,
		ProviderShipmentID: shipment.ProviderShipmentID,
		Service:            shipment.Service,
	}

	if err := h.repo.CreateShipment(ctx, rec); err != nil {
		RespondErr(c, fmt.Errorf("shipments: persist record: %w", err), h.logger)
		return
	}

	// Some carriers (Delhivery) gate the label PDF behind an Authorization
	// header, so the browser can't deep-link directly to the provider's
	// packing-slip URL. We expose a stable admin endpoint that proxies the
	// fetch through this service and persist that as the canonical
	// label_url. Carriers that already returned a public LabelURL from
	// CreateShipment keep theirs.
	if rec.LabelURL == "" {
		if _, ok := carrier.(shipping.LabelFetcher); ok {
			rec.LabelURL = fmt.Sprintf("/api/v1/admin/stores/%s/orders/%s/shipments/%s/label",
				storeID, orderID.String(), rec.ID.String())
			if err := h.db.WithContext(ctx).
				Table("shipments").
				Where("id = ?", rec.ID).
				Update("label_url", rec.LabelURL).Error; err != nil && h.logger != nil {
				h.logger.Warn("shipments: persist label_url failed",
					"shipment_id", rec.ID, "err", err)
			}
		}
	}

	// Auto-schedule a Delhivery pickup so the waybill doesn't sit at
	// "Manifested" waiting for the merchant to click "Add to Pickup"
	// on one.delhivery.com. Only runs when the carrier implements
	// PickupScheduler AND the carrier-config has the master toggle
	// on. Failures here are deliberately non-fatal — the shipment
	// row is already committed and the merchant can retry via the
	// manual reschedule endpoint. Runs before the timeline event so
	// a successful pickup shows up on the same render cycle.
	if carrierCfg.AutoSchedulePickup {
		if ps, ok := carrier.(shipping.PickupScheduler); ok {
			h.tryAutoSchedulePickup(ctx, rec, ps, carrierCfg)
		}
	}

	// Timeline event — customer order-detail page reads order_events to
	// render the tracking feed. Failures here only log; the shipment
	// still persisted successfully.
	h.appendShipmentEvent(ctx, orderID, rec, order.EventKindShipmentCreated,
		"Shipping label created — package will be picked up shortly.")

	c.JSON(http.StatusCreated, toShipmentResponse(rec))
}

// tryAutoSchedulePickup calls SchedulePickup with the next-business-day
// default and persists the pr_id on the shipment row. Errors log WARN
// but never return — the caller's Create response must not fail just
// because Delhivery's pickup endpoint had a hiccup. Duplicates (same
// warehouse + date already booked) are treated as success so the
// merchant isn't punished for retrying a shipment on the same day.
func (h *ShipmentsHandler) tryAutoSchedulePickup(
	ctx context.Context,
	rec *shipping.ShipmentRecord,
	ps shipping.PickupScheduler,
	cfg *shipping.CarrierConfig,
) {
	slot := strings.TrimSpace(cfg.DefaultPickupSlotStart)
	if slot == "" {
		slot = "14:00:00"
	}
	date := nextBusinessDay(time.Now().UTC())
	req := shipping.PickupRequest{
		WarehouseName:        cfg.WarehouseName,
		Date:                 date,
		TimeStart:            slot,
		ExpectedPackageCount: 1,
	}
	p, err := ps.SchedulePickup(ctx, req)
	if err != nil && !errors.Is(err, shipping.ErrPickupAlreadyScheduled) {
		if h.logger != nil {
			h.logger.Warn("shipments: auto-schedule pickup failed",
				"shipment_id", rec.ID.String(),
				"carrier", rec.Carrier,
				"warehouse", cfg.WarehouseName,
				"date", date.Format("2006-01-02"),
				"err", err)
		}
		return
	}
	// Success or duplicate — persist the pickup metadata and append a
	// timeline event either way. The duplicate path still has a valid
	// ScheduledFor on the Pickup so the admin UI "Pickup: <time>" row
	// renders even though the carrier didn't echo back a fresh id.
	if p == nil {
		return
	}
	updates := map[string]any{
		"pickup_request_id":    p.ProviderPickupID,
		"pickup_scheduled_for": p.ScheduledFor,
		"updated_at":           time.Now().UTC(),
	}
	if err := h.db.WithContext(ctx).
		Table("shipments").
		Where("id = ?", rec.ID).
		Updates(updates).Error; err != nil {
		if h.logger != nil {
			h.logger.Warn("shipments: persist pickup metadata failed",
				"shipment_id", rec.ID.String(), "err", err)
		}
		return
	}
	rec.PickupRequestID = p.ProviderPickupID
	rec.PickupScheduledFor = &p.ScheduledFor

	desc := fmt.Sprintf("Pickup scheduled for %s.", p.ScheduledFor.Format("Mon, Jan 2 at 15:04"))
	if errors.Is(err, shipping.ErrPickupAlreadyScheduled) {
		desc = fmt.Sprintf("Pickup already on schedule for %s.", p.ScheduledFor.Format("Mon, Jan 2"))
	}
	h.appendShipmentEvent(ctx, rec.OrderID, rec, order.EventKindPickupScheduled, desc)

	if h.logger != nil {
		h.logger.Info("shipments: pickup scheduled",
			"shipment_id", rec.ID.String(),
			"pickup_request_id", p.ProviderPickupID,
			"scheduled_for", p.ScheduledFor,
			"duplicate", errors.Is(err, shipping.ErrPickupAlreadyScheduled))
	}
}

// nextBusinessDay returns the next Mon–Sat date after t. Delhivery
// treats Sundays as non-operational — submitting a Sunday pickup_date
// is accepted silently but no agent is dispatched — so we skip to
// Monday. We deliberately do NOT handle Indian public holidays here:
// the calendar is long, merchant-specific, and Delhivery's own UI
// handles the rollover on their side. Merchants who need a precise
// date can still use the manual reschedule endpoint.
func nextBusinessDay(t time.Time) time.Time {
	d := t.AddDate(0, 0, 1)
	for d.Weekday() == time.Sunday {
		d = d.AddDate(0, 0, 1)
	}
	return d
}

// ─────────────────────────────────────────────────────────────────────────
// Manual pickup reschedule — POST .../shipments/:shipmentId/pickup/schedule
// ─────────────────────────────────────────────────────────────────────────

// SchedulePickupRequest is the wire body for manual pickup scheduling.
// Both fields are optional; blank values fall back to the carrier
// config's default_pickup_slot_start and the next business day from
// now. Date format matches Delhivery's — "YYYY-MM-DD".
type SchedulePickupRequest struct {
	Date      string `json:"date"`
	SlotStart string `json:"slot_start"`
}

// SchedulePickup handles POST
// /admin/stores/:storeId/orders/:id/shipments/:shipmentId/pickup/schedule.
// Merchants hit this from the "Reschedule pickup" button in the admin
// order panel when the auto-schedule on create missed (wallet low,
// carrier outage) or they want to move the pickup to a different day.
// Returns the updated shipment DTO so the client can echo the new row
// without a separate refetch.
func (h *ShipmentsHandler) SchedulePickup(c *gin.Context) {
	ctx := c.Request.Context()
	storeID := c.Param("storeId")
	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "must be a uuid"), h.logger)
		return
	}
	shipmentID, err := uuid.Parse(c.Param("shipmentId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("shipmentId", "must be a uuid"), h.logger)
		return
	}

	var req SchedulePickupRequest
	// Body is optional on this endpoint — the empty-body "use the
	// defaults" case is the most common manual action. Tolerate the
	// missing-body / invalid-json case by treating it as zero.
	_ = c.ShouldBindJSON(&req)

	rec, err := h.repo.GetShipmentByID(ctx, shipmentID)
	if err != nil || rec.OrderID != orderID || rec.StoreID.String() != storeID {
		RespondErr(c, apperrors.NotFound("shipment"), h.logger)
		return
	}

	carrierCfg, err := h.repo.GetCarrierConfig(ctx, storeID, rec.Carrier)
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("provider",
			fmt.Sprintf("no active carrier config for %q", rec.Carrier)), h.logger)
		return
	}
	apiKey, secretKey, err := h.resolveCarrierCreds(ctx, carrierCfg)
	if err != nil {
		RespondErr(c, fmt.Errorf("shipments: %w", err), h.logger)
		return
	}
	h.maybeRewrapCarrierCreds(ctx, carrierCfg, apiKey, secretKey)
	carrier, err := shipping.NewCarrier(rec.Carrier, apiKey, secretKey, carrierCfg.Mode)
	if err != nil {
		RespondErr(c, fmt.Errorf("shipments: create carrier: %w", err), h.logger)
		return
	}

	ps, ok := carrier.(shipping.PickupScheduler)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{
			"error":   "unsupported_by_carrier",
			"message": fmt.Sprintf("carrier %q does not support pickup scheduling", rec.Carrier),
		})
		return
	}

	// Resolve date: explicit request > next business day from now.
	date := nextBusinessDay(time.Now().UTC())
	if strings.TrimSpace(req.Date) != "" {
		parsed, perr := time.Parse("2006-01-02", strings.TrimSpace(req.Date))
		if perr != nil {
			RespondErr(c, apperrors.ValidationFailed("date", "must be YYYY-MM-DD"), h.logger)
			return
		}
		date = parsed
	}
	// Resolve slot: explicit request > config default > 14:00:00.
	slot := strings.TrimSpace(req.SlotStart)
	if slot == "" {
		slot = strings.TrimSpace(carrierCfg.DefaultPickupSlotStart)
	}
	if slot == "" {
		slot = "14:00:00"
	}

	p, schedErr := ps.SchedulePickup(ctx, shipping.PickupRequest{
		WarehouseName:        carrierCfg.WarehouseName,
		Date:                 date,
		TimeStart:            slot,
		ExpectedPackageCount: 1,
	})
	if schedErr != nil && !errors.Is(schedErr, shipping.ErrPickupAlreadyScheduled) {
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{
			"error":   "pickup_schedule_failed",
			"message": schedErr.Error(),
		})
		return
	}

	updates := map[string]any{
		"pickup_request_id":    p.ProviderPickupID,
		"pickup_scheduled_for": p.ScheduledFor,
		"updated_at":           time.Now().UTC(),
	}
	if err := h.db.WithContext(ctx).
		Table("shipments").
		Where("id = ?", shipmentID).
		Updates(updates).Error; err != nil {
		RespondErr(c, fmt.Errorf("shipments: persist pickup: %w", err), h.logger)
		return
	}
	rec.PickupRequestID = p.ProviderPickupID
	rec.PickupScheduledFor = &p.ScheduledFor

	desc := fmt.Sprintf("Pickup rescheduled for %s.", p.ScheduledFor.Format("Mon, Jan 2 at 15:04"))
	if errors.Is(schedErr, shipping.ErrPickupAlreadyScheduled) {
		desc = fmt.Sprintf("Pickup already on schedule for %s.", p.ScheduledFor.Format("Mon, Jan 2"))
	}
	h.appendShipmentEvent(ctx, rec.OrderID, rec, order.EventKindPickupScheduled, desc)

	c.JSON(http.StatusOK, toShipmentResponse(rec))
}

// derefStringPtr returns "" when the input is nil — used while building
// ship_to / ship_from JSONB from the OrderAddress's nullable columns.
func derefStringPtr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// appendShipmentEvent writes a row into order_events describing a
// shipment lifecycle transition. Separate from the main response path
// so failures don't block the user-visible action.
func (h *ShipmentsHandler) appendShipmentEvent(
	ctx context.Context,
	orderID uuid.UUID,
	rec *shipping.ShipmentRecord,
	kind order.EventKind,
	description string,
) {
	evt := &order.OrderEvent{
		OrderID: orderID,
		Kind:    string(kind),
		Payload: order.EncodeShipment(order.ShipmentEventPayload{
			ShipmentID:     rec.ID.String(),
			Carrier:        rec.Provider,
			TrackingNumber: rec.TrackingNumber,
			Status:         rec.Status,
			Description:    description,
		}),
	}
	if err := h.db.WithContext(ctx).Create(evt).Error; err != nil && h.logger != nil {
		h.logger.Error("shipments: append event failed",
			"order_id", orderID, "kind", kind, "err", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Status advance — PATCH .../shipments/:shipmentId/status
// ─────────────────────────────────────────────────────────────────────────

// UpdateStatusRequest is the wire body for advancing a shipment through
// its tracking lifecycle. Accepted values: in_transit,
// out_for_delivery, delivered, exception.
type UpdateStatusRequest struct {
	Status      string `json:"status" binding:"required"`
	Description string `json:"description,omitempty"`
}

var shipmentStatusKinds = map[string]order.EventKind{
	"in_transit":       order.EventKindShipmentInTransit,
	"out_for_delivery": order.EventKindShipmentOutForDelivery,
	"delivered":        order.EventKindShipmentDelivered,
	"exception":        order.EventKindShipmentException,
}

var shipmentStatusDefaultCopy = map[string]string{
	"in_transit":       "Package is on its way.",
	"out_for_delivery": "Out for delivery — it will arrive today.",
	"delivered":        "Package delivered.",
	"exception":        "Delivery exception — we're looking into it.",
}

// UpdateStatus handles PATCH /admin/stores/:storeId/orders/:id/shipments/:shipmentId/status.
func (h *ShipmentsHandler) UpdateStatus(c *gin.Context) {
	ctx := c.Request.Context()

	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "must be a uuid"), h.logger)
		return
	}
	shipmentID, err := uuid.Parse(c.Param("shipmentId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("shipmentId", "must be a uuid"), h.logger)
		return
	}

	var req UpdateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}
	status := strings.ToLower(strings.TrimSpace(req.Status))
	kind, ok := shipmentStatusKinds[status]
	if !ok {
		RespondErr(c, apperrors.ValidationFailed("status",
			"must be one of in_transit, out_for_delivery, delivered, exception"), h.logger)
		return
	}

	rec, err := h.repo.GetShipmentByOrderID(ctx, orderID)
	if err != nil {
		RespondErr(c, apperrors.NotFound("shipment"), h.logger)
		return
	}
	if rec.ID != shipmentID || rec.StoreID.String() != c.Param("storeId") {
		RespondErr(c, apperrors.NotFound("shipment"), h.logger)
		return
	}

	// Update the shipment row + stamp the relevant timestamp. The
	// shipped_at/delivered_at columns exist in the schema but aren't on
	// the GORM model; write them with a raw field update.
	now := time.Now().UTC()
	fields := map[string]any{"status": status, "updated_at": now}
	if status == "in_transit" {
		fields["shipped_at"] = now
	}
	if status == "delivered" {
		fields["delivered_at"] = now
	}
	if err := h.db.WithContext(ctx).
		Table("shipments").
		Where("id = ?", shipmentID).
		Updates(fields).Error; err != nil {
		RespondErr(c, fmt.Errorf("shipments: update status: %w", err), h.logger)
		return
	}

	rec.Status = status

	desc := req.Description
	if desc == "" {
		desc = shipmentStatusDefaultCopy[status]
	}
	h.appendShipmentEvent(ctx, orderID, rec, kind, desc)

	// Customer notification: shipment just moved into "in_transit" —
	// fire the "your order has shipped" email so the buyer gets
	// tracking visibility. Detached background context.
	if status == "in_transit" {
		h.dispatchShipmentDispatchedEmail(orderID, rec)
	}

	// Customer notification: shipment just moved into "delivered" — fire
	// the receipt email. Detached from the request context so the merchant
	// gets a fast 200 even if SendGrid is slow.
	if status == "delivered" {
		h.dispatchReceiptEmail(orderID)
		fulfilledMsg := "An order was marked delivered."
		fulfilledResource := "order"
		notification.Emit(ctx, h.notify, h.logger, notification.Notification{
			TenantID:     rec.TenantID,
			StoreID:      rec.StoreID,
			Type:         notification.TypeOrderFulfilled,
			Title:        "Order fulfilled",
			Message:      &fulfilledMsg,
			ResourceType: &fulfilledResource,
			ResourceID:   &orderID,
		})
	}

	c.JSON(http.StatusOK, toShipmentResponse(rec))
}

// GetByOrder handles GET /admin/stores/:storeId/orders/:id/shipments.
func (h *ShipmentsHandler) GetByOrder(c *gin.Context) {
	ctx := c.Request.Context()

	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "must be a uuid"), h.logger)
		return
	}

	rec, err := h.repo.GetShipmentByOrderID(ctx, orderID)
	if err != nil {
		// No shipment found — return null.
		c.JSON(http.StatusOK, nil)
		return
	}

	// Verify the shipment belongs to this store.
	if rec.StoreID.String() != c.Param("storeId") {
		c.JSON(http.StatusOK, nil)
		return
	}

	c.JSON(http.StatusOK, toShipmentResponse(rec))
}

// Delete handles DELETE /admin/stores/:storeId/orders/:id/shipments/:shipmentId.
//
// Used to clear a shipment record so the merchant can retry "Create
// shipping label" against the real carrier. Specifically needed for
// any rows carrying TEST-DLV-* tracking numbers from the old test-mode
// stub — they can never produce a real label and would otherwise
// block the admin from retrying. Also handy when a carrier API call
// succeeded but the operator wants to cancel before pickup.
//
// We do NOT call the carrier's cancel endpoint here — that is a
// separate operator action (Delhivery charges for cancelled pickups
// that have already been scheduled). This endpoint only drops the
// local record.
func (h *ShipmentsHandler) Delete(c *gin.Context) {
	ctx := c.Request.Context()
	storeID := c.Param("storeId")
	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "must be a uuid"), h.logger)
		return
	}
	shipmentID, err := uuid.Parse(c.Param("shipmentId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("shipmentId", "must be a uuid"), h.logger)
		return
	}

	rec, err := h.repo.GetShipmentByID(ctx, shipmentID)
	if err != nil || rec.OrderID != orderID || rec.StoreID.String() != storeID {
		RespondErr(c, apperrors.NotFound("shipment"), h.logger)
		return
	}

	if err := h.db.WithContext(ctx).
		Table("shipments").
		Where("id = ?", shipmentID).
		Delete(nil).Error; err != nil {
		RespondErr(c, fmt.Errorf("shipments: delete: %w", err), h.logger)
		return
	}

	h.appendShipmentEvent(ctx, orderID, rec, order.EventKindShipmentCreated,
		"Shipment record cleared — ready to regenerate shipping label.")

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ─────────────────────────────────────────────────────────────────────────
// Label PDF — download + email
// ─────────────────────────────────────────────────────────────────────────

// LabelMailer sends the shipping-label PDF as an email attachment.
// Kept as an interface so this package doesn't depend on a specific
// transport — main.go wires shipping.EmailLabelMailer on top of the
// shared internal/email sender (SendGrid → Resend in prod, log-only
// when no provider keys are configured).
type LabelMailer interface {
	SendLabel(ctx context.Context, in shipping.LabelEmailPayload) error
}

// fetchLabelBytes loads the shipment + carrier config, decrypts the
// credentials, and calls FetchLabel on the carrier. Shared by
// DownloadLabel and EmailLabel so the two surfaces stay in lock-step.
func (h *ShipmentsHandler) fetchLabelBytes(ctx context.Context, storeID string, orderID, shipmentID uuid.UUID) (*shipping.ShipmentRecord, []byte, string, error) {
	rec, err := h.repo.GetShipmentByID(ctx, shipmentID)
	if err != nil {
		return nil, nil, "", apperrors.NotFound("shipment")
	}
	if rec.OrderID != orderID || rec.StoreID.String() != storeID {
		return nil, nil, "", apperrors.NotFound("shipment")
	}
	if rec.TrackingNumber == "" {
		return nil, nil, "", apperrors.ValidationFailed("shipment", "tracking number not yet assigned by carrier")
	}
	// Guard against stubbed waybills from the pre-fix era. The old
	// test-mode fallback wrote "TEST-DLV-<nanos>" into tracking_number
	// so the admin journey could proceed without a registered
	// warehouse. Those rows still live in the DB and will happily hit
	// Delhivery's packing_slip endpoint with a waybill the carrier
	// never heard of — producing a JSON error the browser saves as
	// "label.txt". Refuse to call out to the carrier for stubs and
	// tell the merchant to delete + recreate the shipment.
	if strings.HasPrefix(rec.TrackingNumber, "TEST-DLV-") {
		return nil, nil, "", apperrors.ValidationFailed("shipment",
			"this shipment has a mock tracking number from an earlier test run and will never produce a real label — delete it and click 'Create shipping label' again")
	}

	// If the carrier returned a public download URL on create
	// (ShipEngine /v1/downloads/..., Sendle's S3 link, etc.), just GET
	// that URL and stream the bytes back. Saves a round-trip through
	// the carrier API, and unblocks carriers that don't implement
	// LabelFetcher (anything other than Delhivery today). Skip our own
	// proxy URL — that recurses.
	if strings.HasPrefix(rec.LabelURL, "https://") || strings.HasPrefix(rec.LabelURL, "http://") {
		pdf, ct, err := fetchLabelByURL(ctx, rec.LabelURL)
		if err != nil {
			return nil, nil, "", fmt.Errorf("shipments: fetch label url: %w", err)
		}
		return rec, pdf, ct, nil
	}

	carrierCfg, err := h.repo.GetCarrierConfig(ctx, storeID, rec.Carrier)
	if err != nil {
		return nil, nil, "", apperrors.ValidationFailed("provider",
			fmt.Sprintf("no active carrier config for %q", rec.Carrier))
	}
	apiKey, secretKey, err := h.resolveCarrierCreds(ctx, carrierCfg)
	if err != nil {
		return nil, nil, "", fmt.Errorf("shipments: %w", err)
	}
	h.maybeRewrapCarrierCreds(ctx, carrierCfg, apiKey, secretKey)
	carrier, err := shipping.NewCarrier(rec.Carrier, apiKey, secretKey, carrierCfg.Mode)
	if err != nil {
		return nil, nil, "", fmt.Errorf("shipments: create carrier: %w", err)
	}
	fetcher, ok := carrier.(shipping.LabelFetcher)
	if !ok {
		return nil, nil, "", apperrors.ValidationFailed("provider",
			fmt.Sprintf("carrier %q does not expose a label endpoint", rec.Carrier))
	}
	pdf, ct, err := fetcher.FetchLabel(ctx, rec.TrackingNumber)
	if err != nil {
		return nil, nil, "", fmt.Errorf("shipments: fetch label: %w", err)
	}
	return rec, pdf, ct, nil
}

// fetchLabelByURL streams a public label URL back as bytes. Used for
// carriers (ShipEngine, Sendle) that return a downloadable URL on
// CreateShipment instead of requiring a separate authenticated call.
func fetchLabelByURL(ctx context.Context, url string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("upstream label fetch returned %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/pdf"
	}
	return body, ct, nil
}

// DownloadLabel handles GET /admin/stores/:storeId/orders/:id/shipments/:shipmentId/label.
// Streams the PDF as an attachment; browser triggers a file download.
func (h *ShipmentsHandler) DownloadLabel(c *gin.Context) {
	ctx := c.Request.Context()
	storeID := c.Param("storeId")
	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "must be a uuid"), h.logger)
		return
	}
	shipmentID, err := uuid.Parse(c.Param("shipmentId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("shipmentId", "must be a uuid"), h.logger)
		return
	}
	rec, pdf, ct, err := h.fetchLabelBytes(ctx, storeID, orderID, shipmentID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	filename := fmt.Sprintf("shipping-label-%s.pdf", rec.TrackingNumber)
	c.Header("Content-Type", ct)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Header("Content-Length", fmt.Sprintf("%d", len(pdf)))
	c.Data(http.StatusOK, ct, pdf)
}

// EmailLabelRequest is the wire body for POST .../shipments/:shipmentId/label/email.
type EmailLabelRequest struct {
	Recipient string `json:"recipient" binding:"required,email"`
}

// EmailLabel handles POST /admin/stores/:storeId/orders/:id/shipments/:shipmentId/label/email.
// Fetches the label PDF from the carrier and emails it as an attachment
// to the nominated address. Useful when the fulfilment staff is not
// the same person who physically prints labels (e.g. a 3PL warehouse).
func (h *ShipmentsHandler) EmailLabel(c *gin.Context) {
	if h.labelMailer == nil {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
			"error":   "mailer_unavailable",
			"message": "Label email is not configured for this deployment.",
		})
		return
	}

	ctx := c.Request.Context()
	storeID := c.Param("storeId")
	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "must be a uuid"), h.logger)
		return
	}
	shipmentID, err := uuid.Parse(c.Param("shipmentId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("shipmentId", "must be a uuid"), h.logger)
		return
	}

	var req EmailLabelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	rec, pdf, ct, err := h.fetchLabelBytes(ctx, storeID, orderID, shipmentID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	// Resolve order_number and store display name for the email envelope.
	var meta struct {
		OrderNumber string `gorm:"column:order_number"`
		StoreName   string `gorm:"column:name"`
		TenantID    string `gorm:"column:tenant_id"`
	}
	_ = h.db.WithContext(ctx).Raw(`
		SELECT o.order_number, s.name, s.tenant_id
		FROM orders o
		JOIN stores s ON s.id = o.store_id
		WHERE o.id = ? LIMIT 1`, orderID).Scan(&meta).Error

	if err := h.labelMailer.SendLabel(ctx, shipping.LabelEmailPayload{
		Recipient:      strings.TrimSpace(req.Recipient),
		TenantID:       meta.TenantID,
		StoreName:      meta.StoreName,
		OrderNumber:    meta.OrderNumber,
		Carrier:        rec.Carrier,
		TrackingNumber: rec.TrackingNumber,
		PDF:            pdf,
		ContentType:    ct,
		Filename:       fmt.Sprintf("shipping-label-%s.pdf", rec.TrackingNumber),
	}); err != nil {
		if h.logger != nil {
			h.logger.Error("shipments: label email dispatch failed",
				"shipment_id", shipmentID, "err", err)
		}
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{
			"error":   "email_dispatch_failed",
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":        true,
		"recipient": req.Recipient,
	})
}

// ─────────────────────────────────────────────────────────────────────────
// Tracking sync — pull latest status from the carrier
// ─────────────────────────────────────────────────────────────────────────

// RefreshTracking handles POST /admin/stores/:storeId/orders/:id/shipments/:shipmentId/tracking/refresh.
// Calls the carrier's tracking API and, if the upstream status has
// advanced, persists the new status + writes an order_events row so the
// customer-facing timeline reflects reality. Returns the (possibly
// updated) shipment record.
func (h *ShipmentsHandler) RefreshTracking(c *gin.Context) {
	ctx := c.Request.Context()
	storeID := c.Param("storeId")
	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("id", "must be a uuid"), h.logger)
		return
	}
	shipmentID, err := uuid.Parse(c.Param("shipmentId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("shipmentId", "must be a uuid"), h.logger)
		return
	}

	rec, err := h.repo.GetShipmentByID(ctx, shipmentID)
	if err != nil || rec.OrderID != orderID || rec.StoreID.String() != storeID {
		RespondErr(c, apperrors.NotFound("shipment"), h.logger)
		return
	}
	if rec.TrackingNumber == "" {
		RespondErr(c, apperrors.ValidationFailed("shipment", "no tracking number yet"), h.logger)
		return
	}

	carrierCfg, err := h.repo.GetCarrierConfig(ctx, storeID, rec.Carrier)
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("provider",
			fmt.Sprintf("no active carrier config for %q", rec.Carrier)), h.logger)
		return
	}
	apiKey, secretKey, err := h.resolveCarrierCreds(ctx, carrierCfg)
	if err != nil {
		RespondErr(c, fmt.Errorf("shipments: %w", err), h.logger)
		return
	}
	h.maybeRewrapCarrierCreds(ctx, carrierCfg, apiKey, secretKey)
	carrier, err := shipping.NewCarrier(rec.Carrier, apiKey, secretKey, carrierCfg.Mode)
	if err != nil {
		RespondErr(c, fmt.Errorf("shipments: create carrier: %w", err), h.logger)
		return
	}

	tracking, err := carrier.GetTracking(ctx, rec.TrackingNumber)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{
			"error":   "tracking_fetch_failed",
			"message": err.Error(),
		})
		return
	}

	if _, err := h.AdvanceShipmentFromTracking(ctx, rec, tracking); err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, toShipmentResponse(rec))
}

// AdvanceShipmentFromTracking applies a carrier tracking response to
// the local shipment row. Returns (changed, err). changed==true means
// the status transitioned and callers should count the row as
// updated.
//
// Shared by RefreshTracking (single-shipment, on-demand),
// SyncAllOpenShipments (background poller, every 2 min), and the
// public Delhivery webhook receiver (push path). Centralizing the
// mutation here keeps all three entry points in lock-step — if one
// ever advances a shipment the others can't, the timeline drifts and
// the customer sees conflicting state. Exported so the public
// webhook handler can call into the same code without importing the
// admin package directly (avoids an import cycle).
//
// Caller owns the carrier timeout (this helper is context-respecting).
// rec is mutated in place: its Status is updated when the transition
// persists, so the caller can echo it back.
func (h *ShipmentsHandler) AdvanceShipmentFromTracking(
	ctx context.Context,
	rec *shipping.ShipmentRecord,
	tracking *shipping.Tracking,
) (bool, error) {
	if tracking == nil {
		return false, nil
	}
	// Map the carrier's coarse status onto our internal status vocab.
	// GetTracking returns one of "in_transit", "delivered", "exception".
	// We leave "out_for_delivery" / fine-grained states to the per-scan
	// heuristics inside the carrier (Delhivery surfaces "OFD" scans
	// through tracking.Events rather than the top-level status).
	newStatus := tracking.Status
	if newStatus == "" || newStatus == rec.Status {
		return false, nil
	}
	// Don't regress — delivered is terminal.
	statusOrder := []string{"pending", "created", "in_transit", "out_for_delivery", "delivered"}
	cur := indexOf(statusOrder, rec.Status)
	next := indexOf(statusOrder, newStatus)
	if cur >= 0 && next >= 0 && next < cur {
		return false, nil
	}

	now := time.Now().UTC()
	fields := map[string]any{"status": newStatus, "updated_at": now}
	if newStatus == "in_transit" && rec.Status != "in_transit" {
		fields["shipped_at"] = now
	}
	if newStatus == "delivered" {
		fields["delivered_at"] = now
	}
	if err := h.db.WithContext(ctx).
		Table("shipments").
		Where("id = ?", rec.ID).
		Updates(fields).Error; err != nil {
		return false, fmt.Errorf("shipments: update status: %w", err)
	}
	rec.Status = newStatus

	kind, ok := shipmentStatusKinds[newStatus]
	if ok {
		desc := shipmentStatusDefaultCopy[newStatus]
		if len(tracking.Events) > 0 {
			last := tracking.Events[len(tracking.Events)-1]
			if last.Description != "" {
				desc = last.Description
			}
		}
		h.appendShipmentEvent(ctx, rec.OrderID, rec, kind, desc)
	}
	// Customer email on in_transit transition. Dedup'd at the DB layer
	// via shipments.dispatched_email_sent_at — if the admin's manual
	// PATCH /shipments/:id/status path got here first the email already
	// fired, and dispatchShipmentDispatchedEmail will see 0 rows
	// affected on its gate UPDATE and skip silently.
	if newStatus == "in_transit" {
		h.dispatchShipmentDispatchedEmail(rec.OrderID, rec)
	}
	if newStatus == "delivered" {
		h.dispatchReceiptEmail(rec.OrderID)
		// Fire the in-app notification for the merchant — same path
		// the manual UpdateStatus handler uses so the sync loop's
		// transitions are indistinguishable from human actions.
		fulfilledMsg := "An order was marked delivered."
		fulfilledResource := "order"
		orderID := rec.OrderID
		notification.Emit(ctx, h.notify, h.logger, notification.Notification{
			TenantID:     rec.TenantID,
			StoreID:      rec.StoreID,
			Type:         notification.TypeOrderFulfilled,
			Title:        "Order fulfilled",
			Message:      &fulfilledMsg,
			ResourceType: &fulfilledResource,
			ResourceID:   &orderID,
		})
	}

	return true, nil
}

func indexOf(xs []string, x string) int {
	for i, v := range xs {
		if v == x {
			return i
		}
	}
	return -1
}
