package main

import (
	"log/slog"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/authbffclient"
	"github.com/mark8ly/marketplace-api/internal/bao"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/handlers/public"
	"github.com/mark8ly/marketplace-api/internal/sso"
	"github.com/mark8ly/marketplace-api/internal/ssousers"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/config"
)

// wiring_sso.go — per-tenant SSO (#820).
//
// Both surfaces are built here, together, or neither is. Letting a merchant
// SAVE an IdP configuration that nobody can then sign in with is the
// "looks delivered, does nothing" state #820 exists to remove, and mounting
// the login route without an identity source is worse: it would take the
// merchant through their IdP and fail at the last step.

// ssoDeps is what building the SSO surfaces needs from main.
type ssoDeps struct {
	cfg      *config.Config
	repo     *sso.Repository
	stores   stores.Repository
	platform stores.Client
	bao      *bao.Client
	audit    *audit.Emitter
	log      *slog.Logger
}

// buildSSO returns the config handler and the login handler, or (nil, nil)
// when this deployment cannot serve SSO.
//
// Nil rather than handlers that fail: the route registrations are guarded on
// nil, so an unconfigured deployment simply has no SSO routes — which is the
// honest answer, and distinguishable from a broken one.
//
// The preconditions are the ones without which a login CANNOT complete:
//
//   - platform-api, which is the only thing that can say who an email is
//     (marketplace-api has no user identity store at all — see
//     internal/ssousers);
//   - OpenBao, which holds every tenant's OIDC client secret;
//   - auth-bff, which mints the session at the end of a successful callback.
//
// A missing one is logged at Error, not silently skipped: an operator who
// configured SSO and finds no routes deserves to be told which piece is
// absent.
func buildSSO(d ssoDeps) (*admin.SSOConfigHandler, *public.SSOLoginHandler) {
	switch {
	case d.cfg.PlatformAPIURL == "":
		d.log.Error("sso: MARKETPLACE_PLATFORM_API_URL is unset — SSO routes stay unmounted; " +
			"identity resolution needs platform-api")
		return nil, nil
	case d.bao == nil:
		d.log.Error("sso: no OpenBao client — SSO routes stay unmounted; " +
			"tenant OIDC client secrets live there")
		return nil, nil
	case d.cfg.AuthBFFURL == "":
		d.log.Error("sso: AUTH_BFF_URL is unset — SSO routes stay unmounted; " +
			"a successful callback has nowhere to mint a session")
		return nil, nil
	}

	// The relying-party cache reads client secrets straight from OpenBao at
	// the path each tenant's config names. It builds per tenant on first use,
	// so one customer's unreachable IdP cannot delay startup or affect
	// another tenant (see sso/rp_cache.go).
	rps := sso.NewRelyingPartyCache(d.bao, 0)

	users := ssousers.NewClient(d.cfg.PlatformAPIURL, d.cfg.PlatformAPISecret, nil)
	jit := sso.NewJITProvisioner(users, d.repo)

	// The tenant resolver shares the slug cache's shape but not its instance:
	// StoreMiddleware's cache is built for the admin request path, and an
	// unauthenticated login route should not be able to evict entries that
	// signed-in traffic depends on.
	slugs := stores.NewSlugCache(d.stores, d.platform, &singleflight.Group{}, 5*time.Minute)

	login := public.NewSSOLoginHandler(
		d.repo,
		jit,
		authbffclient.NewSessionIssuer(d.cfg.AuthBFFURL, d.cfg.InternalAuthSecret, nil),
		rps,
		public.NewMemStateCache(),
		d.audit,
		public.NewStoreSlugTenantResolver(slugs),
		d.log,
	)

	// The config handler gets the SAME cache the login path uses, so
	// "Test connection" exercises the exact object a login would build —
	// discovery against the tenant's issuer and the client secret out of
	// OpenBao — rather than re-running the validation that just accepted the
	// save. It invalidates before testing, so it tests the config as edited.
	config := admin.NewSSOConfigHandler(d.repo, d.audit, d.log).WithRelyingParties(rps)

	return config, login
}
