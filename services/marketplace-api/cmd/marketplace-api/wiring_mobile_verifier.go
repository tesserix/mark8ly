package main

import (
	"context"
	"log/slog"

	"github.com/mark8ly/marketplace-api/internal/auth"
	"github.com/mark8ly/marketplace-api/pkg/config"
)

// selectMobileTokenVerifier chooses the bearer-token verifier mounted on
// the mobile admin route group: Zitadel when ZITADEL_ENABLED=true,
// otherwise the incumbent GIP/Firebase verifier — mirroring
// RegisterAdminMobile's existing "nil TokenVerifier disables the whole
// group" contract for whichever provider is selected.
//
// cfg.ZitadelEnabled=true reaching this function already implies
// cfg.ZitadelIssuer and cfg.ZitadelAdminProjectID are non-empty:
// config.Load calls Config.ValidateZitadel unconditionally (every
// environment, not just non-dev), so a misconfigured flag panics main()
// at boot — see pkg/config/config.go — rather than falling through to
// this selection logic disabled or half-mounted. What newZitadel can
// still fail on here is a genuine runtime problem (e.g. the issuer's
// discovery endpoint unreachable at boot), and that failure disables
// mobile admin routes exactly like a Firebase app-init failure already
// does for GIP — it does NOT fall back to newGIP. Falling back would let
// a broken Zitadel deployment silently keep running on GIP, masking the
// very misconfiguration operators need to see.
//
// newGIP is invoked only when Zitadel is not selected, so a Zitadel
// deployment never touches Firebase at all. It returns (nil, nil) — not
// an error — for the pre-existing "no GIP config" case (empty
// GIP_PROJECT_ID), so that silent, log-free disablement is preserved
// byte-for-byte when ZITADEL_ENABLED is unset (the default).
//
// Caution: both error branches below only log.Error and return nil — there
// is no alert wired on either line. If Zitadel discovery (or Firebase
// init) starts failing after a clean boot, mobile admin auth silently goes
// dark: RegisterAdminMobile sees a nil TokenVerifier and returns without
// mounting any routes, with nothing surfacing beyond this log line. This
// matches the incumbent GIP/Firebase behaviour on purpose (see the doc
// comment above), but it means an operator only finds out from a support
// ticket, not a page. Alerting on this log line is worth doing; it has not
// been done here.
func selectMobileTokenVerifier(
	ctx context.Context,
	cfg *config.Config,
	log *slog.Logger,
	newZitadel func(ctx context.Context, issuer, audience string) (auth.TokenVerifier, error),
	newGIP func() (auth.TokenVerifier, error),
) auth.TokenVerifier {
	if cfg.ZitadelEnabled {
		v, err := newZitadel(ctx, cfg.ZitadelIssuer, cfg.ZitadelAdminProjectID)
		if err != nil {
			log.Error("zitadel: verifier construction failed, mobile admin routes disabled", "error", err)
			return nil
		}
		return v
	}

	v, err := newGIP()
	if err != nil {
		log.Error("gip: verifier construction failed, mobile admin routes disabled", "error", err)
		return nil
	}
	return v
}
