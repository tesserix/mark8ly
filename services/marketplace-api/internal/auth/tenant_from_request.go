package auth

import (
	"context"
	"log/slog"

	"github.com/gin-gonic/gin"
)

// ActingTenantHeader carries the tenant the client states it wants to act
// as, for mobile admin routes that are not store-scoped (so there is no
// tenant in the path) and therefore cannot rely on a Zitadel/GIP token
// claim to carry it either — Zitadel does not mint the old custom
// tenant_id claim, and FGA membership is a list, not a single value, so
// the client has to say which one it means.
//
// This is deliberately NOT X-Tenant-Id: that header name is already load-
// bearing trust in internal/auth/middleware.go's HeaderTrustAuth, where it
// is populated by Istio's request-authentication filter from a verified
// JWT and treated as authoritative with no further check. A client-stated
// value must never be confused with that trusted value, so it gets its
// own name and its own (weaker, FGA-checked) trust level.
const ActingTenantHeader = "X-Acting-Tenant-Id"

// TenantMembershipChecker is the narrow slice of authz.Client this
// middleware depends on, so it can take an interface rather than the
// concrete FGA client (or a second one constructed just for this).
type TenantMembershipChecker interface {
	CheckMembership(ctx context.Context, userID, tenantID string) (bool, error)
}

// TenantFromRequest returns gin middleware, mounted after bearer auth,
// that reads the caller's stated tenant from ActingTenantHeader and, only
// if FGA confirms the caller (user_id, set by GIPBearerAuth) is a member
// of that tenant, sets tenant_id on the gin context.
//
// It NEVER aborts. A missing header, a non-member, or an FGA error all
// leave tenant_id untouched (empty) and fall through via c.Next() exactly
// the same way. RequireBoundTenant (require_bound_tenant.go) is what
// turns an empty tenant_id into a response — and it does so as 404, never
// 401, because a 401 here would make the mobile client call signOut() and
// bounce a validly authenticated user to /login. Authentication (who is
// this) and authorization (what may they act as) are deliberately kept
// as separate questions with separate failure paths; this middleware only
// ever answers the second one, and only ever by omission.
//
// An FGA error fails closed: the header is not trusted just because the
// check couldn't be completed, and the request is never trusted with an
// unvalidated tenant.
func TenantFromRequest(checker TenantMembershipChecker, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := c.GetHeader(ActingTenantHeader)
		if tenantID == "" {
			c.Next()
			return
		}

		userID := c.GetString("user_id")
		if userID == "" {
			c.Next()
			return
		}

		isMember, err := checker.CheckMembership(c.Request.Context(), userID, tenantID)
		if err != nil {
			if logger != nil {
				logger.Error("tenant_from_request: fga membership check failed",
					"user_id", userID, "tenant_id", tenantID, "err", err)
			}
			c.Next()
			return
		}
		if !isMember {
			c.Next()
			return
		}

		c.Set("tenant_id", tenantID)
		c.Next()
	}
}
