// services/marketplace-api/internal/handlers/platformadmin/tenant_purge.go
package platformadmin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/tenantdirectory"
	"github.com/mark8ly/marketplace-api/internal/tenantlifecycle"
	"github.com/mark8ly/marketplace-api/internal/tenantpurge"
)

// PurgeReasonCodes is the closed set of reasons a tenant may be purged
// for. Deliberately a different set from SuspendReasonCodes and from
// ExtendReasonCodes: the reasons for destroying a tenant are not the
// reasons for pausing one.
//
// merchant_request and erasure_request are kept distinct because only the
// second carries a statutory clock, and an audit trail that cannot tell
// them apart cannot answer the question a regulator asks.
var PurgeReasonCodes = []string{
	"merchant_request", // the merchant asked for their account and data to be deleted
	"erasure_request",  // a statutory erasure demand (GDPR art.17) — see #259
	"fraud",            // confirmed fraudulent tenant, removed after investigation
	"abandoned",        // onboarding never completed; a dormant tenant reclaimed
	"legal",            // a legal or regulatory demand other than erasure
	"operator_error",   // a tenant created in error, or a test tenant
}

// maxReasonRunes caps the free-text reason. Counted in RUNES, not bytes: a
// byte-truncated multibyte string is invalid UTF-8, Postgres rejects the
// jsonb, and the audit emit fails — which on this endpoint would mean an
// irreversible destruction recorded nowhere.
const maxReasonRunes = 500

// operatorAuditFunc records a platform-operator action SYNCHRONOUSLY.
// Mirrors #287's lifecycleAuditFunc: test doubles capture the raw
// audit.Event, which the real *audit.Emitter cannot be made to do for its
// async Emit. Purge uses EmitSync instead — see NewOperatorActionSyncFunc.
type operatorAuditFunc func(c *gin.Context, tenantID uuid.UUID, ev audit.Event) error

// NewOperatorActionSyncFunc adapts a real *audit.Emitter into an
// operatorAuditFunc via EmitSync, mirroring
// tenant_lifecycle.go's NewOperatorActionAuditFunc except that it emits
// SYNCHRONOUSLY and reports the write's own outcome — see EmitSync's doc
// for why an async Emit is unsafe on this specific endpoint (purgePlan
// deletes audit_logs WHERE tenant_id = ?).
func NewOperatorActionSyncFunc(em *audit.Emitter) operatorAuditFunc {
	return func(c *gin.Context, tenantID uuid.UUID, ev audit.Event) error {
		if tenantID == uuid.Nil {
			return ErrMissingTenant
		}
		ev.TenantID = tenantID
		return em.EmitSync(c, ev)
	}
}

// TenantTeardown is the subset of tenantlifecycle.Client this handler
// needs, declared locally so the handler is stubbable.
type TenantTeardown interface {
	Teardown(ctx context.Context, tenantID string, storeSlugs []string) (*tenantlifecycle.TeardownResult, error)
}

// Purger is tenantpurge's two entry points, declared locally for the same
// reason.
type Purger interface {
	Purge(ctx context.Context, tenantID string, storeIDs []string) (tenantpurge.Report, error)
	Count(ctx context.Context, tenantID string, storeIDs []string) (tenantpurge.Report, error)
}

// TenantPurgeHandler serves the platform console's tenant purge and purge
// preview endpoints (#288) — the surface's first IRREVERSIBLE write.
//
// The purge handler's order is the whole design: teardown (upstream,
// confirms and deletes the tenant row) -> purge (local, inline destruction
// report) -> gate invalidation (best-effort) -> audit (LAST, SYNCHRONOUS).
// purgePlan contains `DELETE FROM audit_logs WHERE tenant_id = ?`, so an
// audit row written before the purge is destroyed BY the purge, and an
// async write races it. See purge() below for the full sequencing
// rationale inline.
type TenantPurgeHandler struct {
	teardown   TenantTeardown
	purger     Purger
	dir        TenantDirectory
	emit       operatorAuditFunc
	invalidate TenantGateInvalidator
	logger     *slog.Logger
}

// NewTenantPurgeHandler constructs the handler. emit may be nil, in which
// case it defaults to a no-op — matching NewTenantLifecycleHandler's
// pattern. The real guard against mounting an unattributable write
// endpoint belongs in Register (routes.go), not here, exactly as it does
// for TenantLifecycle. logger may be nil.
func NewTenantPurgeHandler(
	teardown TenantTeardown,
	purger Purger,
	dir TenantDirectory,
	emit operatorAuditFunc,
	invalidator TenantGateInvalidator,
	logger *slog.Logger,
) *TenantPurgeHandler {
	if emit == nil {
		emit = func(*gin.Context, uuid.UUID, audit.Event) error { return nil }
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &TenantPurgeHandler{
		teardown:   teardown,
		purger:     purger,
		dir:        dir,
		emit:       emit,
		invalidate: invalidator,
		logger:     logger,
	}
}

// Register mounts both routes on the supplied group.
//
// These MUST stay on the platformadmin group. The merchant admin tree
// registers /admin/tenants/:tenantId/... under a DIFFERENT wildcard name
// at this same path position, and two wildcard names at one position
// panic gin at router build time. :id here matches suspend/unsuspend,
// already mounted on this group (tenant_lifecycle.go).
func (h *TenantPurgeHandler) Register(g *gin.RouterGroup) {
	g.GET("/admin/tenants/:id/purge/preview", h.preview)
	g.POST("/admin/tenants/:id/purge", h.purge)
}

// purgeRequest is the wire body.
//
// ABSENT and EMPTY store_slugs mean different things and stay distinguishable
// all the way down: absent is a client that dropped the confirmation and must
// fail; empty asserts the tenant has no stores and must reach the check.
//
// The POINTER is not what makes that work — encoding/json already leaves a
// plain []string nil for `{}` and non-nil for `[]`. It is here to state the
// requirement in the type, and to avoid depending on a JSON library's
// nil-vs-empty slice convention.
type purgeRequest struct {
	StoreSlugs *[]string `json:"store_slugs"`
	ReasonCode string    `json:"reason_code"`
	Reason     string    `json:"reason"`
}

// purgeResponse is the POST's `data` payload. Six fields always present;
// `reason` is the only omitempty.
type purgeResponse struct {
	TenantID   string                    `json:"tenant_id"`
	TenantName string                    `json:"tenant_name"`
	StoreIDs   []string                  `json:"store_ids"`
	StoreSlugs []string                  `json:"store_slugs"`
	ReasonCode string                    `json:"reason_code"`
	Reason     string                    `json:"reason,omitempty"`
	Tables     []tenantpurge.TableResult `json:"tables"`
	TotalRows  int64                     `json:"total_rows"`
	PurgedAt   string                    `json:"purged_at"`
}

// previewResponse is the GET's `data` payload.
type previewResponse struct {
	TenantID   string                    `json:"tenant_id"`
	TenantName string                    `json:"tenant_name"`
	Status     string                    `json:"status"`
	StoreSlugs []string                  `json:"store_slugs"`
	Tables     []tenantpurge.TableResult `json:"tables"`
	TotalRows  int64                     `json:"total_rows"`
}

// purge is POST /admin/tenants/:id/purge. IRREVERSIBLE.
func (h *TenantPurgeHandler) purge(c *gin.Context) {
	tenantIDStr := c.Param("id")
	tenantUUID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_tenant_id", "message": "id must be a UUID", "field": "id",
		})
		return
	}

	var req purgeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// gin's JSON binder returns io.EOF for a wholly empty body, so an
		// omitted body is rejected HERE and never reaches the store_slugs
		// or reason-code checks below. `{}` binds successfully to the
		// zero value (StoreSlugs stays nil) and is what the next check
		// catches.
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_request", "message": "request body could not be parsed",
		})
		return
	}

	if req.StoreSlugs == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": "store_slugs is required; send [] to confirm the tenant has no stores",
			"field":   "store_slugs",
		})
		return
	}

	if !isKnownReasonCode(req.ReasonCode, PurgeReasonCodes) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_reason_code",
			"message": "reason_code is required and must be one of the declared codes",
			"field":   "reason_code",
			"allowed": PurgeReasonCodes,
		})
		return
	}

	reason := capReasonRunes(req.Reason, maxReasonRunes)
	storeSlugs := *req.StoreSlugs

	// 1. Upstream teardown. Its transaction runs the confirmation check
	//    and deletes the tenant row; on return, the tenant.deleted outbox
	//    event guarantees the marketplace purge happens eventually
	//    whatever this request does next.
	res, err := h.teardown.Teardown(c.Request.Context(), tenantIDStr, storeSlugs)
	if err != nil {
		h.respondTeardownErr(c, err)
		return
	}

	// 2. Purge inline, for a real destruction report. The drainer is the
	//    backstop: if this fails, it retries, and Purge is idempotent.
	rep, purgeErr := h.purger.Purge(c.Request.Context(), tenantIDStr, res.StoreIDs)

	// 3. Drop the admin gate's cached status — without it the gate serves
	//    a cached status for up to its TTL for a tenant that no longer
	//    exists. Best-effort and nil-safe, matching #287.
	if h.invalidate != nil {
		h.invalidate.Invalidate(tenantIDStr)
	}

	purgedAt := time.Now().UTC()

	// 4. Audit LAST and SYNCHRONOUSLY. purgePlan contains
	//    DELETE FROM audit_logs WHERE tenant_id = ?, so a row written
	//    before step 2 is destroyed by step 2, and an async write races
	//    it. EmitSync after the purge transaction has committed is the
	//    only ordering that survives.
	auditErr := h.emit(c, tenantUUID, audit.Event{
		Action:       "tenant.purged",
		ResourceType: "tenant",
		ResourceID:   tenantIDStr,
		Severity:     audit.SeverityCritical,
		Metadata: map[string]any{
			"reason_code": req.ReasonCode,
			"reason":      reason,
			"store_slugs": storeSlugs,
			"store_ids":   res.StoreIDs,
			"tables":      rep.Tables,
			"total_rows":  rep.TotalRows,
			"capability":  c.GetString(CtxCapability),
		},
	})

	// Both purgeErr and auditErr are reported to the operator rather than
	// swallowed: the tenant row is already gone (teardown succeeded), so
	// a silent 200 would tell the operator a destruction completed and
	// was recorded when neither may be true.
	if purgeErr != nil {
		h.logger.Error("tenant purge: teardown succeeded but the local purge failed",
			"tenant_id", tenantIDStr, "err", purgeErr)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":     "purge_incomplete",
			"message":   "the tenant was torn down upstream but the local purge failed; the tenant.deleted outbox event will retry it",
			"tenant_id": tenantIDStr,
		})
		return
	}

	if auditErr != nil {
		h.logger.Error("tenant purge: succeeded but was not audited",
			"tenant_id", tenantIDStr, "err", auditErr)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":     "purge_unaudited",
			"message":   "the tenant was purged but the audit record could not be written",
			"tenant_id": tenantIDStr,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": purgeResponse{
		TenantID:   res.TenantID,
		TenantName: res.TenantName,
		StoreIDs:   res.StoreIDs,
		StoreSlugs: res.StoreSlugs,
		ReasonCode: req.ReasonCode,
		Reason:     reason,
		Tables:     rep.Tables,
		TotalRows:  rep.TotalRows,
		PurgedAt:   purgedAt.Format(time.RFC3339),
	}})
}

// preview is GET /admin/tenants/:id/purge/preview. Non-destructive: it
// must never call h.purger.Purge, only Count.
func (h *TenantPurgeHandler) preview(c *gin.Context) {
	tenantIDStr := c.Param("id")
	if _, err := uuid.Parse(tenantIDStr); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_tenant_id", "message": "id must be a UUID", "field": "id",
		})
		return
	}

	detail, err := h.dir.Get(c.Request.Context(), tenantIDStr)
	if err != nil {
		h.respondDirectoryErr(c, err)
		return
	}

	storeIDs := make([]string, 0, len(detail.Stores))
	storeSlugs := make([]string, 0, len(detail.Stores))
	for _, s := range detail.Stores {
		storeIDs = append(storeIDs, s.ID)
		storeSlugs = append(storeSlugs, s.Slug)
	}

	rep, err := h.purger.Count(c.Request.Context(), tenantIDStr, storeIDs)
	if err != nil {
		h.logger.Error("tenant purge preview: count failed", "tenant_id", tenantIDStr, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal_error", "message": "could not compute purge preview",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": previewResponse{
		TenantID:   detail.ID,
		TenantName: detail.Name,
		Status:     detail.Status,
		StoreSlugs: storeSlugs,
		Tables:     rep.Tables,
		TotalRows:  rep.TotalRows,
	}})
}

// respondTeardownErr maps tenantlifecycle.Teardown's sentinels to this
// surface's stable HTTP codes. A refusal here must purge NOTHING — the
// caller of purge() returns immediately after this, before h.purger.Purge
// is ever reached.
func (h *TenantPurgeHandler) respondTeardownErr(c *gin.Context, err error) {
	var mismatch *tenantlifecycle.ConfirmationMismatchError
	switch {
	case errors.As(err, &mismatch):
		c.JSON(http.StatusConflict, gin.H{
			"error":    "confirmation_mismatch",
			"message":  "store_slugs did not match the tenant's current stores",
			"expected": mismatch.Expected,
		})
	case errors.Is(err, tenantlifecycle.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"error": "tenant_not_found", "message": "tenant not found",
		})
	case errors.Is(err, tenantlifecycle.ErrUnavailable):
		h.logger.Error("tenant purge: teardown upstream unavailable", "err", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "upstream_unavailable", "message": "platform-api is unavailable",
		})
	default:
		h.logger.Error("tenant purge: teardown failed", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal_error", "message": "could not tear down tenant",
		})
	}
}

// respondDirectoryErr maps tenantdirectory.Get's sentinels for the preview
// route. An unreachable upstream must never read as "nothing to purge".
func (h *TenantPurgeHandler) respondDirectoryErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, tenantdirectory.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"error": "tenant_not_found", "message": "tenant not found",
		})
	case errors.Is(err, tenantdirectory.ErrUnavailable):
		h.logger.Error("tenant purge preview: directory upstream unavailable", "err", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "upstream_unavailable", "message": "platform-api is unavailable",
		})
	default:
		h.logger.Error("tenant purge preview: directory lookup failed", "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal_error", "message": "could not look up tenant",
		})
	}
}

// capReasonRunes caps s at maxRunes RUNES, not bytes. A byte cut through a
// multibyte character yields invalid UTF-8; Postgres rejects the jsonb
// write, and the audit emit fails — on this endpoint that would mean an
// irreversible destruction recorded nowhere. Slicing a []rune conversion
// only ever cuts at rune boundaries, so the result is always valid UTF-8.
func capReasonRunes(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	r := []rune(s)
	return string(r[:maxRunes])
}
