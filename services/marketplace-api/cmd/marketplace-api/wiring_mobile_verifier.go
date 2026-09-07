package main

import (
	"context"
	"log/slog"

	"github.com/mark8ly/marketplace-api/internal/auth"
	"github.com/mark8ly/marketplace-api/pkg/config"
)

// selectMobileTokenVerifier builds the bearer-token verifier mounted on
// the mobile admin route group. Since #786 there is exactly one issuer —
// Zitadel — so this is a construction step with a kill switch, not a
// choice between providers: the GIP/Firebase verifier and the
// dual-issuer composite are both gone, having been unreachable in
// production since ZITADEL_DUAL_ISSUER=false shipped.
//
// It honours RegisterAdminMobile's "nil TokenVerifier disables the whole
// group" contract in both of its non-happy paths:
//
//   - ZITADEL_ENABLED unset: nothing to verify tokens with, so mobile
//     admin routes stay unmounted. There is no fallback issuer left for
//     them to run on.
//   - newZitadel fails: a genuine runtime problem (e.g. the issuer's
//     discovery endpoint unreachable at boot) that config-time validation
//     cannot catch. Routes are disabled rather than half-mounted.
//
// cfg.ZitadelEnabled=true reaching this function already implies
// cfg.ZitadelIssuer and cfg.ZitadelAdminProjectID are non-empty:
// config.Load calls Config.ValidateZitadel unconditionally (every
// environment, not just non-dev), so a misconfigured flag panics main()
// at boot — see pkg/config/config.go — rather than falling through to
// this construction disabled or half-mounted.
//
// Caution: the error branch below only log.Error and returns nil — there
// is no alert wired on that line. If Zitadel discovery starts failing
// after a clean boot, mobile admin auth silently goes dark:
// RegisterAdminMobile sees a nil TokenVerifier and returns without
// mounting any routes, with nothing surfacing beyond this log line. An
// operator only finds out from a support ticket, not a page. Alerting on
// this log line is worth doing; it has not been done here.
func selectMobileTokenVerifier(
	ctx context.Context,
	cfg *config.Config,
	log *slog.Logger,
	newZitadel func(ctx context.Context, issuer, audience string) (auth.TokenVerifier, error),
) auth.TokenVerifier {
	if !cfg.ZitadelEnabled {
		log.Info("zitadel: ZITADEL_ENABLED is false; mobile admin routes disabled (no fallback issuer since #786)")
		return nil
	}

	v, err := newZitadel(ctx, cfg.ZitadelIssuer, cfg.ZitadelAdminProjectID)
	if err != nil {
		log.Error("zitadel: verifier construction failed, mobile admin routes disabled", "error", err)
		return nil
	}
	return v
}
