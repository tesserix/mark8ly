package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/plangate"
)

// breakGlassPlanResolver is the narrow slice of *plangate.PlanResolver this
// middleware needs. Declared as an interface (rather than depending on the
// concrete type) so tests can supply a fake that never touches a *gorm.DB —
// PlanResolver.ResolveByTenant runs a raw query against store_subscriptions,
// which a unit test has no business standing up a database for.
type breakGlassPlanResolver interface {
	ResolveByTenant(ctx context.Context, tenantID uuid.UUID) plangate.Plan
}

// breakGlassTenantIDPeek is the minimal shape this middleware needs out of
// the login request body — just enough to resolve a plan, nothing that
// would duplicate breakGlassLoginRequest's validation.
type breakGlassTenantIDPeek struct {
	TenantID string `json:"tenant_id"`
}

// RequireBreakGlassFeature returns middleware that 403s a break-glass login
// attempt for a tenant whose plan doesn't meet minPlan, mirroring
// plangate.RequirePlan's response shape.
//
// It cannot reuse RequirePlan as-is: that function reads tenant_id off the
// Gin context, which is only ever set by upstream auth middleware — and
// POST /admin/break-glass/login deliberately runs with NONE. This is the
// recovery path; requiring a working auth pipeline to reach it would defeat
// the point. The tenant id it needs instead travels in the JSON body
// (breakGlassLoginRequest.TenantID), exactly like every other input to this
// handler.
//
// So this middleware peeks the body: reads it fully, restores
// c.Request.Body so the handler's own c.ShouldBindJSON still sees the full
// payload, and — only if a well-formed UUID tenant_id was present —
// resolves that tenant's plan and gates on it.
//
// A missing or malformed tenant_id is deliberately NOT rejected here. The
// handler's own step 2 (shape validation) already turns that into the same
// uniform {"error":"invalid_credentials"} response the wrong-password path
// returns — matching this endpoint's documented invariant that response
// shape must never reveal which check failed. Duplicating that rejection
// here, with a differently-shaped error, would break that invariant for
// exactly the requests this middleware runs on.
func RequireBreakGlassFeature(resolver breakGlassPlanResolver, minPlan plangate.Plan, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID, ok := peekBreakGlassTenantID(c)
		if !ok {
			c.Next()
			return
		}

		plan := resolver.ResolveByTenant(c.Request.Context(), tenantID)
		if !plangate.PlanAtLeast(plan, minPlan) {
			c.JSON(http.StatusForbidden, gin.H{
				"error":    "plan_required",
				"message":  fmt.Sprintf("This feature requires the %s plan or higher", minPlan),
				"required": string(minPlan),
				"current":  string(plan),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// peekBreakGlassTenantID reads tenant_id out of the request body WITHOUT
// consuming it for downstream readers: c.Request.Body is drained into
// memory and then replaced with a fresh reader over the same bytes, so
// BreakGlassLoginHandler.Login's own c.ShouldBindJSON call still sees the
// complete body exactly as the client sent it.
func peekBreakGlassTenantID(c *gin.Context) (uuid.UUID, bool) {
	if c.Request == nil || c.Request.Body == nil {
		return uuid.Nil, false
	}

	raw, err := io.ReadAll(c.Request.Body)
	_ = c.Request.Body.Close()
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	if err != nil {
		return uuid.Nil, false
	}

	var peek breakGlassTenantIDPeek
	if err := json.Unmarshal(raw, &peek); err != nil {
		return uuid.Nil, false
	}

	tenantID, err := uuid.Parse(peek.TenantID)
	if err != nil {
		return uuid.Nil, false
	}
	return tenantID, true
}
