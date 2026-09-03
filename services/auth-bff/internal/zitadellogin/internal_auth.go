package zitadellogin

import (
	"net/http"

	"github.com/mark8ly/auth-bff/internal/internalauth"
)

// Both /auth/zitadel/{login,totp} and /auth/customer/{login,totp} take a
// {login_name, password} pair and answer whether it is valid. auth-bff is
// publicly reachable (the mark8ly-wildcard VirtualService routes
// auth.mark8ly.com on any path to it), the login-client PAT behind
// CreatePasswordSession is instance-level rather than org-scoped, and
// nothing in this service rate-limits. Left open, these four routes are a
// credential-validity oracle over every user in the Zitadel instance,
// merchant admins included.
//
// Every caller is server-side — apps/admin and apps/storefront reach them
// from server actions, never from the browser — so the fix is the same
// shared-secret scheme internal/session's /internal/users handler already
// uses inbound and internal/audit + internal/notify already use outbound:
// an X-Internal-Auth header compared in constant time
// (internal/internalauth).
//
// An empty secret here means "unchecked", NOT "reject everything": the
// guard is enforced at boot instead, where config.Config.ValidateZitadel
// refuses to start a Zitadel-enabled auth-bff whose internal secret is
// unset. A per-request 503 would let a misconfigured deploy come up and
// only fail when someone tried to log in; panicking at boot means the
// routes can never be mounted unauthenticated in the first place.

// internalAuthorized reports whether the request presented the shared
// internal secret. A handler configured with no secret (tests, and only
// tests — see the boot guard above) accepts everything.
func internalAuthorized(r *http.Request, secret string) bool {
	if secret == "" {
		return true
	}
	return internalauth.Equal(r.Header.Get(internalauth.Header), secret)
}

// writeUnauthorized emits the single response used for BOTH a missing and a
// wrong header. They must stay indistinguishable: a caller that can tell
// "no header" from "wrong header" apart learns whether a secret is
// configured at all.
func writeUnauthorized(w http.ResponseWriter) {
	writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
}
