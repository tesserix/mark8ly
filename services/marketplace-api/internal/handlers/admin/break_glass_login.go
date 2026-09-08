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
	Repo    *breakglass.Repository
	Secrets *breakglass.SecretManager
	Audit   *breakglass.AuditEmitter
	Slack   *breakglass.SlackClient
	// RateLimiter is the IN-MEMORY window, and it is required.
	//
	// It is no longer the counter of record — see Window — but it cannot be
	// dropped: it is the signal `degradedLockDecision` reads when the lockout
	// store is unreachable, which is exactly the moment Window (also
	// Postgres-backed) has nothing to offer either. It is therefore stamped
	// on every failure even while Window is healthy, so it is warm when it
	// is needed.
	RateLimiter *breakglass.LoginRateLimiter
	// Window is the ESTATE-WIDE window, backed by break_glass_login_attempts.
	//
	// Optional: nil falls back to RateLimiter alone, which is the pre-#846
	// behaviour and is correct on a single-replica deployment. With more than
	// one replica it must be set, or the 3-strike threshold becomes 3 strikes
	// PER POD.
	Window    breakglass.LoginWindow
	IPHMACKey breakglass.HMACKey
	Sessions  authbffclient.SessionIssuer
	Logger    *slog.Logger
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
		// The IN-MEMORY window deliberately, not Window: the branch we are in
		// is "Postgres could not be read", so the durable window has nothing
		// to add and asking it would just fail again.
		recent, _ := h.deps.RateLimiter.Count(ctx, breakglass.LoginKey{IPHash: ipHash})
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

	// Both windows, because both are consulted: the durable one decides the
	// threshold and the in-memory one is the degraded fallback. Clearing only
	// one leaves a successful login still counted against the next attempt by
	// whichever window was missed.
	h.resetWindows(ctx, ipHash)

	c.JSON(http.StatusOK, gin.H{"session_ttl_seconds": int(BreakGlassSessionTTL.Seconds())})
}

// recordFailure increments the in-memory counter, persists a 24h
// lockout row on the Nth failure, and emits a critical-severity audit
// event. Slack is also pinged so on-call sees the attempt in real
// time.
func (h *BreakGlassLoginHandler) recordFailure(c *gin.Context, ipHash []byte, tenantID uuid.UUID, reason string) {
	count := h.recordAcrossWindows(c.Request.Context(), ipHash)

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

// recordAcrossWindows stamps the failure on both windows and returns the count
// the threshold is judged on.
//
// The in-memory window is stamped FIRST and unconditionally. It is the
// degraded signal, and a durable window that is healthy today must not leave
// it cold for the day the database is not — `degradedLockDecision` reads it
// precisely when Window cannot answer.
//
// The DURABLE count wins when it is available, because it is the only one
// that is a property of the estate rather than of this pod. When it errors,
// the in-memory count is used rather than zero: falling back to "no failures
// recorded" would make a database blip into a bypass of the whole threshold.
func (h *BreakGlassLoginHandler) recordAcrossWindows(ctx context.Context, ipHash []byte) int {
	key := breakglass.LoginKey{IPHash: ipHash}

	local, _ := h.deps.RateLimiter.RecordFailure(ctx, key)
	if h.deps.Window == nil {
		return local
	}

	durable, err := h.deps.Window.RecordFailure(ctx, key)
	if err != nil {
		// Error, not Warn, for the reason #468 gives about the lockout write:
		// this is the durable half of a security control failing, and the
		// symptom (a threshold counted per-pod again) is invisible otherwise.
		h.logger().Error("break-glass: durable login window unavailable; counting per-pod",
			"err", err, "failures_in_window", local)
		return local
	}
	return durable
}

// resetWindows clears both windows for an ip_hash. Errors are logged, never
// returned: this runs after a SUCCESSFUL login, and refusing the session
// because a cleanup delete failed would deny an operator the emergency access
// they just proved they were entitled to. The cost of a missed clear is that
// the IP keeps stale failures until they age out of the window.
func (h *BreakGlassLoginHandler) resetWindows(ctx context.Context, ipHash []byte) {
	key := breakglass.LoginKey{IPHash: ipHash}
	_ = h.deps.RateLimiter.Reset(ctx, key)
	if h.deps.Window == nil {
		return
	}
	if err := h.deps.Window.Reset(ctx, key); err != nil {
		h.logger().Error("break-glass: durable login window not cleared after a successful login",
			"err", err)
	}
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
