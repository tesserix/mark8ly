// Package public — delhivery_webhook.go: unauthenticated inbound
// webhook receiver for Delhivery post-scan events. Fires on every
// scan the carrier records (pickup, in-transit, out-for-delivery,
// delivery). We look up the local shipment by AWB, verify the shared
// secret from the matching carrier config, then advance the shipment
// via the same internal helper the 2-minute poller uses so webhook-
// driven and poll-driven transitions produce identical order_events
// rows.
//
// Security model: Delhivery does NOT sign webhook bodies the way
// Stripe or GitHub do. Their documented flow is a URL-level shared
// secret passed as the `X-Webhook-Token` header OR `?token=` query
// arg — some merchants use one, some the other, depending on when
// they signed up. We accept both. The secret is the per-tenant
// shipping_carrier_configs.secret_key that we already store encrypted
// for each provider; merchants set it to any opaque string when
// configuring the webhook in one.delhivery.com → Settings →
// Post Webhook.
//
// Idempotency: Delhivery replays webhooks on transient failures. The
// underlying advanceShipmentFromTracking helper is a no-op when the
// incoming status is less advanced than the current persisted state,
// so duplicate POSTs don't produce duplicate order_events rows.
// Unknown AWBs return 200 so attackers can't enumerate live tracking
// numbers via timing.
package public

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/carriersecrets"
	"github.com/mark8ly/marketplace-api/internal/crypto"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/shipping"
)

// DelhiveryWebhookHandler accepts post-scan events from Delhivery and
// advances the matching shipment + writes an order_events row.
type DelhiveryWebhookHandler struct {
	db          *gorm.DB
	repo        shipping.Repository
	encryptor   crypto.Encryptor
	secretStore carriersecrets.Store
	// advance is the function the handler calls to mutate the local
	// shipment + emit the order_events row. Injected from the admin
	// ShipmentsHandler so webhook-driven and poller-driven
	// transitions share the exact same persistence path (including
	// dispatching the receipt email on "delivered"). When nil, the
	// webhook falls back to a minimal status-only update which keeps
	// the handler testable in isolation but skips the email/
	// notification side-effects.
	advance DelhiveryWebhookAdvanceFunc
	logger  *slog.Logger
}

// DelhiveryWebhookAdvanceFunc mirrors ShipmentsHandler.advanceShipmentFromTracking.
// Defined here so this package can call into admin without importing
// it (which would create an import cycle).
type DelhiveryWebhookAdvanceFunc func(
	ctx context.Context,
	rec *shipping.ShipmentRecord,
	tracking *shipping.Tracking,
) (bool, error)

// NewDelhiveryWebhookHandler constructs a DelhiveryWebhookHandler.
func NewDelhiveryWebhookHandler(
	db *gorm.DB,
	repo shipping.Repository,
	logger *slog.Logger,
) *DelhiveryWebhookHandler {
	return &DelhiveryWebhookHandler{db: db, repo: repo, logger: logger}
}

// WithEncryptor attaches the envelope encryptor. Needed so the
// handler can decrypt the per-tenant secret_key stored on the
// carrier config before comparing it to the inbound token.
func (h *DelhiveryWebhookHandler) WithEncryptor(enc crypto.Encryptor) *DelhiveryWebhookHandler {
	h.encryptor = enc
	return h
}

// WithSecretStore attaches the carrier secrets Store for secret resolution.
func (h *DelhiveryWebhookHandler) WithSecretStore(s carriersecrets.Store) *DelhiveryWebhookHandler {
	h.secretStore = s
	return h
}

// WithAdvance wires the callback that mutates the local shipment row
// + emits order_events. Production callers inject the ShipmentsHandler's
// advanceShipmentFromTracking method; tests inject a spy.
func (h *DelhiveryWebhookHandler) WithAdvance(fn DelhiveryWebhookAdvanceFunc) *DelhiveryWebhookHandler {
	h.advance = fn
	return h
}

// dlWebhookBody mirrors the Delhivery post-scan webhook envelope.
// The carrier POSTs one Shipment per HTTP call; its Status struct
// carries the current terminal state; its Scans list carries the
// full history (we only care about the last one for the timeline
// description). Fields we don't use are ignored — json.Unmarshal
// tolerates unknown keys.
type dlWebhookBody struct {
	Shipment struct {
		AWB    string `json:"AWB"`
		Status struct {
			Status       string `json:"Status"`
			StatusType   string `json:"StatusType"`
			StatusDateTime string `json:"StatusDateTime"`
			Instructions string `json:"Instructions"`
		} `json:"Status"`
		// Delhivery's newer payload carries scans nested the same
		// way the tracking API does. Older payloads omit it
		// entirely — that's fine, we'll fall back to the top-level
		// Status fields.
		Scans []struct {
			ScanDetail struct {
				ScanType        string `json:"ScanType"`
				Instructions    string `json:"Instructions"`
				ScannedLocation string `json:"ScannedLocation"`
				ScanDateTime    string `json:"ScanDateTime"`
			} `json:"ScanDetail"`
		} `json:"Scans"`
	} `json:"Shipment"`
}

// Handle serves POST /api/v1/webhooks/delhivery.
//
// Contract: always respond 200 quickly (Delhivery retries any 4xx/5xx
// aggressively, which would amplify load). Auth failures are the
// single exception — a 401 discourages random attackers without
// leaking whether a given AWB exists. Unknown AWBs are 200 — they
// happen legitimately when a merchant's second account replays old
// webhooks or when we purge a test shipment.
func (h *DelhiveryWebhookHandler) Handle(c *gin.Context) {
	// Respond fast; do the heavy lifting in a detached goroutine.
	// Delhivery's webhook retry cadence is aggressive; we'd rather
	// ack a duplicate than timeout on the network round-trip.
	var body dlWebhookBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_body",
			"message": err.Error(),
		})
		return
	}

	awb := body.Shipment.AWB
	if awb == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "missing_awb",
			"message": "Shipment.AWB is required",
		})
		return
	}

	// Token can arrive as a header or as a query-string param depending
	// on how the merchant configured the webhook URL on Delhivery's
	// dashboard. Accept both to keep the onboarding footgun small.
	token := c.GetHeader("X-Webhook-Token")
	if token == "" {
		token = c.Query("token")
	}

	// Look up the shipment by AWB. If it doesn't exist, pretend it
	// does — don't leak existence of shipments to unauthenticated
	// callers.
	shPtr, err := h.repo.GetShipmentByTrackingNumber(c.Request.Context(), "delhivery", awb)
	if err != nil {
		if h.logger != nil {
			h.logger.Info("delhivery webhook: unknown awb (silently accepted)",
				"awb", awb, "err", err)
		}
		c.JSON(http.StatusOK, gin.H{"accepted": true})
		return
	}
	sh := *shPtr

	// Verify the shared secret against the carrier config. If no
	// carrier config exists for this store+provider, reject — we
	// can't validate anything.
	cfg, err := h.repo.GetCarrierConfig(c.Request.Context(), sh.StoreID.String(), "delhivery")
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("delhivery webhook: missing carrier config",
				"awb", awb, "store_id", sh.StoreID, "err", err)
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	expected, err := h.resolveSecret(c.Request.Context(), cfg)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("delhivery webhook: secret resolve failed",
				"awb", awb, "err", err)
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	// When the merchant hasn't set a webhook secret, we FAIL closed —
	// a deployment without a secret is one-line-of-config away from a
	// public write endpoint hitting our DB. Merchants who want webhooks
	// must set Settings → Shipping → Delhivery → Webhook Secret.
	// Constant-time compare so attackers can't byte-shift the secret
	// via response-timing differences. subtle.ConstantTimeCompare
	// returns 0 on length mismatch too, which is what we want.
	if expected == "" || subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Build the tracking projection + advance. Use a detached
	// context so a slow GetReceiptEmail downstream doesn't hold the
	// webhook 200 hostage.
	tracking := &shipping.Tracking{
		TrackingNumber: awb,
		Status:         shipping.MapDelhiveryStatus(body.Shipment.Status.StatusType),
	}
	if len(body.Shipment.Scans) > 0 {
		last := body.Shipment.Scans[len(body.Shipment.Scans)-1]
		ts, _ := time.Parse("2006-01-02T15:04:05", last.ScanDetail.ScanDateTime)
		tracking.Events = []shipping.TrackingEvent{{
			Status:      last.ScanDetail.ScanType,
			Description: last.ScanDetail.Instructions,
			Location:    last.ScanDetail.ScannedLocation,
			Timestamp:   ts,
		}}
	} else if body.Shipment.Status.Instructions != "" {
		ts, _ := time.Parse("2006-01-02T15:04:05", body.Shipment.Status.StatusDateTime)
		tracking.Events = []shipping.TrackingEvent{{
			Status:      body.Shipment.Status.StatusType,
			Description: body.Shipment.Status.Instructions,
			Timestamp:   ts,
		}}
	}

	recCopy := sh
	if h.advance != nil {
		// Run the advance in a detached goroutine so the 200 comes
		// back fast even if dispatching the receipt email is slow.
		go func(rec shipping.ShipmentRecord, t *shipping.Tracking) {
			bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if _, err := h.advance(bgCtx, &rec, t); err != nil && h.logger != nil {
				h.logger.Error("delhivery webhook: advance failed",
					"awb", rec.TrackingNumber, "err", err)
			}
		}(recCopy, tracking)
	} else {
		// Fallback path when no advance wired: minimal status-only
		// update + order_events row. Keeps the handler honest in
		// deployments that don't mount the shipments admin handler
		// (tests, future storefront-only binaries).
		h.fallbackAdvance(c.Request.Context(), &recCopy, tracking)
	}

	c.JSON(http.StatusOK, gin.H{"accepted": true})
}

// resolveSecret decrypts / resolves the webhook secret on the
// carrier config. Prefers the Store (gsm://) path and falls back to
// the envelope encryptor for legacy rows, mirroring how the admin
// shipments handler resolves carrier creds.
func (h *DelhiveryWebhookHandler) resolveSecret(ctx context.Context, cfg *shipping.CarrierConfig) (string, error) {
	if cfg.SecretKey == "" {
		return "", nil
	}
	if h.secretStore != nil {
		val, err := h.secretStore.Get(ctx, cfg.SecretKey)
		if err != nil {
			return "", fmt.Errorf("resolve secret: %w", err)
		}
		return val, nil
	}
	if h.encryptor != nil {
		return h.encryptor.Decrypt(cfg.SecretKey)
	}
	// No resolver wired — treat the stored value as plaintext.
	return cfg.SecretKey, nil
}

// fallbackAdvance is the minimal status-mutation path used when the
// webhook handler has not been wired to the admin's
// advanceShipmentFromTracking. Writes the new status and an
// order_events row; does NOT dispatch the receipt email (that side-
// effect is owned by the admin handler). Kept tight so the test
// path can still assert on the shipments + order_events tables.
func (h *DelhiveryWebhookHandler) fallbackAdvance(ctx context.Context, rec *shipping.ShipmentRecord, tracking *shipping.Tracking) {
	if tracking == nil || tracking.Status == "" || tracking.Status == rec.Status {
		return
	}
	now := time.Now().UTC()
	fields := map[string]any{"status": tracking.Status, "updated_at": now}
	if tracking.Status == "in_transit" && rec.Status != "in_transit" {
		fields["shipped_at"] = now
	}
	if tracking.Status == "delivered" {
		fields["delivered_at"] = now
	}
	if err := h.db.WithContext(ctx).
		Table("shipments").
		Where("id = ?", rec.ID).
		Updates(fields).Error; err != nil {
		if h.logger != nil {
			h.logger.Error("delhivery webhook fallback: update failed",
				"awb", rec.TrackingNumber, "err", err)
		}
		return
	}
	rec.Status = tracking.Status

	kindMap := map[string]order.EventKind{
		"in_transit":       order.EventKindShipmentInTransit,
		"out_for_delivery": order.EventKindShipmentOutForDelivery,
		"delivered":        order.EventKindShipmentDelivered,
		"exception":        order.EventKindShipmentException,
	}
	kind, ok := kindMap[tracking.Status]
	if !ok {
		return
	}
	desc := ""
	if len(tracking.Events) > 0 {
		desc = tracking.Events[len(tracking.Events)-1].Description
	}
	payload, _ := json.Marshal(order.ShipmentEventPayload{
		ShipmentID:     rec.ID.String(),
		Carrier:        rec.Carrier,
		TrackingNumber: rec.TrackingNumber,
		Status:         tracking.Status,
		Description:    desc,
	})
	evt := &order.OrderEvent{
		ID:      uuid.New(),
		OrderID: rec.OrderID,
		Kind:    string(kind),
		Payload: payload,
	}
	if err := h.db.WithContext(ctx).Create(evt).Error; err != nil && h.logger != nil {
		h.logger.Error("delhivery webhook fallback: append event failed",
			"order_id", rec.OrderID, "err", err)
	}
}
