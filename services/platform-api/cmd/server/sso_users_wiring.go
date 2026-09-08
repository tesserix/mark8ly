package main

import (
	"github.com/gin-gonic/gin"

	"github.com/mark8ly/platform-api/internal/authz"
	"github.com/mark8ly/platform-api/internal/routes"
	"github.com/mark8ly/platform-api/internal/ssousers"
	"github.com/mark8ly/platform-api/internal/zitadeladmin"
)

// ssoUsersRegistrar wires the SSO identity routes marketplace-api's JIT
// provisioning calls (mark8ly#820), or returns nil when this deployment cannot
// serve them.
//
// Nil rather than a handler that 503s, because MountInternal skips a nil
// registrar and the route is then genuinely absent — which is the honest
// answer for a deployment with no Zitadel configured. A mounted route that
// always fails would look like an outage instead of a configuration.
//
// Both dependencies are required and both can legitimately be absent:
// staffProvisioner is nil unless ZITADEL_ENABLED, and fga is nil when OpenFGA
// is unconfigured. Provisioning needs both — an account nobody can grant a
// role to is not a provisioned user — so either being nil means no route.
func ssoUsersRegistrar(provisioner *zitadeladmin.StaffProvisioner, fga authz.Client) routes.Registrar {
	// A typed nil would satisfy the interface and panic on first use, the
	// #288 shape; compared against the concrete type before it is widened.
	if provisioner == nil || fga == nil {
		return nil
	}
	h := ssousers.NewHandler(ssousers.NewService(provisioner, fga))
	return func(g *gin.RouterGroup) { h.Register(g) }
}
