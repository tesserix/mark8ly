// Package admin — shipments_tracking_sync.go: internal-auth endpoint
// POST /internal/shipments/tracking/sync. Scans every non-terminal
// shipment across every tenant / store, asks each carrier for the
// latest tracking state, and advances the local row + emits an
// order_events entry on transitions. Invoked by an in-cluster CronJob
// every 2 minutes so customer-facing timelines reflect carrier state
// without any admin button click.
//
// This handler bypasses the normal admin auth chain (no Keycloak, no
// FGA). It is gated only by the shared X-Internal-Auth secret that
// also guards /internal/audit-events. The CronJob reads the same
// secret from the mark8ly-marketplace-api-internal-auth Secret.
package admin

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/mark8ly/marketplace-api/internal/shipping"
)

// terminalShipmentStatuses are never re-examined by the sync loop.
// "cancelled" is included even though the current schema doesn't emit
// it (we reserve it for the merchant-side cancel flow once that lands)
// so that a future code path writing "cancelled" doesn't accidentally
// keep hitting Delhivery forever.
var terminalShipmentStatuses = []string{"delivered", "exception", "cancelled"}

// syncConcurrency is the max number of in-flight carrier GetTracking
// calls the sync loop will issue at once. Sized small on purpose —
// Delhivery rate-limits tracking at ~60 req/min per token, and a
// large merchant might have hundreds of open shipments. Four is low
// enough to stay well under that budget even with occasional retries
// and keeps the loop tail-latency bounded by the carrier timeout, not
// by our own queueing.
const syncConcurrency = 4

// perShipmentTimeout caps each carrier GetTracking call. Delhivery's
// p99 for /api/v1/packages/json is ~4-6s; 10s is a generous upper
// bound that lets us move on from a stuck call without wedging the
// entire run.
const perShipmentTimeout = 10 * time.Second

// SyncSummary is the JSON body returned by the sync endpoint. Primary
// consumer is the CronJob log — ops can grep for "updated" counts
// over time to validate the poller is doing useful work.
type SyncSummary struct {
	Examined    int            `json:"examined"`
	Updated     int            `json:"updated"`
	Errors      int            `json:"errors"`
	DurationsMS int64          `json:"durations_ms"`
	ByStatus    map[string]int `json:"by_status"`
	// ErrorSamples is a bounded list of human-readable errors from the
	// most recent run. Capped so a bad day doesn't blow the response
	// size. Useful for tailing CronJob logs without digging into
	// individual shipments.
	ErrorSamples []string `json:"error_samples,omitempty"`
}

// openShipment is the projection of shipments + shipping_carrier_configs
// we pull for the sync loop. Flat-struct GORM scan keeps the query to
// one round-trip.
type openShipment struct {
	ID              uuid.UUID `gorm:"column:id"`
	TenantID        uuid.UUID `gorm:"column:tenant_id"`
	StoreID         uuid.UUID `gorm:"column:store_id"`
	OrderID         uuid.UUID `gorm:"column:order_id"`
	Carrier         string    `gorm:"column:carrier"`
	TrackingNumber  string    `gorm:"column:tracking_number"`
	Status          string    `gorm:"column:status"`
	CurrencyCode    string    `gorm:"column:currency_code"`
}

// SyncAllOpenShipments handles POST /internal/shipments/tracking/sync.
//
// It returns 200 with a JSON summary regardless of per-shipment
// failures — the CronJob treats any 200 as "ran cleanly, check the
// body for partial-failure counts". A 5xx would fire CronJob-level
// alerts that drown real outages in noise.
func (h *ShipmentsHandler) SyncAllOpenShipments(c *gin.Context) {
	// Use a fresh background context with a generous deadline so the
	// sync still completes even if the originating HTTP request is
	// cancelled by the client after the run starts. The CronJob POSTs
	// with curl --max-time=110 (below the 2-minute schedule); this
	// gives us headroom but still tears down cleanly on runaway.
	parent := c.Request.Context()
	deadline := 105 * time.Second
	ctx, cancel := context.WithTimeout(parent, deadline)
	defer cancel()

	summary := h.runTrackingSync(ctx)
	c.JSON(http.StatusOK, summary)
}

// runTrackingSync is the testable core: no gin dependency. Returns the
// same summary the HTTP handler serializes.
func (h *ShipmentsHandler) runTrackingSync(ctx context.Context) SyncSummary {
	start := time.Now()
	summary := SyncSummary{ByStatus: make(map[string]int)}

	shipments, err := h.listOpenShipments(ctx)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("shipments sync: list failed", "err", err)
		}
		summary.Errors++
		summary.ErrorSamples = appendErrorSample(summary.ErrorSamples, fmt.Sprintf("list: %v", err))
		summary.DurationsMS = time.Since(start).Milliseconds()
		return summary
	}
	summary.Examined = len(shipments)
	if len(shipments) == 0 {
		summary.DurationsMS = time.Since(start).Milliseconds()
		return summary
	}

	// Cache carrier configs per (storeID, provider) so a hundred
	// shipments for the same store don't hammer the DB a hundred times
	// on the hot path. The cache also drives the error path — if the
	// config for a store is missing, we mark every shipment for that
	// store as an error once and skip the rest.
	type cfgKey struct {
		StoreID  uuid.UUID
		Provider string
	}
	var (
		cfgMu    sync.Mutex
		cfgCache = make(map[cfgKey]*shipping.CarrierConfig)
		cfgErr   = make(map[cfgKey]error)
	)

	var (
		sumMu sync.Mutex
		g     errgroup.Group
	)
	g.SetLimit(syncConcurrency)

	for i := range shipments {
		sh := shipments[i] // capture
		g.Go(func() error {
			if err := ctx.Err(); err != nil {
				return nil // outer deadline hit — stop enqueueing work
			}
			key := cfgKey{StoreID: sh.StoreID, Provider: strings.ToLower(sh.Carrier)}

			cfgMu.Lock()
			cfg, haveCfg := cfgCache[key]
			cErr, haveErr := cfgErr[key]
			cfgMu.Unlock()

			if !haveCfg && !haveErr {
				loaded, loadErr := h.repo.GetCarrierConfig(ctx, sh.StoreID.String(), key.Provider)
				cfgMu.Lock()
				if loadErr != nil {
					cfgErr[key] = loadErr
					cErr = loadErr
				} else {
					cfgCache[key] = loaded
					cfg = loaded
				}
				cfgMu.Unlock()
			}

			if cErr != nil {
				h.recordSyncOutcome(&summary, &sumMu, sh, false,
					fmt.Errorf("carrier config: %w", cErr))
				return nil
			}

			apiKey, secretKey, err := h.resolveCarrierCreds(ctx, cfg)
			if err != nil {
				h.recordSyncOutcome(&summary, &sumMu, sh, false,
					fmt.Errorf("resolve creds: %w", err))
				return nil
			}
			h.maybeRewrapCarrierCreds(ctx, cfg, apiKey, secretKey)

			carrier, err := shipping.NewCarrier(key.Provider, apiKey, secretKey, cfg.Mode)
			if err != nil {
				h.recordSyncOutcome(&summary, &sumMu, sh, false,
					fmt.Errorf("new carrier: %w", err))
				return nil
			}

			// Allow the caller to swap a stub carrier in tests. Nil
			// override → use the resolved production carrier.
			if h.carrierFactory != nil {
				if c, ok := h.carrierFactory(key.Provider, sh); ok {
					carrier = c
				}
			}

			callCtx, callCancel := context.WithTimeout(ctx, perShipmentTimeout)
			tracking, err := carrier.GetTracking(callCtx, sh.TrackingNumber)
			callCancel()
			if err != nil {
				h.recordSyncOutcome(&summary, &sumMu, sh, false,
					fmt.Errorf("get tracking %s: %w", sh.TrackingNumber, err))
				return nil
			}

			rec := &shipping.ShipmentRecord{
				ID:             sh.ID,
				TenantID:       sh.TenantID,
				StoreID:        sh.StoreID,
				OrderID:        sh.OrderID,
				Carrier:        sh.Carrier,
				Provider:       sh.Carrier,
				TrackingNumber: sh.TrackingNumber,
				Status:         sh.Status,
				CurrencyCode:   sh.CurrencyCode,
			}

			changed, advErr := h.AdvanceShipmentFromTracking(ctx, rec, tracking)
			if advErr != nil {
				h.recordSyncOutcome(&summary, &sumMu, sh, false,
					fmt.Errorf("advance: %w", advErr))
				return nil
			}
			h.recordSyncOutcome(&summary, &sumMu, openShipment{
				ID: sh.ID, StoreID: sh.StoreID, Status: rec.Status,
			}, changed, nil)
			return nil
		})
	}

	// errgroup never returns a non-nil err because workers swallow
	// their own failures into the summary — we still Wait so the
	// goroutines drain before we serialize the response.
	_ = g.Wait()

	summary.DurationsMS = time.Since(start).Milliseconds()
	if h.logger != nil {
		h.logger.Info("shipments sync: complete",
			"examined", summary.Examined,
			"updated", summary.Updated,
			"errors", summary.Errors,
			"duration_ms", summary.DurationsMS,
		)
	}
	return summary
}

// listOpenShipments returns every shipment row whose status is NOT in
// the terminal set. Scoped across all tenants / stores — callers of
// this endpoint are the in-cluster CronJob, not a per-tenant admin
// user, so there is no tenant filter.
func (h *ShipmentsHandler) listOpenShipments(ctx context.Context) ([]openShipment, error) {
	var rows []openShipment
	err := h.db.WithContext(ctx).
		Table("shipments").
		Select("id, tenant_id, store_id, order_id, carrier, tracking_number, status, currency_code").
		Where("status NOT IN ?", terminalShipmentStatuses).
		Where("tracking_number <> ''").
		// Ignore synthetic stubs from the pre-fix era — those will
		// never resolve at Delhivery and would burn the per-minute
		// API budget for no good reason.
		Where("tracking_number NOT LIKE 'TEST-DLV-%'").
		Order("updated_at ASC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("shipments sync: list open: %w", err)
	}
	return rows, nil
}

// recordSyncOutcome is a tiny mutex-guarded helper that updates the
// running summary counts. Centralised so the per-shipment workers
// don't each re-implement the locking.
func (h *ShipmentsHandler) recordSyncOutcome(
	summary *SyncSummary,
	mu *sync.Mutex,
	sh openShipment,
	changed bool,
	syncErr error,
) {
	mu.Lock()
	defer mu.Unlock()
	if syncErr != nil {
		summary.Errors++
		summary.ErrorSamples = appendErrorSample(summary.ErrorSamples, syncErr.Error())
		if h.logger != nil {
			h.logger.Warn("shipments sync: per-shipment failure",
				"shipment_id", sh.ID, "store_id", sh.StoreID, "err", syncErr)
		}
		return
	}
	if changed {
		summary.Updated++
	}
	summary.ByStatus[sh.Status]++
}

// appendErrorSample caps the error-sample slice so a bad run doesn't
// produce a multi-megabyte JSON body.
func appendErrorSample(samples []string, e string) []string {
	const maxSamples = 25
	if len(samples) >= maxSamples {
		return samples
	}
	return append(samples, e)
}

// WithCarrierFactory lets tests substitute a stub Carrier without
// spinning up HTTPS test servers for every provider. Returns the
// receiver for chaining. Production code never sets this. The
// factory's second arg is passed an `any` wrapping the internal
// open-shipment projection — tests typically ignore it and just
// return the same stub for every call.
func (h *ShipmentsHandler) WithCarrierFactory(fn func(provider string, sh any) (shipping.Carrier, bool)) *ShipmentsHandler {
	h.carrierFactory = fn
	return h
}

// SyncLogger allows callers (notably cmd/marketplace-api/main.go) to
// inject a logger after construction. Kept consistent with the other
// With* builders in this file.
func (h *ShipmentsHandler) SyncLogger(l *slog.Logger) *ShipmentsHandler {
	h.logger = l
	return h
}
