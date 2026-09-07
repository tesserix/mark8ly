package platformauth

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Context keys set by RequirePlatformAuth and read by audit.buildEntry.
const (
	CtxOperatorID = "platform_operator_id"
	CtxCapability = "platform_capability"
)

// defaultWindow bounds how far a request's timestamp may be from ours.
const defaultWindow = 5 * time.Minute

// maxBodyBytes caps what we will buffer to hash. The platform API reads our
// responses through a 1 MiB limit; matching it on the request side keeps a
// hostile or buggy caller from making us allocate without bound.
const maxBodyBytes = 1 << 20

// maxIdentityLen bounds the signed Operator and Capability fields. The
// columns they eventually land in (via audit.buildEntry) are TEXT, so this
// is a sanity bound against a buggy or compromised gateway writing
// unbounded junk into an integrity record — not a schema constraint.
const maxIdentityLen = 256

// CapabilityValueChecked reports whether this surface enforces the VALUE
// of the capability a write presents, as opposed to merely its PRESENCE.
//
// It is FALSE. Presence is enforced in RequirePlatformAuth below; value is
// not. #288's acceptance asks for "the highest-privilege capability the
// gateway can assert", which is not expressible until the console's
// capability vocabulary is settled — the same blocker as #333, and the
// reason #287 declined to invent capability names.
//
// This is a SWITCH, not a marker: RequirePlatformAuth's write branch reads
// it. Flipping it to true, with RequiredWriteCapabilities' values filled
// in, turns value enforcement on for every write on this surface with no
// other edit. It lives here, beside the check it controls and beside
// RequiredWriteCapabilities itself, so that reading the enforcement matrix
// in this file tells you the whole truth about it.
const CapabilityValueChecked = false

// CapabilityKey builds the lookup key that RequiredWriteCapabilities (and
// RequirePlatformAuth's write branch, below) use: the write's HTTP method,
// upper-cased, plus the route TEMPLATE gin matched — exactly what
// c.FullPath() returns once routing has resolved (populated by the time
// this middleware runs, per gin's docs), not the literal request path.
// Keying on the template rather than the literal path is what lets one map
// entry cover every tenant this route is ever called for.
//
// Exported so tests build the SAME key this file computes internally,
// rather than a hand-written string that could silently drift from it —
// see routes_capability_coverage_test.go.
func CapabilityKey(method, routeTemplate string) string {
	return strings.ToUpper(strings.TrimSpace(method)) + " " + routeTemplate
}

// MountPrefix is the path prefix this surface is always mounted under —
// see the Register doc comment in routes.go for why it must be
// /api/v1/platform and never /api/v1/admin. It is the single source for
// that literal: RequiredReadCapabilities below, cmd/marketplace-api/main.go
// (both call sites), and routes_capability_coverage_test.go all build off
// this constant so that changing the mount prefix in one of them cannot
// silently drift from the others and ungate break-glass.
const MountPrefix = "/api/v1/platform"

// RequiredWriteCapabilities is the per-route capability enforcement
// matrix. It replaces what used to be a single global RequiredWriteCapability
// string: one value could never express that this surface's four writes
// are NOT equally privileged (#288's tenant purge needs "the
// highest-privilege capability the gateway can assert" — a claim a single
// shared value cannot make against three lesser writes at the same time).
//
// Match rule is EXACT STRING EQUALITY against ONE required capability per
// route — not a set, not a lattice, not a prefix match. Per #275, mark8ly
// does not own the privilege model: the console asserts the capability for
// the action it is performing, and this surface's job is only to record it
// and refuse a mismatch, never to reason about what a capability implies
// or ranks against another.
//
// Every value below is EMPTY, and CapabilityValueChecked (above) is FALSE:
// this map exists to have the right SHAPE — one declared entry per write
// route — not the right VALUES. #287 declined to invent capability names
// and #275 says the vocabulary belongs to the console, so no value here is
// a guess; #333's entire job is filling these in once that vocabulary is
// settled, not reshaping this map.
//
// Every write route Register (routes.go) can mount MUST have an entry
// here, even while every value is empty. TestAllWriteRoutesDeclareACapability
// (routes_capability_coverage_test.go) builds the real router with every
// write route wired and fails the build — naming the offending route — if
// one is undeclared, so a route added without a line here is caught here,
// not as a production 403 the day #333 turns enforcement on.
var RequiredWriteCapabilities = map[string]string{
	CapabilityKey(http.MethodPost, "/api/v1/platform/admin/billing/trials/:storeID/extend"): "",
	CapabilityKey(http.MethodPost, "/api/v1/platform/admin/tenants/:id/suspend"):            "",
	CapabilityKey(http.MethodPost, "/api/v1/platform/admin/tenants/:id/unsuspend"):          "",
	CapabilityKey(http.MethodPost, "/api/v1/platform/admin/tenants/:id/purge"):              "",
	// #405: outbox requeue (single + batch) and dead-letter.
	CapabilityKey(http.MethodPost, "/api/v1/platform/admin/outbox/:id/requeue"):     "",
	CapabilityKey(http.MethodPost, "/api/v1/platform/admin/outbox/requeue"):         "",
	CapabilityKey(http.MethodPost, "/api/v1/platform/admin/outbox/:id/dead-letter"): "",
	// tesserix-home#588: the email template registry. The PUT changes what
	// every merchant receives; the test-send puts a real message through
	// the production provider at an operator-chosen address. Both are
	// writes by isWrite's rule and both need a declaration here.
	CapabilityKey(http.MethodPut, "/api/v1/platform/admin/email-templates/:key"):            "",
	CapabilityKey(http.MethodPost, "/api/v1/platform/admin/email-templates/:key/test-send"): "",
	// #660: apply and revoke a console-minted tenant discount. BOTH
	// directions are declared — a revoke is as much a billing change as a
	// grant. The revoke is a POST on its own path, not a DELETE on the
	// apply's path; see the Register doc comment in
	// billing_tenant_discount.go.
	CapabilityKey(http.MethodPost, "/api/v1/platform/admin/billing/tenants/:tenantID/discount"):        "",
	CapabilityKey(http.MethodPost, "/api/v1/platform/admin/billing/tenants/:tenantID/discount/remove"): "",
}

// RequiredReadCapabilities declares reads that require a specific
// capability VALUE, not merely a valid signature.
//
// Unlike RequiredWriteCapabilities this map is NOT gated on
// CapabilityValueChecked and its values are NOT empty: it is opt-in per
// route, and a read route ABSENT from it keeps today's behaviour —
// signature only, no operator required, no capability required. That
// undeclared-route behaviour is load-bearing: every read this surface
// already serves in production (/admin/audit-logs, /admin/billing/subscriptions,
// /admin/billing/trials, /admin/health) has no entry here and must keep
// working exactly as it does today.
//
// That asymmetry with the write map is deliberate, not an oversight:
// flipping value enforcement on for every write at once is #364's problem,
// and it needs the console to send more than one capability value before
// that switch can be turned on safely. Requiring one named capability on
// one route does not carry that same requirement, so this map can enforce
// today without waiting on #364.
//
// What this pins on the console: platform-api has no break-glass module
// yet, so nothing that succeeds today is refused by this entry. But the
// audit module currently sends "platform" as its X-Platform-Capability
// value, and that is NOT the value break-glass must send. When the
// break-glass module is built, it MUST send "rotate-credentials" — matching
// what the console's own route config already declares for
// platform.breakGlass — or every request it makes will be refused with 403
// capability_insufficient. Whoever builds that module should find this
// requirement here, in the code, rather than discover it as a production
// 403.
var RequiredReadCapabilities = map[string]string{
	CapabilityKey(http.MethodGet, MountPrefix+"/admin/break-glass"): "rotate-credentials",
}

// AuthConfig configures RequirePlatformAuth.
type AuthConfig struct {
	// Secret is the shared HMAC key. Empty means NOT CONFIGURED, and every
	// request is refused with 503 — this surface fails closed.
	Secret string
	// NonceStore records nonces for replay defence. Required when Secret is set.
	NonceStore NonceStore
	// Now is injectable for tests. Defaults to time.Now.
	Now func() time.Time
	// Window overrides the +/- timestamp tolerance. Defaults to 5 minutes.
	Window time.Duration
	// Logger receives rejection detail. Optional.
	Logger *slog.Logger

	// CapabilityChecked overrides CapabilityValueChecked for this
	// instance of RequirePlatformAuth. Nil — the value every production
	// caller (Register in routes.go, and cmd/marketplace-api/main.go,
	// which never sets this field) leaves it at — falls back to
	// CapabilityValueChecked, the actual production switch, which stays
	// false. This exists ONLY so middleware_test.go can exercise both
	// arms of the capability-value gate without editing the production
	// constant; it is not a runtime knob production wiring is meant to
	// use. See TestAuthConfigCapabilityCheckedNilDefaultsToProductionOff
	// for the proof that leaving it unset really does mean "off".
	CapabilityChecked *bool

	// RequiredCapabilities overrides RequiredWriteCapabilities for this
	// instance of RequirePlatformAuth. Nil — again, every production
	// caller's value — falls back to the real RequiredWriteCapabilities
	// matrix declared above. Tests use this to exercise match, mismatch,
	// and undeclared-route behaviour with non-empty capability values
	// without touching the production matrix, whose values #364 requires
	// stay empty until #333.
	RequiredCapabilities map[string]string

	// RequiredReadCaps overrides RequiredReadCapabilities for this
	// instance of RequirePlatformAuth. Nil — again, every production
	// caller's value — falls back to the real RequiredReadCapabilities map
	// declared above. Tests use this to exercise declared-and-matched,
	// declared-and-mismatched, and undeclared-read behaviour without
	// touching the production map, which only break-glass needs today.
	RequiredReadCaps map[string]string
}

// RequirePlatformAuth verifies the gateway signature, enforces the replay
// window, and extracts the acting operator and the capability being
// exercised (#275).
//
// Deliberately NOT modelled on internalsvc.RequireInternalAuth, which no-ops
// when its secret is empty. That permissive branch is right for its purpose;
// on a surface serving cross-tenant tenant, billing and audit data it would
// mean an unconfigured deploy is wide open.
func RequirePlatformAuth(cfg AuthConfig) gin.HandlerFunc {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	window := cfg.Window
	if window <= 0 {
		window = defaultWindow
	}

	return func(c *gin.Context) {
		if cfg.Secret == "" || cfg.NonceStore == nil {
			abort(c, http.StatusServiceUnavailable, "not_configured",
				"platform admin surface is not configured")
			return
		}

		body, err := readAndRestoreBody(c)
		if err != nil {
			abort(c, http.StatusBadRequest, "invalid_request", "request body could not be read")
			return
		}

		in := SignatureInput{
			Method:     c.Request.Method,
			Path:       c.Request.URL.Path,
			RawQuery:   c.Request.URL.RawQuery,
			Body:       body,
			Timestamp:  c.GetHeader(HeaderTimestamp),
			Nonce:      c.GetHeader(HeaderNonce),
			Operator:   c.GetHeader(HeaderOperator),
			Capability: c.GetHeader(HeaderCapability),
		}

		// Signature, timestamp and nonce failures all return the same code.
		// Distinguishing them tells an attacker which half of the check they
		// passed; the detail goes to our logs instead.
		presented := c.GetHeader(HeaderSignature)
		if presented == "" || in.Timestamp == "" || in.Nonce == "" {
			reject(c, cfg.Logger, "missing signature headers")
			return
		}
		if len(in.Operator) > maxIdentityLen || len(in.Capability) > maxIdentityLen {
			// Same opaque path as every other rejection here — this is a
			// sanity bound on a buggy/compromised gateway, not a new
			// failure mode an attacker should be able to distinguish.
			reject(c, cfg.Logger, "operator or capability exceeds max length")
			return
		}

		// Parsed once: the same instant backs both the window check and the
		// nonce TTL below, so the two can never disagree about when this
		// request stops being valid.
		signedTS, err := parseTimestamp(in.Timestamp)
		if err != nil || !withinWindow(signedTS, now(), window) {
			reject(c, cfg.Logger, "timestamp outside window")
			return
		}

		ok, err := Verify(cfg.Secret, presented, in)
		if err != nil || !ok {
			reject(c, cfg.Logger, "signature mismatch")
			return
		}

		// Claim AFTER the signature verifies, so an unauthenticated caller
		// cannot burn nonces the real gateway might later use.
		//
		// The TTL is anchored to the signed timestamp, not to arrival time.
		// A request signed at signedTS stays signature-valid for the whole
		// window around it — including the future-dated edge, which can
		// still arrive here well inside tolerance. Anchoring the TTL to
		// "now" would let a captured request outlive its own row once
		// something starts sweeping expired nonces, making it replayable
		// again. signedTS.Add(window) is exactly the instant this request
		// stops being signature-valid, so the row can never expire before
		// the request itself does.
		fresh, err := cfg.NonceStore.Claim(c.Request.Context(), in.Nonce, signedTS.Add(window))
		if err != nil || !fresh {
			reject(c, cfg.Logger, "nonce replayed or unverifiable")
			return
		}

		// Authority is asserted upstream by the console and the gateway.
		// Mark8ly records it and refuses its absence — it never infers one.
		// Normalise case here the same way CanonicalString does (it upper-
		// cases Method before signing), so the method the HMAC covers is
		// exactly the method write-enforcement classifies.
		writeMethod := strings.ToUpper(strings.TrimSpace(c.Request.Method))
		if isWrite(writeMethod) {
			if in.Operator == "" {
				abort(c, http.StatusUnauthorized, "operator_required",
					"write requests must carry an operator identity")
				return
			}
			if in.Capability == "" {
				abort(c, http.StatusUnauthorized, "capability_required",
					"write requests must carry a capability")
				return
			}

			// Capability PRESENCE is enforced (above). Capability VALUE is
			// NOT — pending #333. Every write on this surface, including
			// the irreversible tenant purge (#288), is admitted on ANY
			// non-empty capability string; the value is recorded on the
			// audit row and gates nothing.
			//
			// That is deliberate, not an oversight: the console's
			// capability vocabulary is not settled, and hard-coding a
			// value here would refuse every real request. This block is
			// the gate, wired and inert: when #333 lands, fill in
			// RequiredWriteCapabilities' values and flip
			// CapabilityValueChecked, and enforcement starts here —
			// nothing else needs writing.
			capabilityChecked := CapabilityValueChecked
			if cfg.CapabilityChecked != nil {
				capabilityChecked = *cfg.CapabilityChecked
			}
			if capabilityChecked {
				requirements := RequiredWriteCapabilities
				if cfg.RequiredCapabilities != nil {
					requirements = cfg.RequiredCapabilities
				}

				// c.FullPath() is the matched route TEMPLATE (e.g.
				// ".../tenants/:id/purge"), not the literal request path —
				// see CapabilityKey's doc comment. It is populated by gin
				// before middleware runs.
				required, declared := requirements[CapabilityKey(writeMethod, c.FullPath())]
				if !declared {
					// Distinguishable from "capability_insufficient" below
					// on purpose: this means the GATEWAY hasn't declared
					// what this route needs, not that the CALLER presented
					// the wrong thing. The two point an operator at
					// different fixes — one at mark8ly's route
					// declaration, the other at the console's capability
					// assertion — and conflating them would send whoever
					// is debugging a production refusal to the wrong
					// place. Fails CLOSED: an undeclared write route is
					// refused, never admitted on any capability, so a
					// forgotten declaration is a refusal in production,
					// not a silent hole — the same posture this surface
					// already takes on a missing secret (503
					// not_configured, above).
					abort(c, http.StatusForbidden, "capability_route_undeclared",
						"this route has not declared a required capability")
					return
				}
				if in.Capability != required {
					abort(c, http.StatusForbidden, "capability_insufficient",
						"the presented capability does not authorize this request")
					return
				}
			}
		} else {
			// Reads are ungated by default — see RequiredReadCapabilities'
			// doc comment. Only a route with an entry in that map requires
			// an operator and a matching capability; every other read on
			// this surface (audit logs, billing subscriptions, billing
			// trials, health) has no entry and must proceed exactly as it
			// does today. This is the reverse posture from the write branch
			// above, deliberately: an UNDECLARED write fails closed because
			// every write must declare; an undeclared READ is simply an
			// ungated read, because gating every read has never been
			// required and this map exists to opt specific routes in, not
			// to opt every route out by default.
			readRequirements := RequiredReadCapabilities
			if cfg.RequiredReadCaps != nil {
				readRequirements = cfg.RequiredReadCaps
			}

			if required, declared := readRequirements[CapabilityKey(writeMethod, c.FullPath())]; declared {
				if in.Operator == "" {
					abort(c, http.StatusUnauthorized, "operator_required",
						"this read requires an operator identity")
					return
				}
				if in.Capability == "" {
					abort(c, http.StatusUnauthorized, "capability_required",
						"this read requires a capability")
					return
				}
				// Same exact-match rule as the write branch: no lattice, no
				// prefix match, no implication between capabilities — see
				// RequiredWriteCapabilities' doc comment and #275.
				if in.Capability != required {
					abort(c, http.StatusForbidden, "capability_insufficient",
						"the presented capability does not authorize this request")
					return
				}
			}
		}

		if in.Operator != "" {
			c.Set(CtxOperatorID, in.Operator)
		}
		if in.Capability != "" {
			c.Set(CtxCapability, in.Capability)
		}
		c.Next()
	}
}

func isWrite(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// parseTimestamp parses the signed timestamp as a strict decimal Unix
// second count. A leading '+' or '-' is rejected rather than silently
// accepted: strconv.ParseInt allows one, which would let "+1755859200"
// and "1755859200" denote the same instant with two different byte
// strings — a wart in a scheme published as the reference implementation
// for the console team, whose whole job is to be unambiguous.
func parseTimestamp(ts string) (time.Time, error) {
	if ts == "" {
		return time.Time{}, errors.New("platformauth: timestamp is empty")
	}
	if ts[0] == '+' || ts[0] == '-' {
		return time.Time{}, errors.New("platformauth: timestamp must be an unsigned decimal integer")
	}
	secs, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(secs, 0), nil
}

func withinWindow(signedTS, now time.Time, window time.Duration) bool {
	delta := now.Sub(signedTS)
	if delta < 0 {
		delta = -delta
	}
	return delta <= window
}

// readAndRestoreBody buffers the body so it can be hashed, then puts it back
// so the downstream handler can still bind it.
func readAndRestoreBody(c *gin.Context) ([]byte, error) {
	if c.Request.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxBodyBytes))
	if err != nil {
		return nil, err
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

func reject(c *gin.Context, logger *slog.Logger, reason string) {
	if logger != nil {
		logger.Warn("platform admin auth rejected",
			"reason", reason,
			"path", c.Request.URL.Path,
			"method", c.Request.Method)
	}
	abort(c, http.StatusUnauthorized, "unauthenticated", "platform authentication failed")
}

func abort(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": code, "message": message})
}
