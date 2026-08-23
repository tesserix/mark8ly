package platformadmin

import (
	"errors"

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
		return nil // safe to call when wiring opted out, matching Emit's own nil-receiver tolerance
	}

	ev.TenantID = tenantID
	em.Emit(c, ev)
	return nil
}
