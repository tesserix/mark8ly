package admin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/mark8ly/marketplace-api/internal/authbffclient"
	"github.com/mark8ly/marketplace-api/internal/breakglass"
)

// BreakGlassSessionTTL is the lifetime of a session minted by a
// successful break-glass login. Short by design — this is an
// emergency access path, not a daily-driver login.
const BreakGlassSessionTTL = 2 * time.Hour

// BreakGlassDeps groups every dependency the login handler needs.
type BreakGlassDeps struct {
	Repo        *breakglass.Repository
	Secrets     *breakglass.SecretManager
	Audit       *breakglass.AuditEmitter
	Slack       *breakglass.SlackClient
	RateLimiter *breakglass.LoginRateLimiter
	IPHMACKey   breakglass.HMACKey
	Sessions    authbffclient.SessionIssuer
	Logger      *slog.Logger
}

// BreakGlassLoginHandler answers POST /admin/break-glass/login.
// Mount OUTSIDE the store-scoped RequireActive group: this endpoint
// exists precisely to survive read-only / store_closed subscription
// states (§12.4).
type BreakGlassLoginHandler struct {
	deps BreakGlassDeps
}

// NewBreakGlassLoginHandler constructs the handler. Any nil field in
// deps is a programmer error; the handler nil-checks only `Sessions`
// since it's legitimately optional in tests.
func NewBreakGlassLoginHandler(deps BreakGlassDeps) *BreakGlassLoginHandler {
	if deps.Sessions == nil {
		deps.Sessions = authbffclient.NoopIssuer{}
	}
	return &BreakGlassLoginHandler{deps: deps}
}

// breakGlassLoginRequest is the single-step dual-factor body. Both
// factors arrive in one POST so no partial response can leak which
// side failed.
type breakGlassLoginRequest struct {
	TenantID string `json:"tenant_id" binding:"required,uuid"`
	Password string `json:"password" binding:"required,min=1"`
	TOTPCode string `json:"totp_code" binding:"required,len=6"`
}

// Login is the Gin handler. The response body on failure is always
// uniform ({"error":"invalid_credentials"}) regardless of which
// factor tripped — forensics lives in the audit log, not the HTTP
// response.
func (h *BreakGlassLoginHandler) Login(c *gin.Context) {
	ctx := c.Request.Context()

	ip := breakglass.ClientIPFromRequest(c)
	ipHash := breakglass.HMACIPHash(h.deps.IPHMACKey, ip)

	// 1. Fast-path: is this IP currently hard-locked? Returned early
	// so timing can't distinguish locked vs. unlocked states.
	locked, err := h.deps.Repo.IsIPLocked(ctx, ipHash)
	if err != nil {
		// The lockout store is unreachable. This used to fail open
		// unconditionally: the error was logged at Warn and `locked` stayed
		// false, so an existing lockout went unenforced (#468). #457 showed
		// that is not hypothetical — every read failed for weeks and the
		// only sign was a Warn line nobody read.
		//
		// Degrade to the in-memory limiter rather than choosing one extreme.
		// It is per-pod and resets on deploy, but it is the signal still
		// available, and it refuses exactly the IPs that have earned it.
		recent := h.deps.RateLimiter.Count(rlKey(ipHash))
		locked = degradedLockDecision(recent)
		h.logger().Error("break-glass: lockout lookup failed; degraded to the in-memory limiter",
			"err", err, "recent_failures", recent, "treated_as_locked", locked)
	}
	if locked {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "locked"})
		return
	}

	// 2. Parse + shape-validate.
	var req breakGlassLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.recordFailure(c, ipHash, uuid.Nil, "bad_request")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_credentials"})
		return
	}
	tenantID, err := uuid.Parse(req.TenantID)
	if err != nil {
		h.recordFailure(c, ipHash, uuid.Nil, "bad_tenant_id")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_credentials"})
		return
	}

	// 3. Fetch account — no row = same response as wrong credentials.
	acc, err := h.deps.Repo.GetByTenant(ctx, tenantID)
	if err != nil {
		reason := "no_account"
		if !errors.Is(err, breakglass.ErrNotFound) {
			reason = "db_error"
			h.logger().Error("break-glass: db lookup failed", "err", err)
		}
		h.recordFailure(c, ipHash, tenantID, reason)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_credentials"})
		return
	}

	// 3.5. Refuse a disabled account with a response BYTE-IDENTICAL to a
	// wrong-password failure (#404). No new dependency on this path:
	// DisabledAt is a field on `acc`, the row step 3 already fetched — see
	// the package constraint that nothing added here may introduce a new
	// query or service call. Sits BEFORE the Secret Manager fetch (step 4)
	// so a disabled account is refused even when Secret Manager is
	// unreachable.
	if acc.DisabledAt != nil {
		h.recordFailure(c, ipHash, tenantID, "account_disabled")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_credentials"})
		return
	}

	// 4. Fetch blob from Secret Manager. Fetch failure is a 500 (ops
	// needs to see it, not a security event).
	blob, err := h.deps.Secrets.Fetch(ctx, acc.SecretPath)
	if err != nil {
		h.logger().Error("break-glass: secret manager fetch failed", "err", err, "tenant", tenantID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "secret_fetch_failed"})
		return
	}

	// 5. Dual-factor verify — password AND TOTP in one step.
	pwOK := bcrypt.CompareHashAndPassword([]byte(acc.PasswordHash), []byte(req.Password)) == nil
	totpOK := breakglass.VerifyTOTP(blob.TOTPSecret, req.TOTPCode, time.Now())

	if !(pwOK && totpOK) {
		reason := "both_wrong"
		switch {
		case pwOK && !totpOK:
			reason = "totp_wrong"
		case !pwOK && totpOK:
			reason = "password_wrong"
		}
		h.recordFailure(c, ipHash, tenantID, reason)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_credentials"})
		return
	}

	// 6. Mint session cookie via auth-bff. The synthetic user id
	// (UUIDv5 over a break-glass namespace + tenant) is stable across
	// sessions so ops audit logs consistently tie the same actor to
	// every break-glass login for a tenant.
	userID := BreakGlassUserID(tenantID)
	setCookie, err := h.deps.Sessions.Issue(ctx, tenantID, userID, "break_glass", BreakGlassSessionTTL)
	if err != nil {
		h.logger().Error("break-glass: session issue failed", "err", err, "tenant", tenantID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "session_mint_failed"})
		return
	}
	if setCookie != "" {
		c.Writer.Header().Add("Set-Cookie", setCookie)
	}

	// 7. Post-use hooks — rotation schedule, audit, Slack. Each is
	// best-effort; the caller must see 200 even if one hook fails.
	if err := h.deps.Repo.UpdateAfterUse(ctx, tenantID); err != nil {
		h.logger().Warn("break-glass: UpdateAfterUse failed", "err", err, "tenant", tenantID)
	}
	if h.deps.Audit != nil {
		h.deps.Audit.EmitLogin(c, tenantID, true, userID.String(), "")
	}
	if h.deps.Slack != nil {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = h.deps.Slack.PostLoginAlert(bgCtx, tenantID, true)
		}()
	}

	h.deps.RateLimiter.Reset(rlKey(ipHash))

	c.JSON(http.StatusOK, gin.H{"session_ttl_seconds": int(BreakGlassSessionTTL.Seconds())})
}

// recordFailure increments the in-memory counter, persists a 24h
// lockout row on the Nth failure, and emits a critical-severity audit
// event. Slack is also pinged so on-call sees the attempt in real
// time.
func (h *BreakGlassLoginHandler) recordFailure(c *gin.Context, ipHash []byte, tenantID uuid.UUID, reason string) {
	count := h.deps.RateLimiter.RecordFailure(rlKey(ipHash))

	if count >= breakglass.LoginMaxFailures {
		var tidPtr *uuid.UUID
		if tenantID != uuid.Nil {
			tidPtr = &tenantID
		}
		if err := h.deps.Repo.LockIP(c.Request.Context(), ipHash, tidPtr, reason, breakglass.LoginLockoutDuration); err != nil {
			// Error, not Warn (#468). This is the durable half of a security
			// control failing: the 24h lockout is not persisted, so it will
			// not survive this pod, and every other replica remains unaware.
			// It was logged at Warn while broken for weeks and nobody saw it.
			h.logger().Error("break-glass: lockout NOT persisted; the 24h hard lockout is not in effect",
				"err", err, "failures_in_window", count)
		}
	}

	if h.deps.Audit != nil && tenantID != uuid.Nil {
		h.deps.Audit.EmitLogin(c, tenantID, false, "", reason)
	}
	if h.deps.Slack != nil && tenantID != uuid.Nil {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = h.deps.Slack.PostLoginAlert(bgCtx, tenantID, false)
		}()
	}
}

func (h *BreakGlassLoginHandler) logger() *slog.Logger {
	if h.deps.Logger != nil {
		return h.deps.Logger
	}
	return slog.Default()
}

// rlKey shapes the rate-limit bucket key. Keeping it tiny (hex first
// 16 bytes of ip_hash) avoids memory bloat across many concurrent IPs.
func rlKey(ipHash []byte) string {
	n := len(ipHash)
	if n > 16 {
		n = 16
	}
	return string(ipHash[:n])
}

// breakGlassNamespace is a fixed UUIDv5 namespace for synthesising
// deterministic user IDs from tenant IDs. Any UUID works — the point
// is a stable input that never collides with real user IDs.
var breakGlassNamespace = uuid.MustParse("1ffb2c0e-8c3a-4d78-9aee-62267a3c110a")

// BreakGlassUserID returns the synthetic user id for a tenant's
// break-glass actor. UUIDv5 over the fixed namespace guarantees the
// value is stable and unforgeable (you can't pick a tenant id that
// collides with a real user).
func BreakGlassUserID(tenantID uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(breakGlassNamespace, []byte(tenantID.String()))
}

// degradedLockDecision decides whether to treat a login as locked when the
// lockout store could not be read (#468).
//
// Neither extreme is right. Failing open unconditionally — the old
// behaviour — means an existing lockout is silently unenforced for as long
// as the store is unreachable, which #457 showed can be weeks. Failing
// closed unconditionally turns a database blip into "nobody can break-glass
// in", precisely when break-glass is most likely to be needed: this
// endpoint is mounted outside the store-scoped group specifically to
// survive states other paths cannot.
//
// So it falls back to the in-memory limiter. That counter is per-pod and
// resets on deploy, so it is a weaker signal than the table — but it is the
// signal still available, and it refuses only IPs that have already reached
// the same threshold a persisted lockout would have used. An operator whose
// IP has done nothing wrong is unaffected.
func degradedLockDecision(recentFailures int) bool {
	return recentFailures >= breakglass.LoginMaxFailures
}
