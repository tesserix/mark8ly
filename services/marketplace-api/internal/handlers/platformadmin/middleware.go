package platformadmin

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
		if isWrite(strings.ToUpper(strings.TrimSpace(c.Request.Method))) {
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
		return time.Time{}, errors.New("platformadmin: timestamp is empty")
	}
	if ts[0] == '+' || ts[0] == '-' {
		return time.Time{}, errors.New("platformadmin: timestamp must be an unsigned decimal integer")
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
