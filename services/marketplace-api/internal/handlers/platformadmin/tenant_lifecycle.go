// services/marketplace-api/internal/handlers/platformadmin/tenant_lifecycle.go
package platformadmin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/tenantlifecycle"
)

// TenantLifecycle is the subset of tenantlifecycle.Client this handler
// needs, declared here (not the concrete client type) so the handler is
// stubbable — the same reason EstateCounts and OnboardingFunnel are
// declared locally rather than importing their client packages' types.
type TenantLifecycle interface {
	Suspend(ctx context.Context, tenantID string) (*tenantlifecycle.Result, error)
	Unsuspend(ctx context.Context, tenantID string) (*tenantlifecycle.Result, error)
}

// SuspendReasonCodes is the closed set of reasons a tenant may be
// suspended for. An audit row saying WHAT happened without WHY is the gap
// this series exists to close (#287), so the code is REQUIRED; free text
// (`reason`) is accepted IN ADDITION, never instead.
var SuspendReasonCodes = []string{
	"abuse",         // abusive content or behaviour toward customers or staff
	"fraud",         // suspected fraudulent transactions or identity
	"non_payment",   // billing failed and the dunning ladder is exhausted
	"legal",         // legal or regulatory demand
	"tos_violation", // terms breach not covered by abuse or fraud
	"security",      // compromised account or active security incident
	"voluntary",     // merchant asked for the store to be paused
}

// UnsuspendReasonCodes is deliberately a different set from
// SuspendReasonCodes: the reasons for lifting a suspension are not the
// reasons for applying one.
var UnsuspendReasonCodes = []string{
	"resolved", "appeal_upheld", "operator_error", "voluntary_end",
}

// lifecycleAuditFunc records a platform-operator action against a tenant.
// In production this closes over a real *audit.Emitter via
// EmitOperatorAction; test doubles capture the raw audit.Event
// synchronously, which the real Emitter cannot do since its write happens
// on an async worker goroutine (see audit.Emitter.Emit).
type lifecycleAuditFunc func(c *gin.Context, tenantID uuid.UUID, ev audit.Event) error

// NewOperatorActionAuditFunc adapts a real *audit.Emitter into a
// lifecycleAuditFunc via EmitOperatorAction. em may be nil at the type
// level — EmitOperatorAction tolerates it — but Register (routes.go)
// never actually calls this with a nil em for the tenant-lifecycle
// routes: it mounts them only when deps.Emitter != nil in the first
// place, precisely so this adapter is never the thing standing between a
// write endpoint and an unaudited one.
func NewOperatorActionAuditFunc(em *audit.Emitter) lifecycleAuditFunc {
	return func(c *gin.Context, tenantID uuid.UUID, ev audit.Event) error {
		return EmitOperatorAction(c, em, tenantID, ev)
	}
}

// TenantLifecycleHandler serves the platform console's tenant
// suspend/unsuspend endpoints (#287) — the surface's first WRITE
// endpoints.
//
// The local `stores` projection update on a changed call is deliberately
// ASYMMETRIC:
//
//   - Suspend eagerly flips the tenant's local ACTIVE stores to suspended.
//     Over-enforcing briefly (serving a store as suspended a beat before
//     platform-api's own cascade finishes) is the safe direction, and it
//     means enforcement does not wait out StoreMiddleware's FreshTTL.
//
//   - Unsuspend does NOT eagerly flip stores back to active. This
//     projection has no column distinguishing a store suspended by the
//     tenant-level cascade from one suspended individually in
//     platform-api (that flag, suspended_by_tenant, exists only upstream).
//     An eager local unsuspend would reactivate an individually-suspended
//     store here, under-enforcing — the exact failure this endpoint exists
//     to prevent. Instead it marks the tenant's local rows stale, forcing
//     the next request through StoreMiddleware's refresh path so the
//     authoritative status is refetched from platform-api. Unsuspend takes
//     effect on the next refresh, not instantly — that is deliberate.
type TenantLifecycleHandler struct {
	client     TenantLifecycle
	stores     stores.Repository
	emit       lifecycleAuditFunc
	invalidate TenantGateInvalidator
	logger     *slog.Logger
}

// NewTenantLifecycleHandler constructs the handler. logger may be nil;
// storeRepo may be nil, in which case the local projection update is
// skipped (logged, not failed) after a changed call. emit may be nil, in
// which case it defaults to a no-op — that default exists for direct,
// low-level callers (this constructor has no way to know WHY emit is
// nil), but it is not where this surface's auditing guarantee actually
// lives.
//
// The real guard is in Register: a handler that cannot audit must not
// exist on this surface at all, so Register mounts these two routes only
// when deps.Emitter != nil (see routes.go), rather than mounting a live
// write endpoint whose audit trail silently no-ops. A construction-time
// panic here was tried and rejected — production never passes a literal
// nil (NewOperatorActionAuditFunc(deps.Emitter) always returns a non-nil
// closure, even when deps.Emitter itself is nil), so the panic could
// never fire on the one path that would actually need it; the unmounted
// route is what closes the loophole.
// invalidator may be nil: an unwired invalidator leaves today's TTL-lag
// behaviour in place on the admin gate (see TenantGateInvalidator's doc in
// routes.go) rather than failing the request — it is never required the
// way emit's audit trail is.
func NewTenantLifecycleHandler(client TenantLifecycle, storeRepo stores.Repository, emit lifecycleAuditFunc, invalidator TenantGateInvalidator, logger *slog.Logger) *TenantLifecycleHandler {
	if emit == nil {
		emit = func(*gin.Context, uuid.UUID, audit.Event) error { return nil }
	}
	return &TenantLifecycleHandler{client: client, stores: storeRepo, emit: emit, invalidate: invalidator, logger: logger}
}

// Register mounts both routes on the supplied group.
//
// These MUST stay on the platformadmin group (see the long doc comment on
// Register in routes.go) — the merchant admin tree already registers
// /admin/tenants/:tenantId/... under a different wildcard name at the same
// path position, and gin panics at router build time on that collision.
func (h *TenantLifecycleHandler) Register(g *gin.RouterGroup) {
	g.POST("/admin/tenants/:id/suspend", h.suspend)
	g.POST("/admin/tenants/:id/unsuspend", h.unsuspend)
}

// lifecycleRequest is the shared wire shape for both routes.
type lifecycleRequest struct {
	ReasonCode string `json:"reason_code"`
	Reason     string `json:"reason"`
}

// lifecycleResult is the projected upstream result. It is a NEW type, not
// tenantlifecycle.Result passed through: the upstream shape is an
// implementation detail of the client package, and pinning our own wire
// type here keeps the console's contract from silently drifting if that
// package's struct ever grows a field this surface shouldn't expose.
type lifecycleResult struct {
	TenantID       string `json:"tenant_id"`
	Status         string `json:"status"`
	StoresAffected int    `json:"stores_affected"`
	Changed        bool   `json:"changed"`
}

func (h *TenantLifecycleHandler) suspend(c *gin.Context) {
	h.handle(c, "suspend", SuspendReasonCodes, "tenant.suspended", h.client.Suspend, h.suspendActiveForTenant)
}

func (h *TenantLifecycleHandler) unsuspend(c *gin.Context) {
	h.handle(c, "unsuspend", UnsuspendReasonCodes, "tenant.unsuspended", h.client.Unsuspend, h.markStaleForTenant)
}

// suspendActiveForTenant and markStaleForTenant are nil-safe wrappers
// around h.stores's methods: h.stores is an interface, and forming a
// method value directly on a nil interface (h.stores.SuspendActiveForTenant)
// panics at the point the value is formed, not lazily at call time — these
// wrappers keep the nil check (see handle's own belt-and-braces
// `h.stores != nil` guard around the call site) from being load-bearing on
// its own.
func (h *TenantLifecycleHandler) suspendActiveForTenant(ctx context.Context, tenantID string) error {
	if h.stores == nil {
		return nil
	}
	return h.stores.SuspendActiveForTenant(ctx, tenantID)
}

func (h *TenantLifecycleHandler) markStaleForTenant(ctx context.Context, tenantID string) error {
	if h.stores == nil {
		return nil
	}
	return h.stores.MarkStaleForTenant(ctx, tenantID)
}

// handle runs the shared validate -> call upstream -> map errors ->
// (on Changed) update local projection + audit -> respond pipeline for
// both routes. action names the route for logging; reasonCodes is the
// closed set for THIS route (suspend and unsuspend use different sets);
// auditAction is the audit.Event.Action; call is the upstream client
// method; projectionUpdate is the local-projection side effect to run on a
// changed call — it differs between suspend (SuspendActiveForTenant) and
// unsuspend (MarkStaleForTenant), see the type doc for why.
func (h *TenantLifecycleHandler) handle(
	c *gin.Context,
	action string,
	reasonCodes []string,
	auditAction string,
	call func(ctx context.Context, tenantID string) (*tenantlifecycle.Result, error),
	projectionUpdate func(ctx context.Context, tenantID string) error,
) {
	tenantIDStr := c.Param("id")
	if _, err := uuid.Parse(tenantIDStr); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_tenant_id", "message": "id must be a UUID", "field": "id",
		})
		return
	}

	var req lifecycleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// gin's JSON binder returns io.EOF for a completely empty body, so
		// an omitted body is rejected HERE as invalid_request — it never
		// reaches the reason-code check below. `{}` (valid JSON, all
		// fields absent) DOES bind successfully to the zero value and is
		// the case the reason-code check exists to catch.
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid_request", "message": "request body could not be parsed",
		})
		return
	}

	if !isKnownReasonCode(req.ReasonCode, reasonCodes) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_reason_code",
			"message": "reason_code is required and must be one of the declared codes",
			"field":   "reason_code",
			"allowed": reasonCodes,
		})
		return
	}

	res, err := call(c.Request.Context(), tenantIDStr)
	if err != nil {
		h.respondUpstreamErr(c, action, err)
		return
	}

	if res.Changed {
		// #287 fix-round-1: drop the admin gate's cached status for this
		// tenant so the suspend/unsuspend takes effect on the very next
		// admin request, instead of lagging behind by up to the gate's
		// TTL. Best-effort like projectionUpdate below: nil-safe, and its
		// absence is a degraded-lag, not a failure worth surfacing to the
		// caller — the upstream write already succeeded.
		if h.invalidate != nil {
			h.invalidate.Invalidate(tenantIDStr)
		}

		if err := projectionUpdate(c.Request.Context(), tenantIDStr); err != nil {
			// The upstream call already SUCCEEDED. A projection-update
			// failure must not turn that into an error response — log
			// loudly and let the response reflect the real outcome. The
			// local projection will still catch up: suspend's worst case
			// is a brief window of under-enforcement (same as before this
			// endpoint existed), and unsuspend never enforces off local
			// rows without a refresh anyway.
			if h.logger != nil {
				h.logger.Error("tenant lifecycle: local projection update failed",
					"action", action, "tenant_id", tenantIDStr, "err", err)
			}
		}

		tenantUUID, _ := uuid.Parse(tenantIDStr) // already validated above
		if err := h.emit(c, tenantUUID, audit.Event{
			Action:       auditAction,
			ResourceType: "tenant",
			ResourceID:   tenantIDStr,
			Metadata: map[string]any{
				"reason_code":     req.ReasonCode,
				"reason":          req.Reason,
				"stores_affected": res.StoresAffected,
				"capability":      c.GetString(CtxCapability),
			},
		}); err != nil {
			// ErrMissingTenant (or any other emit failure): the operation
			// SUCCEEDED upstream but we cannot attribute it. Log loudly;
			// do not fail the response, and do not pretend it was
			// attributed.
			if h.logger != nil {
				h.logger.Error("tenant lifecycle: action succeeded but was not audited",
					"action", action, "tenant_id", tenantIDStr, "err", err)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"data": lifecycleResult{
		TenantID:       res.TenantID,
		Status:         res.Status,
		StoresAffected: res.StoresAffected,
		Changed:        res.Changed,
	}})
}

// respondUpstreamErr maps tenantlifecycle's sentinels to this surface's
// stable HTTP codes. An unavailable upstream must never read as "nothing
// to do" — see tenantlifecycle's own package doc for the failure mode this
// guards against.
func (h *TenantLifecycleHandler) respondUpstreamErr(c *gin.Context, action string, err error) {
	switch {
	case errors.Is(err, tenantlifecycle.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"error": "not_found", "message": "tenant not found",
		})
	case errors.Is(err, tenantlifecycle.ErrConflict):
		c.JSON(http.StatusConflict, gin.H{
			"error": "invalid_status_transition", "message": "tenant cannot transition to the requested status",
		})
	case errors.Is(err, tenantlifecycle.ErrUnavailable):
		if h.logger != nil {
			h.logger.Error("tenant lifecycle upstream unavailable", "action", action, "err", err)
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "upstream_unavailable", "message": "platform-api is unavailable",
		})
	default:
		if h.logger != nil {
			h.logger.Error("tenant lifecycle", "action", action, "err", err)
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal_error", "message": "could not update tenant status",
		})
	}
}

// isKnownReasonCode returns true only for an exact, non-empty membership
// match. Never coerced, never a fallback to free text.
func isKnownReasonCode(code string, allowed []string) bool {
	if code == "" {
		return false
	}
	for _, c := range allowed {
		if c == code {
			return true
		}
	}
	return false
}
