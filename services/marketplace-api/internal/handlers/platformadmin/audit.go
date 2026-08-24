package platformadmin

import (
	"errors"
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/audit"
)

// ErrMissingTenant is returned by EmitOperatorAction when the caller passes
// uuid.Nil for tenantID.
var ErrMissingTenant = errors.New("platformadmin: EmitOperatorAction requires a non-nil tenant ID")

// EmitOperatorAction records a platform-operator action against a tenant.
//
// The tenant is a REQUIRED parameter rather than something pulled from the
// gin context, because nothing on this surface sets `tenant_id` — see #310.
// audit.Emit would otherwise silently drop the event.
//
// Returns an error when the tenant is missing, so a caller cannot ignore
// the failure the way it can ignore a dropped Emit.
func EmitOperatorAction(c *gin.Context, em *audit.Emitter, tenantID uuid.UUID, ev audit.Event) error {
	if tenantID == uuid.Nil {
		return ErrMissingTenant
	}
	if em == nil {
		// Still safe to call — matching Emit's own nil-receiver tolerance —
		// but never silent: a nil emitter here means SOME write path is
		// producing an audit-worthy event with no way to record it, and
		// that must show up in logs even when this specific caller (e.g.
		// wiring that opted out of auditing entirely) has no *slog.Logger
		// of its own to hand in. slog.Default() rather than a threaded
		// logger param, so this stays a drop-in replacement for every
		// existing call site.
		slog.Default().Warn("platformadmin: EmitOperatorAction called with a nil emitter, event dropped",
			"action", ev.Action, "resource_type", ev.ResourceType, "tenant_id", tenantID)
		return nil
	}

	ev.TenantID = tenantID
	em.Emit(c, ev)
	return nil
}
