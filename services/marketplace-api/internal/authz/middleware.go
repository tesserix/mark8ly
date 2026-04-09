package authz

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Middleware bundles a Client and a logger so each route's
// RequireTenantRelation call doesn't need to thread them through.
type Middleware struct {
	client Client
	logger *slog.Logger
}

// NewMiddleware constructs a Middleware. logger may be nil.
func NewMiddleware(c Client, logger *slog.Logger) *Middleware {
	return &Middleware{client: c, logger: logger}
}

// RequireTenantRelation returns a gin.HandlerFunc that aborts with 404
// unless the caller (identified by user_id in the gin context) holds the
// given role on the tenant (identified by tenant_id in the gin context).
//
// Per spec §13.1.1 the response on deny is 404 not_found, not 403, to
// prevent existence leaks across tenants.
//
// The middleware depends on upstream middleware (auth + tenant) having
// populated the user_id and tenant_id keys on the gin context. M5 wires
// that upstream chain. For tests, set the keys directly via c.Set.
func (m *Middleware) RequireTenantRelation(role Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		tenantID := c.GetString("tenant_id")
		if userID == "" || tenantID == "" {
			respondNotFound(c)
			return
		}
		ok, err := m.client.Check(c.Request.Context(), userID, string(role), tenantID)
		if err != nil {
			if m.logger != nil {
				m.logger.Error("authz check failed",
					"user_id", userID, "tenant_id", tenantID, "role", role, "err", err)
			}
			respondInternal(c)
			return
		}
		if !ok {
			respondNotFound(c)
			return
		}
		c.Next()
	}
}

func respondNotFound(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusNotFound, map[string]any{
		"error":   string(apperrors.CodeNotFound),
		"message": "not found",
	})
}

func respondInternal(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]any{
		"error":   "internal",
		"message": "internal server error",
	})
}
