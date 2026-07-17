// Package admin — setupprogress.go: store-setup progress endpoints.
//
// The dashboard's setup checklist gets its own read model + realtime
// endpoints so the admin UI can show live progress while the merchant
// completes onboarding steps on other pages or in other tabs:
//
//	GET /admin/stores/:storeId/dashboard/setup-progress         — JSON snapshot
//	GET /admin/stores/:storeId/dashboard/setup-progress/stream  — SSE
//	GET /admin/stores/:storeId/dashboard/setup-progress/ws      — WebSocket
//
// Progress counts a "Create your store" pseudo-step that is complete by
// definition (the store row exists), so a brand-new store starts at
// 2/10 = 20% (store created + settings prefilled from onboarding) rather
// than a demoralising 0% — the LinkedIn profile-strength pattern.
//
// Both realtime transports share the same watch loop: recompute the
// checklist every pollInterval and push only when something changed.
// The EXISTS queries are cheap (all hit small per-store tables through
// indexed store_id/tenant_id columns), and the stream ends itself once
// setup is complete or maxStreamAge elapses, so an abandoned tab does
// not hold a connection forever.
package admin

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

const (
	// pollInterval is how often the watch loop recomputes the checklist.
	pollInterval = 3 * time.Second
	// heartbeatInterval keeps intermediaries from timing out idle streams.
	heartbeatInterval = 20 * time.Second
	// maxStreamAge bounds a single connection; clients reconnect.
	maxStreamAge = 30 * time.Minute
	// totalSetupSteps = 1 pseudo-step (store created) + 9 checklist flags.
	totalSetupSteps = 10
)

// SetupProgress is the wire format shared by the snapshot, SSE, and WS
// endpoints. Percent is integer 0–100 so the client never has to round.
type SetupProgress struct {
	Checklist      SetupChecklist `json:"checklist"`
	CompletedSteps int            `json:"completed_steps"`
	TotalSteps     int            `json:"total_steps"`
	Percent        int            `json:"percent"`
}

// computeSetupChecklist runs the per-flag EXISTS queries. Shared with the
// dashboard aggregation endpoint so the two can never drift.
func computeSetupChecklist(db *gorm.DB, storeID, tenantID uuid.UUID) SetupChecklist {
	var checklist SetupChecklist
	checklist.HasStore = true // store middleware passed → the store exists

	db.Raw(`SELECT EXISTS(SELECT 1 FROM products WHERE store_id = ? AND tenant_id = ?)`,
		storeID, tenantID).Scan(&checklist.HasProduct)
	db.Raw(`SELECT EXISTS(SELECT 1 FROM store_branding
		WHERE store_id = ? AND tenant_id = ?
		AND logo_url IS NOT NULL AND logo_url != '')`,
		storeID, tenantID).Scan(&checklist.HasBrandAssets)
	db.Raw(`SELECT EXISTS(SELECT 1 FROM payment_gateway_configs WHERE store_id = ? AND tenant_id = ?)`,
		storeID, tenantID).Scan(&checklist.HasPaymentProvider)
	db.Raw(`SELECT EXISTS(SELECT 1 FROM shipping_carrier_configs WHERE store_id = ? AND tenant_id = ?)`,
		storeID, tenantID).Scan(&checklist.HasShippingCarrier)
	db.Raw(`SELECT EXISTS(SELECT 1 FROM store_branding
		WHERE store_id = ? AND tenant_id = ?
		AND return_policy IS NOT NULL AND return_policy != '')`,
		storeID, tenantID).Scan(&checklist.HasReturnPolicy)
	db.Raw(`SELECT EXISTS(SELECT 1 FROM custom_domains WHERE store_id = ? AND tenant_id = ?)`,
		storeID, tenantID).Scan(&checklist.HasCustomDomain)
	db.Raw(`SELECT EXISTS(SELECT 1 FROM store_branding WHERE store_id = ? AND tenant_id = ?)`,
		storeID, tenantID).Scan(&checklist.HasStorefrontTheme)
	db.Raw(`SELECT EXISTS(SELECT 1 FROM orders WHERE store_id = ? AND tenant_id = ?)`,
		storeID, tenantID).Scan(&checklist.HasFirstOrder)

	return checklist
}

// progressFrom wraps a checklist in the SetupProgress envelope, counting
// the always-complete "store created" pseudo-step.
func progressFrom(c SetupChecklist) SetupProgress {
	completed := 1 // pseudo-step: the store was created
	for _, done := range []bool{
		c.HasStore, c.HasBrandAssets, c.HasProduct, c.HasStorefrontTheme,
		c.HasPaymentProvider, c.HasShippingCarrier, c.HasReturnPolicy,
		c.HasCustomDomain, c.HasFirstOrder,
	} {
		if done {
			completed++
		}
	}
	return SetupProgress{
		Checklist:      c,
		CompletedSteps: completed,
		TotalSteps:     totalSetupSteps,
		Percent:        completed * 100 / totalSetupSteps,
	}
}

// ---------- Handler ----------

// SetupProgressHandler serves the setup-progress snapshot and realtime
// stream endpoints.
type SetupProgressHandler struct {
	db     *gorm.DB
	logger *slog.Logger
}

// NewSetupProgressHandler constructs a SetupProgressHandler.
func NewSetupProgressHandler(db *gorm.DB, logger *slog.Logger) *SetupProgressHandler {
	return &SetupProgressHandler{db: db, logger: logger}
}

func (h *SetupProgressHandler) ids(c *gin.Context) (storeID, tenantID uuid.UUID, ok bool) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return uuid.Nil, uuid.Nil, false
	}
	tenantID, err = uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("tenant_id", "invalid uuid"), h.logger)
		return uuid.Nil, uuid.Nil, false
	}
	return storeID, tenantID, true
}

// Get handles GET /admin/stores/:storeId/dashboard/setup-progress.
func (h *SetupProgressHandler) Get(c *gin.Context) {
	storeID, tenantID, ok := h.ids(c)
	if !ok {
		return
	}
	db := h.db.WithContext(c.Request.Context())
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, progressFrom(computeSetupChecklist(db, storeID, tenantID)))
}

// Stream handles GET /admin/stores/:storeId/dashboard/setup-progress/stream
// as Server-Sent Events. Sends the current snapshot immediately, then a
// "progress" event whenever a step flips, and ": ping" comments as
// heartbeats. Ends once setup is complete.
func (h *SetupProgressHandler) Stream(c *gin.Context) {
	storeID, tenantID, ok := h.ids(c)
	if !ok {
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	// Disable proxy buffering (nginx/istio sidecars) so events flush through.
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	send := func(p SetupProgress) bool {
		data, err := json.Marshal(p)
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(c.Writer, "event: progress\ndata: %s\n\n", data); err != nil {
			return false
		}
		c.Writer.Flush()
		return true
	}

	db := h.db.WithContext(c.Request.Context())
	last := progressFrom(computeSetupChecklist(db, storeID, tenantID))
	if !send(last) {
		return
	}
	if last.CompletedSteps == last.TotalSteps {
		return // nothing left to watch
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()
	deadline := time.NewTimer(maxStreamAge)
	defer deadline.Stop()
	ctx := c.Request.Context()

	for {
		select {
		case <-ctx.Done():
			return
		case <-deadline.C:
			return // client reconnects with a fresh stream
		case <-heartbeat.C:
			if _, err := fmt.Fprint(c.Writer, ": ping\n\n"); err != nil {
				return
			}
			c.Writer.Flush()
		case <-ticker.C:
			cur := progressFrom(computeSetupChecklist(db, storeID, tenantID))
			if cur.Checklist == last.Checklist {
				continue
			}
			last = cur
			if !send(cur) {
				return
			}
			if cur.CompletedSteps == cur.TotalSteps {
				return // setup finished — let the client celebrate and close
			}
		}
	}
}

// wsUpgrader relies on the auth middleware chain (header-trust +
// RequireTenantRelation) for access control; the Origin header is not a
// trust boundary here because browsers cannot reach this endpoint
// without the proxy-injected identity headers.
var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  512,
	WriteBufferSize: 1024,
	CheckOrigin:     func(*http.Request) bool { return true },
}

// WS handles GET /admin/stores/:storeId/dashboard/setup-progress/ws,
// pushing the same SetupProgress JSON frames over a WebSocket. Same watch
// loop as Stream; the connection closes once setup completes.
func (h *SetupProgressHandler) WS(c *gin.Context) {
	storeID, tenantID, ok := h.ids(c)
	if !ok {
		return
	}

	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return // Upgrade already wrote the HTTP error
	}
	defer conn.Close()

	// Reader pump: we never expect client frames, but reading is required
	// to process control frames and notice the peer going away.
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn.SetReadLimit(512)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	writeJSON := func(p SetupProgress) bool {
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return conn.WriteJSON(p) == nil
	}

	db := h.db.WithContext(c.Request.Context())
	last := progressFrom(computeSetupChecklist(db, storeID, tenantID))
	if !writeJSON(last) {
		return
	}
	if last.CompletedSteps == last.TotalSteps {
		return
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()
	deadline := time.NewTimer(maxStreamAge)
	defer deadline.Stop()

	for {
		select {
		case <-done:
			return
		case <-c.Request.Context().Done():
			return
		case <-deadline.C:
			return
		case <-heartbeat.C:
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-ticker.C:
			cur := progressFrom(computeSetupChecklist(db, storeID, tenantID))
			if cur.Checklist == last.Checklist {
				continue
			}
			last = cur
			if !writeJSON(cur) {
				return
			}
			if cur.CompletedSteps == cur.TotalSteps {
				return
			}
		}
	}
}
